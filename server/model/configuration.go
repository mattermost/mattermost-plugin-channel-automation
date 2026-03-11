package model

// Configuration provides plugin configuration values needed by the flow layer.
type Configuration interface {
	MaxFlowsPerChannel() int
}
