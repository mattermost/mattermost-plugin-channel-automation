package model

// Store defines the interface for flow persistence.
type Store interface {
	Get(id string) (*Flow, error)
	List() ([]*Flow, error)
	ListByTriggerChannel(channelID string) ([]*Flow, error)
	ListScheduled() ([]*Flow, error)
	Save(flow *Flow) error
	Delete(id string) error
	GetFlowIDsForChannel(channelID string) ([]string, error)
}
