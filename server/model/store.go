package model

import "errors"

// ErrChannelAutomationLimitExceeded is returned by Store.SaveWithChannelLimit
// when persisting the automation would push the count of automations on its
// trigger channel above the supplied limit. The sentinel lives alongside the
// interface so callers can branch on it via errors.Is without importing any
// concrete store implementation.
var ErrChannelAutomationLimitExceeded = errors.New("channel automation limit exceeded")

// Store defines the interface for automation persistence.
type Store interface {
	Get(id string) (*Automation, error)
	List() ([]*Automation, error)
	ListByTriggerChannel(channelID string) ([]*Automation, error)
	ListScheduled() ([]*Automation, error)
	Save(automation *Automation) error
	// SaveWithChannelLimit persists the automation atomically with respect
	// to a per-channel quota check. limit <= 0 disables the check.
	// excludeID, when non-empty, names the automation being updated so it
	// does not count against itself when targeting the same channel.
	//
	// Returns ErrChannelAutomationLimitExceeded when persisting would push
	// the count above limit; the automation is not saved in that case. All
	// other returned errors indicate backend failures.
	//
	// When the automation has no trigger channel (e.g. channel_created or
	// user_joined_team triggers), the quota does not apply and the call
	// degrades to a plain Save.
	SaveWithChannelLimit(automation *Automation, limit int, excludeID string) error
	Delete(id string) error
	CountByTriggerChannel(channelID string) (int, error)
	GetAutomationIDsForChannel(channelID string) ([]string, error)
	GetAutomationIDsForMembershipChannel(channelID string) ([]string, error)
	GetChannelCreatedAutomationIDs() ([]string, error)
	GetAutomationIDsForUserJoinedTeam(teamID string) ([]string, error)
}
