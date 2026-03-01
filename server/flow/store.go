package flow

import (
	"encoding/json"
	"fmt"
	"slices"

	"github.com/mattermost/mattermost/server/public/plugin"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

const (
	flowKeyPrefix      = "flow:"
	flowIndexKey       = "flow_index"
	triggerIndexPrefix = "ti:mp:" // shortened to stay within 50-char KV key limit (6 + 26 = 32)
)

// KVStore implements Store using the Mattermost plugin KV store.
type KVStore struct {
	api plugin.API
}

// NewStore creates a new KV-backed flow store.
func NewStore(api plugin.API) model.Store {
	return &KVStore{api: api}
}

func (s *KVStore) Get(id string) (*model.Flow, error) {
	data, appErr := s.api.KVGet(flowKeyPrefix + id)
	if appErr != nil {
		return nil, fmt.Errorf("failed to get flow %s: %w", id, appErr)
	}
	if data == nil {
		return nil, nil
	}

	var f model.Flow
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("failed to unmarshal flow %s: %w", id, err)
	}
	return &f, nil
}

func (s *KVStore) List() ([]*model.Flow, error) {
	ids, err := s.getIndex()
	if err != nil {
		return nil, err
	}

	flows := make([]*model.Flow, 0, len(ids))
	for _, id := range ids {
		f, err := s.Get(id)
		if err != nil {
			return nil, err
		}
		if f != nil {
			flows = append(flows, f)
		}
	}
	return flows, nil
}

func (s *KVStore) Save(f *model.Flow) error {
	// Read old flow to clean up stale trigger index entries.
	old, err := s.Get(f.ID)
	if err != nil {
		return err
	}

	data, err := json.Marshal(f)
	if err != nil {
		return fmt.Errorf("failed to marshal flow %s: %w", f.ID, err)
	}

	if appErr := s.api.KVSet(flowKeyPrefix+f.ID, data); appErr != nil {
		return fmt.Errorf("failed to save flow %s: %w", f.ID, appErr)
	}

	// Update the flow index (add if new).
	if old == nil {
		if err := s.addToIndex(f.ID); err != nil {
			return err
		}
	}

	// Update trigger index: remove old entry, add new entry.
	if old != nil && old.Trigger.ChannelID != "" {
		if err := s.removeTriggerIndex(old.Trigger.ChannelID, f.ID); err != nil {
			return err
		}
	}
	if f.Trigger.ChannelID != "" {
		if err := s.addTriggerIndex(f.Trigger.ChannelID, f.ID); err != nil {
			return err
		}
	}

	return nil
}

func (s *KVStore) Delete(id string) error {
	f, err := s.Get(id)
	if err != nil {
		return err
	}
	if f == nil {
		return nil
	}

	if appErr := s.api.KVDelete(flowKeyPrefix + id); appErr != nil {
		return fmt.Errorf("failed to delete flow %s: %w", id, appErr)
	}

	if err := s.removeFromIndex(id); err != nil {
		return err
	}

	if f.Trigger.ChannelID != "" {
		if err := s.removeTriggerIndex(f.Trigger.ChannelID, id); err != nil {
			return err
		}
	}

	return nil
}

// GetFlowIDsForChannel returns flow IDs triggered by messages in the given channel.
func (s *KVStore) GetFlowIDsForChannel(channelID string) ([]string, error) {
	return s.getTriggerIndex(channelID)
}

// --- Index helpers ---

func (s *KVStore) getIndex() ([]string, error) {
	data, appErr := s.api.KVGet(flowIndexKey)
	if appErr != nil {
		return nil, fmt.Errorf("failed to get flow index: %w", appErr)
	}
	if data == nil {
		return nil, nil
	}

	var ids []string
	if err := json.Unmarshal(data, &ids); err != nil {
		return nil, fmt.Errorf("failed to unmarshal flow index: %w", err)
	}
	return ids, nil
}

func (s *KVStore) setIndex(ids []string) error {
	data, err := json.Marshal(ids)
	if err != nil {
		return fmt.Errorf("failed to marshal flow index: %w", err)
	}
	if appErr := s.api.KVSet(flowIndexKey, data); appErr != nil {
		return fmt.Errorf("failed to save flow index: %w", appErr)
	}
	return nil
}

func (s *KVStore) addToIndex(id string) error {
	ids, err := s.getIndex()
	if err != nil {
		return err
	}
	ids = append(ids, id)
	return s.setIndex(ids)
}

func (s *KVStore) removeFromIndex(id string) error {
	ids, err := s.getIndex()
	if err != nil {
		return err
	}
	filtered := make([]string, 0, len(ids))
	for _, existingID := range ids {
		if existingID != id {
			filtered = append(filtered, existingID)
		}
	}
	return s.setIndex(filtered)
}

func makeTriggerIndexKey(channelID string) string {
	return triggerIndexPrefix + channelID
}

func (s *KVStore) getTriggerIndex(channelID string) ([]string, error) {
	key := makeTriggerIndexKey(channelID)
	data, appErr := s.api.KVGet(key)
	if appErr != nil {
		return nil, fmt.Errorf("failed to get trigger index: %w", appErr)
	}
	if data == nil {
		return nil, nil
	}

	var ids []string
	if err := json.Unmarshal(data, &ids); err != nil {
		return nil, fmt.Errorf("failed to unmarshal trigger index: %w", err)
	}
	return ids, nil
}

func (s *KVStore) setTriggerIndex(channelID string, ids []string) error {
	key := makeTriggerIndexKey(channelID)
	if len(ids) == 0 {
		if appErr := s.api.KVDelete(key); appErr != nil {
			return fmt.Errorf("failed to delete trigger index: %w", appErr)
		}
		return nil
	}
	data, err := json.Marshal(ids)
	if err != nil {
		return fmt.Errorf("failed to marshal trigger index: %w", err)
	}
	if appErr := s.api.KVSet(key, data); appErr != nil {
		return fmt.Errorf("failed to save trigger index: %w", appErr)
	}
	return nil
}

func (s *KVStore) addTriggerIndex(channelID, flowID string) error {
	ids, err := s.getTriggerIndex(channelID)
	if err != nil {
		return err
	}
	if slices.Contains(ids, flowID) {
		return nil
	}
	ids = append(ids, flowID)
	return s.setTriggerIndex(channelID, ids)
}

func (s *KVStore) removeTriggerIndex(channelID, flowID string) error {
	ids, err := s.getTriggerIndex(channelID)
	if err != nil {
		return err
	}
	filtered := make([]string, 0, len(ids))
	for _, id := range ids {
		if id != flowID {
			filtered = append(filtered, id)
		}
	}
	return s.setTriggerIndex(channelID, filtered)
}
