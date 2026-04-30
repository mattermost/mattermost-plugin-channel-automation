package workqueue

import (
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	mmmodel "github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/flow"
	"github.com/mattermost/mattermost-plugin-channel-automation/server/flow/notifier"
	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
	"github.com/mattermost/mattermost-plugin-channel-automation/server/permissions"
)

// WorkerPool processes work items from the queue using a bounded pool
// of concurrent goroutines.
type WorkerPool struct {
	store        *Store
	executor     *flow.FlowExecutor
	flowStore    model.Store
	historyStore model.ExecutionStore
	notifier     notifier.FailureNotifier
	api          plugin.API
	maxWorkers   int
	pollInterval time.Duration
	notify       chan struct{}
	stop         chan struct{}
	wg           sync.WaitGroup
	startOnce    sync.Once
	stopOnce     sync.Once
}

// NewWorkerPool creates a WorkerPool. Call Start to begin processing.
// notifier may be nil to disable failure notifications.
func NewWorkerPool(store *Store, executor *flow.FlowExecutor, flowStore model.Store, historyStore model.ExecutionStore, failureNotifier notifier.FailureNotifier, api plugin.API, maxWorkers int) *WorkerPool {
	return &WorkerPool{
		store:        store,
		executor:     executor,
		flowStore:    flowStore,
		historyStore: historyStore,
		notifier:     failureNotifier,
		api:          api,
		maxWorkers:   maxWorkers,
		pollInterval: 30 * time.Second,
		notify:       make(chan struct{}, 1),
		stop:         make(chan struct{}),
	}
}

// Start launches the dispatcher goroutine. Safe to call multiple times;
// only the first call starts the loop.
func (wp *WorkerPool) Start() {
	wp.startOnce.Do(func() {
		wp.wg.Add(1)
		go wp.dispatchLoop()
	})
}

// Stop signals the dispatcher to shut down and waits for all in-flight
// workers to finish. Safe to call multiple times.
func (wp *WorkerPool) Stop() {
	wp.stopOnce.Do(func() {
		close(wp.stop)
	})
	wp.wg.Wait()
}

// Notify wakes the dispatcher to check for new work. Non-blocking.
func (wp *WorkerPool) Notify() {
	select {
	case wp.notify <- struct{}{}:
	default:
	}
}

func (wp *WorkerPool) dispatchLoop() {
	defer wp.wg.Done()

	sem := make(chan struct{}, wp.maxWorkers)
	ticker := time.NewTicker(wp.pollInterval)
	defer ticker.Stop()

	for {
		wp.drainPending(sem)

		select {
		case <-wp.stop:
			// Wait for all in-flight workers to release their semaphore slots.
			for range wp.maxWorkers {
				sem <- struct{}{}
			}
			return
		case <-wp.notify:
		case <-ticker.C:
		}
	}
}

func (wp *WorkerPool) drainPending(sem chan struct{}) {
	for {
		// Try to acquire a semaphore slot. If stop is signaled, return.
		select {
		case <-wp.stop:
			return
		case sem <- struct{}{}:
		}

		item, err := wp.store.ClaimNext()
		if err != nil {
			wp.api.LogError("Failed to claim next work item", "err", err.Error())
			<-sem // release slot
			return
		}
		if item == nil {
			<-sem // release slot, queue empty
			return
		}

		wp.wg.Add(1)
		go wp.runWorker(item, sem)
	}
}

func (wp *WorkerPool) runWorker(item *model.WorkItem, sem chan struct{}) {
	defer wp.wg.Done()
	defer func() { <-sem }()
	defer func() {
		if r := recover(); r != nil {
			errMsg := fmt.Sprintf("panic: %v", r)
			wp.api.LogError("Worker panicked",
				"work_item_id", item.ID,
				"flow_id", item.FlowID,
				"err", errMsg,
			)
			if err := wp.store.Fail(item.ID); err != nil {
				wp.api.LogError("Failed to mark work item as failed after panic",
					"work_item_id", item.ID,
					"err", err.Error(),
				)
			}
		}
	}()

	f, err := wp.flowStore.Get(item.FlowID)
	if err != nil {
		wp.api.LogError("Failed to get flow for work item",
			"work_item_id", item.ID,
			"flow_id", item.FlowID,
			"err", err.Error(),
		)
		if storeErr := wp.store.Fail(item.ID); storeErr != nil {
			wp.api.LogError("Failed to mark work item as failed",
				"work_item_id", item.ID,
				"err", storeErr.Error(),
			)
		}
		return
	}

	// Flow was deleted or disabled between enqueue and execute — silently complete.
	if f == nil || !f.Enabled {
		if err := wp.store.Complete(item.ID); err != nil {
			wp.api.LogError("Failed to complete work item for deleted/disabled flow",
				"work_item_id", item.ID,
				"flow_id", item.FlowID,
				"err", err.Error(),
			)
		}
		return
	}

	// Check that the flow creator is still an active user.
	creator, appErr := wp.api.GetUser(f.CreatedBy)
	if appErr != nil {
		if appErr.StatusCode == http.StatusNotFound {
			// Creator has been permanently deleted — disable the flow.
			wp.disableFlow(f, item, "flow creator account has been deactivated or deleted")
			return
		}

		// Transient API error — fail this execution but leave the flow enabled.
		reason := fmt.Sprintf("failed to look up flow creator %q: %s", f.CreatedBy, appErr.Error())
		wp.api.LogError("Failed to verify flow creator",
			"work_item_id", item.ID,
			"flow_id", f.ID,
			"created_by", f.CreatedBy,
			"err", appErr.Error(),
		)
		if storeErr := wp.store.Fail(item.ID); storeErr != nil {
			wp.api.LogError("Failed to remove work item after creator lookup error",
				"work_item_id", item.ID,
				"err", storeErr.Error(),
			)
		}
		wp.saveExecutionRecord(item, nil, fmt.Errorf("%s", reason), model.NowTimestamp())
		return
	}
	if creator.DeleteAt != 0 {
		// Creator has been deactivated — disable the flow.
		wp.disableFlow(f, item, "flow creator account has been deactivated or deleted")
		return
	}

	// Verify the creator still has the required permissions (e.g. channel
	// admin, team admin, or system admin) — the same check performed at
	// flow creation time.
	if permErr := permissions.CheckFlowPermissions(wp.api, f.CreatedBy, f); permErr != nil {
		var appErr *mmmodel.AppError
		if errors.As(permErr, &appErr) {
			// Transient API error — fail this execution but leave the flow enabled.
			wp.api.LogError("Failed to verify flow creator permissions",
				"work_item_id", item.ID,
				"flow_id", f.ID,
				"created_by", f.CreatedBy,
				"err", permErr.Error(),
			)
			if storeErr := wp.store.Fail(item.ID); storeErr != nil {
				wp.api.LogError("Failed to remove work item after permission check error",
					"work_item_id", item.ID,
					"err", storeErr.Error(),
				)
			}
			wp.saveExecutionRecord(item, nil, fmt.Errorf("failed to verify flow creator permissions: %s", permErr.Error()), model.NowTimestamp())
			return
		}

		// Creator lost the required permissions — disable the flow.
		wp.disableFlow(f, item, fmt.Sprintf("flow creator no longer has the required permissions: %s", permErr.Error()))
		return
	}

	ctx, execErr := wp.executor.Execute(f, item.TriggerData)
	completedAt := model.NowTimestamp()

	if execErr != nil {
		wp.api.LogError("Flow execution failed",
			"work_item_id", item.ID,
			"flow_id", item.FlowID,
			"err", execErr.Error(),
		)
		if storeErr := wp.store.Fail(item.ID); storeErr != nil {
			wp.api.LogError("Failed to mark work item as failed",
				"work_item_id", item.ID,
				"err", storeErr.Error(),
			)
		}
		wp.notifyFailure(f, item, execErr)
	} else {
		if err := wp.store.Complete(item.ID); err != nil {
			wp.api.LogError("Failed to complete work item",
				"work_item_id", item.ID,
				"err", err.Error(),
			)
		}

		wp.api.LogInfo("Flow executed successfully",
			"work_item_id", item.ID,
			"flow_id", item.FlowID,
		)
	}

	wp.saveExecutionRecord(item, ctx, execErr, completedAt)
}

func (wp *WorkerPool) disableFlow(f *model.Flow, item *model.WorkItem, reason string) {
	wp.api.LogWarn("Disabling flow",
		"flow_id", f.ID,
		"created_by", f.CreatedBy,
		"reason", reason,
	)

	f.Enabled = false
	if saveErr := wp.flowStore.Save(f); saveErr != nil {
		wp.api.LogError("Failed to disable flow with inactive creator",
			"flow_id", f.ID,
			"err", saveErr.Error(),
		)
	}

	if storeErr := wp.store.Fail(item.ID); storeErr != nil {
		wp.api.LogError("Failed to remove work item after disabling flow",
			"work_item_id", item.ID,
			"flow_id", f.ID,
			"err", storeErr.Error(),
		)
	}
	wp.saveExecutionRecord(item, nil, fmt.Errorf("%s", reason), model.NowTimestamp())
}

// notifyFailure surfaces the failure to the flow creator via the configured
// notifier (typically a DM from the plugin bot). Safe to call with a nil
// notifier or nil flow.
func (wp *WorkerPool) notifyFailure(f *model.Flow, item *model.WorkItem, execErr error) {
	if wp.notifier == nil || f == nil || execErr == nil {
		wp.api.LogWarn("Skipping failure notification due to missing inputs",
			"work_item_id", item.ID,
			"notifier_nil", wp.notifier == nil,
			"flow_nil", f == nil,
			"err_nil", execErr == nil,
		)
		return
	}

	details := notifier.FailureDetails{
		FlowID:      f.ID,
		FlowName:    f.Name,
		CreatedBy:   f.CreatedBy,
		ErrorMsg:    execErr.Error(),
		ExecutionID: item.ID,
	}
	if ch := item.TriggerData.Channel; ch != nil {
		details.ChannelID = ch.Id
		// Prefer DisplayName for readability; fall back to Name (handle).
		if ch.DisplayName != "" {
			details.ChannelDisplayName = ch.DisplayName
		} else {
			details.ChannelDisplayName = ch.Name
		}
	}

	var actionErr *flow.ActionError
	if errors.As(execErr, &actionErr) {
		details.ActionID = actionErr.ActionID
		details.ActionType = actionErr.ActionType
		details.ErrorMsg = actionErr.Err.Error()
	}

	wp.notifier.NotifyFailure(details)
}

func (wp *WorkerPool) saveExecutionRecord(item *model.WorkItem, ctx *model.FlowContext, execErr error, completedAt int64) {
	if wp.historyStore == nil {
		return
	}

	record := &model.ExecutionRecord{
		ID:          item.ID,
		FlowID:      item.FlowID,
		FlowName:    item.FlowName,
		Status:      "success",
		TriggerData: item.TriggerData,
		CreatedAt:   item.CreatedAt,
		StartedAt:   item.StartedAt,
		CompletedAt: completedAt,
	}
	if execErr != nil {
		record.Status = "failed"
		record.Error = execErr.Error()
	}
	if ctx != nil {
		record.Steps = ctx.Steps
	}

	if err := wp.historyStore.Save(record); err != nil {
		wp.api.LogError("Failed to save execution record",
			"work_item_id", item.ID,
			"err", err.Error(),
		)
	}
}
