package flow

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	mmmodel "github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin/plugintest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

func setupAPI(t *testing.T) (*mux.Router, model.Store, *plugintest.API) {
	t.Helper()

	store, _ := setupStore(t)

	api := &plugintest.API{}
	api.On("LogError", mock.Anything, mock.Anything, mock.Anything).Maybe()
	api.On("HasPermissionTo", mock.Anything, mmmodel.PermissionManageSystem).Return(false).Maybe()
	api.On("GetChannelMember", mock.Anything, mock.Anything).Return(
		&mmmodel.ChannelMember{SchemeAdmin: true}, nil,
	).Maybe()

	handler := NewAPIHandler(store, nil, api, nil)
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
		"actions": [{"id": "send-message", "send_message": {"channel_id": "ch2", "body": "hello"}}]
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
		"actions": [{"id": "new-action", "send_message": {"channel_id": "ch3", "body": "updated"}}]
	}`

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPut, "/flows/f1", bytes.NewBufferString(body))
	r.Header.Set("Mattermost-User-ID", "other-user")

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
		"trigger": {"schedule": {"channel_id": "ch1", "interval": "1m"}},
		"actions": [{"id": "send-message", "send_message": {"channel_id": "ch2", "body": "hello"}}]
	}`

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/flows", bytes.NewBufferString(body))
	r.Header.Set("Mattermost-User-ID", "user1")

	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "at least")
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
		ID:      "f1",
		Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
	}))

	body := `{
		"name": "Updated",
		"enabled": true,
		"trigger": {"schedule": {"channel_id": "ch1", "interval": "2m"}}
	}`

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPut, "/flows/f1", bytes.NewBufferString(body))
	r.Header.Set("Mattermost-User-ID", "user1")

	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "at least")
}

func TestAPI_DeleteFlow(t *testing.T) {
	router, store, _ := setupAPI(t)

	require.NoError(t, store.Save(&model.Flow{ID: "f1", Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}}}))

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
	r := httptest.NewRequest(http.MethodPut, "/flows/f1", bytes.NewBufferString(`{"name":"x","trigger":{"message_posted":{"channel_id":"ch1"}}}`))
	// Deliberately omit Mattermost-User-ID header.

	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// setupAPIWithCustomMock creates an API handler with a custom plugintest.API
// so callers can set their own GetChannelMember expectations.
func setupAPIWithCustomMock(t *testing.T, api *plugintest.API) (*mux.Router, model.Store) {
	t.Helper()

	store, _ := setupStore(t)

	handler := NewAPIHandler(store, nil, api, nil)
	router := mux.NewRouter()
	handler.RegisterRoutes(router)

	return router, store
}

func TestAPI_CreateFlow_PermissionDenied(t *testing.T) {
	api := &plugintest.API{}
	api.On("LogError", mock.Anything, mock.Anything, mock.Anything).Maybe()
	api.On("HasPermissionTo", "user1", mmmodel.PermissionManageSystem).Return(false)
	api.On("GetChannelMember", "ch1", "user1").Return(
		&mmmodel.ChannelMember{SchemeAdmin: false}, nil,
	)

	router, _ := setupAPIWithCustomMock(t, api)

	body := `{
		"name": "Test Flow",
		"enabled": true,
		"trigger": {"message_posted": {"channel_id": "ch1"}},
		"actions": [{"id": "send-message", "send_message": {"channel_id": "ch2", "body": "hello"}}]
	}`

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/flows", bytes.NewBufferString(body))
	r.Header.Set("Mattermost-User-ID", "user1")

	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "ch1")
}

func TestAPI_CreateFlow_ActionPermissionDenied(t *testing.T) {
	api := &plugintest.API{}
	api.On("LogError", mock.Anything, mock.Anything, mock.Anything).Maybe()
	api.On("HasPermissionTo", "user1", mmmodel.PermissionManageSystem).Return(false)
	api.On("GetChannelMember", "ch1", "user1").Return(
		&mmmodel.ChannelMember{SchemeAdmin: true}, nil,
	)
	api.On("GetChannelMember", "ch2", "user1").Return(
		&mmmodel.ChannelMember{SchemeAdmin: false}, nil,
	)

	router, _ := setupAPIWithCustomMock(t, api)

	body := `{
		"name": "Test Flow",
		"enabled": true,
		"trigger": {"message_posted": {"channel_id": "ch1"}},
		"actions": [{"id": "send-message", "send_message": {"channel_id": "ch2", "body": "hello"}}]
	}`

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/flows", bytes.NewBufferString(body))
	r.Header.Set("Mattermost-User-ID", "user1")

	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "ch2")
}

func TestAPI_CreateFlow_NotChannelMember(t *testing.T) {
	api := &plugintest.API{}
	api.On("LogError", mock.Anything, mock.Anything, mock.Anything).Maybe()
	api.On("HasPermissionTo", "user1", mmmodel.PermissionManageSystem).Return(false)
	api.On("GetChannelMember", "ch1", "user1").Return(
		nil, mmmodel.NewAppError("GetChannelMember", "not_found", nil, "", http.StatusNotFound),
	)

	router, _ := setupAPIWithCustomMock(t, api)

	body := `{
		"name": "Test Flow",
		"enabled": true,
		"trigger": {"message_posted": {"channel_id": "ch1"}},
		"actions": [{"id": "send-message", "send_message": {"channel_id": "ch2", "body": "hello"}}]
	}`

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/flows", bytes.NewBufferString(body))
	r.Header.Set("Mattermost-User-ID", "user1")

	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "ch1")
}

func TestAPI_UpdateFlow_PermissionDenied(t *testing.T) {
	api := &plugintest.API{}
	api.On("LogError", mock.Anything, mock.Anything, mock.Anything).Maybe()
	api.On("HasPermissionTo", "user1", mmmodel.PermissionManageSystem).Return(false)
	// Allow existing flow's channel.
	api.On("GetChannelMember", "ch1", "user1").Return(
		&mmmodel.ChannelMember{SchemeAdmin: true}, nil,
	)
	// Deny new flow's channel.
	api.On("GetChannelMember", "ch-new", "user1").Return(
		&mmmodel.ChannelMember{SchemeAdmin: false}, nil,
	)

	router, store := setupAPIWithCustomMock(t, api)

	require.NoError(t, store.Save(&model.Flow{
		ID:      "f1",
		Name:    "Original",
		Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
	}))

	body := `{
		"name": "Updated",
		"enabled": true,
		"trigger": {"message_posted": {"channel_id": "ch-new"}},
		"actions": []
	}`

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPut, "/flows/f1", bytes.NewBufferString(body))
	r.Header.Set("Mattermost-User-ID", "user1")

	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "ch-new")
}

func TestAPI_CreateFlow_SystemAdminBypass(t *testing.T) {
	api := &plugintest.API{}
	api.On("LogError", mock.Anything, mock.Anything, mock.Anything).Maybe()
	api.On("HasPermissionTo", "admin1", mmmodel.PermissionManageSystem).Return(true)
	// No GetChannelMember expectation — system admin should skip channel checks.

	router, _ := setupAPIWithCustomMock(t, api)

	body := `{
		"name": "Admin Flow",
		"enabled": true,
		"trigger": {"message_posted": {"channel_id": "ch1"}},
		"actions": [{"id": "send-message", "send_message": {"channel_id": "ch2", "body": "hello"}}]
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
	api.On("LogError", mock.Anything, mock.Anything, mock.Anything).Maybe()
	api.On("HasPermissionTo", "admin1", mmmodel.PermissionManageSystem).Return(true)

	router, store := setupAPIWithCustomMock(t, api)

	require.NoError(t, store.Save(&model.Flow{
		ID:      "f1",
		Name:    "Original",
		Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
	}))

	body := `{
		"name": "Updated by admin",
		"enabled": true,
		"trigger": {"message_posted": {"channel_id": "ch-new"}},
		"actions": [{"id": "send-message", "send_message": {"channel_id": "ch3", "body": "hello"}}]
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
	api.On("LogError", mock.Anything, mock.Anything, mock.Anything).Maybe()
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
