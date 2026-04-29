package automation

import (
	"fmt"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

// TriggerService evaluates incoming events against stored automations.
type TriggerService struct {
	store    model.Store
	registry *Registry
}

// NewTriggerService creates a new TriggerService.
func NewTriggerService(store model.Store, registry *Registry) *TriggerService {
	return &TriggerService{store: store, registry: registry}
}

// FindMatchingAutomations returns all enabled automations whose trigger matches the given event.
func (t *TriggerService) FindMatchingAutomations(event *model.Event) ([]*model.Automation, error) {
	handler, ok := t.registry.GetTrigger(event.Type)
	if !ok {
		return nil, nil
	}

	var candidateIDs []string
	var err error

	switch event.Type {
	case model.TriggerTypeMessagePosted:
		if event.Post == nil {
			return nil, nil
		}
		candidateIDs, err = t.store.GetAutomationIDsForChannel(event.Post.ChannelId)
	case model.TriggerTypeMembershipChanged:
		if event.Channel == nil {
			return nil, nil
		}
		candidateIDs, err = t.store.GetAutomationIDsForMembershipChannel(event.Channel.Id)
	case model.TriggerTypeChannelCreated:
		if event.Channel == nil {
			return nil, nil
		}
		candidateIDs, err = t.store.GetChannelCreatedAutomationIDs()
	case model.TriggerTypeUserJoinedTeam:
		if event.Team == nil || event.Team.Id == "" {
			return nil, nil
		}
		candidateIDs, err = t.store.GetAutomationIDsForUserJoinedTeam(event.Team.Id)
	default:
		return nil, fmt.Errorf("trigger type %q is registered but has no candidate resolution logic", event.Type)
	}

	if err != nil {
		return nil, err
	}
	if len(candidateIDs) == 0 {
		return nil, nil
	}

	var automations []*model.Automation
	for _, id := range candidateIDs {
		a, err := t.store.Get(id)
		if err != nil {
			return nil, err
		}
		if a == nil {
			continue
		}
		if !a.Enabled {
			continue
		}
		if !handler.Matches(&a.Trigger, event) {
			continue
		}
		automations = append(automations, a)
	}
	return automations, nil
}
