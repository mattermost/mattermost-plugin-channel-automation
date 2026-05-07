package notifier

import (
	"testing"
	"time"

	mmmodel "github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin/plugintest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKVCooldownStore_ClaimSucceedsOnFirstWriter(t *testing.T) {
	api := &plugintest.API{}
	expectedOpts := mmmodel.PluginKVSetOptions{
		Atomic:          true,
		OldValue:        nil,
		ExpireInSeconds: 3600,
	}
	api.On("KVSetWithOptions", "automation_failure_notify_flow1", []byte{1}, expectedOpts).
		Return(true, nil)

	s := NewCooldownStore(api, time.Hour)
	ok, err := s.Claim("flow1")

	require.NoError(t, err)
	assert.True(t, ok)
	api.AssertExpectations(t)
}

func TestKVCooldownStore_ClaimReturnsFalseWhenSlotHeld(t *testing.T) {
	api := &plugintest.API{}
	api.On("KVSetWithOptions", "automation_failure_notify_flow1", []byte{1}, mmmodel.PluginKVSetOptions{
		Atomic:          true,
		OldValue:        nil,
		ExpireInSeconds: 3600,
	}).Return(false, nil)

	s := NewCooldownStore(api, time.Hour)
	ok, err := s.Claim("flow1")

	require.NoError(t, err)
	assert.False(t, ok)
	api.AssertExpectations(t)
}

func TestKVCooldownStore_ClaimWrapsKVError(t *testing.T) {
	api := &plugintest.API{}
	appErr := mmmodel.NewAppError("KVSetWithOptions", "kv.fail", nil, "boom", 500)
	api.On("KVSetWithOptions", "automation_failure_notify_flow1", []byte{1}, mmmodel.PluginKVSetOptions{
		Atomic:          true,
		OldValue:        nil,
		ExpireInSeconds: 3600,
	}).Return(false, appErr)

	s := NewCooldownStore(api, time.Hour)
	ok, err := s.Claim("flow1")

	require.Error(t, err)
	assert.False(t, ok)
	assert.Contains(t, err.Error(), "flow1")
	api.AssertExpectations(t)
}

func TestKVCooldownStore_ReleaseDeletesNamespacedKey(t *testing.T) {
	api := &plugintest.API{}
	api.On("KVDelete", "automation_failure_notify_flow1").Return(nil)

	s := NewCooldownStore(api, time.Hour)
	require.NoError(t, s.Release("flow1"))

	api.AssertExpectations(t)
}

func TestKVCooldownStore_ReleaseWrapsKVError(t *testing.T) {
	api := &plugintest.API{}
	appErr := mmmodel.NewAppError("KVDelete", "kv.fail", nil, "boom", 500)
	api.On("KVDelete", "automation_failure_notify_flow1").Return(appErr)

	s := NewCooldownStore(api, time.Hour)
	err := s.Release("flow1")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "flow1")
	api.AssertExpectations(t)
}

func TestKVCooldownStore_ClaimUsesConfiguredTTL(t *testing.T) {
	api := &plugintest.API{}
	api.On("KVSetWithOptions", "automation_failure_notify_flow1", []byte{1}, mmmodel.PluginKVSetOptions{
		Atomic:          true,
		OldValue:        nil,
		ExpireInSeconds: 30,
	}).Return(true, nil)

	s := NewCooldownStore(api, 30*time.Second)
	_, err := s.Claim("flow1")
	require.NoError(t, err)

	api.AssertExpectations(t)
}
