package trigger

import (
	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

// isChannelMember reports whether userID is currently a member of channelID.
// Returns false on any lookup error so an unverifiable caller is never
// silently authorized.
func isChannelMember(api model.HookCallerAPI, channelID, userID string) bool {
	if channelID == "" || userID == "" {
		return false
	}
	member, appErr := api.GetChannelMember(channelID, userID)
	return appErr == nil && member != nil
}

// isTeamMember reports whether userID is currently an active member of teamID
// (DeleteAt == 0). Returns false on any lookup error.
func isTeamMember(api model.HookCallerAPI, teamID, userID string) bool {
	if teamID == "" || userID == "" {
		return false
	}
	member, appErr := api.GetTeamMember(teamID, userID)
	return appErr == nil && member != nil && member.DeleteAt == 0
}

// userMatchesUserType applies a user_joined_team-style user_type filter.
// Empty user_type accepts both users and guests; "user" rejects guests;
// "guest" requires a guest. Validation rejects other values at save time, so
// any unexpected value here is treated as a non-match.
func userMatchesUserType(api model.HookCallerAPI, userID, userType string) bool {
	if userType == "" {
		return true
	}
	user, appErr := api.GetUser(userID)
	if appErr != nil || user == nil {
		return false
	}
	switch userType {
	case "guest":
		return user.IsGuest()
	case "user":
		return !user.IsGuest()
	}
	return false
}
