package workqueue

import (
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	mmmodel "github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/automation"
	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
	"github.com/mattermost/mattermost-plugin-channel-automation/server/permissions"
)

// WorkerPool processes work items from the queue using a bounded pool
// of concurrent goroutines.
type WorkerPool struct {
	store           *Store
	executor        *automation.AutomationExecutor
	automationStore model.Store
	historyStore    model.ExecutionStore
	api             plugin.API
	maxWorkers      int
	pollInterval    time.Duration
	notify          chan struct{}
	stop            chan struct{}
	wg              sync.WaitGroup
	startOnce       sync.Once
	stopOnce        sync.Once
}

// NewWorkerPool creates a WorkerPool. Call Start to begin processing.
func NewWorkerPool(store *Store, executor *automation.AutomationExecutor, automationStore model.Store, historyStore model.ExecutionStore, api plugin.API, maxWorkers int) *WorkerPool {
	return &WorkerPool{
		store:           store,
		executor:        executor,
		automationStore: automationStore,
		historyStore:    historyStore,
		api:             api,
		maxWorkers:      maxWorkers,
		pollInterval:    30 * time.Second,
		notify:          make(chan struct{}, 1),
		stop:            make(chan struct{}),
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
				"automation_id", item.AutomationID,
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

	a, err := wp.automationStore.Get(item.AutomationID)
	if err != nil {
		wp.api.LogError("Failed to get automation for work item",
			"work_item_id", item.ID,
			"automation_id", item.AutomationID,
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

	// Automation was deleted or disabled between enqueue and execute — silently complete.
	if a == nil || !a.Enabled {
		if err := wp.store.Complete(item.ID); err != nil {
			wp.api.LogError("Failed to complete work item for deleted/disabled automation",
				"work_item_id", item.ID,
				"automation_id", item.AutomationID,
				"err", err.Error(),
			)
		}
		return
	}

	// Check that the automation creator is still an active user.
	creator, appErr := wp.api.GetUser(a.CreatedBy)
	if appErr != nil {
		if appErr.StatusCode == http.StatusNotFound {
			// Creator has been permanently deleted — disable the automation.
			wp.disableAutomation(a, item, "automation creator account has been deactivated or deleted")
			return
		}

		// Transient API error — fail this execution but leave the automation enabled.
		reason := fmt.Sprintf("failed to look up automation creator %q: %s", a.CreatedBy, appErr.Error())
		wp.api.LogError("Failed to verify automation creator",
			"work_item_id", item.ID,
			"automation_id", a.ID,
			"automation_name", a.Name,
			"created_by", a.CreatedBy,
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
		// Creator has been deactivated — disable the automation.
		wp.disableAutomation(a, item, "automation creator account has been deactivated or deleted")
		return
	}

	// Verify the creator still has the required permissions (e.g. channel
	// admin, team admin, or system admin) — the same check performed at
	// automation creation time.
	if permErr := permissions.CheckAutomationPermissions(wp.api, a.CreatedBy, a); permErr != nil {
		var appErr *mmmodel.AppError
		if errors.As(permErr, &appErr) {
			// Transient API error — fail this execution but leave the automation enabled.
			wp.api.LogError("Failed to verify automation creator permissions",
				"work_item_id", item.ID,
				"automation_id", a.ID,
				"automation_name", a.Name,
				"created_by", a.CreatedBy,
				"err", permErr.Error(),
			)
			if storeErr := wp.store.Fail(item.ID); storeErr != nil {
				wp.api.LogError("Failed to remove work item after permission check error",
					"work_item_id", item.ID,
					"err", storeErr.Error(),
				)
			}
			wp.saveExecutionRecord(item, nil, fmt.Errorf("failed to verify automation creator permissions: %s", permErr.Error()), model.NowTimestamp())
			return
		}

		// Creator lost the required permissions — disable the automation.
		wp.disableAutomation(a, item, fmt.Sprintf("automation creator no longer has the required permissions: %s", permErr.Error()))
		return
	}

	ctx, execErr := wp.executor.Execute(a, item.TriggerData)
	completedAt := model.NowTimestamp()

	if execErr != nil {
		wp.api.LogError("Automation execution failed",
			"work_item_id", item.ID,
			"automation_id", item.AutomationID,
			"automation_name", item.AutomationName,
			"err", execErr.Error(),
		)
		if storeErr := wp.store.Fail(item.ID); storeErr != nil {
			wp.api.LogError("Failed to mark work item as failed",
				"work_item_id", item.ID,
				"err", storeErr.Error(),
			)
		}
	} else {
		if err := wp.store.Complete(item.ID); err != nil {
			wp.api.LogError("Failed to complete work item",
				"work_item_id", item.ID,
				"err", err.Error(),
			)
		}

		wp.api.LogInfo("Automation executed successfully",
			"work_item_id", item.ID,
			"automation_id", item.AutomationID,
			"automation_name", item.AutomationName,
		)
	}

	wp.saveExecutionRecord(item, ctx, execErr, completedAt)
}

func (wp *WorkerPool) disableAutomation(a *model.Automation, item *model.WorkItem, reason string) {
	wp.api.LogWarn("Disabling automation",
		"automation_id", a.ID,
		"automation_name", a.Name,
		"created_by", a.CreatedBy,
		"reason", reason,
	)

	a.Enabled = false
	if saveErr := wp.automationStore.Save(a); saveErr != nil {
		wp.api.LogError("Failed to disable automation with inactive creator",
			"automation_id", a.ID,
			"err", saveErr.Error(),
		)
	}

	if storeErr := wp.store.Fail(item.ID); storeErr != nil {
		wp.api.LogError("Failed to remove work item after disabling automation",
			"work_item_id", item.ID,
			"automation_id", a.ID,
			"err", storeErr.Error(),
		)
	}
	wp.saveExecutionRecord(item, nil, fmt.Errorf("%s", reason), model.NowTimestamp())
}

func (wp *WorkerPool) saveExecutionRecord(item *model.WorkItem, ctx *model.AutomationContext, execErr error, completedAt int64) {
	if wp.historyStore == nil {
		return
	}

	record := &model.ExecutionRecord{
		ID:             item.ID,
		AutomationID:   item.AutomationID,
		AutomationName: item.AutomationName,
		Status:         "success",
		TriggerData:    item.TriggerData,
		CreatedAt:      item.CreatedAt,
		StartedAt:      item.StartedAt,
		CompletedAt:    completedAt,
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
