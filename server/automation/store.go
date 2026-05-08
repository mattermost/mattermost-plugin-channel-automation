package automation

import (
	"encoding/json"
	"fmt"
	"slices"
	"sync"

	"github.com/mattermost/mattermost/server/public/plugin"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

const (
	automationKeyPrefix              = "automation:"
	automationIndexKey               = "automation_index"
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

// NewStore creates a new KV-backed automation store. The caller must supply
// a cluster-safe mutex (e.g. cluster.Mutex) to protect index
// read-modify-write cycles across plugin instances.
func NewStore(api plugin.API, indexMu sync.Locker) model.Store {
	return &KVStore{api: api, indexMu: indexMu}
}

func (s *KVStore) Get(id string) (*model.Automation, error) {
	data, appErr := s.api.KVGet(automationKeyPrefix + id)
	if appErr != nil {
		return nil, fmt.Errorf("failed to get automation %s: %w", id, appErr)
	}
	if data == nil {
		return nil, nil
	}

	var a model.Automation
	if err := json.Unmarshal(data, &a); err != nil {
		return nil, fmt.Errorf("failed to unmarshal automation %s: %w", id, err)
	}
	return &a, nil
}

func (s *KVStore) List() ([]*model.Automation, error) {
	ids, err := s.getIndex()
	if err != nil {
		return nil, err
	}

	automations := make([]*model.Automation, 0, len(ids))
	for _, id := range ids {
		a, err := s.Get(id)
		if err != nil {
			return nil, err
		}
		if a != nil {
			automations = append(automations, a)
		}
	}
	return automations, nil
}

func (s *KVStore) ListByTriggerChannel(channelID string) ([]*model.Automation, error) {
	ids, err := s.getChannelTriggerIndex(channelID)
	if err != nil {
		return nil, err
	}

	automations := make([]*model.Automation, 0, len(ids))
	for _, id := range ids {
		a, err := s.Get(id)
		if err != nil {
			return nil, err
		}
		if a != nil {
			automations = append(automations, a)
		}
	}
	return automations, nil
}

func (s *KVStore) ListScheduled() ([]*model.Automation, error) {
	ids, err := s.getScheduleIndex()
	if err != nil {
		return nil, err
	}

	automations := make([]*model.Automation, 0, len(ids))
	for _, id := range ids {
		a, err := s.Get(id)
		if err != nil {
			return nil, err
		}
		if a != nil {
			automations = append(automations, a)
		}
	}
	return automations, nil
}

func (s *KVStore) Save(a *model.Automation) error {
	s.indexMu.Lock()
	defer s.indexMu.Unlock()
	return s.saveLocked(a)
}

// SaveWithChannelLimit persists the automation atomically with respect to
// a per-channel quota check. limit <= 0 disables the check. excludeID,
// when non-empty, names the automation being updated so it does not count
// against itself when targeting the same channel.
//
// Returns model.ErrChannelAutomationLimitExceeded when persisting would
// push the count above limit; the automation is not saved in that case.
// All other returned errors indicate backend failures.
//
// When the automation has no trigger channel (e.g. channel_created or
// user_joined_team triggers), the quota does not apply and the call
// degrades to a plain Save.
//
// indexMu is acquired for the entire body so the count read and the
// persist share one critical section. The method MUST NOT be called
// while indexMu is already held; it always takes the lock itself.
func (s *KVStore) SaveWithChannelLimit(a *model.Automation, limit int, excludeID string) error {
	s.indexMu.Lock()
	defer s.indexMu.Unlock()

	channelID := a.TriggerChannelID()
	if limit <= 0 || channelID == "" {
		return s.saveLocked(a)
	}

	ids, err := s.getChannelTriggerIndex(channelID)
	if err != nil {
		return fmt.Errorf("failed to read channel trigger index: %w", err)
	}
	count := len(ids)

	// Self-exclusion: if the automation being updated already targets this
	// channel, it is already counted in `ids`, so subtract 1.
	if excludeID != "" && slices.Contains(ids, excludeID) {
		count--
	}

	if count >= limit {
		return model.ErrChannelAutomationLimitExceeded
	}

	// Pass the slice we just read to saveLocked so it can skip the
	// redundant getChannelTriggerIndex(channelID) inside the same
	// critical section.
	return s.saveLockedWithChannelTriggerHint(a, channelID, ids)
}

// saveLocked persists the automation record and reconciles every secondary
// index (global, message-posted, membership-changed, channel-trigger,
// schedule, channel-created, user-joined-team) against the prior version
// under a single indexMu critical section. The caller MUST hold indexMu;
// saveLocked invokes the *Locked helpers exclusively because indexMu is a
// sync.Locker (in production a non-reentrant cluster.Mutex), so
// re-acquiring it would deadlock. Shared by Save and SaveWithChannelLimit
// so both paths see a consistent view of the index they just read.
func (s *KVStore) saveLocked(a *model.Automation) error {
	return s.saveLockedWithChannelTriggerHint(a, "", nil)
}

// saveLockedWithChannelTriggerHint is the shared core for Save and
// SaveWithChannelLimit. When hintChannelID matches the automation's new
// trigger channel, hintIDs is used as the pre-read channel-trigger-index
// so the implementation can skip a redundant
// getChannelTriggerIndex(channelID) call already performed by
// SaveWithChannelLimit's quota check.
func (s *KVStore) saveLockedWithChannelTriggerHint(a *model.Automation, hintChannelID string, hintIDs []string) error {
	// Read old automation to clean up stale trigger index entries.
	old, err := s.Get(a.ID)
	if err != nil {
		return err
	}

	data, err := json.Marshal(a)
	if err != nil {
		return fmt.Errorf("failed to marshal automation %s: %w", a.ID, err)
	}

	if appErr := s.api.KVSet(automationKeyPrefix+a.ID, data); appErr != nil {
		return fmt.Errorf("failed to save automation %s: %w", a.ID, appErr)
	}

	// Update the automation index (add if new).
	if old == nil {
		if err := s.addToIndexLocked(a.ID); err != nil {
			return err
		}
	}

	// Update trigger index: remove old entry, add new entry.
	if old != nil && old.Trigger.MessagePosted != nil && old.Trigger.MessagePosted.ChannelID != "" {
		if err := s.removeTriggerIndexLocked(old.Trigger.MessagePosted.ChannelID, a.ID); err != nil {
			return err
		}
	}
	if a.Trigger.MessagePosted != nil && a.Trigger.MessagePosted.ChannelID != "" {
		if err := s.addTriggerIndexLocked(a.Trigger.MessagePosted.ChannelID, a.ID); err != nil {
			return err
		}
	}

	// Update membership trigger index: remove old entry, add new entry.
	if old != nil && old.Trigger.MembershipChanged != nil && old.Trigger.MembershipChanged.ChannelID != "" {
		if err := s.removeMembershipTriggerIndexLocked(old.Trigger.MembershipChanged.ChannelID, a.ID); err != nil {
			return err
		}
	}
	if a.Trigger.MembershipChanged != nil && a.Trigger.MembershipChanged.ChannelID != "" {
		if err := s.addMembershipTriggerIndexLocked(a.Trigger.MembershipChanged.ChannelID, a.ID); err != nil {
			return err
		}
	}

	// Update channel-trigger index (covers all trigger types).
	if old != nil {
		if oldChID := old.TriggerChannelID(); oldChID != "" {
			if err := s.removeChannelTriggerIndexLocked(oldChID, a.ID); err != nil {
				return err
			}
		}
	}
	if newChID := a.TriggerChannelID(); newChID != "" {
		if newChID == hintChannelID {
			if err := s.appendChannelTriggerIndexLocked(newChID, a.ID, hintIDs); err != nil {
				return err
			}
		} else if err := s.addChannelTriggerIndexLocked(newChID, a.ID); err != nil {
			return err
		}
	}

	// Update schedule index.
	oldHasSchedule := old != nil && old.Trigger.Schedule != nil
	newHasSchedule := a.Trigger.Schedule != nil

	if oldHasSchedule && !newHasSchedule {
		if err := s.removeFromScheduleIndexLocked(a.ID); err != nil {
			return err
		}
	} else if !oldHasSchedule && newHasSchedule {
		if err := s.addToScheduleIndexLocked(a.ID); err != nil {
			return err
		}
	}

	// Update channel-created index.
	oldHasChannelCreated := old != nil && old.Trigger.ChannelCreated != nil
	newHasChannelCreated := a.Trigger.ChannelCreated != nil

	if oldHasChannelCreated && !newHasChannelCreated {
		if err := s.removeFromChannelCreatedIndexLocked(a.ID); err != nil {
			return err
		}
	} else if !oldHasChannelCreated && newHasChannelCreated {
		if err := s.addToChannelCreatedIndexLocked(a.ID); err != nil {
			return err
		}
	}

	// Update user-joined-team trigger index.
	if old != nil && old.Trigger.UserJoinedTeam != nil && old.Trigger.UserJoinedTeam.TeamID != "" {
		if err := s.removeUserJoinedTeamTriggerIndexLocked(old.Trigger.UserJoinedTeam.TeamID, a.ID); err != nil {
			return err
		}
	}
	if a.Trigger.UserJoinedTeam != nil && a.Trigger.UserJoinedTeam.TeamID != "" {
		if err := s.addUserJoinedTeamTriggerIndexLocked(a.Trigger.UserJoinedTeam.TeamID, a.ID); err != nil {
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

// deleteLocked removes the automation record and reconciles every
// secondary index under a single indexMu critical section. The caller
// MUST hold indexMu; deleteLocked invokes the *Locked helpers exclusively
// because indexMu is non-reentrant. Holding the lock across the KVDelete
// and the index cleanup prevents a concurrent SaveWithChannelLimit from
// observing an automation ID that lingers in the channel-trigger index
// after the record itself has been removed.
func (s *KVStore) deleteLocked(id string) error {
	a, err := s.Get(id)
	if err != nil {
		return err
	}
	if a == nil {
		return nil
	}

	if appErr := s.api.KVDelete(automationKeyPrefix + id); appErr != nil {
		return fmt.Errorf("failed to delete automation %s: %w", id, appErr)
	}

	if err := s.removeFromIndexLocked(id); err != nil {
		return err
	}

	if a.Trigger.MessagePosted != nil && a.Trigger.MessagePosted.ChannelID != "" {
		if err := s.removeTriggerIndexLocked(a.Trigger.MessagePosted.ChannelID, id); err != nil {
			return err
		}
	}

	if a.Trigger.MembershipChanged != nil && a.Trigger.MembershipChanged.ChannelID != "" {
		if err := s.removeMembershipTriggerIndexLocked(a.Trigger.MembershipChanged.ChannelID, id); err != nil {
			return err
		}
	}

	if chID := a.TriggerChannelID(); chID != "" {
		if err := s.removeChannelTriggerIndexLocked(chID, id); err != nil {
			return err
		}
	}

	if a.Trigger.Schedule != nil {
		if err := s.removeFromScheduleIndexLocked(id); err != nil {
			return err
		}
	}

	if a.Trigger.ChannelCreated != nil {
		if err := s.removeFromChannelCreatedIndexLocked(id); err != nil {
			return err
		}
	}

	if a.Trigger.UserJoinedTeam != nil && a.Trigger.UserJoinedTeam.TeamID != "" {
		if err := s.removeUserJoinedTeamTriggerIndexLocked(a.Trigger.UserJoinedTeam.TeamID, id); err != nil {
			return err
		}
	}

	return nil
}

// CountByTriggerChannel returns the number of automations targeting the given channel
// across all trigger types (message_posted, schedule, membership_changed).
func (s *KVStore) CountByTriggerChannel(channelID string) (int, error) {
	ids, err := s.getChannelTriggerIndex(channelID)
	if err != nil {
		return 0, err
	}
	return len(ids), nil
}

// GetAutomationIDsForChannel returns automation IDs triggered by messages in the given channel.
func (s *KVStore) GetAutomationIDsForChannel(channelID string) ([]string, error) {
	return s.getTriggerIndex(channelID)
}

// GetAutomationIDsForMembershipChannel returns automation IDs triggered by membership changes in the given channel.
func (s *KVStore) GetAutomationIDsForMembershipChannel(channelID string) ([]string, error) {
	return s.getMembershipTriggerIndex(channelID)
}

// GetChannelCreatedAutomationIDs returns automation IDs triggered by channel creation events.
func (s *KVStore) GetChannelCreatedAutomationIDs() ([]string, error) {
	return s.getChannelCreatedIndex()
}

// --- Index helpers ---

func (s *KVStore) getIndex() ([]string, error) {
	data, appErr := s.api.KVGet(automationIndexKey)
	if appErr != nil {
		return nil, fmt.Errorf("failed to get automation index: %w", appErr)
	}
	if data == nil {
		return nil, nil
	}

	var ids []string
	if err := json.Unmarshal(data, &ids); err != nil {
		return nil, fmt.Errorf("failed to unmarshal automation index: %w", err)
	}
	return ids, nil
}

func (s *KVStore) setIndex(ids []string) error {
	data, err := json.Marshal(ids)
	if err != nil {
		return fmt.Errorf("failed to marshal automation index: %w", err)
	}
	if appErr := s.api.KVSet(automationIndexKey, data); appErr != nil {
		return fmt.Errorf("failed to save automation index: %w", appErr)
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

func (s *KVStore) addTriggerIndexLocked(channelID, automationID string) error {
	ids, err := s.getTriggerIndex(channelID)
	if err != nil {
		return err
	}
	if slices.Contains(ids, automationID) {
		return nil
	}
	ids = append(ids, automationID)
	return s.setTriggerIndex(channelID, ids)
}

func (s *KVStore) removeTriggerIndexLocked(channelID, automationID string) error {
	ids, err := s.getTriggerIndex(channelID)
	if err != nil {
		return err
	}
	filtered := make([]string, 0, len(ids))
	for _, id := range ids {
		if id != automationID {
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

func (s *KVStore) addMembershipTriggerIndexLocked(channelID, automationID string) error {
	ids, err := s.getMembershipTriggerIndex(channelID)
	if err != nil {
		return err
	}
	if slices.Contains(ids, automationID) {
		return nil
	}
	ids = append(ids, automationID)
	return s.setMembershipTriggerIndex(channelID, ids)
}

func (s *KVStore) removeMembershipTriggerIndexLocked(channelID, automationID string) error {
	ids, err := s.getMembershipTriggerIndex(channelID)
	if err != nil {
		return err
	}
	filtered := make([]string, 0, len(ids))
	for _, id := range ids {
		if id != automationID {
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

func (s *KVStore) addChannelTriggerIndexLocked(channelID, automationID string) error {
	ids, err := s.getChannelTriggerIndex(channelID)
	if err != nil {
		return err
	}
	return s.appendChannelTriggerIndexLocked(channelID, automationID, ids)
}

// appendChannelTriggerIndexLocked appends automationID to the supplied
// (already fetched) channel-trigger-index slice and writes it back. The
// caller MUST have read currentIDs while holding indexMu and MUST still
// hold it.
func (s *KVStore) appendChannelTriggerIndexLocked(channelID, automationID string, currentIDs []string) error {
	if slices.Contains(currentIDs, automationID) {
		return nil
	}
	currentIDs = append(currentIDs, automationID)
	return s.setChannelTriggerIndex(channelID, currentIDs)
}

func (s *KVStore) removeChannelTriggerIndexLocked(channelID, automationID string) error {
	ids, err := s.getChannelTriggerIndex(channelID)
	if err != nil {
		return err
	}
	filtered := make([]string, 0, len(ids))
	for _, id := range ids {
		if id != automationID {
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

// GetAutomationIDsForUserJoinedTeam returns automation IDs triggered by user-joined-team events for the given team.
func (s *KVStore) GetAutomationIDsForUserJoinedTeam(teamID string) ([]string, error) {
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

func (s *KVStore) addUserJoinedTeamTriggerIndexLocked(teamID, automationID string) error {
	ids, err := s.getUserJoinedTeamTriggerIndex(teamID)
	if err != nil {
		return err
	}
	if slices.Contains(ids, automationID) {
		return nil
	}
	ids = append(ids, automationID)
	return s.setUserJoinedTeamTriggerIndex(teamID, ids)
}

func (s *KVStore) removeUserJoinedTeamTriggerIndexLocked(teamID, automationID string) error {
	ids, err := s.getUserJoinedTeamTriggerIndex(teamID)
	if err != nil {
		return err
	}
	filtered := make([]string, 0, len(ids))
	for _, id := range ids {
		if id != automationID {
			filtered = append(filtered, id)
		}
	}
	return s.setUserJoinedTeamTriggerIndex(teamID, filtered)
}
