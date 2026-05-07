package automation

import (
	mmmodel "github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

// Dispatcher resolves matching automations for an event, builds the trigger data
// once, and enqueues a work item per matched automation. It is the single path
// from plugin hooks to the work queue for event-driven triggers.
type Dispatcher struct {
	api            plugin.API
	triggerService *TriggerService
	enqueuer       WorkItemEnqueuer
	notifier       WorkerNotifier
}

// NewDispatcher creates a Dispatcher.
func NewDispatcher(api plugin.API, triggerService *TriggerService, enqueuer WorkItemEnqueuer, notifier WorkerNotifier) *Dispatcher {
	return &Dispatcher{
		api:            api,
		triggerService: triggerService,
		enqueuer:       enqueuer,
		notifier:       notifier,
	}
}

// Dispatch finds matching automations for the event, builds TriggerData via the
// registered handler, and enqueues a work item per matched automation. Logs and
// returns silently on errors. Avoids blocking the Mattermost server.
func (d *Dispatcher) Dispatch(event *model.Event) {
	if event == nil {
		d.api.LogError("Dispatch called with nil event")
		return
	}

	handler, automations, err := d.triggerService.FindMatchingAutomations(event)
	if err != nil {
		d.api.LogError("Failed to find matching automations", "type", event.Type, "err", err.Error())
		return
	}
	if len(automations) == 0 {
		return
	}

	triggerData, err := handler.BuildTriggerData(d.api, event)
	if err != nil {
		d.api.LogError("Failed to build trigger data", "type", event.Type, "err", err.Error())
		return
	}

	failures := 0
	for _, a := range automations {
		item := &model.WorkItem{
			ID:             mmmodel.NewId(),
			AutomationID:   a.ID,
			AutomationName: a.Name,
			TriggerData:    triggerData,
		}
		if err := d.enqueuer.Enqueue(item); err != nil {
			d.api.LogError("Failed to enqueue work item",
				"automation_id", a.ID,
				"type", event.Type,
				"err", err.Error(),
			)
			failures++
			continue
		}
		d.api.LogDebug("Work item enqueued",
			"work_item_id", item.ID,
			"automation_id", a.ID,
			"type", event.Type,
		)
	}

	if failures > 0 {
		d.api.LogError("Some work items failed to enqueue",
			"type", event.Type,
			"total", len(automations),
			"failed", failures,
		)
	}

	// Notify regardless of per-item failures: items persisted by concurrent
	// producers should still wake the pool.
	d.notifier.Notify()
}
