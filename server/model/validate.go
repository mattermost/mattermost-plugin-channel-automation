package model

import (
	"fmt"
	"regexp"
	"time"
)

const minScheduleInterval = 5 * time.Minute

var actionIDPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)

// ValidateTrigger validates a trigger configuration based on its type.
// An optional existing trigger can be passed for update scenarios; when
// provided, start_at is only validated if it changed from the existing value.
func ValidateTrigger(t *Trigger, existing *Trigger) error {
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
			return fmt.Errorf("schedule trigger interval must be at least %s", minScheduleInterval)
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
		// No fields to validate — fires on any new public channel.
	default:
		return fmt.Errorf("unknown trigger type: %s", t.Type())
	}
	return nil
}

// ValidateActions validates a list of actions.
// Each action must have a unique, non-empty ID matching the slug pattern
// (lowercase alphanumeric + hyphens) and at least one action config set.
func ValidateActions(actions []Action) error {
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
		if a.Type() == "" {
			return fmt.Errorf("action %d: exactly one action config must be set", i)
		}
	}
	return nil
}
