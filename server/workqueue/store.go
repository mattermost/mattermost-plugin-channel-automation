package workqueue

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/mattermost/mattermost/server/public/plugin"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

const (
	workItemKeyPrefix = "wq:"
	pendingIndexKey   = "wq_pending"
	runningIndexKey   = "wq_running"
)

// Store persists work items using the Mattermost plugin KV store.
// Index mutations are serialized via indexMu to prevent lost updates
// when multiple plugin instances operate on the same queue.
type Store struct {
	api     plugin.API
	indexMu sync.Locker
}

// NewStore creates a new KV-backed work queue store. The caller must
// supply a cluster-safe mutex (e.g. cluster.Mutex) to protect index
// read-modify-write cycles across plugin instances.
func NewStore(api plugin.API, indexMu sync.Locker) *Store {
	return &Store{api: api, indexMu: indexMu}
}

// Get retrieves a work item by ID. Returns nil if not found.
func (s *Store) Get(id string) (*model.WorkItem, error) {
	data, appErr := s.api.KVGet(workItemKeyPrefix + id)
	if appErr != nil {
		return nil, fmt.Errorf("failed to get work item %s: %w", id, appErr)
	}
	if data == nil {
		return nil, nil
	}

	var item model.WorkItem
	if err := json.Unmarshal(data, &item); err != nil {
		return nil, fmt.Errorf("failed to unmarshal work item %s: %w", id, err)
	}
	return &item, nil
}

// Enqueue saves a work item and appends it to the pending index.
func (s *Store) Enqueue(item *model.WorkItem) error {
	item.Status = model.WorkItemStatusPending
	item.CreatedAt = time.Now().UnixMilli()

	data, err := json.Marshal(item)
	if err != nil {
		return fmt.Errorf("failed to marshal work item %s: %w", item.ID, err)
	}

	if appErr := s.api.KVSet(workItemKeyPrefix+item.ID, data); appErr != nil {
		return fmt.Errorf("failed to save work item %s: %w", item.ID, appErr)
	}

	if err := s.appendToIndex(pendingIndexKey, item.ID); err != nil {
		// Data is saved but not indexed. Caller can retry, and the item
		// will be overwritten. Orphaned data is harmless.
		return fmt.Errorf("failed to add work item %s to pending index: %w", item.ID, err)
	}

	return nil
}

// ClaimNext pops the first item from the pending index, moves it to the
// running index, and updates its status to running. Returns nil if the
// queue is empty. Holds indexMu for the entire operation to atomically
// move an item between the two indexes.
func (s *Store) ClaimNext() (*model.WorkItem, error) {
	s.indexMu.Lock()
	defer s.indexMu.Unlock()

	for {
		// Pop first entry from pending index.
		pendingIDs, err := s.getIndex(pendingIndexKey)
		if err != nil {
			return nil, err
		}
		if len(pendingIDs) == 0 {
			return nil, nil
		}

		id := pendingIDs[0]
		err = s.setIndex(pendingIndexKey, pendingIDs[1:])
		if err != nil {
			return nil, err
		}

		var item *model.WorkItem
		item, err = s.Get(id)
		if err != nil {
			return nil, err
		}
		if item == nil {
			// Stale index entry — item was deleted. Try the next one.
			continue
		}

		item.Status = model.WorkItemStatusRunning
		item.StartedAt = time.Now().UnixMilli()

		data, err := json.Marshal(item)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal work item %s: %w", id, err)
		}

		if appErr := s.api.KVSet(workItemKeyPrefix+id, data); appErr != nil {
			return nil, fmt.Errorf("failed to update work item %s: %w", id, appErr)
		}

		// Append to running index. If this fails, roll back to avoid
		// losing the item (it would be in neither index).
		runningIDs, err := s.getIndex(runningIndexKey)
		if err != nil {
			// Roll back: re-add to pending and reset status.
			item.Status = model.WorkItemStatusPending
			item.StartedAt = 0
			if rbData, rbErr := json.Marshal(item); rbErr == nil {
				if appErr := s.api.KVSet(workItemKeyPrefix+id, rbData); appErr != nil {
					s.api.LogError("rollback failed - work item may be orphaned",
						"work_item_id", id, "err", appErr.Error())
				}
			}
			if rbErr := s.setIndex(pendingIndexKey, append([]string{id}, pendingIDs[1:]...)); rbErr != nil {
				s.api.LogError("rollback failed - pending index restore failed",
					"work_item_id", id, "err", rbErr.Error())
			}
			return nil, err
		}
		if err := s.setIndex(runningIndexKey, append(runningIDs, id)); err != nil {
			// Roll back: re-add to pending and reset status.
			item.Status = model.WorkItemStatusPending
			item.StartedAt = 0
			if rbData, rbErr := json.Marshal(item); rbErr == nil {
				if appErr := s.api.KVSet(workItemKeyPrefix+id, rbData); appErr != nil {
					s.api.LogError("rollback failed - work item may be orphaned",
						"work_item_id", id, "err", appErr.Error())
				}
			}
			if rbErr := s.setIndex(pendingIndexKey, append([]string{id}, pendingIDs[1:]...)); rbErr != nil {
				s.api.LogError("rollback failed - pending index restore failed",
					"work_item_id", id, "err", rbErr.Error())
			}
			return nil, err
		}

		return item, nil
	}
}

// Complete removes a work item from the running index and deletes it
// from the KV store.
func (s *Store) Complete(id string) error {
	if appErr := s.api.KVDelete(workItemKeyPrefix + id); appErr != nil {
		return fmt.Errorf("failed to delete work item %s: %w", id, appErr)
	}

	// Remove from running index. If this fails, the stale entry is
	// harmless: ClaimNext skips items whose KV data is nil, and
	// ResetRunningToPending also skips nil items.
	return s.removeFromIndex(runningIndexKey, id)
}

// Fail removes a work item from the running index and deletes it from
// the KV store. Failure details are captured in the execution record
// by the caller, so the work item itself is no longer needed.
func (s *Store) Fail(id string) error {
	if appErr := s.api.KVDelete(workItemKeyPrefix + id); appErr != nil {
		return fmt.Errorf("failed to delete work item %s: %w", id, appErr)
	}

	return s.removeFromIndex(runningIndexKey, id)
}

// ResetRunningToPending moves all items from the running index back to
// the pending index and resets their status. Used for crash recovery on
// startup. Returns the number of items reset. Holds indexMu for the
// entire operation to atomically drain running and prepend to pending.
func (s *Store) ResetRunningToPending() (int, error) {
	s.indexMu.Lock()
	defer s.indexMu.Unlock()

	runningIDs, err := s.getIndex(runningIndexKey)
	if err != nil {
		return 0, err
	}
	if len(runningIDs) == 0 {
		return 0, nil
	}

	pendingIDs, err := s.getIndex(pendingIndexKey)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, id := range runningIDs {
		item, err := s.Get(id)
		if err != nil {
			return count, err
		}
		if item == nil {
			continue
		}

		item.Status = model.WorkItemStatusPending
		item.StartedAt = 0

		data, err := json.Marshal(item)
		if err != nil {
			return count, fmt.Errorf("failed to marshal work item %s: %w", id, err)
		}

		if appErr := s.api.KVSet(workItemKeyPrefix+id, data); appErr != nil {
			return count, fmt.Errorf("failed to update work item %s: %w", id, appErr)
		}

		pendingIDs = append(pendingIDs, id)
		count++
	}

	// Clear the running index FIRST.
	if err := s.setIndex(runningIndexKey, nil); err != nil {
		return count, err
	}

	// Deduplicate pending IDs in case of prior partial failures.
	seen := make(map[string]struct{}, len(pendingIDs))
	deduped := make([]string, 0, len(pendingIDs))
	for _, id := range pendingIDs {
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			deduped = append(deduped, id)
		}
	}

	if err := s.setIndex(pendingIndexKey, deduped); err != nil {
		return count, err
	}

	return count, nil
}

// --- Index helpers ---
//
// getIndex and setIndex are raw primitives without locking. They are
// used by ClaimNext and ResetRunningToPending which hold indexMu at
// the method level.
//
// appendToIndex, removeFromIndex, and popFromIndex acquire indexMu
// themselves and are used by the simpler public methods (Enqueue,
// Complete, Fail).

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

	data, err := json.Marshal(ids)
	if err != nil {
		return fmt.Errorf("failed to marshal index %s: %w", key, err)
	}
	if appErr := s.api.KVSet(key, data); appErr != nil {
		return fmt.Errorf("failed to save index %s: %w", key, appErr)
	}
	return nil
}

func (s *Store) appendToIndex(key, id string) error {
	s.indexMu.Lock()
	defer s.indexMu.Unlock()

	ids, err := s.getIndex(key)
	if err != nil {
		return err
	}
	ids = append(ids, id)
	return s.setIndex(key, ids)
}

func (s *Store) removeFromIndex(key, id string) error {
	s.indexMu.Lock()
	defer s.indexMu.Unlock()

	ids, err := s.getIndex(key)
	if err != nil {
		return err
	}
	filtered := make([]string, 0, len(ids))
	for _, existingID := range ids {
		if existingID != id {
			filtered = append(filtered, existingID)
		}
	}
	return s.setIndex(key, filtered)
}
