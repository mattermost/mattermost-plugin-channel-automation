package trigger

import (
	"fmt"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

// MessagePostedTrigger matches when a message is posted in the configured channel.
type MessagePostedTrigger struct{}

func (t *MessagePostedTrigger) Type() string { return model.TriggerTypeMessagePosted }

func (t *MessagePostedTrigger) Matches(trigger *model.Trigger, event *model.Event) bool {
	if event.Post == nil {
		return false
	}
	if trigger.MessagePosted == nil {
		return false
	}
	if trigger.MessagePosted.ChannelID != event.Post.ChannelId {
		return false
	}
	if event.Post.RootId != "" && !trigger.MessagePosted.IncludeThreadReplies {
		return false
	}
	return true
}

func (t *MessagePostedTrigger) Validate(trigger *model.Trigger, _ *model.Trigger) error {
	if trigger.MessagePosted == nil {
		return fmt.Errorf("message_posted trigger config is missing")
	}
	if trigger.MessagePosted.ChannelID == "" {
		return fmt.Errorf("message_posted trigger requires channel_id")
	}
	return nil
}

func (t *MessagePostedTrigger) CandidateFlowIDs(store model.Store, event *model.Event) ([]string, error) {
	if event.Post == nil {
		return nil, nil
	}
	return store.GetFlowIDsForChannel(event.Post.ChannelId)
}

func (t *MessagePostedTrigger) BuildTriggerData(api model.TriggerAPI, event *model.Event) (model.TriggerData, error) {
	if event.Post == nil {
		return model.TriggerData{}, fmt.Errorf("message_posted event has no post")
	}

	channel, appErr := api.GetChannel(event.Post.ChannelId)
	if appErr != nil {
		return model.TriggerData{}, fmt.Errorf("get channel %s: %w", event.Post.ChannelId, appErr)
	}
	user, appErr := api.GetUser(event.Post.UserId)
	if appErr != nil {
		return model.TriggerData{}, fmt.Errorf("get user %s: %w", event.Post.UserId, appErr)
	}

	safeUser := model.NewSafeUser(user)
	data := model.TriggerData{
		Post:    model.NewSafePost(event.Post, safeUser),
		Channel: model.NewSafeChannel(channel),
		User:    safeUser,
	}

	// This branch is only reachable when MessagePostedConfig
	// .IncludeThreadReplies is on (otherwise Matches drops reply events
	// before BuildTriggerData runs).
	if event.Post.RootId != "" {
		data.Thread = fetchThread(api, event.Post.RootId)
	}

	return data, nil
}

// fetchThread loads the full thread rooted at rootID and returns it as
// a SafeThread with each author's user pre-resolved. Returns nil on any
// fetch failure (logging a warning) so the caller can attach nil and
// continue — losing thread context is preferable to dropping the event.
func fetchThread(api model.TriggerAPI, rootID string) *model.SafeThread {
	postList, appErr := api.GetPostThread(rootID)
	if appErr != nil {
		api.LogWarn("message_posted trigger: failed to fetch thread for root post, continuing without thread context",
			"root_id", rootID,
			"error", appErr.Error(),
		)
		return nil
	}
	userFor := func(userID string) *model.SafeUser {
		u, userErr := api.GetUser(userID)
		if userErr != nil || u == nil {
			errMsg := "<nil>"
			if userErr != nil {
				errMsg = userErr.Error()
			}
			api.LogWarn("message_posted trigger: failed to resolve thread author user, falling back to user-id-only display",
				"user_id", userID,
				"error", errMsg,
			)
			return nil
		}
		return model.NewSafeUser(u)
	}
	return model.NewSafeThread(postList, rootID, userFor)
}
