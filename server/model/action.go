package model

// ActionHandler executes a single action step within a flow.
type ActionHandler interface {
	Type() string
	Execute(action *Action, ctx *FlowContext) (*StepOutput, error)
}
