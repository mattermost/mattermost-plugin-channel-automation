package hooks

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

// maxAllowedChannelsInError caps how many channel IDs are listed in a hook
// error message before truncation, keeping the payload returned to the LLM
// reasonable when guardrails permit a large set of channels.
const maxAllowedChannelsInError = 10

// BeforeFunc validates tool arguments before the resolver runs.
type BeforeFunc func(ctx HookCtx, args map[string]any) error

// AfterFunc filters or validates the tool output JSON after the resolver runs.
type AfterFunc func(ctx HookCtx, output json.RawMessage) (json.RawMessage, error)

type toolEntry struct {
	Before BeforeFunc
	After  AfterFunc
}

// mattermostMCPServerToolNames is the exhaustive set of tool names registered by the
// Mattermost agents plugin built-in MCP server when dev mode is off
// (github.com/mattermost/mattermost-plugin-agents/mcpserver/tools — ProvideTools in provider.go).
//
// Dev-only tools (gated by devMode in the agents plugin) are intentionally omitted:
//   - create_user
//   - create_post_as_user
//   - create_team
//   - add_user_to_team
//
// Production Mattermost MCP tools (20), by area:
//
// Posts:       read_post, create_post, dm, group_message
// Channels:    read_channel, create_channel, get_channel_info, get_channel_members, add_user_to_channel, get_user_channels
// Teams:       get_team_info, get_team_members
// Search:      search_posts, search_users
// Agents:      list_agents
// Automations: list_automations, get_automation_instructions, create_automation, update_automation, delete_automation
//
// Keep this map in sync when the agents plugin adds or removes MCP tools. Unit tests
// assert every guardrail-supported tool is listed here and that the guardrail registry
// only contains names from this set.
var mattermostMCPServerToolNames = map[string]struct{}{
	// Posts (mcpserver/tools/posts.go — getPostTools)
	"read_post":     {},
	"create_post":   {},
	"dm":            {},
	"group_message": {},
	// Channels (mcpserver/tools/channels.go — getChannelTools)
	"read_channel":        {},
	"create_channel":      {},
	"get_channel_info":    {},
	"get_channel_members": {},
	"add_user_to_channel": {},
	"get_user_channels":   {},
	// Teams (mcpserver/tools/teams.go — getTeamTools)
	"get_team_info":    {},
	"get_team_members": {},
	// Search (mcpserver/tools/search.go — getSearchTools)
	"search_posts": {},
	"search_users": {},
	// Agents (mcpserver/tools/agents.go — getAgentTools)
	"list_agents": {},
	// Automations (mcpserver/tools/automations.go — getAutomationTools)
	"list_automations":            {},
	"get_automation_instructions": {},
	"create_automation":           {},
	"update_automation":           {},
	"delete_automation":           {},
}

func isMattermostMCPServerTool(toolName string) bool {
	_, ok := mattermostMCPServerToolNames[toolName]
	return ok
}

// toolRegistry maps MCP tool names to explicit before/after handlers (fail-closed if missing).
var toolRegistry = map[string]toolEntry{
	"search_posts":        {Before: beforeSearchPosts, After: afterSearchPosts},
	"get_channel_info":    {Before: beforeGetChannelInfo, After: afterGetChannelInfo},
	"get_user_channels":   {Before: beforeGetUserChannels, After: afterGetUserChannels},
	"read_channel":        {Before: beforeReadChannel, After: afterReadChannel},
	"get_channel_members": {Before: beforeGetChannelMembers, After: afterGetChannelMembers},
	"read_post":           {Before: beforeReadPost, After: afterReadPost},
	"get_team_info":       {Before: beforeGetTeamInfo, After: afterGetTeamInfo},
	"get_team_members":    {Before: beforeGetTeamMembers, After: afterGetTeamMembers},
}

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
