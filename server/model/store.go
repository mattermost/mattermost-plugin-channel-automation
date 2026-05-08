package model

// Store defines the interface for flow persistence.
type Store interface {
	Get(id string) (*Flow, error)
	List() ([]*Flow, error)
	ListByTriggerChannel(channelID string) ([]*Flow, error)
	ListScheduled() ([]*Flow, error)
	Save(flow *Flow) error
	// SaveWithChannelLimit persists the flow atomically with respect to a
	// per-channel quota check. limit <= 0 disables the check. excludeID,
	// when non-empty, names the flow being updated so it does not count
	// against itself when targeting the same channel.
	//
	// Returns flow.ErrChannelFlowLimitExceeded when persisting would push
	// the count above limit; the flow is not saved in that case. All
	// other returned errors indicate backend failures.
	//
	// When the flow has no trigger channel (e.g. channel_created or
	// user_joined_team triggers), the quota does not apply and the call
	// degrades to a plain Save.
	SaveWithChannelLimit(flow *Flow, limit int, excludeID string) error
	Delete(id string) error
	CountByTriggerChannel(channelID string) (int, error)
	GetFlowIDsForChannel(channelID string) ([]string, error)
	GetFlowIDsForMembershipChannel(channelID string) ([]string, error)
	GetChannelCreatedFlowIDs() ([]string, error)
	GetFlowIDsForUserJoinedTeam(teamID string) ([]string, error)
}
