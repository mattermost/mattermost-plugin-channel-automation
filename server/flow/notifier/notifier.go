// Package notifier sends DM notifications to flow creators when their
// automations fail. Notifications are rate-limited per flow and cluster-aware
// via the plugin KV store.
package notifier

import (
	"fmt"
	"time"

	mmmodel "github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
)

// notificationCooldown is the minimum interval between failure notifications
// for the same flow. The KV store entry's TTL enforces this window cluster-wide.
const notificationCooldown = time.Hour

// kvKeyPrefix namespaces the cooldown keys in the plugin KV store.
const kvKeyPrefix = "flow_failure_notify_"

// FailureDetails carries the information needed to render a failure DM.
// This is the single source of truth for failure-notification payloads;
// the workqueue package consumes the FailureNotifier interface defined
// below and constructs values of this type directly.
type FailureDetails struct {
	FlowID             string
	FlowName           string
	CreatedBy          string
	ActionID           string
	ActionType         string
	ErrorMsg           string
	ExecutionID        string
	ChannelID          string // optional; the triggering channel, if known
	ChannelDisplayName string // optional; human-readable channel name for the message
}

// FailureNotifier is the interface satisfied by CreatorNotifier and consumed
// by the worker pool. Defined here (rather than in workqueue) so callers
// share a single FailureDetails type.
type FailureNotifier interface {
	NotifyFailure(d FailureDetails)
}

// CreatorNotifier sends a DM from the plugin's bot user to the flow creator
// when a flow execution fails, applying a per-flow cooldown via the
// Mattermost plugin KV store so cluster nodes coordinate naturally.
type CreatorNotifier struct {
	api       plugin.API
	botUserID string
}

// NewCreatorNotifier creates a CreatorNotifier.
func NewCreatorNotifier(api plugin.API, botUserID string) *CreatorNotifier {
	return &CreatorNotifier{api: api, botUserID: botUserID}
}

// NotifyFailure DMs the flow creator about a failed execution. If another
// notification for the same flow has been sent within notificationCooldown
// (on this node or any other cluster node), this call is a no-op.
//
// All errors are logged but not returned: notification failures must never
// affect the worker's failure-handling path.
func (n *CreatorNotifier) NotifyFailure(d FailureDetails) {
	if n == nil || n.api == nil {
		return
	}
	if d.CreatedBy == "" || n.botUserID == "" {
		return
	}

	if !n.claimCooldown(d.FlowID) {
		return
	}

	channel, appErr := n.api.GetDirectChannel(d.CreatedBy, n.botUserID)
	if appErr != nil {
		n.api.LogError("Failed to open DM channel for flow failure notification",
			"flow_id", d.FlowID,
			"created_by", d.CreatedBy,
			"err", appErr.Error(),
		)
		n.releaseCooldown(d.FlowID)
		return
	}

	post := &mmmodel.Post{
		UserId:    n.botUserID,
		ChannelId: channel.Id,
		Message:   formatMessage(d),
	}
	if _, appErr := n.api.CreatePost(post); appErr != nil {
		n.api.LogError("Failed to post flow failure DM",
			"flow_id", d.FlowID,
			"created_by", d.CreatedBy,
			"err", appErr.Error(),
		)
		n.releaseCooldown(d.FlowID)
		return
	}
}

// claimCooldown attempts to atomically reserve the cooldown slot for flowID.
// Returns true if this caller "won" and should send the DM, false if another
// caller (or a recent prior call on any node) already holds the slot.
//
// The KV TTL handles expiry, so the slot becomes claimable again automatically
// after notificationCooldown elapses.
func (n *CreatorNotifier) claimCooldown(flowID string) bool {
	key := kvKeyPrefix + flowID
	ok, appErr := n.api.KVSetWithOptions(key, []byte{1}, mmmodel.PluginKVSetOptions{
		Atomic:          true,
		OldValue:        nil,
		ExpireInSeconds: int64(notificationCooldown / time.Second),
	})
	if appErr != nil {
		// On KV error, suppress the notification rather than risk spamming.
		n.api.LogError("Failed to claim flow failure notification cooldown",
			"flow_id", flowID,
			"err", appErr.Error(),
		)
		return false
	}
	return ok
}

// releaseCooldown removes the cooldown entry so another notification attempt
// can be made. Called when the notification fails after claiming the cooldown.
func (n *CreatorNotifier) releaseCooldown(flowID string) {
	key := kvKeyPrefix + flowID
	appErr := n.api.KVDelete(key)
	if appErr != nil {
		n.api.LogError("Failed to release flow failure notification cooldown",
			"flow_id", flowID,
			"err", appErr.Error(),
		)
	}
}

func formatMessage(d FailureDetails) string {
	var channelLine string
	switch {
	case d.ChannelDisplayName != "" && d.ChannelID != "":
		channelLine = fmt.Sprintf("- Channel: %s (`%s`)\n", d.ChannelDisplayName, d.ChannelID)
	case d.ChannelID != "":
		channelLine = fmt.Sprintf("- Channel: `%s`\n", d.ChannelID)
	}

	var actionLine string
	if d.ActionID != "" || d.ActionType != "" {
		actionLine = fmt.Sprintf("- Action: `%s` (`%s`)\n", d.ActionID, d.ActionType)
	}

	return fmt.Sprintf(
		"Automation %q failed.\n\n"+
			"%s"+
			"- Error: %s\n"+
			"%s"+
			"- Execution ID: `%s`\n"+
			"- Flow ID: `%s`\n\n"+
			"This notification is rate-limited to once per hour per flow. "+
			"Check the server logs for more details.",
		d.FlowName, actionLine, d.ErrorMsg, channelLine, d.ExecutionID, d.FlowID,
	)
}
