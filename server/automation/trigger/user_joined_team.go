package trigger

import (
	"fmt"

	mmmodel "github.com/mattermost/mattermost/server/public/model"

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

func (t *UserJoinedTeamTrigger) Validate(trigger *model.Trigger, _ *model.Trigger) error {
	if trigger.UserJoinedTeam == nil {
		return fmt.Errorf("user_joined_team trigger config is missing")
	}
	if trigger.UserJoinedTeam.TeamID == "" {
		return fmt.Errorf("user_joined_team trigger requires team_id")
	}
	if ut := trigger.UserJoinedTeam.UserType; ut != "" && ut != "user" && ut != "guest" {
		return fmt.Errorf("user_joined_team trigger user_type must be \"user\", \"guest\", or empty (both)")
	}
	return nil
}

func (t *UserJoinedTeamTrigger) CandidateAutomationIDs(store model.Store, event *model.Event) ([]string, error) {
	if event.Team == nil || event.Team.Id == "" {
		return nil, nil
	}
	return store.GetAutomationIDsForUserJoinedTeam(event.Team.Id)
}

func (t *UserJoinedTeamTrigger) CallerCanTrigger(api model.HookCallerAPI, trigger *model.Trigger, userID string) bool {
	if trigger.UserJoinedTeam == nil {
		return false
	}
	cfg := trigger.UserJoinedTeam
	if !isTeamMember(api, cfg.TeamID, userID) {
		return false
	}
	return userMatchesUserType(api, userID, cfg.UserType)
}

func (t *UserJoinedTeamTrigger) BuildTriggerData(api model.TriggerAPI, event *model.Event) (model.TriggerData, error) {
	if event.Team == nil || event.Team.Id == "" {
		return model.TriggerData{}, fmt.Errorf("user_joined_team event has no team")
	}
	if event.User == nil {
		return model.TriggerData{}, fmt.Errorf("user_joined_team event has no user")
	}

	// Team lookup is best-effort. On failure we still preserve the team ID
	// from the event so templates referencing .Trigger.Team.Id keep working.
	team, appErr := api.GetTeam(event.Team.Id)
	if appErr != nil {
		api.LogWarn("Failed to get team for team join trigger, continuing with partial data",
			"team_id", event.Team.Id, "err", appErr.Error())
		team = &mmmodel.Team{Id: event.Team.Id}
	}
	safeTeam := model.NewSafeTeam(team)

	// Default channel lookup is best-effort — templates referencing it will
	// render empty when this fails, which surfaces the problem at execution.
	defaultChannel, appErr := api.GetChannelByName(event.Team.Id, mmmodel.DefaultChannelName, false)
	if appErr != nil {
		api.LogWarn("Failed to get default channel for team join trigger",
			"team_id", event.Team.Id, "err", appErr.Error())
	} else {
		safeTeam.DefaultChannelId = defaultChannel.Id
	}

	return model.TriggerData{
		User: model.NewSafeUser(event.User),
		Team: safeTeam,
	}, nil
}
