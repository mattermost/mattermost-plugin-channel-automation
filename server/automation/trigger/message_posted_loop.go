package trigger

import (
	"slices"

	mmmodel "github.com/mattermost/mattermost/server/public/model"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

// SendMessageBotUserIDs returns distinct bot user IDs used by send_message actions.
// Empty AsBotID resolves to defaultBotUserID (the plugin default automation bot).
func SendMessageBotUserIDs(automation *model.Automation, defaultBotUserID string) []string {
	if automation == nil {
		return nil
	}
	seen := make(map[string]struct{})
	var ids []string
	for _, act := range automation.Actions {
		if act.SendMessage == nil {
			continue
		}
		botID := act.SendMessage.AsBotID
		if botID == "" {
			botID = defaultBotUserID
		}
		if botID == "" {
			continue
		}
		if _, ok := seen[botID]; ok {
			continue
		}
		seen[botID] = struct{}{}
		ids = append(ids, botID)
	}
	return ids
}

// IsSendMessageThreadLoopPost reports whether post is a thread reply authored by a
// bot that would post via the automation's send_message actions.
func IsSendMessageThreadLoopPost(post *mmmodel.Post, botUserIDs []string) bool {
	if post == nil || post.RootId == "" {
		return false
	}
	return slices.Contains(botUserIDs, post.UserId)
}
