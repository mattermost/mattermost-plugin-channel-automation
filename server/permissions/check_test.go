package permissions

import (
	"errors"
	"net/http"
	"testing"

	mmmodel "github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin/plugintest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

func TestCheckFlowPermissions_SystemAdminBypass(t *testing.T) {
	api := &plugintest.API{}
	api.On("HasPermissionTo", "admin1", mmmodel.PermissionManageSystem).Return(true)

	f := &model.Flow{
		Trigger: model.Trigger{ChannelCreated: &model.ChannelCreatedConfig{TeamID: "team1"}},
	}

	err := CheckFlowPermissions(api, "admin1", f)
	require.NoError(t, err)
}

func TestCheckFlowPermissions_ChannelCreated_TeamAdminAllowed(t *testing.T) {
	api := &plugintest.API{}
	api.On("HasPermissionTo", "user1", mmmodel.PermissionManageSystem).Return(false)
	api.On("GetTeam", "team1").Return(&mmmodel.Team{Id: "team1"}, nil)
	api.On("HasPermissionToTeam", "user1", "team1", mmmodel.PermissionManageTeam).Return(true)

	f := &model.Flow{
		Trigger: model.Trigger{ChannelCreated: &model.ChannelCreatedConfig{TeamID: "team1"}},
		Actions: []model.Action{
			{ID: "a1", SendMessage: &model.SendMessageActionConfig{ChannelID: "{{.Trigger.Channel.Id}}", Body: "hi"}},
		},
	}

	err := CheckFlowPermissions(api, "user1", f)
	require.NoError(t, err)
}

func TestCheckFlowPermissions_ChannelCreated_NonTeamAdminDenied(t *testing.T) {
	api := &plugintest.API{}
	api.On("HasPermissionTo", "user1", mmmodel.PermissionManageSystem).Return(false)
	api.On("GetTeam", "team1").Return(&mmmodel.Team{Id: "team1"}, nil)
	api.On("HasPermissionToTeam", "user1", "team1", mmmodel.PermissionManageTeam).Return(false)

	f := &model.Flow{
		Trigger: model.Trigger{ChannelCreated: &model.ChannelCreatedConfig{TeamID: "team1"}},
	}

	err := CheckFlowPermissions(api, "user1", f)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "team admin")
}

func TestCheckFlowPermissions_ChannelCreated_LiteralChannelSameTeam(t *testing.T) {
	api := &plugintest.API{}
	api.On("HasPermissionTo", "user1", mmmodel.PermissionManageSystem).Return(false)
	api.On("GetTeam", "team1").Return(&mmmodel.Team{Id: "team1"}, nil)
	api.On("HasPermissionToTeam", "user1", "team1", mmmodel.PermissionManageTeam).Return(true)
	api.On("GetChannel", "ch1").Return(&mmmodel.Channel{Id: "ch1", TeamId: "team1"}, nil)

	f := &model.Flow{
		Trigger: model.Trigger{ChannelCreated: &model.ChannelCreatedConfig{TeamID: "team1"}},
		Actions: []model.Action{
			{ID: "a1", SendMessage: &model.SendMessageActionConfig{ChannelID: "ch1", Body: "hi"}},
		},
	}

	err := CheckFlowPermissions(api, "user1", f)
	require.NoError(t, err)
}

func TestCheckFlowPermissions_ChannelCreated_LiteralChannelWrongTeam(t *testing.T) {
	api := &plugintest.API{}
	api.On("HasPermissionTo", "user1", mmmodel.PermissionManageSystem).Return(false)
	api.On("GetTeam", "team1").Return(&mmmodel.Team{Id: "team1"}, nil)
	api.On("HasPermissionToTeam", "user1", "team1", mmmodel.PermissionManageTeam).Return(true)
	api.On("GetChannel", "ch-other").Return(&mmmodel.Channel{Id: "ch-other", TeamId: "team2"}, nil)

	f := &model.Flow{
		Trigger: model.Trigger{ChannelCreated: &model.ChannelCreatedConfig{TeamID: "team1"}},
		Actions: []model.Action{
			{ID: "a1", SendMessage: &model.SendMessageActionConfig{ChannelID: "ch-other", Body: "hi"}},
		},
	}

	err := CheckFlowPermissions(api, "user1", f)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not belong to the team")
}

func TestCheckFlowPermissions_ChannelCreated_GetTeamServerError(t *testing.T) {
	api := &plugintest.API{}
	api.On("HasPermissionTo", "user1", mmmodel.PermissionManageSystem).Return(false)
	api.On("GetTeam", "team1").Return(nil, &mmmodel.AppError{
		Message:    "database connection lost",
		StatusCode: http.StatusInternalServerError,
	})

	f := &model.Flow{
		Trigger: model.Trigger{ChannelCreated: &model.ChannelCreatedConfig{TeamID: "team1"}},
	}

	err := CheckFlowPermissions(api, "user1", f)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to verify team")

	var appErr *mmmodel.AppError
	assert.True(t, errors.As(err, &appErr), "error should wrap AppError for 5xx classification")
}

func TestCheckFlowPermissions_ChannelCreated_GetTeamNotFound(t *testing.T) {
	api := &plugintest.API{}
	api.On("HasPermissionTo", "user1", mmmodel.PermissionManageSystem).Return(false)
	api.On("GetTeam", "bad-team").Return(nil, &mmmodel.AppError{
		Message:    "team not found",
		StatusCode: http.StatusNotFound,
	})

	f := &model.Flow{
		Trigger: model.Trigger{ChannelCreated: &model.ChannelCreatedConfig{TeamID: "bad-team"}},
	}

	err := CheckFlowPermissions(api, "user1", f)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found or not accessible")
}

func TestCheckFlowPermissions_ChannelCreated_GetChannelServerError(t *testing.T) {
	api := &plugintest.API{}
	api.On("HasPermissionTo", "user1", mmmodel.PermissionManageSystem).Return(false)
	api.On("GetTeam", "team1").Return(&mmmodel.Team{Id: "team1"}, nil)
	api.On("HasPermissionToTeam", "user1", "team1", mmmodel.PermissionManageTeam).Return(true)
	api.On("GetChannel", "ch1").Return(nil, &mmmodel.AppError{
		Message:    "database error",
		StatusCode: http.StatusInternalServerError,
	})

	f := &model.Flow{
		Trigger: model.Trigger{ChannelCreated: &model.ChannelCreatedConfig{TeamID: "team1"}},
		Actions: []model.Action{
			{ID: "a1", SendMessage: &model.SendMessageActionConfig{ChannelID: "ch1", Body: "hi"}},
		},
	}

	err := CheckFlowPermissions(api, "user1", f)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to verify channel team membership")

	var appErr *mmmodel.AppError
	assert.True(t, errors.As(err, &appErr), "error should wrap AppError for 5xx classification")
}

func TestCheckFlowPermissions_ChannelCreated_GetChannelNotFound(t *testing.T) {
	api := &plugintest.API{}
	api.On("HasPermissionTo", "user1", mmmodel.PermissionManageSystem).Return(false)
	api.On("GetTeam", "team1").Return(&mmmodel.Team{Id: "team1"}, nil)
	api.On("HasPermissionToTeam", "user1", "team1", mmmodel.PermissionManageTeam).Return(true)
	api.On("GetChannel", "ch-gone").Return(nil, &mmmodel.AppError{
		Message:    "channel not found",
		StatusCode: http.StatusNotFound,
	})

	f := &model.Flow{
		Trigger: model.Trigger{ChannelCreated: &model.ChannelCreatedConfig{TeamID: "team1"}},
		Actions: []model.Action{
			{ID: "a1", SendMessage: &model.SendMessageActionConfig{ChannelID: "ch-gone", Body: "hi"}},
		},
	}

	err := CheckFlowPermissions(api, "user1", f)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found or not accessible")
}

func TestCheckFlowPermissions_NonChannelCreated_ChannelAdminRequired(t *testing.T) {
	api := &plugintest.API{}
	api.On("HasPermissionTo", "user1", mmmodel.PermissionManageSystem).Return(false)
	api.On("GetChannelMember", "ch1", "user1").Return(
		&mmmodel.ChannelMember{SchemeAdmin: true}, nil,
	)

	f := &model.Flow{
		Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
		Actions: []model.Action{
			{ID: "a1", SendMessage: &model.SendMessageActionConfig{ChannelID: "ch1", Body: "hi"}},
		},
	}

	err := CheckFlowPermissions(api, "user1", f)
	require.NoError(t, err)
}

func TestValidateTeamBotConfig_NoConfig(t *testing.T) {
	api := &plugintest.API{}

	f := &model.Flow{
		Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
	}

	err := ValidateTeamBotConfig(api, f)
	require.NoError(t, err)
}

func TestValidateTeamBotConfig_TeamBotActionWithoutConfig(t *testing.T) {
	api := &plugintest.API{}

	f := &model.Flow{
		Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
		Actions: []model.Action{
			{ID: "a1", AIPrompt: &model.AIPromptActionConfig{
				Prompt:        "test",
				ProviderType:  "agent",
				ProviderID:    "bot1",
				ExecutionMode: "team_bot",
			}},
		},
	}

	err := ValidateTeamBotConfig(api, f)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no team_bot_config")
}

func TestValidateTeamBotConfig_InvalidExecutionMode(t *testing.T) {
	api := &plugintest.API{}

	f := &model.Flow{
		Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
		Actions: []model.Action{
			{ID: "a1", AIPrompt: &model.AIPromptActionConfig{
				Prompt:        "test",
				ProviderType:  "agent",
				ProviderID:    "bot1",
				ExecutionMode: "invalid",
			}},
		},
	}

	err := ValidateTeamBotConfig(api, f)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid execution_mode")
}

func TestValidateTeamBotConfig_MissingTeamID(t *testing.T) {
	api := &plugintest.API{}

	f := &model.Flow{
		Trigger:       model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
		TeamBotConfig: &model.TeamBotConfig{},
	}

	err := ValidateTeamBotConfig(api, f)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "team_id is required")
}

func TestValidateTeamBotConfig_ChannelIDsNotValidated(t *testing.T) {
	api := &plugintest.API{}
	api.On("GetTeam", "team1").Return(&mmmodel.Team{Id: "team1"}, nil)

	f := &model.Flow{
		Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
		TeamBotConfig: &model.TeamBotConfig{
			TeamID:     "team1",
			ChannelIDs: []string{"ch1", "ch-doesnt-exist", "ch-private"},
		},
		Actions: []model.Action{
			{ID: "a1", AIPrompt: &model.AIPromptActionConfig{
				Prompt:        "test",
				ProviderType:  "agent",
				ProviderID:    "bot1",
				ExecutionMode: "team_bot",
			}},
		},
	}

	err := ValidateTeamBotConfig(api, f)
	require.NoError(t, err, "channel IDs are not validated at creation time; enforcement happens at runtime via hooks")
}

func TestValidateTeamBotConfig_CreatorModeAllowed(t *testing.T) {
	api := &plugintest.API{}
	api.On("GetTeam", "team1").Return(&mmmodel.Team{Id: "team1"}, nil)

	f := &model.Flow{
		Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
		TeamBotConfig: &model.TeamBotConfig{
			TeamID: "team1",
		},
		Actions: []model.Action{
			{ID: "a1", AIPrompt: &model.AIPromptActionConfig{
				Prompt:        "test",
				ProviderType:  "agent",
				ProviderID:    "bot1",
				ExecutionMode: "creator",
			}},
		},
	}

	err := ValidateTeamBotConfig(api, f)
	require.NoError(t, err)
}
