package flow

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/mattermost/mattermost/server/public/plugin/plugintest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

func setupAPI(t *testing.T) (*mux.Router, model.Store) {
	t.Helper()

	store, _ := setupStore(t)

	api := &plugintest.API{}
	api.On("LogError", mock.Anything, mock.Anything, mock.Anything).Maybe()

	handler := NewAPIHandler(store, api, nil)
	router := mux.NewRouter()
	handler.RegisterRoutes(router)

	return router, store
}

func TestAPI_CreateFlow(t *testing.T) {
	router, store := setupAPI(t)

	body := `{
		"name": "Test Flow",
		"enabled": true,
		"trigger": {"type": "message_posted", "channel_id": "ch1"},
		"actions": [{"name": "Send", "type": "send_message", "channel_id": "ch2", "body": "hello"}]
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
	assert.NotEmpty(t, created.Actions[0].ID)
	assert.Equal(t, "hello", created.Actions[0].Body)

	// Verify it was persisted.
	got, err := store.Get(created.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, created.Name, got.Name)
}

func TestAPI_CreateFlow_InvalidBody(t *testing.T) {
	router, _ := setupAPI(t)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/flows", bytes.NewBufferString("not json"))
	r.Header.Set("Mattermost-User-ID", "user1")

	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAPI_GetFlow(t *testing.T) {
	router, store := setupAPI(t)

	require.NoError(t, store.Save(&model.Flow{
		ID:      "f1",
		Name:    "Flow 1",
		Trigger: model.Trigger{Type: "message_posted", ChannelID: "ch1"},
	}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/flows/f1", nil)

	router.ServeHTTP(w, r)

	require.Equal(t, http.StatusOK, w.Code)

	var got model.Flow
	require.NoError(t, json.NewDecoder(w.Body).Decode(&got))
	assert.Equal(t, "f1", got.ID)
	assert.Equal(t, "Flow 1", got.Name)
}

func TestAPI_GetFlow_NotFound(t *testing.T) {
	router, _ := setupAPI(t)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/flows/nonexistent", nil)

	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestAPI_ListFlows(t *testing.T) {
	router, store := setupAPI(t)

	require.NoError(t, store.Save(&model.Flow{ID: "f1", Name: "Flow 1", Trigger: model.Trigger{Type: "message_posted", ChannelID: "ch1"}}))
	require.NoError(t, store.Save(&model.Flow{ID: "f2", Name: "Flow 2", Trigger: model.Trigger{Type: "message_posted", ChannelID: "ch2"}}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/flows", nil)

	router.ServeHTTP(w, r)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var flows []*model.Flow
	require.NoError(t, json.NewDecoder(w.Body).Decode(&flows))
	assert.Len(t, flows, 2)
}

func TestAPI_ListFlows_Empty(t *testing.T) {
	router, _ := setupAPI(t)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/flows", nil)

	router.ServeHTTP(w, r)

	require.Equal(t, http.StatusOK, w.Code)

	var flows []*model.Flow
	require.NoError(t, json.NewDecoder(w.Body).Decode(&flows))
	assert.Empty(t, flows)
}

func TestAPI_UpdateFlow(t *testing.T) {
	router, store := setupAPI(t)

	original := &model.Flow{
		ID:        "f1",
		Name:      "Original",
		CreatedAt: 1000,
		CreatedBy: "original-user",
		Trigger:   model.Trigger{Type: "message_posted", ChannelID: "ch1"},
	}
	require.NoError(t, store.Save(original))

	body := `{
		"name": "Updated",
		"enabled": true,
		"trigger": {"type": "message_posted", "channel_id": "ch2"},
		"actions": [{"name": "New Action", "type": "send_message", "channel_id": "ch3", "body": "updated"}]
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
	assert.NotEmpty(t, updated.Actions[0].ID)
}

func TestAPI_UpdateFlow_NotFound(t *testing.T) {
	router, _ := setupAPI(t)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPut, "/flows/nonexistent", bytes.NewBufferString(`{"name":"x"}`))

	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestAPI_UpdateFlow_InvalidBody(t *testing.T) {
	router, store := setupAPI(t)

	require.NoError(t, store.Save(&model.Flow{ID: "f1", Trigger: model.Trigger{Type: "message_posted", ChannelID: "ch1"}}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPut, "/flows/f1", bytes.NewBufferString("not json"))

	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAPI_CreateFlow_ScheduleTrigger_MissingInterval(t *testing.T) {
	router, _ := setupAPI(t)

	body := `{
		"name": "Schedule Flow",
		"enabled": true,
		"trigger": {"type": "schedule"},
		"actions": [{"name": "Send", "type": "send_message", "channel_id": "ch2", "body": "hello"}]
	}`

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/flows", bytes.NewBufferString(body))
	r.Header.Set("Mattermost-User-ID", "user1")

	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "interval")
}

func TestAPI_CreateFlow_ScheduleTrigger_IntervalTooSmall(t *testing.T) {
	router, _ := setupAPI(t)

	body := `{
		"name": "Schedule Flow",
		"enabled": true,
		"trigger": {"type": "schedule", "interval": "1m"},
		"actions": [{"name": "Send", "type": "send_message", "channel_id": "ch2", "body": "hello"}]
	}`

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/flows", bytes.NewBufferString(body))
	r.Header.Set("Mattermost-User-ID", "user1")

	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "at least")
}

func TestAPI_CreateFlow_UnknownTriggerType(t *testing.T) {
	router, _ := setupAPI(t)

	body := `{
		"name": "Bad Trigger",
		"enabled": true,
		"trigger": {"type": "unknown_type"},
		"actions": [{"name": "Send", "type": "send_message", "channel_id": "ch2", "body": "hello"}]
	}`

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/flows", bytes.NewBufferString(body))
	r.Header.Set("Mattermost-User-ID", "user1")

	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAPI_UpdateFlow_ScheduleValidation(t *testing.T) {
	router, store := setupAPI(t)

	require.NoError(t, store.Save(&model.Flow{
		ID:      "f1",
		Trigger: model.Trigger{Type: "message_posted", ChannelID: "ch1"},
	}))

	body := `{
		"name": "Updated",
		"enabled": true,
		"trigger": {"type": "schedule", "interval": "2m"}
	}`

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPut, "/flows/f1", bytes.NewBufferString(body))

	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "at least")
}

func TestAPI_DeleteFlow(t *testing.T) {
	router, store := setupAPI(t)

	require.NoError(t, store.Save(&model.Flow{ID: "f1", Trigger: model.Trigger{Type: "message_posted", ChannelID: "ch1"}}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodDelete, "/flows/f1", nil)

	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusNoContent, w.Code)

	// Verify deleted.
	got, err := store.Get("f1")
	require.NoError(t, err)
	assert.Nil(t, got)
}
