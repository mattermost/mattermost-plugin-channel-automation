package model

import (
	"fmt"
	"time"
)

const minScheduleInterval = 5 * time.Minute

// ValidateTrigger validates a trigger configuration based on its type.
func ValidateTrigger(t *Trigger) error {
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
		if t.Schedule.StartAt < 0 {
			return fmt.Errorf("schedule trigger start_at must not be negative")
		}
	default:
		return fmt.Errorf("unknown trigger type: %s", t.Type())
	}
	return nil
}
