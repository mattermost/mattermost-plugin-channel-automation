package flow

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/mattermost/mattermost-plugin-agents/public/bridgeclient"
	mmmodel "github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin/plugintest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

// testConfig is a simple Configuration implementation for tests.
type testConfig struct {
	maxFlowsPerChannel int
}

func (c *testConfig) MaxFlowsPerChannel() int {
	return c.maxFlowsPerChannel
}

// expectLogCalls registers permissive LogError and LogWarn expectations that
// accept any number of arguments. This covers enriched log calls that include
// user_id, flow_id, and other context fields.
func expectLogCalls(api *plugintest.API) {
	// LogError with 3, 5, 7, or 9 args (msg + 1-4 key-value pairs).
	api.On("LogError", mock.Anything, mock.Anything, mock.Anything).Maybe()
	api.On("LogError", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Maybe()
	api.On("LogError", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Maybe()
	api.On("LogError", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Maybe()
	// LogWarn with 3, 5, 7, or 9 args.
	api.On("LogWarn", mock.Anything, mock.Anything, mock.Anything).Maybe()
	api.On("LogWarn", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Maybe()
	api.On("LogWarn", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Maybe()
	api.On("LogWarn", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Maybe()
}

func setupAPI(t *testing.T) (*mux.Router, model.Store, *plugintest.API) {
	t.Helper()

	store, _ := setupStore(t)

	api := &plugintest.API{}
	expectLogCalls(api)
	api.On("HasPermissionTo", mock.Anything, mmmodel.PermissionManageSystem).Return(false).Maybe()
	api.On("GetChannelMember", mock.Anything, mock.Anything).Return(
		&mmmodel.ChannelMember{SchemeAdmin: true}, nil,
	).Maybe()

	handler := NewAPIHandler(store, nil, api, newTestRegistry(), nil, nil, nil)
	router := mux.NewRouter()
	handler.RegisterRoutes(router)

	return router, store, api
}

func TestAPI_CreateFlow(t *testing.T) {
	router, store, _ := setupAPI(t)

	body := `{
		"name": "Test Flow",
		"enabled": true,
		"trigger": {"message_posted": {"channel_id": "ch1"}},
		"actions": [{"id": "send-message", "send_message": {"channel_id": "ch1", "body": "hello"}}]
	}`

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/flows", bytes.NewBufferString(body))
	r.Header.Set("Mattermost-User-ID", "user1")

	router.ServeHTTP(w, r)

	require.Equal(t, http.StatusCreated, w.Code)

	var created model.Flow
	require.NoError(t, json.NewDecoder(w.Body).Decode(&created))
	assert.NotEmpty(t, created.ID)
	assert.Equal(t, "Test Flow", created.Name)
	assert.True(t, created.Enabled)
	assert.Equal(t, "user1", created.CreatedBy)
	assert.NotZero(t, created.CreatedAt)
	assert.Equal(t, created.CreatedAt, created.UpdatedAt)
	require.Len(t, created.Actions, 1)
	assert.Equal(t, "send-message", created.Actions[0].ID)
	require.NotNil(t, created.Actions[0].SendMessage)
	assert.Equal(t, "hello", created.Actions[0].SendMessage.Body)

	// Verify it was persisted.
	got, err := store.Get(created.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, created.Name, got.Name)
}

func TestAPI_CreateFlow_InvalidBody(t *testing.T) {
	router, _, _ := setupAPI(t)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/flows", bytes.NewBufferString("not json"))
	r.Header.Set("Mattermost-User-ID", "user1")

	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAPI_CreateFlow_InvalidActionID(t *testing.T) {
	router, _, _ := setupAPI(t)

	body := `{
		"name": "Test Flow",
		"enabled": true,
		"trigger": {"message_posted": {"channel_id": "ch1"}},
		"actions": [{"id": "BAD ID", "send_message": {"channel_id": "ch2", "body": "hello"}}]
	}`

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/flows", bytes.NewBufferString(body))
	r.Header.Set("Mattermost-User-ID", "user1")

	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "invalid")
}

func TestAPI_CreateFlow_MissingActionID(t *testing.T) {
	router, _, _ := setupAPI(t)

	body := `{
		"name": "Test Flow",
		"enabled": true,
		"trigger": {"message_posted": {"channel_id": "ch1"}},
		"actions": [{"send_message": {"channel_id": "ch2", "body": "hello"}}]
	}`

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/flows", bytes.NewBufferString(body))
	r.Header.Set("Mattermost-User-ID", "user1")

	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "id is required")
}

func TestAPI_CreateFlow_DuplicateActionIDs(t *testing.T) {
	router, _, _ := setupAPI(t)

	body := `{
		"name": "Test Flow",
		"enabled": true,
		"trigger": {"message_posted": {"channel_id": "ch1"}},
		"actions": [
			{"id": "send-msg", "send_message": {"channel_id": "ch2", "body": "hello"}},
			{"id": "send-msg", "send_message": {"channel_id": "ch3", "body": "world"}}
		]
	}`

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/flows", bytes.NewBufferString(body))
	r.Header.Set("Mattermost-User-ID", "user1")

	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "duplicate")
}

func TestAPI_GetFlow(t *testing.T) {
	router, store, _ := setupAPI(t)

	require.NoError(t, store.Save(&model.Flow{
		ID:      "f1",
		Name:    "Flow 1",
		Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
	}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/flows/f1", nil)
	r.Header.Set("Mattermost-User-ID", "user1")

	router.ServeHTTP(w, r)

	require.Equal(t, http.StatusOK, w.Code)

	var got model.Flow
	require.NoError(t, json.NewDecoder(w.Body).Decode(&got))
	assert.Equal(t, "f1", got.ID)
	assert.Equal(t, "Flow 1", got.Name)
}

func TestAPI_GetFlow_NotFound(t *testing.T) {
	router, _, _ := setupAPI(t)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/flows/nonexistent", nil)
	r.Header.Set("Mattermost-User-ID", "user1")

	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestAPI_ListFlows(t *testing.T) {
	router, store, _ := setupAPI(t)

	require.NoError(t, store.Save(&model.Flow{ID: "f1", Name: "Flow 1", Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}}}))
	require.NoError(t, store.Save(&model.Flow{ID: "f2", Name: "Flow 2", Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch2"}}}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/flows", nil)
	r.Header.Set("Mattermost-User-ID", "user1")

	router.ServeHTTP(w, r)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var flows []*model.Flow
	require.NoError(t, json.NewDecoder(w.Body).Decode(&flows))
	assert.Len(t, flows, 2)
}

func TestAPI_ListFlows_Empty(t *testing.T) {
	router, _, _ := setupAPI(t)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/flows", nil)
	r.Header.Set("Mattermost-User-ID", "user1")

	router.ServeHTTP(w, r)

	require.Equal(t, http.StatusOK, w.Code)

	var flows []*model.Flow
	require.NoError(t, json.NewDecoder(w.Body).Decode(&flows))
	assert.Empty(t, flows)
}

func TestAPI_UpdateFlow(t *testing.T) {
	router, store, _ := setupAPI(t)

	original := &model.Flow{
		ID:        "f1",
		Name:      "Original",
		CreatedAt: 1000,
		CreatedBy: "original-user",
		Trigger:   model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
	}
	require.NoError(t, store.Save(original))

	body := `{
		"name": "Updated",
		"enabled": true,
		"trigger": {"message_posted": {"channel_id": "ch2"}},
		"actions": [{"id": "new-action", "send_message": {"channel_id": "ch2", "body": "updated"}}]
	}`

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPut, "/flows/f1", bytes.NewBufferString(body))
	r.Header.Set("Mattermost-User-ID", "original-user")

	router.ServeHTTP(w, r)

	require.Equal(t, http.StatusOK, w.Code)

	var updated model.Flow
	require.NoError(t, json.NewDecoder(w.Body).Decode(&updated))
	assert.Equal(t, "f1", updated.ID)
	assert.Equal(t, "Updated", updated.Name)
	assert.True(t, updated.Enabled)
	// Immutable fields preserved.
	assert.Equal(t, int64(1000), updated.CreatedAt)
	assert.Equal(t, "original-user", updated.CreatedBy)
	assert.Greater(t, updated.UpdatedAt, updated.CreatedAt)
	require.Len(t, updated.Actions, 1)
	assert.Equal(t, "new-action", updated.Actions[0].ID)
}

func TestAPI_UpdateFlow_NotFound(t *testing.T) {
	router, _, _ := setupAPI(t)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPut, "/flows/nonexistent", bytes.NewBufferString(`{"name":"x"}`))
	r.Header.Set("Mattermost-User-ID", "user1")

	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestAPI_UpdateFlow_InvalidBody(t *testing.T) {
	router, store, _ := setupAPI(t)

	require.NoError(t, store.Save(&model.Flow{ID: "f1", Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}}}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPut, "/flows/f1", bytes.NewBufferString("not json"))
	r.Header.Set("Mattermost-User-ID", "user1")

	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAPI_CreateFlow_ScheduleTrigger_MissingInterval(t *testing.T) {
	router, _, _ := setupAPI(t)

	body := `{
		"name": "Schedule Flow",
		"enabled": true,
		"trigger": {"schedule": {"channel_id": "ch1"}},
		"actions": [{"id": "send-message", "send_message": {"channel_id": "ch2", "body": "hello"}}]
	}`

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/flows", bytes.NewBufferString(body))
	r.Header.Set("Mattermost-User-ID", "user1")

	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "interval")
}

func TestAPI_CreateFlow_ScheduleTrigger_IntervalTooSmall(t *testing.T) {
	router, _, _ := setupAPI(t)

	body := `{
		"name": "Schedule Flow",
		"enabled": true,
		"trigger": {"schedule": {"channel_id": "ch1", "interval": "30m"}},
		"actions": [{"id": "send-message", "send_message": {"channel_id": "ch2", "body": "hello"}}]
	}`

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/flows", bytes.NewBufferString(body))
	r.Header.Set("Mattermost-User-ID", "user1")

	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "at least 1h")
}

func TestAPI_CreateFlow_UnknownTriggerType(t *testing.T) {
	router, _, _ := setupAPI(t)

	body := `{
		"name": "Bad Trigger",
		"enabled": true,
		"trigger": {},
		"actions": [{"id": "send-message", "send_message": {"channel_id": "ch2", "body": "hello"}}]
	}`

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/flows", bytes.NewBufferString(body))
	r.Header.Set("Mattermost-User-ID", "user1")

	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAPI_UpdateFlow_ScheduleValidation(t *testing.T) {
	router, store, _ := setupAPI(t)

	require.NoError(t, store.Save(&model.Flow{
		ID:        "f1",
		CreatedBy: "user1",
		Trigger:   model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
	}))

	body := `{
		"name": "Updated",
		"enabled": true,
		"trigger": {"schedule": {"channel_id": "ch1", "interval": "30m"}},
		"actions": [{"id": "a", "send_message": {"channel_id": "ch1", "body": "hi"}}]
	}`

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPut, "/flows/f1", bytes.NewBufferString(body))
	r.Header.Set("Mattermost-User-ID", "user1")

	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "at least 1h")
}

func TestAPI_UpdateFlow_UnchangedPastStartAt(t *testing.T) {
	router, store, _ := setupAPI(t)

	pastStartAt := model.TimeToTimestamp(time.Now().Add(-1 * time.Hour))
	require.NoError(t, store.Save(&model.Flow{
		ID:        "f1",
		CreatedBy: "user1",
		Trigger:   model.Trigger{Schedule: &model.ScheduleConfig{ChannelID: "ch1", Interval: "2h", StartAt: pastStartAt}},
	}))

	body := fmt.Sprintf(`{
		"name": "Updated name",
		"enabled": true,
		"trigger": {"schedule": {"channel_id": "ch1", "interval": "2h", "start_at": %d}},
		"actions": [{"id": "a", "send_message": {"channel_id": "ch1", "body": "hi"}}]
	}`, pastStartAt)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPut, "/flows/f1", bytes.NewBufferString(body))
	r.Header.Set("Mattermost-User-ID", "user1")

	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAPI_DeleteFlow(t *testing.T) {
	router, store, _ := setupAPI(t)

	require.NoError(t, store.Save(&model.Flow{ID: "f1", CreatedBy: "user1", Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}}}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodDelete, "/flows/f1", nil)
	r.Header.Set("Mattermost-User-ID", "user1")

	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusNoContent, w.Code)

	// Verify deleted.
	got, err := store.Get("f1")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestAPI_DeleteFlow_Unauthorized(t *testing.T) {
	router, store, _ := setupAPI(t)

	require.NoError(t, store.Save(&model.Flow{ID: "f1", Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}}}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodDelete, "/flows/f1", nil)
	// Deliberately omit Mattermost-User-ID header.

	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAPI_UpdateFlow_Unauthorized(t *testing.T) {
	router, store, _ := setupAPI(t)

	require.NoError(t, store.Save(&model.Flow{ID: "f1", Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}}}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPut, "/flows/f1", bytes.NewBufferString(`{"name":"x","trigger":{"message_posted":{"channel_id":"ch1"}},"actions":[{"id":"a","send_message":{"channel_id":"ch1","body":"hi"}}]}`))
	// Deliberately omit Mattermost-User-ID header.

	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// setupAPIWithCustomMock creates an API handler with a custom plugintest.API
// so callers can set their own GetChannelMember expectations.
func setupAPIWithCustomMock(t *testing.T, api *plugintest.API) (*mux.Router, model.Store) {
	t.Helper()

	store, _ := setupStore(t)

	handler := NewAPIHandler(store, nil, api, newTestRegistry(), nil, nil, nil)
	router := mux.NewRouter()
	handler.RegisterRoutes(router)

	return router, store
}

// setupAPIWithLimit creates an API handler with a per-channel flow limit.
func setupAPIWithLimit(t *testing.T, limit int) (*mux.Router, model.Store) {
	t.Helper()

	store, _ := setupStore(t)

	api := &plugintest.API{}
	expectLogCalls(api)
	api.On("HasPermissionTo", mock.Anything, mmmodel.PermissionManageSystem).Return(false).Maybe()
	api.On("GetChannelMember", mock.Anything, mock.Anything).Return(
		&mmmodel.ChannelMember{SchemeAdmin: true}, nil,
	).Maybe()

	handler := NewAPIHandler(store, nil, api, newTestRegistry(), nil, &testConfig{maxFlowsPerChannel: limit}, nil)
	router := mux.NewRouter()
	handler.RegisterRoutes(router)

	return router, store
}

// stubAgentToolsLister implements hooks.AgentToolsLister for API tests. It
// returns a fixed tools/err pair and records every (agent, user) pair the
// validator queried so tests can assert dedup and "skipped entirely" cases.
type stubAgentToolsLister struct {
	tools []bridgeclient.BridgeToolInfo
	err   error
	calls []stubBridgeCall
}

type stubBridgeCall struct {
	agentID string
	userID  string
}

func (s *stubAgentToolsLister) GetAgentTools(agentID, userID string) ([]bridgeclient.BridgeToolInfo, error) {
	s.calls = append(s.calls, stubBridgeCall{agentID: agentID, userID: userID})
	return s.tools, s.err
}

// setupAPIWithBridge creates an API handler with a custom plugintest.API and
// a stub bridge so tests can exercise save-time agent access verification.
func setupAPIWithBridge(t *testing.T, api *plugintest.API, bridge *stubAgentToolsLister) (*mux.Router, model.Store) {
	t.Helper()

	store, _ := setupStore(t)

	handler := NewAPIHandler(store, nil, api, newTestRegistry(), nil, nil, bridge)
	router := mux.NewRouter()
	handler.RegisterRoutes(router)

	return router, store
}

func TestAPI_CreateFlow_PermissionDenied(t *testing.T) {
	api := &plugintest.API{}
	expectLogCalls(api)
	api.On("HasPermissionTo", "user1", mmmodel.PermissionManageSystem).Return(false)
	api.On("GetChannelMember", "ch1", "user1").Return(
		&mmmodel.ChannelMember{SchemeAdmin: false}, nil,
	)
	api.On("GetChannel", "ch1").Return(
		&mmmodel.Channel{Id: "ch1", Type: mmmodel.ChannelTypeOpen}, nil,
	)

	router, _ := setupAPIWithCustomMock(t, api)

	body := `{
		"name": "Test Flow",
		"enabled": true,
		"trigger": {"message_posted": {"channel_id": "ch1"}},
		"actions": [{"id": "send-message", "send_message": {"channel_id": "ch1", "body": "hello"}}]
	}`

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/flows", bytes.NewBufferString(body))
	r.Header.Set("Mattermost-User-ID", "user1")

	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "channel admin permissions")
}

func TestAPI_CreateFlow_ActionPermissionDenied(t *testing.T) {
	api := &plugintest.API{}
	expectLogCalls(api)
	api.On("HasPermissionTo", "user1", mmmodel.PermissionManageSystem).Return(false)
	api.On("GetChannelMember", "ch1", "user1").Return(
		&mmmodel.ChannelMember{SchemeAdmin: false}, nil,
	)
	api.On("GetChannel", "ch1").Return(
		&mmmodel.Channel{Id: "ch1", Type: mmmodel.ChannelTypeOpen}, nil,
	)

	router, _ := setupAPIWithCustomMock(t, api)

	body := `{
		"name": "Test Flow",
		"enabled": true,
		"trigger": {"message_posted": {"channel_id": "ch1"}},
		"actions": [{"id": "send-message", "send_message": {"channel_id": "ch1", "body": "hello"}}]
	}`

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/flows", bytes.NewBufferString(body))
	r.Header.Set("Mattermost-User-ID", "user1")

	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "channel admin permissions")
}

func TestAPI_CreateFlow_NotChannelMember(t *testing.T) {
	api := &plugintest.API{}
	expectLogCalls(api)
	api.On("HasPermissionTo", "user1", mmmodel.PermissionManageSystem).Return(false)
	api.On("GetChannelMember", "ch1", "user1").Return(
		nil, mmmodel.NewAppError("GetChannelMember", "not_found", nil, "", http.StatusNotFound),
	)

	router, _ := setupAPIWithCustomMock(t, api)

	body := `{
		"name": "Test Flow",
		"enabled": true,
		"trigger": {"message_posted": {"channel_id": "ch1"}},
		"actions": [{"id": "send-message", "send_message": {"channel_id": "ch1", "body": "hello"}}]
	}`

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/flows", bytes.NewBufferString(body))
	r.Header.Set("Mattermost-User-ID", "user1")

	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "channel admin permissions")
}

func TestAPI_UpdateFlow_PermissionDenied(t *testing.T) {
	api := &plugintest.API{}
	expectLogCalls(api)
	api.On("HasPermissionTo", "user1", mmmodel.PermissionManageSystem).Return(false)
	// Allow existing flow's channel.
	api.On("GetChannelMember", "ch1", "user1").Return(
		&mmmodel.ChannelMember{SchemeAdmin: true}, nil,
	)
	// Deny new flow's channel.
	api.On("GetChannelMember", "ch-new", "user1").Return(
		&mmmodel.ChannelMember{SchemeAdmin: false}, nil,
	)
	api.On("GetChannel", "ch-new").Return(
		&mmmodel.Channel{Id: "ch-new", Type: mmmodel.ChannelTypeOpen}, nil,
	)

	router, store := setupAPIWithCustomMock(t, api)

	require.NoError(t, store.Save(&model.Flow{
		ID:        "f1",
		Name:      "Original",
		CreatedBy: "user1",
		Trigger:   model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
	}))

	body := `{
		"name": "Updated",
		"enabled": true,
		"trigger": {"message_posted": {"channel_id": "ch-new"}},
		"actions": [{"id": "send-msg", "send_message": {"channel_id": "ch-new", "body": "hi"}}]
	}`

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPut, "/flows/f1", bytes.NewBufferString(body))
	r.Header.Set("Mattermost-User-ID", "user1")

	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "channel admin permissions")
}

func TestAPI_UpdateFlow_NonCreatorRejected(t *testing.T) {
	api := &plugintest.API{}
	expectLogCalls(api)
	api.On("HasPermissionTo", "user2", mmmodel.PermissionManageSystem).Return(false)

	router, store := setupAPIWithCustomMock(t, api)

	require.NoError(t, store.Save(&model.Flow{
		ID:        "f1",
		Name:      "Original",
		CreatedBy: "user1",
		Trigger:   model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
	}))

	body := `{
		"name": "Updated",
		"enabled": true,
		"trigger": {"message_posted": {"channel_id": "ch1"}},
		"actions": [{"id": "send-msg", "send_message": {"channel_id": "ch1", "body": "hi"}}]
	}`

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPut, "/flows/f1", bytes.NewBufferString(body))
	r.Header.Set("Mattermost-User-ID", "user2")

	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "automation creator or a system admin")
	api.AssertNotCalled(t, "GetChannelMember", mock.Anything, mock.Anything)
}

func TestAPI_CreateFlow_SystemAdminBypass(t *testing.T) {
	api := &plugintest.API{}
	expectLogCalls(api)
	api.On("HasPermissionTo", "admin1", mmmodel.PermissionManageSystem).Return(true)
	// No GetChannelMember expectation — system admin should skip channel checks.

	router, _ := setupAPIWithCustomMock(t, api)

	body := `{
		"name": "Admin Flow",
		"enabled": true,
		"trigger": {"message_posted": {"channel_id": "ch1"}},
		"actions": [{"id": "send-message", "send_message": {"channel_id": "ch1", "body": "hello"}}]
	}`

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/flows", bytes.NewBufferString(body))
	r.Header.Set("Mattermost-User-ID", "admin1")

	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusCreated, w.Code)

	// Verify GetChannelMember was never called.
	api.AssertNotCalled(t, "GetChannelMember", mock.Anything, mock.Anything)
}

func TestAPI_UpdateFlow_SystemAdminBypass(t *testing.T) {
	api := &plugintest.API{}
	expectLogCalls(api)
	api.On("HasPermissionTo", "admin1", mmmodel.PermissionManageSystem).Return(true)
	// The flow's creator is also a sysadmin so the creator-anchored config
	// validity check skips channel admin lookups too.
	api.On("HasPermissionTo", "creator1", mmmodel.PermissionManageSystem).Return(true)

	router, store := setupAPIWithCustomMock(t, api)

	require.NoError(t, store.Save(&model.Flow{
		ID:        "f1",
		Name:      "Original",
		CreatedBy: "creator1",
		Trigger:   model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
	}))

	body := `{
		"name": "Updated by admin",
		"enabled": true,
		"trigger": {"message_posted": {"channel_id": "ch-new"}},
		"actions": [{"id": "send-message", "send_message": {"channel_id": "ch-new", "body": "hello"}}]
	}`

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPut, "/flows/f1", bytes.NewBufferString(body))
	r.Header.Set("Mattermost-User-ID", "admin1")

	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusOK, w.Code)

	api.AssertNotCalled(t, "GetChannelMember", mock.Anything, mock.Anything)
}

func TestAPI_CreateFlow_TemplatedChannelSkipped(t *testing.T) {
	api := &plugintest.API{}
	expectLogCalls(api)
	api.On("HasPermissionTo", "user1", mmmodel.PermissionManageSystem).Return(false)
	// Only the trigger channel should be checked; the templated action channel is skipped.
	api.On("GetChannelMember", "ch1", "user1").Return(
		&mmmodel.ChannelMember{SchemeAdmin: true}, nil,
	)

	router, _ := setupAPIWithCustomMock(t, api)

	body := `{
		"name": "Test Flow",
		"enabled": true,
		"trigger": {"message_posted": {"channel_id": "ch1"}},
		"actions": [{"id": "reply-echo", "send_message": {"channel_id": "{{.Trigger.Channel.Id}}", "body": "echo"}}]
	}`

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/flows", bytes.NewBufferString(body))
	r.Header.Set("Mattermost-User-ID", "user1")

	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusCreated, w.Code)

	// Verify GetChannelMember was only called for ch1.
	api.AssertNumberOfCalls(t, "GetChannelMember", 1)
}

func TestAPI_CreateFlow_ChannelCreated_NonTeamAdminDenied(t *testing.T) {
	api := &plugintest.API{}
	expectLogCalls(api)
	api.On("HasPermissionTo", "user1", mmmodel.PermissionManageSystem).Return(false)
	api.On("GetTeam", "team1").Return(&mmmodel.Team{Id: "team1"}, nil)
	api.On("HasPermissionToTeam", "user1", "team1", mmmodel.PermissionManageTeam).Return(false)

	router, _ := setupAPIWithCustomMock(t, api)

	body := `{
		"name": "Team Flow",
		"enabled": true,
		"trigger": {"channel_created": {"team_id": "team1"}},
		"actions": [{"id": "announce", "send_message": {"channel_id": "{{.Trigger.Channel.Id}}", "body": "hello"}}]
	}`

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/flows", bytes.NewBufferString(body))
	r.Header.Set("Mattermost-User-ID", "user1")

	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "team admin")
}

func TestAPI_CreateFlow_ChannelCreated_AIPromptOnly_NonTeamAdminDenied(t *testing.T) {
	api := &plugintest.API{}
	expectLogCalls(api)
	api.On("HasPermissionTo", "user1", mmmodel.PermissionManageSystem).Return(false)
	api.On("GetTeam", "team1").Return(&mmmodel.Team{Id: "team1"}, nil)
	api.On("HasPermissionToTeam", "user1", "team1", mmmodel.PermissionManageTeam).Return(false)

	router, _ := setupAPIWithCustomMock(t, api)

	body := `{
		"name": "AI on new channels",
		"enabled": true,
		"trigger": {"channel_created": {"team_id": "team1"}},
		"actions": [{"id": "ai-task", "ai_prompt": {"prompt": "summarize", "provider_type": "agent", "provider_id": "bot1"}}]
	}`

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/flows", bytes.NewBufferString(body))
	r.Header.Set("Mattermost-User-ID", "user1")

	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "team admin")
}

func TestAPI_CreateFlow_ChannelCreated_SystemAdminAllowed(t *testing.T) {
	api := &plugintest.API{}
	expectLogCalls(api)
	api.On("HasPermissionTo", "admin1", mmmodel.PermissionManageSystem).Return(true)

	router, _ := setupAPIWithCustomMock(t, api)

	body := `{
		"name": "Team Flow",
		"enabled": true,
		"trigger": {"channel_created": {"team_id": "team1"}},
		"actions": [{"id": "announce", "send_message": {"channel_id": "{{.Trigger.Channel.Id}}", "body": "hello"}}]
	}`

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/flows", bytes.NewBufferString(body))
	r.Header.Set("Mattermost-User-ID", "admin1")

	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusCreated, w.Code)
}

func TestAPI_CreateFlow_ChannelCreated_TeamAdminAllowed(t *testing.T) {
	api := &plugintest.API{}
	expectLogCalls(api)
	api.On("HasPermissionTo", "user1", mmmodel.PermissionManageSystem).Return(false)
	api.On("GetTeam", "team1").Return(&mmmodel.Team{Id: "team1"}, nil)
	api.On("HasPermissionToTeam", "user1", "team1", mmmodel.PermissionManageTeam).Return(true)

	router, _ := setupAPIWithCustomMock(t, api)

	body := `{
		"name": "Team Flow",
		"enabled": true,
		"trigger": {"channel_created": {"team_id": "team1"}},
		"actions": [{"id": "announce", "send_message": {"channel_id": "{{.Trigger.Channel.Id}}", "body": "hello"}}]
	}`

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/flows", bytes.NewBufferString(body))
	r.Header.Set("Mattermost-User-ID", "user1")

	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusCreated, w.Code)
}

func TestAPI_CreateFlow_ChannelCreated_LiteralChannelRejected(t *testing.T) {
	// With the temporary channel guardrail, channel_created triggers must use
	// the template expression for send_message channel_id.
	router, _, _ := setupAPI(t)

	body := `{
		"name": "Team Flow",
		"enabled": true,
		"trigger": {"channel_created": {"team_id": "team1"}},
		"actions": [{"id": "announce", "send_message": {"channel_id": "ch-other", "body": "hello"}}]
	}`

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/flows", bytes.NewBufferString(body))
	r.Header.Set("Mattermost-User-ID", "user1")

	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "must use a template expression")
}

func TestAPI_ListFlows_ChannelCreated_HiddenFromNonTeamAdmin(t *testing.T) {
	api := &plugintest.API{}
	expectLogCalls(api)
	api.On("HasPermissionTo", "user1", mmmodel.PermissionManageSystem).Return(false)
	api.On("GetChannelMember", "ch1", "user1").Return(
		&mmmodel.ChannelMember{SchemeAdmin: true}, nil,
	)
	api.On("GetTeam", "team1").Return(&mmmodel.Team{Id: "team1"}, nil)
	api.On("HasPermissionToTeam", "user1", "team1", mmmodel.PermissionManageTeam).Return(false)

	router, store := setupAPIWithCustomMock(t, api)

	// A normal flow the user can see.
	require.NoError(t, store.Save(&model.Flow{
		ID:      "f1",
		Name:    "Normal Flow",
		Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
	}))
	// A channel_created flow — should be hidden from non-team-admin.
	require.NoError(t, store.Save(&model.Flow{
		ID:      "f2",
		Name:    "Team Flow",
		Trigger: model.Trigger{ChannelCreated: &model.ChannelCreatedConfig{TeamID: "team1"}},
	}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/flows", nil)
	r.Header.Set("Mattermost-User-ID", "user1")

	router.ServeHTTP(w, r)

	require.Equal(t, http.StatusOK, w.Code)

	var flows []*model.Flow
	require.NoError(t, json.NewDecoder(w.Body).Decode(&flows))
	require.Len(t, flows, 1)
	assert.Equal(t, "f1", flows[0].ID)
}

func TestAPI_ListFlows_FilterByChannel(t *testing.T) {
	router, store, _ := setupAPI(t)

	require.NoError(t, store.Save(&model.Flow{ID: "f1", Name: "Flow 1", Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}}}))
	require.NoError(t, store.Save(&model.Flow{ID: "f2", Name: "Flow 2", Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch2"}}}))
	require.NoError(t, store.Save(&model.Flow{ID: "f3", Name: "Flow 3", Trigger: model.Trigger{Schedule: &model.ScheduleConfig{ChannelID: "ch1", Interval: "1h"}}}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/flows?channel_id=ch1", nil)
	r.Header.Set("Mattermost-User-ID", "user1")

	router.ServeHTTP(w, r)

	require.Equal(t, http.StatusOK, w.Code)

	var flows []*model.Flow
	require.NoError(t, json.NewDecoder(w.Body).Decode(&flows))
	require.Len(t, flows, 2)
	ids := []string{flows[0].ID, flows[1].ID}
	assert.ElementsMatch(t, []string{"f1", "f3"}, ids)
}

func TestAPI_ListFlows_FilterByChannel_NoMatch(t *testing.T) {
	router, store, _ := setupAPI(t)

	require.NoError(t, store.Save(&model.Flow{ID: "f1", Name: "Flow 1", Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}}}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/flows?channel_id=ch-nonexistent", nil)
	r.Header.Set("Mattermost-User-ID", "user1")

	router.ServeHTTP(w, r)

	require.Equal(t, http.StatusOK, w.Code)

	var flows []*model.Flow
	require.NoError(t, json.NewDecoder(w.Body).Decode(&flows))
	assert.Empty(t, flows)
}

func TestAPI_CreateFlow_ChannelLimitReached(t *testing.T) {
	router, store := setupAPIWithLimit(t, 1)

	// Save one flow on ch1 — fills the limit.
	require.NoError(t, store.Save(&model.Flow{
		ID:      "f1",
		Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
	}))

	body := `{
		"name": "Second Flow",
		"enabled": true,
		"trigger": {"message_posted": {"channel_id": "ch1"}},
		"actions": [{"id": "send-msg", "send_message": {"channel_id": "ch1", "body": "hello"}}]
	}`

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/flows", bytes.NewBufferString(body))
	r.Header.Set("Mattermost-User-ID", "user1")

	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Contains(t, w.Body.String(), "maximum")
}

func TestAPI_CreateFlow_DifferentChannelSucceeds(t *testing.T) {
	router, store := setupAPIWithLimit(t, 1)

	// ch1 is full.
	require.NoError(t, store.Save(&model.Flow{
		ID:      "f1",
		Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
	}))

	// Creating on ch2 should succeed.
	body := `{
		"name": "Other Channel Flow",
		"enabled": true,
		"trigger": {"message_posted": {"channel_id": "ch2"}},
		"actions": [{"id": "send-msg", "send_message": {"channel_id": "ch2", "body": "hello"}}]
	}`

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/flows", bytes.NewBufferString(body))
	r.Header.Set("Mattermost-User-ID", "user1")

	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusCreated, w.Code)
}

func TestAPI_UpdateFlow_SameChannelSelfExclusion(t *testing.T) {
	router, store := setupAPIWithLimit(t, 1)

	// ch1 has one flow — at the limit.
	require.NoError(t, store.Save(&model.Flow{
		ID:        "f1",
		Name:      "Original",
		CreatedBy: "user1",
		Trigger:   model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
	}))

	// Updating the same flow on the same channel should succeed (self-exclusion).
	body := `{
		"name": "Updated",
		"enabled": true,
		"trigger": {"message_posted": {"channel_id": "ch1"}},
		"actions": [{"id": "send-msg", "send_message": {"channel_id": "ch1", "body": "updated"}}]
	}`

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPut, "/flows/f1", bytes.NewBufferString(body))
	r.Header.Set("Mattermost-User-ID", "user1")

	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAPI_UpdateFlow_MoveToFullChannel(t *testing.T) {
	router, store := setupAPIWithLimit(t, 1)

	// ch1 has a flow, ch2 has a flow.
	require.NoError(t, store.Save(&model.Flow{
		ID:        "f1",
		CreatedBy: "user1",
		Trigger:   model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
	}))
	require.NoError(t, store.Save(&model.Flow{
		ID:        "f2",
		CreatedBy: "user1",
		Trigger:   model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch2"}},
	}))

	// Moving f1 to ch2 should be blocked (ch2 already at limit).
	body := `{
		"name": "Moved",
		"enabled": true,
		"trigger": {"message_posted": {"channel_id": "ch2"}},
		"actions": [{"id": "send-msg", "send_message": {"channel_id": "ch2", "body": "moved"}}]
	}`

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPut, "/flows/f1", bytes.NewBufferString(body))
	r.Header.Set("Mattermost-User-ID", "user1")

	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Contains(t, w.Body.String(), "maximum")
}

func TestAPI_CreateFlow_UnlimitedAllowsAny(t *testing.T) {
	router, store := setupAPIWithLimit(t, 0)

	// Even with many flows, limit=0 means unlimited.
	require.NoError(t, store.Save(&model.Flow{
		ID:      "f1",
		Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
	}))
	require.NoError(t, store.Save(&model.Flow{
		ID:      "f2",
		Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
	}))

	body := `{
		"name": "Third Flow",
		"enabled": true,
		"trigger": {"message_posted": {"channel_id": "ch1"}},
		"actions": [{"id": "send-msg", "send_message": {"channel_id": "ch1", "body": "hello"}}]
	}`

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/flows", bytes.NewBufferString(body))
	r.Header.Set("Mattermost-User-ID", "user1")

	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusCreated, w.Code)
}

func TestAPI_CreateFlow_ChannelCreatedBypassesLimit(t *testing.T) {
	api := &plugintest.API{}
	expectLogCalls(api)
	api.On("HasPermissionTo", "admin1", mmmodel.PermissionManageSystem).Return(true)

	store, _ := setupStore(t)

	handler := NewAPIHandler(store, nil, api, newTestRegistry(), nil, &testConfig{maxFlowsPerChannel: 1}, nil)
	router := mux.NewRouter()
	handler.RegisterRoutes(router)

	// channel_created flows have no trigger channel, so they bypass the limit.
	body := `{
		"name": "Team Flow",
		"enabled": true,
		"trigger": {"channel_created": {"team_id": "team1"}},
		"actions": [{"id": "announce", "send_message": {"channel_id": "{{.Trigger.Channel.Id}}", "body": "hello"}}]
	}`

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/flows", bytes.NewBufferString(body))
	r.Header.Set("Mattermost-User-ID", "admin1")

	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusCreated, w.Code)
}

func TestAPI_CreateFlow_EmptyName(t *testing.T) {
	router, _, _ := setupAPI(t)

	body := `{
		"name": "",
		"enabled": true,
		"trigger": {"message_posted": {"channel_id": "ch1"}},
		"actions": [{"id": "send-msg", "send_message": {"channel_id": "ch1", "body": "hello"}}]
	}`

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/flows", bytes.NewBufferString(body))
	r.Header.Set("Mattermost-User-ID", "user1")

	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "name is required")
}

func TestAPI_CreateFlow_NameTooLong(t *testing.T) {
	router, _, _ := setupAPI(t)

	longName := strings.Repeat("a", 101)
	body := fmt.Sprintf(`{
		"name": %q,
		"enabled": true,
		"trigger": {"message_posted": {"channel_id": "ch1"}},
		"actions": [{"id": "send-msg", "send_message": {"channel_id": "ch1", "body": "hello"}}]
	}`, longName)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/flows", bytes.NewBufferString(body))
	r.Header.Set("Mattermost-User-ID", "user1")

	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "100 characters")
}

func TestAPI_UpdateFlow_EmptyName(t *testing.T) {
	router, store, _ := setupAPI(t)

	require.NoError(t, store.Save(&model.Flow{
		ID:        "f1",
		Name:      "Original",
		CreatedBy: "user1",
		Trigger:   model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
	}))

	body := `{
		"name": "",
		"enabled": true,
		"trigger": {"message_posted": {"channel_id": "ch1"}},
		"actions": [{"id": "a", "send_message": {"channel_id": "ch1", "body": "hi"}}]
	}`

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPut, "/flows/f1", bytes.NewBufferString(body))
	r.Header.Set("Mattermost-User-ID", "user1")

	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "name is required")
}

func TestAPI_UpdateFlow_NameTooLong(t *testing.T) {
	router, store, _ := setupAPI(t)

	require.NoError(t, store.Save(&model.Flow{
		ID:        "f1",
		Name:      "Original",
		CreatedBy: "user1",
		Trigger:   model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
	}))

	longName := strings.Repeat("a", 101)
	body := fmt.Sprintf(`{
		"name": %q,
		"enabled": true,
		"trigger": {"message_posted": {"channel_id": "ch1"}},
		"actions": [{"id": "a", "send_message": {"channel_id": "ch1", "body": "hi"}}]
	}`, longName)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPut, "/flows/f1", bytes.NewBufferString(body))
	r.Header.Set("Mattermost-User-ID", "user1")

	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "100 characters")
}

func TestAPI_CreateFlow_EmptyActions(t *testing.T) {
	router, _, _ := setupAPI(t)

	body := `{
		"name": "Test Flow",
		"enabled": true,
		"trigger": {"message_posted": {"channel_id": "ch1"}},
		"actions": []
	}`

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/flows", bytes.NewBufferString(body))
	r.Header.Set("Mattermost-User-ID", "user1")

	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "at least one action")
}

func TestAPI_CreateFlow_MultipleTriggerTypes(t *testing.T) {
	router, _, _ := setupAPI(t)

	body := `{
		"name": "Test Flow",
		"enabled": true,
		"trigger": {"message_posted": {"channel_id": "ch1"}, "schedule": {"channel_id": "ch1", "interval": "2h"}},
		"actions": [{"id": "send-msg", "send_message": {"channel_id": "ch1", "body": "hello"}}]
	}`

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/flows", bytes.NewBufferString(body))
	r.Header.Set("Mattermost-User-ID", "user1")

	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "exactly one trigger type")
}

func TestAPI_CreateFlow_UserJoinedTeam_TeamAdminAllowed(t *testing.T) {
	api := &plugintest.API{}
	expectLogCalls(api)
	api.On("HasPermissionTo", "user1", mmmodel.PermissionManageSystem).Return(false)
	api.On("GetTeam", "team1").Return(&mmmodel.Team{Id: "team1"}, nil)
	api.On("HasPermissionToTeam", "user1", "team1", mmmodel.PermissionManageTeam).Return(true)

	router, _ := setupAPIWithCustomMock(t, api)

	body := `{
		"name": "Team Join Flow",
		"enabled": true,
		"trigger": {"user_joined_team": {"team_id": "team1"}},
		"actions": [{"id": "greet", "send_message": {"channel_id": "{{.Trigger.Team.DefaultChannelId}}", "body": "welcome"}}]
	}`

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/flows", bytes.NewBufferString(body))
	r.Header.Set("Mattermost-User-ID", "user1")

	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusCreated, w.Code)
}

func TestAPI_CreateFlow_UserJoinedTeam_NotTeamAdminDenied(t *testing.T) {
	api := &plugintest.API{}
	expectLogCalls(api)
	api.On("HasPermissionTo", "user1", mmmodel.PermissionManageSystem).Return(false)
	api.On("GetTeam", "team1").Return(&mmmodel.Team{Id: "team1"}, nil)
	api.On("HasPermissionToTeam", "user1", "team1", mmmodel.PermissionManageTeam).Return(false)

	router, _ := setupAPIWithCustomMock(t, api)

	body := `{
		"name": "Team Join Flow",
		"enabled": true,
		"trigger": {"user_joined_team": {"team_id": "team1"}},
		"actions": [{"id": "greet", "send_message": {"channel_id": "{{.Trigger.Team.DefaultChannelId}}", "body": "welcome"}}]
	}`

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/flows", bytes.NewBufferString(body))
	r.Header.Set("Mattermost-User-ID", "user1")

	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "team admin")
}

func TestAPI_CreateFlow_UserJoinedTeam_GetTeam500(t *testing.T) {
	api := &plugintest.API{}
	expectLogCalls(api)
	api.On("HasPermissionTo", "user1", mmmodel.PermissionManageSystem).Return(false)
	api.On("GetTeam", "team1").Return(
		nil, &mmmodel.AppError{StatusCode: http.StatusInternalServerError, Message: "db down"},
	)

	router, _ := setupAPIWithCustomMock(t, api)

	body := `{
		"name": "Team Join Flow",
		"enabled": true,
		"trigger": {"user_joined_team": {"team_id": "team1"}},
		"actions": [{"id": "greet", "send_message": {"channel_id": "{{.Trigger.Team.DefaultChannelId}}", "body": "welcome"}}]
	}`

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/flows", bytes.NewBufferString(body))
	r.Header.Set("Mattermost-User-ID", "user1")

	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// adminAPIWithBridge wires a sysadmin actor (so channel/team checks are
// skipped) plus a stub bridge, so the only relevant assertion in the test
// body is the bridge's own behavior.
func adminAPIWithBridge(t *testing.T, userID string, bridge *stubAgentToolsLister) (*mux.Router, model.Store, *plugintest.API) {
	t.Helper()
	api := &plugintest.API{}
	expectLogCalls(api)
	api.On("HasPermissionTo", userID, mmmodel.PermissionManageSystem).Return(true)
	router, store := setupAPIWithBridge(t, api, bridge)
	return router, store, api
}

const aiAgentFlowBody = `{
	"name": "AI Flow",
	"enabled": true,
	"trigger": {"channel_created": {"team_id": "team1"}},
	"actions": [{"id": "ai-task", "ai_prompt": {"prompt": "summarize", "provider_type": "agent", "provider_id": "bot1"}}]
}`

// TestAPI_CreateFlow_AIPromptAgent_NoAllowedTools_ChecksBridge is the core
// regression guard: a flow with provider_type "agent" and empty allowed_tools
// must trigger a bridge call to verify the creator has access to the agent.
func TestAPI_CreateFlow_AIPromptAgent_NoAllowedTools_ChecksBridge(t *testing.T) {
	bridge := &stubAgentToolsLister{tools: []bridgeclient.BridgeToolInfo{}}
	router, _, _ := adminAPIWithBridge(t, "admin1", bridge)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/flows", bytes.NewBufferString(aiAgentFlowBody))
	r.Header.Set("Mattermost-User-ID", "admin1")
	router.ServeHTTP(w, r)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	require.Len(t, bridge.calls, 1)
	assert.Equal(t, "bot1", bridge.calls[0].agentID)
	assert.Equal(t, "admin1", bridge.calls[0].userID)
}

// TestAPI_CreateFlow_AIPromptAgent_AccessDenied surfaces a bridge denial as
// 502 (ErrToolDiscovery -> http.StatusBadGateway in the handler).
func TestAPI_CreateFlow_AIPromptAgent_AccessDenied(t *testing.T) {
	bridge := &stubAgentToolsLister{err: fmt.Errorf("request failed with status 403: permission denied")}
	router, _, _ := adminAPIWithBridge(t, "admin1", bridge)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/flows", bytes.NewBufferString(aiAgentFlowBody))
	r.Header.Set("Mattermost-User-ID", "admin1")
	router.ServeHTTP(w, r)

	require.Equal(t, http.StatusBadGateway, w.Code)
	assert.Contains(t, w.Body.String(), `failed to list tools for agent \"bot1\"`)
	assert.Contains(t, w.Body.String(), "status 403")
}

// TestAPI_CreateFlow_AIPromptAgent_BridgeUnavailable rejects when the bridge
// is nil and the flow has an ai_prompt agent action.
func TestAPI_CreateFlow_AIPromptAgent_BridgeUnavailable(t *testing.T) {
	api := &plugintest.API{}
	expectLogCalls(api)
	api.On("HasPermissionTo", "admin1", mmmodel.PermissionManageSystem).Return(true)

	router, _ := setupAPIWithCustomMock(t, api) // bridge is nil

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/flows", bytes.NewBufferString(aiAgentFlowBody))
	r.Header.Set("Mattermost-User-ID", "admin1")
	router.ServeHTTP(w, r)

	require.Equal(t, http.StatusBadGateway, w.Code)
	assert.Contains(t, w.Body.String(), "bridge client unavailable")
}

// TestAPI_CreateFlow_AIPromptService_SkipsBridge confirms service providers
// do not trigger any bridge call (out of scope for the agent ACL).
func TestAPI_CreateFlow_AIPromptService_SkipsBridge(t *testing.T) {
	bridge := &stubAgentToolsLister{}
	router, _, _ := adminAPIWithBridge(t, "admin1", bridge)

	body := `{
		"name": "Service Flow",
		"enabled": true,
		"trigger": {"channel_created": {"team_id": "team1"}},
		"actions": [{"id": "ai-task", "ai_prompt": {"prompt": "summarize", "provider_type": "service", "provider_id": "openai"}}]
	}`
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/flows", bytes.NewBufferString(body))
	r.Header.Set("Mattermost-User-ID", "admin1")
	router.ServeHTTP(w, r)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	assert.Empty(t, bridge.calls)
}

// TestAPI_CreateFlow_AIPromptAgent_DuplicateID_DedupesCalls is the property
// that prevents a "two requests" regression: two ai_prompt actions sharing
// one agent ID must result in exactly one bridge call.
func TestAPI_CreateFlow_AIPromptAgent_DuplicateID_DedupesCalls(t *testing.T) {
	bridge := &stubAgentToolsLister{tools: []bridgeclient.BridgeToolInfo{
		{Name: "search", ServerOrigin: "external-mcp"},
	}}
	router, _, api := adminAPIWithBridge(t, "admin1", bridge)

	// Trigger is channel_created so allowed_tools requires guardrails: stub
	// the channel-permission checks for the guardrail channel.
	guardrailChannelID := mmmodel.NewId()
	api.On("GetChannel", guardrailChannelID).Return(&mmmodel.Channel{Id: guardrailChannelID, TeamId: "team1"}, (*mmmodel.AppError)(nil))
	api.On("HasPermissionToChannel", "admin1", guardrailChannelID, mmmodel.PermissionReadChannel).Return(true)

	body := fmt.Sprintf(`{
		"name": "AI Flow",
		"enabled": true,
		"trigger": {"channel_created": {"team_id": "team1"}},
		"actions": [
			{"id": "with-tools", "ai_prompt": {"prompt": "p", "provider_type": "agent", "provider_id": "bot1", "allowed_tools": ["search"], "guardrails": {"channel_ids": [%q]}}},
			{"id": "without-tools", "ai_prompt": {"prompt": "p", "provider_type": "agent", "provider_id": "bot1"}}
		]
	}`, guardrailChannelID)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/flows", bytes.NewBufferString(body))
	r.Header.Set("Mattermost-User-ID", "admin1")
	router.ServeHTTP(w, r)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	require.Len(t, bridge.calls, 1, "bridge must be called exactly once for repeated agent ID")
	assert.Equal(t, "bot1", bridge.calls[0].agentID)
}

// TestAPI_CreateFlow_AIPromptAgent_PermissionFailureSkipsBridge verifies the
// permission check still short-circuits before the bridge round-trip.
func TestAPI_CreateFlow_AIPromptAgent_PermissionFailureSkipsBridge(t *testing.T) {
	api := &plugintest.API{}
	expectLogCalls(api)
	api.On("HasPermissionTo", "user1", mmmodel.PermissionManageSystem).Return(false)
	api.On("GetTeam", "team1").Return(&mmmodel.Team{Id: "team1"}, nil)
	api.On("HasPermissionToTeam", "user1", "team1", mmmodel.PermissionManageTeam).Return(false)

	bridge := &stubAgentToolsLister{}
	router, _ := setupAPIWithBridge(t, api, bridge)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/flows", bytes.NewBufferString(aiAgentFlowBody))
	r.Header.Set("Mattermost-User-ID", "user1")
	router.ServeHTTP(w, r)

	require.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "team admin")
	assert.Empty(t, bridge.calls, "bridge must not be called when the user cannot manage the flow")
}

// TestAPI_UpdateFlow_AIPromptAgent_AccessDenied mirrors the create path for
// the PUT handler. The check uses the existing flow's CreatedBy (matching
// the runtime model where the bridge ACL is checked against created_by).
func TestAPI_UpdateFlow_AIPromptAgent_AccessDenied(t *testing.T) {
	bridge := &stubAgentToolsLister{err: fmt.Errorf("request failed with status 403: permission denied")}

	api := &plugintest.API{}
	expectLogCalls(api)
	api.On("HasPermissionTo", "creator1", mmmodel.PermissionManageSystem).Return(true)

	router, store := setupAPIWithBridge(t, api, bridge)

	require.NoError(t, store.Save(&model.Flow{
		ID:        "f1",
		Name:      "Original",
		CreatedBy: "creator1",
		Trigger:   model.Trigger{ChannelCreated: &model.ChannelCreatedConfig{TeamID: "team1"}},
		Actions: []model.Action{{
			ID:       "ai-task",
			AIPrompt: &model.AIPromptActionConfig{Prompt: "p", ProviderType: "agent", ProviderID: "bot1"},
		}},
	}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPut, "/flows/f1", bytes.NewBufferString(aiAgentFlowBody))
	r.Header.Set("Mattermost-User-ID", "creator1")
	router.ServeHTTP(w, r)

	require.Equal(t, http.StatusBadGateway, w.Code)
	assert.Contains(t, w.Body.String(), `failed to list tools for agent \"bot1\"`)
	require.Len(t, bridge.calls, 1)
	assert.Equal(t, "creator1", bridge.calls[0].userID, "PUT must use existing.CreatedBy for the bridge check")
}

// errStore wraps an underlying model.Store and forces SaveWithChannelLimit
// to return a configured error so handler-level error mapping can be tested
// without driving the real KV store into a failure mode.
type errStore struct {
	model.Store
	saveErr error
}

func (e *errStore) SaveWithChannelLimit(_ *model.Flow, _ int, _ string) error {
	return e.saveErr
}

func setupAPIWithStore(t *testing.T, store model.Store, limit int) *mux.Router {
	t.Helper()

	api := &plugintest.API{}
	expectLogCalls(api)
	api.On("HasPermissionTo", mock.Anything, mmmodel.PermissionManageSystem).Return(false).Maybe()
	api.On("GetChannelMember", mock.Anything, mock.Anything).Return(
		&mmmodel.ChannelMember{SchemeAdmin: true}, nil,
	).Maybe()

	handler := NewAPIHandler(store, nil, api, newTestRegistry(), nil, &testConfig{maxFlowsPerChannel: limit}, nil)
	router := mux.NewRouter()
	handler.RegisterRoutes(router)
	return router
}

func TestAPI_CreateFlow_BackendErrorReturns500(t *testing.T) {
	underlying, _ := setupStore(t)
	store := &errStore{Store: underlying, saveErr: errors.New("boom: backend down")}
	router := setupAPIWithStore(t, store, 5)

	body := `{
		"name": "Backend Error Flow",
		"enabled": true,
		"trigger": {"message_posted": {"channel_id": "ch1"}},
		"actions": [{"id": "send-msg", "send_message": {"channel_id": "ch1", "body": "hi"}}]
	}`

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/flows", bytes.NewBufferString(body))
	r.Header.Set("Mattermost-User-ID", "user1")

	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "failed to create flow")
}

func TestAPI_CreateFlow_QuotaSentinelReturns409(t *testing.T) {
	underlying, _ := setupStore(t)
	store := &errStore{Store: underlying, saveErr: model.ErrChannelFlowLimitExceeded}
	router := setupAPIWithStore(t, store, 5)

	body := `{
		"name": "Quota Flow",
		"enabled": true,
		"trigger": {"message_posted": {"channel_id": "ch1"}},
		"actions": [{"id": "send-msg", "send_message": {"channel_id": "ch1", "body": "hi"}}]
	}`

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/flows", bytes.NewBufferString(body))
	r.Header.Set("Mattermost-User-ID", "user1")

	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Contains(t, w.Body.String(), "maximum")
}

func TestAPI_UpdateFlow_BackendErrorReturns500(t *testing.T) {
	underlying, _ := setupStore(t)
	// Seed an existing flow via the underlying store so the handler can
	// load it before invoking SaveWithChannelLimit.
	require.NoError(t, underlying.Save(&model.Flow{
		ID:        "f1",
		Name:      "Original",
		CreatedBy: "user1",
		Trigger:   model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
	}))

	store := &errStore{Store: underlying, saveErr: errors.New("boom: backend down")}
	router := setupAPIWithStore(t, store, 5)

	body := `{
		"name": "Updated",
		"enabled": true,
		"trigger": {"message_posted": {"channel_id": "ch1"}},
		"actions": [{"id": "send-msg", "send_message": {"channel_id": "ch1", "body": "hi"}}]
	}`

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPut, "/flows/f1", bytes.NewBufferString(body))
	r.Header.Set("Mattermost-User-ID", "user1")

	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "failed to update flow")
}

func TestAPI_UpdateFlow_QuotaSentinelReturns409(t *testing.T) {
	underlying, _ := setupStore(t)
	require.NoError(t, underlying.Save(&model.Flow{
		ID:        "f1",
		Name:      "Original",
		CreatedBy: "user1",
		Trigger:   model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
	}))

	store := &errStore{Store: underlying, saveErr: model.ErrChannelFlowLimitExceeded}
	router := setupAPIWithStore(t, store, 5)

	body := `{
		"name": "Updated",
		"enabled": true,
		"trigger": {"message_posted": {"channel_id": "ch1"}},
		"actions": [{"id": "send-msg", "send_message": {"channel_id": "ch1", "body": "hi"}}]
	}`

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPut, "/flows/f1", bytes.NewBufferString(body))
	r.Header.Set("Mattermost-User-ID", "user1")

	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Contains(t, w.Body.String(), "maximum")
}
