package model

import mmmodel "github.com/mattermost/mattermost/server/public/model"

// FlowContext is the runtime context built up during flow execution.
type FlowContext struct {
	Trigger TriggerData           `json:"trigger"`
	Steps   map[string]StepOutput `json:"steps"`
}

// TriggerData holds the data from the event that triggered the flow.
// It uses safe sub-structs that expose only scalar fields needed by
// templates, preventing PII leakage and blocking method calls on
// live model objects.
type TriggerData struct {
	Post     *SafePost     `json:"post,omitempty"`
	Channel  *SafeChannel  `json:"channel,omitempty"`
	User     *SafeUser     `json:"user,omitempty"`
	Schedule *ScheduleInfo `json:"schedule,omitempty"`
}

// ScheduleInfo contains metadata about a schedule trigger firing.
type ScheduleInfo struct {
	FiredAt  int64  `json:"fired_at"` // Unix ms when schedule fired
	Interval string `json:"interval"` // configured interval
}

// SafePost contains only the post fields needed for template rendering.
type SafePost struct {
	Id        string `json:"id"`
	ChannelId string `json:"channel_id"`
	Message   string `json:"message"`
}

// SafeChannel contains only the channel fields needed for template rendering.
type SafeChannel struct {
	Id          string `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
}

// SafeUser contains only the user fields needed for template rendering.
// Sensitive fields (email, AuthData, password, NotifyProps) are excluded.
type SafeUser struct {
	Id        string `json:"id"`
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

// NewSafePost creates a SafePost from a Mattermost Post.
func NewSafePost(p *mmmodel.Post) *SafePost {
	if p == nil {
		return nil
	}
	return &SafePost{
		Id:        p.Id,
		ChannelId: p.ChannelId,
		Message:   p.Message,
	}
}

// NewSafeChannel creates a SafeChannel from a Mattermost Channel.
func NewSafeChannel(c *mmmodel.Channel) *SafeChannel {
	if c == nil {
		return nil
	}
	return &SafeChannel{
		Id:          c.Id,
		Name:        c.Name,
		DisplayName: c.DisplayName,
	}
}

// NewSafeUser creates a SafeUser from a Mattermost User,
// stripping all sensitive fields.
func NewSafeUser(u *mmmodel.User) *SafeUser {
	if u == nil {
		return nil
	}
	return &SafeUser{
		Id:        u.Id,
		Username:  u.Username,
		FirstName: u.FirstName,
		LastName:  u.LastName,
	}
}

// StepOutput captures the result of an executed action.
type StepOutput struct {
	PostID    string `json:"post_id"`
	ChannelID string `json:"channel_id"`
	Message   string `json:"message"`
}
