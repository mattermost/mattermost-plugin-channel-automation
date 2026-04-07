package model

// Configuration provides plugin configuration values needed by the automation layer.
type Configuration interface {
	MaxAutomationsPerChannel() int
}
