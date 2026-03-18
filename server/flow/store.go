package flow

import (
	"encoding/json"
	"fmt"
	"slices"
	"sync"

	"github.com/mattermost/mattermost/server/public/plugin"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

const (
	flowKeyPrefix                = "flow:"
	flowIndexKey                 = "flow_index"
	triggerIndexPrefix           = "ti:mp:" // shortened to stay within 50-char KV key limit (6 + 26 = 32)
	channelTriggerIndexPrefix    = "ct:"    // all trigger types by channel (3 + 26 = 29)
	membershipTriggerIndexPrefix = "ti:mc:" // membership_changed by channel (6 + 26 = 32)
	scheduleIndexKey             = "sched_index"
	channelCreatedIndexKey       = "cc_index"
	userJoinedTeamIndexKey       = "ujt_index"
)

// KVStore implements Store using the Mattermost plugin KV store.
// Index mutations are serialized via indexMu to prevent lost updates
// when multiple goroutines operate on the same store.
type KVStore struct {
	api     plugin.API
	indexMu sync.Locker
}

// NewStore creates a new KV-backed flow store. The caller must supply
// a cluster-safe mutex (e.g. cluster.Mutex) to protect index
// read-modify-write cycles across plugin instances.
func NewStore(api plugin.API, indexMu sync.Locker) model.Store {
	return &KVStore{api: api, indexMu: indexMu}
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

func (s *KVStore) ListByTriggerChannel(channelID string) ([]*model.Flow, error) {
	ids, err := s.getChannelTriggerIndex(channelID)
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

func (s *KVStore) ListScheduled() ([]*model.Flow, error) {
	ids, err := s.getScheduleIndex()
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
	if old != nil && old.Trigger.MessagePosted != nil && old.Trigger.MessagePosted.ChannelID != "" {
		if err := s.removeTriggerIndex(old.Trigger.MessagePosted.ChannelID, f.ID); err != nil {
			return err
		}
	}
	if f.Trigger.MessagePosted != nil && f.Trigger.MessagePosted.ChannelID != "" {
		if err := s.addTriggerIndex(f.Trigger.MessagePosted.ChannelID, f.ID); err != nil {
			return err
		}
	}

	// Update membership trigger index: remove old entry, add new entry.
	if old != nil && old.Trigger.MembershipChanged != nil && old.Trigger.MembershipChanged.ChannelID != "" {
		if err := s.removeMembershipTriggerIndex(old.Trigger.MembershipChanged.ChannelID, f.ID); err != nil {
			return err
		}
	}
	if f.Trigger.MembershipChanged != nil && f.Trigger.MembershipChanged.ChannelID != "" {
		if err := s.addMembershipTriggerIndex(f.Trigger.MembershipChanged.ChannelID, f.ID); err != nil {
			return err
		}
	}

	// Update channel-trigger index (covers all trigger types).
	if old != nil {
		if oldChID := old.TriggerChannelID(); oldChID != "" {
			if err := s.removeChannelTriggerIndex(oldChID, f.ID); err != nil {
				return err
			}
		}
	}
	if newChID := f.TriggerChannelID(); newChID != "" {
		if err := s.addChannelTriggerIndex(newChID, f.ID); err != nil {
			return err
		}
	}

	// Update schedule index.
	oldHasSchedule := old != nil && old.Trigger.Schedule != nil
	newHasSchedule := f.Trigger.Schedule != nil

	if oldHasSchedule && !newHasSchedule {
		if err := s.removeFromScheduleIndex(f.ID); err != nil {
			return err
		}
	} else if !oldHasSchedule && newHasSchedule {
		if err := s.addToScheduleIndex(f.ID); err != nil {
			return err
		}
	}

	// Update channel-created index.
	oldHasChannelCreated := old != nil && old.Trigger.ChannelCreated != nil
	newHasChannelCreated := f.Trigger.ChannelCreated != nil

	if oldHasChannelCreated && !newHasChannelCreated {
		if err := s.removeFromChannelCreatedIndex(f.ID); err != nil {
			return err
		}
	} else if !oldHasChannelCreated && newHasChannelCreated {
		if err := s.addToChannelCreatedIndex(f.ID); err != nil {
			return err
		}
	}

	// Update user-joined-team index.
	oldHasUserJoinedTeam := old != nil && old.Trigger.UserJoinedTeam != nil
	newHasUserJoinedTeam := f.Trigger.UserJoinedTeam != nil

	if oldHasUserJoinedTeam && !newHasUserJoinedTeam {
		if err := s.removeFromUserJoinedTeamIndex(f.ID); err != nil {
			return err
		}
	} else if !oldHasUserJoinedTeam && newHasUserJoinedTeam {
		if err := s.addToUserJoinedTeamIndex(f.ID); err != nil {
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

	if f.Trigger.MessagePosted != nil && f.Trigger.MessagePosted.ChannelID != "" {
		if err := s.removeTriggerIndex(f.Trigger.MessagePosted.ChannelID, id); err != nil {
			return err
		}
	}

	if f.Trigger.MembershipChanged != nil && f.Trigger.MembershipChanged.ChannelID != "" {
		if err := s.removeMembershipTriggerIndex(f.Trigger.MembershipChanged.ChannelID, id); err != nil {
			return err
		}
	}

	if chID := f.TriggerChannelID(); chID != "" {
		if err := s.removeChannelTriggerIndex(chID, id); err != nil {
			return err
		}
	}

	if f.Trigger.Schedule != nil {
		if err := s.removeFromScheduleIndex(id); err != nil {
			return err
		}
	}

	if f.Trigger.ChannelCreated != nil {
		if err := s.removeFromChannelCreatedIndex(id); err != nil {
			return err
		}
	}

	if f.Trigger.UserJoinedTeam != nil {
		if err := s.removeFromUserJoinedTeamIndex(id); err != nil {
			return err
		}
	}

	return nil
}

// CountByTriggerChannel returns the number of flows targeting the given channel
// across all trigger types (message_posted, schedule, membership_changed).
func (s *KVStore) CountByTriggerChannel(channelID string) (int, error) {
	ids, err := s.getChannelTriggerIndex(channelID)
	if err != nil {
		return 0, err
	}
	return len(ids), nil
}

// GetFlowIDsForChannel returns flow IDs triggered by messages in the given channel.
func (s *KVStore) GetFlowIDsForChannel(channelID string) ([]string, error) {
	return s.getTriggerIndex(channelID)
}

// GetFlowIDsForMembershipChannel returns flow IDs triggered by membership changes in the given channel.
func (s *KVStore) GetFlowIDsForMembershipChannel(channelID string) ([]string, error) {
	return s.getMembershipTriggerIndex(channelID)
}

// GetChannelCreatedFlowIDs returns flow IDs triggered by channel creation events.
func (s *KVStore) GetChannelCreatedFlowIDs() ([]string, error) {
	return s.getChannelCreatedIndex()
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
	s.indexMu.Lock()
	defer s.indexMu.Unlock()

	ids, err := s.getIndex()
	if err != nil {
		return err
	}
	if slices.Contains(ids, id) {
		return nil
	}
	ids = append(ids, id)
	return s.setIndex(ids)
}

func (s *KVStore) removeFromIndex(id string) error {
	s.indexMu.Lock()
	defer s.indexMu.Unlock()

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
	s.indexMu.Lock()
	defer s.indexMu.Unlock()

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
	s.indexMu.Lock()
	defer s.indexMu.Unlock()

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

// --- Membership trigger index helpers ---

func makeMembershipTriggerIndexKey(channelID string) string {
	return membershipTriggerIndexPrefix + channelID
}

func (s *KVStore) getMembershipTriggerIndex(channelID string) ([]string, error) {
	key := makeMembershipTriggerIndexKey(channelID)
	data, appErr := s.api.KVGet(key)
	if appErr != nil {
		return nil, fmt.Errorf("failed to get membership trigger index: %w", appErr)
	}
	if data == nil {
		return nil, nil
	}

	var ids []string
	if err := json.Unmarshal(data, &ids); err != nil {
		return nil, fmt.Errorf("failed to unmarshal membership trigger index: %w", err)
	}
	return ids, nil
}

func (s *KVStore) setMembershipTriggerIndex(channelID string, ids []string) error {
	key := makeMembershipTriggerIndexKey(channelID)
	if len(ids) == 0 {
		if appErr := s.api.KVDelete(key); appErr != nil {
			return fmt.Errorf("failed to delete membership trigger index: %w", appErr)
		}
		return nil
	}
	data, err := json.Marshal(ids)
	if err != nil {
		return fmt.Errorf("failed to marshal membership trigger index: %w", err)
	}
	if appErr := s.api.KVSet(key, data); appErr != nil {
		return fmt.Errorf("failed to save membership trigger index: %w", appErr)
	}
	return nil
}

func (s *KVStore) addMembershipTriggerIndex(channelID, flowID string) error {
	s.indexMu.Lock()
	defer s.indexMu.Unlock()

	ids, err := s.getMembershipTriggerIndex(channelID)
	if err != nil {
		return err
	}
	if slices.Contains(ids, flowID) {
		return nil
	}
	ids = append(ids, flowID)
	return s.setMembershipTriggerIndex(channelID, ids)
}

func (s *KVStore) removeMembershipTriggerIndex(channelID, flowID string) error {
	s.indexMu.Lock()
	defer s.indexMu.Unlock()

	ids, err := s.getMembershipTriggerIndex(channelID)
	if err != nil {
		return err
	}
	filtered := make([]string, 0, len(ids))
	for _, id := range ids {
		if id != flowID {
			filtered = append(filtered, id)
		}
	}
	return s.setMembershipTriggerIndex(channelID, filtered)
}

// --- Channel-trigger index helpers (all trigger types) ---

func makeChannelTriggerIndexKey(channelID string) string {
	return channelTriggerIndexPrefix + channelID
}

func (s *KVStore) getChannelTriggerIndex(channelID string) ([]string, error) {
	key := makeChannelTriggerIndexKey(channelID)
	data, appErr := s.api.KVGet(key)
	if appErr != nil {
		return nil, fmt.Errorf("failed to get channel trigger index: %w", appErr)
	}
	if data == nil {
		return nil, nil
	}

	var ids []string
	if err := json.Unmarshal(data, &ids); err != nil {
		return nil, fmt.Errorf("failed to unmarshal channel trigger index: %w", err)
	}
	return ids, nil
}

func (s *KVStore) setChannelTriggerIndex(channelID string, ids []string) error {
	key := makeChannelTriggerIndexKey(channelID)
	if len(ids) == 0 {
		if appErr := s.api.KVDelete(key); appErr != nil {
			return fmt.Errorf("failed to delete channel trigger index: %w", appErr)
		}
		return nil
	}
	data, err := json.Marshal(ids)
	if err != nil {
		return fmt.Errorf("failed to marshal channel trigger index: %w", err)
	}
	if appErr := s.api.KVSet(key, data); appErr != nil {
		return fmt.Errorf("failed to save channel trigger index: %w", appErr)
	}
	return nil
}

func (s *KVStore) addChannelTriggerIndex(channelID, flowID string) error {
	s.indexMu.Lock()
	defer s.indexMu.Unlock()

	ids, err := s.getChannelTriggerIndex(channelID)
	if err != nil {
		return err
	}
	if slices.Contains(ids, flowID) {
		return nil
	}
	ids = append(ids, flowID)
	return s.setChannelTriggerIndex(channelID, ids)
}

func (s *KVStore) removeChannelTriggerIndex(channelID, flowID string) error {
	s.indexMu.Lock()
	defer s.indexMu.Unlock()

	ids, err := s.getChannelTriggerIndex(channelID)
	if err != nil {
		return err
	}
	filtered := make([]string, 0, len(ids))
	for _, id := range ids {
		if id != flowID {
			filtered = append(filtered, id)
		}
	}
	return s.setChannelTriggerIndex(channelID, filtered)
}

// --- Schedule index helpers ---

func (s *KVStore) getScheduleIndex() ([]string, error) {
	data, appErr := s.api.KVGet(scheduleIndexKey)
	if appErr != nil {
		return nil, fmt.Errorf("failed to get schedule index: %w", appErr)
	}
	if data == nil {
		return nil, nil
	}

	var ids []string
	if err := json.Unmarshal(data, &ids); err != nil {
		return nil, fmt.Errorf("failed to unmarshal schedule index: %w", err)
	}
	return ids, nil
}

func (s *KVStore) setScheduleIndex(ids []string) error {
	if len(ids) == 0 {
		if appErr := s.api.KVDelete(scheduleIndexKey); appErr != nil {
			return fmt.Errorf("failed to delete schedule index: %w", appErr)
		}
		return nil
	}
	data, err := json.Marshal(ids)
	if err != nil {
		return fmt.Errorf("failed to marshal schedule index: %w", err)
	}
	if appErr := s.api.KVSet(scheduleIndexKey, data); appErr != nil {
		return fmt.Errorf("failed to save schedule index: %w", appErr)
	}
	return nil
}

func (s *KVStore) addToScheduleIndex(id string) error {
	s.indexMu.Lock()
	defer s.indexMu.Unlock()

	ids, err := s.getScheduleIndex()
	if err != nil {
		return err
	}
	if slices.Contains(ids, id) {
		return nil
	}
	ids = append(ids, id)
	return s.setScheduleIndex(ids)
}

func (s *KVStore) removeFromScheduleIndex(id string) error {
	s.indexMu.Lock()
	defer s.indexMu.Unlock()

	ids, err := s.getScheduleIndex()
	if err != nil {
		return err
	}
	filtered := make([]string, 0, len(ids))
	for _, existingID := range ids {
		if existingID != id {
			filtered = append(filtered, existingID)
		}
	}
	return s.setScheduleIndex(filtered)
}

// --- Channel-created index helpers ---

func (s *KVStore) getChannelCreatedIndex() ([]string, error) {
	data, appErr := s.api.KVGet(channelCreatedIndexKey)
	if appErr != nil {
		return nil, fmt.Errorf("failed to get channel created index: %w", appErr)
	}
	if data == nil {
		return nil, nil
	}

	var ids []string
	if err := json.Unmarshal(data, &ids); err != nil {
		return nil, fmt.Errorf("failed to unmarshal channel created index: %w", err)
	}
	return ids, nil
}

func (s *KVStore) setChannelCreatedIndex(ids []string) error {
	if len(ids) == 0 {
		if appErr := s.api.KVDelete(channelCreatedIndexKey); appErr != nil {
			return fmt.Errorf("failed to delete channel created index: %w", appErr)
		}
		return nil
	}
	data, err := json.Marshal(ids)
	if err != nil {
		return fmt.Errorf("failed to marshal channel created index: %w", err)
	}
	if appErr := s.api.KVSet(channelCreatedIndexKey, data); appErr != nil {
		return fmt.Errorf("failed to save channel created index: %w", appErr)
	}
	return nil
}

func (s *KVStore) addToChannelCreatedIndex(id string) error {
	s.indexMu.Lock()
	defer s.indexMu.Unlock()

	ids, err := s.getChannelCreatedIndex()
	if err != nil {
		return err
	}
	if slices.Contains(ids, id) {
		return nil
	}
	ids = append(ids, id)
	return s.setChannelCreatedIndex(ids)
}

func (s *KVStore) removeFromChannelCreatedIndex(id string) error {
	s.indexMu.Lock()
	defer s.indexMu.Unlock()

	ids, err := s.getChannelCreatedIndex()
	if err != nil {
		return err
	}
	filtered := make([]string, 0, len(ids))
	for _, existingID := range ids {
		if existingID != id {
			filtered = append(filtered, existingID)
		}
	}
	return s.setChannelCreatedIndex(filtered)
}

// --- User-joined-team index helpers ---

// GetUserJoinedTeamFlowIDs returns flow IDs triggered by user-joined-team events.
func (s *KVStore) GetUserJoinedTeamFlowIDs() ([]string, error) {
	return s.getUserJoinedTeamIndex()
}

func (s *KVStore) getUserJoinedTeamIndex() ([]string, error) {
	data, appErr := s.api.KVGet(userJoinedTeamIndexKey)
	if appErr != nil {
		return nil, fmt.Errorf("failed to get user joined team index: %w", appErr)
	}
	if data == nil {
		return nil, nil
	}

	var ids []string
	if err := json.Unmarshal(data, &ids); err != nil {
		return nil, fmt.Errorf("failed to unmarshal user joined team index: %w", err)
	}
	return ids, nil
}

func (s *KVStore) setUserJoinedTeamIndex(ids []string) error {
	if len(ids) == 0 {
		if appErr := s.api.KVDelete(userJoinedTeamIndexKey); appErr != nil {
			return fmt.Errorf("failed to delete user joined team index: %w", appErr)
		}
		return nil
	}
	data, err := json.Marshal(ids)
	if err != nil {
		return fmt.Errorf("failed to marshal user joined team index: %w", err)
	}
	if appErr := s.api.KVSet(userJoinedTeamIndexKey, data); appErr != nil {
		return fmt.Errorf("failed to save user joined team index: %w", appErr)
	}
	return nil
}

func (s *KVStore) addToUserJoinedTeamIndex(id string) error {
	s.indexMu.Lock()
	defer s.indexMu.Unlock()

	ids, err := s.getUserJoinedTeamIndex()
	if err != nil {
		return err
	}
	if slices.Contains(ids, id) {
		return nil
	}
	ids = append(ids, id)
	return s.setUserJoinedTeamIndex(ids)
}

func (s *KVStore) removeFromUserJoinedTeamIndex(id string) error {
	s.indexMu.Lock()
	defer s.indexMu.Unlock()

	ids, err := s.getUserJoinedTeamIndex()
	if err != nil {
		return err
	}
	filtered := make([]string, 0, len(ids))
	for _, existingID := range ids {
		if existingID != id {
			filtered = append(filtered, existingID)
		}
	}
	return s.setUserJoinedTeamIndex(filtered)
}
