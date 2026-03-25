package execution

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gorilla/mux"
	mmmodel "github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin/plugintest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

// mockFlowStore implements model.Store for testing.
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

// expectLogCalls registers permissive LogError and LogWarn expectations that
// accept any number of arguments. This covers enriched log calls that include
// user_id, flow_id, execution_id, and other context fields.
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

func setupAPIHandler(t *testing.T) (*mux.Router, *Store, *mockFlowStore, *plugintest.API) {
	t.Helper()

	execStore, _ := setupStore(t)
	flowStore := &mockFlowStore{flows: make(map[string]*model.Flow)}

	api := &plugintest.API{}
	expectLogCalls(api)
	api.On("HasPermissionTo", mock.Anything, mock.Anything).Return(false).Maybe()
	api.On("GetChannelMember", mock.Anything, mock.Anything).Return(
		&mmmodel.ChannelMember{SchemeAdmin: true}, nil,
	).Maybe()

	handler := NewAPIHandler(execStore, flowStore, api)
	router := mux.NewRouter()
	handler.RegisterRoutes(router)

	return router, execStore, flowStore, api
}

func setupAPIHandlerWithCustomMock(t *testing.T, api *plugintest.API) (*mux.Router, *Store, *mockFlowStore) {
	t.Helper()

	execStore, _ := setupStore(t)
	flowStore := &mockFlowStore{flows: make(map[string]*model.Flow)}

	handler := NewAPIHandler(execStore, flowStore, api)
	router := mux.NewRouter()
	handler.RegisterRoutes(router)

	return router, execStore, flowStore
}

func TestParseLimit(t *testing.T) {
	t.Run("default when missing", func(t *testing.T) {
		r := httptest.NewRequest("GET", "/executions", nil)
		assert.Equal(t, defaultLimit, parseLimit(r))
	})

	t.Run("valid value", func(t *testing.T) {
		r := httptest.NewRequest("GET", "/executions?limit=50", nil)
		assert.Equal(t, 50, parseLimit(r))
	})

	t.Run("capped at max", func(t *testing.T) {
		r := httptest.NewRequest("GET", "/executions?limit=999999999", nil)
		assert.Equal(t, maxLimit, parseLimit(r))
	})

	t.Run("exactly max", func(t *testing.T) {
		r := httptest.NewRequest("GET", "/executions?limit=100", nil)
		assert.Equal(t, maxLimit, parseLimit(r))
	})

	t.Run("zero falls back to default", func(t *testing.T) {
		r := httptest.NewRequest("GET", "/executions?limit=0", nil)
		assert.Equal(t, defaultLimit, parseLimit(r))
	})

	t.Run("negative falls back to default", func(t *testing.T) {
		r := httptest.NewRequest("GET", "/executions?limit=-5", nil)
		assert.Equal(t, defaultLimit, parseLimit(r))
	})

	t.Run("non-numeric falls back to default", func(t *testing.T) {
		r := httptest.NewRequest("GET", "/executions?limit=abc", nil)
		assert.Equal(t, defaultLimit, parseLimit(r))
	})
}

func TestExecutionAPI_ListByFlow_RequiresAuth(t *testing.T) {
	router, _, _, _ := setupAPIHandler(t)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/flows/f1/executions", nil)
	// No Mattermost-User-ID header.

	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestExecutionAPI_ListByFlow_FlowNotFound(t *testing.T) {
	router, _, _, _ := setupAPIHandler(t)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/flows/nonexistent/executions", nil)
	r.Header.Set("Mattermost-User-ID", "user1")

	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestExecutionAPI_ListByFlow_RequiresFlowPermission(t *testing.T) {
	api := &plugintest.API{}
	expectLogCalls(api)
	api.On("HasPermissionTo", "user1", mmmodel.PermissionManageSystem).Return(false)
	api.On("GetChannelMember", "ch1", "user1").Return(
		&mmmodel.ChannelMember{SchemeAdmin: false}, nil,
	)

	router, _, flowStore := setupAPIHandlerWithCustomMock(t, api)
	flowStore.flows["f1"] = &model.Flow{
		ID:      "f1",
		Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/flows/f1/executions", nil)
	r.Header.Set("Mattermost-User-ID", "user1")

	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestExecutionAPI_ListByFlow_ChannelCreatedRequiresTeamAdmin(t *testing.T) {
	api := &plugintest.API{}
	expectLogCalls(api)
	api.On("HasPermissionTo", "user1", mmmodel.PermissionManageSystem).Return(false)
	api.On("GetTeam", "team1").Return(&mmmodel.Team{Id: "team1"}, nil)
	api.On("HasPermissionToTeam", "user1", "team1", mmmodel.PermissionManageTeam).Return(false)

	router, _, flowStore := setupAPIHandlerWithCustomMock(t, api)
	flowStore.flows["f1"] = &model.Flow{
		ID:      "f1",
		Trigger: model.Trigger{ChannelCreated: &model.ChannelCreatedConfig{TeamID: "team1"}},
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/flows/f1/executions", nil)
	r.Header.Set("Mattermost-User-ID", "user1")

	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestExecutionAPI_ListByFlow_Success(t *testing.T) {
	router, execStore, flowStore, _ := setupAPIHandler(t)

	flowStore.flows["f1"] = &model.Flow{
		ID:      "f1",
		Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
	}

	require.NoError(t, execStore.Save(makeRecord("x1", "f1")))
	require.NoError(t, execStore.Save(makeRecord("x2", "f1")))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/flows/f1/executions", nil)
	r.Header.Set("Mattermost-User-ID", "user1")

	router.ServeHTTP(w, r)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var records []*model.ExecutionRecord
	require.NoError(t, json.NewDecoder(w.Body).Decode(&records))
	assert.Len(t, records, 2)
}

func TestExecutionAPI_Get_NotFound(t *testing.T) {
	router, _, _, _ := setupAPIHandler(t)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/executions/nonexistent", nil)
	r.Header.Set("Mattermost-User-ID", "user1")

	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestExecutionAPI_Get_Success(t *testing.T) {
	router, execStore, flowStore, _ := setupAPIHandler(t)

	flowStore.flows["f1"] = &model.Flow{
		ID:      "f1",
		Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
	}

	require.NoError(t, execStore.Save(makeRecord("x1", "f1")))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/executions/x1", nil)
	r.Header.Set("Mattermost-User-ID", "user1")

	router.ServeHTTP(w, r)

	require.Equal(t, http.StatusOK, w.Code)

	var rec model.ExecutionRecord
	require.NoError(t, json.NewDecoder(w.Body).Decode(&rec))
	assert.Equal(t, "x1", rec.ID)
}

func TestExecutionAPI_Get_DeletedFlow_RequiresSystemAdmin(t *testing.T) {
	api := &plugintest.API{}
	expectLogCalls(api)
	api.On("HasPermissionTo", "user1", mmmodel.PermissionManageSystem).Return(false)

	router, execStore, _ := setupAPIHandlerWithCustomMock(t, api)

	// Save a record but don't add the flow to the flow store (simulates deleted flow).
	require.NoError(t, execStore.Save(makeRecord("x1", "deleted-flow")))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/executions/x1", nil)
	r.Header.Set("Mattermost-User-ID", "user1")

	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestExecutionAPI_Get_DeletedFlow_SystemAdminAllowed(t *testing.T) {
	api := &plugintest.API{}
	expectLogCalls(api)
	api.On("HasPermissionTo", "admin1", mmmodel.PermissionManageSystem).Return(true)

	router, execStore, _ := setupAPIHandlerWithCustomMock(t, api)

	require.NoError(t, execStore.Save(makeRecord("x1", "deleted-flow")))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/executions/x1", nil)
	r.Header.Set("Mattermost-User-ID", "admin1")

	router.ServeHTTP(w, r)

	require.Equal(t, http.StatusOK, w.Code)

	var rec model.ExecutionRecord
	require.NoError(t, json.NewDecoder(w.Body).Decode(&rec))
	assert.Equal(t, "x1", rec.ID)
}

func TestExecutionAPI_ListRecent_RequiresAuth(t *testing.T) {
	router, _, _, _ := setupAPIHandler(t)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/executions", nil)
	// No Mattermost-User-ID header.

	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestExecutionAPI_ListRecent_SystemAdminOnly(t *testing.T) {
	router, _, _, _ := setupAPIHandler(t)

	// Default mock returns false for HasPermissionTo.
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/executions", nil)
	r.Header.Set("Mattermost-User-ID", "user1")

	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestExecutionAPI_ListRecent_Success(t *testing.T) {
	api := &plugintest.API{}
	expectLogCalls(api)
	api.On("HasPermissionTo", "admin1", mmmodel.PermissionManageSystem).Return(true)

	// Need a real execution store backed by KV mocks.
	kv := &inMemoryKV{data: make(map[string][]byte)}
	kvAPI := &plugintest.API{}
	kvAPI.On("KVGet", mock.Anything).Return(
		func(key string) []byte {
			kv.mu.Lock()
			defer kv.mu.Unlock()
			d := kv.data[key]
			if d == nil {
				return nil
			}
			cp := make([]byte, len(d))
			copy(cp, d)
			return cp
		},
		func(_ string) *mmmodel.AppError { return nil },
	)
	kvAPI.On("KVSet", mock.Anything, mock.Anything).Return(
		func(key string, value []byte) *mmmodel.AppError {
			kv.mu.Lock()
			defer kv.mu.Unlock()
			cp := make([]byte, len(value))
			copy(cp, value)
			kv.data[key] = cp
			return nil
		},
	)
	kvAPI.On("KVSetWithExpiry", mock.Anything, mock.Anything, mock.Anything).Return(
		func(key string, value []byte, _ int64) *mmmodel.AppError {
			kv.mu.Lock()
			defer kv.mu.Unlock()
			cp := make([]byte, len(value))
			copy(cp, value)
			kv.data[key] = cp
			return nil
		},
	)
	kvAPI.On("KVDelete", mock.Anything).Return(
		func(key string) *mmmodel.AppError {
			kv.mu.Lock()
			defer kv.mu.Unlock()
			delete(kv.data, key)
			return nil
		},
	)
	expectLogCalls(kvAPI)

	execStore := NewStore(kvAPI, &sync.Mutex{})
	flowStore := &mockFlowStore{flows: make(map[string]*model.Flow)}

	require.NoError(t, execStore.Save(makeRecord("x1", "f1")))
	require.NoError(t, execStore.Save(makeRecord("x2", "f2")))

	handler := NewAPIHandler(execStore, flowStore, api)
	router := mux.NewRouter()
	handler.RegisterRoutes(router)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/executions", nil)
	r.Header.Set("Mattermost-User-ID", "admin1")

	router.ServeHTTP(w, r)

	require.Equal(t, http.StatusOK, w.Code)

	var records []*model.ExecutionRecord
	require.NoError(t, json.NewDecoder(w.Body).Decode(&records))
	assert.Len(t, records, 2)
}
