package trigger_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/automation/trigger"
	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

func TestScheduleTrigger_Type(t *testing.T) {
	tr := &trigger.ScheduleTrigger{}
	assert.Equal(t, model.TriggerTypeSchedule, tr.Type())
}

func TestScheduleTrigger_Matches_AlwaysFalse(t *testing.T) {
	tr := &trigger.ScheduleTrigger{}
	trig := &model.Trigger{Schedule: &model.ScheduleConfig{ChannelID: "ch1", Interval: "1h"}}
	event := &model.Event{Type: model.TriggerTypeSchedule}

	assert.False(t, tr.Matches(trig, event))
}

func TestScheduleTrigger_Validate(t *testing.T) {
	tr := &trigger.ScheduleTrigger{}

	t.Run("valid minimal", func(t *testing.T) {
		require.NoError(t, tr.Validate(&model.Trigger{Schedule: &model.ScheduleConfig{ChannelID: "ch1", Interval: "1h"}}, nil))
	})

	t.Run("valid with future start_at", func(t *testing.T) {
		require.NoError(t, tr.Validate(&model.Trigger{Schedule: &model.ScheduleConfig{ChannelID: "ch1", Interval: "1h", StartAt: time.Now().Add(time.Hour).UnixMilli()}}, nil))
	})

	t.Run("missing config", func(t *testing.T) {
		require.Error(t, tr.Validate(&model.Trigger{}, nil))
	})

	t.Run("missing channel_id", func(t *testing.T) {
		err := tr.Validate(&model.Trigger{Schedule: &model.ScheduleConfig{Interval: "1h"}}, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "channel_id")
	})

	t.Run("missing interval", func(t *testing.T) {
		err := tr.Validate(&model.Trigger{Schedule: &model.ScheduleConfig{ChannelID: "ch1"}}, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "interval")
	})

	t.Run("unparseable interval", func(t *testing.T) {
		err := tr.Validate(&model.Trigger{Schedule: &model.ScheduleConfig{ChannelID: "ch1", Interval: "not-a-duration"}}, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid interval")
	})

	t.Run("interval too small", func(t *testing.T) {
		err := tr.Validate(&model.Trigger{Schedule: &model.ScheduleConfig{ChannelID: "ch1", Interval: "30m"}}, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "at least 1h")
	})

	t.Run("start_at in the past is rejected", func(t *testing.T) {
		err := tr.Validate(&model.Trigger{Schedule: &model.ScheduleConfig{ChannelID: "ch1", Interval: "2h", StartAt: time.Now().Add(-time.Hour).UnixMilli()}}, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "start_at")
	})

	t.Run("update with unchanged past start_at is valid", func(t *testing.T) {
		pastStartAt := time.Now().Add(-time.Hour).UnixMilli()
		existing := &model.Trigger{Schedule: &model.ScheduleConfig{ChannelID: "ch1", Interval: "2h", StartAt: pastStartAt}}
		updated := &model.Trigger{Schedule: &model.ScheduleConfig{ChannelID: "ch1", Interval: "2h", StartAt: pastStartAt}}
		require.NoError(t, tr.Validate(updated, existing))
	})

	t.Run("update with round-tripped past start_at is valid", func(t *testing.T) {
		pastStartAt := time.Now().Add(-time.Hour).UnixMilli()
		truncated := time.UnixMilli(pastStartAt).Truncate(time.Minute).UnixMilli()
		existing := &model.Trigger{Schedule: &model.ScheduleConfig{ChannelID: "ch1", Interval: "2h", StartAt: pastStartAt}}
		updated := &model.Trigger{Schedule: &model.ScheduleConfig{ChannelID: "ch1", Interval: "2h", StartAt: truncated}}
		require.NoError(t, tr.Validate(updated, existing))
	})

	t.Run("update with new past start_at is rejected", func(t *testing.T) {
		existing := &model.Trigger{Schedule: &model.ScheduleConfig{ChannelID: "ch1", Interval: "2h", StartAt: time.Now().Add(-2 * time.Hour).UnixMilli()}}
		updated := &model.Trigger{Schedule: &model.ScheduleConfig{ChannelID: "ch1", Interval: "2h", StartAt: time.Now().Add(-time.Hour).UnixMilli()}}
		err := tr.Validate(updated, existing)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "start_at")
	})
}

func TestScheduleTrigger_CandidateAutomationIDs_Nil(t *testing.T) {
	tr := &trigger.ScheduleTrigger{}
	ids, err := tr.CandidateAutomationIDs(&stubStore{}, &model.Event{Type: model.TriggerTypeSchedule})
	require.NoError(t, err)
	assert.Nil(t, ids)
}

func TestScheduleTrigger_BuildTriggerData_Errors(t *testing.T) {
	tr := &trigger.ScheduleTrigger{}
	td, err := tr.BuildTriggerData(newFakeTriggerAPI(), &model.Event{Type: model.TriggerTypeSchedule})
	require.Error(t, err, "ScheduleTrigger must error on event-path BuildTriggerData to prevent double firing via dispatcher and ScheduleManager")
	assert.Contains(t, err.Error(), "schedule trigger does not build data from events")
	assert.Equal(t, model.TriggerData{}, td)
}
