package model

// Store defines the interface for flow persistence.
type Store interface {
	Get(id string) (*Flow, error)
	List() ([]*Flow, error)
	ListByTriggerChannel(channelID string) ([]*Flow, error)
	ListScheduled() ([]*Flow, error)
	Save(flow *Flow) error
	Delete(id string) error
	CountByTriggerChannel(channelID string) (int, error)
	GetFlowIDsForChannel(channelID string) ([]string, error)
	GetFlowIDsForMembershipChannel(channelID string) ([]string, error)
	GetChannelCreatedFlowIDs() ([]string, error)
}
