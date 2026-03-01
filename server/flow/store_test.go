package flow

import (
	"encoding/json"
	"sync"
	"testing"

	mmmodel "github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin/plugintest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

// inMemoryKV backs the plugintest.API mock with an in-memory map so
// tests can exercise the real store logic without per-call expectations.
type inMemoryKV struct {
	mu   sync.Mutex
	data map[string][]byte
}

func setupStore(t *testing.T) (model.Store, *inMemoryKV) {
	t.Helper()

	kv := &inMemoryKV{data: make(map[string][]byte)}

	api := &plugintest.API{}
	api.On("KVGet", mock.Anything).Return(
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
	api.On("KVSet", mock.Anything, mock.Anything).Return(
		func(key string, value []byte) *mmmodel.AppError {
			kv.mu.Lock()
			defer kv.mu.Unlock()
			cp := make([]byte, len(value))
			copy(cp, value)
			kv.data[key] = cp
			return nil
		},
	)
	api.On("KVDelete", mock.Anything).Return(
		func(key string) *mmmodel.AppError {
			kv.mu.Lock()
			defer kv.mu.Unlock()
			delete(kv.data, key)
			return nil
		},
	)

	return NewStore(api), kv
}

func TestStore_SaveAndGet(t *testing.T) {
	store, _ := setupStore(t)

	f := &model.Flow{
		ID:      "flow1",
		Name:    "Test Flow",
		Enabled: true,
		Trigger: model.Trigger{Type: "message_posted", ChannelID: "ch1"},
		Actions: []model.Action{
			{ID: "act1", Name: "Send", Type: "send_message", ChannelID: "ch2", Body: "hello"},
		},
	}

	require.NoError(t, store.Save(f))

	got, err := store.Get("flow1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "flow1", got.ID)
	assert.Equal(t, "Test Flow", got.Name)
	assert.True(t, got.Enabled)
	assert.Equal(t, "message_posted", got.Trigger.Type)
	assert.Equal(t, "ch1", got.Trigger.ChannelID)
	require.Len(t, got.Actions, 1)
	assert.Equal(t, "hello", got.Actions[0].Body)
}

func TestStore_GetNonExistent(t *testing.T) {
	store, _ := setupStore(t)

	got, err := store.Get("nonexistent")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestStore_List(t *testing.T) {
	store, _ := setupStore(t)

	flows, err := store.List()
	require.NoError(t, err)
	assert.Empty(t, flows)

	require.NoError(t, store.Save(&model.Flow{ID: "f1", Name: "Flow 1", Trigger: model.Trigger{Type: "message_posted", ChannelID: "ch1"}}))
	require.NoError(t, store.Save(&model.Flow{ID: "f2", Name: "Flow 2", Trigger: model.Trigger{Type: "message_posted", ChannelID: "ch2"}}))

	flows, err = store.List()
	require.NoError(t, err)
	require.Len(t, flows, 2)
	assert.Equal(t, "f1", flows[0].ID)
	assert.Equal(t, "f2", flows[1].ID)
}

func TestStore_Delete(t *testing.T) {
	store, _ := setupStore(t)

	require.NoError(t, store.Save(&model.Flow{ID: "f1", Name: "Flow 1", Trigger: model.Trigger{Type: "message_posted", ChannelID: "ch1"}}))

	got, err := store.Get("f1")
	require.NoError(t, err)
	require.NotNil(t, got)

	require.NoError(t, store.Delete("f1"))

	got, err = store.Get("f1")
	require.NoError(t, err)
	assert.Nil(t, got)

	flows, err := store.List()
	require.NoError(t, err)
	assert.Empty(t, flows)
}

func TestStore_DeleteNonExistent(t *testing.T) {
	store, _ := setupStore(t)
	require.NoError(t, store.Delete("nonexistent"))
}

func TestStore_TriggerIndex(t *testing.T) {
	store, kv := setupStore(t)
	kvStore := store.(*KVStore)

	require.NoError(t, store.Save(&model.Flow{ID: "f1", Trigger: model.Trigger{Type: "message_posted", ChannelID: "ch1"}}))
	require.NoError(t, store.Save(&model.Flow{ID: "f2", Trigger: model.Trigger{Type: "message_posted", ChannelID: "ch1"}}))

	ids, err := kvStore.GetFlowIDsForChannel("ch1")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"f1", "f2"}, ids)

	require.NoError(t, store.Delete("f1"))
	ids, err = kvStore.GetFlowIDsForChannel("ch1")
	require.NoError(t, err)
	assert.Equal(t, []string{"f2"}, ids)

	require.NoError(t, store.Delete("f2"))
	ids, err = kvStore.GetFlowIDsForChannel("ch1")
	require.NoError(t, err)
	assert.Nil(t, ids)

	// Key should be cleaned up from KV store.
	kv.mu.Lock()
	_, exists := kv.data[makeTriggerIndexKey("ch1")]
	kv.mu.Unlock()
	assert.False(t, exists)
}

func TestStore_TriggerIndex_UpdateChannel(t *testing.T) {
	store, _ := setupStore(t)
	kvStore := store.(*KVStore)

	require.NoError(t, store.Save(&model.Flow{ID: "f1", Trigger: model.Trigger{Type: "message_posted", ChannelID: "ch1"}}))

	ids, err := kvStore.GetFlowIDsForChannel("ch1")
	require.NoError(t, err)
	assert.Equal(t, []string{"f1"}, ids)

	// Update flow to watch ch2.
	require.NoError(t, store.Save(&model.Flow{ID: "f1", Trigger: model.Trigger{Type: "message_posted", ChannelID: "ch2"}}))

	ids, err = kvStore.GetFlowIDsForChannel("ch1")
	require.NoError(t, err)
	assert.Nil(t, ids)

	ids, err = kvStore.GetFlowIDsForChannel("ch2")
	require.NoError(t, err)
	assert.Equal(t, []string{"f1"}, ids)
}

func TestStore_TriggerIndex_NoDuplicates(t *testing.T) {
	store, _ := setupStore(t)
	kvStore := store.(*KVStore)

	// Save the same flow twice (simulating update with same channel).
	require.NoError(t, store.Save(&model.Flow{ID: "f1", Trigger: model.Trigger{Type: "message_posted", ChannelID: "ch1"}}))
	require.NoError(t, store.Save(&model.Flow{ID: "f1", Trigger: model.Trigger{Type: "message_posted", ChannelID: "ch1"}}))

	ids, err := kvStore.GetFlowIDsForChannel("ch1")
	require.NoError(t, err)
	assert.Equal(t, []string{"f1"}, ids)
}

func TestStore_FlowIndex_Consistency(t *testing.T) {
	store, kv := setupStore(t)

	require.NoError(t, store.Save(&model.Flow{ID: "f1", Trigger: model.Trigger{Type: "message_posted", ChannelID: "ch1"}}))
	require.NoError(t, store.Save(&model.Flow{ID: "f2", Trigger: model.Trigger{Type: "message_posted", ChannelID: "ch2"}}))
	require.NoError(t, store.Save(&model.Flow{ID: "f3", Trigger: model.Trigger{Type: "message_posted", ChannelID: "ch1"}}))
	require.NoError(t, store.Delete("f2"))

	kv.mu.Lock()
	indexData := kv.data[flowIndexKey]
	kv.mu.Unlock()

	var ids []string
	require.NoError(t, json.Unmarshal(indexData, &ids))
	assert.ElementsMatch(t, []string{"f1", "f3"}, ids)
}
