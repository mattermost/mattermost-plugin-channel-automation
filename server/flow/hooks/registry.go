package hooks

import (
	"fmt"
	"strings"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

// maxAllowedChannelsInError caps how many channel IDs are listed in a hook
// error message before truncation, keeping the payload returned to the LLM
// reasonable when guardrails permit a large set of channels.
const maxAllowedChannelsInError = 10

func channelAllowed(allowed map[string]struct{}, channelID string) bool {
	if channelID == "" {
		return false
	}
	_, ok := allowed[channelID]
	return ok
}

// formatAllowedChannels renders the guardrail's allowed channel IDs as a
// comma-separated list, capped at maxAllowedChannelsInError. Returns an empty
// string when no channel IDs are configured so callers can omit the suffix.
func formatAllowedChannels(gr *model.Guardrails) string {
	if gr == nil || len(gr.ChannelIDs) == 0 {
		return ""
	}
	ids := gr.ChannelIDs
	truncated := false
	if len(ids) > maxAllowedChannelsInError {
		ids = ids[:maxAllowedChannelsInError]
		truncated = true
	}
	out := strings.Join(ids, ", ")
	if truncated {
		out += fmt.Sprintf(" (+%d more)", len(gr.ChannelIDs)-maxAllowedChannelsInError)
	}
	return out
}
