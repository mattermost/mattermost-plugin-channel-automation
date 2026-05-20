package model

import mmmodel "github.com/mattermost/mattermost/server/public/model"

// Event represents something that happened in Mattermost.
type Event struct {
	Type             string
	Post             *mmmodel.Post
	Channel          *mmmodel.Channel
	User             *mmmodel.User
	Team             *mmmodel.Team
	MembershipAction string // "joined" or "left" for membership_changed events

	// AutomationBotUserID is the plugin's default automation bot user ID.
	// Message-posted handlers use it to skip self-triggered loop posts.
	AutomationBotUserID string
}
