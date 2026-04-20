package trigger_test

import (
	"testing"

	mmmodel "github.com/mattermost/mattermost/server/public/model"
	"github.com/stretchr/testify/assert"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/flow/trigger"
	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

func TestUserJoinedTeamTrigger_Type(t *testing.T) {
	tr := &trigger.UserJoinedTeamTrigger{}
	assert.Equal(t, model.TriggerTypeUserJoinedTeam, tr.Type())
}

func TestUserJoinedTeamTrigger_Matches_CorrectTeam(t *testing.T) {
	tr := &trigger.UserJoinedTeamTrigger{}
	trig := &model.Trigger{UserJoinedTeam: &model.UserJoinedTeamConfig{TeamID: "team1"}}
	event := &model.Event{
		Type: model.TriggerTypeUserJoinedTeam,
		Team: &mmmodel.Team{Id: "team1"},
	}

	assert.True(t, tr.Matches(trig, event))
}

func TestUserJoinedTeamTrigger_Matches_WrongTeam(t *testing.T) {
	tr := &trigger.UserJoinedTeamTrigger{}
	trig := &model.Trigger{UserJoinedTeam: &model.UserJoinedTeamConfig{TeamID: "team1"}}
	event := &model.Event{
		Type: model.TriggerTypeUserJoinedTeam,
		Team: &mmmodel.Team{Id: "team2"},
	}

	assert.False(t, tr.Matches(trig, event))
}

func TestUserJoinedTeamTrigger_Matches_NilTeam(t *testing.T) {
	tr := &trigger.UserJoinedTeamTrigger{}
	trig := &model.Trigger{UserJoinedTeam: &model.UserJoinedTeamConfig{TeamID: "team1"}}
	event := &model.Event{
		Type: model.TriggerTypeUserJoinedTeam,
		Team: nil,
	}

	assert.False(t, tr.Matches(trig, event))
}

func TestUserJoinedTeamTrigger_Matches_NilConfig(t *testing.T) {
	tr := &trigger.UserJoinedTeamTrigger{}
	trig := &model.Trigger{}
	event := &model.Event{
		Type: model.TriggerTypeUserJoinedTeam,
		Team: &mmmodel.Team{Id: "team1"},
	}

	assert.False(t, tr.Matches(trig, event))
}

func TestUserJoinedTeamTrigger_Matches_UserTypeFilter(t *testing.T) {
	tr := &trigger.UserJoinedTeamTrigger{}

	regularUser := &mmmodel.User{Roles: "system_user"}
	guestUser := &mmmodel.User{Roles: "system_guest"}

	t.Run("user_type=user matches regular user", func(t *testing.T) {
		trig := &model.Trigger{UserJoinedTeam: &model.UserJoinedTeamConfig{TeamID: "team1", UserType: "user"}}
		event := &model.Event{Type: model.TriggerTypeUserJoinedTeam, Team: &mmmodel.Team{Id: "team1"}, User: regularUser}
		assert.True(t, tr.Matches(trig, event))
	})

	t.Run("user_type=user does not match guest", func(t *testing.T) {
		trig := &model.Trigger{UserJoinedTeam: &model.UserJoinedTeamConfig{TeamID: "team1", UserType: "user"}}
		event := &model.Event{Type: model.TriggerTypeUserJoinedTeam, Team: &mmmodel.Team{Id: "team1"}, User: guestUser}
		assert.False(t, tr.Matches(trig, event))
	})

	t.Run("user_type=guest matches guest", func(t *testing.T) {
		trig := &model.Trigger{UserJoinedTeam: &model.UserJoinedTeamConfig{TeamID: "team1", UserType: "guest"}}
		event := &model.Event{Type: model.TriggerTypeUserJoinedTeam, Team: &mmmodel.Team{Id: "team1"}, User: guestUser}
		assert.True(t, tr.Matches(trig, event))
	})

	t.Run("user_type=guest does not match regular user", func(t *testing.T) {
		trig := &model.Trigger{UserJoinedTeam: &model.UserJoinedTeamConfig{TeamID: "team1", UserType: "guest"}}
		event := &model.Event{Type: model.TriggerTypeUserJoinedTeam, Team: &mmmodel.Team{Id: "team1"}, User: regularUser}
		assert.False(t, tr.Matches(trig, event))
	})

	t.Run("user_type=empty matches regular user", func(t *testing.T) {
		trig := &model.Trigger{UserJoinedTeam: &model.UserJoinedTeamConfig{TeamID: "team1", UserType: ""}}
		event := &model.Event{Type: model.TriggerTypeUserJoinedTeam, Team: &mmmodel.Team{Id: "team1"}, User: regularUser}
		assert.True(t, tr.Matches(trig, event))
	})

	t.Run("user_type=empty matches guest", func(t *testing.T) {
		trig := &model.Trigger{UserJoinedTeam: &model.UserJoinedTeamConfig{TeamID: "team1", UserType: ""}}
		event := &model.Event{Type: model.TriggerTypeUserJoinedTeam, Team: &mmmodel.Team{Id: "team1"}, User: guestUser}
		assert.True(t, tr.Matches(trig, event))
	})
}
