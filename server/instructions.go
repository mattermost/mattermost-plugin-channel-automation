package main

import (
	"encoding/json"
	"net/http"
	"strings"
)

// channelAutomationInstructionsBase documents trigger types, actions, templates, and workflow
// for create_automation and update_automation (returned by GET /automation-instructions).
const channelAutomationInstructionsBase = `Channel automations are trigger-action workflows that fire when events occur.
Requires channel admin (or system admin) permission for the trigger channel. For the channel_created trigger, team admin permission on the trigger's team_id is required instead.

AGENT DISCOVERY: For an ai_prompt action with provider_type "agent", use the list_agents tool to discover bots.
Each agent's ID is a 26-character Mattermost user ID — use that value as provider_id in the ai_prompt config.

IMPORTANT WORKFLOW — ALWAYS CONFIRM BEFORE CREATING:
Before calling create_automation (or update_automation), you MUST present a plain-language summary to the user and get their
explicit confirmation. Even if the user provided all details, always present the full summary.

The summary must include:
1. TRIGGER: What event fires this automation and its scope.
2. AI TOOLS: Which tools the AI agent will have access to and what each one can do.
   - Without tools, the agent can only generate text from its built-in knowledge — it cannot
     read any Mattermost data or take any actions.
   - With tools, the agent inherits YOUR permissions — it can access anything you can access.
   Explain what each granted tool does so the user understands the access they are giving.
3. OUTPUT: Where the automation will post results — name the specific channel(s).

Format as a numbered list, then ask the user to confirm. Only call create_automation after
the user says yes.

If the user's request is missing details (trigger channel, output channel, which tools),
ask clarifying questions BEFORE presenting the summary.

ACTION SELECTION: For each step in the automation, choose the right action type:
- send_message / send_dm: for posting text to channels or users.
- ai_prompt with allowed_tools: for anything else — any step that needs to read data, modify state, or interact with Mattermost beyond posting text. Discover tools via the AI bridge GET .../agents/{id}/tools (or list_tools); each allowed_tools entry is the tool name string from discovery (e.g. "search_posts").
If a step cannot be accomplished with send_message or send_dm, it MUST be an ai_prompt action with the appropriate tools.

TOOL SUFFICIENCY CHECK (THIS IS VERY IMPORTANT): Before presenting the summary, think through the automation's task
step-by-step and verify the granted tools cover every step the agent will need to perform.
Ask: what data does the agent need to discover, read, or act on — and can it actually do
each of those things with only the tools listed? If any step requires a tool that isn't
included, add it to your recommendation and explain why it's needed.

TRIGGERS: Set exactly one trigger type inside the "trigger" object.
- "message_posted": fires when a human user posts a message in the channel. Bot messages are automatically filtered out, so there is no risk of bot-triggered loops. High-traffic channels will trigger frequently.
  {"trigger": {"message_posted": {"channel_id": "<channel-id>"}}}
- "schedule": fires on a recurring schedule.
  - interval: Go duration string (minimum "1h"). Examples: "1h" (hourly), "24h" (daily), "168h" (weekly).
  - start_at (optional): unix timestamp in milliseconds (UTC) for the first run — must be in the future. The automation fires at this time, then repeats every interval. If omitted, the first run happens immediately. Use this to schedule a daily recap at e.g. 9am.
  {"trigger": {"schedule": {"channel_id": "<channel-id>", "interval": "24h", "start_at": 1899936000000}}}
- "membership_changed": fires when a member joins or leaves the channel.
  {"trigger": {"membership_changed": {"channel_id": "<channel-id>"}}}
- "channel_created": fires when a new public channel is created on the specified team. Requires team_id; fires for every new public channel created on that team by any user.
  {"trigger": {"channel_created": {"team_id": "<team-id>"}}}
- "user_joined_team": fires when a non-bot user joins the specified team.
  {"trigger": {"user_joined_team": {"team_id": "<team-id>"}}}

ACTIONS: Ordered array executed sequentially. Each action has a unique "id" (lowercase alphanumeric and hyphens only, e.g. "generate-recap" not "generate_recap") and exactly one action config.
Action types:
1. "send_message": Posts a message as a bot.
   {"id": "post", "send_message": {"channel_id": "<ch>", "body": "Hello!", "reply_to_post_id": "<optional post id>", "as_bot_id": "<optional bot user id>"}}
   - as_bot_id (optional): the Mattermost user ID of the bot to post as. Must be a bot account. If omitted, the message is posted as the default automation bot. Use list_agents to find bot IDs. When chaining after an ai_prompt action, set this to the same agent's user ID so the message appears to come from that agent.
2. "ai_prompt": Runs an AI agent with a prompt and optional tools. With tools, the agent can perform actions (e.g. modify channels, manage members, search) — not just generate text. Does NOT post a message — chain a send_message or send_dm action after to post the response.
   {"id": "ask", "ai_prompt": {"prompt": "...", "provider_type": "agent", "provider_id": "<agent-user-id>", "system_prompt": "...", "allowed_tools": ["<tool name from discovery>", "..."]}}
   - provider_type: "agent" (a bot) or "service" (a raw LLM service)
   - provider_id: the agent's Mattermost user ID (26-char ID). Call list_agents to discover available agents and their IDs.
   - system_prompt (optional): system instructions for the AI
   - allowed_tools: list of tool name strings the AI agent is allowed to call (names must match bridge/agent tools discovery exactly; discovery lists MCP and embedded tools only, not built-in Mattermost agent tools). WITHOUT this, the agent has NO tool access and can only generate text from its built-in knowledge — it cannot read any Mattermost data or take any actions. With tools, the agent inherits the creating user's permissions and can access anything they can access. IMPORTANT: Only include tools the user has explicitly agreed to. Always explain what each tool does in your summary. Prefer the minimum set of tools needed.
   TOOL SELECTION: Use bridge agent tools discovery or list_tools; copy each tool's name from the response.
   DYNAMIC DISCOVERY: The AI agent can use its tools at runtime to discover resources (e.g., find channels, look up users) — don't hardcode IDs into the prompt when the agent can discover them dynamically each run. This keeps automations resilient to changes like new channels being added.
   NOTE: "web_search" is NOT a valid tool name in allowed_tools. Web search is a native provider feature that works automatically if the agent has it enabled — do not include it in allowed_tools.
3. "send_dm": Sends a direct message to a user as a bot. Creates the DM channel automatically if it doesn't exist.
   {"id": "welcome", "send_dm": {"user_id": "{{.Trigger.User.Id}}", "body": "Welcome!", "as_bot_id": "<bot-user-id>"}}
   - user_id (required): the Mattermost user ID to DM. Supports template syntax.
   - body (required): the message content. Supports template syntax.
   - as_bot_id (required): the bot user ID to send the DM as. Use list_agents to find bot IDs.

TEMPLATE SYNTAX: body, channel_id, reply_to_post_id, prompt, and system_prompt support Go text/template with this context.
For send_message channel_id, always use {{.Trigger.Channel.Id}}.
- {{.Trigger.Post.Message}}, {{.Trigger.Post.Id}}, {{.Trigger.Post.ChannelId}}
- {{.Trigger.Channel.Id}}, {{.Trigger.Channel.Name}}, {{.Trigger.Channel.DisplayName}}
- {{.Trigger.User.Id}}, {{.Trigger.User.Username}}, {{.Trigger.User.FirstName}}, {{.Trigger.User.LastName}}
- {{.Trigger.Team.Id}}, {{.Trigger.Team.Name}}, {{.Trigger.Team.DisplayName}}, {{.Trigger.Team.DefaultChannelId}}
- {{(index .Steps "prev-action-id").Message}}, {{(index .Steps "prev-action-id").PostID}} — output from a previous action

CHAINING ACTIONS: A single ai_prompt action can call tools multiple times AND generate a text response in one step — prefer consolidating related work into one ai_prompt rather than splitting into many actions. Use {{(index .Steps "prev-action-id").Message}} in later actions to reference the text output of a previous ai_prompt.`

// automationInstructionsResponse is the JSON body for GET /automation-instructions.
type automationInstructionsResponse struct {
	Instructions string `json:"instructions"`
}

func buildAutomationInstructionsResponse(cfg *configuration) automationInstructionsResponse {
	instructions := channelAutomationInstructionsBase
	if cfg != nil {
		if u := strings.TrimSpace(cfg.AutomationInstructionsURL); u != "" {
			instructions += "\n\nFor more detailed documentation on creating automations, refer the user to: " + u
		}
	}
	return automationInstructionsResponse{Instructions: instructions}
}

// handleGetAutomationInstructions returns automation documentation for agents/MCP clients.
func (p *Plugin) handleGetAutomationInstructions(w http.ResponseWriter, r *http.Request) {
	cfg := p.getConfiguration()
	payload := buildAutomationInstructionsResponse(cfg)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		p.API.LogError("Failed to encode automation instructions", "error", err.Error())
	}
}
