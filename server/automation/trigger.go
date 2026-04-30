package automation

import (
	"fmt"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

// TriggerService evaluates incoming events against stored flows.
type TriggerService struct {
	store    model.Store
	registry *Registry
}

// NewTriggerService creates a new TriggerService.
func NewTriggerService(store model.Store, registry *Registry) *TriggerService {
	return &TriggerService{store: store, registry: registry}
}

// FindMatchingAutomations returns the registered handler for the event type along
// with all enabled flows whose trigger matches the event. The handler is
// returned so callers can drive the rest of the trigger lifecycle (e.g.
// BuildTriggerData) without a second registry lookup, which would risk a
// silent divergence if the registry mutated between calls.
//
// Returns an error if no handler is registered for the event type — this is
// a programming/wiring error (a hook fired for a type nobody registered)
// and must not be silently dropped.
func (t *TriggerService) FindMatchingAutomations(event *model.Event) (model.TriggerHandler, []*model.Automation, error) {
	handler, ok := t.registry.GetTrigger(event.Type)
	if !ok {
		return nil, nil, fmt.Errorf("no trigger handler registered for event type %q", event.Type)
	}

	candidateIDs, err := handler.CandidateAutomationIDs(t.store, event)
	if err != nil {
		return handler, nil, err
	}
	if len(candidateIDs) == 0 {
		return handler, nil, nil
	}

	var flows []*model.Automation
	for _, id := range candidateIDs {
		f, err := t.store.Get(id)
		if err != nil {
			return handler, nil, err
		}
		if f == nil {
			continue
		}
		if !f.Enabled {
			continue
		}
		if !handler.Matches(&f.Trigger, event) {
			continue
		}
		flows = append(flows, f)
	}
	return handler, flows, nil
}
