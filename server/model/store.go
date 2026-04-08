package model

// Store defines the interface for automation persistence.
type Store interface {
	Get(id string) (*Automation, error)
	List() ([]*Automation, error)
	ListByTriggerChannel(channelID string) ([]*Automation, error)
	ListScheduled() ([]*Automation, error)
	Save(automation *Automation) error
	Delete(id string) error
	CountByTriggerChannel(channelID string) (int, error)
	GetAutomationIDsForChannel(channelID string) ([]string, error)
	GetAutomationIDsForMembershipChannel(channelID string) ([]string, error)
	GetChannelCreatedAutomationIDs() ([]string, error)
}
