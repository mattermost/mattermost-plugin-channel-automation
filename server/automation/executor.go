package automation

import (
	"fmt"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

// ActionError is returned by AutomationExecutor.Execute when a specific action
// fails. It carries the failing action's ID and type so callers can surface
// targeted error notifications without parsing the wrapped error string.
type ActionError struct {
	ActionID   string
	ActionType string
	Err        error
}

func (e *ActionError) Error() string {
	return fmt.Sprintf("action %q failed: %s", e.ActionID, e.Err)
}

func (e *ActionError) Unwrap() error { return e.Err }

// AutomationExecutor dispatches flow actions using the registry.
type AutomationExecutor struct {
	registry *Registry
}

// NewAutomationExecutor creates a AutomationExecutor with the given registry.
func NewAutomationExecutor(registry *Registry) *AutomationExecutor {
	return &AutomationExecutor{registry: registry}
}

// Execute runs all actions in the flow sequentially, building up the AutomationContext.
// Returns the context (with any partial step outputs) and an error on the first
// failure or if an action type is unknown.
func (e *AutomationExecutor) Execute(f *model.Automation, triggerData model.TriggerData) (*model.AutomationContext, error) {
	ctx := &model.AutomationContext{
		AutomationID: f.ID,
		CreatedBy:    f.CreatedBy,
		Trigger:      triggerData,
		Steps:        make(map[string]model.StepOutput),
	}

	for _, action := range f.Actions {
		handler, ok := e.registry.GetAction(action.Type())
		if !ok {
			return ctx, fmt.Errorf("unknown action type %q for action %q", action.Type(), action.ID)
		}

		output, err := handler.Execute(&action, ctx)
		if err != nil {
			return ctx, &ActionError{
				ActionID:   action.ID,
				ActionType: action.Type(),
				Err:        err,
			}
		}

		if output != nil {
			ctx.Steps[action.ID] = *output
		}
	}

	return ctx, nil
}
