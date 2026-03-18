package trigger

import (
	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

// UserJoinedTeamTrigger matches when a user joins any team.
type UserJoinedTeamTrigger struct{}

func (t *UserJoinedTeamTrigger) Type() string { return "user_joined_team" }

func (t *UserJoinedTeamTrigger) Matches(trigger *model.Trigger, event *model.Event) bool {
	if trigger.UserJoinedTeam == nil {
		return false
	}
	return event.Team != nil
}
