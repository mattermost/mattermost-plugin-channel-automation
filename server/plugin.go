package main

import (
	"crypto/sha256"
	"encoding/hex"
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
	"github.com/mattermost/mattermost-plugin-channel-automation/server/flow"
	"github.com/mattermost/mattermost-plugin-channel-automation/server/flow/action"
	"github.com/mattermost/mattermost-plugin-channel-automation/server/flow/trigger"
	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
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

	backgroundJob *cluster.Job

	botUserID      string
	registry       *flow.Registry
	flowStore      model.Store
	triggerService *flow.TriggerService
	flowExecutor   *flow.FlowExecutor

	// configurationLock synchronizes access to the configuration.
	configurationLock sync.RWMutex

	// configuration is the active plugin configuration. Consult getConfiguration and
	// setConfiguration for usage.
	configuration *configuration
}

// OnActivate is invoked when the plugin is activated. If an error is returned, the plugin will be deactivated.
func (p *Plugin) OnActivate() error {
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

	// TODO: Register tools in the bridge client

	p.registry = flow.NewRegistry()
	p.registry.RegisterTrigger(&trigger.MessagePostedTrigger{})
	p.registry.RegisterAction(action.NewSendMessageAction(p.API, p.botUserID))
	p.registry.RegisterAction(action.NewAIPromptAction(p.API, bc))

	p.flowStore = flow.NewStore(p.API)
	p.triggerService = flow.NewTriggerService(p.flowStore, p.registry)
	p.flowExecutor = flow.NewFlowExecutor(p.registry)

	return nil
}

// OnDeactivate is invoked when the plugin is deactivated.
func (p *Plugin) OnDeactivate() error {
	if p.backgroundJob != nil {
		if err := p.backgroundJob.Close(); err != nil {
			p.API.LogError("Failed to close background job", "err", err)
		}
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
// flows and claims execution using an atomic KV key for HA deduplication.
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

	event := &model.Event{
		Type: "message_posted",
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

	for _, f := range flows {
		channel, appErr := p.API.GetChannel(post.ChannelId)
		if appErr != nil {
			p.API.LogError("Failed to get channel for trigger", "channel_id", post.ChannelId, "err", appErr.Error())
			continue
		}
		user, appErr := p.API.GetUser(post.UserId)
		if appErr != nil {
			p.API.LogError("Failed to get user for trigger", "user_id", post.UserId, "err", appErr.Error())
			continue
		}

		triggerData := model.TriggerData{
			Post:    model.NewSafePost(post),
			Channel: model.NewSafeChannel(channel),
			User:    model.NewSafeUser(user),
		}

		if err := p.flowExecutor.Execute(f, triggerData); err != nil {
			p.API.LogError("Flow execution failed",
				"flow_id", f.ID,
				"flow_name", f.Name,
				"post_id", post.Id,
				"err", err.Error(),
			)
			continue
		}

		p.API.LogInfo("Flow executed successfully",
			"flow_id", f.ID,
			"flow_name", f.Name,
			"post_id", post.Id,
		)
	}
}

// See https://developers.mattermost.com/extend/plugins/server/reference/
