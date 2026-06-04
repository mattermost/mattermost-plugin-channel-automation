package trigger_test

import (
	"testing"

	mmmodel "github.com/mattermost/mattermost/server/public/model"
	"github.com/stretchr/testify/assert"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/automation/trigger"
	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

func TestIsSendMessageLoopPost(t *testing.T) {
	t.Run("nil post", func(t *testing.T) {
		a := &model.Automation{
			Actions: []model.Action{{
				ID:          "a1",
				SendMessage: &model.SendMessageActionConfig{ChannelID: "ch1", AsBotID: "custom-bot", Body: "hi"},
			}},
		}
		assert.False(t, trigger.IsSendMessageLoopPost(nil, a, "default-bot"))
	})

	t.Run("nil automation", func(t *testing.T) {
		post := &mmmodel.Post{UserId: "custom-bot"}
		assert.False(t, trigger.IsSendMessageLoopPost(post, nil, "default-bot"))
	})

	t.Run("no send_message actions", func(t *testing.T) {
		post := &mmmodel.Post{UserId: "custom-bot"}
		assert.False(t, trigger.IsSendMessageLoopPost(post, &model.Automation{}, "default-bot"))
	})

	t.Run("explicit as_bot_id root post is a loop", func(t *testing.T) {
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
		post := &mmmodel.Post{UserId: "custom-bot", RootId: ""}
		assert.True(t, trigger.IsSendMessageLoopPost(post, a, "default-bot"))
	})

	t.Run("explicit as_bot_id thread reply is a loop", func(t *testing.T) {
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
		post := &mmmodel.Post{UserId: "custom-bot", RootId: "root1"}
		assert.True(t, trigger.IsSendMessageLoopPost(post, a, "default-bot"))
	})

	t.Run("empty as_bot_id uses default bot", func(t *testing.T) {
		a := &model.Automation{
			Actions: []model.Action{{
				ID:          "a1",
				SendMessage: &model.SendMessageActionConfig{ChannelID: "ch1", Body: "hi"},
			}},
		}
		post := &mmmodel.Post{UserId: "default-bot"}
		assert.True(t, trigger.IsSendMessageLoopPost(post, a, "default-bot"))
	})

	t.Run("post from other user is not a loop", func(t *testing.T) {
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
		post := &mmmodel.Post{UserId: "human", RootId: "root1"}
		assert.False(t, trigger.IsSendMessageLoopPost(post, a, "default-bot"))
	})

	t.Run("matches any send_message bot in automation", func(t *testing.T) {
		a := &model.Automation{
			Actions: []model.Action{
				{ID: "a1", SendMessage: &model.SendMessageActionConfig{ChannelID: "ch1", AsBotID: "bot-a", Body: "1"}},
				{ID: "a2", SendMessage: &model.SendMessageActionConfig{ChannelID: "ch1", AsBotID: "bot-b", Body: "2"}},
			},
		}
		post := &mmmodel.Post{UserId: "bot-b"}
		assert.True(t, trigger.IsSendMessageLoopPost(post, a, "default-bot"))
	})
}
