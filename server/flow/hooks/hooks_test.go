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

const chAllow = "aaaaaaaaaaaaaaaaaaaaaaaaaa"
const chDeny = "bbbbbbbbbbbbbbbbbbbbbbbbbb"
const teamAutomation = "tttttttttttttttttttttttttt" // 26-char test team id (automation anchor)

// mockFlowStore implements model.Store for hook tests.
type mockFlowStore struct {
	flows map[string]*model.Flow
}

func (m *mockFlowStore) Get(id string) (*model.Flow, error) { return m.flows[id], nil }
func (m *mockFlowStore) List() ([]*model.Flow, error)       { return nil, nil }
func (m *mockFlowStore) ListByTriggerChannel(_ string) ([]*model.Flow, error) {
	return nil, nil
}
func (m *mockFlowStore) ListScheduled() ([]*model.Flow, error)       { return nil, nil }
func (m *mockFlowStore) Save(_ *model.Flow) error                    { return nil }
func (m *mockFlowStore) Delete(_ string) error                       { return nil }
func (m *mockFlowStore) CountByTriggerChannel(_ string) (int, error) { return 0, nil }
func (m *mockFlowStore) GetFlowIDsForChannel(_ string) ([]string, error) {
	return nil, nil
}
func (m *mockFlowStore) GetFlowIDsForMembershipChannel(_ string) ([]string, error) {
	return nil, nil
}
func (m *mockFlowStore) GetChannelCreatedFlowIDs() ([]string, error) { return nil, nil }

func testRouter(t *testing.T, store *mockFlowStore, api *plugintest.API) *mux.Router {
	t.Helper()
	// Hook handlers emit a debug log at entry that includes per-call key/value
	// pairs for tool_name and either args (before, 11 total args) or output
	// (after, 9 total args). Allow both shapes optionally.
	api.On("LogDebug",
		mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything,
		mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything,
		mock.Anything,
	).Return().Maybe()
	api.On("LogDebug",
		mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything,
		mock.Anything, mock.Anything, mock.Anything, mock.Anything,
	).Return().Maybe()
	api.On("GetChannel", chAllow).Return(&mmmodel.Channel{Id: chAllow, TeamId: teamAutomation}, (*mmmodel.AppError)(nil)).Maybe()
	r := mux.NewRouter()
	apiRouter := r.PathPrefix("/api/v1").Subrouter()
	NewAPIHandler(store, api).RegisterRoutes(apiRouter)
	return r
}

func postBefore(t *testing.T, r *mux.Router, flowID, actionID string, body any) (int, mcptool.BeforeHookResponse) {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, json.NewEncoder(&buf).Encode(body))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/hooks/tools/"+flowID+"/"+actionID+"/before", &buf)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	var resp mcptool.BeforeHookResponse
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	return rec.Code, resp
}

func postAfter(t *testing.T, r *mux.Router, flowID, actionID string, body any) (int, mcptool.AfterHookResponse) {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, json.NewEncoder(&buf).Encode(body))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/hooks/tools/"+flowID+"/"+actionID+"/after", &buf)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	var resp mcptool.AfterHookResponse
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	return rec.Code, resp
}

func guardrailFlow() *model.Flow {
	return &model.Flow{
		ID: "flow1",
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
	store := &mockFlowStore{flows: map[string]*model.Flow{"flow1": guardrailFlow()}}
	api := &plugintest.API{}
	r := testRouter(t, store, api)

	code, resp := postBefore(t, r, "flow1", "ai1", mcptool.BeforeHookRequest{
		ToolName: "search_posts",
		Args:     map[string]any{"query": "hello"},
		UserID:   "user1",
	})
	require.Equal(t, http.StatusOK, code)
	assert.NotEmpty(t, resp.Error)
	assert.Contains(t, resp.Error, "channel_id")
	assert.Contains(t, resp.Error, chAllow, "error should list allowed channel ids")
}

func TestHooks_Before_SearchPostsChannelNotAllowed(t *testing.T) {
	store := &mockFlowStore{flows: map[string]*model.Flow{"flow1": guardrailFlow()}}
	api := &plugintest.API{}
	r := testRouter(t, store, api)

	code, resp := postBefore(t, r, "flow1", "ai1", mcptool.BeforeHookRequest{
		ToolName: "search_posts",
		Args:     map[string]any{"query": "hello", "channel_id": chDeny},
		UserID:   "user1",
	})
	require.Equal(t, http.StatusOK, code)
	assert.NotEmpty(t, resp.Error)
	assert.Contains(t, resp.Error, "not permitted")
	assert.Contains(t, resp.Error, chDeny, "error should echo the rejected channel id")
	assert.Contains(t, resp.Error, chAllow, "error should list allowed channel ids")
}

func TestHooks_Before_SearchPostsOK(t *testing.T) {
	store := &mockFlowStore{flows: map[string]*model.Flow{"flow1": guardrailFlow()}}
	api := &plugintest.API{}
	r := testRouter(t, store, api)

	code, resp := postBefore(t, r, "flow1", "ai1", mcptool.BeforeHookRequest{
		ToolName: "search_posts",
		Args:     map[string]any{"query": "hello", "channel_id": chAllow},
		UserID:   "user1",
	})
	require.Equal(t, http.StatusOK, code)
	assert.Empty(t, resp.Error)
}

func TestHooks_Before_ReadChannelOK(t *testing.T) {
	store := &mockFlowStore{flows: map[string]*model.Flow{"flow1": guardrailFlow()}}
	api := &plugintest.API{}
	r := testRouter(t, store, api)

	code, resp := postBefore(t, r, "flow1", "ai1", mcptool.BeforeHookRequest{
		ToolName: "read_channel",
		Args:     map[string]any{"channel_id": chAllow},
		UserID:   "user1",
	})
	require.Equal(t, http.StatusOK, code)
	assert.Empty(t, resp.Error)
}

func TestHooks_Before_GetChannelMembersDenied(t *testing.T) {
	store := &mockFlowStore{flows: map[string]*model.Flow{"flow1": guardrailFlow()}}
	api := &plugintest.API{}
	r := testRouter(t, store, api)

	code, resp := postBefore(t, r, "flow1", "ai1", mcptool.BeforeHookRequest{
		ToolName: "get_channel_members",
		Args:     map[string]any{"channel_id": chDeny},
		UserID:   "user1",
	})
	require.Equal(t, http.StatusOK, code)
	assert.NotEmpty(t, resp.Error)
}

func TestHooks_Before_AddUserToChannel_RequiresChannelID(t *testing.T) {
	store := &mockFlowStore{flows: map[string]*model.Flow{"flow1": guardrailFlow()}}
	api := &plugintest.API{}
	r := testRouter(t, store, api)

	code, resp := postBefore(t, r, "flow1", "ai1", mcptool.BeforeHookRequest{
		ToolName: "add_user_to_channel",
		Args:     map[string]any{"user_id": "user1"},
		UserID:   "user1",
	})
	require.Equal(t, http.StatusOK, code)
	assert.Contains(t, resp.Error, "channel_id")
	assert.Contains(t, resp.Error, chAllow)
}

func TestHooks_Before_AddUserToChannel_RejectsForeignChannel(t *testing.T) {
	store := &mockFlowStore{flows: map[string]*model.Flow{"flow1": guardrailFlow()}}
	api := &plugintest.API{}
	r := testRouter(t, store, api)

	code, resp := postBefore(t, r, "flow1", "ai1", mcptool.BeforeHookRequest{
		ToolName: "add_user_to_channel",
		Args:     map[string]any{"channel_id": chDeny, "user_id": "user1"},
		UserID:   "user1",
	})
	require.Equal(t, http.StatusOK, code)
	assert.Contains(t, resp.Error, "not permitted")
	assert.Contains(t, resp.Error, chDeny)
}

func TestHooks_Before_AddUserToChannel_OK(t *testing.T) {
	store := &mockFlowStore{flows: map[string]*model.Flow{"flow1": guardrailFlow()}}
	api := &plugintest.API{}
	r := testRouter(t, store, api)

	code, resp := postBefore(t, r, "flow1", "ai1", mcptool.BeforeHookRequest{
		ToolName: "add_user_to_channel",
		Args:     map[string]any{"channel_id": chAllow, "user_id": "user1"},
		UserID:   "user1",
	})
	require.Equal(t, http.StatusOK, code)
	assert.Empty(t, resp.Error)
}

func TestHooks_Before_GetChannelInfo_ResolveAndReject(t *testing.T) {
	store := &mockFlowStore{flows: map[string]*model.Flow{"flow1": guardrailFlow()}}
	api := &plugintest.API{}
	api.On("GetChannelByName", "team1", "town-square", false).Return(&mmmodel.Channel{Id: chDeny, TeamId: "team1"}, (*mmmodel.AppError)(nil))
	r := testRouter(t, store, api)

	code, resp := postBefore(t, r, "flow1", "ai1", mcptool.BeforeHookRequest{
		ToolName: "get_channel_info",
		Args:     map[string]any{"channel_name": "town-square", "team_id": "team1"},
		UserID:   "user1",
	})
	require.Equal(t, http.StatusOK, code)
	assert.NotEmpty(t, resp.Error)
	api.AssertExpectations(t)
}

func TestHooks_Before_GetChannelInfo_ResolveOK(t *testing.T) {
	store := &mockFlowStore{flows: map[string]*model.Flow{"flow1": guardrailFlow()}}
	api := &plugintest.API{}
	api.On("GetChannelByName", "team1", "town-square", false).Return(&mmmodel.Channel{Id: chAllow, TeamId: "team1"}, (*mmmodel.AppError)(nil))
	r := testRouter(t, store, api)

	code, resp := postBefore(t, r, "flow1", "ai1", mcptool.BeforeHookRequest{
		ToolName: "get_channel_info",
		Args:     map[string]any{"channel_name": "town-square", "team_id": "team1"},
		UserID:   "user1",
	})
	require.Equal(t, http.StatusOK, code)
	assert.Empty(t, resp.Error)
	api.AssertExpectations(t)
}

func TestHooks_Before_GetUserChannelsPassThrough(t *testing.T) {
	store := &mockFlowStore{flows: map[string]*model.Flow{"flow1": guardrailFlow()}}
	api := &plugintest.API{}
	r := testRouter(t, store, api)

	code, resp := postBefore(t, r, "flow1", "ai1", mcptool.BeforeHookRequest{
		ToolName: "get_user_channels",
		Args:     map[string]any{},
		UserID:   "user1",
	})
	require.Equal(t, http.StatusOK, code)
	assert.Empty(t, resp.Error)
}

func TestHooks_Before_ReadPost_NotRegistered(t *testing.T) {
	// read_post is allowlisted with only an After hook in the catalog, so the
	// bridge does not register a Before callback for it. If a Before request
	// nevertheless arrives, the handler rejects it as an unsupported direction.
	store := &mockFlowStore{flows: map[string]*model.Flow{"flow1": guardrailFlow()}}
	api := &plugintest.API{}
	r := testRouter(t, store, api)

	code, resp := postBefore(t, r, "flow1", "ai1", mcptool.BeforeHookRequest{
		ToolName: "read_post",
		Args:     map[string]any{"post_id": strings.Repeat("p", 26)},
		UserID:   "user1",
	})
	require.Equal(t, http.StatusOK, code)
	assert.Contains(t, resp.Error, "not supported with channel guardrails")
}

func TestHooks_Before_MattermostToolNotSupportedByGuardrails(t *testing.T) {
	store := &mockFlowStore{flows: map[string]*model.Flow{"flow1": guardrailFlow()}}
	api := &plugintest.API{}
	r := testRouter(t, store, api)

	code, resp := postBefore(t, r, "flow1", "ai1", mcptool.BeforeHookRequest{
		ToolName: "search_users",
		Args:     map[string]any{"term": "x"},
		UserID:   "user1",
	})
	require.Equal(t, http.StatusOK, code)
	assert.NotEmpty(t, resp.Error)
	assert.Contains(t, resp.Error, "not supported with channel guardrails")
}

func TestHooks_Before_UnrecognizedToolRejected(t *testing.T) {
	store := &mockFlowStore{flows: map[string]*model.Flow{"flow1": guardrailFlow()}}
	api := &plugintest.API{}
	r := testRouter(t, store, api)

	code, resp := postBefore(t, r, "flow1", "ai1", mcptool.BeforeHookRequest{
		ToolName: "some_external_or_typo_tool",
		Args:     map[string]any{},
		UserID:   "user1",
	})
	require.Equal(t, http.StatusOK, code)
	assert.NotEmpty(t, resp.Error)
	assert.Contains(t, resp.Error, "not a known Mattermost MCP server tool")
}

func TestHooks_GuardrailsNotFound_MissingFlow(t *testing.T) {
	store := &mockFlowStore{flows: map[string]*model.Flow{}}
	api := &plugintest.API{}
	r := testRouter(t, store, api)

	code, resp := postBefore(t, r, "flow1", "ai1", mcptool.BeforeHookRequest{
		ToolName: "search_posts",
		Args:     map[string]any{"query": "x", "channel_id": chAllow},
		UserID:   "user1",
	})
	require.Equal(t, http.StatusOK, code)
	assert.NotEmpty(t, resp.Error)
	assert.Contains(t, resp.Error, "guardrails not found")
}

func TestHooks_GuardrailsNotFound_WrongAction(t *testing.T) {
	store := &mockFlowStore{flows: map[string]*model.Flow{"flow1": guardrailFlow()}}
	api := &plugintest.API{}
	r := testRouter(t, store, api)

	code, resp := postBefore(t, r, "flow1", "other", mcptool.BeforeHookRequest{
		ToolName: "search_posts",
		Args:     map[string]any{"query": "x", "channel_id": chAllow},
		UserID:   "user1",
	})
	require.Equal(t, http.StatusOK, code)
	assert.NotEmpty(t, resp.Error)
}

func TestHooks_GuardrailsNotFound_NilGuardrailsOnAction(t *testing.T) {
	f := &model.Flow{
		ID: "flow1",
		Actions: []model.Action{
			{ID: "ai1", AIPrompt: &model.AIPromptActionConfig{
				Prompt: "x", ProviderType: "agent", ProviderID: "bot", AllowedTools: []string{"search_posts"},
			}},
		},
	}
	store := &mockFlowStore{flows: map[string]*model.Flow{"flow1": f}}
	api := &plugintest.API{}
	r := testRouter(t, store, api)

	code, resp := postBefore(t, r, "flow1", "ai1", mcptool.BeforeHookRequest{
		ToolName: "search_posts",
		Args:     map[string]any{"query": "x", "channel_id": chAllow},
		UserID:   "user1",
	})
	require.Equal(t, http.StatusOK, code)
	assert.NotEmpty(t, resp.Error)
}

func TestHooks_After_SearchPostsFilters(t *testing.T) {
	store := &mockFlowStore{flows: map[string]*model.Flow{"flow1": guardrailFlow()}}
	api := &plugintest.API{}
	r := testRouter(t, store, api)

	pid1 := strings.Repeat("p", 26)
	pid2 := strings.Repeat("q", 26)
	out := mcptool.SearchPostsOutput{
		Query:           "hi",
		SemanticEnabled: true,
		SemanticResults: []mcptool.SearchPostResult{
			{Post: &mmmodel.Post{Id: pid1, ChannelId: chDeny, Message: "a"}},
			{Post: &mmmodel.Post{Id: pid2, ChannelId: chAllow, Message: "b"}},
		},
		KeywordResults: []mcptool.SearchPostResult{
			{Post: &mmmodel.Post{Id: strings.Repeat("r", 26), ChannelId: chAllow, Message: "c"}},
		},
	}
	raw, err := json.Marshal(out)
	require.NoError(t, err)

	code, resp := postAfter(t, r, "flow1", "ai1", mcptool.AfterHookRequest{
		ToolName: "search_posts",
		Output:   raw,
	})
	require.Equal(t, http.StatusOK, code)
	require.Empty(t, resp.Error)
	require.NotEmpty(t, resp.Output)

	var parsed mcptool.SearchPostsOutput
	require.NoError(t, json.Unmarshal(resp.Output, &parsed))
	require.Len(t, parsed.SemanticResults, 1)
	assert.Equal(t, chAllow, parsed.SemanticResults[0].Post.ChannelId)
	require.Len(t, parsed.KeywordResults, 1)
	assert.Contains(t, strings.Join(parsed.PluginAnnotations, " "), "removed")
}

func TestHooks_After_ReadChannelDenied(t *testing.T) {
	store := &mockFlowStore{flows: map[string]*model.Flow{"flow1": guardrailFlow()}}
	api := &plugintest.API{}
	r := testRouter(t, store, api)

	out := mcptool.ReadChannelOutput{
		Channel: &mmmodel.Channel{Id: chDeny},
		Posts:   []*mmmodel.Post{{Id: strings.Repeat("p", 26), ChannelId: chDeny}},
	}
	raw, err := json.Marshal(out)
	require.NoError(t, err)

	code, resp := postAfter(t, r, "flow1", "ai1", mcptool.AfterHookRequest{ToolName: "read_channel", Output: raw})
	require.Equal(t, http.StatusOK, code)
	assert.NotEmpty(t, resp.Error)
	assert.Empty(t, resp.Output)
	_ = code
}

func TestHooks_After_ReadPostDenied(t *testing.T) {
	store := &mockFlowStore{flows: map[string]*model.Flow{"flow1": guardrailFlow()}}
	api := &plugintest.API{}
	r := testRouter(t, store, api)

	out := mcptool.ReadPostOutput{
		Posts: []*mmmodel.Post{{Id: strings.Repeat("p", 26), ChannelId: chDeny, Message: "m"}},
	}
	raw, err := json.Marshal(out)
	require.NoError(t, err)

	_, resp := postAfter(t, r, "flow1", "ai1", mcptool.AfterHookRequest{ToolName: "read_post", Output: raw})
	assert.NotEmpty(t, resp.Error)
}

func TestHooks_After_MattermostToolNotSupportedByGuardrails(t *testing.T) {
	store := &mockFlowStore{flows: map[string]*model.Flow{"flow1": guardrailFlow()}}
	api := &plugintest.API{}
	r := testRouter(t, store, api)

	_, resp := postAfter(t, r, "flow1", "ai1", mcptool.AfterHookRequest{
		ToolName: "search_users",
		Output:   json.RawMessage(`{}`),
	})
	assert.Contains(t, resp.Error, "not supported with channel guardrails")
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
	f := &model.Flow{
		ID: "flow1",
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
	store := &mockFlowStore{flows: map[string]*model.Flow{"flow1": f}}
	api := &plugintest.API{}
	api.On("GetChannel", ids[0]).Return(&mmmodel.Channel{Id: ids[0], TeamId: teamAutomation}, (*mmmodel.AppError)(nil))
	r := testRouter(t, store, api)

	code, resp := postBefore(t, r, "flow1", "ai1", mcptool.BeforeHookRequest{
		ToolName: "search_posts",
		Args:     map[string]any{"query": "hi", "channel_id": chDeny},
		UserID:   "user1",
	})
	require.Equal(t, http.StatusOK, code)
	assert.Contains(t, resp.Error, "(+5 more)")
	assert.NotContains(t, resp.Error, ids[maxAllowedChannelsInError], "should not include channels past the cap")
}

func TestHooks_After_UnrecognizedToolRejected(t *testing.T) {
	store := &mockFlowStore{flows: map[string]*model.Flow{"flow1": guardrailFlow()}}
	api := &plugintest.API{}
	r := testRouter(t, store, api)

	_, resp := postAfter(t, r, "flow1", "ai1", mcptool.AfterHookRequest{
		ToolName: "custom_plugin_tool_xyz",
		Output:   json.RawMessage(`{}`),
	})
	assert.Contains(t, resp.Error, "not a known Mattermost MCP server tool")
}

func TestHooks_After_ResolverErrorPassthrough(t *testing.T) {
	store := &mockFlowStore{flows: map[string]*model.Flow{"flow1": guardrailFlow()}}
	api := &plugintest.API{}
	r := testRouter(t, store, api)

	code, resp := postAfter(t, r, "flow1", "ai1", mcptool.AfterHookRequest{
		ToolName: "search_posts",
		Error:    "resolver failed",
	})
	require.Equal(t, http.StatusOK, code)
	assert.Empty(t, resp.Error)
	assert.Empty(t, resp.Output)
}

func flowChannelCreatedTeam(teamID string) *model.Flow {
	return &model.Flow{
		ID: "flow1",
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

func TestHooks_Before_GetTeamInfo_ChannelCreated_OK(t *testing.T) {
	const team1 = "wwwwwwwwwwwwwwwwwwwwwwwwww"
	store := &mockFlowStore{flows: map[string]*model.Flow{"flow1": flowChannelCreatedTeam(team1)}}
	api := &plugintest.API{}
	r := testRouter(t, store, api)

	code, resp := postBefore(t, r, "flow1", "ai1", mcptool.BeforeHookRequest{
		ToolName: "get_team_info",
		Args:     map[string]any{"team_id": team1},
		UserID:   "user1",
	})
	require.Equal(t, http.StatusOK, code)
	assert.Empty(t, resp.Error)
}

func TestHooks_Before_GetTeamInfo_ChannelCreated_WrongTeam(t *testing.T) {
	const team1 = "wwwwwwwwwwwwwwwwwwwwwwwwww"
	const team2 = "xxxxxxxxxxxxxxxxxxxxxxxxxx"
	store := &mockFlowStore{flows: map[string]*model.Flow{"flow1": flowChannelCreatedTeam(team1)}}
	api := &plugintest.API{}
	r := testRouter(t, store, api)

	code, resp := postBefore(t, r, "flow1", "ai1", mcptool.BeforeHookRequest{
		ToolName: "get_team_info",
		Args:     map[string]any{"team_id": team2},
		UserID:   "user1",
	})
	require.Equal(t, http.StatusOK, code)
	assert.Contains(t, resp.Error, "not permitted by guardrails")
	assert.Contains(t, resp.Error, team2)
	assert.Contains(t, resp.Error, team1)
}

func TestHooks_Before_GetTeamInfo_RequiresTeamID(t *testing.T) {
	const team1 = "wwwwwwwwwwwwwwwwwwwwwwwwww"
	store := &mockFlowStore{flows: map[string]*model.Flow{"flow1": flowChannelCreatedTeam(team1)}}
	api := &plugintest.API{}
	r := testRouter(t, store, api)

	code, resp := postBefore(t, r, "flow1", "ai1", mcptool.BeforeHookRequest{
		ToolName: "get_team_info",
		Args:     map[string]any{"team_name": "Engineering"},
		UserID:   "user1",
	})
	require.Equal(t, http.StatusOK, code)
	assert.Contains(t, resp.Error, "requires team_id")
	assert.Contains(t, resp.Error, team1)
}

func TestHooks_Before_GetTeamMembers_MultiTeamAllowed(t *testing.T) {
	// Guardrail spans two teams: chAllow in teamAutomation and chOther in teamOther.
	const chOther = "cccccccccccccccccccccccccc"
	const teamOther = "ssssssssssssssssssssssssss"
	f := guardrailFlow()
	f.Actions[0].AIPrompt.Guardrails = &model.Guardrails{Channels: []model.GuardrailChannel{
		{ChannelID: chAllow, TeamID: teamAutomation},
		{ChannelID: chOther, TeamID: teamOther},
	}}
	f.Actions[0].AIPrompt.AllowedTools = []string{"get_team_members"}
	store := &mockFlowStore{flows: map[string]*model.Flow{"flow1": f}}
	api := &plugintest.API{}
	r := testRouter(t, store, api)

	code, resp := postBefore(t, r, "flow1", "ai1", mcptool.BeforeHookRequest{
		ToolName: "get_team_members",
		Args:     map[string]any{"team_id": teamOther},
		UserID:   "user1",
	})
	require.Equal(t, http.StatusOK, code)
	assert.Empty(t, resp.Error)
}

func TestHooks_Before_GetTeamMembers_MessagePosted_OK(t *testing.T) {
	store := &mockFlowStore{flows: map[string]*model.Flow{"flow1": guardrailFlow()}}
	api := &plugintest.API{}
	r := testRouter(t, store, api)

	code, resp := postBefore(t, r, "flow1", "ai1", mcptool.BeforeHookRequest{
		ToolName: "get_team_members",
		Args:     map[string]any{"team_id": teamAutomation},
		UserID:   "user1",
	})
	require.Equal(t, http.StatusOK, code)
	assert.Empty(t, resp.Error)
}

func TestHooks_Before_GetTeamMembers_MessagePosted_WrongTeam(t *testing.T) {
	store := &mockFlowStore{flows: map[string]*model.Flow{"flow1": guardrailFlow()}}
	api := &plugintest.API{}
	r := testRouter(t, store, api)

	wrongTeam := "uuuuuuuuuuuuuuuuuuuuuuuuuu"
	code, resp := postBefore(t, r, "flow1", "ai1", mcptool.BeforeHookRequest{
		ToolName: "get_team_members",
		Args:     map[string]any{"team_id": wrongTeam},
		UserID:   "user1",
	})
	require.Equal(t, http.StatusOK, code)
	assert.Contains(t, resp.Error, "not permitted by guardrails")
}

func TestHooks_After_GetTeamInfo_FilterCandidates(t *testing.T) {
	store := &mockFlowStore{flows: map[string]*model.Flow{"flow1": guardrailFlow()}}
	api := &plugintest.API{}
	r := testRouter(t, store, api)

	otherTeam := "uuuuuuuuuuuuuuuuuuuuuuuuuu"
	out := mcptool.TeamInfoOutput{
		Teams: []*mmmodel.Team{
			{Id: otherTeam, DisplayName: "Other"},
			{Id: teamAutomation, DisplayName: "Mine"},
		},
	}
	raw, err := json.Marshal(out)
	require.NoError(t, err)

	code, resp := postAfter(t, r, "flow1", "ai1", mcptool.AfterHookRequest{
		ToolName: "get_team_info",
		Output:   raw,
	})
	require.Equal(t, http.StatusOK, code)
	require.Empty(t, resp.Error)
	require.NotEmpty(t, resp.Output)
	var parsed mcptool.TeamInfoOutput
	require.NoError(t, json.Unmarshal(resp.Output, &parsed))
	require.Len(t, parsed.Teams, 1)
	assert.Equal(t, teamAutomation, parsed.Teams[0].Id)
	assert.Contains(t, strings.Join(parsed.PluginAnnotations, " "), "removed")
}

func TestHooks_After_GetTeamInfo_AllCandidatesRejected(t *testing.T) {
	store := &mockFlowStore{flows: map[string]*model.Flow{"flow1": guardrailFlow()}}
	api := &plugintest.API{}
	r := testRouter(t, store, api)

	otherTeam := "uuuuuuuuuuuuuuuuuuuuuuuuuu"
	out := mcptool.TeamInfoOutput{
		Teams: []*mmmodel.Team{
			{Id: otherTeam, DisplayName: "Other"},
		},
	}
	raw, err := json.Marshal(out)
	require.NoError(t, err)

	code, resp := postAfter(t, r, "flow1", "ai1", mcptool.AfterHookRequest{
		ToolName: "get_team_info",
		Output:   raw,
	})
	require.Equal(t, http.StatusOK, code)
	assert.NotEmpty(t, resp.Error)
	assert.Empty(t, resp.Output)
}

// multiTeamGuardrailFlow returns a flow whose guardrail spans two teams.
func multiTeamGuardrailFlow(chA, teamA, chB, teamB string) *model.Flow {
	return &model.Flow{
		ID: "flow1",
		Trigger: model.Trigger{
			MessagePosted: &model.MessagePostedConfig{ChannelID: chA},
		},
		Actions: []model.Action{
			{
				ID: "ai1",
				AIPrompt: &model.AIPromptActionConfig{
					AllowedTools: []string{"get_team_info", "get_user_channels", "get_channel_info"},
					Guardrails: &model.Guardrails{Channels: []model.GuardrailChannel{
						{ChannelID: chA, TeamID: teamA},
						{ChannelID: chB, TeamID: teamB},
					}},
				},
			},
		},
	}
}

func TestHooks_After_GetTeamInfo_MultiTeamKeepsBoth(t *testing.T) {
	const (
		chA   = "aaaaaaaaaaaaaaaaaaaaaaaaaa"
		chB   = "bbbbbbbbbbbbbbbbbbbbbbbbbb"
		teamA = "tttttttttttttttttttttttttt"
		teamB = "ssssssssssssssssssssssssss"
		teamC = "uuuuuuuuuuuuuuuuuuuuuuuuuu"
	)
	store := &mockFlowStore{flows: map[string]*model.Flow{
		"flow1": multiTeamGuardrailFlow(chA, teamA, chB, teamB),
	}}
	api := &plugintest.API{}
	api.On("GetChannel", chA).Return(&mmmodel.Channel{Id: chA, TeamId: teamA}, (*mmmodel.AppError)(nil)).Maybe()
	r := testRouter(t, store, api)

	out := mcptool.TeamInfoOutput{
		Teams: []*mmmodel.Team{
			{Id: teamA},
			{Id: teamB},
			{Id: teamC},
		},
	}
	raw, err := json.Marshal(out)
	require.NoError(t, err)

	code, resp := postAfter(t, r, "flow1", "ai1", mcptool.AfterHookRequest{
		ToolName: "get_team_info",
		Output:   raw,
	})
	require.Equal(t, http.StatusOK, code)
	require.Empty(t, resp.Error)
	var parsed mcptool.TeamInfoOutput
	require.NoError(t, json.Unmarshal(resp.Output, &parsed))
	require.Len(t, parsed.Teams, 2)
	gotIDs := []string{parsed.Teams[0].Id, parsed.Teams[1].Id}
	assert.ElementsMatch(t, []string{teamA, teamB}, gotIDs)
}

func TestHooks_After_GetUserChannels_PrunesTeamInfoByID(t *testing.T) {
	const (
		chA   = "aaaaaaaaaaaaaaaaaaaaaaaaaa"
		chB   = "bbbbbbbbbbbbbbbbbbbbbbbbbb"
		chC   = "cccccccccccccccccccccccccc"
		teamA = "tttttttttttttttttttttttttt"
		teamB = "ssssssssssssssssssssssssss"
		teamC = "uuuuuuuuuuuuuuuuuuuuuuuuuu"
	)
	store := &mockFlowStore{flows: map[string]*model.Flow{
		"flow1": multiTeamGuardrailFlow(chA, teamA, chB, teamB),
	}}
	api := &plugintest.API{}
	api.On("GetChannel", chA).Return(&mmmodel.Channel{Id: chA, TeamId: teamA}, (*mmmodel.AppError)(nil)).Maybe()
	r := testRouter(t, store, api)

	out := mcptool.UserChannelsOutput{
		Channels: []*mmmodel.Channel{
			{Id: chA, TeamId: teamA},
			{Id: chB, TeamId: teamB},
			{Id: chC, TeamId: teamC},
		},
		TeamInfoByID: map[string]*mmmodel.Team{
			teamA: {Id: teamA},
			teamB: {Id: teamB},
			teamC: {Id: teamC},
		},
	}
	raw, err := json.Marshal(out)
	require.NoError(t, err)

	code, resp := postAfter(t, r, "flow1", "ai1", mcptool.AfterHookRequest{
		ToolName: "get_user_channels",
		Output:   raw,
	})
	require.Equal(t, http.StatusOK, code)
	require.Empty(t, resp.Error)
	var parsed mcptool.UserChannelsOutput
	require.NoError(t, json.Unmarshal(resp.Output, &parsed))
	require.Len(t, parsed.Channels, 2)
	require.Len(t, parsed.TeamInfoByID, 2)
	_, hasA := parsed.TeamInfoByID[teamA]
	_, hasB := parsed.TeamInfoByID[teamB]
	_, hasC := parsed.TeamInfoByID[teamC]
	assert.True(t, hasA)
	assert.True(t, hasB)
	assert.False(t, hasC, "team C had no surviving channel and should be pruned")
}

func TestHooks_After_GetChannelInfo_PrunesAllTeamKeyedMaps(t *testing.T) {
	const (
		chA   = "aaaaaaaaaaaaaaaaaaaaaaaaaa"
		chB   = "bbbbbbbbbbbbbbbbbbbbbbbbbb"
		chC   = "cccccccccccccccccccccccccc"
		teamA = "tttttttttttttttttttttttttt"
		teamB = "ssssssssssssssssssssssssss"
		teamC = "uuuuuuuuuuuuuuuuuuuuuuuuuu"
	)
	store := &mockFlowStore{flows: map[string]*model.Flow{
		"flow1": multiTeamGuardrailFlow(chA, teamA, chB, teamB),
	}}
	api := &plugintest.API{}
	api.On("GetChannel", chA).Return(&mmmodel.Channel{Id: chA, TeamId: teamA}, (*mmmodel.AppError)(nil)).Maybe()
	r := testRouter(t, store, api)

	out := mcptool.ChannelInfoOutput{
		Channels: []*mmmodel.Channel{
			{Id: chA, TeamId: teamA},
			{Id: chB, TeamId: teamB},
			{Id: chC, TeamId: teamC},
		},
		TeamByID: map[string]*mmmodel.Team{
			teamA: {Id: teamA},
			teamB: {Id: teamB},
			teamC: {Id: teamC},
		},
		MemberCountByChannelID: map[string]int64{
			chA: 1,
			chB: 2,
			chC: 3,
		},
		ChannelRoleByID: map[string]string{
			chA: "channel_user",
			chB: "channel_admin",
			chC: "channel_user",
		},
	}
	raw, err := json.Marshal(out)
	require.NoError(t, err)

	code, resp := postAfter(t, r, "flow1", "ai1", mcptool.AfterHookRequest{
		ToolName: "get_channel_info",
		Output:   raw,
	})
	require.Equal(t, http.StatusOK, code)
	require.Empty(t, resp.Error)
	var parsed mcptool.ChannelInfoOutput
	require.NoError(t, json.Unmarshal(resp.Output, &parsed))
	require.Len(t, parsed.Channels, 2)
	assert.Len(t, parsed.TeamByID, 2)
	_, hasC := parsed.TeamByID[teamC]
	assert.False(t, hasC)
	assert.Len(t, parsed.MemberCountByChannelID, 2)
	_, hasChC := parsed.MemberCountByChannelID[chC]
	assert.False(t, hasChC)
	assert.Len(t, parsed.ChannelRoleByID, 2)
	_, hasRoleC := parsed.ChannelRoleByID[chC]
	assert.False(t, hasRoleC)
}

func TestHooks_After_ReadPost_RejectsMixedChannels(t *testing.T) {
	store := &mockFlowStore{flows: map[string]*model.Flow{"flow1": guardrailFlow()}}
	api := &plugintest.API{}
	r := testRouter(t, store, api)

	out := mcptool.ReadPostOutput{
		Posts: []*mmmodel.Post{
			{Id: strings.Repeat("p", 26), ChannelId: chAllow},
			{Id: strings.Repeat("q", 26), ChannelId: chDeny},
		},
	}
	raw, err := json.Marshal(out)
	require.NoError(t, err)

	code, resp := postAfter(t, r, "flow1", "ai1", mcptool.AfterHookRequest{
		ToolName: "read_post",
		Output:   raw,
	})
	require.Equal(t, http.StatusOK, code)
	assert.Contains(t, resp.Error, "spans multiple channels")
}
