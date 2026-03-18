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
	assert.Equal(t, "user_joined_team", tr.Type())
}

func TestUserJoinedTeamTrigger_Matches_CorrectTeam(t *testing.T) {
	tr := &trigger.UserJoinedTeamTrigger{}
	trig := &model.Trigger{UserJoinedTeam: &model.UserJoinedTeamConfig{TeamID: "team1"}}
	event := &model.Event{
		Type: "user_joined_team",
		Team: &mmmodel.Team{Id: "team1"},
	}

	assert.True(t, tr.Matches(trig, event))
}

func TestUserJoinedTeamTrigger_Matches_WrongTeam(t *testing.T) {
	tr := &trigger.UserJoinedTeamTrigger{}
	trig := &model.Trigger{UserJoinedTeam: &model.UserJoinedTeamConfig{TeamID: "team1"}}
	event := &model.Event{
		Type: "user_joined_team",
		Team: &mmmodel.Team{Id: "team2"},
	}

	assert.False(t, tr.Matches(trig, event))
}

func TestUserJoinedTeamTrigger_Matches_NilTeam(t *testing.T) {
	tr := &trigger.UserJoinedTeamTrigger{}
	trig := &model.Trigger{UserJoinedTeam: &model.UserJoinedTeamConfig{TeamID: "team1"}}
	event := &model.Event{
		Type: "user_joined_team",
		Team: nil,
	}

	assert.False(t, tr.Matches(trig, event))
}

func TestUserJoinedTeamTrigger_Matches_NilConfig(t *testing.T) {
	tr := &trigger.UserJoinedTeamTrigger{}
	trig := &model.Trigger{}
	event := &model.Event{
		Type: "user_joined_team",
		Team: &mmmodel.Team{Id: "team1"},
	}

	assert.False(t, tr.Matches(trig, event))
}
