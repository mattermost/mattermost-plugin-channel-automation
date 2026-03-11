package pluginbridge

import "encoding/json"

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

// Trigger defines when a flow should fire. Exactly one config pointer should be set.
type Trigger struct {
	MessagePosted     *MessagePostedConfig     `json:"message_posted,omitempty"`
	Schedule          *ScheduleConfig          `json:"schedule,omitempty"`
	MembershipChanged *MembershipChangedConfig `json:"membership_changed,omitempty"`
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
	StartAt   int64  `json:"start_at,omitempty"`
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
	Body          string `json:"body"`
}

// ToolConstraints maps tool names to their parameter constraints.
type ToolConstraints map[string]map[string]ParamConstraint

// ParamConstraint defines allowed values for a tool parameter.
// Supports both static values and dynamic expansion from other tools' outputs.
type ParamConstraint struct {
	AllowedValues  []string        `json:"allowed_values,omitempty"`
	FromToolOutput []OutputBinding `json:"from_tool_output,omitempty"`
}

// OutputBinding declares that values from a source tool's MCP _meta should be
// accepted as allowed values for this parameter.
type OutputBinding struct {
	Tool  string `json:"tool"`  // source tool name
	Field string `json:"field"` // key in source tool's _meta
}

// UnmarshalJSON supports both shorthand ([]string) and full struct forms.
// Shorthand: ["val1", "val2"] → ParamConstraint{AllowedValues: ["val1", "val2"]}
// Full: {"allowed_values": [...], "from_tool_output": [...]}
func (p *ParamConstraint) UnmarshalJSON(data []byte) error {
	var values []string
	if err := json.Unmarshal(data, &values); err == nil {
		p.AllowedValues = values
		return nil
	}
	type alias ParamConstraint
	return json.Unmarshal(data, (*alias)(p))
}

// AIPromptActionConfig holds config for the ai_prompt action type.
type AIPromptActionConfig struct {
	SystemPrompt    string          `json:"system_prompt,omitempty"`
	Prompt          string          `json:"prompt"`
	ProviderType    string          `json:"provider_type"`
	ProviderID      string          `json:"provider_id"`
	AllowedTools    []string        `json:"allowed_tools,omitempty"`
	ToolConstraints ToolConstraints `json:"tool_constraints,omitempty"`
}
