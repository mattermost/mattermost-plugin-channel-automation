package flow

import (
	"errors"
	"testing"

	mmmodel "github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin/plugintest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

type fakeEnqueuer struct {
	items []*model.WorkItem
	// err, if set, fails every Enqueue.
	err error
	// failOn, if non-nil, returns the error to use on the Nth call (0-indexed)
	// or nil to succeed. Takes precedence over err.
	failOn func(callIdx int) error
	calls  int
}

func (e *fakeEnqueuer) Enqueue(item *model.WorkItem) error {
	idx := e.calls
	e.calls++
	if e.failOn != nil {
		if err := e.failOn(idx); err != nil {
			return err
		}
	} else if e.err != nil {
		return e.err
	}
	e.items = append(e.items, item)
	return nil
}

type fakeNotifier struct {
	called int
}

func (n *fakeNotifier) Notify() { n.called++ }

func setupDispatcher(t *testing.T) (*Dispatcher, model.Store, *fakeEnqueuer, *fakeNotifier, *plugintest.API) {
	t.Helper()

	store, _ := setupStore(t)
	registry := newTestRegistry()
	api := &plugintest.API{}
	for _, name := range []string{"LogError", "LogDebug", "LogWarn", "LogInfo"} {
		for args := 1; args <= 11; args += 2 {
			anys := make([]any, args)
			for i := range anys {
				anys[i] = mock.Anything
			}
			api.On(name, anys...).Maybe()
		}
	}

	triggerSvc := NewTriggerService(store, registry)
	enqueuer := &fakeEnqueuer{}
	notifier := &fakeNotifier{}

	d := NewDispatcher(api, triggerSvc, enqueuer, notifier)
	return d, store, enqueuer, notifier, api
}

func TestDispatcher_NoMatchingFlows_IsSilent(t *testing.T) {
	d, _, enqueuer, notifier, _ := setupDispatcher(t)

	d.Dispatch(&model.Event{
		Type: model.TriggerTypeMessagePosted,
		Post: &mmmodel.Post{ChannelId: "no-such-channel", UserId: "u1"},
	})

	assert.Empty(t, enqueuer.items)
	assert.Zero(t, notifier.called)
}

func TestDispatcher_EnqueuesOnePerMatchingFlow(t *testing.T) {
	d, store, enqueuer, notifier, api := setupDispatcher(t)

	require.NoError(t, store.Save(&model.Flow{
		ID:      "f1",
		Name:    "Flow 1",
		Enabled: true,
		Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
	}))
	require.NoError(t, store.Save(&model.Flow{
		ID:      "f2",
		Name:    "Flow 2",
		Enabled: true,
		Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
	}))

	api.On("GetChannel", "ch1").Return(&mmmodel.Channel{Id: "ch1", Name: "n"}, (*mmmodel.AppError)(nil))
	api.On("GetUser", "u1").Return(&mmmodel.User{Id: "u1", Username: "alice"}, (*mmmodel.AppError)(nil))

	d.Dispatch(&model.Event{
		Type: model.TriggerTypeMessagePosted,
		Post: &mmmodel.Post{Id: "p1", ChannelId: "ch1", UserId: "u1"},
	})

	require.Len(t, enqueuer.items, 2)
	assert.Equal(t, "f1", enqueuer.items[0].FlowID)
	assert.Equal(t, "f2", enqueuer.items[1].FlowID)
	for _, item := range enqueuer.items {
		require.NotNil(t, item.TriggerData.Post)
		require.NotNil(t, item.TriggerData.Channel)
		require.NotNil(t, item.TriggerData.User)
	}
	assert.Equal(t, 1, notifier.called)
}

func TestDispatcher_BuildTriggerDataErrorAborts(t *testing.T) {
	d, store, enqueuer, notifier, api := setupDispatcher(t)

	require.NoError(t, store.Save(&model.Flow{
		ID:      "f1",
		Name:    "Flow 1",
		Enabled: true,
		Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
	}))

	api.On("GetChannel", "ch1").Return((*mmmodel.Channel)(nil), mmmodel.NewAppError("", "", nil, "boom", 500))

	d.Dispatch(&model.Event{
		Type: model.TriggerTypeMessagePosted,
		Post: &mmmodel.Post{Id: "p1", ChannelId: "ch1", UserId: "u1"},
	})

	assert.Empty(t, enqueuer.items)
	assert.Zero(t, notifier.called)
}

func TestDispatcher_EnqueueFailureSkipsItemButNotifies(t *testing.T) {
	_, store, _, _, api := setupDispatcher(t)

	require.NoError(t, store.Save(&model.Flow{
		ID:      "f1",
		Name:    "Flow 1",
		Enabled: true,
		Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
	}))

	api.On("GetChannel", "ch1").Return(&mmmodel.Channel{Id: "ch1"}, (*mmmodel.AppError)(nil))
	api.On("GetUser", "u1").Return(&mmmodel.User{Id: "u1"}, (*mmmodel.AppError)(nil))

	enqueuer := &fakeEnqueuer{err: errors.New("queue full")}
	notifier := &fakeNotifier{}
	d := NewDispatcher(api, NewTriggerService(store, newTestRegistry()), enqueuer, notifier)

	d.Dispatch(&model.Event{
		Type: model.TriggerTypeMessagePosted,
		Post: &mmmodel.Post{Id: "p1", ChannelId: "ch1", UserId: "u1"},
	})

	assert.Empty(t, enqueuer.items)
	// Notify is still called so the pool wakes for any items that were persisted
	// by concurrent producers; enqueue failure is logged per-flow but does not
	// abort the batch.
	assert.Equal(t, 1, notifier.called)
}

// TestDispatcher_PartialEnqueueFailure verifies the per-flow loop's continue
// semantics: a failure on one flow must not abort the batch.
func TestDispatcher_PartialEnqueueFailure(t *testing.T) {
	_, store, _, _, api := setupDispatcher(t)

	for _, id := range []string{"f1", "f2", "f3"} {
		require.NoError(t, store.Save(&model.Flow{
			ID:      id,
			Name:    id,
			Enabled: true,
			Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
		}))
	}

	api.On("GetChannel", "ch1").Return(&mmmodel.Channel{Id: "ch1"}, (*mmmodel.AppError)(nil))
	api.On("GetUser", "u1").Return(&mmmodel.User{Id: "u1"}, (*mmmodel.AppError)(nil))

	enqueuer := &fakeEnqueuer{
		failOn: func(idx int) error {
			if idx == 1 {
				return errors.New("queue full")
			}
			return nil
		},
	}
	notifier := &fakeNotifier{}
	d := NewDispatcher(api, NewTriggerService(store, newTestRegistry()), enqueuer, notifier)

	d.Dispatch(&model.Event{
		Type: model.TriggerTypeMessagePosted,
		Post: &mmmodel.Post{Id: "p1", ChannelId: "ch1", UserId: "u1"},
	})

	require.Len(t, enqueuer.items, 2, "first and third flow should still enqueue despite middle failure")
	assert.Equal(t, "f1", enqueuer.items[0].FlowID)
	assert.Equal(t, "f3", enqueuer.items[1].FlowID)
	assert.Equal(t, 3, enqueuer.calls)
	assert.Equal(t, 1, notifier.called)
}
