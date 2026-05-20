package trigger_test

import (
	"testing"

	mmmodel "github.com/mattermost/mattermost/server/public/model"
	"github.com/stretchr/testify/assert"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/automation/trigger"
	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

func TestSendMessageBotUserIDs(t *testing.T) {
	t.Run("empty actions", func(t *testing.T) {
		assert.Empty(t, trigger.SendMessageBotUserIDs(&model.Automation{}, "default-bot"))
	})

	t.Run("explicit as_bot_id", func(t *testing.T) {
		a := &model.Automation{
			Actions: []model.Action{{
				ID: "a1",
				SendMessage: &model.SendMessageActionConfig{
					ChannelID: "ch1",
					AsBotID:   "custom-bot",
					Body:      "hi",
				},
			}},
		}
		assert.Equal(t, []string{"custom-bot"}, trigger.SendMessageBotUserIDs(a, "default-bot"))
	})

	t.Run("missing as_bot_id uses default", func(t *testing.T) {
		a := &model.Automation{
			Actions: []model.Action{{
				ID:          "a1",
				SendMessage: &model.SendMessageActionConfig{ChannelID: "ch1", Body: "hi"},
			}},
		}
		assert.Equal(t, []string{"default-bot"}, trigger.SendMessageBotUserIDs(a, "default-bot"))
	})

	t.Run("dedupes multiple send_message actions", func(t *testing.T) {
		a := &model.Automation{
			Actions: []model.Action{
				{ID: "a1", SendMessage: &model.SendMessageActionConfig{ChannelID: "ch1", AsBotID: "bot-a", Body: "1"}},
				{ID: "a2", SendMessage: &model.SendMessageActionConfig{ChannelID: "ch1", AsBotID: "bot-a", Body: "2"}},
				{ID: "a3", SendMessage: &model.SendMessageActionConfig{ChannelID: "ch1", AsBotID: "bot-b", Body: "3"}},
			},
		}
		assert.ElementsMatch(t, []string{"bot-a", "bot-b"}, trigger.SendMessageBotUserIDs(a, "default-bot"))
	})
}

func TestIsSendMessageThreadLoopPost(t *testing.T) {
	botIDs := []string{"custom-bot", "default-bot"}

	t.Run("top-level post from bot is not a loop", func(t *testing.T) {
		post := &mmmodel.Post{UserId: "custom-bot", RootId: ""}
		assert.False(t, trigger.IsSendMessageThreadLoopPost(post, botIDs))
	})

	t.Run("thread reply from listed bot is a loop", func(t *testing.T) {
		post := &mmmodel.Post{UserId: "custom-bot", RootId: "root1"}
		assert.True(t, trigger.IsSendMessageThreadLoopPost(post, botIDs))
	})

	t.Run("thread reply from other user is not a loop", func(t *testing.T) {
		post := &mmmodel.Post{UserId: "human", RootId: "root1"}
		assert.False(t, trigger.IsSendMessageThreadLoopPost(post, botIDs))
	})
}
