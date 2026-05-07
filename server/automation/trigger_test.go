package automation

import (
	"testing"

	mmmodel "github.com/mattermost/mattermost/server/public/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/automation/trigger"
	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

func newTestRegistry() *Registry {
	r := NewRegistry()
	r.RegisterTrigger(&trigger.MessagePostedTrigger{})
	r.RegisterTrigger(&trigger.ScheduleTrigger{})
	r.RegisterTrigger(&trigger.MembershipChangedTrigger{})
	r.RegisterTrigger(&trigger.ChannelCreatedTrigger{})
	r.RegisterTrigger(&trigger.UserJoinedTeamTrigger{})
	return r
}

func newMessagePostedEvent(channelID string) *model.Event {
	return &model.Event{
		Type: model.TriggerTypeMessagePosted,
		Post: &mmmodel.Post{ChannelId: channelID},
	}
}

func newMembershipChangedEvent(channelID, action string) *model.Event {
	return &model.Event{
		Type:             model.TriggerTypeMembershipChanged,
		Channel:          &mmmodel.Channel{Id: channelID},
		MembershipAction: action,
	}
}

func TestTriggerService_FindMatchingAutomations_NoAutomations(t *testing.T) {
	store, _ := setupStore(t)
	svc := NewTriggerService(store, newTestRegistry())

	_, automations, err := svc.FindMatchingAutomations(newMessagePostedEvent("ch-empty"))
	require.NoError(t, err)
	assert.Nil(t, automations)
}

func TestTriggerService_FindMatchingAutomations_UnregisteredHandler(t *testing.T) {
	store, _ := setupStore(t)
	svc := NewTriggerService(store, NewRegistry()) // empty registry

	handler, automations, err := svc.FindMatchingAutomations(newMessagePostedEvent("ch1"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no trigger handler registered")
	assert.Nil(t, handler)
	assert.Nil(t, automations)
}

func TestTriggerService_FindMatchingAutomations_ReturnsHandler(t *testing.T) {
	store, _ := setupStore(t)
	svc := NewTriggerService(store, newTestRegistry())

	require.NoError(t, store.Save(&model.Automation{
		ID:      "f1",
		Name:    "Automation",
		Enabled: true,
		Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
	}))

	handler, _, err := svc.FindMatchingAutomations(newMessagePostedEvent("ch1"))
	require.NoError(t, err)
	require.NotNil(t, handler)
	assert.Equal(t, model.TriggerTypeMessagePosted, handler.Type())
}

func TestTriggerService_FindMatchingAutomations_ReturnsEnabledAutomations(t *testing.T) {
	store, _ := setupStore(t)
	svc := NewTriggerService(store, newTestRegistry())

	require.NoError(t, store.Save(&model.Automation{
		ID:      "f1",
		Name:    "Enabled Automation",
		Enabled: true,
		Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
	}))
	require.NoError(t, store.Save(&model.Automation{
		ID:      "f2",
		Name:    "Also Enabled",
		Enabled: true,
		Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
	}))

	_, automations, err := svc.FindMatchingAutomations(newMessagePostedEvent("ch1"))
	require.NoError(t, err)
	require.Len(t, automations, 2)
	assert.Equal(t, "f1", automations[0].ID)
	assert.Equal(t, "f2", automations[1].ID)
}

func TestTriggerService_FindMatchingAutomations_FiltersDisabled(t *testing.T) {
	store, _ := setupStore(t)
	svc := NewTriggerService(store, newTestRegistry())

	require.NoError(t, store.Save(&model.Automation{
		ID:      "f1",
		Name:    "Enabled",
		Enabled: true,
		Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
	}))
	require.NoError(t, store.Save(&model.Automation{
		ID:      "f2",
		Name:    "Disabled",
		Enabled: false,
		Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
	}))

	_, automations, err := svc.FindMatchingAutomations(newMessagePostedEvent("ch1"))
	require.NoError(t, err)
	require.Len(t, automations, 1)
	assert.Equal(t, "f1", automations[0].ID)
}

func TestTriggerService_FindMatchingAutomations_AllDisabled(t *testing.T) {
	store, _ := setupStore(t)
	svc := NewTriggerService(store, newTestRegistry())

	require.NoError(t, store.Save(&model.Automation{
		ID:      "f1",
		Name:    "Disabled 1",
		Enabled: false,
		Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
	}))
	require.NoError(t, store.Save(&model.Automation{
		ID:      "f2",
		Name:    "Disabled 2",
		Enabled: false,
		Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
	}))

	_, automations, err := svc.FindMatchingAutomations(newMessagePostedEvent("ch1"))
	require.NoError(t, err)
	assert.Nil(t, automations)
}

func TestTriggerService_FindMatchingAutomations_DifferentChannels(t *testing.T) {
	store, _ := setupStore(t)
	svc := NewTriggerService(store, newTestRegistry())

	require.NoError(t, store.Save(&model.Automation{
		ID:      "f1",
		Name:    "Channel 1 Automation",
		Enabled: true,
		Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
	}))
	require.NoError(t, store.Save(&model.Automation{
		ID:      "f2",
		Name:    "Channel 2 Automation",
		Enabled: true,
		Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch2"}},
	}))

	_, automations, err := svc.FindMatchingAutomations(newMessagePostedEvent("ch1"))
	require.NoError(t, err)
	require.Len(t, automations, 1)
	assert.Equal(t, "f1", automations[0].ID)

	_, automations, err = svc.FindMatchingAutomations(newMessagePostedEvent("ch2"))
	require.NoError(t, err)
	require.Len(t, automations, 1)
	assert.Equal(t, "f2", automations[0].ID)
}

func TestTriggerService_FindMatchingAutomations_DeletedAutomationSkipped(t *testing.T) {
	store, _ := setupStore(t)
	svc := NewTriggerService(store, newTestRegistry())

	require.NoError(t, store.Save(&model.Automation{
		ID:      "f1",
		Name:    "Will Delete",
		Enabled: true,
		Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
	}))
	require.NoError(t, store.Save(&model.Automation{
		ID:      "f2",
		Name:    "Stays",
		Enabled: true,
		Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
	}))

	require.NoError(t, store.Delete("f1"))

	_, automations, err := svc.FindMatchingAutomations(newMessagePostedEvent("ch1"))
	require.NoError(t, err)
	require.Len(t, automations, 1)
	assert.Equal(t, "f2", automations[0].ID)
}

func TestTriggerService_FindMatchingAutomations_MembershipChanged(t *testing.T) {
	store, _ := setupStore(t)
	svc := NewTriggerService(store, newTestRegistry())

	require.NoError(t, store.Save(&model.Automation{
		ID:      "f1",
		Name:    "Welcome",
		Enabled: true,
		Trigger: model.Trigger{MembershipChanged: &model.MembershipChangedConfig{ChannelID: "ch1"}},
	}))

	_, automations, err := svc.FindMatchingAutomations(newMembershipChangedEvent("ch1", "joined"))
	require.NoError(t, err)
	require.Len(t, automations, 1)
	assert.Equal(t, "f1", automations[0].ID)
}

func TestTriggerService_FindMatchingAutomations_MembershipChanged_WrongChannel(t *testing.T) {
	store, _ := setupStore(t)
	svc := NewTriggerService(store, newTestRegistry())

	require.NoError(t, store.Save(&model.Automation{
		ID:      "f1",
		Name:    "Welcome",
		Enabled: true,
		Trigger: model.Trigger{MembershipChanged: &model.MembershipChangedConfig{ChannelID: "ch1"}},
	}))

	_, automations, err := svc.FindMatchingAutomations(newMembershipChangedEvent("ch2", "joined"))
	require.NoError(t, err)
	assert.Nil(t, automations)
}

func TestTriggerService_FindMatchingAutomations_MembershipChanged_Disabled(t *testing.T) {
	store, _ := setupStore(t)
	svc := NewTriggerService(store, newTestRegistry())

	require.NoError(t, store.Save(&model.Automation{
		ID:      "f1",
		Name:    "Disabled",
		Enabled: false,
		Trigger: model.Trigger{MembershipChanged: &model.MembershipChangedConfig{ChannelID: "ch1"}},
	}))

	_, automations, err := svc.FindMatchingAutomations(newMembershipChangedEvent("ch1", "joined"))
	require.NoError(t, err)
	assert.Nil(t, automations)
}

func TestTriggerService_FindMatchingAutomations_MembershipChanged_NilChannel(t *testing.T) {
	store, _ := setupStore(t)
	svc := NewTriggerService(store, newTestRegistry())

	_, automations, err := svc.FindMatchingAutomations(&model.Event{Type: model.TriggerTypeMembershipChanged, Channel: nil})
	require.NoError(t, err)
	assert.Nil(t, automations)
}

func newChannelCreatedEvent(channelID, teamID string) *model.Event {
	return &model.Event{
		Type:    model.TriggerTypeChannelCreated,
		Channel: &mmmodel.Channel{Id: channelID, TeamId: teamID},
	}
}

func TestTriggerService_FindMatchingAutomations_ChannelCreated(t *testing.T) {
	store, _ := setupStore(t)
	svc := NewTriggerService(store, newTestRegistry())

	require.NoError(t, store.Save(&model.Automation{
		ID:      "f1",
		Name:    "On Channel Created",
		Enabled: true,
		Trigger: model.Trigger{ChannelCreated: &model.ChannelCreatedConfig{TeamID: "team1"}},
	}))

	_, automations, err := svc.FindMatchingAutomations(newChannelCreatedEvent("any-channel", "team1"))
	require.NoError(t, err)
	require.Len(t, automations, 1)
	assert.Equal(t, "f1", automations[0].ID)
}

func TestTriggerService_FindMatchingAutomations_ChannelCreated_Disabled(t *testing.T) {
	store, _ := setupStore(t)
	svc := NewTriggerService(store, newTestRegistry())

	require.NoError(t, store.Save(&model.Automation{
		ID:      "f1",
		Name:    "Disabled",
		Enabled: false,
		Trigger: model.Trigger{ChannelCreated: &model.ChannelCreatedConfig{TeamID: "team1"}},
	}))

	_, automations, err := svc.FindMatchingAutomations(newChannelCreatedEvent("any-channel", "team1"))
	require.NoError(t, err)
	assert.Nil(t, automations)
}

func TestTriggerService_FindMatchingAutomations_ChannelCreated_NilChannel(t *testing.T) {
	store, _ := setupStore(t)
	svc := NewTriggerService(store, newTestRegistry())

	_, automations, err := svc.FindMatchingAutomations(&model.Event{Type: model.TriggerTypeChannelCreated, Channel: nil})
	require.NoError(t, err)
	assert.Nil(t, automations)
}

func TestTriggerService_FindMatchingAutomations_ChannelCreated_MultipleAutomations(t *testing.T) {
	store, _ := setupStore(t)
	svc := NewTriggerService(store, newTestRegistry())

	require.NoError(t, store.Save(&model.Automation{
		ID:      "f1",
		Name:    "Automation 1",
		Enabled: true,
		Trigger: model.Trigger{ChannelCreated: &model.ChannelCreatedConfig{TeamID: "team1"}},
	}))
	require.NoError(t, store.Save(&model.Automation{
		ID:      "f2",
		Name:    "Automation 2",
		Enabled: true,
		Trigger: model.Trigger{ChannelCreated: &model.ChannelCreatedConfig{TeamID: "team1"}},
	}))

	_, automations, err := svc.FindMatchingAutomations(newChannelCreatedEvent("any-channel", "team1"))
	require.NoError(t, err)
	require.Len(t, automations, 2)
}

func newUserJoinedTeamEvent(teamID string) *model.Event {
	return &model.Event{
		Type: model.TriggerTypeUserJoinedTeam,
		Team: &mmmodel.Team{Id: teamID},
	}
}

func TestTriggerService_FindMatchingAutomations_UserJoinedTeam(t *testing.T) {
	store, _ := setupStore(t)
	svc := NewTriggerService(store, newTestRegistry())

	require.NoError(t, store.Save(&model.Automation{
		ID:      "f1",
		Name:    "On Team Join",
		Enabled: true,
		Trigger: model.Trigger{UserJoinedTeam: &model.UserJoinedTeamConfig{TeamID: "team1"}},
	}))

	_, automations, err := svc.FindMatchingAutomations(newUserJoinedTeamEvent("team1"))
	require.NoError(t, err)
	require.Len(t, automations, 1)
	assert.Equal(t, "f1", automations[0].ID)
}

func TestTriggerService_FindMatchingAutomations_UserJoinedTeam_WrongTeam(t *testing.T) {
	store, _ := setupStore(t)
	svc := NewTriggerService(store, newTestRegistry())

	require.NoError(t, store.Save(&model.Automation{
		ID:      "f1",
		Name:    "On Team Join",
		Enabled: true,
		Trigger: model.Trigger{UserJoinedTeam: &model.UserJoinedTeamConfig{TeamID: "team1"}},
	}))

	_, automations, err := svc.FindMatchingAutomations(newUserJoinedTeamEvent("team2"))
	require.NoError(t, err)
	assert.Nil(t, automations)
}

func TestTriggerService_FindMatchingAutomations_UserJoinedTeam_Disabled(t *testing.T) {
	store, _ := setupStore(t)
	svc := NewTriggerService(store, newTestRegistry())

	require.NoError(t, store.Save(&model.Automation{
		ID:      "f1",
		Name:    "Disabled",
		Enabled: false,
		Trigger: model.Trigger{UserJoinedTeam: &model.UserJoinedTeamConfig{TeamID: "team1"}},
	}))

	_, automations, err := svc.FindMatchingAutomations(newUserJoinedTeamEvent("team1"))
	require.NoError(t, err)
	assert.Nil(t, automations)
}

func TestTriggerService_FindMatchingAutomations_UserJoinedTeam_NilTeam(t *testing.T) {
	store, _ := setupStore(t)
	svc := NewTriggerService(store, newTestRegistry())

	_, automations, err := svc.FindMatchingAutomations(&model.Event{Type: model.TriggerTypeUserJoinedTeam, Team: nil})
	require.NoError(t, err)
	assert.Nil(t, automations)
}

func TestTriggerService_FindMatchingAutomations_UserJoinedTeam_MultipleAutomations(t *testing.T) {
	store, _ := setupStore(t)
	svc := NewTriggerService(store, newTestRegistry())

	require.NoError(t, store.Save(&model.Automation{
		ID:      "f1",
		Name:    "Automation 1",
		Enabled: true,
		Trigger: model.Trigger{UserJoinedTeam: &model.UserJoinedTeamConfig{TeamID: "team1"}},
	}))
	require.NoError(t, store.Save(&model.Automation{
		ID:      "f2",
		Name:    "Automation 2",
		Enabled: true,
		Trigger: model.Trigger{UserJoinedTeam: &model.UserJoinedTeamConfig{TeamID: "team1"}},
	}))

	_, automations, err := svc.FindMatchingAutomations(newUserJoinedTeamEvent("team1"))
	require.NoError(t, err)
	require.Len(t, automations, 2)
}
