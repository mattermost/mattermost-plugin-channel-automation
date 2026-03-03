package trigger_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/flow/trigger"
	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

func TestScheduleTrigger_Type(t *testing.T) {
	tr := &trigger.ScheduleTrigger{}
	assert.Equal(t, "schedule", tr.Type())
}

func TestScheduleTrigger_Matches_AlwaysFalse(t *testing.T) {
	tr := &trigger.ScheduleTrigger{}
	trig := &model.Trigger{Type: "schedule", Interval: "5m"}
	event := &model.Event{Type: "schedule"}

	assert.False(t, tr.Matches(trig, event))
}
