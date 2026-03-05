package model

import "strings"

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

// Trigger defines when a flow should fire. Exactly one config pointer should be set.
type Trigger struct {
	MessagePosted *MessagePostedConfig `json:"message_posted,omitempty"`
	Schedule      *ScheduleConfig      `json:"schedule,omitempty"`
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
	return ""
}

// SendMessageActionConfig holds config for the send_message action type.
type SendMessageActionConfig struct {
	ChannelID     string `json:"channel_id"`
	ReplyToPostID string `json:"reply_to_post_id,omitempty"`
	Body          string `json:"body"`
}

// AIPromptActionConfig holds config for the ai_prompt action type.
type AIPromptActionConfig struct {
	Prompt       string `json:"prompt"`
	ProviderType string `json:"provider_type"`
	ProviderID   string `json:"provider_id"`
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

	for i := range f.Actions {
		if f.Actions[i].SendMessage != nil {
			add(f.Actions[i].SendMessage.ChannelID)
		}
	}

	return ids
}
