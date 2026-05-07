package automation

import (
	"io"
	"sync"
	"testing"
	"time"

	"github.com/mattermost/mattermost/server/public/plugin"
	"github.com/mattermost/mattermost/server/public/plugin/plugintest"
	"github.com/mattermost/mattermost/server/public/pluginapi/cluster"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

// --- makeScheduleWaitInterval tests ---

func TestMakeScheduleWaitInterval_NoStartAt(t *testing.T) {
	interval := 10 * time.Minute
	fn := makeScheduleWaitInterval(interval, 0)

	now := time.Now()

	t.Run("first run executes immediately", func(t *testing.T) {
		d := fn(now, cluster.JobMetadata{})
		assert.Equal(t, time.Duration(0), d)
	})

	t.Run("subsequent run uses interval", func(t *testing.T) {
		lastFinished := now.Add(-3 * time.Minute)
		d := fn(now, cluster.JobMetadata{LastFinished: lastFinished})
		assert.InDelta(t, (7 * time.Minute).Seconds(), d.Seconds(), 1)
	})
}

func TestMakeScheduleWaitInterval_StartAtInFuture(t *testing.T) {
	interval := 10 * time.Minute
	now := time.Now()
	startAt := now.Add(5 * time.Minute)
	fn := makeScheduleWaitInterval(interval, model.TimeToTimestamp(startAt))

	t.Run("first run defers to startAt", func(t *testing.T) {
		d := fn(now, cluster.JobMetadata{})
		assert.InDelta(t, (5 * time.Minute).Seconds(), d.Seconds(), 1)
	})

	t.Run("defers to startAt even with stale LastFinished", func(t *testing.T) {
		// When a job is restarted (automation updated), the cluster KV metadata
		// from the previous job persists. The wait function must still
		// honour startAt even though LastFinished is non-zero.
		staleLastFinished := now.Add(-2 * time.Minute)
		d := fn(now, cluster.JobMetadata{LastFinished: staleLastFinished})
		assert.InDelta(t, (5 * time.Minute).Seconds(), d.Seconds(), 1,
			"must wait until startAt regardless of stale LastFinished")
	})

	t.Run("after startAt has passed uses interval", func(t *testing.T) {
		// Simulate being called after startAt: now is 1 minute past startAt,
		// and the job just finished at startAt.
		afterStartAt := startAt.Add(1 * time.Minute)
		d := fn(afterStartAt, cluster.JobMetadata{LastFinished: startAt})
		// Should wait interval minus elapsed time = 10m - 1m = 9m
		assert.InDelta(t, (9 * time.Minute).Seconds(), d.Seconds(), 1)
	})
}

func TestMakeScheduleWaitInterval_StartAtInPast(t *testing.T) {
	interval := 10 * time.Minute
	now := time.Now()
	startAt := now.Add(-2 * time.Minute)
	fn := makeScheduleWaitInterval(interval, model.TimeToTimestamp(startAt))

	t.Run("first run executes immediately", func(t *testing.T) {
		d := fn(now, cluster.JobMetadata{})
		assert.Equal(t, time.Duration(0), d)
	})
}

// --- ScheduleManager tests ---

// mockEnqueuer records enqueued items for assertion.
type mockEnqueuer struct {
	mu    sync.Mutex
	items []*model.WorkItem
}

func (m *mockEnqueuer) Enqueue(item *model.WorkItem) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items = append(m.items, item)
	return nil
}

// mockNotifier records notifications.
type mockNotifier struct {
	mu    sync.Mutex
	count int
}

func (m *mockNotifier) Notify() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.count++
}

// fakeCloser tracks Close calls for testing job lifecycle.
type fakeCloser struct {
	closed bool
}

func (f *fakeCloser) Close() error {
	f.closed = true
	return nil
}

func newTestAPI(t *testing.T) *plugintest.API {
	t.Helper()
	api := &plugintest.API{}
	api.On("LogError", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Maybe()
	api.On("LogInfo", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Maybe()
	api.On("LogDebug", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Maybe()
	return api
}

func TestScheduleManager_StartSkipsNonScheduleAndDisabled(t *testing.T) {
	store, _ := setupStore(t)
	api := newTestAPI(t)
	enq := &mockEnqueuer{}
	notif := &mockNotifier{}

	// Save a message_posted automation and a disabled schedule automation.
	require.NoError(t, store.Save(&model.Automation{
		ID:      "f1",
		Enabled: true,
		Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
	}))
	require.NoError(t, store.Save(&model.Automation{
		ID:      "f2",
		Enabled: false,
		Trigger: model.Trigger{Schedule: &model.ScheduleConfig{ChannelID: "ch1", Interval: "1h"}},
	}))

	sm := NewScheduleManager(api, store, enq, notif)
	// We can't call Start() because it would call cluster.Schedule
	// which needs real KV store. Instead test the filtering logic directly.
	automations, err := store.ListScheduled()
	require.NoError(t, err)

	var scheduled int
	for _, f := range automations {
		if f.Enabled {
			scheduled++
		}
	}
	assert.Equal(t, 0, scheduled)

	// Verify manager was created with empty jobs map.
	sm.mu.Lock()
	assert.Empty(t, sm.jobs)
	sm.mu.Unlock()
}

func TestScheduleManager_StopClosesAllJobs(t *testing.T) {
	api := newTestAPI(t)
	store, _ := setupStore(t)
	enq := &mockEnqueuer{}
	notif := &mockNotifier{}

	sm := NewScheduleManager(api, store, enq, notif)

	// Inject fake jobs.
	j1 := &fakeCloser{}
	j2 := &fakeCloser{}
	sm.mu.Lock()
	sm.jobs["f1"] = j1
	sm.jobs["f2"] = j2
	sm.mu.Unlock()

	sm.Stop()

	assert.True(t, j1.closed)
	assert.True(t, j2.closed)

	sm.mu.Lock()
	assert.Empty(t, sm.jobs)
	sm.mu.Unlock()
}

func TestScheduleManager_RemoveAutomationStopsJob(t *testing.T) {
	api := newTestAPI(t)
	store, _ := setupStore(t)
	enq := &mockEnqueuer{}
	notif := &mockNotifier{}

	sm := NewScheduleManager(api, store, enq, notif)

	j := &fakeCloser{}
	sm.mu.Lock()
	sm.jobs["f1"] = j
	sm.mu.Unlock()

	sm.RemoveAutomation("f1")

	assert.True(t, j.closed)
	sm.mu.Lock()
	_, exists := sm.jobs["f1"]
	sm.mu.Unlock()
	assert.False(t, exists)
}

func TestScheduleManager_RemoveAutomationNoOp(t *testing.T) {
	api := newTestAPI(t)
	store, _ := setupStore(t)
	enq := &mockEnqueuer{}
	notif := &mockNotifier{}

	sm := NewScheduleManager(api, store, enq, notif)

	// Should not panic when removing a automation that has no job.
	sm.RemoveAutomation("nonexistent")
}

func TestScheduleManager_SyncAutomationStopsOldJob(t *testing.T) {
	api := newTestAPI(t)
	store, _ := setupStore(t)
	enq := &mockEnqueuer{}
	notif := &mockNotifier{}

	sm := NewScheduleManager(api, store, enq, notif)

	oldJob := &fakeCloser{}
	sm.mu.Lock()
	sm.jobs["f1"] = oldJob
	sm.mu.Unlock()

	// Sync with a non-schedule automation should stop the old job and not add a new one.
	// The existing automation was a schedule trigger, so the job should be removed.
	existing := &model.Automation{
		ID:      "f1",
		Enabled: true,
		Trigger: model.Trigger{Schedule: &model.ScheduleConfig{ChannelID: "ch1", Interval: "1h"}},
	}
	err := sm.SyncAutomation(existing, &model.Automation{
		ID:      "f1",
		Enabled: true,
		Trigger: model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}},
	})
	require.NoError(t, err)

	assert.True(t, oldJob.closed)
	sm.mu.Lock()
	_, exists := sm.jobs["f1"]
	sm.mu.Unlock()
	assert.False(t, exists)
}

func TestScheduleManager_FireSchedule(t *testing.T) {
	api := newTestAPI(t)
	store, _ := setupStore(t)
	enq := &mockEnqueuer{}
	notif := &mockNotifier{}

	sm := NewScheduleManager(api, store, enq, notif)

	sm.fireSchedule("auto1", "Test Automation", "1h", "ch1")

	enq.mu.Lock()
	require.Len(t, enq.items, 1)
	item := enq.items[0]
	enq.mu.Unlock()

	assert.Equal(t, "auto1", item.AutomationID)
	assert.Equal(t, "Test Automation", item.AutomationName)
	require.NotNil(t, item.TriggerData.Schedule)
	assert.Equal(t, "1h", item.TriggerData.Schedule.Interval)
	assert.NotZero(t, item.TriggerData.Schedule.FiredAt)
	require.NotNil(t, item.TriggerData.Channel)
	assert.Equal(t, "ch1", item.TriggerData.Channel.Id)

	notif.mu.Lock()
	assert.Equal(t, 1, notif.count)
	notif.mu.Unlock()
}

// fakeScheduleFunc returns a fake scheduleFn that creates fakeClosers
// and records the keys it was called with.
func fakeScheduleFunc(calls *[]string) scheduleFunc {
	return func(_ plugin.API, key string, _ cluster.NextWaitInterval, _ func()) (io.Closer, error) {
		*calls = append(*calls, key)
		return &fakeCloser{}, nil
	}
}

func TestScheduleManager_SyncAutomationUpdatedIntervalStopsOldJob(t *testing.T) {
	api := newTestAPI(t)
	store, _ := setupStore(t)
	enq := &mockEnqueuer{}
	notif := &mockNotifier{}

	sm := NewScheduleManager(api, store, enq, notif)
	var calls []string
	sm.scheduleFn = fakeScheduleFunc(&calls)

	// Simulate an existing job for a schedule automation with a 1h interval.
	oldJob := &fakeCloser{}
	sm.mu.Lock()
	sm.jobs["f1"] = oldJob
	sm.mu.Unlock()

	// SyncAutomation with a new interval — the old job must be closed and
	// a new job created with the updated parameters.
	existing := &model.Automation{
		ID:      "f1",
		Enabled: true,
		Trigger: model.Trigger{Schedule: &model.ScheduleConfig{ChannelID: "ch1", Interval: "1h"}},
	}
	err := sm.SyncAutomation(existing, &model.Automation{
		ID:      "f1",
		Enabled: true,
		Trigger: model.Trigger{Schedule: &model.ScheduleConfig{ChannelID: "ch1", Interval: "2h"}},
	})
	require.NoError(t, err)

	assert.True(t, oldJob.closed, "old job must be closed when interval changes")
	require.Len(t, calls, 1, "a new job must be scheduled")
	assert.Equal(t, scheduleJobKeyPrefix+"f1", calls[0])

	// Verify the new job is tracked.
	sm.mu.Lock()
	_, exists := sm.jobs["f1"]
	sm.mu.Unlock()
	assert.True(t, exists, "new job must be in the jobs map")
}

func TestScheduleManager_SyncAutomationUpdatedStartAtStopsOldJob(t *testing.T) {
	api := newTestAPI(t)
	store, _ := setupStore(t)
	enq := &mockEnqueuer{}
	notif := &mockNotifier{}

	sm := NewScheduleManager(api, store, enq, notif)
	var calls []string
	sm.scheduleFn = fakeScheduleFunc(&calls)

	// Simulate an existing job for a schedule automation.
	oldJob := &fakeCloser{}
	sm.mu.Lock()
	sm.jobs["f1"] = oldJob
	sm.mu.Unlock()

	// SyncAutomation with a new start_at — the old job must be closed and
	// a new one created with the updated parameters.
	existing := &model.Automation{
		ID:      "f1",
		Enabled: true,
		Trigger: model.Trigger{Schedule: &model.ScheduleConfig{ChannelID: "ch1", Interval: "1h"}},
	}
	err := sm.SyncAutomation(existing, &model.Automation{
		ID:      "f1",
		Enabled: true,
		Trigger: model.Trigger{Schedule: &model.ScheduleConfig{ChannelID: "ch1", Interval: "1h", StartAt: 1700000000000}},
	})
	require.NoError(t, err)

	assert.True(t, oldJob.closed, "old job must be closed when start_at changes")
	require.Len(t, calls, 1, "a new job must be scheduled")
}

func TestScheduleManager_SyncAutomationDisabledStopsJob(t *testing.T) {
	api := newTestAPI(t)
	store, _ := setupStore(t)
	enq := &mockEnqueuer{}
	notif := &mockNotifier{}

	sm := NewScheduleManager(api, store, enq, notif)

	// Inject an existing job.
	existingJob := &fakeCloser{}
	sm.mu.Lock()
	sm.jobs["f1"] = existingJob
	sm.mu.Unlock()

	// Sync a disabled schedule automation — should close the old job.
	existing := &model.Automation{
		ID:      "f1",
		Enabled: true,
		Trigger: model.Trigger{Schedule: &model.ScheduleConfig{ChannelID: "ch1", Interval: "1h"}},
	}
	err := sm.SyncAutomation(existing, &model.Automation{
		ID:      "f1",
		Enabled: false,
		Trigger: model.Trigger{Schedule: &model.ScheduleConfig{ChannelID: "ch1", Interval: "1h"}},
	})
	require.NoError(t, err)

	assert.True(t, existingJob.closed)
	sm.mu.Lock()
	assert.NotContains(t, sm.jobs, "f1")
	sm.mu.Unlock()
}

func TestScheduleManager_SyncAutomationNoOpWhenScheduleUnchanged(t *testing.T) {
	api := newTestAPI(t)
	store, _ := setupStore(t)
	enq := &mockEnqueuer{}
	notif := &mockNotifier{}

	sm := NewScheduleManager(api, store, enq, notif)
	var calls []string
	sm.scheduleFn = fakeScheduleFunc(&calls)

	// Inject an existing job.
	existingJob := &fakeCloser{}
	sm.mu.Lock()
	sm.jobs["f1"] = existingJob
	sm.mu.Unlock()

	// Update only the automation name and actions — schedule fields stay the same.
	existing := &model.Automation{
		ID:      "f1",
		Name:    "Old Name",
		Enabled: true,
		Trigger: model.Trigger{Schedule: &model.ScheduleConfig{ChannelID: "ch1", Interval: "1h", StartAt: 1700000000000}},
	}
	err := sm.SyncAutomation(existing, &model.Automation{
		ID:      "f1",
		Name:    "New Name",
		Enabled: true,
		Trigger: model.Trigger{Schedule: &model.ScheduleConfig{ChannelID: "ch1", Interval: "1h", StartAt: 1700000000000}},
	})
	require.NoError(t, err)

	assert.False(t, existingJob.closed, "job must NOT be stopped when schedule fields are unchanged")
	assert.Empty(t, calls, "no new job should be created when schedule fields are unchanged")

	// Verify the original job is still tracked.
	sm.mu.Lock()
	job, exists := sm.jobs["f1"]
	sm.mu.Unlock()
	assert.True(t, exists)
	assert.Equal(t, existingJob, job, "original job reference must be preserved")
}

func TestScheduleManager_SyncAutomationCreateNewSchedule(t *testing.T) {
	api := newTestAPI(t)
	store, _ := setupStore(t)
	enq := &mockEnqueuer{}
	notif := &mockNotifier{}

	sm := NewScheduleManager(api, store, enq, notif)
	var calls []string
	sm.scheduleFn = fakeScheduleFunc(&calls)

	// Create (nil existing) with a schedule automation — should start a new job.
	err := sm.SyncAutomation(nil, &model.Automation{
		ID:      "f1",
		Enabled: true,
		Trigger: model.Trigger{Schedule: &model.ScheduleConfig{ChannelID: "ch1", Interval: "1h"}},
	})
	require.NoError(t, err)

	require.Len(t, calls, 1, "a new job must be created on automation creation")
	assert.Equal(t, scheduleJobKeyPrefix+"f1", calls[0])

	sm.mu.Lock()
	_, exists := sm.jobs["f1"]
	sm.mu.Unlock()
	assert.True(t, exists)
}

func TestScheduleManager_StopIsIdempotent(t *testing.T) {
	api := newTestAPI(t)
	store, _ := setupStore(t)

	sm := NewScheduleManager(api, store, &mockEnqueuer{}, &mockNotifier{})
	sm.mu.Lock()
	sm.jobs["f1"] = &fakeCloser{}
	sm.mu.Unlock()

	sm.Stop()
	sm.Stop() // should not panic
}
