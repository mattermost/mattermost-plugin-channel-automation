package model

// Trigger type constants.
const (
	TriggerTypeMessagePosted     = "message_posted"
	TriggerTypeSchedule          = "schedule"
	TriggerTypeMembershipChanged = "membership_changed"
	TriggerTypeChannelCreated    = "channel_created"
	TriggerTypeUserJoinedTeam    = "user_joined_team"
)

// TriggerHandler evaluates whether a trigger matches an incoming event.
type TriggerHandler interface {
	Type() string
	Matches(trigger *Trigger, event *Event) bool
}
