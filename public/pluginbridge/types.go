package pluginbridge

import "github.com/mattermost/mattermost-plugin-ai/public/bridgeclient"

// Flow represents a trigger-action workflow.
type Flow struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Enabled   bool     `json:"enabled"`
	Trigger   Trigger  `json:"trigger"`
	Actions   []Action `json:"actions"`
	CreatedAt int64    `json:"created_at"`
	UpdatedAt int64    `json:"updated_at"`
	CreatedBy string   `json:"created_by"`
}

// ChannelCreatedConfig holds trigger config for the channel_created trigger type.
// No fields are needed — the trigger fires on any new public channel.
type ChannelCreatedConfig struct{}

// Trigger defines when a flow should fire. Exactly one config pointer should be set.
type Trigger struct {
	MessagePosted     *MessagePostedConfig     `json:"message_posted,omitempty"`
	Schedule          *ScheduleConfig          `json:"schedule,omitempty"`
	MembershipChanged *MembershipChangedConfig `json:"membership_changed,omitempty"`
	ChannelCreated    *ChannelCreatedConfig    `json:"channel_created,omitempty"`
}

// MembershipChangedConfig holds trigger config for the membership_changed trigger type.
type MembershipChangedConfig struct {
	ChannelID string `json:"channel_id"`
	Action    string `json:"action,omitempty"` // "joined", "left", or "" (both)
}

// MessagePostedConfig holds trigger config for the message_posted trigger type.
type MessagePostedConfig struct {
	ChannelID string `json:"channel_id"`
}

// ScheduleConfig holds trigger config for the schedule trigger type.
type ScheduleConfig struct {
	ChannelID string `json:"channel_id"`
	Interval  string `json:"interval"`
	StartAt   int64  `json:"start_at,omitempty"` // UTC Unix milliseconds; must be in the future if set
}

// Action defines a single step in a flow. Exactly one config pointer should be set.
type Action struct {
	ID          string                   `json:"id"`
	SendMessage *SendMessageActionConfig `json:"send_message,omitempty"`
	AIPrompt    *AIPromptActionConfig    `json:"ai_prompt,omitempty"`
}

// SendMessageActionConfig holds config for the send_message action type.
type SendMessageActionConfig struct {
	ChannelID     string `json:"channel_id"`
	ReplyToPostID string `json:"reply_to_post_id,omitempty"`
	AsBotID       string `json:"as_bot_id,omitempty"`
	Body          string `json:"body"`
}

// AIPromptActionConfig holds config for the ai_prompt action type.
type AIPromptActionConfig struct {
	SystemPrompt string   `json:"system_prompt,omitempty"`
	Prompt       string   `json:"prompt"`
	ProviderType string   `json:"provider_type"`
	ProviderID   string   `json:"provider_id"`
	AllowedTools bridgeclient.AllowedToolsList `json:"allowed_tools,omitempty"`
}
