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

	return NewStore(api, &sync.Mutex{}), kv
}

func TestStore_SaveAndGet(t *testing.T) {
	store, _ := setupStore(t)

	f := &model.Flow{
		ID:      "flow1",
		Name:    "Test Flow",
		Enabled: true,
		Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
		Actions: []model.Action{
			{ID: "act1", SendMessage: &model.SendMessageActionConfig{ChannelID: "ch2", Body: "hello"}},
		},
	}

	require.NoError(t, store.Save(f))

	got, err := store.Get("flow1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "flow1", got.ID)
	assert.Equal(t, "Test Flow", got.Name)
	assert.True(t, got.Enabled)
	assert.Equal(t, "message_posted", got.Trigger.Type())
	require.NotNil(t, got.Trigger.MessagePosted)
	assert.Equal(t, "ch1", got.Trigger.MessagePosted.ChannelID)
	require.Len(t, got.Actions, 1)
	require.NotNil(t, got.Actions[0].SendMessage)
	assert.Equal(t, "hello", got.Actions[0].SendMessage.Body)
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

	require.NoError(t, store.Save(&model.Flow{ID: "f1", Name: "Flow 1", Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}}}))
	require.NoError(t, store.Save(&model.Flow{ID: "f2", Name: "Flow 2", Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch2"}}}))

	flows, err = store.List()
	require.NoError(t, err)
	require.Len(t, flows, 2)
	assert.Equal(t, "f1", flows[0].ID)
	assert.Equal(t, "f2", flows[1].ID)
}

func TestStore_Delete(t *testing.T) {
	store, _ := setupStore(t)

	require.NoError(t, store.Save(&model.Flow{ID: "f1", Name: "Flow 1", Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}}}))

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

	require.NoError(t, store.Save(&model.Flow{ID: "f1", Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}}}))
	require.NoError(t, store.Save(&model.Flow{ID: "f2", Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}}}))

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

	require.NoError(t, store.Save(&model.Flow{ID: "f1", Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}}}))

	ids, err := kvStore.GetFlowIDsForChannel("ch1")
	require.NoError(t, err)
	assert.Equal(t, []string{"f1"}, ids)

	// Update flow to watch ch2.
	require.NoError(t, store.Save(&model.Flow{ID: "f1", Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch2"}}}))

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
	require.NoError(t, store.Save(&model.Flow{ID: "f1", Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}}}))
	require.NoError(t, store.Save(&model.Flow{ID: "f1", Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}}}))

	ids, err := kvStore.GetFlowIDsForChannel("ch1")
	require.NoError(t, err)
	assert.Equal(t, []string{"f1"}, ids)
}

func TestStore_ChannelTriggerIndex(t *testing.T) {
	store, _ := setupStore(t)

	// Save flows with both trigger types targeting the same channel.
	require.NoError(t, store.Save(&model.Flow{ID: "f1", Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}}}))
	require.NoError(t, store.Save(&model.Flow{ID: "f2", Trigger: model.Trigger{Schedule: &model.ScheduleConfig{ChannelID: "ch1", Interval: "1h"}}}))
	require.NoError(t, store.Save(&model.Flow{ID: "f3", Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch2"}}}))

	flows, err := store.ListByTriggerChannel("ch1")
	require.NoError(t, err)
	require.Len(t, flows, 2)
	ids := []string{flows[0].ID, flows[1].ID}
	assert.ElementsMatch(t, []string{"f1", "f2"}, ids)

	flows, err = store.ListByTriggerChannel("ch2")
	require.NoError(t, err)
	require.Len(t, flows, 1)
	assert.Equal(t, "f3", flows[0].ID)

	flows, err = store.ListByTriggerChannel("ch-nonexistent")
	require.NoError(t, err)
	assert.Empty(t, flows)
}

func TestStore_ChannelTriggerIndex_UpdateChannel(t *testing.T) {
	store, _ := setupStore(t)

	require.NoError(t, store.Save(&model.Flow{ID: "f1", Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}}}))

	flows, err := store.ListByTriggerChannel("ch1")
	require.NoError(t, err)
	require.Len(t, flows, 1)

	// Update flow to target ch2 instead.
	require.NoError(t, store.Save(&model.Flow{ID: "f1", Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch2"}}}))

	flows, err = store.ListByTriggerChannel("ch1")
	require.NoError(t, err)
	assert.Empty(t, flows)

	flows, err = store.ListByTriggerChannel("ch2")
	require.NoError(t, err)
	require.Len(t, flows, 1)
	assert.Equal(t, "f1", flows[0].ID)
}

func TestStore_ChannelTriggerIndex_Delete(t *testing.T) {
	store, kv := setupStore(t)

	require.NoError(t, store.Save(&model.Flow{ID: "f1", Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}}}))
	require.NoError(t, store.Save(&model.Flow{ID: "f2", Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}}}))

	require.NoError(t, store.Delete("f1"))
	flows, err := store.ListByTriggerChannel("ch1")
	require.NoError(t, err)
	require.Len(t, flows, 1)
	assert.Equal(t, "f2", flows[0].ID)

	require.NoError(t, store.Delete("f2"))
	flows, err = store.ListByTriggerChannel("ch1")
	require.NoError(t, err)
	assert.Empty(t, flows)

	// Key should be cleaned up from KV store.
	kv.mu.Lock()
	_, exists := kv.data[makeChannelTriggerIndexKey("ch1")]
	kv.mu.Unlock()
	assert.False(t, exists)
}

func TestStore_ChannelTriggerIndex_ScheduleTrigger(t *testing.T) {
	store, _ := setupStore(t)

	require.NoError(t, store.Save(&model.Flow{ID: "f1", Trigger: model.Trigger{Schedule: &model.ScheduleConfig{ChannelID: "ch1", Interval: "1h"}}}))

	flows, err := store.ListByTriggerChannel("ch1")
	require.NoError(t, err)
	require.Len(t, flows, 1)
	assert.Equal(t, "f1", flows[0].ID)

	// Verify that the message_posted trigger index does NOT contain this flow.
	kvStore := store.(*KVStore)
	ids, err := kvStore.GetFlowIDsForChannel("ch1")
	require.NoError(t, err)
	assert.Empty(t, ids)
}

func TestStore_ScheduleIndex(t *testing.T) {
	store, kv := setupStore(t)

	// Save two schedule flows.
	require.NoError(t, store.Save(&model.Flow{ID: "f1", Trigger: model.Trigger{Schedule: &model.ScheduleConfig{ChannelID: "ch1", Interval: "1h"}}}))
	require.NoError(t, store.Save(&model.Flow{ID: "f2", Trigger: model.Trigger{Schedule: &model.ScheduleConfig{ChannelID: "ch2", Interval: "30m"}}}))

	flows, err := store.ListScheduled()
	require.NoError(t, err)
	require.Len(t, flows, 2)
	ids := []string{flows[0].ID, flows[1].ID}
	assert.ElementsMatch(t, []string{"f1", "f2"}, ids)

	// Delete one — returns one.
	require.NoError(t, store.Delete("f1"))
	flows, err = store.ListScheduled()
	require.NoError(t, err)
	require.Len(t, flows, 1)
	assert.Equal(t, "f2", flows[0].ID)

	// Delete both — empty + KV key cleaned up.
	require.NoError(t, store.Delete("f2"))
	flows, err = store.ListScheduled()
	require.NoError(t, err)
	assert.Empty(t, flows)

	kv.mu.Lock()
	_, exists := kv.data[scheduleIndexKey]
	kv.mu.Unlock()
	assert.False(t, exists)
}

func TestStore_ScheduleIndex_MessagePostedExcluded(t *testing.T) {
	store, _ := setupStore(t)

	require.NoError(t, store.Save(&model.Flow{ID: "f1", Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}}}))

	flows, err := store.ListScheduled()
	require.NoError(t, err)
	assert.Empty(t, flows)
}

func TestStore_ScheduleIndex_DisabledIncluded(t *testing.T) {
	store, _ := setupStore(t)

	require.NoError(t, store.Save(&model.Flow{
		ID:      "f1",
		Enabled: false,
		Trigger: model.Trigger{Schedule: &model.ScheduleConfig{ChannelID: "ch1", Interval: "1h"}},
	}))

	flows, err := store.ListScheduled()
	require.NoError(t, err)
	require.Len(t, flows, 1)
	assert.Equal(t, "f1", flows[0].ID)
}

func TestStore_ScheduleIndex_TriggerTypeChange(t *testing.T) {
	store, _ := setupStore(t)

	// Start with a schedule flow.
	require.NoError(t, store.Save(&model.Flow{ID: "f1", Trigger: model.Trigger{Schedule: &model.ScheduleConfig{ChannelID: "ch1", Interval: "1h"}}}))

	flows, err := store.ListScheduled()
	require.NoError(t, err)
	require.Len(t, flows, 1)

	// Change to message_posted — should remove from schedule index.
	require.NoError(t, store.Save(&model.Flow{ID: "f1", Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}}}))

	flows, err = store.ListScheduled()
	require.NoError(t, err)
	assert.Empty(t, flows)

	// Change back to schedule — should add to schedule index.
	require.NoError(t, store.Save(&model.Flow{ID: "f1", Trigger: model.Trigger{Schedule: &model.ScheduleConfig{ChannelID: "ch1", Interval: "30m"}}}))

	flows, err = store.ListScheduled()
	require.NoError(t, err)
	require.Len(t, flows, 1)
	assert.Equal(t, "f1", flows[0].ID)
}

func TestStore_ScheduleIndex_NoDuplicates(t *testing.T) {
	store, kv := setupStore(t)

	// Save the same schedule flow twice.
	require.NoError(t, store.Save(&model.Flow{ID: "f1", Trigger: model.Trigger{Schedule: &model.ScheduleConfig{ChannelID: "ch1", Interval: "1h"}}}))
	require.NoError(t, store.Save(&model.Flow{ID: "f1", Trigger: model.Trigger{Schedule: &model.ScheduleConfig{ChannelID: "ch1", Interval: "30m"}}}))

	flows, err := store.ListScheduled()
	require.NoError(t, err)
	require.Len(t, flows, 1)
	assert.Equal(t, "f1", flows[0].ID)

	// Verify the raw index has exactly one entry.
	kv.mu.Lock()
	indexData := kv.data[scheduleIndexKey]
	kv.mu.Unlock()

	var ids []string
	require.NoError(t, json.Unmarshal(indexData, &ids))
	assert.Equal(t, []string{"f1"}, ids)
}

func TestStore_MembershipTriggerIndex(t *testing.T) {
	store, kv := setupStore(t)
	kvStore := store.(*KVStore)

	require.NoError(t, store.Save(&model.Flow{ID: "f1", Trigger: model.Trigger{MembershipChanged: &model.MembershipChangedConfig{ChannelID: "ch1"}}}))
	require.NoError(t, store.Save(&model.Flow{ID: "f2", Trigger: model.Trigger{MembershipChanged: &model.MembershipChangedConfig{ChannelID: "ch1"}}}))

	ids, err := kvStore.GetFlowIDsForMembershipChannel("ch1")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"f1", "f2"}, ids)

	require.NoError(t, store.Delete("f1"))
	ids, err = kvStore.GetFlowIDsForMembershipChannel("ch1")
	require.NoError(t, err)
	assert.Equal(t, []string{"f2"}, ids)

	require.NoError(t, store.Delete("f2"))
	ids, err = kvStore.GetFlowIDsForMembershipChannel("ch1")
	require.NoError(t, err)
	assert.Nil(t, ids)

	// Key should be cleaned up from KV store.
	kv.mu.Lock()
	_, exists := kv.data[makeMembershipTriggerIndexKey("ch1")]
	kv.mu.Unlock()
	assert.False(t, exists)
}

func TestStore_MembershipTriggerIndex_UpdateChannel(t *testing.T) {
	store, _ := setupStore(t)
	kvStore := store.(*KVStore)

	require.NoError(t, store.Save(&model.Flow{ID: "f1", Trigger: model.Trigger{MembershipChanged: &model.MembershipChangedConfig{ChannelID: "ch1"}}}))

	ids, err := kvStore.GetFlowIDsForMembershipChannel("ch1")
	require.NoError(t, err)
	assert.Equal(t, []string{"f1"}, ids)

	// Update flow to watch ch2.
	require.NoError(t, store.Save(&model.Flow{ID: "f1", Trigger: model.Trigger{MembershipChanged: &model.MembershipChangedConfig{ChannelID: "ch2"}}}))

	ids, err = kvStore.GetFlowIDsForMembershipChannel("ch1")
	require.NoError(t, err)
	assert.Nil(t, ids)

	ids, err = kvStore.GetFlowIDsForMembershipChannel("ch2")
	require.NoError(t, err)
	assert.Equal(t, []string{"f1"}, ids)
}

func TestStore_MembershipTriggerIndex_NoDuplicates(t *testing.T) {
	store, _ := setupStore(t)
	kvStore := store.(*KVStore)

	// Save the same flow twice (simulating update with same channel).
	require.NoError(t, store.Save(&model.Flow{ID: "f1", Trigger: model.Trigger{MembershipChanged: &model.MembershipChangedConfig{ChannelID: "ch1"}}}))
	require.NoError(t, store.Save(&model.Flow{ID: "f1", Trigger: model.Trigger{MembershipChanged: &model.MembershipChangedConfig{ChannelID: "ch1"}}}))

	ids, err := kvStore.GetFlowIDsForMembershipChannel("ch1")
	require.NoError(t, err)
	assert.Equal(t, []string{"f1"}, ids)
}

func TestStore_MembershipTriggerIndex_CrossTypeIsolation(t *testing.T) {
	store, _ := setupStore(t)
	kvStore := store.(*KVStore)

	// Save a membership flow and a message_posted flow on the same channel.
	require.NoError(t, store.Save(&model.Flow{ID: "f1", Trigger: model.Trigger{MembershipChanged: &model.MembershipChangedConfig{ChannelID: "ch1"}}}))
	require.NoError(t, store.Save(&model.Flow{ID: "f2", Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}}}))

	// Membership index should only contain f1.
	memberIDs, err := kvStore.GetFlowIDsForMembershipChannel("ch1")
	require.NoError(t, err)
	assert.Equal(t, []string{"f1"}, memberIDs)

	// Message posted index should only contain f2.
	postIDs, err := kvStore.GetFlowIDsForChannel("ch1")
	require.NoError(t, err)
	assert.Equal(t, []string{"f2"}, postIDs)
}

func TestStore_ChannelTriggerIndex_MembershipChangedTrigger(t *testing.T) {
	store, _ := setupStore(t)

	require.NoError(t, store.Save(&model.Flow{ID: "f1", Trigger: model.Trigger{MembershipChanged: &model.MembershipChangedConfig{ChannelID: "ch1"}}}))

	flows, err := store.ListByTriggerChannel("ch1")
	require.NoError(t, err)
	require.Len(t, flows, 1)
	assert.Equal(t, "f1", flows[0].ID)

	// Verify that the message_posted trigger index does NOT contain this flow.
	kvStore := store.(*KVStore)
	ids, err := kvStore.GetFlowIDsForChannel("ch1")
	require.NoError(t, err)
	assert.Empty(t, ids)
}

func TestStore_ChannelCreatedIndex(t *testing.T) {
	store, kv := setupStore(t)
	kvStore := store.(*KVStore)

	require.NoError(t, store.Save(&model.Flow{ID: "f1", Trigger: model.Trigger{ChannelCreated: &model.ChannelCreatedConfig{}}}))
	require.NoError(t, store.Save(&model.Flow{ID: "f2", Trigger: model.Trigger{ChannelCreated: &model.ChannelCreatedConfig{}}}))

	ids, err := kvStore.GetChannelCreatedFlowIDs()
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"f1", "f2"}, ids)

	require.NoError(t, store.Delete("f1"))
	ids, err = kvStore.GetChannelCreatedFlowIDs()
	require.NoError(t, err)
	assert.Equal(t, []string{"f2"}, ids)

	require.NoError(t, store.Delete("f2"))
	ids, err = kvStore.GetChannelCreatedFlowIDs()
	require.NoError(t, err)
	assert.Nil(t, ids)

	// Key should be cleaned up from KV store.
	kv.mu.Lock()
	_, exists := kv.data[channelCreatedIndexKey]
	kv.mu.Unlock()
	assert.False(t, exists)
}

func TestStore_ChannelCreatedIndex_NoDuplicates(t *testing.T) {
	store, kv := setupStore(t)

	// Save the same channel_created flow twice.
	require.NoError(t, store.Save(&model.Flow{ID: "f1", Trigger: model.Trigger{ChannelCreated: &model.ChannelCreatedConfig{}}}))
	require.NoError(t, store.Save(&model.Flow{ID: "f1", Trigger: model.Trigger{ChannelCreated: &model.ChannelCreatedConfig{}}}))

	kvStore := store.(*KVStore)
	ids, err := kvStore.GetChannelCreatedFlowIDs()
	require.NoError(t, err)
	assert.Equal(t, []string{"f1"}, ids)

	// Verify the raw index has exactly one entry.
	kv.mu.Lock()
	indexData := kv.data[channelCreatedIndexKey]
	kv.mu.Unlock()

	var rawIDs []string
	require.NoError(t, json.Unmarshal(indexData, &rawIDs))
	assert.Equal(t, []string{"f1"}, rawIDs)
}

func TestStore_ChannelCreatedIndex_TriggerTypeChange(t *testing.T) {
	store, _ := setupStore(t)
	kvStore := store.(*KVStore)

	// Start with a channel_created flow.
	require.NoError(t, store.Save(&model.Flow{ID: "f1", Trigger: model.Trigger{ChannelCreated: &model.ChannelCreatedConfig{}}}))

	ids, err := kvStore.GetChannelCreatedFlowIDs()
	require.NoError(t, err)
	assert.Equal(t, []string{"f1"}, ids)

	// Change to message_posted — should remove from channel_created index.
	require.NoError(t, store.Save(&model.Flow{ID: "f1", Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}}}))

	ids, err = kvStore.GetChannelCreatedFlowIDs()
	require.NoError(t, err)
	assert.Nil(t, ids)

	// Change back to channel_created — should add to channel_created index.
	require.NoError(t, store.Save(&model.Flow{ID: "f1", Trigger: model.Trigger{ChannelCreated: &model.ChannelCreatedConfig{}}}))

	ids, err = kvStore.GetChannelCreatedFlowIDs()
	require.NoError(t, err)
	assert.Equal(t, []string{"f1"}, ids)
}

func TestStore_ChannelCreatedIndex_NoChannelTriggerIndex(t *testing.T) {
	store, _ := setupStore(t)

	// channel_created flows should NOT appear in the channel-trigger index (they have no channel ID).
	require.NoError(t, store.Save(&model.Flow{ID: "f1", Trigger: model.Trigger{ChannelCreated: &model.ChannelCreatedConfig{}}}))

	flows, err := store.ListByTriggerChannel("any")
	require.NoError(t, err)
	assert.Empty(t, flows)
}

func TestStore_FlowIndex_Consistency(t *testing.T) {
	store, kv := setupStore(t)

	require.NoError(t, store.Save(&model.Flow{ID: "f1", Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}}}))
	require.NoError(t, store.Save(&model.Flow{ID: "f2", Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch2"}}}))
	require.NoError(t, store.Save(&model.Flow{ID: "f3", Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}}}))
	require.NoError(t, store.Delete("f2"))

	kv.mu.Lock()
	indexData := kv.data[flowIndexKey]
	kv.mu.Unlock()

	var ids []string
	require.NoError(t, json.Unmarshal(indexData, &ids))
	assert.ElementsMatch(t, []string{"f1", "f3"}, ids)
}
