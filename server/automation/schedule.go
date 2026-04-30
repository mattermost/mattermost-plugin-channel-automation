package automation

import (
	"io"
	"sync"
	"time"

	mmmodel "github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
	"github.com/mattermost/mattermost/server/public/pluginapi/cluster"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

const scheduleJobKeyPrefix = "sched_"

// WorkItemEnqueuer enqueues work items into the persistent queue.
type WorkItemEnqueuer interface {
	Enqueue(item *model.WorkItem) error
}

// WorkerNotifier wakes the worker pool to process new items.
type WorkerNotifier interface {
	Notify()
}

// scheduleFunc is the function used to create cluster jobs. It matches
// the signature of cluster.Schedule and can be replaced in tests.
type scheduleFunc func(api plugin.API, key string, wait cluster.NextWaitInterval, cb func()) (io.Closer, error)

func defaultScheduleFunc(api plugin.API, key string, wait cluster.NextWaitInterval, cb func()) (io.Closer, error) {
	return cluster.Schedule(api, key, wait, cb)
}

// ScheduleManager manages cluster.Job instances for schedule-triggered automations.
type ScheduleManager struct {
	api             plugin.API
	automationStore model.Store
	enqueuer        WorkItemEnqueuer
	notifier        WorkerNotifier

	scheduleFn scheduleFunc
	mu         sync.Mutex
	jobs       map[string]io.Closer // automation ID → active cluster.Job
}

// NewScheduleManager creates a new ScheduleManager.
func NewScheduleManager(api plugin.API, automationStore model.Store, enqueuer WorkItemEnqueuer, notifier WorkerNotifier) *ScheduleManager {
	return &ScheduleManager{
		api:             api,
		automationStore: automationStore,
		enqueuer:        enqueuer,
		notifier:        notifier,
		scheduleFn:      defaultScheduleFunc,
		jobs:            make(map[string]io.Closer),
	}
}

// Start lists schedule-triggered automations and creates cluster jobs for enabled ones.
func (sm *ScheduleManager) Start() error {
	automations, err := sm.automationStore.ListScheduled()
	if err != nil {
		return err
	}

	for _, a := range automations {
		if !a.Enabled {
			continue
		}
		if err := sm.startJob(a); err != nil {
			sm.api.LogError("Failed to start schedule job",
				"automation_id", a.ID,
				"err", err.Error(),
			)
		}
	}
	return nil
}

// Stop closes all active cluster jobs.
func (sm *ScheduleManager) Stop() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	for id, job := range sm.jobs {
		if err := job.Close(); err != nil {
			sm.api.LogError("Failed to close schedule job", "automation_id", id, "err", err.Error())
		}
	}
	sm.jobs = make(map[string]io.Closer)
}

// SyncAutomation updates the schedule job for an automation after create or update.
// It compares the existing automation (nil on create) with the new automation and
// only restarts the cluster job when the schedule-relevant fields
// (Trigger.Type, Enabled, Interval, StartAt) actually changed.
func (sm *ScheduleManager) SyncAutomation(existing *model.Automation, a *model.Automation) error {
	oldIsActive := existing != nil && existing.Trigger.Schedule != nil && existing.Enabled
	newIsActive := a.Trigger.Schedule != nil && a.Enabled

	// Determine whether the schedule parameters changed.
	scheduleChanged := true // default true for create (no existing)
	if oldIsActive && newIsActive {
		scheduleChanged = existing.Trigger.Schedule.ChannelID != a.Trigger.Schedule.ChannelID ||
			existing.Trigger.Schedule.Interval != a.Trigger.Schedule.Interval ||
			existing.Trigger.Schedule.StartAt != a.Trigger.Schedule.StartAt
	}

	// Stop the old job if it was active and either no longer active or params changed.
	if oldIsActive && (!newIsActive || scheduleChanged) {
		sm.stopJob(a.ID)
	}

	// Start a new job if it should be active and either wasn't before or params changed.
	if newIsActive && (!oldIsActive || scheduleChanged) {
		if err := sm.startJob(a); err != nil {
			sm.api.LogError("Failed to sync schedule job",
				"automation_id", a.ID,
				"err", err.Error(),
			)
			return err
		}
	}

	return nil
}

// RemoveAutomation stops the schedule job for a deleted automation.
func (sm *ScheduleManager) RemoveAutomation(automationID string) {
	sm.stopJob(automationID)
}

func (sm *ScheduleManager) startJob(a *model.Automation) error {
	interval, err := time.ParseDuration(a.Trigger.Schedule.Interval)
	if err != nil {
		return err
	}

	key := scheduleJobKeyPrefix + a.ID
	waitFn := makeScheduleWaitInterval(interval, a.Trigger.Schedule.StartAt)
	automationID := a.ID
	automationName := a.Name
	intervalStr := a.Trigger.Schedule.Interval
	channelID := a.Trigger.Schedule.ChannelID

	job, err := sm.scheduleFn(sm.api, key, waitFn, func() {
		sm.fireSchedule(automationID, automationName, intervalStr, channelID)
	})
	if err != nil {
		return err
	}

	sm.mu.Lock()
	sm.jobs[a.ID] = job
	sm.mu.Unlock()

	sm.api.LogInfo("Schedule job started", "automation_id", a.ID, "interval", a.Trigger.Schedule.Interval)
	return nil
}

func (sm *ScheduleManager) stopJob(automationID string) {
	sm.mu.Lock()
	job, ok := sm.jobs[automationID]
	if ok {
		delete(sm.jobs, automationID)
	}
	sm.mu.Unlock()

	if ok {
		if err := job.Close(); err != nil {
			sm.api.LogError("Failed to close schedule job", "automation_id", automationID, "err", err.Error())
		}
	}
}

func (sm *ScheduleManager) fireSchedule(automationID, automationName, interval, channelID string) {
	item := &model.WorkItem{
		ID:             mmmodel.NewId(),
		AutomationID:   automationID,
		AutomationName: automationName,
		TriggerData: model.TriggerData{
			Channel: &model.SafeChannel{Id: channelID},
			Schedule: &model.ScheduleInfo{
				FiredAt:  model.NowTimestamp(),
				Interval: interval,
			},
		},
	}

	if err := sm.enqueuer.Enqueue(item); err != nil {
		sm.api.LogError("Failed to enqueue scheduled work item",
			"automation_id", automationID,
			"err", err.Error(),
		)
		return
	}

	sm.api.LogDebug("Scheduled work item enqueued",
		"work_item_id", item.ID,
		"automation_id", automationID,
	)

	sm.notifier.Notify()
}

// makeScheduleWaitInterval returns a NextWaitInterval that defers
// execution until startAtMs (Unix milliseconds) if it is in the future,
// then uses a fixed interval for subsequent runs.
func makeScheduleWaitInterval(interval time.Duration, startAtMs int64) cluster.NextWaitInterval {
	baseWait := cluster.MakeWaitForInterval(interval)

	return func(now time.Time, metadata cluster.JobMetadata) time.Duration {
		// If startAt is set and still in the future, always wait until
		// then — even if stale LastFinished metadata exists from a
		// previous cluster job (e.g. after an automation update).
		if startAtMs > 0 {
			startAt := model.TimestampToTime(startAtMs)
			if now.Before(startAt) {
				return startAt.Sub(now)
			}
		}
		return baseWait(now, metadata)
	}
}
