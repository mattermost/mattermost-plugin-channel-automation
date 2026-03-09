package execution

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/mattermost/mattermost/server/public/plugin"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

const (
	recordKeyPrefix = "xh:"
	flowIndexPrefix = "xhi:"
	globalIndexKey  = "xh_index"

	maxFlowIndexSize   = 50
	maxGlobalIndexSize = 500
	recordTTLSeconds   = 7 * 24 * 60 * 60 // 7 days
	maxMessageBytes    = 16 * 1024        // 16 KB
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

	if err := s.prependToIndex(flowIndexPrefix+record.FlowID, record.ID, maxFlowIndexSize); err != nil {
		s.api.LogError("Failed to update flow execution index",
			"flow_id", record.FlowID,
			"execution_id", record.ID,
			"err", err.Error(),
		)
	}

	if err := s.prependToIndex(globalIndexKey, record.ID, maxGlobalIndexSize); err != nil {
		s.api.LogError("Failed to update global execution index",
			"execution_id", record.ID,
			"err", err.Error(),
		)
	}

	return nil
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

// ListByFlow returns recent execution records for a flow.
func (s *Store) ListByFlow(flowID string, limit int) ([]*model.ExecutionRecord, error) {
	return s.listFromIndex(flowIndexPrefix+flowID, limit)
}

// ListRecent returns the most recent execution records across all flows.
func (s *Store) ListRecent(limit int) ([]*model.ExecutionRecord, error) {
	return s.listFromIndex(globalIndexKey, limit)
}

// PurgeFlow removes the per-flow index for a deleted flow.
// Individual records are left to expire via TTL.
func (s *Store) PurgeFlow(flowID string) error {
	if appErr := s.api.KVDelete(flowIndexPrefix + flowID); appErr != nil {
		return fmt.Errorf("failed to delete flow execution index for %s: %w", flowID, appErr)
	}
	return nil
}

func (s *Store) listFromIndex(key string, limit int) ([]*model.ExecutionRecord, error) {
	ids, err := s.getIndex(key)
	if err != nil {
		return nil, err
	}

	if limit <= 0 || limit > len(ids) {
		limit = len(ids)
	}

	records := make([]*model.ExecutionRecord, 0, limit)
	for _, id := range ids[:limit] {
		rec, err := s.Get(id)
		if err != nil {
			return nil, err
		}
		if rec == nil {
			continue // expired via TTL
		}
		records = append(records, rec)
	}
	return records, nil
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

	// Indexes don't need TTL — they're bounded by cap and cleaned on purge.
	// Records themselves expire, so stale index entries just resolve to nil.
	data, err := json.Marshal(ids)
	if err != nil {
		return fmt.Errorf("failed to marshal index %s: %w", key, err)
	}
	if appErr := s.api.KVSet(key, data); appErr != nil {
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

// NowMillis returns the current time in milliseconds. Exposed for testing.
var NowMillis = func() int64 {
	return time.Now().UnixMilli()
}
