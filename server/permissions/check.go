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
// System admins are always allowed. For channel_created flows the user must be
// a team admin on the trigger's team, and all literal channel references must
// belong to that team. For other flows the user must be a channel admin
// (SchemeAdmin) on every literal channel referenced in the flow.
//
// When no concrete channels can be verified (e.g. only templated or AI-only
// actions on a non-channel_created trigger), we require system admin permission.
func CheckFlowPermissions(api plugin.API, userID string, f *model.Flow) error {
	if api.HasPermissionTo(userID, mmmodel.PermissionManageSystem) {
		return nil
	}

	// channel_created flows use team-level authorization.
	// We call GetTeam first because HasPermissionToTeam is boolean-only and
	// cannot distinguish infrastructure failures from genuine permission denials.
	if f.Trigger.ChannelCreated != nil {
		teamID := f.Trigger.ChannelCreated.TeamID
		if _, appErr := api.GetTeam(teamID); appErr != nil {
			if appErr.StatusCode >= http.StatusInternalServerError {
				return fmt.Errorf("failed to verify team: %w", appErr)
			}
			return fmt.Errorf("team %s not found or not accessible", teamID)
		}
		if !api.HasPermissionToTeam(userID, teamID, mmmodel.PermissionManageTeam) {
			return fmt.Errorf("you must be a team admin on the team specified in the channel_created trigger")
		}
		for _, chID := range model.CollectChannelIDs(f) {
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

// ValidateTeamBotConfig checks that a flow's TeamBotConfig is structurally
// valid and that execution_mode values on actions are recognized. Channel IDs
// in TeamBotConfig are not validated here -- they are used at runtime to add
// the bot to channels before execution, and the ChannelMemberWillBeAdded hook
// enforces the public-channel restriction.
func ValidateTeamBotConfig(api plugin.API, f *model.Flow) error {
	hasTeamBotAction := false
	for _, a := range f.Actions {
		if a.AIPrompt == nil {
			continue
		}
		switch a.AIPrompt.ExecutionMode {
		case "", "creator":
			// valid
		case "team_bot":
			hasTeamBotAction = true
		default:
			return fmt.Errorf("action %q has invalid execution_mode %q; must be \"team_bot\" or \"creator\"", a.ID, a.AIPrompt.ExecutionMode)
		}
	}

	if f.TeamBotConfig == nil {
		if hasTeamBotAction {
			return fmt.Errorf("flow has actions with execution_mode \"team_bot\" but no team_bot_config")
		}
		return nil
	}

	if f.TeamBotConfig.TeamID == "" {
		return fmt.Errorf("team_bot_config.team_id is required")
	}

	if _, appErr := api.GetTeam(f.TeamBotConfig.TeamID); appErr != nil {
		if appErr.StatusCode >= http.StatusInternalServerError {
			return fmt.Errorf("failed to verify team_bot_config team: %w", appErr)
		}
		return fmt.Errorf("team %s not found or not accessible", f.TeamBotConfig.TeamID)
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
