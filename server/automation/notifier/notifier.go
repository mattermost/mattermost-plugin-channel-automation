// Package notifier sends DM notifications to automation creators when their
// automations fail. Notifications are rate-limited per automation and cluster-aware
// via the plugin KV store.
package notifier

import (
	"fmt"
	"time"

	mmmodel "github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
)

// NotificationCooldown is the minimum interval between failure notifications
// for the same automation. The CooldownStore TTL enforces this window cluster-wide.
const NotificationCooldown = time.Hour

// FailureDetails carries the information needed to render a failure DM.
// This is the single source of truth for failure-notification payloads;
// the workqueue package consumes the FailureNotifier interface defined
// below and constructs values of this type directly.
type FailureDetails struct {
	AutomationID       string
	AutomationName     string
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

// CreatorNotifier sends a DM from the plugin's bot user to the automation creator
// when an automation execution fails, applying a per-automation cooldown via CooldownStore
// so cluster nodes coordinate naturally.
type CreatorNotifier struct {
	api       plugin.API
	cooldown  CooldownStore
	botUserID string
}

// NewCreatorNotifier creates a CreatorNotifier. The cooldown store is
// responsible for all KV interactions; the notifier itself only handles
// DM delivery and message formatting.
func NewCreatorNotifier(api plugin.API, cooldown CooldownStore, botUserID string) *CreatorNotifier {
	return &CreatorNotifier{api: api, cooldown: cooldown, botUserID: botUserID}
}

// NotifyFailure DMs the automation creator about a failed execution. If another
// notification for the same automation has been sent within the cooldown window
// (on this node or any other cluster node), this call is a no-op.
//
// All errors are logged but not returned: notification failures must never
// affect the worker's failure-handling path.
func (n *CreatorNotifier) NotifyFailure(d FailureDetails) {
	if n == nil || n.api == nil || n.cooldown == nil {
		return
	}
	if d.CreatedBy == "" || n.botUserID == "" {
		return
	}

	claimed, err := n.cooldown.Claim(d.AutomationID)
	if err != nil {
		// On store error, suppress the notification rather than risk spamming.
		n.api.LogError("Failed to claim automation failure notification cooldown",
			"automation_id", d.AutomationID,
			"err", err.Error(),
		)
		return
	}
	if !claimed {
		return
	}

	channel, appErr := n.api.GetDirectChannel(d.CreatedBy, n.botUserID)
	if appErr != nil {
		n.api.LogError("Failed to open DM channel for automation failure notification",
			"automation_id", d.AutomationID,
			"created_by", d.CreatedBy,
			"err", appErr.Error(),
		)
		n.releaseAfterFailure(d.AutomationID)
		return
	}

	post := &mmmodel.Post{
		UserId:    n.botUserID,
		ChannelId: channel.Id,
		Message:   formatMessage(d),
	}
	if _, appErr := n.api.CreatePost(post); appErr != nil {
		n.api.LogError("Failed to post automation failure DM",
			"automation_id", d.AutomationID,
			"created_by", d.CreatedBy,
			"err", appErr.Error(),
		)
		n.releaseAfterFailure(d.AutomationID)
		return
	}
}

// releaseAfterFailure releases a previously-claimed cooldown so the next
// failure for the same automation can attempt a notification again. Errors are
// logged but never propagated.
func (n *CreatorNotifier) releaseAfterFailure(automationID string) {
	if err := n.cooldown.Release(automationID); err != nil {
		n.api.LogError("Failed to release automation failure notification cooldown",
			"automation_id", automationID,
			"err", err.Error(),
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
			"- Automation ID: `%s`\n\n"+
			"This notification is rate-limited to once per hour per automation. "+
			"Check the server logs for more details.",
		d.AutomationName, actionLine, d.ErrorMsg, channelLine, d.ExecutionID, d.AutomationID,
	)
}
