package model

// TriggerHandler evaluates whether a trigger matches an incoming event.
type TriggerHandler interface {
	Type() string
	Matches(trigger *Trigger, event *Event) bool
}
