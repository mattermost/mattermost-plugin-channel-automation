package model

import (
	"fmt"
	"regexp"
	"strings"
)

var actionIDPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)

// ValidateTriggerExclusivity checks that exactly one trigger-config pointer
// is set. Per-type field validation lives on each TriggerHandler and is
// dispatched by the flow package's validator.
func ValidateTriggerExclusivity(t *Trigger) error {
	count := 0
	if t.MessagePosted != nil {
		count++
	}
	if t.Schedule != nil {
		count++
	}
	if t.MembershipChanged != nil {
		count++
	}
	if t.ChannelCreated != nil {
		count++
	}
	if t.UserJoinedTeam != nil {
		count++
	}
	if count == 0 {
		return fmt.Errorf("exactly one trigger type must be set")
	}
	if count > 1 {
		return fmt.Errorf("exactly one trigger type must be set, got %d", count)
	}
	return nil
}

// ValidateSendMessageChannel checks that every send_message action in the flow
// targets the same channel that the trigger is bound to. For triggers with a
// channel_id (message_posted, schedule, membership_changed), the action channel
// must be either the literal trigger channel ID or a template containing
// ".Trigger.Channel.Id". For triggers without a bound channel (channel_created,
// user_joined_team), any Go template expression is accepted (e.g.
// "{{.Trigger.Channel.Id}}" or "{{.Trigger.Team.DefaultChannelId}}").
func ValidateSendMessageChannel(f *Flow) error {
	triggerChannelID := f.TriggerChannelID()

	for i, a := range f.Actions {
		if a.SendMessage == nil {
			continue
		}
		chID := a.SendMessage.ChannelID
		if isTriggerChannelTemplate(chID) {
			continue
		}
		// triggerChannelID is empty for triggers that are not tied to a channel
		// (channel_created, user_joined_team). In that case only template
		// expressions are valid — fail early with a clear message.
		if triggerChannelID == "" {
			return fmt.Errorf("action %d: send_message channel_id must use a template expression (e.g. \"{{.Trigger.Channel.Id}}\" or \"{{.Trigger.Team.DefaultChannelId}}\") for this trigger type", i)
		}
		if chID == triggerChannelID {
			continue
		}
		return fmt.Errorf("action %d: send_message channel_id must reference the triggering channel (use %q or the template expression \"{{.Trigger.Channel.Id}}\")", i, triggerChannelID)
	}
	return nil
}

// isTriggerChannelTemplate returns true if s is a Go template expression that
// resolves to a channel ID sourced from the trigger. The whole value must be a
// single template expression (leading/trailing whitespace aside) and must
// reference one of the allowlisted channel-bearing fields:
//
//   - {{.Trigger.Channel.Id}}            — the triggering channel
//   - {{.Trigger.Team.DefaultChannelId}} — user_joined_team default channel
//   - {{.Trigger.Post.ChannelId}}        — parent channel of a triggering post
//
// Templates referencing other fields (e.g. {{.Trigger.User.Id}}) or step
// outputs (e.g. {{.Steps.<id>.ChannelID}}) are rejected at create time. Step
// output chaining is not currently supported for send_message channel IDs.
//
// This is a UX guardrail, not a security boundary — CheckFlowPermissions is
// the actual authorization layer for literal channel IDs. Templates are
// intentionally excluded from permission checks because their values aren't
// known until runtime.
func isTriggerChannelTemplate(s string) bool {
	trimmed := strings.TrimSpace(s)
	if !strings.HasPrefix(trimmed, "{{") || !strings.HasSuffix(trimmed, "}}") {
		return false
	}
	allowed := []string{
		".Trigger.Channel.Id",
		".Trigger.Team.DefaultChannelId",
		".Trigger.Post.ChannelId",
	}
	for _, a := range allowed {
		if strings.Contains(trimmed, a) {
			return true
		}
	}
	return false
}

// disallowedTools lists tool names that may not appear in allowed_tools
// because they would let an automation post messages outside the flow's
// controlled output path.
var disallowedTools = map[string]struct{}{
	"create_post":   {},
	"dm":            {},
	"group_message": {},
}

// ValidateActions validates a list of actions.
// At least one action is required. Each action must have a unique, non-empty ID
// matching the slug pattern (lowercase alphanumeric + hyphens) and exactly one
// action config set. For ai_prompt actions, allowed_tools entries are checked
// against a blacklist of disallowed tool names.
func ValidateActions(actions []Action) error {
	if len(actions) == 0 {
		return fmt.Errorf("at least one action is required")
	}
	seen := make(map[string]struct{}, len(actions))
	for i, a := range actions {
		if a.ID == "" {
			return fmt.Errorf("action %d: id is required", i)
		}
		if !actionIDPattern.MatchString(a.ID) {
			return fmt.Errorf("action %d: id %q is invalid (must be lowercase alphanumeric with hyphens, e.g. \"send-greeting\")", i, a.ID)
		}
		if _, ok := seen[a.ID]; ok {
			return fmt.Errorf("action %d: duplicate id %q", i, a.ID)
		}
		seen[a.ID] = struct{}{}
		configCount := 0
		if a.SendMessage != nil {
			configCount++
		}
		if a.AIPrompt != nil {
			configCount++
		}
		if configCount == 0 {
			return fmt.Errorf("action %d: exactly one action config must be set", i)
		}
		if configCount > 1 {
			return fmt.Errorf("action %d: exactly one action config must be set, got %d", i, configCount)
		}
		if a.AIPrompt != nil {
			for _, rawTool := range a.AIPrompt.AllowedTools {
				tool := strings.ToLower(strings.TrimSpace(rawTool))
				if tool == "" {
					continue
				}
				if _, blocked := disallowedTools[tool]; blocked {
					return fmt.Errorf("action %d: tool %q is not allowed in automations", i, tool)
				}
			}
		}
	}
	return nil
}
