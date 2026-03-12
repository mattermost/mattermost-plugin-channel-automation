package model

import (
	"encoding/json"
	"strings"
)

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

// MembershipChangedConfig holds trigger config for the membership_changed trigger type.
type MembershipChangedConfig struct {
	ChannelID string `json:"channel_id"`
	Action    string `json:"action,omitempty"` // "joined", "left", or "" (both)
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

// TriggerChannelID returns the channel ID from the flow's trigger config,
// regardless of trigger type. Returns empty string if no trigger is set.
func (f *Flow) TriggerChannelID() string {
	if f.Trigger.MessagePosted != nil {
		return f.Trigger.MessagePosted.ChannelID
	}
	if f.Trigger.Schedule != nil {
		return f.Trigger.Schedule.ChannelID
	}
	if f.Trigger.MembershipChanged != nil {
		return f.Trigger.MembershipChanged.ChannelID
	}
	return ""
}

// Type returns the trigger type based on which config is present.
func (t *Trigger) Type() string {
	if t.MessagePosted != nil {
		return "message_posted"
	}
	if t.Schedule != nil {
		return "schedule"
	}
	if t.MembershipChanged != nil {
		return "membership_changed"
	}
	if t.ChannelCreated != nil {
		return "channel_created"
	}
	return ""
}

// SendMessageActionConfig holds config for the send_message action type.
type SendMessageActionConfig struct {
	ChannelID     string `json:"channel_id"`
	ReplyToPostID string `json:"reply_to_post_id,omitempty"`
	AsBotID       string `json:"as_bot_id,omitempty"`
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

// Action defines a single step in a flow. Exactly one config pointer should be set.
type Action struct {
	ID          string                   `json:"id"`
	SendMessage *SendMessageActionConfig `json:"send_message,omitempty"`
	AIPrompt    *AIPromptActionConfig    `json:"ai_prompt,omitempty"`
}

// Type returns the action type based on which config is present.
func (a *Action) Type() string {
	if a.SendMessage != nil {
		return "send_message"
	}
	if a.AIPrompt != nil {
		return "ai_prompt"
	}
	return ""
}

// CollectChannelIDs returns all unique, literal (non-template) channel IDs
// referenced in the flow's trigger and actions.
func CollectChannelIDs(f *Flow) []string {
	seen := make(map[string]struct{})
	var ids []string

	add := func(id string) {
		if id == "" || strings.Contains(id, "{{") {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}

	if f.Trigger.MessagePosted != nil {
		add(f.Trigger.MessagePosted.ChannelID)
	}
	if f.Trigger.Schedule != nil {
		add(f.Trigger.Schedule.ChannelID)
	}
	if f.Trigger.MembershipChanged != nil {
		add(f.Trigger.MembershipChanged.ChannelID)
	}

	for i := range f.Actions {
		if f.Actions[i].SendMessage != nil {
			add(f.Actions[i].SendMessage.ChannelID)
		}
	}

	return ids
}
