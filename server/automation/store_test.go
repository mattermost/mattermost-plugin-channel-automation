package automation

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
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

	f := &model.Automation{
		ID:      "auto1",
		Name:    "Test Automation",
		Enabled: true,
		Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
		Actions: []model.Action{
			{ID: "act1", SendMessage: &model.SendMessageActionConfig{ChannelID: "ch2", Body: "hello"}},
		},
	}

	require.NoError(t, store.Save(f))

	got, err := store.Get("auto1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "auto1", got.ID)
	assert.Equal(t, "Test Automation", got.Name)
	assert.True(t, got.Enabled)
	assert.Equal(t, model.TriggerTypeMessagePosted, got.Trigger.Type())
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

	automations, err := store.List()
	require.NoError(t, err)
	assert.Empty(t, automations)

	require.NoError(t, store.Save(&model.Automation{ID: "f1", Name: "Automation 1", Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}}}))
	require.NoError(t, store.Save(&model.Automation{ID: "f2", Name: "Automation 2", Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch2"}}}))

	automations, err = store.List()
	require.NoError(t, err)
	require.Len(t, automations, 2)
	assert.Equal(t, "f1", automations[0].ID)
	assert.Equal(t, "f2", automations[1].ID)
}

func TestStore_Delete(t *testing.T) {
	store, _ := setupStore(t)

	require.NoError(t, store.Save(&model.Automation{ID: "f1", Name: "Automation 1", Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}}}))

	got, err := store.Get("f1")
	require.NoError(t, err)
	require.NotNil(t, got)

	require.NoError(t, store.Delete("f1"))

	got, err = store.Get("f1")
	require.NoError(t, err)
	assert.Nil(t, got)

	automations, err := store.List()
	require.NoError(t, err)
	assert.Empty(t, automations)
}

func TestStore_DeleteNonExistent(t *testing.T) {
	store, _ := setupStore(t)
	require.NoError(t, store.Delete("nonexistent"))
}

func TestStore_TriggerIndex(t *testing.T) {
	store, kv := setupStore(t)
	kvStore := store.(*KVStore)

	require.NoError(t, store.Save(&model.Automation{ID: "f1", Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}}}))
	require.NoError(t, store.Save(&model.Automation{ID: "f2", Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}}}))

	ids, err := kvStore.GetAutomationIDsForChannel("ch1")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"f1", "f2"}, ids)

	require.NoError(t, store.Delete("f1"))
	ids, err = kvStore.GetAutomationIDsForChannel("ch1")
	require.NoError(t, err)
	assert.Equal(t, []string{"f2"}, ids)

	require.NoError(t, store.Delete("f2"))
	ids, err = kvStore.GetAutomationIDsForChannel("ch1")
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

	require.NoError(t, store.Save(&model.Automation{ID: "f1", Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}}}))

	ids, err := kvStore.GetAutomationIDsForChannel("ch1")
	require.NoError(t, err)
	assert.Equal(t, []string{"f1"}, ids)

	// Update automation to watch ch2.
	require.NoError(t, store.Save(&model.Automation{ID: "f1", Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch2"}}}))

	ids, err = kvStore.GetAutomationIDsForChannel("ch1")
	require.NoError(t, err)
	assert.Nil(t, ids)

	ids, err = kvStore.GetAutomationIDsForChannel("ch2")
	require.NoError(t, err)
	assert.Equal(t, []string{"f1"}, ids)
}

func TestStore_TriggerIndex_NoDuplicates(t *testing.T) {
	store, _ := setupStore(t)
	kvStore := store.(*KVStore)

	// Save the same automation twice (simulating update with same channel).
	require.NoError(t, store.Save(&model.Automation{ID: "f1", Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}}}))
	require.NoError(t, store.Save(&model.Automation{ID: "f1", Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}}}))

	ids, err := kvStore.GetAutomationIDsForChannel("ch1")
	require.NoError(t, err)
	assert.Equal(t, []string{"f1"}, ids)
}

func TestStore_ChannelTriggerIndex(t *testing.T) {
	store, _ := setupStore(t)

	// Save automations with both trigger types targeting the same channel.
	require.NoError(t, store.Save(&model.Automation{ID: "f1", Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}}}))
	require.NoError(t, store.Save(&model.Automation{ID: "f2", Trigger: model.Trigger{Schedule: &model.ScheduleConfig{ChannelID: "ch1", Interval: "1h"}}}))
	require.NoError(t, store.Save(&model.Automation{ID: "f3", Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch2"}}}))

	automations, err := store.ListByTriggerChannel("ch1")
	require.NoError(t, err)
	require.Len(t, automations, 2)
	ids := []string{automations[0].ID, automations[1].ID}
	assert.ElementsMatch(t, []string{"f1", "f2"}, ids)

	automations, err = store.ListByTriggerChannel("ch2")
	require.NoError(t, err)
	require.Len(t, automations, 1)
	assert.Equal(t, "f3", automations[0].ID)

	automations, err = store.ListByTriggerChannel("ch-nonexistent")
	require.NoError(t, err)
	assert.Empty(t, automations)
}

func TestStore_ChannelTriggerIndex_UpdateChannel(t *testing.T) {
	store, _ := setupStore(t)

	require.NoError(t, store.Save(&model.Automation{ID: "f1", Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}}}))

	automations, err := store.ListByTriggerChannel("ch1")
	require.NoError(t, err)
	require.Len(t, automations, 1)

	// Update automation to target ch2 instead.
	require.NoError(t, store.Save(&model.Automation{ID: "f1", Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch2"}}}))

	automations, err = store.ListByTriggerChannel("ch1")
	require.NoError(t, err)
	assert.Empty(t, automations)

	automations, err = store.ListByTriggerChannel("ch2")
	require.NoError(t, err)
	require.Len(t, automations, 1)
	assert.Equal(t, "f1", automations[0].ID)
}

func TestStore_ChannelTriggerIndex_Delete(t *testing.T) {
	store, kv := setupStore(t)

	require.NoError(t, store.Save(&model.Automation{ID: "f1", Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}}}))
	require.NoError(t, store.Save(&model.Automation{ID: "f2", Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}}}))

	require.NoError(t, store.Delete("f1"))
	automations, err := store.ListByTriggerChannel("ch1")
	require.NoError(t, err)
	require.Len(t, automations, 1)
	assert.Equal(t, "f2", automations[0].ID)

	require.NoError(t, store.Delete("f2"))
	automations, err = store.ListByTriggerChannel("ch1")
	require.NoError(t, err)
	assert.Empty(t, automations)

	// Key should be cleaned up from KV store.
	kv.mu.Lock()
	_, exists := kv.data[makeChannelTriggerIndexKey("ch1")]
	kv.mu.Unlock()
	assert.False(t, exists)
}

func TestStore_ChannelTriggerIndex_ScheduleTrigger(t *testing.T) {
	store, _ := setupStore(t)

	require.NoError(t, store.Save(&model.Automation{ID: "f1", Trigger: model.Trigger{Schedule: &model.ScheduleConfig{ChannelID: "ch1", Interval: "1h"}}}))

	automations, err := store.ListByTriggerChannel("ch1")
	require.NoError(t, err)
	require.Len(t, automations, 1)
	assert.Equal(t, "f1", automations[0].ID)

	// Verify that the message_posted trigger index does NOT contain this automation.
	kvStore := store.(*KVStore)
	ids, err := kvStore.GetAutomationIDsForChannel("ch1")
	require.NoError(t, err)
	assert.Empty(t, ids)
}

func TestStore_ScheduleIndex(t *testing.T) {
	store, kv := setupStore(t)

	// Save two schedule automations.
	require.NoError(t, store.Save(&model.Automation{ID: "f1", Trigger: model.Trigger{Schedule: &model.ScheduleConfig{ChannelID: "ch1", Interval: "1h"}}}))
	require.NoError(t, store.Save(&model.Automation{ID: "f2", Trigger: model.Trigger{Schedule: &model.ScheduleConfig{ChannelID: "ch2", Interval: "1h"}}}))

	automations, err := store.ListScheduled()
	require.NoError(t, err)
	require.Len(t, automations, 2)
	ids := []string{automations[0].ID, automations[1].ID}
	assert.ElementsMatch(t, []string{"f1", "f2"}, ids)

	// Delete one — returns one.
	require.NoError(t, store.Delete("f1"))
	automations, err = store.ListScheduled()
	require.NoError(t, err)
	require.Len(t, automations, 1)
	assert.Equal(t, "f2", automations[0].ID)

	// Delete both — empty + KV key cleaned up.
	require.NoError(t, store.Delete("f2"))
	automations, err = store.ListScheduled()
	require.NoError(t, err)
	assert.Empty(t, automations)

	kv.mu.Lock()
	_, exists := kv.data[scheduleIndexKey]
	kv.mu.Unlock()
	assert.False(t, exists)
}

func TestStore_ScheduleIndex_MessagePostedExcluded(t *testing.T) {
	store, _ := setupStore(t)

	require.NoError(t, store.Save(&model.Automation{ID: "f1", Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}}}))

	automations, err := store.ListScheduled()
	require.NoError(t, err)
	assert.Empty(t, automations)
}

func TestStore_ScheduleIndex_DisabledIncluded(t *testing.T) {
	store, _ := setupStore(t)

	require.NoError(t, store.Save(&model.Automation{
		ID:      "f1",
		Enabled: false,
		Trigger: model.Trigger{Schedule: &model.ScheduleConfig{ChannelID: "ch1", Interval: "1h"}},
	}))

	automations, err := store.ListScheduled()
	require.NoError(t, err)
	require.Len(t, automations, 1)
	assert.Equal(t, "f1", automations[0].ID)
}

func TestStore_ScheduleIndex_TriggerTypeChange(t *testing.T) {
	store, _ := setupStore(t)

	// Start with a schedule automation.
	require.NoError(t, store.Save(&model.Automation{ID: "f1", Trigger: model.Trigger{Schedule: &model.ScheduleConfig{ChannelID: "ch1", Interval: "1h"}}}))

	automations, err := store.ListScheduled()
	require.NoError(t, err)
	require.Len(t, automations, 1)

	// Change to message_posted — should remove from schedule index.
	require.NoError(t, store.Save(&model.Automation{ID: "f1", Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}}}))

	automations, err = store.ListScheduled()
	require.NoError(t, err)
	assert.Empty(t, automations)

	// Change back to schedule — should add to schedule index.
	require.NoError(t, store.Save(&model.Automation{ID: "f1", Trigger: model.Trigger{Schedule: &model.ScheduleConfig{ChannelID: "ch1", Interval: "1h"}}}))

	automations, err = store.ListScheduled()
	require.NoError(t, err)
	require.Len(t, automations, 1)
	assert.Equal(t, "f1", automations[0].ID)
}

func TestStore_ScheduleIndex_NoDuplicates(t *testing.T) {
	store, kv := setupStore(t)

	// Save the same schedule automation twice with different intervals.
	require.NoError(t, store.Save(&model.Automation{ID: "f1", Trigger: model.Trigger{Schedule: &model.ScheduleConfig{ChannelID: "ch1", Interval: "1h"}}}))
	require.NoError(t, store.Save(&model.Automation{ID: "f1", Trigger: model.Trigger{Schedule: &model.ScheduleConfig{ChannelID: "ch1", Interval: "2h"}}}))

	automations, err := store.ListScheduled()
	require.NoError(t, err)
	require.Len(t, automations, 1)
	assert.Equal(t, "f1", automations[0].ID)

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

	require.NoError(t, store.Save(&model.Automation{ID: "f1", Trigger: model.Trigger{MembershipChanged: &model.MembershipChangedConfig{ChannelID: "ch1"}}}))
	require.NoError(t, store.Save(&model.Automation{ID: "f2", Trigger: model.Trigger{MembershipChanged: &model.MembershipChangedConfig{ChannelID: "ch1"}}}))

	ids, err := kvStore.GetAutomationIDsForMembershipChannel("ch1")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"f1", "f2"}, ids)

	require.NoError(t, store.Delete("f1"))
	ids, err = kvStore.GetAutomationIDsForMembershipChannel("ch1")
	require.NoError(t, err)
	assert.Equal(t, []string{"f2"}, ids)

	require.NoError(t, store.Delete("f2"))
	ids, err = kvStore.GetAutomationIDsForMembershipChannel("ch1")
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

	require.NoError(t, store.Save(&model.Automation{ID: "f1", Trigger: model.Trigger{MembershipChanged: &model.MembershipChangedConfig{ChannelID: "ch1"}}}))

	ids, err := kvStore.GetAutomationIDsForMembershipChannel("ch1")
	require.NoError(t, err)
	assert.Equal(t, []string{"f1"}, ids)

	// Update automation to watch ch2.
	require.NoError(t, store.Save(&model.Automation{ID: "f1", Trigger: model.Trigger{MembershipChanged: &model.MembershipChangedConfig{ChannelID: "ch2"}}}))

	ids, err = kvStore.GetAutomationIDsForMembershipChannel("ch1")
	require.NoError(t, err)
	assert.Nil(t, ids)

	ids, err = kvStore.GetAutomationIDsForMembershipChannel("ch2")
	require.NoError(t, err)
	assert.Equal(t, []string{"f1"}, ids)
}

func TestStore_MembershipTriggerIndex_NoDuplicates(t *testing.T) {
	store, _ := setupStore(t)
	kvStore := store.(*KVStore)

	// Save the same automation twice (simulating update with same channel).
	require.NoError(t, store.Save(&model.Automation{ID: "f1", Trigger: model.Trigger{MembershipChanged: &model.MembershipChangedConfig{ChannelID: "ch1"}}}))
	require.NoError(t, store.Save(&model.Automation{ID: "f1", Trigger: model.Trigger{MembershipChanged: &model.MembershipChangedConfig{ChannelID: "ch1"}}}))

	ids, err := kvStore.GetAutomationIDsForMembershipChannel("ch1")
	require.NoError(t, err)
	assert.Equal(t, []string{"f1"}, ids)
}

func TestStore_MembershipTriggerIndex_CrossTypeIsolation(t *testing.T) {
	store, _ := setupStore(t)
	kvStore := store.(*KVStore)

	// Save a membership automation and a message_posted automation on the same channel.
	require.NoError(t, store.Save(&model.Automation{ID: "f1", Trigger: model.Trigger{MembershipChanged: &model.MembershipChangedConfig{ChannelID: "ch1"}}}))
	require.NoError(t, store.Save(&model.Automation{ID: "f2", Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}}}))

	// Membership index should only contain f1.
	memberIDs, err := kvStore.GetAutomationIDsForMembershipChannel("ch1")
	require.NoError(t, err)
	assert.Equal(t, []string{"f1"}, memberIDs)

	// Message posted index should only contain f2.
	postIDs, err := kvStore.GetAutomationIDsForChannel("ch1")
	require.NoError(t, err)
	assert.Equal(t, []string{"f2"}, postIDs)
}

func TestStore_ChannelTriggerIndex_MembershipChangedTrigger(t *testing.T) {
	store, _ := setupStore(t)

	require.NoError(t, store.Save(&model.Automation{ID: "f1", Trigger: model.Trigger{MembershipChanged: &model.MembershipChangedConfig{ChannelID: "ch1"}}}))

	automations, err := store.ListByTriggerChannel("ch1")
	require.NoError(t, err)
	require.Len(t, automations, 1)
	assert.Equal(t, "f1", automations[0].ID)

	// Verify that the message_posted trigger index does NOT contain this automation.
	kvStore := store.(*KVStore)
	ids, err := kvStore.GetAutomationIDsForChannel("ch1")
	require.NoError(t, err)
	assert.Empty(t, ids)
}

func TestStore_ChannelCreatedIndex(t *testing.T) {
	store, kv := setupStore(t)
	kvStore := store.(*KVStore)

	require.NoError(t, store.Save(&model.Automation{ID: "f1", Trigger: model.Trigger{ChannelCreated: &model.ChannelCreatedConfig{TeamID: "team1"}}}))
	require.NoError(t, store.Save(&model.Automation{ID: "f2", Trigger: model.Trigger{ChannelCreated: &model.ChannelCreatedConfig{TeamID: "team1"}}}))

	ids, err := kvStore.GetChannelCreatedAutomationIDs()
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"f1", "f2"}, ids)

	require.NoError(t, store.Delete("f1"))
	ids, err = kvStore.GetChannelCreatedAutomationIDs()
	require.NoError(t, err)
	assert.Equal(t, []string{"f2"}, ids)

	require.NoError(t, store.Delete("f2"))
	ids, err = kvStore.GetChannelCreatedAutomationIDs()
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

	// Save the same channel_created automation twice.
	require.NoError(t, store.Save(&model.Automation{ID: "f1", Trigger: model.Trigger{ChannelCreated: &model.ChannelCreatedConfig{TeamID: "team1"}}}))
	require.NoError(t, store.Save(&model.Automation{ID: "f1", Trigger: model.Trigger{ChannelCreated: &model.ChannelCreatedConfig{TeamID: "team1"}}}))

	kvStore := store.(*KVStore)
	ids, err := kvStore.GetChannelCreatedAutomationIDs()
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

	// Start with a channel_created automation.
	require.NoError(t, store.Save(&model.Automation{ID: "f1", Trigger: model.Trigger{ChannelCreated: &model.ChannelCreatedConfig{TeamID: "team1"}}}))

	ids, err := kvStore.GetChannelCreatedAutomationIDs()
	require.NoError(t, err)
	assert.Equal(t, []string{"f1"}, ids)

	// Change to message_posted — should remove from channel_created index.
	require.NoError(t, store.Save(&model.Automation{ID: "f1", Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}}}))

	ids, err = kvStore.GetChannelCreatedAutomationIDs()
	require.NoError(t, err)
	assert.Nil(t, ids)

	// Change back to channel_created — should add to channel_created index.
	require.NoError(t, store.Save(&model.Automation{ID: "f1", Trigger: model.Trigger{ChannelCreated: &model.ChannelCreatedConfig{TeamID: "team1"}}}))

	ids, err = kvStore.GetChannelCreatedAutomationIDs()
	require.NoError(t, err)
	assert.Equal(t, []string{"f1"}, ids)
}

func TestStore_ChannelCreatedIndex_NoChannelTriggerIndex(t *testing.T) {
	store, _ := setupStore(t)

	// channel_created automations should NOT appear in the channel-trigger index (they have no channel ID).
	require.NoError(t, store.Save(&model.Automation{ID: "f1", Trigger: model.Trigger{ChannelCreated: &model.ChannelCreatedConfig{TeamID: "team1"}}}))

	automations, err := store.ListByTriggerChannel("any")
	require.NoError(t, err)
	assert.Empty(t, automations)
}

func TestStore_UserJoinedTeamIndex(t *testing.T) {
	store, kv := setupStore(t)
	kvStore := store.(*KVStore)

	require.NoError(t, store.Save(&model.Automation{ID: "f1", Trigger: model.Trigger{UserJoinedTeam: &model.UserJoinedTeamConfig{TeamID: "team1"}}}))
	require.NoError(t, store.Save(&model.Automation{ID: "f2", Trigger: model.Trigger{UserJoinedTeam: &model.UserJoinedTeamConfig{TeamID: "team1"}}}))

	ids, err := kvStore.GetAutomationIDsForUserJoinedTeam("team1")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"f1", "f2"}, ids)

	require.NoError(t, store.Delete("f1"))
	ids, err = kvStore.GetAutomationIDsForUserJoinedTeam("team1")
	require.NoError(t, err)
	assert.Equal(t, []string{"f2"}, ids)

	require.NoError(t, store.Delete("f2"))
	ids, err = kvStore.GetAutomationIDsForUserJoinedTeam("team1")
	require.NoError(t, err)
	assert.Nil(t, ids)

	// Key should be cleaned up from KV store.
	kv.mu.Lock()
	_, exists := kv.data[makeUserJoinedTeamTriggerIndexKey("team1")]
	kv.mu.Unlock()
	assert.False(t, exists)
}

func TestStore_UserJoinedTeamIndex_NoDuplicates(t *testing.T) {
	store, kv := setupStore(t)

	require.NoError(t, store.Save(&model.Automation{ID: "f1", Trigger: model.Trigger{UserJoinedTeam: &model.UserJoinedTeamConfig{TeamID: "team1"}}}))
	require.NoError(t, store.Save(&model.Automation{ID: "f1", Trigger: model.Trigger{UserJoinedTeam: &model.UserJoinedTeamConfig{TeamID: "team1"}}}))

	kvStore := store.(*KVStore)
	ids, err := kvStore.GetAutomationIDsForUserJoinedTeam("team1")
	require.NoError(t, err)
	assert.Equal(t, []string{"f1"}, ids)

	kv.mu.Lock()
	indexData := kv.data[makeUserJoinedTeamTriggerIndexKey("team1")]
	kv.mu.Unlock()

	var rawIDs []string
	require.NoError(t, json.Unmarshal(indexData, &rawIDs))
	assert.Equal(t, []string{"f1"}, rawIDs)
}

func TestStore_UserJoinedTeamIndex_TriggerTypeChange(t *testing.T) {
	store, _ := setupStore(t)
	kvStore := store.(*KVStore)

	require.NoError(t, store.Save(&model.Automation{ID: "f1", Trigger: model.Trigger{UserJoinedTeam: &model.UserJoinedTeamConfig{TeamID: "team1"}}}))

	ids, err := kvStore.GetAutomationIDsForUserJoinedTeam("team1")
	require.NoError(t, err)
	assert.Equal(t, []string{"f1"}, ids)

	// Change to message_posted — should remove from user_joined_team index.
	require.NoError(t, store.Save(&model.Automation{ID: "f1", Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}}}))

	ids, err = kvStore.GetAutomationIDsForUserJoinedTeam("team1")
	require.NoError(t, err)
	assert.Nil(t, ids)

	// Change back to user_joined_team — should add to index.
	require.NoError(t, store.Save(&model.Automation{ID: "f1", Trigger: model.Trigger{UserJoinedTeam: &model.UserJoinedTeamConfig{TeamID: "team1"}}}))

	ids, err = kvStore.GetAutomationIDsForUserJoinedTeam("team1")
	require.NoError(t, err)
	assert.Equal(t, []string{"f1"}, ids)
}

func TestStore_UserJoinedTeamIndex_DifferentTeams(t *testing.T) {
	store, _ := setupStore(t)
	kvStore := store.(*KVStore)

	require.NoError(t, store.Save(&model.Automation{ID: "f1", Trigger: model.Trigger{UserJoinedTeam: &model.UserJoinedTeamConfig{TeamID: "team1"}}}))
	require.NoError(t, store.Save(&model.Automation{ID: "f2", Trigger: model.Trigger{UserJoinedTeam: &model.UserJoinedTeamConfig{TeamID: "team2"}}}))

	ids, err := kvStore.GetAutomationIDsForUserJoinedTeam("team1")
	require.NoError(t, err)
	assert.Equal(t, []string{"f1"}, ids)

	ids, err = kvStore.GetAutomationIDsForUserJoinedTeam("team2")
	require.NoError(t, err)
	assert.Equal(t, []string{"f2"}, ids)
}

func TestStore_UserJoinedTeamIndex_UpdateTeam(t *testing.T) {
	store, _ := setupStore(t)
	kvStore := store.(*KVStore)

	require.NoError(t, store.Save(&model.Automation{ID: "f1", Trigger: model.Trigger{UserJoinedTeam: &model.UserJoinedTeamConfig{TeamID: "team1"}}}))

	ids, err := kvStore.GetAutomationIDsForUserJoinedTeam("team1")
	require.NoError(t, err)
	assert.Equal(t, []string{"f1"}, ids)

	// Update automation to watch team2.
	require.NoError(t, store.Save(&model.Automation{ID: "f1", Trigger: model.Trigger{UserJoinedTeam: &model.UserJoinedTeamConfig{TeamID: "team2"}}}))

	ids, err = kvStore.GetAutomationIDsForUserJoinedTeam("team1")
	require.NoError(t, err)
	assert.Nil(t, ids)

	ids, err = kvStore.GetAutomationIDsForUserJoinedTeam("team2")
	require.NoError(t, err)
	assert.Equal(t, []string{"f1"}, ids)
}

func TestStore_UserJoinedTeamIndex_NoChannelTriggerIndex(t *testing.T) {
	store, _ := setupStore(t)

	// user_joined_team automations should NOT appear in the channel-trigger index.
	require.NoError(t, store.Save(&model.Automation{ID: "f1", Trigger: model.Trigger{UserJoinedTeam: &model.UserJoinedTeamConfig{TeamID: "team1"}}}))

	automations, err := store.ListByTriggerChannel("any")
	require.NoError(t, err)
	assert.Empty(t, automations)
}

func TestStore_AutomationIndex_Consistency(t *testing.T) {
	store, kv := setupStore(t)

	require.NoError(t, store.Save(&model.Automation{ID: "f1", Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}}}))
	require.NoError(t, store.Save(&model.Automation{ID: "f2", Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch2"}}}))
	require.NoError(t, store.Save(&model.Automation{ID: "f3", Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}}}))
	require.NoError(t, store.Delete("f2"))

	kv.mu.Lock()
	indexData := kv.data[automationIndexKey]
	kv.mu.Unlock()

	var ids []string
	require.NoError(t, json.Unmarshal(indexData, &ids))
	assert.ElementsMatch(t, []string{"f1", "f3"}, ids)
}

func TestStore_CountByTriggerChannel(t *testing.T) {
	store, _ := setupStore(t)

	// Empty channel returns 0.
	count, err := store.CountByTriggerChannel("ch1")
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	// Add automations with different trigger types targeting the same channel.
	require.NoError(t, store.Save(&model.Automation{ID: "f1", Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}}}))
	require.NoError(t, store.Save(&model.Automation{ID: "f2", Trigger: model.Trigger{Schedule: &model.ScheduleConfig{ChannelID: "ch1", Interval: "1h"}}}))
	require.NoError(t, store.Save(&model.Automation{ID: "f3", Trigger: model.Trigger{MembershipChanged: &model.MembershipChangedConfig{ChannelID: "ch1"}}}))
	// Different channel should not count.
	require.NoError(t, store.Save(&model.Automation{ID: "f4", Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch2"}}}))

	count, err = store.CountByTriggerChannel("ch1")
	require.NoError(t, err)
	assert.Equal(t, 3, count)

	count, err = store.CountByTriggerChannel("ch2")
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	// Delete one and verify count decreases.
	require.NoError(t, store.Delete("f2"))

	count, err = store.CountByTriggerChannel("ch1")
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

func TestStore_SaveWithChannelLimit_AllowsBelowLimit(t *testing.T) {
	store, _ := setupStore(t)

	const limit = 3
	for i := range limit - 1 {
		a := &model.Automation{
			ID:      fmt.Sprintf("a%d", i),
			Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
		}
		require.NoError(t, store.SaveWithChannelLimit(a, limit, ""))
	}

	// One more brings us exactly to the limit; must still succeed.
	final := &model.Automation{
		ID:      "a-final",
		Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
	}
	require.NoError(t, store.SaveWithChannelLimit(final, limit, ""))

	count, err := store.CountByTriggerChannel("ch1")
	require.NoError(t, err)
	assert.Equal(t, limit, count)
}

func TestStore_SaveWithChannelLimit_RejectsAtLimit(t *testing.T) {
	store, _ := setupStore(t)

	const limit = 2
	for i := range limit {
		a := &model.Automation{
			ID:      fmt.Sprintf("a%d", i),
			Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
		}
		require.NoError(t, store.SaveWithChannelLimit(a, limit, ""))
	}

	// One past the limit must be rejected with the sentinel error.
	overflow := &model.Automation{
		ID:      "a-overflow",
		Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
	}
	err := store.SaveWithChannelLimit(overflow, limit, "")
	require.Error(t, err)
	assert.True(t, errors.Is(err, model.ErrChannelAutomationLimitExceeded), "expected ErrChannelAutomationLimitExceeded, got %v", err)

	// Overflow automation must not be persisted.
	got, err := store.Get("a-overflow")
	require.NoError(t, err)
	assert.Nil(t, got, "rejected automation must not be saved")

	count, err := store.CountByTriggerChannel("ch1")
	require.NoError(t, err)
	assert.Equal(t, limit, count, "channel count must not change after a rejected save")
}

func TestStore_SaveWithChannelLimit_SelfExclusion(t *testing.T) {
	store, _ := setupStore(t)

	const limit = 1
	original := &model.Automation{
		ID:      "a1",
		Name:    "Original",
		Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
	}
	require.NoError(t, store.SaveWithChannelLimit(original, limit, ""))

	// Updating the same automation on the same channel must succeed because
	// the existing record is self-excluded from the count.
	updated := &model.Automation{
		ID:      "a1",
		Name:    "Updated",
		Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
	}
	require.NoError(t, store.SaveWithChannelLimit(updated, limit, "a1"))

	got, err := store.Get("a1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "Updated", got.Name)
}

func TestStore_SaveWithChannelLimit_MovingToDifferentChannel(t *testing.T) {
	store, _ := setupStore(t)

	const limit = 1
	require.NoError(t, store.Save(&model.Automation{
		ID:      "a1",
		Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
	}))
	require.NoError(t, store.Save(&model.Automation{
		ID:      "a2",
		Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch2"}},
	}))

	// Moving a1 onto ch2 (already at limit) must be rejected;
	// self-exclusion does not apply because a1 isn't on ch2.
	moved := &model.Automation{
		ID:      "a1",
		Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch2"}},
	}
	err := store.SaveWithChannelLimit(moved, limit, "a1")
	require.Error(t, err)
	assert.True(t, errors.Is(err, model.ErrChannelAutomationLimitExceeded))
}

func TestStore_SaveWithChannelLimit_NoLimitBypassesCheck(t *testing.T) {
	for _, limit := range []int{0, -1} {
		t.Run(fmt.Sprintf("limit=%d", limit), func(t *testing.T) {
			store, _ := setupStore(t)

			for i := range 5 {
				a := &model.Automation{
					ID:      fmt.Sprintf("a%d", i),
					Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
				}
				require.NoError(t, store.SaveWithChannelLimit(a, limit, ""))
			}

			count, err := store.CountByTriggerChannel("ch1")
			require.NoError(t, err)
			assert.Equal(t, 5, count)
		})
	}
}

func TestStore_SaveWithChannelLimit_NoChannelBypassesCheck(t *testing.T) {
	store, _ := setupStore(t)

	// channel_created automations have no trigger channel; the quota cannot
	// apply regardless of how restrictive the limit is.
	const limit = 1
	for i := range 3 {
		a := &model.Automation{
			ID:      fmt.Sprintf("a%d", i),
			Trigger: model.Trigger{ChannelCreated: &model.ChannelCreatedConfig{TeamID: "team1"}},
		}
		require.NoError(t, store.SaveWithChannelLimit(a, limit, ""))
	}
}

// TestStore_SaveWithChannelLimit_AtomicUnderConcurrency is the regression
// test for the original TOCTOU bug: with a per-channel limit of N and 2N
// concurrent saves on the same channel, exactly N must succeed and N must
// be rejected with the sentinel error.
func TestStore_SaveWithChannelLimit_AtomicUnderConcurrency(t *testing.T) {
	store, _ := setupStore(t)
	const limit = 5
	const goroutines = 2 * limit

	var (
		wg       sync.WaitGroup
		successN int64
		rejectN  int64
		otherN   int64
	)
	wg.Add(goroutines)
	for i := range goroutines {
		go func() {
			defer wg.Done()
			a := &model.Automation{
				ID:      fmt.Sprintf("a%02d", i),
				Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
			}
			err := store.SaveWithChannelLimit(a, limit, "")
			switch {
			case err == nil:
				atomic.AddInt64(&successN, 1)
			case errors.Is(err, model.ErrChannelAutomationLimitExceeded):
				atomic.AddInt64(&rejectN, 1)
			default:
				atomic.AddInt64(&otherN, 1)
			}
		}()
	}
	wg.Wait()

	assert.EqualValues(t, limit, successN, "exactly limit saves must succeed")
	assert.EqualValues(t, goroutines-limit, rejectN, "the rest must hit the sentinel")
	assert.EqualValues(t, 0, otherN, "no other error type is permitted")

	count, err := store.CountByTriggerChannel("ch1")
	require.NoError(t, err)
	assert.Equal(t, limit, count)
}

// TestStore_DeleteSaveWithChannelLimit_NoStaleIndex covers the Delete /
// SaveWithChannelLimit race: a concurrent Delete must not leave a stale
// channel-trigger index entry that causes SaveWithChannelLimit to spuriously
// reject a save below the limit.
func TestStore_DeleteSaveWithChannelLimit_NoStaleIndex(t *testing.T) {
	const limit = 1
	const iterations = 200

	for i := range iterations {
		store, _ := setupStore(t)
		require.NoError(t, store.Save(&model.Automation{
			ID:      "a1",
			Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
		}))

		var (
			wg          sync.WaitGroup
			deleteErr   error
			saveErr     error
			afterDelete int
		)
		wg.Add(2)
		go func() {
			defer wg.Done()
			deleteErr = store.Delete("a1")
		}()
		go func() {
			defer wg.Done()
			a := &model.Automation{
				ID:      "a2",
				Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
			}
			saveErr = store.SaveWithChannelLimit(a, limit, "")
		}()
		wg.Wait()

		require.NoError(t, deleteErr, "iteration %d", i)

		// After both finish, ch1 must hold at most one automation ID — no
		// stale entry.
		ids, err := store.(*KVStore).getChannelTriggerIndex("ch1")
		require.NoError(t, err)
		afterDelete = len(ids)
		assert.LessOrEqual(t, afterDelete, limit, "iteration %d: stale index entry, ids=%v", i, ids)

		// Save must either succeed (Delete won the lock first) or be
		// rejected with the sentinel (Save won the lock first while a1
		// was still indexed). It must never return any other error.
		if saveErr != nil {
			assert.True(t, errors.Is(saveErr, model.ErrChannelAutomationLimitExceeded),
				"iteration %d: unexpected error %v", i, saveErr)
		}
	}
}
