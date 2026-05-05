package trigger

import (
	"fmt"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

// ChannelCreatedTrigger matches when a new public channel is created on the trigger's team.
type ChannelCreatedTrigger struct{}

func (t *ChannelCreatedTrigger) Type() string { return model.TriggerTypeChannelCreated }

func (t *ChannelCreatedTrigger) Matches(trigger *model.Trigger, event *model.Event) bool {
	if trigger.ChannelCreated == nil {
		return false
	}
	if event.Channel == nil {
		return false
	}
	return event.Channel.TeamId == trigger.ChannelCreated.TeamID
}

func (t *ChannelCreatedTrigger) Validate(trigger *model.Trigger, _ *model.Trigger) error {
	if trigger.ChannelCreated == nil {
		return fmt.Errorf("channel_created trigger config is missing")
	}
	if trigger.ChannelCreated.TeamID == "" {
		return fmt.Errorf("channel_created trigger requires team_id")
	}
	return nil
}

func (t *ChannelCreatedTrigger) CandidateFlowIDs(store model.Store, event *model.Event) ([]string, error) {
	if event.Channel == nil {
		return nil, nil
	}
	return store.GetChannelCreatedFlowIDs()
}

func (t *ChannelCreatedTrigger) BuildTriggerData(api model.TriggerAPI, event *model.Event) (model.TriggerData, error) {
	if event.Channel == nil {
		return model.TriggerData{}, fmt.Errorf("channel_created event has no channel")
	}

	td := model.TriggerData{
		Channel: model.NewSafeChannel(event.Channel),
	}

	// Creator lookup is best-effort — a missing user should not block dispatch.
	if event.Channel.CreatorId != "" {
		user, appErr := api.GetUser(event.Channel.CreatorId)
		if appErr != nil {
			api.LogWarn("Failed to get user for channel creation trigger, continuing with partial data",
				"user_id", event.Channel.CreatorId, "err", appErr.Error())
		} else {
			td.User = model.NewSafeUser(user)
		}
	}
	return td, nil
}
