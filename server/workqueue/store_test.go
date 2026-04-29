package workqueue

import (
	"sync"
	"testing"

	mmmodel "github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin/plugintest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

type inMemoryKV struct {
	mu   sync.Mutex
	data map[string][]byte
}

func setupStore(t *testing.T) (*Store, *inMemoryKV) {
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

	return NewStore(api, &sync.Mutex{}), kv
}

func makeItem(id, automationID, automationName string) *model.WorkItem {
	return &model.WorkItem{
		ID:             id,
		AutomationID:   automationID,
		AutomationName: automationName,
		TriggerData: model.TriggerData{
			Post: &model.SafePost{Id: "post1", ChannelId: "ch1", Message: "hello"},
		},
	}
}

func TestStore_EnqueueAndClaimNext(t *testing.T) {
	store, _ := setupStore(t)

	item := makeItem("w1", "f1", "Flow 1")
	require.NoError(t, store.Enqueue(item))

	claimed, err := store.ClaimNext()
	require.NoError(t, err)
	require.NotNil(t, claimed)
	assert.Equal(t, "w1", claimed.ID)
	assert.Equal(t, "f1", claimed.AutomationID)
	assert.Equal(t, "Flow 1", claimed.AutomationName)
	assert.Equal(t, model.WorkItemStatusRunning, claimed.Status)
	assert.NotZero(t, claimed.CreatedAt)
	assert.NotZero(t, claimed.StartedAt)
}

func TestStore_ClaimNextEmptyQueue(t *testing.T) {
	store, _ := setupStore(t)

	claimed, err := store.ClaimNext()
	require.NoError(t, err)
	assert.Nil(t, claimed)
}

func TestStore_ClaimNextOrdering(t *testing.T) {
	store, _ := setupStore(t)

	require.NoError(t, store.Enqueue(makeItem("w1", "f1", "Flow 1")))
	require.NoError(t, store.Enqueue(makeItem("w2", "f2", "Flow 2")))
	require.NoError(t, store.Enqueue(makeItem("w3", "f3", "Flow 3")))

	claimed, err := store.ClaimNext()
	require.NoError(t, err)
	require.NotNil(t, claimed)
	assert.Equal(t, "w1", claimed.ID)

	claimed, err = store.ClaimNext()
	require.NoError(t, err)
	require.NotNil(t, claimed)
	assert.Equal(t, "w2", claimed.ID)

	claimed, err = store.ClaimNext()
	require.NoError(t, err)
	require.NotNil(t, claimed)
	assert.Equal(t, "w3", claimed.ID)

	claimed, err = store.ClaimNext()
	require.NoError(t, err)
	assert.Nil(t, claimed)
}

func TestStore_Complete(t *testing.T) {
	store, kv := setupStore(t)

	require.NoError(t, store.Enqueue(makeItem("w1", "f1", "Flow 1")))

	claimed, err := store.ClaimNext()
	require.NoError(t, err)
	require.NotNil(t, claimed)

	require.NoError(t, store.Complete("w1"))

	// Item should be deleted from KV.
	got, err := store.Get("w1")
	require.NoError(t, err)
	assert.Nil(t, got)

	// Running index should be empty.
	kv.mu.Lock()
	_, exists := kv.data[runningIndexKey]
	kv.mu.Unlock()
	assert.False(t, exists)
}

func TestStore_Fail(t *testing.T) {
	store, kv := setupStore(t)

	require.NoError(t, store.Enqueue(makeItem("w1", "f1", "Flow 1")))

	claimed, err := store.ClaimNext()
	require.NoError(t, err)
	require.NotNil(t, claimed)

	require.NoError(t, store.Fail("w1"))

	// Item should be deleted from KV.
	got, err := store.Get("w1")
	require.NoError(t, err)
	assert.Nil(t, got)

	// Running index should be empty.
	kv.mu.Lock()
	_, exists := kv.data[runningIndexKey]
	kv.mu.Unlock()
	assert.False(t, exists)
}

func TestStore_ResetRunningToPending(t *testing.T) {
	store, _ := setupStore(t)

	require.NoError(t, store.Enqueue(makeItem("w1", "f1", "Flow 1")))
	require.NoError(t, store.Enqueue(makeItem("w2", "f2", "Flow 2")))

	// Claim both items (move to running).
	_, err := store.ClaimNext()
	require.NoError(t, err)
	_, err = store.ClaimNext()
	require.NoError(t, err)

	// Nothing should be pending now.
	claimed, err := store.ClaimNext()
	require.NoError(t, err)
	assert.Nil(t, claimed)

	// Reset running → pending.
	count, err := store.ResetRunningToPending()
	require.NoError(t, err)
	assert.Equal(t, 2, count)

	// Items should now be claimable again.
	claimed, err = store.ClaimNext()
	require.NoError(t, err)
	require.NotNil(t, claimed)
	assert.Equal(t, "w1", claimed.ID)

	claimed, err = store.ClaimNext()
	require.NoError(t, err)
	require.NotNil(t, claimed)
	assert.Equal(t, "w2", claimed.ID)
}

func TestStore_StaleIndexEntry(t *testing.T) {
	store, kv := setupStore(t)

	// Enqueue two items.
	require.NoError(t, store.Enqueue(makeItem("w1", "f1", "Flow 1")))
	require.NoError(t, store.Enqueue(makeItem("w2", "f2", "Flow 2")))

	// Simulate stale index by deleting w1's data directly from KV.
	kv.mu.Lock()
	delete(kv.data, workItemKeyPrefix+"w1")
	kv.mu.Unlock()

	// ClaimNext should skip the stale entry and return w2.
	claimed, err := store.ClaimNext()
	require.NoError(t, err)
	require.NotNil(t, claimed)
	assert.Equal(t, "w2", claimed.ID)
}

func TestStore_ResetRunningToPending_StaleEntry(t *testing.T) {
	store, kv := setupStore(t)

	require.NoError(t, store.Enqueue(makeItem("w1", "f1", "Flow 1")))
	require.NoError(t, store.Enqueue(makeItem("w2", "f2", "Flow 2")))

	_, err := store.ClaimNext()
	require.NoError(t, err)
	_, err = store.ClaimNext()
	require.NoError(t, err)

	// Delete w1 to simulate a stale entry.
	kv.mu.Lock()
	delete(kv.data, workItemKeyPrefix+"w1")
	kv.mu.Unlock()

	count, err := store.ResetRunningToPending()
	require.NoError(t, err)
	assert.Equal(t, 1, count) // Only w2 was reset.
}
