package hooks

import "encoding/json"

// EmbeddedMattermostMCPOrigin identifies tools served by the Mattermost
// agents plugin's embedded MCP server. Mirrors mcp.EmbeddedClientKey from
// github.com/mattermost/mattermost-plugin-agents.
const EmbeddedMattermostMCPOrigin = "embedded://mattermost"

// BeforeFunc validates tool arguments before the resolver runs.
type BeforeFunc func(ctx HookCtx, args map[string]any) error

// AfterFunc filters or validates the tool output JSON after the resolver runs.
type AfterFunc func(ctx HookCtx, output json.RawMessage) (json.RawMessage, error)

// MattermostMCPTool describes a known tool exposed by the Mattermost embedded
// MCP server.
//
// Allowed indicates whether the tool may appear in an automation's
// allowed_tools. Tools that bypass the controlled output path
// (create_post, dm, group_message) are present here with Allowed=false so
// this catalog stays the single source of truth for what's known and what's
// permitted.
//
// Before/After are optional per-direction guardrail hooks. The ai_prompt
// action only registers the callback URL with the bridge for directions
// where a hook is set, so a tool with only Before (or only After) declared
// here is fine: the bridge simply doesn't invoke the missing direction.
// The HTTP handler still fails closed if it's called for a direction that
// has no implementation, as defense in depth.
type MattermostMCPTool struct {
	Allowed bool
	Before  BeforeFunc
	After   AfterFunc
}

// mattermostMCPServerTools is the exhaustive set of tools registered by the
// Mattermost agents plugin built-in MCP server when dev mode is off
// (github.com/mattermost/mattermost-plugin-agents/mcpserver/tools — ProvideTools
// in provider.go).
//
// Dev-only tools (gated by devMode in the agents plugin) are intentionally
// omitted:
//   - create_user
//   - create_post_as_user
//   - create_team
//   - add_user_to_team
//
// Production Mattermost MCP tools (20), by area:
//
// Posts:       read_post, create_post, dm, group_message
// Channels:    read_channel, create_channel, get_channel_info,
//
//	get_channel_members, add_user_to_channel, get_user_channels
//
// Teams:       get_team_info, get_team_members
// Search:      search_posts, search_users
// Agents:      list_agents
// Automations: list_automations, get_automation_instructions,
//
//	create_automation, update_automation, delete_automation
//
// Keep this map in sync when the agents plugin adds or removes MCP tools.
// Unit tests assert Before/After handlers are registered only for tools that
// also appear here with Allowed=true.
var mattermostMCPServerTools = map[string]MattermostMCPTool{
	// Posts (mcpserver/tools/posts.go — getPostTools)
	"read_post":     {Allowed: true, Before: beforeReadPost, After: afterReadPost},
	"create_post":   {Allowed: false},
	"dm":            {Allowed: false},
	"group_message": {Allowed: false},

	// Channels (mcpserver/tools/channels.go — getChannelTools)
	"read_channel":        {Allowed: true, Before: beforeReadChannel, After: afterReadChannel},
	"create_channel":      {Allowed: true},
	"get_channel_info":    {Allowed: true, Before: beforeGetChannelInfo, After: afterGetChannelInfo},
	"get_channel_members": {Allowed: true, Before: beforeGetChannelMembers, After: afterGetChannelMembers},
	"add_user_to_channel": {Allowed: true, Before: beforeAddUserToChannel},
	"get_user_channels":   {Allowed: true, Before: beforeGetUserChannels, After: afterGetUserChannels},

	// Teams (mcpserver/tools/teams.go — getTeamTools)
	"get_team_info":    {Allowed: true, Before: beforeGetTeamInfo, After: afterGetTeamInfo},
	"get_team_members": {Allowed: true, Before: beforeGetTeamMembers, After: afterGetTeamMembers},

	// Search (mcpserver/tools/search.go — getSearchTools)
	"search_posts": {Allowed: true, Before: beforeSearchPosts, After: afterSearchPosts},
	"search_users": {Allowed: true},

	// Agents (mcpserver/tools/agents.go — getAgentTools)
	"list_agents": {Allowed: true},

	// Automations (mcpserver/tools/automations.go — getAutomationTools)
	"list_automations":            {Allowed: false},
	"get_automation_instructions": {Allowed: false},
	"create_automation":           {Allowed: false},
	"update_automation":           {Allowed: false},
	"delete_automation":           {Allowed: false},
}

// LookupMattermostMCPTool returns the catalog entry for the given tool name.
// The second return value is false when the tool is not in the catalog.
func LookupMattermostMCPTool(name string) (MattermostMCPTool, bool) {
	entry, ok := mattermostMCPServerTools[name]
	return entry, ok
}

// IsAllowedMattermostMCPTool reports whether the given tool name is in the
// catalog (known) and whether automations may include it in allowed_tools
// (allowed). Tools known with Allowed=false are explicitly rejected.
func IsAllowedMattermostMCPTool(name string) (known, allowed bool) {
	entry, ok := mattermostMCPServerTools[name]
	if !ok {
		return false, false
	}
	return true, entry.Allowed
}

// MattermostMCPTools returns a copy of the catalog suitable for iteration in
// tests. Mutating the returned map does not affect the catalog.
func MattermostMCPTools() map[string]MattermostMCPTool {
	out := make(map[string]MattermostMCPTool, len(mattermostMCPServerTools))
	for k, v := range mattermostMCPServerTools {
		out[k] = v
	}
	return out
}
