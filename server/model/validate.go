package model

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

const minScheduleInterval = 1 * time.Hour

var actionIDPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)

// ValidateTrigger validates a trigger configuration based on its type.
// Exactly one trigger type must be set. An optional existing trigger can be
// passed for update scenarios; when provided, start_at is only validated if
// it changed from the existing value.
func ValidateTrigger(t *Trigger, existing *Trigger) error {
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
	if count == 0 {
		return fmt.Errorf("exactly one trigger type must be set")
	}
	if count > 1 {
		return fmt.Errorf("exactly one trigger type must be set, got %d", count)
	}

	switch {
	case t.MessagePosted != nil:
		if t.MessagePosted.ChannelID == "" {
			return fmt.Errorf("message_posted trigger requires channel_id")
		}
	case t.Schedule != nil:
		if t.Schedule.ChannelID == "" {
			return fmt.Errorf("schedule trigger requires channel_id")
		}
		if t.Schedule.Interval == "" {
			return fmt.Errorf("schedule trigger requires interval")
		}
		d, err := time.ParseDuration(t.Schedule.Interval)
		if err != nil {
			return fmt.Errorf("schedule trigger has invalid interval: %w", err)
		}
		if d < minScheduleInterval {
			return fmt.Errorf("schedule trigger interval must be at least %dh", int(minScheduleInterval.Hours()))
		}
		startAtChanged := existing == nil || existing.Schedule == nil ||
			time.UnixMilli(existing.Schedule.StartAt).Truncate(time.Minute) != time.UnixMilli(t.Schedule.StartAt).Truncate(time.Minute)
		if startAtChanged && t.Schedule.StartAt != 0 && time.UnixMilli(t.Schedule.StartAt).Before(time.Now().UTC()) {
			return fmt.Errorf("schedule trigger start_at must be a future UTC timestamp")
		}
	case t.MembershipChanged != nil:
		if t.MembershipChanged.ChannelID == "" {
			return fmt.Errorf("membership_changed trigger requires channel_id")
		}
		if a := t.MembershipChanged.Action; a != "" && a != "joined" && a != "left" {
			return fmt.Errorf("membership_changed trigger action must be \"joined\", \"left\", or empty (both)")
		}
	case t.ChannelCreated != nil:
		if t.ChannelCreated.TeamID == "" {
			return fmt.Errorf("channel_created trigger requires team_id")
		}
	default:
		return fmt.Errorf("unknown trigger type: %s", t.Type())
	}
	return nil
}

// ValidateSendMessageChannel checks that every send_message action in the automation
// targets the same channel that the trigger is bound to. For triggers with a
// channel_id (message_posted, schedule, membership_changed), the action channel
// must be either the literal trigger channel ID or a template containing
// ".Trigger.Channel.Id". For channel_created (no trigger channel ID), only the
// template form is accepted.
func ValidateSendMessageChannel(a *Automation) error {
	triggerChannelID := a.TriggerChannelID()

	for i, act := range a.Actions {
		if act.SendMessage == nil {
			continue
		}
		chID := act.SendMessage.ChannelID
		if isTriggerChannelTemplate(chID) {
			continue
		}
		// triggerChannelID is empty for triggers that are not tied to a channel (channel_created).
		// In that case only the template form is valid — fail early with a clear message.
		if triggerChannelID == "" {
			return fmt.Errorf("action %d: send_message channel_id must use the template expression \"{{.Trigger.Channel.Id}}\" for this trigger type", i)
		}
		if chID == triggerChannelID {
			continue
		}
		return fmt.Errorf("action %d: send_message channel_id must reference the triggering channel (use %q or the template expression \"{{.Trigger.Channel.Id}}\")", i, triggerChannelID)
	}
	return nil
}

// isTriggerChannelTemplate returns true if s is a Go template expression that
// references .Trigger.Channel.Id, with any amount of whitespace around it.
func isTriggerChannelTemplate(s string) bool {
	return strings.Contains(s, "{{") && strings.Contains(s, ".Trigger.Channel.Id")
}

// ValidateActions validates a list of actions.
// At least one action is required. Each action must have a unique, non-empty ID
// matching the slug pattern (lowercase alphanumeric + hyphens) and exactly one
// action config set.
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
	}
	return nil
}
