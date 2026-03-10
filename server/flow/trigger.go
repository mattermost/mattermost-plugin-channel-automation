package flow

import (
	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

// TriggerService evaluates incoming events against stored flows.
type TriggerService struct {
	store    model.Store
	registry *Registry
}

// NewTriggerService creates a new TriggerService.
func NewTriggerService(store model.Store, registry *Registry) *TriggerService {
	return &TriggerService{store: store, registry: registry}
}

// FindMatchingFlows returns all enabled flows whose trigger matches the given event.
func (t *TriggerService) FindMatchingFlows(event *model.Event) ([]*model.Flow, error) {
	handler, ok := t.registry.GetTrigger(event.Type)
	if !ok {
		return nil, nil
	}

	var candidateIDs []string
	var err error

	switch event.Type {
	case "message_posted":
		if event.Post == nil {
			return nil, nil
		}
		candidateIDs, err = t.store.GetFlowIDsForChannel(event.Post.ChannelId)
	case "membership_changed":
		if event.Channel == nil {
			return nil, nil
		}
		candidateIDs, err = t.store.GetFlowIDsForMembershipChannel(event.Channel.Id)
	case "channel_created":
		if event.Channel == nil {
			return nil, nil
		}
		candidateIDs, err = t.store.GetChannelCreatedFlowIDs()
	default:
		return nil, nil
	}

	if err != nil {
		return nil, err
	}
	if len(candidateIDs) == 0 {
		return nil, nil
	}

	var flows []*model.Flow
	for _, id := range candidateIDs {
		f, err := t.store.Get(id)
		if err != nil {
			return nil, err
		}
		if f == nil {
			continue
		}
		if !f.Enabled {
			continue
		}
		if !handler.Matches(&f.Trigger, event) {
			continue
		}
		flows = append(flows, f)
	}
	return flows, nil
}
