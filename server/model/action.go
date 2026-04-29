package model

// ActionHandler executes a single action step within an automation.
type ActionHandler interface {
	Type() string
	Execute(action *Action, ctx *AutomationContext) (*StepOutput, error)
}
