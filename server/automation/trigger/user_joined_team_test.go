package trigger_test

import (
	"net/http"
	"testing"

	mmmodel "github.com/mattermost/mattermost/server/public/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/automation/trigger"
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

func TestUserJoinedTeamTrigger_Validate(t *testing.T) {
	tr := &trigger.UserJoinedTeamTrigger{}

	t.Run("valid", func(t *testing.T) {
		require.NoError(t, tr.Validate(&model.Trigger{UserJoinedTeam: &model.UserJoinedTeamConfig{TeamID: "team1"}}, nil))
	})

	for _, ut := range []string{"user", "guest", ""} {
		t.Run("valid user_type "+ut, func(t *testing.T) {
			require.NoError(t, tr.Validate(&model.Trigger{UserJoinedTeam: &model.UserJoinedTeamConfig{TeamID: "team1", UserType: ut}}, nil))
		})
	}

	t.Run("invalid user_type", func(t *testing.T) {
		err := tr.Validate(&model.Trigger{UserJoinedTeam: &model.UserJoinedTeamConfig{TeamID: "team1", UserType: "admin"}}, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "user_type")
	})

	t.Run("missing team_id", func(t *testing.T) {
		err := tr.Validate(&model.Trigger{UserJoinedTeam: &model.UserJoinedTeamConfig{}}, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "team_id")
	})

	t.Run("missing config", func(t *testing.T) {
		require.Error(t, tr.Validate(&model.Trigger{}, nil))
	})
}

func TestUserJoinedTeamTrigger_BuildTriggerData(t *testing.T) {
	tr := &trigger.UserJoinedTeamTrigger{}

	t.Run("success with team and default channel", func(t *testing.T) {
		api := newFakeTriggerAPI()
		api.teams["team1"] = &mmmodel.Team{Id: "team1", Name: "eng", DisplayName: "Engineering"}
		api.channelByName["team1/"+mmmodel.DefaultChannelName] = &mmmodel.Channel{Id: "town-square-id", Name: mmmodel.DefaultChannelName}

		event := &model.Event{
			Type: model.TriggerTypeUserJoinedTeam,
			Team: &mmmodel.Team{Id: "team1"},
			User: &mmmodel.User{Id: "user1", Username: "alice"},
		}

		td, err := tr.BuildTriggerData(api, event)
		require.NoError(t, err)
		require.NotNil(t, td.Team)
		require.NotNil(t, td.User)
		assert.Equal(t, "eng", td.Team.Name)
		assert.Equal(t, "town-square-id", td.Team.DefaultChannelId)
		assert.Equal(t, "alice", td.User.Username)
	})

	t.Run("team fetch failure preserves Id from event", func(t *testing.T) {
		api := newFakeTriggerAPI()
		api.teamErrors["team1"] = mmmodel.NewAppError("fake", "boom", nil, "boom", http.StatusInternalServerError)
		api.channelByName["team1/"+mmmodel.DefaultChannelName] = &mmmodel.Channel{Id: "town-square-id", Name: mmmodel.DefaultChannelName}

		event := &model.Event{
			Type: model.TriggerTypeUserJoinedTeam,
			Team: &mmmodel.Team{Id: "team1"},
			User: &mmmodel.User{Id: "user1", Username: "alice"},
		}

		td, err := tr.BuildTriggerData(api, event)
		require.NoError(t, err)
		require.NotNil(t, td.Team)
		assert.Equal(t, "team1", td.Team.Id, "Id must propagate from event when GetTeam fails")
		assert.Empty(t, td.Team.Name)
		assert.Empty(t, td.Team.DisplayName)
		assert.Equal(t, "town-square-id", td.Team.DefaultChannelId)
		assert.NotEmpty(t, api.warnCalls)
	})

	t.Run("default channel failure leaves DefaultChannelId empty", func(t *testing.T) {
		api := newFakeTriggerAPI()
		api.teams["team1"] = &mmmodel.Team{Id: "team1", Name: "eng"}

		event := &model.Event{
			Type: model.TriggerTypeUserJoinedTeam,
			Team: &mmmodel.Team{Id: "team1"},
			User: &mmmodel.User{Id: "user1"},
		}

		td, err := tr.BuildTriggerData(api, event)
		require.NoError(t, err)
		require.NotNil(t, td.Team)
		assert.Empty(t, td.Team.DefaultChannelId)
		assert.NotEmpty(t, api.warnCalls)
	})

	t.Run("nil team errors", func(t *testing.T) {
		_, err := tr.BuildTriggerData(newFakeTriggerAPI(), &model.Event{Type: model.TriggerTypeUserJoinedTeam, User: &mmmodel.User{Id: "u"}})
		require.Error(t, err)
	})

	t.Run("nil user errors", func(t *testing.T) {
		_, err := tr.BuildTriggerData(newFakeTriggerAPI(), &model.Event{Type: model.TriggerTypeUserJoinedTeam, Team: &mmmodel.Team{Id: "t"}})
		require.Error(t, err)
	})
}

func TestUserJoinedTeamTrigger_CandidateAutomationIDs(t *testing.T) {
	tr := &trigger.UserJoinedTeamTrigger{}

	t.Run("nil team returns empty", func(t *testing.T) {
		ids, err := tr.CandidateAutomationIDs(&stubStore{}, &model.Event{Type: model.TriggerTypeUserJoinedTeam})
		require.NoError(t, err)
		assert.Nil(t, ids)
	})

	t.Run("delegates to store.GetAutomationIDsForUserJoinedTeam", func(t *testing.T) {
		st := &stubStore{flowIDsByTeam: map[string][]string{"team1": {"f1"}}}
		event := &model.Event{Type: model.TriggerTypeUserJoinedTeam, Team: &mmmodel.Team{Id: "team1"}}
		ids, err := tr.CandidateAutomationIDs(st, event)
		require.NoError(t, err)
		assert.Equal(t, []string{"f1"}, ids)
	})
}
