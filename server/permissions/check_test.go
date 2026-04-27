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

func TestCheckGuardrailChannelPermissions_SystemAdminPassesPerChannelCheck(t *testing.T) {
	// Sysadmins are not short-circuited; they are expected to satisfy
	// PermissionReadChannel on every guardrail channel via the normal check.
	api := &plugintest.API{}
	api.On("GetChannel", "ch1").Return(&mmmodel.Channel{Id: "ch1"}, nil)
	api.On("HasPermissionToChannel", "admin1", "ch1", mmmodel.PermissionReadChannel).Return(true)

	f := &model.Flow{
		Actions: []model.Action{
			{ID: "a1", AIPrompt: &model.AIPromptActionConfig{
				Prompt: "p", ProviderType: "agent", ProviderID: "bot",
				Guardrails: &model.Guardrails{Channels: []model.GuardrailChannel{{ChannelID: "ch1"}}},
			}},
		},
	}
	require.NoError(t, CheckGuardrailChannelPermissions(api, "admin1", f))
	api.AssertExpectations(t)
}

func TestCheckGuardrailChannelPermissions_NoAIPromptOrGuardrails(t *testing.T) {
	api := &plugintest.API{}

	f := &model.Flow{
		Actions: []model.Action{
			{ID: "a1", SendMessage: &model.SendMessageActionConfig{ChannelID: "ch1", Body: "hi"}},
			{ID: "a2", AIPrompt: &model.AIPromptActionConfig{Prompt: "p", ProviderType: "agent", ProviderID: "bot"}},
		},
	}
	require.NoError(t, CheckGuardrailChannelPermissions(api, "user1", f))
	api.AssertExpectations(t)
}

func TestCheckGuardrailChannelPermissions_AllAccessible(t *testing.T) {
	api := &plugintest.API{}
	api.On("GetChannel", "ch1").Return(&mmmodel.Channel{Id: "ch1"}, nil)
	api.On("GetChannel", "ch2").Return(&mmmodel.Channel{Id: "ch2"}, nil)
	api.On("HasPermissionToChannel", "user1", "ch1", mmmodel.PermissionReadChannel).Return(true)
	api.On("HasPermissionToChannel", "user1", "ch2", mmmodel.PermissionReadChannel).Return(true)

	f := &model.Flow{
		Actions: []model.Action{
			{ID: "a1", AIPrompt: &model.AIPromptActionConfig{
				Prompt: "p", ProviderType: "agent", ProviderID: "bot",
				Guardrails: &model.Guardrails{Channels: []model.GuardrailChannel{
					{ChannelID: "ch1"}, {ChannelID: "ch2"},
				}},
			}},
		},
	}
	require.NoError(t, CheckGuardrailChannelPermissions(api, "user1", f))
	api.AssertExpectations(t)
}

func TestCheckGuardrailChannelPermissions_MissingReadPermissionDenied(t *testing.T) {
	api := &plugintest.API{}
	api.On("GetChannel", "ch-secret").Return(&mmmodel.Channel{Id: "ch-secret"}, nil)
	api.On("HasPermissionToChannel", "user1", "ch-secret", mmmodel.PermissionReadChannel).Return(false)

	f := &model.Flow{
		Actions: []model.Action{
			{ID: "a1", AIPrompt: &model.AIPromptActionConfig{
				Prompt: "p", ProviderType: "agent", ProviderID: "bot",
				Guardrails: &model.Guardrails{Channels: []model.GuardrailChannel{{ChannelID: "ch-secret"}}},
			}},
		},
	}
	err := CheckGuardrailChannelPermissions(api, "user1", f)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "do not have permission to read")
	api.AssertExpectations(t)
}

func TestCheckGuardrailChannelPermissions_GetChannelServerError(t *testing.T) {
	api := &plugintest.API{}
	api.On("GetChannel", "ch1").Return(nil, &mmmodel.AppError{
		Message:    "database error",
		StatusCode: http.StatusInternalServerError,
	})

	f := &model.Flow{
		Actions: []model.Action{
			{ID: "a1", AIPrompt: &model.AIPromptActionConfig{
				Prompt: "p", ProviderType: "agent", ProviderID: "bot",
				Guardrails: &model.Guardrails{Channels: []model.GuardrailChannel{{ChannelID: "ch1"}}},
			}},
		},
	}
	err := CheckGuardrailChannelPermissions(api, "user1", f)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to verify guardrail channel")

	var appErr *mmmodel.AppError
	assert.True(t, errors.As(err, &appErr), "error should wrap AppError for 5xx classification")
	api.AssertExpectations(t)
}

func TestCheckGuardrailChannelPermissions_GetChannelNotFoundDenied(t *testing.T) {
	api := &plugintest.API{}
	api.On("GetChannel", "ch-gone").Return(nil, &mmmodel.AppError{
		Message:    "channel not found",
		StatusCode: http.StatusNotFound,
	})

	f := &model.Flow{
		Actions: []model.Action{
			{ID: "a1", AIPrompt: &model.AIPromptActionConfig{
				Prompt: "p", ProviderType: "agent", ProviderID: "bot",
				Guardrails: &model.Guardrails{Channels: []model.GuardrailChannel{{ChannelID: "ch-gone"}}},
			}},
		},
	}
	err := CheckGuardrailChannelPermissions(api, "user1", f)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "do not have permission to read")
	api.AssertExpectations(t)
}

func TestCheckGuardrailChannelPermissions_DuplicateChannelsDeduped(t *testing.T) {
	api := &plugintest.API{}
	api.On("GetChannel", "ch1").Return(&mmmodel.Channel{Id: "ch1"}, nil).Once()
	api.On("HasPermissionToChannel", "user1", "ch1", mmmodel.PermissionReadChannel).Return(true).Once()

	f := &model.Flow{
		Actions: []model.Action{
			{ID: "a1", AIPrompt: &model.AIPromptActionConfig{
				Prompt: "p", ProviderType: "agent", ProviderID: "bot",
				Guardrails: &model.Guardrails{Channels: []model.GuardrailChannel{{ChannelID: "ch1"}}},
			}},
			{ID: "a2", AIPrompt: &model.AIPromptActionConfig{
				Prompt: "p2", ProviderType: "agent", ProviderID: "bot",
				Guardrails: &model.Guardrails{Channels: []model.GuardrailChannel{{ChannelID: "ch1"}}},
			}},
		},
	}
	require.NoError(t, CheckGuardrailChannelPermissions(api, "user1", f))
	api.AssertExpectations(t)
}

func TestCheckFlowPermissions_UserJoinedTeam_TeamAdminAllowed(t *testing.T) {
	api := &plugintest.API{}
	api.On("HasPermissionTo", "user1", mmmodel.PermissionManageSystem).Return(false)
	api.On("GetTeam", "team1").Return(&mmmodel.Team{Id: "team1"}, nil)
	api.On("HasPermissionToTeam", "user1", "team1", mmmodel.PermissionManageTeam).Return(true)

	f := &model.Flow{
		Trigger: model.Trigger{UserJoinedTeam: &model.UserJoinedTeamConfig{TeamID: "team1"}},
		Actions: []model.Action{
			{ID: "a1", SendMessage: &model.SendMessageActionConfig{ChannelID: "{{.Trigger.Team.DefaultChannelId}}", Body: "welcome"}},
		},
	}

	err := CheckFlowPermissions(api, "user1", f)
	require.NoError(t, err)
}

func TestCheckFlowPermissions_UserJoinedTeam_NonTeamAdminDenied(t *testing.T) {
	api := &plugintest.API{}
	api.On("HasPermissionTo", "user1", mmmodel.PermissionManageSystem).Return(false)
	api.On("GetTeam", "team1").Return(&mmmodel.Team{Id: "team1"}, nil)
	api.On("HasPermissionToTeam", "user1", "team1", mmmodel.PermissionManageTeam).Return(false)

	f := &model.Flow{
		Trigger: model.Trigger{UserJoinedTeam: &model.UserJoinedTeamConfig{TeamID: "team1"}},
	}

	err := CheckFlowPermissions(api, "user1", f)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "team admin")
}

func TestCheckFlowPermissions_UserJoinedTeam_GetTeamServerError(t *testing.T) {
	api := &plugintest.API{}
	api.On("HasPermissionTo", "user1", mmmodel.PermissionManageSystem).Return(false)
	api.On("GetTeam", "team1").Return(nil, &mmmodel.AppError{
		Message:    "database connection lost",
		StatusCode: http.StatusInternalServerError,
	})

	f := &model.Flow{
		Trigger: model.Trigger{UserJoinedTeam: &model.UserJoinedTeamConfig{TeamID: "team1"}},
	}

	err := CheckFlowPermissions(api, "user1", f)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to verify team")

	var appErr *mmmodel.AppError
	assert.True(t, errors.As(err, &appErr), "error should wrap AppError for 5xx classification")
}

func TestCheckFlowPermissions_UserJoinedTeam_GetTeamNotFound(t *testing.T) {
	api := &plugintest.API{}
	api.On("HasPermissionTo", "user1", mmmodel.PermissionManageSystem).Return(false)
	api.On("GetTeam", "bad-team").Return(nil, &mmmodel.AppError{
		Message:    "team not found",
		StatusCode: http.StatusNotFound,
	})

	f := &model.Flow{
		Trigger: model.Trigger{UserJoinedTeam: &model.UserJoinedTeamConfig{TeamID: "bad-team"}},
	}

	err := CheckFlowPermissions(api, "user1", f)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found or not accessible")
}

func TestCheckFlowPermissions_UserJoinedTeam_NoTeamIDs_RequiresSysAdmin(t *testing.T) {
	api := &plugintest.API{}
	api.On("HasPermissionTo", "user1", mmmodel.PermissionManageSystem).Return(false)

	f := &model.Flow{
		Trigger: model.Trigger{UserJoinedTeam: &model.UserJoinedTeamConfig{TeamID: ""}},
	}

	err := CheckFlowPermissions(api, "user1", f)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "system admin")
}

func TestCheckFlowPermissions_DMParticipantAllowed(t *testing.T) {
	api := &plugintest.API{}
	api.On("HasPermissionTo", "user1", mmmodel.PermissionManageSystem).Return(false)
	api.On("GetChannelMember", "dm1", "user1").Return(
		&mmmodel.ChannelMember{SchemeAdmin: false}, nil,
	)
	api.On("GetChannel", "dm1").Return(
		&mmmodel.Channel{Id: "dm1", Type: mmmodel.ChannelTypeDirect}, nil,
	)

	f := &model.Flow{
		Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "dm1"}},
		Actions: []model.Action{
			{ID: "a1", SendMessage: &model.SendMessageActionConfig{ChannelID: "dm1", Body: "hi"}},
		},
	}
	require.NoError(t, CheckFlowPermissions(api, "user1", f))
	api.AssertExpectations(t)
}

func TestCheckFlowPermissions_GMParticipantAllowed(t *testing.T) {
	api := &plugintest.API{}
	api.On("HasPermissionTo", "user1", mmmodel.PermissionManageSystem).Return(false)
	api.On("GetChannelMember", "gm1", "user1").Return(
		&mmmodel.ChannelMember{SchemeAdmin: false}, nil,
	)
	api.On("GetChannel", "gm1").Return(
		&mmmodel.Channel{Id: "gm1", Type: mmmodel.ChannelTypeGroup}, nil,
	)

	f := &model.Flow{
		Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "gm1"}},
	}
	require.NoError(t, CheckFlowPermissions(api, "user1", f))
	api.AssertExpectations(t)
}

func TestCheckFlowPermissions_RegularChannelNonAdminDenied(t *testing.T) {
	api := &plugintest.API{}
	api.On("HasPermissionTo", "user1", mmmodel.PermissionManageSystem).Return(false)
	api.On("GetChannelMember", "ch1", "user1").Return(
		&mmmodel.ChannelMember{SchemeAdmin: false}, nil,
	)
	api.On("GetChannel", "ch1").Return(
		&mmmodel.Channel{Id: "ch1", Type: mmmodel.ChannelTypeOpen}, nil,
	)

	f := &model.Flow{
		Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
	}
	err := CheckFlowPermissions(api, "user1", f)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "channel admin permissions")
	api.AssertExpectations(t)
}

func TestCanEditFlow_CreatorAllowed(t *testing.T) {
	api := &plugintest.API{}
	api.On("HasPermissionTo", "creator1", mmmodel.PermissionManageSystem).Return(false)

	f := &model.Flow{CreatedBy: "creator1"}
	require.NoError(t, CanEditFlow(api, "creator1", f))
	api.AssertExpectations(t)
}

func TestCanEditFlow_SystemAdminAllowed(t *testing.T) {
	api := &plugintest.API{}
	api.On("HasPermissionTo", "admin1", mmmodel.PermissionManageSystem).Return(true)

	f := &model.Flow{CreatedBy: "creator1"}
	require.NoError(t, CanEditFlow(api, "admin1", f))
	api.AssertExpectations(t)
}

func TestCanEditFlow_NonCreatorDenied(t *testing.T) {
	api := &plugintest.API{}
	api.On("HasPermissionTo", "user2", mmmodel.PermissionManageSystem).Return(false)

	f := &model.Flow{CreatedBy: "creator1"}
	err := CanEditFlow(api, "user2", f)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "automation creator or a system admin")
	api.AssertExpectations(t)
}

func TestCanEditFlow_MissingCreatedByDenied(t *testing.T) {
	api := &plugintest.API{}
	api.On("HasPermissionTo", "user1", mmmodel.PermissionManageSystem).Return(false)

	f := &model.Flow{}
	err := CanEditFlow(api, "user1", f)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "automation creator or a system admin")
	api.AssertExpectations(t)
}

func TestCanEditFlow_NilFlowDenied(t *testing.T) {
	api := &plugintest.API{}
	api.On("HasPermissionTo", "user1", mmmodel.PermissionManageSystem).Return(false)

	err := CanEditFlow(api, "user1", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "automation creator or a system admin")
	api.AssertExpectations(t)
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
