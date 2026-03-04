package flow

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

// ScheduleManager manages cluster.Job instances for schedule-triggered flows.
type ScheduleManager struct {
	api       plugin.API
	flowStore model.Store
	enqueuer  WorkItemEnqueuer
	notifier  WorkerNotifier

	scheduleFn scheduleFunc
	mu         sync.Mutex
	jobs       map[string]io.Closer // flow ID → active cluster.Job
}

// NewScheduleManager creates a new ScheduleManager.
func NewScheduleManager(api plugin.API, flowStore model.Store, enqueuer WorkItemEnqueuer, notifier WorkerNotifier) *ScheduleManager {
	return &ScheduleManager{
		api:        api,
		flowStore:  flowStore,
		enqueuer:   enqueuer,
		notifier:   notifier,
		scheduleFn: defaultScheduleFunc,
		jobs:       make(map[string]io.Closer),
	}
}

// Start lists all flows and creates cluster jobs for enabled schedule flows.
func (sm *ScheduleManager) Start() error {
	flows, err := sm.flowStore.List()
	if err != nil {
		return err
	}

	for _, f := range flows {
		if f.Trigger.Schedule == nil || !f.Enabled {
			continue
		}
		if err := sm.startJob(f); err != nil {
			sm.api.LogError("Failed to start schedule job",
				"flow_id", f.ID,
				"flow_name", f.Name,
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
			sm.api.LogError("Failed to close schedule job", "flow_id", id, "err", err.Error())
		}
	}
	sm.jobs = make(map[string]io.Closer)
}

// SyncFlow updates the schedule job for a flow after create or update.
// It compares the existing flow (nil on create) with the new flow and
// only restarts the cluster job when the schedule-relevant fields
// (Trigger.Type, Enabled, Interval, StartAt) actually changed.
func (sm *ScheduleManager) SyncFlow(existing *model.Flow, f *model.Flow) {
	oldIsActive := existing != nil && existing.Trigger.Schedule != nil && existing.Enabled
	newIsActive := f.Trigger.Schedule != nil && f.Enabled

	// Determine whether the schedule parameters changed.
	scheduleChanged := true // default true for create (no existing)
	if oldIsActive && newIsActive {
		scheduleChanged = existing.Trigger.Schedule.ChannelID != f.Trigger.Schedule.ChannelID ||
			existing.Trigger.Schedule.Interval != f.Trigger.Schedule.Interval ||
			existing.Trigger.Schedule.StartAt != f.Trigger.Schedule.StartAt
	}

	// Stop the old job if it was active and either no longer active or params changed.
	if oldIsActive && (!newIsActive || scheduleChanged) {
		sm.stopJob(f.ID)
	}

	// Start a new job if it should be active and either wasn't before or params changed.
	if newIsActive && (!oldIsActive || scheduleChanged) {
		if err := sm.startJob(f); err != nil {
			sm.api.LogError("Failed to sync schedule job",
				"flow_id", f.ID,
				"flow_name", f.Name,
				"err", err.Error(),
			)
		}
	}
}

// RemoveFlow stops the schedule job for a deleted flow.
func (sm *ScheduleManager) RemoveFlow(flowID string) {
	sm.stopJob(flowID)
}

func (sm *ScheduleManager) startJob(f *model.Flow) error {
	interval, err := time.ParseDuration(f.Trigger.Schedule.Interval)
	if err != nil {
		return err
	}

	key := scheduleJobKeyPrefix + f.ID
	waitFn := makeScheduleWaitInterval(interval, f.Trigger.Schedule.StartAt)
	flowID := f.ID
	flowName := f.Name
	intervalStr := f.Trigger.Schedule.Interval

	job, err := sm.scheduleFn(sm.api, key, waitFn, func() {
		sm.fireSchedule(flowID, flowName, intervalStr)
	})
	if err != nil {
		return err
	}

	sm.mu.Lock()
	sm.jobs[f.ID] = job
	sm.mu.Unlock()

	sm.api.LogInfo("Schedule job started", "flow_id", f.ID, "flow_name", f.Name, "interval", f.Trigger.Schedule.Interval)
	return nil
}

func (sm *ScheduleManager) stopJob(flowID string) {
	sm.mu.Lock()
	job, ok := sm.jobs[flowID]
	if ok {
		delete(sm.jobs, flowID)
	}
	sm.mu.Unlock()

	if ok {
		if err := job.Close(); err != nil {
			sm.api.LogError("Failed to close schedule job", "flow_id", flowID, "err", err.Error())
		}
	}
}

func (sm *ScheduleManager) fireSchedule(flowID, flowName, interval string) {
	item := &model.WorkItem{
		ID:       mmmodel.NewId(),
		FlowID:   flowID,
		FlowName: flowName,
		TriggerData: model.TriggerData{
			Schedule: &model.ScheduleInfo{
				FiredAt:  time.Now().UnixMilli(),
				Interval: interval,
			},
		},
	}

	if err := sm.enqueuer.Enqueue(item); err != nil {
		sm.api.LogError("Failed to enqueue scheduled work item",
			"flow_id", flowID,
			"flow_name", flowName,
			"err", err.Error(),
		)
		return
	}

	sm.api.LogDebug("Scheduled work item enqueued",
		"work_item_id", item.ID,
		"flow_id", flowID,
		"flow_name", flowName,
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
		// previous cluster job (e.g. after a flow update).
		if startAtMs > 0 {
			startAt := time.UnixMilli(startAtMs)
			if now.Before(startAt) {
				return startAt.Sub(now)
			}
		}
		return baseWait(now, metadata)
	}
}
