package automation

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/automation/action"
	"github.com/mattermost/mattermost-plugin-channel-automation/server/automation/trigger"
)

func TestRegistry_RegisterAndGetTrigger(t *testing.T) {
	r := NewRegistry()
	r.RegisterTrigger(&trigger.MessagePostedTrigger{})

	h, ok := r.GetTrigger("message_posted")
	assert.True(t, ok)
	assert.Equal(t, "message_posted", h.Type())
}

func TestRegistry_RegisterAndGetAction(t *testing.T) {
	r := NewRegistry()
	r.RegisterAction(action.NewSendMessageAction(nil, "bot"))

	h, ok := r.GetAction("send_message")
	assert.True(t, ok)
	assert.Equal(t, "send_message", h.Type())
}

func TestRegistry_GetTrigger_Unknown(t *testing.T) {
	r := NewRegistry()

	_, ok := r.GetTrigger("unknown")
	assert.False(t, ok)
}

func TestRegistry_GetAction_Unknown(t *testing.T) {
	r := NewRegistry()

	_, ok := r.GetAction("unknown")
	assert.False(t, ok)
}
