package automation

import (
	"fmt"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

// ValidateTrigger validates a trigger configuration: mutual exclusion of
// trigger types first, then the per-type checks owned by the registered
// handler. An optional existing trigger can be passed for update scenarios
// (some handlers — e.g. Schedule — only validate changed fields).
func ValidateTrigger(registry *Registry, t *model.Trigger, existing *model.Trigger) error {
	if err := model.ValidateTriggerExclusivity(t); err != nil {
		return err
	}

	typ := t.Type()
	handler, ok := registry.GetTrigger(typ)
	if !ok {
		return fmt.Errorf("unknown trigger type: %s", typ)
	}
	return handler.Validate(t, existing)
}
