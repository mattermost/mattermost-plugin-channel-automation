package hooks

import (
	"fmt"
	"sort"
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
	if gr == nil || len(gr.Channels) == 0 {
		return ""
	}
	ids := make([]string, 0, len(gr.Channels))
	for _, c := range gr.Channels {
		ids = append(ids, c.ChannelID)
	}
	return formatIDList(ids)
}

// formatAllowedTeams renders the team IDs in an allowed-teams set as a
// comma-separated list, capped at maxAllowedChannelsInError. Returns an
// empty string when the set is empty so callers can omit the suffix.
func formatAllowedTeams(allowedTeams map[string]struct{}) string {
	if len(allowedTeams) == 0 {
		return ""
	}
	ids := make([]string, 0, len(allowedTeams))
	for id := range allowedTeams {
		ids = append(ids, id)
	}
	// Stable order for deterministic error messages.
	sort.Strings(ids)
	return formatIDList(ids)
}

func formatIDList(ids []string) string {
	total := len(ids)
	truncated := false
	if total > maxAllowedChannelsInError {
		ids = ids[:maxAllowedChannelsInError]
		truncated = true
	}
	out := strings.Join(ids, ", ")
	if truncated {
		out += fmt.Sprintf(" (+%d more)", total-maxAllowedChannelsInError)
	}
	return out
}
