package flow

import (
	"testing"

	mmmodel "github.com/mattermost/mattermost/server/public/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/flow/trigger"
	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

func newTestRegistry() *Registry {
	r := NewRegistry()
	r.RegisterTrigger(&trigger.MessagePostedTrigger{})
	r.RegisterTrigger(&trigger.MembershipChangedTrigger{})
	return r
}

func newMessagePostedEvent(channelID string) *model.Event {
	return &model.Event{
		Type: "message_posted",
		Post: &mmmodel.Post{ChannelId: channelID},
	}
}

func newMembershipChangedEvent(channelID, action string) *model.Event {
	return &model.Event{
		Type:             "membership_changed",
		Channel:          &mmmodel.Channel{Id: channelID},
		MembershipAction: action,
	}
}

func TestTriggerService_FindMatchingFlows_NoFlows(t *testing.T) {
	store, _ := setupStore(t)
	svc := NewTriggerService(store, newTestRegistry())

	flows, err := svc.FindMatchingFlows(newMessagePostedEvent("ch-empty"))
	require.NoError(t, err)
	assert.Nil(t, flows)
}

func TestTriggerService_FindMatchingFlows_ReturnsEnabledFlows(t *testing.T) {
	store, _ := setupStore(t)
	svc := NewTriggerService(store, newTestRegistry())

	require.NoError(t, store.Save(&model.Flow{
		ID:      "f1",
		Name:    "Enabled Flow",
		Enabled: true,
		Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
	}))
	require.NoError(t, store.Save(&model.Flow{
		ID:      "f2",
		Name:    "Also Enabled",
		Enabled: true,
		Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
	}))

	flows, err := svc.FindMatchingFlows(newMessagePostedEvent("ch1"))
	require.NoError(t, err)
	require.Len(t, flows, 2)
	assert.Equal(t, "f1", flows[0].ID)
	assert.Equal(t, "f2", flows[1].ID)
}

func TestTriggerService_FindMatchingFlows_FiltersDisabled(t *testing.T) {
	store, _ := setupStore(t)
	svc := NewTriggerService(store, newTestRegistry())

	require.NoError(t, store.Save(&model.Flow{
		ID:      "f1",
		Name:    "Enabled",
		Enabled: true,
		Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
	}))
	require.NoError(t, store.Save(&model.Flow{
		ID:      "f2",
		Name:    "Disabled",
		Enabled: false,
		Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
	}))

	flows, err := svc.FindMatchingFlows(newMessagePostedEvent("ch1"))
	require.NoError(t, err)
	require.Len(t, flows, 1)
	assert.Equal(t, "f1", flows[0].ID)
}

func TestTriggerService_FindMatchingFlows_AllDisabled(t *testing.T) {
	store, _ := setupStore(t)
	svc := NewTriggerService(store, newTestRegistry())

	require.NoError(t, store.Save(&model.Flow{
		ID:      "f1",
		Name:    "Disabled 1",
		Enabled: false,
		Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
	}))
	require.NoError(t, store.Save(&model.Flow{
		ID:      "f2",
		Name:    "Disabled 2",
		Enabled: false,
		Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
	}))

	flows, err := svc.FindMatchingFlows(newMessagePostedEvent("ch1"))
	require.NoError(t, err)
	assert.Nil(t, flows)
}

func TestTriggerService_FindMatchingFlows_DifferentChannels(t *testing.T) {
	store, _ := setupStore(t)
	svc := NewTriggerService(store, newTestRegistry())

	require.NoError(t, store.Save(&model.Flow{
		ID:      "f1",
		Name:    "Channel 1 Flow",
		Enabled: true,
		Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
	}))
	require.NoError(t, store.Save(&model.Flow{
		ID:      "f2",
		Name:    "Channel 2 Flow",
		Enabled: true,
		Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch2"}},
	}))

	flows, err := svc.FindMatchingFlows(newMessagePostedEvent("ch1"))
	require.NoError(t, err)
	require.Len(t, flows, 1)
	assert.Equal(t, "f1", flows[0].ID)

	flows, err = svc.FindMatchingFlows(newMessagePostedEvent("ch2"))
	require.NoError(t, err)
	require.Len(t, flows, 1)
	assert.Equal(t, "f2", flows[0].ID)
}

func TestTriggerService_FindMatchingFlows_DeletedFlowSkipped(t *testing.T) {
	store, _ := setupStore(t)
	svc := NewTriggerService(store, newTestRegistry())

	require.NoError(t, store.Save(&model.Flow{
		ID:      "f1",
		Name:    "Will Delete",
		Enabled: true,
		Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
	}))
	require.NoError(t, store.Save(&model.Flow{
		ID:      "f2",
		Name:    "Stays",
		Enabled: true,
		Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
	}))

	require.NoError(t, store.Delete("f1"))

	flows, err := svc.FindMatchingFlows(newMessagePostedEvent("ch1"))
	require.NoError(t, err)
	require.Len(t, flows, 1)
	assert.Equal(t, "f2", flows[0].ID)
}

func TestTriggerService_FindMatchingFlows_MembershipChanged(t *testing.T) {
	store, _ := setupStore(t)
	svc := NewTriggerService(store, newTestRegistry())

	require.NoError(t, store.Save(&model.Flow{
		ID:      "f1",
		Name:    "Welcome",
		Enabled: true,
		Trigger: model.Trigger{MembershipChanged: &model.MembershipChangedConfig{ChannelID: "ch1"}},
	}))

	flows, err := svc.FindMatchingFlows(newMembershipChangedEvent("ch1", "joined"))
	require.NoError(t, err)
	require.Len(t, flows, 1)
	assert.Equal(t, "f1", flows[0].ID)
}

func TestTriggerService_FindMatchingFlows_MembershipChanged_WrongChannel(t *testing.T) {
	store, _ := setupStore(t)
	svc := NewTriggerService(store, newTestRegistry())

	require.NoError(t, store.Save(&model.Flow{
		ID:      "f1",
		Name:    "Welcome",
		Enabled: true,
		Trigger: model.Trigger{MembershipChanged: &model.MembershipChangedConfig{ChannelID: "ch1"}},
	}))

	flows, err := svc.FindMatchingFlows(newMembershipChangedEvent("ch2", "joined"))
	require.NoError(t, err)
	assert.Nil(t, flows)
}

func TestTriggerService_FindMatchingFlows_MembershipChanged_Disabled(t *testing.T) {
	store, _ := setupStore(t)
	svc := NewTriggerService(store, newTestRegistry())

	require.NoError(t, store.Save(&model.Flow{
		ID:      "f1",
		Name:    "Disabled",
		Enabled: false,
		Trigger: model.Trigger{MembershipChanged: &model.MembershipChangedConfig{ChannelID: "ch1"}},
	}))

	flows, err := svc.FindMatchingFlows(newMembershipChangedEvent("ch1", "joined"))
	require.NoError(t, err)
	assert.Nil(t, flows)
}

func TestTriggerService_FindMatchingFlows_MembershipChanged_NilChannel(t *testing.T) {
	store, _ := setupStore(t)
	svc := NewTriggerService(store, newTestRegistry())

	flows, err := svc.FindMatchingFlows(&model.Event{Type: "membership_changed", Channel: nil})
	require.NoError(t, err)
	assert.Nil(t, flows)
}
