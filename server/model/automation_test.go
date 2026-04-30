package model

import (
	"encoding/json"
	"testing"

	mmmodel "github.com/mattermost/mattermost/server/public/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollectChannelIDs_LiteralChannels(t *testing.T) {
	a := &Automation{
		Trigger: Trigger{MessagePosted: &MessagePostedConfig{ChannelID: "ch1"}},
		Actions: []Action{
			{SendMessage: &SendMessageActionConfig{ChannelID: "ch2"}},
			{SendMessage: &SendMessageActionConfig{ChannelID: "ch3"}},
		},
	}

	ids := CollectChannelIDs(a)
	assert.Equal(t, []string{"ch1", "ch2", "ch3"}, ids)
}

func TestCollectChannelIDs_TemplatedChannelSkipped(t *testing.T) {
	a := &Automation{
		Trigger: Trigger{MessagePosted: &MessagePostedConfig{ChannelID: "ch1"}},
		Actions: []Action{
			{SendMessage: &SendMessageActionConfig{ChannelID: "{{.Trigger.Channel.Id}}"}},
		},
	}

	ids := CollectChannelIDs(a)
	assert.Equal(t, []string{"ch1"}, ids)
}

func TestCollectChannelIDs_DuplicatesRemoved(t *testing.T) {
	a := &Automation{
		Trigger: Trigger{MessagePosted: &MessagePostedConfig{ChannelID: "ch1"}},
		Actions: []Action{
			{SendMessage: &SendMessageActionConfig{ChannelID: "ch1"}},
			{SendMessage: &SendMessageActionConfig{ChannelID: "ch2"}},
			{SendMessage: &SendMessageActionConfig{ChannelID: "ch2"}},
		},
	}

	ids := CollectChannelIDs(a)
	assert.Equal(t, []string{"ch1", "ch2"}, ids)
}

func TestCollectChannelIDs_ScheduleTrigger(t *testing.T) {
	a := &Automation{
		Trigger: Trigger{Schedule: &ScheduleConfig{ChannelID: "ch1", Interval: "1h"}},
		Actions: []Action{
			{SendMessage: &SendMessageActionConfig{ChannelID: "ch2"}},
		},
	}

	ids := CollectChannelIDs(a)
	assert.Equal(t, []string{"ch1", "ch2"}, ids)
}

func TestCollectChannelIDs_ScheduleTriggerDuplicateWithAction(t *testing.T) {
	a := &Automation{
		Trigger: Trigger{Schedule: &ScheduleConfig{ChannelID: "ch1", Interval: "1h"}},
		Actions: []Action{
			{SendMessage: &SendMessageActionConfig{ChannelID: "ch1"}},
		},
	}

	ids := CollectChannelIDs(a)
	assert.Equal(t, []string{"ch1"}, ids)
}

func TestCollectChannelIDs_MembershipChangedTrigger(t *testing.T) {
	a := &Automation{
		Trigger: Trigger{MembershipChanged: &MembershipChangedConfig{ChannelID: "ch1"}},
		Actions: []Action{
			{SendMessage: &SendMessageActionConfig{ChannelID: "ch2"}},
		},
	}

	ids := CollectChannelIDs(a)
	assert.Equal(t, []string{"ch1", "ch2"}, ids)
}

func TestCollectChannelIDs_MembershipChangedDuplicateWithAction(t *testing.T) {
	a := &Automation{
		Trigger: Trigger{MembershipChanged: &MembershipChangedConfig{ChannelID: "ch1"}},
		Actions: []Action{
			{SendMessage: &SendMessageActionConfig{ChannelID: "ch1"}},
		},
	}

	ids := CollectChannelIDs(a)
	assert.Equal(t, []string{"ch1"}, ids)
}

func TestCollectChannelIDs_NoChannels(t *testing.T) {
	a := &Automation{
		Trigger: Trigger{Schedule: &ScheduleConfig{Interval: "1h"}},
		Actions: []Action{
			{AIPrompt: &AIPromptActionConfig{Prompt: "test"}},
		},
	}

	ids := CollectChannelIDs(a)
	assert.Empty(t, ids)
}

func TestCollectChannelIDs_ChannelCreatedTrigger(t *testing.T) {
	a := &Automation{
		Trigger: Trigger{ChannelCreated: &ChannelCreatedConfig{TeamID: "team1"}},
		Actions: []Action{
			{SendMessage: &SendMessageActionConfig{ChannelID: "{{.Trigger.Channel.Id}}", Body: "hello"}},
		},
	}

	ids := CollectChannelIDs(a)
	assert.Empty(t, ids, "channel_created with templated action channels should return no concrete IDs")
}

func TestCollectChannelIDs_AIPromptGuardrailsExcluded(t *testing.T) {
	// Guardrail channels are an LLM read-only allowlist enforced at the hook
	// layer, not channels the automation acts on. They must not appear in
	// CollectChannelIDs, which feeds channel-admin permission checks.
	ch1 := mmmodel.NewId()
	ch2 := mmmodel.NewId()
	a := &Automation{
		Trigger: Trigger{MessagePosted: &MessagePostedConfig{ChannelID: ch1}},
		Actions: []Action{
			{
				ID: "ai1",
				AIPrompt: &AIPromptActionConfig{
					Prompt: "x", ProviderType: "agent", ProviderID: "bot",
					Guardrails: &Guardrails{Channels: []GuardrailChannel{{ChannelID: ch2}}},
				},
			},
		},
	}
	ids := CollectChannelIDs(a)
	assert.Equal(t, []string{ch1}, ids)
}

func TestCollectChannelIDs_ChannelCreatedWithLiteralAction(t *testing.T) {
	a := &Automation{
		Trigger: Trigger{ChannelCreated: &ChannelCreatedConfig{TeamID: "team1"}},
		Actions: []Action{
			{SendMessage: &SendMessageActionConfig{ChannelID: "ch-notify", Body: "hello"}},
		},
	}

	ids := CollectChannelIDs(a)
	assert.Equal(t, []string{"ch-notify"}, ids)
}

func TestAutomationJSON_AIPrompt_AllowedToolsStringArray(t *testing.T) {
	const raw = `{
		"id": "f1",
		"name": "n",
		"enabled": true,
		"trigger": {"schedule": {"channel_id": "c1", "interval": "1h"}},
		"actions": [{
			"id": "a1",
			"ai_prompt": {
				"prompt": "p",
				"provider_type": "agent",
				"provider_id": "bot1",
				"allowed_tools": ["search", "create_post"]
			}
		}],
		"created_at": 0,
		"updated_at": 0,
		"created_by": "u1"
	}`
	var a Automation
	err := json.Unmarshal([]byte(raw), &a)
	require.NoError(t, err)
	require.Len(t, a.Actions, 1)
	require.NotNil(t, a.Actions[0].AIPrompt)
	assert.Equal(t, []string{"search", "create_post"}, a.Actions[0].AIPrompt.AllowedTools)
}

func TestCollectTeamIDs_LiteralTeamID(t *testing.T) {
	a := &Automation{
		Trigger: Trigger{UserJoinedTeam: &UserJoinedTeamConfig{TeamID: "team1"}},
	}
	ids := CollectTeamIDs(a)
	assert.Equal(t, []string{"team1"}, ids)
}

func TestCollectTeamIDs_EmptyTeamID(t *testing.T) {
	a := &Automation{
		Trigger: Trigger{UserJoinedTeam: &UserJoinedTeamConfig{TeamID: ""}},
	}
	ids := CollectTeamIDs(a)
	assert.Nil(t, ids)
}

func TestCollectTeamIDs_TemplatedTeamID(t *testing.T) {
	a := &Automation{
		Trigger: Trigger{UserJoinedTeam: &UserJoinedTeamConfig{TeamID: "{{.SomeVar}}"}},
	}
	ids := CollectTeamIDs(a)
	assert.Nil(t, ids)
}

func TestCollectTeamIDs_NonTeamTrigger(t *testing.T) {
	a := &Automation{
		Trigger: Trigger{MessagePosted: &MessagePostedConfig{ChannelID: "ch1"}},
	}
	ids := CollectTeamIDs(a)
	assert.Nil(t, ids)
}
