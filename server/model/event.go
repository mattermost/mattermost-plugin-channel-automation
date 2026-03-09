package model

import mmmodel "github.com/mattermost/mattermost/server/public/model"

// Event represents something that happened in Mattermost.
type Event struct {
	Type             string
	Post             *mmmodel.Post
	Channel          *mmmodel.Channel
	User             *mmmodel.User
	MembershipAction string // "joined" or "left" for membership_changed events
}
