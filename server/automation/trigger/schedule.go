package trigger

import (
	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

// ScheduleTrigger is registered in the registry so the trigger type is
// recognized during validation. Schedule triggers fire via the
// ScheduleManager, not by matching events.
type ScheduleTrigger struct{}

func (t *ScheduleTrigger) Type() string { return "schedule" }

// Matches always returns false — schedule triggers are time-based and
// never match incoming events.
func (t *ScheduleTrigger) Matches(_ *model.Trigger, _ *model.Event) bool {
	return false
}
