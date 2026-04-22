package main

import (
	"fmt"
	"net/http"
	"sync"

	"github.com/gorilla/mux"
	"github.com/mattermost/mattermost-plugin-ai/public/bridgeclient"
	mmmodel "github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
	"github.com/mattermost/mattermost/server/public/pluginapi"
	"github.com/mattermost/mattermost/server/public/pluginapi/cluster"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/command"
	"github.com/mattermost/mattermost-plugin-channel-automation/server/execution"
	"github.com/mattermost/mattermost-plugin-channel-automation/server/flow"
	"github.com/mattermost/mattermost-plugin-channel-automation/server/flow/action"
	"github.com/mattermost/mattermost-plugin-channel-automation/server/flow/trigger"
	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
	"github.com/mattermost/mattermost-plugin-channel-automation/server/workqueue"
)

// Plugin implements the interface expected by the Mattermost server to communicate between the server and plugin processes.
type Plugin struct {
	plugin.MattermostPlugin

	// client is the Mattermost server API client.
	client *pluginapi.Client

	// commandClient is the client used to register and execute slash commands.
	commandClient command.Command

	// router is the HTTP router for handling API requests.
	router *mux.Router

	workQueueStore *workqueue.Store
	workerPool     *workqueue.WorkerPool

	bridgeClient *bridgeclient.Client

	botUserID       string
	registry        *flow.Registry
	flowStore       model.Store
	historyStore    model.ExecutionStore
	triggerService  *flow.TriggerService
	flowExecutor    *flow.FlowExecutor
	scheduleManager *flow.ScheduleManager

	// configurationLock synchronizes access to the configuration.
	configurationLock sync.RWMutex

	// configuration is the active plugin configuration. Consult getConfiguration and
	// setConfiguration for usage.
	configuration *configuration
}

// OnActivate is invoked when the plugin is activated. If an error is returned, the plugin will be deactivated.
func (p *Plugin) OnActivate() error {
	if !pluginapi.IsEnterpriseLicensedOrDevelopment(p.API.GetConfig(), p.API.GetLicense()) {
		return fmt.Errorf("this plugin requires an Enterprise license")
	}

	p.client = pluginapi.NewClient(p.API, p.Driver)

	botUserID, err := p.client.Bot.EnsureBot(&mmmodel.Bot{
		Username:    "channel-automation-bot",
		DisplayName: "Channel Automation",
		Description: "I can help you automate things in channels.",
	})
	if err != nil {
		return err
	}
	p.botUserID = botUserID

	bc := bridgeclient.NewClient(p.API)
	p.bridgeClient = bc

	// TODO: Register tools in the bridge client

	p.registry = flow.NewRegistry()
	p.registry.RegisterTrigger(&trigger.MessagePostedTrigger{})
	p.registry.RegisterTrigger(&trigger.ScheduleTrigger{})
	p.registry.RegisterTrigger(&trigger.MembershipChangedTrigger{})
	p.registry.RegisterTrigger(&trigger.ChannelCreatedTrigger{})
	p.registry.RegisterTrigger(&trigger.UserJoinedTeamTrigger{})
	p.registry.RegisterAction(action.NewSendMessageAction(p.API, p.botUserID))
	p.registry.RegisterAction(action.NewAIPromptAction(p.API, bc))

	flowIndexMu, err := cluster.NewMutex(p.API, "flow_index_mutex")
	if err != nil {
		return fmt.Errorf("failed to create flow index mutex: %w", err)
	}
	p.flowStore = flow.NewStore(p.API, flowIndexMu)
	p.triggerService = flow.NewTriggerService(p.flowStore, p.registry)
	p.flowExecutor = flow.NewFlowExecutor(p.registry)

	// Set up persistent work queue.
	indexMu, err := cluster.NewMutex(p.API, "wq_index_mutex")
	if err != nil {
		return fmt.Errorf("failed to create work queue mutex: %w", err)
	}
	p.workQueueStore = workqueue.NewStore(p.API, indexMu)

	xhIndexMu, err := cluster.NewMutex(p.API, "xh_index_mutex")
	if err != nil {
		return fmt.Errorf("failed to create execution history mutex: %w", err)
	}
	p.historyStore = execution.NewStore(p.API, xhIndexMu)

	resetCount, err := p.workQueueStore.ResetRunningToPending()
	if err != nil {
		p.API.LogError("Failed to reset running work items", "err", err.Error())
	} else if resetCount > 0 {
		p.API.LogInfo("Reset orphaned work items to pending", "count", resetCount)
	}

	maxWorkers := p.getConfiguration().MaxConcurrentFlowsLimit
	if maxWorkers <= 0 {
		maxWorkers = 4
	}

	p.workerPool = workqueue.NewWorkerPool(p.workQueueStore, p.flowExecutor, p.flowStore, p.historyStore, p.API, maxWorkers)
	p.workerPool.Start()

	// Start schedule manager after worker pool so scheduled items can be processed.
	p.scheduleManager = flow.NewScheduleManager(p.API, p.flowStore, p.workQueueStore, p.workerPool)
	if err := p.scheduleManager.Start(); err != nil {
		p.API.LogError("Failed to start schedule manager", "err", err.Error())
	}

	// Initialize router last — it depends on scheduleManager.
	p.router = p.initRouter()

	return nil
}

// OnDeactivate is invoked when the plugin is deactivated.
func (p *Plugin) OnDeactivate() error {
	// Stop schedule manager first — it may be enqueuing items.
	if p.scheduleManager != nil {
		p.scheduleManager.Stop()
	}
	if p.workerPool != nil {
		p.workerPool.Stop()
	}
	return nil
}

// This will execute the commands that were registered in the NewCommandHandler function.
func (p *Plugin) ExecuteCommand(c *plugin.Context, args *mmmodel.CommandArgs) (*mmmodel.CommandResponse, *mmmodel.AppError) {
	response, err := p.commandClient.Handle(args)
	if err != nil {
		return nil, mmmodel.NewAppError("ExecuteCommand", "plugin.command.execute_command.app_error", nil, err.Error(), http.StatusInternalServerError)
	}
	return response, nil
}

// MessageHasBeenPosted is invoked after a message is posted. It finds matching
// flows and enqueues work items for asynchronous execution.
func (p *Plugin) MessageHasBeenPosted(_ *plugin.Context, post *mmmodel.Post) {
	// Ignore system messages, bot posts, and webhook posts to prevent
	// loops and unintended flow triggers.
	if post.UserId == p.botUserID {
		return
	}
	if post.IsSystemMessage() {
		return
	}
	if post.GetProp("from_webhook") == "true" {
		return
	}
	if post.GetProp("from_bot") == "true" {
		return
	}
	if post.GetProp("ai_generated_by") != nil {
		return
	}

	event := &model.Event{
		Type: model.TriggerTypeMessagePosted,
		Post: post,
	}

	flows, err := p.triggerService.FindMatchingFlows(event)
	if err != nil {
		p.API.LogError("Failed to find flows for post", "err", err.Error())
		return
	}
	if len(flows) == 0 {
		return
	}

	// Build trigger data once before the loop.
	channel, appErr := p.API.GetChannel(post.ChannelId)
	if appErr != nil {
		p.API.LogError("Failed to get channel for trigger", "channel_id", post.ChannelId, "err", appErr.Error())
		return
	}
	user, appErr := p.API.GetUser(post.UserId)
	if appErr != nil {
		p.API.LogError("Failed to get user for trigger", "user_id", post.UserId, "err", appErr.Error())
		return
	}

	triggerData := model.TriggerData{
		Post:    model.NewSafePost(post),
		Channel: model.NewSafeChannel(channel),
		User:    model.NewSafeUser(user),
	}

	if post.RootId != "" && anyFlowWantsThreadContext(flows) {
		triggerData.Thread = p.fetchThreadContext(post.RootId)
	}

	for _, f := range flows {
		item := &model.WorkItem{
			ID:          mmmodel.NewId(),
			FlowID:      f.ID,
			FlowName:    f.Name,
			TriggerData: triggerData,
		}

		if err := p.workQueueStore.Enqueue(item); err != nil {
			p.API.LogError("Failed to enqueue work item",
				"flow_id", f.ID,
				"flow_name", f.Name,
				"post_id", post.Id,
				"err", err.Error(),
			)
			continue
		}

		p.API.LogDebug("Work item enqueued",
			"work_item_id", item.ID,
			"flow_id", f.ID,
			"flow_name", f.Name,
			"post_id", post.Id,
		)
	}

	p.workerPool.Notify()
}

// anyFlowWantsThreadContext reports whether any flow in the list has opted
// into thread context on its message_posted trigger.
func anyFlowWantsThreadContext(flows []*model.Flow) bool {
	for _, f := range flows {
		if f.Trigger.MessagePosted != nil && f.Trigger.MessagePosted.IncludeThreadContext {
			return true
		}
	}
	return false
}

// fetchThreadContext retrieves the thread rooted at rootID and returns a
// SafeThread with usernames resolved (deduped by user ID). On error, logs a
// warning and returns nil — callers must tolerate a nil result.
func (p *Plugin) fetchThreadContext(rootID string) *model.SafeThread {
	list, appErr := p.API.GetPostThread(rootID)
	if appErr != nil {
		p.API.LogWarn("Failed to fetch thread for trigger context, continuing without it",
			"root_id", rootID,
			"err", appErr.Error(),
		)
		return nil
	}
	userFor := func(userID string) *model.SafeUser {
		u, err := p.API.GetUser(userID)
		if err != nil {
			p.API.LogDebug("Failed to resolve user for thread context",
				"user_id", userID,
				"err", err.Error(),
			)
			return nil
		}
		return model.NewSafeUser(u)
	}
	return model.NewSafeThread(list, rootID, userFor)
}

// UserHasJoinedChannel is invoked after a user joins a channel.
func (p *Plugin) UserHasJoinedChannel(_ *plugin.Context, member *mmmodel.ChannelMember, _ *mmmodel.User) {
	p.handleMembershipChange(member, "joined")
}

// UserHasLeftChannel is invoked after a user leaves a channel.
func (p *Plugin) UserHasLeftChannel(_ *plugin.Context, member *mmmodel.ChannelMember, _ *mmmodel.User) {
	p.handleMembershipChange(member, "left")
}

// handleMembershipChange is the shared logic for join/leave hooks.
func (p *Plugin) handleMembershipChange(member *mmmodel.ChannelMember, action string) {
	if member.UserId == p.botUserID {
		return
	}

	user, appErr := p.API.GetUser(member.UserId)
	if appErr != nil {
		p.API.LogError("Failed to get user for membership trigger", "user_id", member.UserId, "err", appErr.Error())
		return
	}
	if user.IsBot {
		return
	}

	channel, appErr := p.API.GetChannel(member.ChannelId)
	if appErr != nil {
		p.API.LogError("Failed to get channel for membership trigger", "channel_id", member.ChannelId, "err", appErr.Error())
		return
	}

	event := &model.Event{
		Type:             model.TriggerTypeMembershipChanged,
		Channel:          channel,
		User:             user,
		MembershipAction: action,
	}

	flows, err := p.triggerService.FindMatchingFlows(event)
	if err != nil {
		p.API.LogError("Failed to find flows for membership change", "err", err.Error())
		return
	}
	if len(flows) == 0 {
		return
	}

	triggerData := model.TriggerData{
		Channel:    model.NewSafeChannel(channel),
		User:       model.NewSafeUser(user),
		Membership: &model.MembershipInfo{Action: action},
	}

	for _, f := range flows {
		item := &model.WorkItem{
			ID:          mmmodel.NewId(),
			FlowID:      f.ID,
			FlowName:    f.Name,
			TriggerData: triggerData,
		}

		if err := p.workQueueStore.Enqueue(item); err != nil {
			p.API.LogError("Failed to enqueue work item",
				"flow_id", f.ID,
				"flow_name", f.Name,
				"channel_id", member.ChannelId,
				"user_id", member.UserId,
				"action", action,
				"err", err.Error(),
			)
			continue
		}

		p.API.LogDebug("Work item enqueued",
			"work_item_id", item.ID,
			"flow_id", f.ID,
			"flow_name", f.Name,
			"channel_id", member.ChannelId,
			"user_id", member.UserId,
			"action", action,
		)
	}

	p.workerPool.Notify()
}

// ChannelHasBeenCreated is invoked after a new channel is created.
// Only public channels (type "O") trigger flows.
func (p *Plugin) ChannelHasBeenCreated(_ *plugin.Context, channel *mmmodel.Channel) {
	if channel.Type != mmmodel.ChannelTypeOpen {
		return
	}

	event := &model.Event{
		Type:    model.TriggerTypeChannelCreated,
		Channel: channel,
	}

	flows, err := p.triggerService.FindMatchingFlows(event)
	if err != nil {
		p.API.LogError("Failed to find flows for channel creation", "err", err.Error())
		return
	}
	if len(flows) == 0 {
		return
	}

	triggerData := model.TriggerData{
		Channel: model.NewSafeChannel(channel),
	}

	if channel.CreatorId != "" {
		user, appErr := p.API.GetUser(channel.CreatorId)
		if appErr != nil {
			p.API.LogError("Failed to get user for channel creation trigger", "user_id", channel.CreatorId, "err", appErr.Error())
		} else {
			triggerData.User = model.NewSafeUser(user)
		}
	}

	for _, f := range flows {
		item := &model.WorkItem{
			ID:          mmmodel.NewId(),
			FlowID:      f.ID,
			FlowName:    f.Name,
			TriggerData: triggerData,
		}

		if err := p.workQueueStore.Enqueue(item); err != nil {
			p.API.LogError("Failed to enqueue work item",
				"flow_id", f.ID,
				"flow_name", f.Name,
				"channel_id", channel.Id,
				"err", err.Error(),
			)
			continue
		}

		p.API.LogDebug("Work item enqueued",
			"work_item_id", item.ID,
			"flow_id", f.ID,
			"flow_name", f.Name,
			"channel_id", channel.Id,
		)
	}

	p.workerPool.Notify()
}

// UserHasJoinedTeam is invoked after a user joins a team.
// The actor parameter (who performed the action) is ignored — we always
// resolve the joining user from teamMember.UserId, matching the pattern
// used by UserHasJoinedChannel/UserHasLeftChannel.
func (p *Plugin) UserHasJoinedTeam(_ *plugin.Context, teamMember *mmmodel.TeamMember, _ *mmmodel.User) {
	if teamMember.UserId == p.botUserID {
		return
	}

	user, appErr := p.API.GetUser(teamMember.UserId)
	if appErr != nil {
		p.API.LogError("Failed to get user for team join trigger", "user_id", teamMember.UserId, "err", appErr.Error())
		return
	}
	if user.IsBot {
		return
	}

	event := &model.Event{
		Type: model.TriggerTypeUserJoinedTeam,
		Team: &mmmodel.Team{Id: teamMember.TeamId},
		User: user,
	}

	flows, err := p.triggerService.FindMatchingFlows(event)
	if err != nil {
		p.API.LogError("Failed to find flows for team join", "team_id", teamMember.TeamId, "user_id", teamMember.UserId, "err", err.Error())
		return
	}
	if len(flows) == 0 {
		return
	}

	team, appErr := p.API.GetTeam(teamMember.TeamId)
	if appErr != nil {
		p.API.LogWarn("Failed to get team for team join trigger, continuing with partial data", "team_id", teamMember.TeamId, "err", appErr.Error())
	}

	safeTeam := model.NewSafeTeam(team)

	defaultChannel, appErr := p.API.GetChannelByName(teamMember.TeamId, mmmodel.DefaultChannelName, false)
	if appErr != nil {
		p.API.LogWarn("Failed to get default channel for team join trigger", "team_id", teamMember.TeamId, "err", appErr.Error())
	} else {
		safeTeam.DefaultChannelId = defaultChannel.Id
	}

	triggerData := model.TriggerData{
		User: model.NewSafeUser(user),
		Team: safeTeam,
	}

	for _, f := range flows {
		item := &model.WorkItem{
			ID:          mmmodel.NewId(),
			FlowID:      f.ID,
			FlowName:    f.Name,
			TriggerData: triggerData,
		}

		if err := p.workQueueStore.Enqueue(item); err != nil {
			p.API.LogError("Failed to enqueue work item",
				"flow_id", f.ID,
				"flow_name", f.Name,
				"team_id", teamMember.TeamId,
				"user_id", teamMember.UserId,
				"err", err.Error(),
			)
			continue
		}

		p.API.LogDebug("Work item enqueued",
			"work_item_id", item.ID,
			"flow_id", f.ID,
			"flow_name", f.Name,
			"team_id", teamMember.TeamId,
			"user_id", teamMember.UserId,
		)
	}

	p.workerPool.Notify()
}

// See https://developers.mattermost.com/extend/plugins/server/reference/
