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
	flowKeyPrefix                    = "flow:"
	flowIndexKey                     = "flow_index"
	triggerIndexPrefix               = "ti:mp:" // shortened to stay within 50-char KV key limit (6 + 26 = 32)
	channelTriggerIndexPrefix        = "ct:"    // all trigger types by channel (3 + 26 = 29)
	membershipTriggerIndexPrefix     = "ti:mc:" // membership_changed by channel (6 + 26 = 32)
	scheduleIndexKey                 = "sched_index"
	channelCreatedIndexKey           = "cc_index"
	userJoinedTeamTriggerIndexPrefix = "ti:ujt:" // user_joined_team by team (7 + 26 = 33)
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
	s.indexMu.Lock()
	defer s.indexMu.Unlock()
	return s.saveLocked(f)
}

// SaveWithChannelLimit persists the flow atomically with respect to a
// per-channel quota check. limit <= 0 disables the check. excludeID,
// when non-empty, names the flow being updated so it does not count
// against itself when targeting the same channel.
//
// Returns model.ErrChannelFlowLimitExceeded when persisting would push the
// count above limit; the flow is not saved in that case. All other
// returned errors indicate backend failures.
//
// When the flow has no trigger channel (e.g. channel_created or
// user_joined_team triggers), the quota does not apply and the call
// degrades to a plain Save.
func (s *KVStore) SaveWithChannelLimit(f *model.Flow, limit int, excludeID string) error {
	channelID := f.TriggerChannelID()
	if limit <= 0 || channelID == "" {
		return s.Save(f)
	}

	s.indexMu.Lock()
	defer s.indexMu.Unlock()

	ids, err := s.getChannelTriggerIndex(channelID)
	if err != nil {
		return fmt.Errorf("failed to read channel trigger index: %w", err)
	}
	count := len(ids)

	// Self-exclusion: if the flow being updated already targets this
	// channel, it is already counted in `ids`, so subtract 1.
	if excludeID != "" {
		if slices.Contains(ids, excludeID) {
			count--
		}
	}

	if count >= limit {
		return model.ErrChannelFlowLimitExceeded
	}

	return s.saveLocked(f)
}

// saveLocked persists the flow record and reconciles every secondary index
// (global, message-posted, membership-changed, channel-trigger, schedule,
// channel-created, user-joined-team) against the prior version under a single
// indexMu critical section. The caller MUST hold indexMu; saveLocked invokes
// the *Locked helpers exclusively because indexMu is a sync.Locker (in
// production a non-reentrant cluster.Mutex), so re-acquiring it would
// deadlock. Shared by Save and SaveWithChannelLimit so both paths see a
// consistent view of the index they just read.
func (s *KVStore) saveLocked(f *model.Flow) error {
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
		if err := s.addToIndexLocked(f.ID); err != nil {
			return err
		}
	}

	// Update trigger index: remove old entry, add new entry.
	if old != nil && old.Trigger.MessagePosted != nil && old.Trigger.MessagePosted.ChannelID != "" {
		if err := s.removeTriggerIndexLocked(old.Trigger.MessagePosted.ChannelID, f.ID); err != nil {
			return err
		}
	}
	if f.Trigger.MessagePosted != nil && f.Trigger.MessagePosted.ChannelID != "" {
		if err := s.addTriggerIndexLocked(f.Trigger.MessagePosted.ChannelID, f.ID); err != nil {
			return err
		}
	}

	// Update membership trigger index: remove old entry, add new entry.
	if old != nil && old.Trigger.MembershipChanged != nil && old.Trigger.MembershipChanged.ChannelID != "" {
		if err := s.removeMembershipTriggerIndexLocked(old.Trigger.MembershipChanged.ChannelID, f.ID); err != nil {
			return err
		}
	}
	if f.Trigger.MembershipChanged != nil && f.Trigger.MembershipChanged.ChannelID != "" {
		if err := s.addMembershipTriggerIndexLocked(f.Trigger.MembershipChanged.ChannelID, f.ID); err != nil {
			return err
		}
	}

	// Update channel-trigger index (covers all trigger types).
	if old != nil {
		if oldChID := old.TriggerChannelID(); oldChID != "" {
			if err := s.removeChannelTriggerIndexLocked(oldChID, f.ID); err != nil {
				return err
			}
		}
	}
	if newChID := f.TriggerChannelID(); newChID != "" {
		if err := s.addChannelTriggerIndexLocked(newChID, f.ID); err != nil {
			return err
		}
	}

	// Update schedule index.
	oldHasSchedule := old != nil && old.Trigger.Schedule != nil
	newHasSchedule := f.Trigger.Schedule != nil

	if oldHasSchedule && !newHasSchedule {
		if err := s.removeFromScheduleIndexLocked(f.ID); err != nil {
			return err
		}
	} else if !oldHasSchedule && newHasSchedule {
		if err := s.addToScheduleIndexLocked(f.ID); err != nil {
			return err
		}
	}

	// Update channel-created index.
	oldHasChannelCreated := old != nil && old.Trigger.ChannelCreated != nil
	newHasChannelCreated := f.Trigger.ChannelCreated != nil

	if oldHasChannelCreated && !newHasChannelCreated {
		if err := s.removeFromChannelCreatedIndexLocked(f.ID); err != nil {
			return err
		}
	} else if !oldHasChannelCreated && newHasChannelCreated {
		if err := s.addToChannelCreatedIndexLocked(f.ID); err != nil {
			return err
		}
	}

	// Update user-joined-team trigger index.
	if old != nil && old.Trigger.UserJoinedTeam != nil && old.Trigger.UserJoinedTeam.TeamID != "" {
		if err := s.removeUserJoinedTeamTriggerIndexLocked(old.Trigger.UserJoinedTeam.TeamID, f.ID); err != nil {
			return err
		}
	}
	if f.Trigger.UserJoinedTeam != nil && f.Trigger.UserJoinedTeam.TeamID != "" {
		if err := s.addUserJoinedTeamTriggerIndexLocked(f.Trigger.UserJoinedTeam.TeamID, f.ID); err != nil {
			return err
		}
	}

	return nil
}

func (s *KVStore) Delete(id string) error {
	s.indexMu.Lock()
	defer s.indexMu.Unlock()
	return s.deleteLocked(id)
}

// deleteLocked removes the flow record and reconciles every secondary
// index under a single indexMu critical section. The caller MUST hold
// indexMu; deleteLocked invokes the *Locked helpers exclusively because
// indexMu is non-reentrant. Holding the lock across the KVDelete and
// the index cleanup prevents a concurrent SaveWithChannelLimit from
// observing a flow ID that lingers in the channel-trigger index after
// the record itself has been removed.
func (s *KVStore) deleteLocked(id string) error {
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

	if err := s.removeFromIndexLocked(id); err != nil {
		return err
	}

	if f.Trigger.MessagePosted != nil && f.Trigger.MessagePosted.ChannelID != "" {
		if err := s.removeTriggerIndexLocked(f.Trigger.MessagePosted.ChannelID, id); err != nil {
			return err
		}
	}

	if f.Trigger.MembershipChanged != nil && f.Trigger.MembershipChanged.ChannelID != "" {
		if err := s.removeMembershipTriggerIndexLocked(f.Trigger.MembershipChanged.ChannelID, id); err != nil {
			return err
		}
	}

	if chID := f.TriggerChannelID(); chID != "" {
		if err := s.removeChannelTriggerIndexLocked(chID, id); err != nil {
			return err
		}
	}

	if f.Trigger.Schedule != nil {
		if err := s.removeFromScheduleIndexLocked(id); err != nil {
			return err
		}
	}

	if f.Trigger.ChannelCreated != nil {
		if err := s.removeFromChannelCreatedIndexLocked(id); err != nil {
			return err
		}
	}

	if f.Trigger.UserJoinedTeam != nil && f.Trigger.UserJoinedTeam.TeamID != "" {
		if err := s.removeUserJoinedTeamTriggerIndexLocked(f.Trigger.UserJoinedTeam.TeamID, id); err != nil {
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

func (s *KVStore) addToIndexLocked(id string) error {
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

func (s *KVStore) removeFromIndexLocked(id string) error {
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

func (s *KVStore) addTriggerIndexLocked(channelID, flowID string) error {
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

func (s *KVStore) removeTriggerIndexLocked(channelID, flowID string) error {
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

func (s *KVStore) addMembershipTriggerIndexLocked(channelID, flowID string) error {
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

func (s *KVStore) removeMembershipTriggerIndexLocked(channelID, flowID string) error {
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

func (s *KVStore) addChannelTriggerIndexLocked(channelID, flowID string) error {
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

func (s *KVStore) removeChannelTriggerIndexLocked(channelID, flowID string) error {
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

func (s *KVStore) addToScheduleIndexLocked(id string) error {
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

func (s *KVStore) removeFromScheduleIndexLocked(id string) error {
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

func (s *KVStore) addToChannelCreatedIndexLocked(id string) error {
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

func (s *KVStore) removeFromChannelCreatedIndexLocked(id string) error {
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

// --- User-joined-team trigger index helpers ---

func makeUserJoinedTeamTriggerIndexKey(teamID string) string {
	return userJoinedTeamTriggerIndexPrefix + teamID
}

// GetFlowIDsForUserJoinedTeam returns flow IDs triggered by user-joined-team events for the given team.
func (s *KVStore) GetFlowIDsForUserJoinedTeam(teamID string) ([]string, error) {
	return s.getUserJoinedTeamTriggerIndex(teamID)
}

func (s *KVStore) getUserJoinedTeamTriggerIndex(teamID string) ([]string, error) {
	key := makeUserJoinedTeamTriggerIndexKey(teamID)
	data, appErr := s.api.KVGet(key)
	if appErr != nil {
		return nil, fmt.Errorf("failed to get user joined team trigger index: %w", appErr)
	}
	if data == nil {
		return nil, nil
	}

	var ids []string
	if err := json.Unmarshal(data, &ids); err != nil {
		return nil, fmt.Errorf("failed to unmarshal user joined team trigger index: %w", err)
	}
	return ids, nil
}

func (s *KVStore) setUserJoinedTeamTriggerIndex(teamID string, ids []string) error {
	key := makeUserJoinedTeamTriggerIndexKey(teamID)
	if len(ids) == 0 {
		if appErr := s.api.KVDelete(key); appErr != nil {
			return fmt.Errorf("failed to delete user joined team trigger index: %w", appErr)
		}
		return nil
	}
	data, err := json.Marshal(ids)
	if err != nil {
		return fmt.Errorf("failed to marshal user joined team trigger index: %w", err)
	}
	if appErr := s.api.KVSet(key, data); appErr != nil {
		return fmt.Errorf("failed to save user joined team trigger index: %w", appErr)
	}
	return nil
}

func (s *KVStore) addUserJoinedTeamTriggerIndexLocked(teamID, flowID string) error {
	ids, err := s.getUserJoinedTeamTriggerIndex(teamID)
	if err != nil {
		return err
	}
	if slices.Contains(ids, flowID) {
		return nil
	}
	ids = append(ids, flowID)
	return s.setUserJoinedTeamTriggerIndex(teamID, ids)
}

func (s *KVStore) removeUserJoinedTeamTriggerIndexLocked(teamID, flowID string) error {
	ids, err := s.getUserJoinedTeamTriggerIndex(teamID)
	if err != nil {
		return err
	}
	filtered := make([]string, 0, len(ids))
	for _, id := range ids {
		if id != flowID {
			filtered = append(filtered, id)
		}
	}
	return s.setUserJoinedTeamTriggerIndex(teamID, filtered)
}
