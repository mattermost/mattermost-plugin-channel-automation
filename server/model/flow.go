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
	StartAt   int64  `json:"start_at,omitempty"` // UTC Unix milliseconds; must be in the future if set
}

// MembershipChangedConfig holds trigger config for the membership_changed trigger type.
type MembershipChangedConfig struct {
	ChannelID string `json:"channel_id"`
	Action    string `json:"action,omitempty"` // "joined", "left", or "" (both)
}

// ChannelCreatedConfig holds trigger config for the channel_created trigger type.
// The trigger fires when a new public channel is created on the specified team.
type ChannelCreatedConfig struct {
	TeamID string `json:"team_id"`
}

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

// GuardrailChannel is the runtime representation of one allowed channel and
// the team it belongs to. The TeamID is never serialized — it is resolved on
// demand by the hooks layer (channel -> team is immutable in Mattermost, so a
// process-wide cache keeps lookups cheap).
type GuardrailChannel struct {
	ChannelID string `json:"-"`
	TeamID    string `json:"-"`
}

// Guardrails constrains MCP tool calls for an ai_prompt action (opt-in).
// Additional fields may be added over time without breaking callers.
//
// The JSON wire shape is {"channel_ids": ["..."]}: callers do not need to
// know about teams, and storage round-trips through the same shape. Internal
// code uses Channels and may populate TeamID for fast lookup.
type Guardrails struct {
	Channels []GuardrailChannel `json:"-"`
}

// guardrailsJSON is the on-the-wire shape for Guardrails.
type guardrailsJSON struct {
	ChannelIDs []string `json:"channel_ids,omitempty"`
}

// MarshalJSON emits {"channel_ids": ["..."]}, hiding any resolved team IDs.
func (g Guardrails) MarshalJSON() ([]byte, error) {
	if len(g.Channels) == 0 {
		return json.Marshal(guardrailsJSON{})
	}
	ids := make([]string, 0, len(g.Channels))
	for _, c := range g.Channels {
		ids = append(ids, c.ChannelID)
	}
	return json.Marshal(guardrailsJSON{ChannelIDs: ids})
}

// UnmarshalJSON accepts {"channel_ids": ["..."]} and populates Channels with
// empty TeamIDs. Resolution happens on demand at hook time.
func (g *Guardrails) UnmarshalJSON(data []byte) error {
	var aux guardrailsJSON
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	g.Channels = nil
	if len(aux.ChannelIDs) == 0 {
		return nil
	}
	g.Channels = make([]GuardrailChannel, 0, len(aux.ChannelIDs))
	for _, id := range aux.ChannelIDs {
		g.Channels = append(g.Channels, GuardrailChannel{ChannelID: id})
	}
	return nil
}

// AIPromptActionConfig holds config for the ai_prompt action type.
type AIPromptActionConfig struct {
	SystemPrompt string      `json:"system_prompt,omitempty"`
	Prompt       string      `json:"prompt"`
	ProviderType string      `json:"provider_type"`
	ProviderID   string      `json:"provider_id"`
	AllowedTools []string    `json:"allowed_tools,omitempty"`
	Guardrails   *Guardrails `json:"guardrails,omitempty"`
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
		if f.Actions[i].AIPrompt != nil && f.Actions[i].AIPrompt.Guardrails != nil {
			for _, c := range f.Actions[i].AIPrompt.Guardrails.Channels {
				add(c.ChannelID)
			}
		}
	}

	return ids
}
