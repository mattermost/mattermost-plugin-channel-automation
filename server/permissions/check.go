package permissions

import (
	"errors"
	"fmt"
	"net/http"

	mmmodel "github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

// CheckAutomationPermissions verifies that userID has permission to manage the automation.
// System admins are always allowed. For channel_created automations the user must be
// a team admin on the trigger's team, and all literal channel references must
// belong to that team. For other automations the user must be a channel admin
// (SchemeAdmin) on every literal channel referenced in the automation.
//
// When no concrete channels can be verified (e.g. only templated or AI-only
// actions on a non-channel_created trigger), we require system admin permission.
func CheckAutomationPermissions(api plugin.API, userID string, a *model.Automation) error {
	if api.HasPermissionTo(userID, mmmodel.PermissionManageSystem) {
		return nil
	}

	// channel_created and user_joined_team automations use team-level authorization.
	// We call GetTeam first because HasPermissionToTeam is boolean-only and
	// cannot distinguish infrastructure failures from genuine permission denials.
	if a.Trigger.ChannelCreated != nil {
		teamID := a.Trigger.ChannelCreated.TeamID
		if _, appErr := api.GetTeam(teamID); appErr != nil {
			if appErr.StatusCode >= http.StatusInternalServerError {
				return fmt.Errorf("failed to verify team: %w", appErr)
			}
			return fmt.Errorf("team %s not found or not accessible", teamID)
		}
		if !api.HasPermissionToTeam(userID, teamID, mmmodel.PermissionManageTeam) {
			return fmt.Errorf("you must be a team admin on the team specified in the channel_created trigger")
		}
		for _, chID := range model.CollectChannelIDs(a) {
			ch, appErr := api.GetChannel(chID)
			if appErr != nil {
				if appErr.StatusCode >= http.StatusInternalServerError {
					return fmt.Errorf("failed to verify channel team membership: %w", appErr)
				}
				return fmt.Errorf("channel %s not found or not accessible", chID)
			}
			if ch.TeamId != teamID {
				return fmt.Errorf("channel %s does not belong to the team specified in the channel_created trigger", chID)
			}
		}
		return nil
	}

	if a.Trigger.UserJoinedTeam != nil {
		teamIDs := model.CollectTeamIDs(a)
		if len(teamIDs) == 0 {
			return fmt.Errorf("system admin permission is required for automations without explicit team references")
		}
		for _, teamID := range teamIDs {
			if _, appErr := api.GetTeam(teamID); appErr != nil {
				if appErr.StatusCode >= http.StatusInternalServerError {
					return fmt.Errorf("failed to verify team: %w", appErr)
				}
				return fmt.Errorf("team %s not found or not accessible", teamID)
			}
			if !api.HasPermissionToTeam(userID, teamID, mmmodel.PermissionManageTeam) {
				return fmt.Errorf("you must be a team admin on all teams referenced by this automation")
			}
		}
		return nil
	}

	channelIDs := model.CollectChannelIDs(a)
	if len(channelIDs) == 0 {
		return fmt.Errorf("system admin permission is required for automations without explicit channel references")
	}
	for _, chID := range channelIDs {
		member, appErr := api.GetChannelMember(chID, userID)
		if appErr != nil {
			// 4xx errors (not found, unauthorized) mean the user is not a member;
			// 5xx errors are infrastructure failures that should surface differently.
			if appErr.StatusCode >= http.StatusInternalServerError {
				return fmt.Errorf("failed to verify channel permissions: %w", appErr)
			}
			return fmt.Errorf("you do not have channel admin permissions on one or more channels referenced by this automation")
		}
		if !member.SchemeAdmin {
			return fmt.Errorf("you do not have channel admin permissions on one or more channels referenced by this automation")
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
