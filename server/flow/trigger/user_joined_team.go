package trigger

import (
	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

// UserJoinedTeamTrigger matches when a user joins a team.
type UserJoinedTeamTrigger struct{}

func (t *UserJoinedTeamTrigger) Type() string { return model.TriggerTypeUserJoinedTeam }

func (t *UserJoinedTeamTrigger) Matches(trigger *model.Trigger, event *model.Event) bool {
	if trigger.UserJoinedTeam == nil {
		return false
	}
	if event.Team == nil {
		return false
	}
	if trigger.UserJoinedTeam.TeamID != event.Team.Id {
		return false
	}
	if ut := trigger.UserJoinedTeam.UserType; ut != "" {
		if event.User == nil {
			return false
		}
		if ut == "guest" && !event.User.IsGuest() {
			return false
		}
		if ut == "user" && event.User.IsGuest() {
			return false
		}
	}
	return true
}
