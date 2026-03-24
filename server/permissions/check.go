package permissions

import (
	"errors"
	"fmt"
	"net/http"

	mmmodel "github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

// CheckFlowPermissions verifies that userID has permission to manage the flow.
// System admins are always allowed. Otherwise the user must be a channel admin
// (SchemeAdmin) on every literal channel referenced in the flow.
//
// When no concrete channels can be verified (e.g. a channel_created trigger
// with only templated or AI-only actions), we require system admin permission.
// The authorization model relies on proving the user is admin on every channel
// the flow touches. If there are zero channels to verify, we have no evidence
// the user should be allowed to manage this flow, so we deny rather than
// silently granting access to what is effectively a global-scope operation.
func CheckFlowPermissions(api plugin.API, userID string, f *model.Flow) error {
	if api.HasPermissionTo(userID, mmmodel.PermissionManageSystem) {
		return nil
	}
	channelIDs := model.CollectChannelIDs(f)
	if len(channelIDs) == 0 {
		return fmt.Errorf("system admin permission is required for flows without explicit channel references")
	}
	for _, chID := range channelIDs {
		member, appErr := api.GetChannelMember(chID, userID)
		if appErr != nil {
			// 4xx errors (not found, unauthorized) mean the user is not a member;
			// 5xx errors are infrastructure failures that should surface differently.
			if appErr.StatusCode >= http.StatusInternalServerError {
				return fmt.Errorf("failed to verify channel permissions: %w", appErr)
			}
			return fmt.Errorf("you do not have channel admin permissions on one or more channels referenced by this flow")
		}
		if !member.SchemeAdmin {
			return fmt.Errorf("you do not have channel admin permissions on one or more channels referenced by this flow")
		}
	}
	return nil
}

// HandlePermissionError determines the appropriate HTTP status and log level
// based on whether the permission check failed due to an API/infrastructure
// error (500) or the user genuinely lacking permissions (403).
func HandlePermissionError(api plugin.API, err error, userID, contextID string) (string, int, string) {
	var appErr *mmmodel.AppError
	if errors.As(err, &appErr) {
		api.LogError("Failed to verify permissions", "user_id", userID, "context_id", contextID, "error", err.Error())
		return "failed to verify permissions", http.StatusInternalServerError, err.Error()
	}
	api.LogWarn("Permission denied", "user_id", userID, "context_id", contextID, "error", err.Error())
	return err.Error(), http.StatusForbidden, ""
}
