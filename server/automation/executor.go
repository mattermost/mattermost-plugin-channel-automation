package automation

import (
	"fmt"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

// AutomationExecutor dispatches automation actions using the registry.
type AutomationExecutor struct {
	registry *Registry
}

// NewAutomationExecutor creates an AutomationExecutor with the given registry.
func NewAutomationExecutor(registry *Registry) *AutomationExecutor {
	return &AutomationExecutor{registry: registry}
}

// Execute runs all actions in the automation sequentially, building up the AutomationContext.
// Returns the context (with any partial step outputs) and an error on the first
// failure or if an action type is unknown.
func (e *AutomationExecutor) Execute(a *model.Automation, triggerData model.TriggerData) (*model.AutomationContext, error) {
	ctx := &model.AutomationContext{
		CreatedBy: a.CreatedBy,
		Trigger:   triggerData,
		Steps:     make(map[string]model.StepOutput),
	}

	for _, action := range a.Actions {
		handler, ok := e.registry.GetAction(action.Type())
		if !ok {
			return ctx, fmt.Errorf("unknown action type %q for action %q", action.Type(), action.ID)
		}

		output, err := handler.Execute(&action, ctx)
		if err != nil {
			return ctx, fmt.Errorf("action %q failed: %w", action.ID, err)
		}

		if output != nil {
			ctx.Steps[action.ID] = *output
		}
	}

	return ctx, nil
}
