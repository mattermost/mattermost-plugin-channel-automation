package trigger

import (
	mmmodel "github.com/mattermost/mattermost/server/public/model"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

// IsSendMessageLoopPost reports whether post is authored by a bot that would post
// via the automation's send_message actions (explicit as_bot_id or defaultBotUserID
// when as_bot_id is omitted). Applies to root posts and thread replies.
//
// Loop detection does not use the ai_generated_by post prop: that marker is only
// set on MCP create_post output, which automations cannot invoke, so it cannot
// indicate a self-trigger from this plugin.
func IsSendMessageLoopPost(post *mmmodel.Post, automation *model.Automation, defaultBotUserID string) bool {
	if post == nil || automation == nil {
		return false
	}
	for _, act := range automation.Actions {
		if act.SendMessage == nil {
			continue
		}
		botID := act.SendMessage.AsBotID
		if botID == "" {
			botID = defaultBotUserID
		}
		if botID != "" && botID == post.UserId {
			return true
		}
	}
	return false
}
