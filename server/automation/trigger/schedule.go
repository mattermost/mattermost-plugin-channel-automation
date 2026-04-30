package trigger

import (
	"fmt"
	"time"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

const minScheduleInterval = 1 * time.Hour

// ScheduleTrigger is registered so the trigger type is recognized during
// validation and dispatch. Schedule triggers fire via the ScheduleManager,
// not by matching events: Matches and CandidateAutomationIDs are no-ops on the
// event path, and BuildTriggerData errors defensively if invoked from it
// (preventing accidental double-firing via dispatcher + ScheduleManager).
type ScheduleTrigger struct{}

func (t *ScheduleTrigger) Type() string { return model.TriggerTypeSchedule }

// Matches always returns false — schedule triggers are time-based and
// never match incoming events.
func (t *ScheduleTrigger) Matches(_ *model.Trigger, _ *model.Event) bool {
	return false
}

func (t *ScheduleTrigger) Validate(trigger *model.Trigger, existing *model.Trigger) error {
	if trigger.Schedule == nil {
		return fmt.Errorf("schedule trigger config is missing")
	}
	if trigger.Schedule.ChannelID == "" {
		return fmt.Errorf("schedule trigger requires channel_id")
	}
	if trigger.Schedule.Interval == "" {
		return fmt.Errorf("schedule trigger requires interval")
	}
	d, err := time.ParseDuration(trigger.Schedule.Interval)
	if err != nil {
		return fmt.Errorf("schedule trigger has invalid interval: %w", err)
	}
	if d < minScheduleInterval {
		return fmt.Errorf("schedule trigger interval must be at least %dh", int(minScheduleInterval.Hours()))
	}

	// StartAt is only validated when it changed from the existing value, so
	// that a stored past timestamp can be carried through an unrelated update.
	startAtChanged := existing == nil || existing.Schedule == nil ||
		model.TimestampToTime(existing.Schedule.StartAt).Truncate(time.Minute) != model.TimestampToTime(trigger.Schedule.StartAt).Truncate(time.Minute)
	if startAtChanged && trigger.Schedule.StartAt != 0 && model.TimestampToTime(trigger.Schedule.StartAt).Before(time.Now().UTC()) {
		return fmt.Errorf("schedule trigger start_at must be a future UTC timestamp")
	}
	return nil
}

// CandidateAutomationIDs returns nil — schedule flows are not resolved via the
// event-matching path.
func (t *ScheduleTrigger) CandidateAutomationIDs(_ model.Store, _ *model.Event) ([]string, error) {
	return nil, nil
}

// BuildTriggerData is not used for schedule triggers. ScheduleManager builds
// the TriggerData directly at fire time. Returning an error here prevents
// accidental use via the event dispatch path.
func (t *ScheduleTrigger) BuildTriggerData(_ model.TriggerAPI, _ *model.Event) (model.TriggerData, error) {
	return model.TriggerData{}, fmt.Errorf("schedule trigger does not build data from events")
}
