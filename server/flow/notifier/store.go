package notifier

import (
	"fmt"
	"time"

	mmmodel "github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
)

// kvCooldownKeyPrefix namespaces the cooldown keys in the plugin KV store.
const kvCooldownKeyPrefix = "flow_failure_notify_"

// CooldownStore manages per-flow notification cooldown slots backed by the
// Mattermost plugin KV store. It exists so CreatorNotifier doesn't reach
// into plugin.API for KV operations directly, matching the Store pattern
// used by flow, execution, and workqueue.
type CooldownStore interface {
	// Claim atomically reserves the cooldown slot for flowID. Returns true if
	// this caller won the slot, false if another caller (locally or on
	// another cluster node) already holds it. The slot is auto-released by
	// the KV TTL.
	Claim(flowID string) (bool, error)
	// Release removes the slot so another notification attempt can be made.
	// Used when the DM send fails after a successful Claim.
	Release(flowID string) error
}

type kvCooldownStore struct {
	api plugin.API
	ttl time.Duration
}

// NewCooldownStore creates a KV-backed CooldownStore. The ttl controls how
// long a claimed slot remains held before it auto-expires cluster-wide.
func NewCooldownStore(api plugin.API, ttl time.Duration) CooldownStore {
	return &kvCooldownStore{api: api, ttl: ttl}
}

func (s *kvCooldownStore) Claim(flowID string) (bool, error) {
	key := kvCooldownKeyPrefix + flowID
	ok, appErr := s.api.KVSetWithOptions(key, []byte{1}, mmmodel.PluginKVSetOptions{
		Atomic:          true,
		OldValue:        nil,
		ExpireInSeconds: int64(s.ttl / time.Second),
	})
	if appErr != nil {
		return false, fmt.Errorf("failed to claim cooldown for flow %s: %w", flowID, appErr)
	}
	return ok, nil
}

func (s *kvCooldownStore) Release(flowID string) error {
	key := kvCooldownKeyPrefix + flowID
	if appErr := s.api.KVDelete(key); appErr != nil {
		return fmt.Errorf("failed to release cooldown for flow %s: %w", flowID, appErr)
	}
	return nil
}
