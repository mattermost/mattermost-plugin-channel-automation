package workqueue

import (
	"fmt"
	"sync"
	"time"

	"github.com/mattermost/mattermost/server/public/plugin"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/flow"
	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

// WorkerPool processes work items from the queue using a bounded pool
// of concurrent goroutines.
type WorkerPool struct {
	store        *Store
	executor     *flow.FlowExecutor
	flowStore    model.Store
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
func NewWorkerPool(store *Store, executor *flow.FlowExecutor, flowStore model.Store, api plugin.API, maxWorkers int) *WorkerPool {
	return &WorkerPool{
		store:        store,
		executor:     executor,
		flowStore:    flowStore,
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
			if err := wp.store.Fail(item.ID, errMsg); err != nil {
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
		if storeErr := wp.store.Fail(item.ID, err.Error()); storeErr != nil {
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

	if err := wp.executor.Execute(f, item.TriggerData); err != nil {
		wp.api.LogError("Flow execution failed",
			"work_item_id", item.ID,
			"flow_id", item.FlowID,
			"flow_name", item.FlowName,
			"err", err.Error(),
		)
		if storeErr := wp.store.Fail(item.ID, err.Error()); storeErr != nil {
			wp.api.LogError("Failed to mark work item as failed",
				"work_item_id", item.ID,
				"err", storeErr.Error(),
			)
		}
		return
	}

	if err := wp.store.Complete(item.ID); err != nil {
		wp.api.LogError("Failed to complete work item",
			"work_item_id", item.ID,
			"err", err.Error(),
		)
	}

	wp.api.LogInfo("Flow executed successfully",
		"work_item_id", item.ID,
		"flow_id", item.FlowID,
		"flow_name", item.FlowName,
	)
}
