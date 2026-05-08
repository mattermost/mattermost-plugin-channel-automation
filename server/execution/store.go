package execution

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/mattermost/mattermost/server/public/plugin"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

const (
	recordKeyPrefix       = "xh:"
	automationIndexPrefix = "xhi:"
	globalIndexKey        = "xh_index"

	maxAutomationIndexSize = 50
	maxGlobalIndexSize     = 500
	recordTTLSeconds       = 7 * 24 * 60 * 60 // 7 days
	maxMessageBytes        = 16 * 1024        // 16 KB
)

// Store is a KV-backed execution history store.
type Store struct {
	api     plugin.API
	indexMu sync.Locker
}

// NewStore creates a new execution history store.
func NewStore(api plugin.API, indexMu sync.Locker) *Store {
	return &Store{api: api, indexMu: indexMu}
}

// Save persists an execution record with TTL and updates indexes.
func (s *Store) Save(record *model.ExecutionRecord) error {
	truncateSteps(record)

	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("failed to marshal execution record %s: %w", record.ID, err)
	}

	if appErr := s.api.KVSetWithExpiry(recordKeyPrefix+record.ID, data, recordTTLSeconds); appErr != nil {
		return fmt.Errorf("failed to save execution record %s: %w", record.ID, appErr)
	}

	var indexErr error
	if err := s.prependToIndex(automationIndexPrefix+record.AutomationID, record.ID, maxAutomationIndexSize); err != nil {
		s.api.LogError("Failed to update automation execution index",
			"automation_id", record.AutomationID,
			"execution_id", record.ID,
			"err", err.Error(),
		)
		indexErr = err
	}

	if err := s.prependToIndex(globalIndexKey, record.ID, maxGlobalIndexSize); err != nil {
		s.api.LogError("Failed to update global execution index",
			"execution_id", record.ID,
			"err", err.Error(),
		)
		indexErr = err
	}

	return indexErr
}

// Get retrieves a single execution record by ID.
func (s *Store) Get(id string) (*model.ExecutionRecord, error) {
	data, appErr := s.api.KVGet(recordKeyPrefix + id)
	if appErr != nil {
		return nil, fmt.Errorf("failed to get execution record %s: %w", id, appErr)
	}
	if data == nil {
		return nil, nil
	}

	var record model.ExecutionRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, fmt.Errorf("failed to unmarshal execution record %s: %w", id, err)
	}
	return &record, nil
}

// ListByAutomation returns recent execution records for an automation.
func (s *Store) ListByAutomation(automationID string, limit int) ([]*model.ExecutionRecord, error) {
	return s.listFromIndex(automationIndexPrefix+automationID, limit)
}

// ListRecent returns the most recent execution records across all automations.
func (s *Store) ListRecent(limit int) ([]*model.ExecutionRecord, error) {
	return s.listFromIndex(globalIndexKey, limit)
}

// PurgeAutomation removes all execution data for a deleted automation: individual
// records, the per-automation index, and entries from the global index.
func (s *Store) PurgeAutomation(automationID string) error {
	// Read the per-automation index to discover all record IDs.
	ids, err := s.getIndex(automationIndexPrefix + automationID)
	if err != nil {
		return err
	}

	// Delete individual execution records from the KV store.
	for _, id := range ids {
		if appErr := s.api.KVDelete(recordKeyPrefix + id); appErr != nil {
			return fmt.Errorf("failed to delete execution record %s: %w", id, appErr)
		}
	}

	// Remove purged IDs from the global index.
	// Note: records evicted from the per-automation index by the size cap
	// (maxAutomationIndexSize) are not tracked here and will expire via TTL.
	if len(ids) > 0 {
		if err := s.removeIDsFromIndex(globalIndexKey, ids); err != nil {
			return fmt.Errorf("failed to clean global execution index: %w", err)
		}
	}

	// Delete the per-automation index.
	if appErr := s.api.KVDelete(automationIndexPrefix + automationID); appErr != nil {
		return fmt.Errorf("failed to delete automation execution index for %s: %w", automationID, appErr)
	}
	return nil
}

func (s *Store) listFromIndex(key string, limit int) ([]*model.ExecutionRecord, error) {
	ids, err := s.getIndex(key)
	if err != nil {
		return nil, err
	}

	if limit <= 0 {
		limit = len(ids)
	}

	records := make([]*model.ExecutionRecord, 0, limit)
	var staleIDs []string
	for _, id := range ids {
		if len(records) >= limit {
			break
		}
		rec, err := s.Get(id)
		if err != nil {
			return nil, err
		}
		if rec == nil {
			// Expired record, need cleanup from the index (TTL)
			staleIDs = append(staleIDs, id)
			continue
		}
		records = append(records, rec)
	}

	if len(staleIDs) > 0 {
		s.removeStaleFromIndex(key, staleIDs)
	}

	return records, nil
}

// removeStaleFromIndex removes expired IDs from an index. Best-effort:
// errors are silently ignored since the read already succeeded.
func (s *Store) removeStaleFromIndex(key string, staleIDs []string) {
	s.indexMu.Lock()
	defer s.indexMu.Unlock()

	ids, err := s.getIndex(key)
	if err != nil || len(ids) == 0 {
		return
	}

	staleSet := make(map[string]struct{}, len(staleIDs))
	for _, id := range staleIDs {
		staleSet[id] = struct{}{}
	}

	filtered := make([]string, 0, len(ids))
	for _, id := range ids {
		if _, stale := staleSet[id]; !stale {
			filtered = append(filtered, id)
		}
	}

	_ = s.setIndex(key, filtered)
}

// removeIDsFromIndex removes the given IDs from an index, propagating errors.
// Use this instead of removeStaleFromIndex when failures must be observable.
func (s *Store) removeIDsFromIndex(key string, removeIDs []string) error {
	s.indexMu.Lock()
	defer s.indexMu.Unlock()

	ids, err := s.getIndex(key)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}

	removeSet := make(map[string]struct{}, len(removeIDs))
	for _, id := range removeIDs {
		removeSet[id] = struct{}{}
	}

	filtered := make([]string, 0, len(ids))
	for _, id := range ids {
		if _, remove := removeSet[id]; !remove {
			filtered = append(filtered, id)
		}
	}

	return s.setIndex(key, filtered)
}

func (s *Store) getIndex(key string) ([]string, error) {
	data, appErr := s.api.KVGet(key)
	if appErr != nil {
		return nil, fmt.Errorf("failed to get index %s: %w", key, appErr)
	}
	if data == nil {
		return nil, nil
	}

	var ids []string
	if err := json.Unmarshal(data, &ids); err != nil {
		return nil, fmt.Errorf("failed to unmarshal index %s: %w", key, err)
	}
	return ids, nil
}

func (s *Store) setIndex(key string, ids []string) error {
	if len(ids) == 0 {
		if appErr := s.api.KVDelete(key); appErr != nil {
			return fmt.Errorf("failed to delete index %s: %w", key, appErr)
		}
		return nil
	}

	// Indexes share the same TTL as records. Since prependToIndex rewrites
	// the index on every save, the TTL resets each time. When an automation goes
	// inactive, the index expires alongside its last records.
	data, err := json.Marshal(ids)
	if err != nil {
		return fmt.Errorf("failed to marshal index %s: %w", key, err)
	}
	if appErr := s.api.KVSetWithExpiry(key, data, recordTTLSeconds); appErr != nil {
		return fmt.Errorf("failed to save index %s: %w", key, appErr)
	}
	return nil
}

func (s *Store) prependToIndex(key, id string, maxSize int) error {
	s.indexMu.Lock()
	defer s.indexMu.Unlock()

	ids, err := s.getIndex(key)
	if err != nil {
		return err
	}

	ids = append([]string{id}, ids...)
	if len(ids) > maxSize {
		ids = ids[:maxSize]
	}
	return s.setIndex(key, ids)
}

// truncateSteps truncates step messages that exceed maxMessageBytes.
func truncateSteps(record *model.ExecutionRecord) {
	for key, step := range record.Steps {
		if len(step.Message) > maxMessageBytes {
			step.Message = step.Message[:maxMessageBytes]
			step.Truncated = true
			record.Steps[key] = step
		}
	}
}

// Ensure Store implements model.ExecutionStore at compile time.
var _ model.ExecutionStore = (*Store)(nil)
