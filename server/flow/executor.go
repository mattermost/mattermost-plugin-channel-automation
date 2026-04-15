package flow

import (
	"fmt"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

// FlowExecutor dispatches flow actions using the registry.
type FlowExecutor struct {
	registry *Registry
}

// NewFlowExecutor creates a FlowExecutor with the given registry.
func NewFlowExecutor(registry *Registry) *FlowExecutor {
	return &FlowExecutor{registry: registry}
}

// Execute runs all actions in the flow sequentially, building up the FlowContext.
// teamBotUserID is the resolved bot user ID for flows with a TeamBotConfig; empty otherwise.
// Returns the context (with any partial step outputs) and an error on the first
// failure or if an action type is unknown.
func (e *FlowExecutor) Execute(f *model.Flow, triggerData model.TriggerData, teamBotUserID string) (*model.FlowContext, error) {
	ctx := &model.FlowContext{
		CreatedBy:     f.CreatedBy,
		TeamBotUserID: teamBotUserID,
		Trigger:       triggerData,
		Steps:         make(map[string]model.StepOutput),
	}

	for _, action := range f.Actions {
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
