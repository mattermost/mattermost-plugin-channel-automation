package execution

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
	api.On("KVSetWithExpiry", mock.Anything, mock.Anything, mock.Anything).Return(
		func(key string, value []byte, _ int64) *mmmodel.AppError {
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
	api.On("LogError", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Maybe()

	return NewStore(api, &sync.Mutex{}), kv
}

func makeRecord(id, automationID string) *model.ExecutionRecord {
	return &model.ExecutionRecord{
		ID:             id,
		AutomationID:   automationID,
		AutomationName: "Test Automation",
		Status:         "success",
		Steps: map[string]model.StepOutput{
			"step1": {Message: "done"},
		},
		CreatedAt:   1000,
		StartedAt:   1001,
		CompletedAt: 1002,
	}
}

func TestExecutionStore_SaveAndGet(t *testing.T) {
	store, _ := setupStore(t)

	rec := makeRecord("x1", "f1")
	require.NoError(t, store.Save(rec))

	got, err := store.Get("x1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "x1", got.ID)
	assert.Equal(t, "f1", got.AutomationID)
	assert.Equal(t, "Test Automation", got.AutomationName)
	assert.Equal(t, "success", got.Status)
	assert.Equal(t, "done", got.Steps["step1"].Message)
}

func TestExecutionStore_GetNotFound(t *testing.T) {
	store, _ := setupStore(t)

	got, err := store.Get("nonexistent")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestExecutionStore_ListByAutomation(t *testing.T) {
	store, _ := setupStore(t)

	// Save records for two different automations.
	require.NoError(t, store.Save(makeRecord("x1", "f1")))
	require.NoError(t, store.Save(makeRecord("x2", "f1")))
	require.NoError(t, store.Save(makeRecord("x3", "f2")))

	records, err := store.ListByAutomation("f1", 10)
	require.NoError(t, err)
	assert.Len(t, records, 2)

	// Most recent (last saved) should be first since prependToIndex puts new entries at front.
	assert.Equal(t, "x2", records[0].ID)
	assert.Equal(t, "x1", records[1].ID)
}

func TestExecutionStore_ListRecent(t *testing.T) {
	store, _ := setupStore(t)

	require.NoError(t, store.Save(makeRecord("x1", "f1")))
	require.NoError(t, store.Save(makeRecord("x2", "f2")))
	require.NoError(t, store.Save(makeRecord("x3", "f1")))

	records, err := store.ListRecent(10)
	require.NoError(t, err)
	assert.Len(t, records, 3)
	assert.Equal(t, "x3", records[0].ID)
	assert.Equal(t, "x2", records[1].ID)
	assert.Equal(t, "x1", records[2].ID)
}

func TestExecutionStore_ListRecent_RespectsLimit(t *testing.T) {
	store, _ := setupStore(t)

	require.NoError(t, store.Save(makeRecord("x1", "f1")))
	require.NoError(t, store.Save(makeRecord("x2", "f1")))
	require.NoError(t, store.Save(makeRecord("x3", "f1")))

	records, err := store.ListRecent(2)
	require.NoError(t, err)
	assert.Len(t, records, 2)
}

func TestExecutionStore_ListFromIndex_SkipsExpired(t *testing.T) {
	store, kv := setupStore(t)

	// Save 5 records.
	for i := 1; i <= 5; i++ {
		require.NoError(t, store.Save(makeRecord("x"+string(rune('0'+i)), "f1")))
	}

	// Simulate TTL expiry by deleting 2 records from KV.
	kv.mu.Lock()
	delete(kv.data, recordKeyPrefix+"x2")
	delete(kv.data, recordKeyPrefix+"x4")
	kv.mu.Unlock()

	// Request limit=3: should skip expired entries and still return 3 results.
	records, err := store.ListByAutomation("f1", 3)
	require.NoError(t, err)
	assert.Len(t, records, 3)

	// Stale entries should have been compacted from the index.
	kv.mu.Lock()
	data := kv.data[automationIndexPrefix+"f1"]
	kv.mu.Unlock()

	var ids []string
	require.NoError(t, json.Unmarshal(data, &ids))
	assert.Len(t, ids, 3, "index should have been compacted to remove stale entries")
	for _, id := range ids {
		assert.NotEqual(t, "x2", id)
		assert.NotEqual(t, "x4", id)
	}
}

func TestExecutionStore_PurgeAutomation(t *testing.T) {
	store, kv := setupStore(t)

	// Save records for two automations so we can verify cross-automation isolation.
	require.NoError(t, store.Save(makeRecord("x1", "f1")))
	require.NoError(t, store.Save(makeRecord("x2", "f1")))
	require.NoError(t, store.Save(makeRecord("x3", "f2")))

	// Per-automation index should exist.
	kv.mu.Lock()
	_, exists := kv.data[automationIndexPrefix+"f1"]
	kv.mu.Unlock()
	require.True(t, exists)

	// Global index should contain all 3 records.
	records, err := store.ListRecent(10)
	require.NoError(t, err)
	assert.Len(t, records, 3)

	require.NoError(t, store.PurgeAutomation("f1"))

	// Per-automation index should be deleted.
	kv.mu.Lock()
	_, exists = kv.data[automationIndexPrefix+"f1"]
	kv.mu.Unlock()
	assert.False(t, exists)

	// Individual records should be deleted.
	got, err := store.Get("x1")
	require.NoError(t, err)
	assert.Nil(t, got)

	got, err = store.Get("x2")
	require.NoError(t, err)
	assert.Nil(t, got)

	// Global index should only contain the other automation's record.
	// Inspect raw KV data directly — ListRecent self-heals stale entries,
	// so it would pass even if PurgeAutomation didn't clean the global index.
	kv.mu.Lock()
	globalIndexBytes, exists := kv.data[globalIndexKey]
	kv.mu.Unlock()
	require.True(t, exists)

	var globalIndex []string
	require.NoError(t, json.Unmarshal(globalIndexBytes, &globalIndex))
	assert.Equal(t, []string{"x3"}, globalIndex)

	// Records from the other automation should be untouched.
	got, err = store.Get("x3")
	require.NoError(t, err)
	assert.NotNil(t, got)
}

func TestExecutionStore_TruncateSteps(t *testing.T) {
	store, _ := setupStore(t)

	bigMsg := make([]byte, maxMessageBytes+100)
	for i := range bigMsg {
		bigMsg[i] = 'a'
	}

	rec := makeRecord("x1", "f1")
	rec.Steps = map[string]model.StepOutput{
		"step1": {Message: string(bigMsg)},
	}

	require.NoError(t, store.Save(rec))

	got, err := store.Get("x1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Len(t, got.Steps["step1"].Message, maxMessageBytes)
	assert.True(t, got.Steps["step1"].Truncated)
}

func TestExecutionStore_IndexCapping(t *testing.T) {
	store, kv := setupStore(t)

	// Save more records than maxAutomationIndexSize.
	for range maxAutomationIndexSize + 10 {
		rec := makeRecord(mmmodel.NewId(), "f1")
		require.NoError(t, store.Save(rec))
	}

	// Automation index should be capped at maxAutomationIndexSize.
	kv.mu.Lock()
	data := kv.data[automationIndexPrefix+"f1"]
	kv.mu.Unlock()

	var ids []string
	require.NoError(t, json.Unmarshal(data, &ids))
	assert.Len(t, ids, maxAutomationIndexSize)
}

func TestExecutionStore_GlobalIndexCapping(t *testing.T) {
	store, kv := setupStore(t)

	// Save more records than maxGlobalIndexSize.
	for range maxGlobalIndexSize + 10 {
		rec := makeRecord(mmmodel.NewId(), mmmodel.NewId())
		require.NoError(t, store.Save(rec))
	}

	// Global index should be capped at maxGlobalIndexSize.
	kv.mu.Lock()
	data := kv.data[globalIndexKey]
	kv.mu.Unlock()

	var ids []string
	require.NoError(t, json.Unmarshal(data, &ids))
	assert.Len(t, ids, maxGlobalIndexSize)
}
