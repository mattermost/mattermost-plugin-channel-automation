package hooks

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/mattermost/mattermost-plugin-agents/public/mcptool"
	mmmodel "github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin/plugintest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

var (
	chAllow        = mmmodel.NewId()
	chDeny         = mmmodel.NewId()
	teamAutomation = mmmodel.NewId() // automation anchor team id
)

// creatorUserID is the default automation CreatedBy used by test helpers; the
// hook handlers require the Mattermost-User-ID header to match it.
const creatorUserID = "user1"

// mockAutomationStore implements model.Store for hook tests.
type mockAutomationStore struct {
	automations map[string]*model.Automation
}

func (m *mockAutomationStore) Get(id string) (*model.Automation, error) {
	return m.automations[id], nil
}
func (m *mockAutomationStore) List() ([]*model.Automation, error) { return nil, nil }

func (m *mockAutomationStore) ListByTriggerChannel(_ string) ([]*model.Automation, error) {
	return nil, nil
}

func (m *mockAutomationStore) ListScheduled() ([]*model.Automation, error) { return nil, nil }
func (m *mockAutomationStore) Save(_ *model.Automation) error              { return nil }
func (m *mockAutomationStore) Delete(_ string) error                       { return nil }
func (m *mockAutomationStore) CountByTriggerChannel(_ string) (int, error) { return 0, nil }

func (m *mockAutomationStore) GetAutomationIDsForChannel(_ string) ([]string, error) {
	return nil, nil
}

func (m *mockAutomationStore) GetAutomationIDsForMembershipChannel(_ string) ([]string, error) {
	return nil, nil
}

func (m *mockAutomationStore) GetChannelCreatedAutomationIDs() ([]string, error) { return nil, nil }

func (m *mockAutomationStore) GetAutomationIDsForUserJoinedTeam(_ string) ([]string, error) {
	return nil, nil
}

func testRouter(t *testing.T, store *mockAutomationStore, api *plugintest.API) *mux.Router {
	t.Helper()
	// Hook handlers emit a debug log at entry that includes per-call key/value
	// pairs for tool_name and args (11 total args).
	api.On("LogDebug",
		mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything,
		mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything,
		mock.Anything,
	).Return().Maybe()
	// authorizeFlowCreator emits a warning when a non-creator caller is rejected.
	api.On("LogWarn",
		mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything,
		mock.Anything, mock.Anything, mock.Anything, mock.Anything,
	).Return().Maybe()
	api.On("GetChannel", chAllow).Return(&mmmodel.Channel{Id: chAllow, TeamId: teamAutomation}, (*mmmodel.AppError)(nil)).Maybe()
	r := mux.NewRouter()
	apiRouter := r.PathPrefix("/api/v1").Subrouter()
	NewAPIHandler(store, api).RegisterRoutes(apiRouter)
	return r
}

// argsJSON marshals a map of hook args to json.RawMessage, matching the
// mcptool.BeforeHookRequest.Args field type.
func argsJSON(t *testing.T, m map[string]any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(m)
	require.NoError(t, err)
	return b
}

func postBefore(t *testing.T, r *mux.Router, automationID, actionID string, body any) (int, mcptool.BeforeHookResponse) {
	return postBeforeAs(t, r, automationID, actionID, creatorUserID, body)
}

func postBeforeAs(t *testing.T, r *mux.Router, automationID, actionID, callerUserID string, body any) (int, mcptool.BeforeHookResponse) {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, json.NewEncoder(&buf).Encode(body))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/hooks/tools/"+automationID+"/"+actionID+"/before", &buf)
	req.Header.Set("Content-Type", "application/json")
	if callerUserID != "" {
		req.Header.Set("Mattermost-User-ID", callerUserID)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	var resp mcptool.BeforeHookResponse
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	return rec.Code, resp
}

func guardrailAutomation() *model.Automation {
	return &model.Automation{
		ID:        "flow1",
		CreatedBy: creatorUserID,
		Trigger: model.Trigger{
			MessagePosted: &model.MessagePostedConfig{ChannelID: chAllow},
		},
		Actions: []model.Action{
			{
				ID: "ai1",
				AIPrompt: &model.AIPromptActionConfig{
					AllowedTools: []string{"search_posts"},
					Guardrails: &model.Guardrails{Channels: []model.GuardrailChannel{
						{ChannelID: chAllow, TeamID: teamAutomation},
					}},
				},
			},
		},
	}
}

func TestHooks_Before_SearchPostsRequiresChannelID(t *testing.T) {
	store := &mockAutomationStore{automations: map[string]*model.Automation{"flow1": guardrailAutomation()}}
	api := &plugintest.API{}
	r := testRouter(t, store, api)

	code, resp := postBefore(t, r, "flow1", "ai1", mcptool.BeforeHookRequest{
		ToolName: "search_posts",
		Args:     argsJSON(t, map[string]any{"query": "hello"}),
		UserID:   "user1",
	})
	require.Equal(t, http.StatusOK, code)
	assert.NotEmpty(t, resp.Error)
	assert.Contains(t, resp.Error, "channel_id")
	assert.Contains(t, resp.Error, chAllow, "error should list allowed channel ids")
}

func TestHooks_Before_SearchPostsChannelNotAllowed(t *testing.T) {
	store := &mockAutomationStore{automations: map[string]*model.Automation{"flow1": guardrailAutomation()}}
	api := &plugintest.API{}
	r := testRouter(t, store, api)

	code, resp := postBefore(t, r, "flow1", "ai1", mcptool.BeforeHookRequest{
		ToolName: "search_posts",
		Args:     argsJSON(t, map[string]any{"query": "hello", "channel_id": chDeny}),
		UserID:   "user1",
	})
	require.Equal(t, http.StatusOK, code)
	assert.NotEmpty(t, resp.Error)
	assert.Contains(t, resp.Error, "not permitted")
	assert.Contains(t, resp.Error, chDeny, "error should echo the rejected channel id")
	assert.Contains(t, resp.Error, chAllow, "error should list allowed channel ids")
}

func TestHooks_Before_SearchPostsOK(t *testing.T) {
	store := &mockAutomationStore{automations: map[string]*model.Automation{"flow1": guardrailAutomation()}}
	api := &plugintest.API{}
	r := testRouter(t, store, api)

	code, resp := postBefore(t, r, "flow1", "ai1", mcptool.BeforeHookRequest{
		ToolName: "search_posts",
		Args:     argsJSON(t, map[string]any{"query": "hello", "channel_id": chAllow}),
		UserID:   "user1",
	})
	require.Equal(t, http.StatusOK, code)
	assert.Empty(t, resp.Error)
}

func TestHooks_Before_ReadChannelOK(t *testing.T) {
	store := &mockAutomationStore{automations: map[string]*model.Automation{"flow1": guardrailAutomation()}}
	api := &plugintest.API{}
	r := testRouter(t, store, api)

	code, resp := postBefore(t, r, "flow1", "ai1", mcptool.BeforeHookRequest{
		ToolName: "read_channel",
		Args:     argsJSON(t, map[string]any{"channel_id": chAllow}),
		UserID:   "user1",
	})
	require.Equal(t, http.StatusOK, code)
	assert.Empty(t, resp.Error)
}

func TestHooks_Before_GetChannelMembersDenied(t *testing.T) {
	store := &mockAutomationStore{automations: map[string]*model.Automation{"flow1": guardrailAutomation()}}
	api := &plugintest.API{}
	r := testRouter(t, store, api)

	code, resp := postBefore(t, r, "flow1", "ai1", mcptool.BeforeHookRequest{
		ToolName: "get_channel_members",
		Args:     argsJSON(t, map[string]any{"channel_id": chDeny}),
		UserID:   "user1",
	})
	require.Equal(t, http.StatusOK, code)
	assert.NotEmpty(t, resp.Error)
}

func TestHooks_Before_AddUserToChannel_RequiresChannelID(t *testing.T) {
	store := &mockAutomationStore{automations: map[string]*model.Automation{"flow1": guardrailAutomation()}}
	api := &plugintest.API{}
	r := testRouter(t, store, api)

	code, resp := postBefore(t, r, "flow1", "ai1", mcptool.BeforeHookRequest{
		ToolName: "add_user_to_channel",
		Args:     argsJSON(t, map[string]any{"user_id": "user1"}),
		UserID:   "user1",
	})
	require.Equal(t, http.StatusOK, code)
	assert.Contains(t, resp.Error, "channel_id")
	assert.Contains(t, resp.Error, chAllow)
}

func TestHooks_Before_AddUserToChannel_RejectsForeignChannel(t *testing.T) {
	store := &mockAutomationStore{automations: map[string]*model.Automation{"flow1": guardrailAutomation()}}
	api := &plugintest.API{}
	r := testRouter(t, store, api)

	code, resp := postBefore(t, r, "flow1", "ai1", mcptool.BeforeHookRequest{
		ToolName: "add_user_to_channel",
		Args:     argsJSON(t, map[string]any{"channel_id": chDeny, "user_id": "user1"}),
		UserID:   "user1",
	})
	require.Equal(t, http.StatusOK, code)
	assert.Contains(t, resp.Error, "not permitted")
	assert.Contains(t, resp.Error, chDeny)
}

func TestHooks_Before_AddUserToChannel_OK(t *testing.T) {
	store := &mockAutomationStore{automations: map[string]*model.Automation{"flow1": guardrailAutomation()}}
	api := &plugintest.API{}
	r := testRouter(t, store, api)

	code, resp := postBefore(t, r, "flow1", "ai1", mcptool.BeforeHookRequest{
		ToolName: "add_user_to_channel",
		Args:     argsJSON(t, map[string]any{"channel_id": chAllow, "user_id": "user1"}),
		UserID:   "user1",
	})
	require.Equal(t, http.StatusOK, code)
	assert.Empty(t, resp.Error)
}

func TestHooks_Before_CreateChannel_RequiresTeamID(t *testing.T) {
	store := &mockAutomationStore{automations: map[string]*model.Automation{"flow1": guardrailAutomation()}}
	api := &plugintest.API{}
	r := testRouter(t, store, api)

	code, resp := postBefore(t, r, "flow1", "ai1", mcptool.BeforeHookRequest{
		ToolName: "create_channel",
		Args:     argsJSON(t, map[string]any{"name": "x", "display_name": "X", "type": "O"}),
		UserID:   "user1",
	})
	require.Equal(t, http.StatusOK, code)
	assert.Contains(t, resp.Error, "requires team_id")
	assert.Contains(t, resp.Error, teamAutomation)
}

func TestHooks_Before_CreateChannel_RejectsForeignTeam(t *testing.T) {
	store := &mockAutomationStore{automations: map[string]*model.Automation{"flow1": guardrailAutomation()}}
	api := &plugintest.API{}
	r := testRouter(t, store, api)

	wrongTeam := mmmodel.NewId()
	code, resp := postBefore(t, r, "flow1", "ai1", mcptool.BeforeHookRequest{
		ToolName: "create_channel",
		Args:     argsJSON(t, map[string]any{"name": "x", "display_name": "X", "type": "O", "team_id": wrongTeam}),
		UserID:   "user1",
	})
	require.Equal(t, http.StatusOK, code)
	assert.Contains(t, resp.Error, "not permitted by guardrails")
	assert.Contains(t, resp.Error, wrongTeam)
	assert.Contains(t, resp.Error, teamAutomation)
}

func TestHooks_Before_CreateChannel_OK(t *testing.T) {
	store := &mockAutomationStore{automations: map[string]*model.Automation{"flow1": guardrailAutomation()}}
	api := &plugintest.API{}
	r := testRouter(t, store, api)

	code, resp := postBefore(t, r, "flow1", "ai1", mcptool.BeforeHookRequest{
		ToolName: "create_channel",
		Args:     argsJSON(t, map[string]any{"name": "x", "display_name": "X", "type": "O", "team_id": teamAutomation}),
		UserID:   "user1",
	})
	require.Equal(t, http.StatusOK, code)
	assert.Empty(t, resp.Error)
}

func TestHooks_Before_GetChannelInfo_RequiresChannelID(t *testing.T) {
	store := &mockAutomationStore{automations: map[string]*model.Automation{"flow1": guardrailAutomation()}}
	api := &plugintest.API{}
	r := testRouter(t, store, api)

	code, resp := postBefore(t, r, "flow1", "ai1", mcptool.BeforeHookRequest{
		ToolName: "get_channel_info",
		Args:     argsJSON(t, map[string]any{"channel_name": "town-square", "team_id": "team1"}),
		UserID:   "user1",
	})
	require.Equal(t, http.StatusOK, code)
	assert.Contains(t, resp.Error, "requires channel_id")
	assert.Contains(t, resp.Error, chAllow)
}

func TestHooks_Before_GetChannelInfo_RejectsForeignChannel(t *testing.T) {
	store := &mockAutomationStore{automations: map[string]*model.Automation{"flow1": guardrailAutomation()}}
	api := &plugintest.API{}
	r := testRouter(t, store, api)

	code, resp := postBefore(t, r, "flow1", "ai1", mcptool.BeforeHookRequest{
		ToolName: "get_channel_info",
		Args:     argsJSON(t, map[string]any{"channel_id": chDeny}),
		UserID:   "user1",
	})
	require.Equal(t, http.StatusOK, code)
	assert.Contains(t, resp.Error, "not permitted")
	assert.Contains(t, resp.Error, chDeny)
}

func TestHooks_Before_GetChannelInfo_OK(t *testing.T) {
	store := &mockAutomationStore{automations: map[string]*model.Automation{"flow1": guardrailAutomation()}}
	api := &plugintest.API{}
	r := testRouter(t, store, api)

	code, resp := postBefore(t, r, "flow1", "ai1", mcptool.BeforeHookRequest{
		ToolName: "get_channel_info",
		Args:     argsJSON(t, map[string]any{"channel_id": chAllow}),
		UserID:   "user1",
	})
	require.Equal(t, http.StatusOK, code)
	assert.Empty(t, resp.Error)
}

func TestHooks_Before_GetUserChannels_RejectedWhenGuardrailsActive(t *testing.T) {
	store := &mockAutomationStore{automations: map[string]*model.Automation{"flow1": guardrailAutomation()}}
	api := &plugintest.API{}
	r := testRouter(t, store, api)

	code, resp := postBefore(t, r, "flow1", "ai1", mcptool.BeforeHookRequest{
		ToolName: "get_user_channels",
		Args:     argsJSON(t, map[string]any{}),
		UserID:   "user1",
	})
	require.Equal(t, http.StatusOK, code)
	assert.Contains(t, resp.Error, "get_user_channels is not permitted when channel guardrails are configured")
}

func TestHooks_Before_ReadPost_RequiresPostID(t *testing.T) {
	store := &mockAutomationStore{automations: map[string]*model.Automation{"flow1": guardrailAutomation()}}
	api := &plugintest.API{}
	r := testRouter(t, store, api)

	code, resp := postBefore(t, r, "flow1", "ai1", mcptool.BeforeHookRequest{
		ToolName: "read_post",
		Args:     argsJSON(t, map[string]any{}),
		UserID:   "user1",
	})
	require.Equal(t, http.StatusOK, code)
	assert.Contains(t, resp.Error, "requires post_id")
	assert.Contains(t, resp.Error, chAllow)
}

func TestHooks_Before_ReadPost_RejectsForeignChannel(t *testing.T) {
	store := &mockAutomationStore{automations: map[string]*model.Automation{"flow1": guardrailAutomation()}}
	api := &plugintest.API{}
	pid := strings.Repeat("p", 26)
	api.On("GetPost", pid).Return(&mmmodel.Post{Id: pid, ChannelId: chDeny}, (*mmmodel.AppError)(nil))
	r := testRouter(t, store, api)

	code, resp := postBefore(t, r, "flow1", "ai1", mcptool.BeforeHookRequest{
		ToolName: "read_post",
		Args:     argsJSON(t, map[string]any{"post_id": pid}),
		UserID:   "user1",
	})
	require.Equal(t, http.StatusOK, code)
	assert.Contains(t, resp.Error, "not permitted by guardrails")
	assert.Contains(t, resp.Error, chDeny)
}

func TestHooks_Before_ReadPost_OK(t *testing.T) {
	store := &mockAutomationStore{automations: map[string]*model.Automation{"flow1": guardrailAutomation()}}
	api := &plugintest.API{}
	pid := strings.Repeat("p", 26)
	api.On("GetPost", pid).Return(&mmmodel.Post{Id: pid, ChannelId: chAllow}, (*mmmodel.AppError)(nil))
	r := testRouter(t, store, api)

	code, resp := postBefore(t, r, "flow1", "ai1", mcptool.BeforeHookRequest{
		ToolName: "read_post",
		Args:     argsJSON(t, map[string]any{"post_id": pid}),
		UserID:   "user1",
	})
	require.Equal(t, http.StatusOK, code)
	assert.Empty(t, resp.Error)
}

func TestHooks_Before_ReadPost_GetPostError(t *testing.T) {
	store := &mockAutomationStore{automations: map[string]*model.Automation{"flow1": guardrailAutomation()}}
	api := &plugintest.API{}
	pid := strings.Repeat("p", 26)
	api.On("GetPost", pid).Return((*mmmodel.Post)(nil), &mmmodel.AppError{Message: "not found"})
	r := testRouter(t, store, api)

	code, resp := postBefore(t, r, "flow1", "ai1", mcptool.BeforeHookRequest{
		ToolName: "read_post",
		Args:     argsJSON(t, map[string]any{"post_id": pid}),
		UserID:   "user1",
	})
	require.Equal(t, http.StatusOK, code)
	assert.Contains(t, resp.Error, "cannot resolve post")
}

func TestHooks_Before_MattermostToolNotSupportedByGuardrails(t *testing.T) {
	store := &mockAutomationStore{automations: map[string]*model.Automation{"flow1": guardrailAutomation()}}
	api := &plugintest.API{}
	r := testRouter(t, store, api)

	code, resp := postBefore(t, r, "flow1", "ai1", mcptool.BeforeHookRequest{
		ToolName: "search_users",
		Args:     argsJSON(t, map[string]any{"term": "x"}),
		UserID:   "user1",
	})
	require.Equal(t, http.StatusOK, code)
	assert.NotEmpty(t, resp.Error)
	assert.Contains(t, resp.Error, "not supported with channel guardrails")
}

func TestHooks_Before_UnrecognizedToolRejected(t *testing.T) {
	store := &mockAutomationStore{automations: map[string]*model.Automation{"flow1": guardrailAutomation()}}
	api := &plugintest.API{}
	r := testRouter(t, store, api)

	code, resp := postBefore(t, r, "flow1", "ai1", mcptool.BeforeHookRequest{
		ToolName: "some_external_or_typo_tool",
		Args:     argsJSON(t, map[string]any{}),
		UserID:   "user1",
	})
	require.Equal(t, http.StatusOK, code)
	assert.NotEmpty(t, resp.Error)
	assert.Contains(t, resp.Error, "not a known Mattermost MCP server tool")
}

func TestHooks_GuardrailsNotFound_MissingFlow(t *testing.T) {
	store := &mockAutomationStore{automations: map[string]*model.Automation{}}
	api := &plugintest.API{}
	r := testRouter(t, store, api)

	code, resp := postBefore(t, r, "flow1", "ai1", mcptool.BeforeHookRequest{
		ToolName: "search_posts",
		Args:     argsJSON(t, map[string]any{"query": "x", "channel_id": chAllow}),
		UserID:   "user1",
	})
	require.Equal(t, http.StatusOK, code)
	assert.NotEmpty(t, resp.Error)
	assert.Contains(t, resp.Error, "guardrails not found")
}

func TestHooks_GuardrailsNotFound_WrongAction(t *testing.T) {
	store := &mockAutomationStore{automations: map[string]*model.Automation{"flow1": guardrailAutomation()}}
	api := &plugintest.API{}
	r := testRouter(t, store, api)

	code, resp := postBefore(t, r, "flow1", "other", mcptool.BeforeHookRequest{
		ToolName: "search_posts",
		Args:     argsJSON(t, map[string]any{"query": "x", "channel_id": chAllow}),
		UserID:   "user1",
	})
	require.Equal(t, http.StatusOK, code)
	assert.NotEmpty(t, resp.Error)
}

func TestHooks_GuardrailsNotFound_NilGuardrailsOnAction(t *testing.T) {
	f := &model.Automation{
		ID:        "flow1",
		CreatedBy: creatorUserID,
		Actions: []model.Action{
			{ID: "ai1", AIPrompt: &model.AIPromptActionConfig{
				Prompt: "x", ProviderType: "agent", ProviderID: "bot", AllowedTools: []string{"search_posts"},
			}},
		},
	}
	store := &mockAutomationStore{automations: map[string]*model.Automation{"flow1": f}}
	api := &plugintest.API{}
	r := testRouter(t, store, api)

	code, resp := postBefore(t, r, "flow1", "ai1", mcptool.BeforeHookRequest{
		ToolName: "search_posts",
		Args:     argsJSON(t, map[string]any{"query": "x", "channel_id": chAllow}),
		UserID:   "user1",
	})
	require.Equal(t, http.StatusOK, code)
	assert.NotEmpty(t, resp.Error)
}

func TestHooks_Before_AllowedChannelsTruncatedWhenLarge(t *testing.T) {
	ids := make([]string, 0, maxAllowedChannelsInError+5)
	for i := range maxAllowedChannelsInError + 5 {
		ids = append(ids, strings.Repeat(string(rune('m'+i)), 26))
	}
	channels := make([]model.GuardrailChannel, 0, len(ids))
	for _, id := range ids {
		channels = append(channels, model.GuardrailChannel{ChannelID: id, TeamID: teamAutomation})
	}
	f := &model.Automation{
		ID:        "flow1",
		CreatedBy: creatorUserID,
		Trigger: model.Trigger{
			MessagePosted: &model.MessagePostedConfig{ChannelID: ids[0]},
		},
		Actions: []model.Action{
			{
				ID: "ai1",
				AIPrompt: &model.AIPromptActionConfig{
					AllowedTools: []string{"search_posts"},
					Guardrails:   &model.Guardrails{Channels: channels},
				},
			},
		},
	}
	store := &mockAutomationStore{automations: map[string]*model.Automation{"flow1": f}}
	api := &plugintest.API{}
	api.On("GetChannel", ids[0]).Return(&mmmodel.Channel{Id: ids[0], TeamId: teamAutomation}, (*mmmodel.AppError)(nil))
	r := testRouter(t, store, api)

	code, resp := postBefore(t, r, "flow1", "ai1", mcptool.BeforeHookRequest{
		ToolName: "search_posts",
		Args:     argsJSON(t, map[string]any{"query": "hi", "channel_id": chDeny}),
		UserID:   "user1",
	})
	require.Equal(t, http.StatusOK, code)
	assert.Contains(t, resp.Error, "(+5 more)")
	assert.NotContains(t, resp.Error, ids[maxAllowedChannelsInError], "should not include channels past the cap")
}

func flowChannelCreatedTeam(teamID string) *model.Automation {
	return &model.Automation{
		ID:        "flow1",
		CreatedBy: creatorUserID,
		Trigger: model.Trigger{
			ChannelCreated: &model.ChannelCreatedConfig{TeamID: teamID},
		},
		Actions: []model.Action{
			{
				ID: "ai1",
				AIPrompt: &model.AIPromptActionConfig{
					AllowedTools: []string{"get_team_info"},
					Guardrails: &model.Guardrails{Channels: []model.GuardrailChannel{
						{ChannelID: chAllow, TeamID: teamID},
					}},
				},
			},
		},
	}
}

// automationUserJoinedTeam returns a guardrail automation whose trigger is user_joined_team
// on teamID. Used to verify the anchor-team resolution path includes the
// user_joined_team trigger.
func automationUserJoinedTeam(teamID string) *model.Automation {
	return &model.Automation{
		ID:        "flow1",
		CreatedBy: creatorUserID,
		Trigger: model.Trigger{
			UserJoinedTeam: &model.UserJoinedTeamConfig{TeamID: teamID},
		},
		Actions: []model.Action{
			{
				ID: "ai1",
				AIPrompt: &model.AIPromptActionConfig{
					AllowedTools: []string{"get_team_info"},
					Guardrails: &model.Guardrails{Channels: []model.GuardrailChannel{
						{ChannelID: chAllow, TeamID: teamID},
					}},
				},
			},
		},
	}
}

func TestHooks_Before_GetTeamInfo_UserJoinedTeam_OK(t *testing.T) {
	team1 := mmmodel.NewId()
	store := &mockAutomationStore{automations: map[string]*model.Automation{"flow1": automationUserJoinedTeam(team1)}}
	api := &plugintest.API{}
	r := testRouter(t, store, api)

	code, resp := postBefore(t, r, "flow1", "ai1", mcptool.BeforeHookRequest{
		ToolName: "get_team_info",
		Args:     argsJSON(t, map[string]any{"team_id": team1}),
		UserID:   "user1",
	})
	require.Equal(t, http.StatusOK, code)
	assert.Empty(t, resp.Error)
}

func TestHooks_Before_GetTeamInfo_ChannelCreated_OK(t *testing.T) {
	team1 := mmmodel.NewId()
	store := &mockAutomationStore{automations: map[string]*model.Automation{"flow1": flowChannelCreatedTeam(team1)}}
	api := &plugintest.API{}
	r := testRouter(t, store, api)

	code, resp := postBefore(t, r, "flow1", "ai1", mcptool.BeforeHookRequest{
		ToolName: "get_team_info",
		Args:     argsJSON(t, map[string]any{"team_id": team1}),
		UserID:   "user1",
	})
	require.Equal(t, http.StatusOK, code)
	assert.Empty(t, resp.Error)
}

func TestHooks_Before_GetTeamInfo_ChannelCreated_WrongTeam(t *testing.T) {
	team1 := mmmodel.NewId()
	team2 := mmmodel.NewId()
	store := &mockAutomationStore{automations: map[string]*model.Automation{"flow1": flowChannelCreatedTeam(team1)}}
	api := &plugintest.API{}
	r := testRouter(t, store, api)

	code, resp := postBefore(t, r, "flow1", "ai1", mcptool.BeforeHookRequest{
		ToolName: "get_team_info",
		Args:     argsJSON(t, map[string]any{"team_id": team2}),
		UserID:   "user1",
	})
	require.Equal(t, http.StatusOK, code)
	assert.Contains(t, resp.Error, "not permitted by guardrails")
	assert.Contains(t, resp.Error, team2)
	assert.Contains(t, resp.Error, team1)
}

func TestHooks_Before_GetTeamInfo_RequiresTeamID(t *testing.T) {
	team1 := mmmodel.NewId()
	store := &mockAutomationStore{automations: map[string]*model.Automation{"flow1": flowChannelCreatedTeam(team1)}}
	api := &plugintest.API{}
	r := testRouter(t, store, api)

	code, resp := postBefore(t, r, "flow1", "ai1", mcptool.BeforeHookRequest{
		ToolName: "get_team_info",
		Args:     argsJSON(t, map[string]any{"team_name": "Engineering"}),
		UserID:   "user1",
	})
	require.Equal(t, http.StatusOK, code)
	assert.Contains(t, resp.Error, "requires team_id")
	assert.Contains(t, resp.Error, team1)
}

func TestHooks_Before_GetTeamMembers_MultiTeamAllowed(t *testing.T) {
	// Guardrail spans two teams: chAllow in teamAutomation and chOther in teamOther.
	chOther := mmmodel.NewId()
	teamOther := mmmodel.NewId()
	f := guardrailAutomation()
	f.Actions[0].AIPrompt.Guardrails = &model.Guardrails{Channels: []model.GuardrailChannel{
		{ChannelID: chAllow, TeamID: teamAutomation},
		{ChannelID: chOther, TeamID: teamOther},
	}}
	f.Actions[0].AIPrompt.AllowedTools = []string{"get_team_members"}
	store := &mockAutomationStore{automations: map[string]*model.Automation{"flow1": f}}
	api := &plugintest.API{}
	r := testRouter(t, store, api)

	code, resp := postBefore(t, r, "flow1", "ai1", mcptool.BeforeHookRequest{
		ToolName: "get_team_members",
		Args:     argsJSON(t, map[string]any{"team_id": teamOther}),
		UserID:   "user1",
	})
	require.Equal(t, http.StatusOK, code)
	assert.Empty(t, resp.Error)
}

func TestHooks_Before_GetTeamMembers_MessagePosted_OK(t *testing.T) {
	store := &mockAutomationStore{automations: map[string]*model.Automation{"flow1": guardrailAutomation()}}
	api := &plugintest.API{}
	r := testRouter(t, store, api)

	code, resp := postBefore(t, r, "flow1", "ai1", mcptool.BeforeHookRequest{
		ToolName: "get_team_members",
		Args:     argsJSON(t, map[string]any{"team_id": teamAutomation}),
		UserID:   "user1",
	})
	require.Equal(t, http.StatusOK, code)
	assert.Empty(t, resp.Error)
}

func TestHooks_Before_GetTeamMembers_MessagePosted_WrongTeam(t *testing.T) {
	store := &mockAutomationStore{automations: map[string]*model.Automation{"flow1": guardrailAutomation()}}
	api := &plugintest.API{}
	r := testRouter(t, store, api)

	wrongTeam := mmmodel.NewId()
	code, resp := postBefore(t, r, "flow1", "ai1", mcptool.BeforeHookRequest{
		ToolName: "get_team_members",
		Args:     argsJSON(t, map[string]any{"team_id": wrongTeam}),
		UserID:   "user1",
	})
	require.Equal(t, http.StatusOK, code)
	assert.Contains(t, resp.Error, "not permitted by guardrails")
}

func TestHooks_Before_RejectsNonCreatorCaller(t *testing.T) {
	store := &mockAutomationStore{automations: map[string]*model.Automation{"flow1": guardrailAutomation()}}
	api := &plugintest.API{}
	r := testRouter(t, store, api)

	code, resp := postBeforeAs(t, r, "flow1", "ai1", "someone-else", mcptool.BeforeHookRequest{
		ToolName: "search_posts",
		Args:     argsJSON(t, map[string]any{"query": "hi", "channel_id": chAllow}),
		UserID:   creatorUserID,
	})
	require.Equal(t, http.StatusForbidden, code)
	assert.Contains(t, resp.Error, "forbidden")
}

func TestHooks_Before_RejectsMissingCallerHeader(t *testing.T) {
	store := &mockAutomationStore{automations: map[string]*model.Automation{"flow1": guardrailAutomation()}}
	api := &plugintest.API{}
	r := testRouter(t, store, api)

	code, resp := postBeforeAs(t, r, "flow1", "ai1", "", mcptool.BeforeHookRequest{
		ToolName: "search_posts",
		Args:     argsJSON(t, map[string]any{"query": "hi", "channel_id": chAllow}),
		UserID:   creatorUserID,
	})
	require.Equal(t, http.StatusForbidden, code)
	assert.Contains(t, resp.Error, "forbidden")
}

func TestHooks_RejectsFlowMissingCreator(t *testing.T) {
	f := guardrailAutomation()
	f.CreatedBy = ""
	store := &mockAutomationStore{automations: map[string]*model.Automation{"flow1": f}}
	api := &plugintest.API{}
	r := testRouter(t, store, api)

	code, _ := postBeforeAs(t, r, "flow1", "ai1", creatorUserID, mcptool.BeforeHookRequest{
		ToolName: "search_posts",
		Args:     argsJSON(t, map[string]any{"query": "hi", "channel_id": chAllow}),
		UserID:   creatorUserID,
	})
	require.Equal(t, http.StatusForbidden, code)
}
