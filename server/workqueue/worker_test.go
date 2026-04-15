package workqueue

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	mmmodel "github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin/plugintest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/flow"
	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

// testAction is a controllable action handler for testing.
type testAction struct {
	mu        sync.Mutex
	execCount int
	execFn    func() error
	running   atomic.Int32 // concurrency tracking
	maxSeen   atomic.Int32
}

func (a *testAction) Type() string { return "send_message" }

func (a *testAction) Execute(_ *model.Action, _ *model.FlowContext) (*model.StepOutput, error) {
	cur := a.running.Add(1)
	defer a.running.Add(-1)

	// Track max concurrent
	for {
		old := a.maxSeen.Load()
		if cur <= old || a.maxSeen.CompareAndSwap(old, cur) {
			break
		}
	}

	a.mu.Lock()
	a.execCount++
	fn := a.execFn
	a.mu.Unlock()

	if fn != nil {
		if err := fn(); err != nil {
			return nil, err
		}
	}

	return &model.StepOutput{Message: "done"}, nil
}

func (a *testAction) getExecCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.execCount
}

// testFlowStore is a simple in-memory implementation of model.Store.
type testFlowStore struct {
	mu    sync.Mutex
	flows map[string]*model.Flow
}

func newTestFlowStore() *testFlowStore {
	return &testFlowStore{flows: make(map[string]*model.Flow)}
}

func (s *testFlowStore) Get(id string) (*model.Flow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f := s.flows[id]
	return f, nil
}

func (s *testFlowStore) List() ([]*model.Flow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]*model.Flow, 0, len(s.flows))
	for _, f := range s.flows {
		result = append(result, f)
	}
	return result, nil
}

func (s *testFlowStore) Save(f *model.Flow) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.flows[f.ID] = f
	return nil
}

func (s *testFlowStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.flows, id)
	return nil
}

func (s *testFlowStore) CountByTriggerChannel(_ string) (int, error) {
	return 0, nil
}

func (s *testFlowStore) ListByTriggerChannel(_ string) ([]*model.Flow, error) {
	return nil, nil
}

func (s *testFlowStore) ListScheduled() ([]*model.Flow, error) {
	return nil, nil
}

func (s *testFlowStore) GetFlowIDsForChannel(_ string) ([]string, error) {
	return nil, nil
}

func (s *testFlowStore) GetFlowIDsForMembershipChannel(_ string) ([]string, error) {
	return nil, nil
}

func (s *testFlowStore) GetChannelCreatedFlowIDs() ([]string, error) {
	return nil, nil
}

func setupWorkerPool(t *testing.T, maxWorkers int, act *testAction) (*WorkerPool, *Store, *testFlowStore) {
	t.Helper()

	store, _ := setupStore(t)

	api := &plugintest.API{}
	api.On("LogInfo", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	api.On("LogWarn", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	api.On("LogError", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	api.On("GetUser", mock.Anything).Return(&mmmodel.User{DeleteAt: 0}, nil)
	api.On("HasPermissionTo", mock.Anything, mock.Anything).Return(true)

	registry := flow.NewRegistry()
	registry.RegisterAction(act)
	executor := flow.NewFlowExecutor(registry)

	flowStore := newTestFlowStore()

	wp := NewWorkerPool(store, executor, flowStore, nil, nil, api, maxWorkers)
	wp.pollInterval = 50 * time.Millisecond // speed up tests

	return wp, store, flowStore
}

func TestWorkerPool_ProcessesItems(t *testing.T) {
	act := &testAction{}
	wp, store, flowStore := setupWorkerPool(t, 4, act)

	_ = flowStore.Save(&model.Flow{ID: "f1", Name: "Flow 1", Enabled: true, Actions: []model.Action{{ID: "a1", SendMessage: &model.SendMessageActionConfig{}}}})

	for i := range 3 {
		item := &model.WorkItem{
			ID:       fmt.Sprintf("w%d", i),
			FlowID:   "f1",
			FlowName: "Flow 1",
		}
		require.NoError(t, store.Enqueue(item))
	}

	wp.Start()
	wp.Notify()

	// Wait for items to be processed.
	require.Eventually(t, func() bool {
		return act.getExecCount() == 3
	}, 5*time.Second, 10*time.Millisecond)

	wp.Stop()

	// All items should be completed (deleted from KV).
	for i := range 3 {
		got, err := store.Get(fmt.Sprintf("w%d", i))
		require.NoError(t, err)
		assert.Nil(t, got)
	}
}

func TestWorkerPool_ConcurrencyLimit(t *testing.T) {
	// Use a blocking action to verify concurrency limits.
	proceed := make(chan struct{})
	act := &testAction{
		execFn: func() error {
			<-proceed
			return nil
		},
	}

	wp, store, flowStore := setupWorkerPool(t, 2, act)

	_ = flowStore.Save(&model.Flow{ID: "f1", Name: "Flow 1", Enabled: true, Actions: []model.Action{{ID: "a1", SendMessage: &model.SendMessageActionConfig{}}}})

	for i := range 5 {
		item := &model.WorkItem{
			ID:       fmt.Sprintf("w%d", i),
			FlowID:   "f1",
			FlowName: "Flow 1",
		}
		require.NoError(t, store.Enqueue(item))
	}

	wp.Start()
	wp.Notify()

	// Wait until 2 workers are running.
	require.Eventually(t, func() bool {
		return act.running.Load() == 2
	}, 5*time.Second, 10*time.Millisecond)

	// Max concurrent should be exactly 2.
	assert.Equal(t, int32(2), act.maxSeen.Load())

	// Release all workers.
	close(proceed)

	require.Eventually(t, func() bool {
		return act.getExecCount() == 5
	}, 5*time.Second, 10*time.Millisecond)

	wp.Stop()
}

func TestWorkerPool_GracefulShutdown(t *testing.T) {
	started := make(chan struct{})
	proceed := make(chan struct{})
	act := &testAction{
		execFn: func() error {
			select {
			case started <- struct{}{}:
			default:
			}
			<-proceed
			return nil
		},
	}

	wp, store, flowStore := setupWorkerPool(t, 4, act)

	_ = flowStore.Save(&model.Flow{ID: "f1", Name: "Flow 1", Enabled: true, Actions: []model.Action{{ID: "a1", SendMessage: &model.SendMessageActionConfig{}}}})

	item := &model.WorkItem{ID: "w1", FlowID: "f1", FlowName: "Flow 1"}
	require.NoError(t, store.Enqueue(item))

	wp.Start()
	wp.Notify()

	// Wait for the worker to start.
	<-started

	// Initiate shutdown in a goroutine.
	done := make(chan struct{})
	go func() {
		wp.Stop()
		close(done)
	}()

	// Stop should not return yet because worker is still running.
	select {
	case <-done:
		t.Fatal("Stop returned before worker finished")
	case <-time.After(100 * time.Millisecond):
	}

	// Let the worker finish.
	close(proceed)

	// Now Stop should return.
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop did not return after worker finished")
	}
}

func TestWorkerPool_NotifyWakesDispatcher(t *testing.T) {
	act := &testAction{}
	wp, store, flowStore := setupWorkerPool(t, 4, act)
	wp.pollInterval = 10 * time.Minute // very long poll interval

	_ = flowStore.Save(&model.Flow{ID: "f1", Name: "Flow 1", Enabled: true, Actions: []model.Action{{ID: "a1", SendMessage: &model.SendMessageActionConfig{}}}})

	wp.Start()
	defer wp.Stop()

	item := &model.WorkItem{ID: "w1", FlowID: "f1", FlowName: "Flow 1"}
	require.NoError(t, store.Enqueue(item))

	// Without Notify, the dispatcher won't process until the long poll interval.
	wp.Notify()

	require.Eventually(t, func() bool {
		return act.getExecCount() == 1
	}, 5*time.Second, 10*time.Millisecond)
}

func TestWorkerPool_FailedExecution(t *testing.T) {
	act := &testAction{
		execFn: func() error {
			return fmt.Errorf("action failed")
		},
	}

	wp, store, flowStore := setupWorkerPool(t, 4, act)

	_ = flowStore.Save(&model.Flow{ID: "f1", Name: "Flow 1", Enabled: true, Actions: []model.Action{{ID: "a1", SendMessage: &model.SendMessageActionConfig{}}}})

	item := &model.WorkItem{ID: "w1", FlowID: "f1", FlowName: "Flow 1"}
	require.NoError(t, store.Enqueue(item))

	wp.Start()
	wp.Notify()

	require.Eventually(t, func() bool {
		return act.getExecCount() == 1
	}, 5*time.Second, 10*time.Millisecond)

	wp.Stop()

	// Item should be deleted (failure details live in the execution record).
	got, err := store.Get("w1")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestWorkerPool_DeletedFlow(t *testing.T) {
	act := &testAction{}
	wp, store, flowStore := setupWorkerPool(t, 4, act)

	// Don't add flow to store — it's "deleted"

	item := &model.WorkItem{ID: "w1", FlowID: "f1", FlowName: "Flow 1"}
	require.NoError(t, store.Enqueue(item))

	wp.Start()
	wp.Notify()

	// Wait for processing.
	require.Eventually(t, func() bool {
		got, _ := store.Get("w1")
		return got == nil
	}, 5*time.Second, 10*time.Millisecond)

	wp.Stop()

	// Action should never have been called.
	assert.Equal(t, 0, act.getExecCount())

	// Item should be completed (deleted from KV) — not marked as failed.
	_ = flowStore
}

func TestWorkerPool_PanicRecovery(t *testing.T) {
	callCount := 0
	act := &testAction{
		execFn: func() error {
			callCount++
			if callCount == 1 {
				panic("boom")
			}
			return nil
		},
	}

	wp, store, flowStore := setupWorkerPool(t, 1, act)

	_ = flowStore.Save(&model.Flow{ID: "f1", Name: "Flow 1", Enabled: true, Actions: []model.Action{{ID: "a1", SendMessage: &model.SendMessageActionConfig{}}}})

	// Enqueue an item that will panic.
	item1 := &model.WorkItem{ID: "w1", FlowID: "f1", FlowName: "Flow 1"}
	require.NoError(t, store.Enqueue(item1))

	wp.Start()
	wp.Notify()

	// Wait for the panicking item to be deleted.
	require.Eventually(t, func() bool {
		got, _ := store.Get("w1")
		return got == nil
	}, 5*time.Second, 10*time.Millisecond)

	// Enqueue a second item that should succeed, proving the pool survived.
	item2 := &model.WorkItem{ID: "w2", FlowID: "f1", FlowName: "Flow 1"}
	require.NoError(t, store.Enqueue(item2))
	wp.Notify()

	require.Eventually(t, func() bool {
		got, _ := store.Get("w2")
		return got == nil // completed items are deleted
	}, 5*time.Second, 10*time.Millisecond)

	wp.Stop()

	// The action was called twice total (once panicking, once succeeding).
	assert.Equal(t, 2, act.getExecCount())
}

func TestWorkerPool_CreatorLookupError(t *testing.T) {
	act := &testAction{}

	store, _ := setupStore(t)
	api := &plugintest.API{}
	api.On("LogInfo", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	api.On("LogWarn", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	api.On("LogError", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	api.On("GetUser", "some-user").Return(nil, mmmodel.NewAppError("GetUser", "app.user.get.app_error", nil, "", 500))

	registry := flow.NewRegistry()
	registry.RegisterAction(act)
	executor := flow.NewFlowExecutor(registry)
	flowStore := newTestFlowStore()

	wp := NewWorkerPool(store, executor, flowStore, nil, nil, api, 4)
	wp.pollInterval = 50 * time.Millisecond

	_ = flowStore.Save(&model.Flow{ID: "f1", Name: "Flow 1", Enabled: true, CreatedBy: "some-user", Actions: []model.Action{{ID: "a1", SendMessage: &model.SendMessageActionConfig{}}}})

	item := &model.WorkItem{ID: "w1", FlowID: "f1", FlowName: "Flow 1"}
	require.NoError(t, store.Enqueue(item))

	wp.Start()
	wp.Notify()

	require.Eventually(t, func() bool {
		got, _ := store.Get("w1")
		return got == nil
	}, 5*time.Second, 10*time.Millisecond)

	wp.Stop()

	// Action should never have been called.
	assert.Equal(t, 0, act.getExecCount())

	// Flow should remain enabled — this is a transient error.
	f, _ := flowStore.Get("f1")
	require.NotNil(t, f)
	assert.True(t, f.Enabled)
}

func TestWorkerPool_CreatorPermanentlyDeleted(t *testing.T) {
	act := &testAction{}

	store, _ := setupStore(t)
	api := &plugintest.API{}
	api.On("LogInfo", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	api.On("LogWarn", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	api.On("LogError", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	api.On("GetUser", "deleted-user").Return(nil, mmmodel.NewAppError("GetUser", "app.user.missing.app_error", nil, "", 404))

	registry := flow.NewRegistry()
	registry.RegisterAction(act)
	executor := flow.NewFlowExecutor(registry)
	flowStore := newTestFlowStore()

	wp := NewWorkerPool(store, executor, flowStore, nil, nil, api, 4)
	wp.pollInterval = 50 * time.Millisecond

	_ = flowStore.Save(&model.Flow{ID: "f1", Name: "Flow 1", Enabled: true, CreatedBy: "deleted-user", Actions: []model.Action{{ID: "a1", SendMessage: &model.SendMessageActionConfig{}}}})

	item := &model.WorkItem{ID: "w1", FlowID: "f1", FlowName: "Flow 1"}
	require.NoError(t, store.Enqueue(item))

	wp.Start()
	wp.Notify()

	require.Eventually(t, func() bool {
		got, _ := store.Get("w1")
		return got == nil
	}, 5*time.Second, 10*time.Millisecond)

	wp.Stop()

	// Action should never have been called.
	assert.Equal(t, 0, act.getExecCount())

	// Flow should be disabled — creator is permanently gone.
	f, _ := flowStore.Get("f1")
	require.NotNil(t, f)
	assert.False(t, f.Enabled)
}

func TestWorkerPool_CreatorDeactivated(t *testing.T) {
	act := &testAction{}

	store, _ := setupStore(t)
	api := &plugintest.API{}
	api.On("LogInfo", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	api.On("LogWarn", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	api.On("LogError", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	api.On("GetUser", "deactivated-user").Return(&mmmodel.User{DeleteAt: 1234567890}, nil)

	registry := flow.NewRegistry()
	registry.RegisterAction(act)
	executor := flow.NewFlowExecutor(registry)
	flowStore := newTestFlowStore()

	wp := NewWorkerPool(store, executor, flowStore, nil, nil, api, 4)
	wp.pollInterval = 50 * time.Millisecond

	_ = flowStore.Save(&model.Flow{ID: "f1", Name: "Flow 1", Enabled: true, CreatedBy: "deactivated-user", Actions: []model.Action{{ID: "a1", SendMessage: &model.SendMessageActionConfig{}}}})

	item := &model.WorkItem{ID: "w1", FlowID: "f1", FlowName: "Flow 1"}
	require.NoError(t, store.Enqueue(item))

	wp.Start()
	wp.Notify()

	require.Eventually(t, func() bool {
		got, _ := store.Get("w1")
		return got == nil
	}, 5*time.Second, 10*time.Millisecond)

	wp.Stop()

	// Action should never have been called.
	assert.Equal(t, 0, act.getExecCount())

	// Flow should be disabled.
	f, _ := flowStore.Get("f1")
	require.NotNil(t, f)
	assert.False(t, f.Enabled)
}

func TestWorkerPool_CreatorPermissionDemoted(t *testing.T) {
	act := &testAction{}

	store, _ := setupStore(t)
	api := &plugintest.API{}
	api.On("LogInfo", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	api.On("LogWarn", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	api.On("LogError", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	api.On("GetUser", "demoted-user").Return(&mmmodel.User{DeleteAt: 0}, nil)
	// User is no longer a system admin.
	api.On("HasPermissionTo", "demoted-user", mock.Anything).Return(false)
	// User is no longer a channel admin.
	api.On("GetChannelMember", "ch1", "demoted-user").Return(&mmmodel.ChannelMember{SchemeAdmin: false}, nil)

	registry := flow.NewRegistry()
	registry.RegisterAction(act)
	executor := flow.NewFlowExecutor(registry)
	flowStore := newTestFlowStore()

	wp := NewWorkerPool(store, executor, flowStore, nil, nil, api, 4)
	wp.pollInterval = 50 * time.Millisecond

	_ = flowStore.Save(&model.Flow{
		ID: "f1", Name: "Flow 1", Enabled: true, CreatedBy: "demoted-user",
		Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
		Actions: []model.Action{{ID: "a1", SendMessage: &model.SendMessageActionConfig{ChannelID: "ch1", Body: "hi"}}},
	})

	item := &model.WorkItem{ID: "w1", FlowID: "f1", FlowName: "Flow 1"}
	require.NoError(t, store.Enqueue(item))

	wp.Start()
	wp.Notify()

	require.Eventually(t, func() bool {
		got, _ := store.Get("w1")
		return got == nil
	}, 5*time.Second, 10*time.Millisecond)

	wp.Stop()

	// Action should never have been called.
	assert.Equal(t, 0, act.getExecCount())

	// Flow should be disabled — creator lost permissions.
	f, _ := flowStore.Get("f1")
	require.NotNil(t, f)
	assert.False(t, f.Enabled)
}

func TestWorkerPool_CreatorPermissionCheckTransientError(t *testing.T) {
	act := &testAction{}

	store, _ := setupStore(t)
	api := &plugintest.API{}
	api.On("LogInfo", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	api.On("LogWarn", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	api.On("LogError", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	api.On("GetUser", "some-user").Return(&mmmodel.User{DeleteAt: 0}, nil)
	// User is not a system admin.
	api.On("HasPermissionTo", "some-user", mock.Anything).Return(false)
	// GetChannelMember returns a 500 — transient infrastructure error.
	api.On("GetChannelMember", "ch1", "some-user").Return(nil, mmmodel.NewAppError("GetChannelMember", "app.channel.get_member.app_error", nil, "", 500))

	registry := flow.NewRegistry()
	registry.RegisterAction(act)
	executor := flow.NewFlowExecutor(registry)
	flowStore := newTestFlowStore()

	wp := NewWorkerPool(store, executor, flowStore, nil, nil, api, 4)
	wp.pollInterval = 50 * time.Millisecond

	_ = flowStore.Save(&model.Flow{
		ID: "f1", Name: "Flow 1", Enabled: true, CreatedBy: "some-user",
		Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
		Actions: []model.Action{{ID: "a1", SendMessage: &model.SendMessageActionConfig{ChannelID: "ch1", Body: "hi"}}},
	})

	item := &model.WorkItem{ID: "w1", FlowID: "f1", FlowName: "Flow 1"}
	require.NoError(t, store.Enqueue(item))

	wp.Start()
	wp.Notify()

	require.Eventually(t, func() bool {
		got, _ := store.Get("w1")
		return got == nil
	}, 5*time.Second, 10*time.Millisecond)

	wp.Stop()

	// Action should never have been called.
	assert.Equal(t, 0, act.getExecCount())

	// Flow should remain enabled — this is a transient error.
	f, _ := flowStore.Get("f1")
	require.NotNil(t, f)
	assert.True(t, f.Enabled)
}

func TestWorkerPool_DisabledFlow(t *testing.T) {
	act := &testAction{}
	wp, store, flowStore := setupWorkerPool(t, 4, act)

	_ = flowStore.Save(&model.Flow{ID: "f1", Name: "Flow 1", Enabled: false, Actions: []model.Action{{ID: "a1", SendMessage: &model.SendMessageActionConfig{}}}})

	item := &model.WorkItem{ID: "w1", FlowID: "f1", FlowName: "Flow 1"}
	require.NoError(t, store.Enqueue(item))

	wp.Start()
	wp.Notify()

	// Wait for processing.
	require.Eventually(t, func() bool {
		got, _ := store.Get("w1")
		return got == nil
	}, 5*time.Second, 10*time.Millisecond)

	wp.Stop()

	// Action should never have been called.
	assert.Equal(t, 0, act.getExecCount())
}
