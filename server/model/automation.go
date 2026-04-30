package model

import "strings"

// Valid AIPromptActionConfig.ProviderType values. Agents are bots that go
// through the per-user bridge ACL and can use tools; services are raw LLM
// completion endpoints that cannot.
const (
	AIProviderTypeAgent   = "agent"
	AIProviderTypeService = "service"
)

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

// GuardrailsForAction returns the guardrails configured on the AI prompt
// action with the given ID, or nil if the action does not exist, is not an
// AI prompt action, or has no guardrails configured.
func (a *Automation) GuardrailsForAction(actionID string) *Guardrails {
	for i := range a.Actions {
		act := &a.Actions[i]
		if act.ID == actionID && act.AIPrompt != nil && act.AIPrompt.Guardrails != nil {
			return act.AIPrompt.Guardrails
		}
	}
	return nil
}

// TriggerChannelID returns the channel ID from the automation's trigger config,
// regardless of trigger type. Returns empty string if no trigger is set.
func (a *Automation) TriggerChannelID() string {
	if a.Trigger.MessagePosted != nil {
		return a.Trigger.MessagePosted.ChannelID
	}
	if a.Trigger.Schedule != nil {
		return a.Trigger.Schedule.ChannelID
	}
	if a.Trigger.MembershipChanged != nil {
		return a.Trigger.MembershipChanged.ChannelID
	}
	return ""
}

// Type returns the trigger type based on which config is present.
func (t *Trigger) Type() string {
	if t.MessagePosted != nil {
		return TriggerTypeMessagePosted
	}
	if t.Schedule != nil {
		return TriggerTypeSchedule
	}
	if t.MembershipChanged != nil {
		return TriggerTypeMembershipChanged
	}
	if t.ChannelCreated != nil {
		return TriggerTypeChannelCreated
	}
	if t.UserJoinedTeam != nil {
		return TriggerTypeUserJoinedTeam
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

// AIPromptActionConfig holds config for the ai_prompt action type.
type AIPromptActionConfig struct {
	SystemPrompt string      `json:"system_prompt,omitempty"`
	Prompt       string      `json:"prompt"`
	ProviderType string      `json:"provider_type"`
	ProviderID   string      `json:"provider_id"`
	AllowedTools []string    `json:"allowed_tools,omitempty"`
	Guardrails   *Guardrails `json:"guardrails,omitempty"`
}

// Action defines a single step in an automation. Exactly one config pointer should be set.
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
// referenced in the automation's trigger and actions.
func CollectChannelIDs(a *Automation) []string {
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

	if a.Trigger.MessagePosted != nil {
		add(a.Trigger.MessagePosted.ChannelID)
	}
	if a.Trigger.Schedule != nil {
		add(a.Trigger.Schedule.ChannelID)
	}
	if a.Trigger.MembershipChanged != nil {
		add(a.Trigger.MembershipChanged.ChannelID)
	}

	for i := range a.Actions {
		if a.Actions[i].SendMessage != nil {
			add(a.Actions[i].SendMessage.ChannelID)
		}
	}

	return ids
}

// CollectTeamIDs returns all unique, literal (non-template) team IDs
// referenced in the automation's trigger.
func CollectTeamIDs(a *Automation) []string {
	if a.Trigger.UserJoinedTeam != nil {
		id := a.Trigger.UserJoinedTeam.TeamID
		if id != "" && !strings.Contains(id, "{{") {
			return []string{id}
		}
	}
	return nil
}
