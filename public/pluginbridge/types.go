package pluginbridge

// Automation represents a trigger-action automation.
type Automation struct {
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

// UserJoinedTeamConfig holds trigger config for the user_joined_team trigger type.
type UserJoinedTeamConfig struct {
	TeamID   string `json:"team_id"`
	UserType string `json:"user_type,omitempty"` // "user", "guest", or "" (both)
}

// Trigger defines when an automation should fire. Exactly one config pointer should be set.
type Trigger struct {
	MessagePosted     *MessagePostedConfig     `json:"message_posted,omitempty"`
	Schedule          *ScheduleConfig          `json:"schedule,omitempty"`
	MembershipChanged *MembershipChangedConfig `json:"membership_changed,omitempty"`
	ChannelCreated    *ChannelCreatedConfig    `json:"channel_created,omitempty"`
	UserJoinedTeam    *UserJoinedTeamConfig    `json:"user_joined_team,omitempty"`
}

// MembershipChangedConfig holds trigger config for the membership_changed trigger type.
type MembershipChangedConfig struct {
	ChannelID string `json:"channel_id"`
	Action    string `json:"action,omitempty"` // "joined", "left", or "" (both)
}

// MessagePostedConfig holds trigger config for the message_posted trigger type.
type MessagePostedConfig struct {
	ChannelID            string `json:"channel_id"`
	IncludeThreadReplies bool   `json:"include_thread_replies,omitempty"` // when false (default), thread replies (posts with a non-empty RootId) do not fire this trigger
}

// ScheduleConfig holds trigger config for the schedule trigger type.
type ScheduleConfig struct {
	ChannelID string `json:"channel_id"`
	Interval  string `json:"interval"`
	StartAt   int64  `json:"start_at,omitempty"` // UTC Unix milliseconds; must be in the future if set
}

// Action defines a single step in an automation. Exactly one config pointer should be set.
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

// Guardrails constrains MCP tool calls for an ai_prompt action (opt-in).
type Guardrails struct {
	ChannelIDs []string `json:"channel_ids,omitempty"`
}

// AIPromptActionConfig holds config for the ai_prompt action type.
type AIPromptActionConfig struct {
	SystemPrompt string      `json:"system_prompt,omitempty"`
	Prompt       string      `json:"prompt"`
	ProviderType string      `json:"provider_type"`
	ProviderID   string      `json:"provider_id"`
	AllowedTools []string    `json:"allowed_tools,omitempty"`
	Guardrails   *Guardrails `json:"guardrails,omitempty"`
	// RequestAs selects which user the AI completion request is attributed to.
	// Allowed values: "" or "triggerer" (default — the user who triggered the
	// automation, falling back to the automation creator when the trigger has
	// no associated user) or "creator" (always the automation creator).
	RequestAs string `json:"request_as,omitempty"`
}
