package flow

import (
	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

// Registry is populated once during OnActivate and read-only thereafter.
type Registry struct {
	triggers map[string]model.TriggerHandler
	actions  map[string]model.ActionHandler
}

// NewRegistry creates an empty Registry.
func NewRegistry() *Registry {
	return &Registry{
		triggers: make(map[string]model.TriggerHandler),
		actions:  make(map[string]model.ActionHandler),
	}
}

// RegisterTrigger adds a trigger handler to the registry.
func (r *Registry) RegisterTrigger(h model.TriggerHandler) {
	r.triggers[h.Type()] = h
}

// RegisterAction adds an action handler to the registry.
func (r *Registry) RegisterAction(h model.ActionHandler) {
	r.actions[h.Type()] = h
}

// GetTrigger returns the handler for the given trigger type.
func (r *Registry) GetTrigger(typ string) (model.TriggerHandler, bool) {
	h, ok := r.triggers[typ]
	return h, ok
}

// GetAction returns the handler for the given action type.
func (r *Registry) GetAction(typ string) (model.ActionHandler, bool) {
	h, ok := r.actions[typ]
	return h, ok
}
