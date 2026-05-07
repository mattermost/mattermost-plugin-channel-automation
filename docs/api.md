# Management API

Base URL: `{siteUrl}/plugins/com.mattermost.channel-automation/api/v1`

## Authentication

All endpoints require a valid Mattermost session — the `Mattermost-User-ID` header must be present. Returns `401 Unauthorized` if missing.

All endpoints additionally check permissions. **System admins** (`manage_system`) are always allowed. For non-admins, authorization depends on the flow's trigger type:

- **Channel-scoped triggers** (`message_posted`, `schedule`, `membership_changed`): the user must be a **channel admin** (`SchemeAdmin`) on every literal channel referenced in the flow (the trigger channel and any literal `send_message.channel_id`). Returns `403 Forbidden` with `"you do not have channel admin permissions on one or more channels referenced by this flow"`.
- **Team-scoped triggers** (`channel_created`): the user must be a **team admin** (`manage_team`) on the trigger's `team_id`, and every literal channel referenced in the flow must belong to that team. Returns `403 Forbidden` with either `"you must be a team admin on the team specified in the channel_created trigger"` or `"channel <id> does not belong to the team specified in the channel_created trigger"`.

In practice, validation (`ValidateSendMessageChannel`) already requires `send_message.channel_id` to be either the literal trigger channel ID or the template `{{.Trigger.Channel.Id}}`, so for channel-scoped triggers the set of literal channels checked collapses to the trigger channel, and for `channel_created` any literal `send_message.channel_id` must belong to `team_id`.

The list endpoint filters results to only flows the user has permission to view under the rules above.

### MCP tool hook endpoints

The plugin exposes internal MCP tool hook callbacks at `POST /hooks/tools/{flow_id}/{action_id}/before`. This endpoint is invoked by the Mattermost AI plugin while an `ai_prompt` action runs and **must only be called by the automation creator**: in addition to the global session check, the handler compares `Mattermost-User-ID` against the flow's `created_by` and returns `403 Forbidden` on mismatch (or when the flow has no recorded creator). System admin status does not bypass this check.

## Endpoints

### Get client configuration

```
GET /config
```

Returns the client-relevant plugin configuration. Any authenticated user may call this endpoint — no additional permission checks are performed.

**Response:** `200 OK`

```json
{
    "enable_ui": false
}
```

| Field       | Type    | Description                                                                 |
| ----------- | ------- | --------------------------------------------------------------------------- |
| `enable_ui` | boolean | Whether the Channel Automation UI is enabled in the webapp product switcher |

---

### Get automation instructions (for agents / MCP)

```http
GET /automation-instructions
```

Returns documentation for agents/MCP: a single **`instructions`** string (the body returned by the `get_automation_instructions` tool), including an optional closing paragraph with a plain documentation URL when **Automation instructions URL** is set in plugin settings.

Any authenticated user may call this endpoint — no channel-admin check is performed (same as `GET /config`).

**Response:** `200 OK`

```json
{
  "instructions": "Channel automations are trigger-action workflows..."
}
```

| Field          | Type   | Description                                                                                                                                                                                                 |
| -------------- | ------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `instructions` | string | Detailed documentation for triggers, actions, templates, `allowed_tools`, and the required user-confirmation workflow. If the plugin setting **Automation instructions URL** is set, a closing paragraph is appended with that URL so the model can mention it to users. |

---

### List flows

```
GET /flows
GET /flows?channel_id=<channel-id>
```

Returns all flows visible to the requesting user. System admins see all flows; other users only see flows where they have channel admin permissions on all referenced channels.

**Query parameters:**

| Parameter    | Type   | Description                                                      |
| ------------ | ------ | ---------------------------------------------------------------- |
| `channel_id` | string | _(optional)_ Filter to flows whose trigger targets this channel. |

**Response:** `200 OK`

```json
[
    {
        "id": "abc123...",
        "name": "Notify on new posts",
        "enabled": true,
        "trigger": {
            "message_posted": {
                "channel_id": "channel-id-1"
            }
        },
        "actions": [
            {
                "id": "send-notification",
                "send_message": {
                    "channel_id": "channel-id-2",
                    "body": "New post by {{.Trigger.User.Username}}"
                }
            }
        ],
        "created_at": 1735689600000,
        "updated_at": 1735689600000,
        "created_by": "user-id"
    }
]
```

**Errors:**

| Status | Body                   |
| ------ | ---------------------- |
| 500    | `failed to list flows` |

---

### Create flow

```
POST /flows
```

Creates a new flow. The server assigns `id`, `created_at`, `updated_at`, and `created_by`. Each action must include a user-specified `id` (lowercase slug format, e.g. `"send-greeting"`).

For every `ai_prompt` action with `provider_type: "agent"`, the server confirms with the AI plugin bridge that the flow's creator has access to the chosen `provider_id`, regardless of whether `allowed_tools` is set. Inaccessible, deactivated, or non-existent agent IDs are rejected at create/update time instead of failing with an opaque 403 at execute time. The bridge is queried at most once per distinct `provider_id` per request, so the access check and the `allowed_tools` validation share a single bridge round-trip.

`allowed_tools` and `guardrails` are only valid when `provider_type` is `"agent"`. The bridge rejects `allowed_tools` on service completion endpoints with HTTP 400; the server mirrors that rule at save time so the misconfiguration surfaces immediately.

For `ai_prompt` actions with `allowed_tools`, every entry must be a tool the action's agent actually exposes. Embedded Mattermost MCP tools are additionally validated against an explicit catalog: any embedded tool not in the catalog is rejected, and catalog entries marked not permitted (currently `create_post`, `dm`, `group_message`, and the automation-management tools `list_automations` / `get_automation_instructions` / `create_automation` / `update_automation` / `delete_automation`) are rejected. External (non-embedded) MCP tools are not subject to the catalog check.

When `guardrails` is set on an `ai_prompt` action, `allowed_tools` must be non-empty, and each `guardrails.channel_ids` entry must be a distinct 26-character Mattermost ID.

**Request body** (max 1 MB):

```json
{
    "name": "Notify on new posts",
    "enabled": true,
    "trigger": {
        "message_posted": {
            "channel_id": "channel-id-1"
        }
    },
    "actions": [
        {
            "id": "send-notification",
            "send_message": {
                "channel_id": "channel-id-2",
                "body": "New post by {{.Trigger.User.Username}}"
            }
        }
    ]
}
```

**Response:** `201 Created`

The created flow object with all server-assigned fields populated.

**Errors:**

| Status | Body                                                                                                        |
| ------ | ----------------------------------------------------------------------------------------------------------- |
| 400    | `invalid request body`                                                                                      |
| 400    | `name is required`                                                                                          |
| 400    | `name must be 100 characters or fewer`                                                                      |
| 400    | Action validation error (missing/invalid/duplicate ID)                                                      |
| 400    | Trigger validation error (missing/invalid fields)                                                           |
| 400    | `action <i>: tool "<name>" is not allowed in automations` (disallowed `allowed_tools` entry)               |
| 400    | `action <i>: guardrails requires non-empty allowed_tools`                                                   |
| 400    | `action <i>: invalid channel id ... in guardrails.channel_ids` (or duplicate / empty entry)                  |
| 400    | `action <i>: ai_prompt with provider_type "agent" requires provider_id`                                     |
| 400    | `action <i>: allowed_tools is only supported with provider_type "agent"`                                    |
| 400    | `action <i>: guardrails is only supported with provider_type "agent"`                                       |
| 403    | `you do not have channel admin permissions on one or more channels referenced by this flow`                 |
| 409    | `channel has reached the maximum of <N> flow(s)`                                                            |
| 500    | `failed to create flow`                                                                                     |
| 502    | `action <i>: failed to list tools for agent "<id>": ...` (creator cannot access the agent, or the bridge returned an error) |
| 502    | `action <i>: cannot verify access to agent "<id>": bridge client unavailable` (AI plugin not installed/active) |

---

### Get flow

```
GET /flows/{id}
```

Returns a single flow by ID.

**Response:** `200 OK`

The flow object.

**Errors:**

| Status | Body                                                                                        |
| ------ | ------------------------------------------------------------------------------------------- |
| 403    | `you do not have channel admin permissions on one or more channels referenced by this flow` |
| 404    | `flow not found`                                                                            |
| 500    | `failed to get flow`                                                                        |

---

### Update flow

```
PUT /flows/{id}
```

Replaces a flow. The server preserves immutable fields (`id`, `created_at`, `created_by`) and updates `updated_at`. Each action must include a user-specified `id` (lowercase slug format). `allowed_tools` validation matches [Create flow](#create-flow); the agent access check uses the original `created_by` (not the editor), so a system admin editing on behalf of another user cannot bypass the original creator's bridge ACL.

**Request body** (max 1 MB):

```json
{
    "name": "Updated name",
    "enabled": false,
    "trigger": {
        "message_posted": {
            "channel_id": "channel-id-2"
        }
    },
    "actions": [
        {
            "id": "new-action",
            "send_message": {
                "channel_id": "channel-id-3",
                "body": "updated message"
            }
        }
    ]
}
```

**Response:** `200 OK`

The updated flow object.

**Errors:**

| Status | Body                                                                                                        |
| ------ | ----------------------------------------------------------------------------------------------------------- |
| 400    | `invalid request body`                                                                                      |
| 400    | `name is required`                                                                                          |
| 400    | `name must be 100 characters or fewer`                                                                      |
| 400    | Action validation error (missing/invalid/duplicate ID)                                                      |
| 400    | Trigger validation error (missing/invalid fields)                                                           |
| 400    | `action <i>: tool "<name>" is not allowed in automations` (disallowed `allowed_tools` entry)               |
| 400    | `action <i>: guardrails requires non-empty allowed_tools`                                                   |
| 400    | `action <i>: invalid channel id ... in guardrails.channel_ids` (or duplicate / empty entry)                  |
| 400    | `action <i>: ai_prompt with provider_type "agent" requires provider_id`                                     |
| 400    | `action <i>: allowed_tools is only supported with provider_type "agent"`                                    |
| 400    | `action <i>: guardrails is only supported with provider_type "agent"`                                       |
| 403    | `you do not have channel admin permissions on one or more channels referenced by this flow`                 |
| 404    | `flow not found`                                                                                            |
| 409    | `channel has reached the maximum of <N> flow(s)`                                                            |
| 500    | `failed to update flow`                                                                                     |
| 502    | `action <i>: failed to list tools for agent "<id>": ...` (creator cannot access the agent, or the bridge returned an error) |
| 502    | `action <i>: cannot verify access to agent "<id>": bridge client unavailable` (AI plugin not installed/active) |

---

### Delete flow

```
DELETE /flows/{id}
```

Deletes a flow by ID.

**Response:** `204 No Content`

**Errors:**

| Status | Body                                                                                        |
| ------ | ------------------------------------------------------------------------------------------- |
| 403    | `you do not have channel admin permissions on one or more channels referenced by this flow` |
| 404    | `flow not found`                                                                            |
| 500    | `failed to delete flow`                                                                     |

---

### List agent tools

```
GET /agents/{agent_id}/tools
```

Returns the tools available for a specific AI agent. Proxies the request to the AI plugin bridge.

**Response:** `200 OK`

```json
[
    {
        "name": "search",
        "description": "Search for information"
    },
    {
        "name": "create_post",
        "description": "Create a new post in a channel"
    }
]
```

**Errors:**

| Status | Body                             |
| ------ | -------------------------------- |
| 502    | `failed to get agent tools`      |
| 503    | `AI plugin bridge not available` |

---

### List executions for a flow

```
GET /flows/{flow_id}/executions
GET /flows/{flow_id}/executions?limit=50
```

Returns execution history records for a specific flow, ordered by most recent first. The user must have permission to view the flow (system admin or channel admin on all referenced channels).

**Query parameters:**

| Parameter | Type    | Description                                                           |
| --------- | ------- | --------------------------------------------------------------------- |
| `limit`   | integer | _(optional)_ Maximum number of records to return (1–100, default: 20) |

**Response:** `200 OK`

```json
[
    {
        "id": "exec-id-1",
        "flow_id": "flow-id-1",
        "flow_name": "My Flow",
        "status": "success",
        "steps": {
            "send-greeting": {
                "post_id": "post-id-1",
                "channel_id": "channel-id-1",
                "message": "Hello!"
            }
        },
        "trigger_data": { ... },
        "created_at": 1735689600000,
        "started_at": 1735689600100,
        "completed_at": 1735689600500
    }
]
```

**Errors:**

| Status | Body                        |
| ------ | --------------------------- |
| 403    | `forbidden`                 |
| 404    | `flow not found`            |
| 500    | `failed to get flow`        |
| 500    | `failed to list executions` |

---

### Get execution

```
GET /executions/{id}
```

Returns a single execution record by ID. The user must have permission to view the parent flow. If the flow has been deleted, only system admins can view the execution.

**Response:** `200 OK`

An [ExecutionRecord](#executionrecord) object.

**Errors:**

| Status | Body                      |
| ------ | ------------------------- |
| 403    | `forbidden`               |
| 404    | `execution not found`     |
| 500    | `failed to get execution` |
| 500    | `failed to get flow`      |

---

### List recent executions

```
GET /executions
GET /executions?limit=50
```

Returns recent execution records across all flows. **System admin only.**

**Query parameters:**

| Parameter | Type    | Description                                                           |
| --------- | ------- | --------------------------------------------------------------------- |
| `limit`   | integer | _(optional)_ Maximum number of records to return (1–100, default: 20) |

**Response:** `200 OK`

An array of [ExecutionRecord](#executionrecord) objects.

**Errors:**

| Status | Body                        |
| ------ | --------------------------- |
| 403    | `forbidden`                 |
| 500    | `failed to list executions` |

---

## Data types

### Flow

| Field        | Type                | Description                                                    |
| ------------ | ------------------- | -------------------------------------------------------------- |
| `id`         | string              | 26-character unique ID (server-assigned)                       |
| `name`       | string              | Display name                                                   |
| `enabled`    | boolean             | Whether the flow is active                                     |
| `trigger`    | [Trigger](#trigger) | When the flow fires                                            |
| `actions`    | [Action](#action)[] | Steps to execute                                               |
| `created_at` | integer             | Creation time in milliseconds since epoch (server-assigned)    |
| `updated_at` | integer             | Last update time in milliseconds since epoch (server-assigned) |
| `created_by` | string              | User ID of the creator (server-assigned)                       |

### ExecutionRecord

| Field          | Type                                 | Description                                               |
| -------------- | ------------------------------------ | --------------------------------------------------------- |
| `id`           | string                               | Unique execution ID (server-assigned)                     |
| `flow_id`      | string                               | ID of the flow that was executed                          |
| `flow_name`    | string                               | Name of the flow at execution time                        |
| `status`       | string                               | `"success"` or `"failed"`                                 |
| `error`        | string                               | _(optional)_ Error message if status is `"failed"`        |
| `steps`        | map[string][StepOutput](#stepoutput) | Output from each executed action step, keyed by action ID |
| `trigger_data` | [TriggerData](#trigger-data-fields)  | Snapshot of the trigger event data                        |
| `created_at`   | integer                              | Time the execution was queued (ms since epoch)            |
| `started_at`   | integer                              | Time execution started (ms since epoch)                   |
| `completed_at` | integer                              | Time execution completed (ms since epoch)                 |

### StepOutput

| Field        | Type    | Description                                    |
| ------------ | ------- | ---------------------------------------------- |
| `post_id`    | string  | Post ID created by the action (if applicable)  |
| `channel_id` | string  | Channel ID where the action operated           |
| `message`    | string  | Output message from the action                 |
| `truncated`  | boolean | _(optional)_ Whether the message was truncated |

### Trigger

Exactly one key should be set, indicating the trigger type:

| Field                | Type                                                  | Description                                                       |
| -------------------- | ----------------------------------------------------- | ----------------------------------------------------------------- |
| `message_posted`     | [MessagePostedConfig](#messagepostedconfig)           | _(optional)_ Fires on new messages in a channel                   |
| `schedule`           | [ScheduleConfig](#scheduleconfig)                     | _(optional)_ Fires on a recurring schedule                        |
| `membership_changed` | [MembershipChangedConfig](#membershipchangedconfig)   | _(optional)_ Fires when a user joins or leaves a channel          |
| `channel_created`    | [ChannelCreatedConfig](#channelcreatedconfig)         | _(optional)_ Fires when a new public channel is created on a team |
| `user_joined_team`   | [UserJoinedTeamConfig](#userjoinedteamconfig)         | Fires when a user joins the configured team (`team_id` required)  |

#### MessagePostedConfig

| Field                     | Type    | Description                                                                                                  |
| ------------------------- | ------- | ------------------------------------------------------------------------------------------------------------ |
| `channel_id`              | string  | Channel to watch (required)                                                                                  |
| `include_thread_replies`  | boolean | _(optional)_ When `true`, posts that are thread replies (have a `root_id`) also fire the trigger. Defaults to `false` — thread replies are ignored. |

#### ScheduleConfig

| Field        | Type    | Description                                                                                                                  |
| ------------ | ------- | ---------------------------------------------------------------------------------------------------------------------------- |
| `channel_id` | string  | Channel associated with the schedule (required)                                                                              |
| `interval`   | string  | Go duration string, e.g. `"1h"`, `"24h"` (required, minimum 1h)                                                              |
| `start_at`   | integer | _(optional)_ Future UTC timestamp in milliseconds since epoch. Must be in the future; omit or set to 0 to start immediately. |

#### MembershipChangedConfig

| Field        | Type   | Description                                                                      |
| ------------ | ------ | -------------------------------------------------------------------------------- |
| `channel_id` | string | Channel to watch (required)                                                      |
| `action`     | string | _(optional)_ `"joined"`, `"left"`, or empty string to match both (default: `""`) |

#### ChannelCreatedConfig

| Field     | Type   | Description                                                    |
| --------- | ------ | -------------------------------------------------------------- |
| `team_id` | string | Team to watch for new public channels (required)               |

```json
{ "channel_created": { "team_id": "team-id-1" } }
```

#### UserJoinedTeamConfig

| Field       | Type   | Description                                                                     |
| ----------- | ------ | ------------------------------------------------------------------------------- |
| `team_id`   | string | Team to watch (required)                                                        |
| `user_type` | string | _(optional)_ `"user"`, `"guest"`, or empty string to match both (default: `""`) |

```json
{ "user_joined_team": { "team_id": "team-id-1", "user_type": "user" } }
```

### Action

Exactly one type-specific config key should be set alongside `id`:

| Field          | Type                                                | Description                                                                                                                     |
| -------------- | --------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------- |
| `id`           | string                                              | User-specified slug ID (required, lowercase alphanumeric with hyphens, e.g. `"send-greeting"`). Must be unique within the flow. |
| `send_message` | [SendMessageActionConfig](#sendmessageactionconfig) | _(optional)_ Posts a message                                                                                                    |
| `ai_prompt`    | [AIPromptActionConfig](#aipromptactionconfig)       | _(optional)_ Sends a prompt to an AI service                                                                                    |

#### SendMessageActionConfig

| Field              | Type   | Description                                                                       |
| ------------------ | ------ | --------------------------------------------------------------------------------- |
| `channel_id`       | string | Target channel ID. Supports Go templates.                                         |
| `reply_to_post_id` | string | _(optional)_ Post ID to reply to, creating a thread. Supports Go templates.       |
| `as_bot_id`        | string | _(optional)_ Bot user ID to post as. Defaults to the plugin bot if not specified. |
| `body`             | string | Message body as a Go `text/template` string                                       |

#### AIPromptActionConfig

| Field              | Type                            | Description                                                                                                             |
| ------------------ | ------------------------------- | ----------------------------------------------------------------------------------------------------------------------- |
| `system_prompt`    | string                          | _(optional)_ System prompt template (Go `text/template` syntax). Sent as the first message with role `"system"`.        |
| `prompt`           | string                          | The prompt template (Go `text/template` syntax)                                                                         |
| `provider_type`    | string                          | Either `"agent"` or `"service"`                                                                                         |
| `provider_id`      | string                          | ID of the agent or service to use                                                                                       |
| `allowed_tools`    | string[]                        | _(optional, agent only)_ Allowlist of tool names the agent may use without approval. Only valid when `provider_type` is `"agent"` (services reject `allowed_tools` at the bridge). Each entry must be available to the action's agent. Embedded Mattermost MCP tools must be in the supported catalog with `Allowed=true`; unknown embedded tools and disallowed catalog entries (`create_post`, `dm`, `group_message`, and the `*_automation` management tools) are rejected. |
| `guardrails`       | [Guardrails](#guardrails)       | _(optional, agent only)_ When set with non-empty `channel_ids` and non-empty `allowed_tools`, registers MCP tool hooks so tool args/results are constrained to those channels. Only valid when `provider_type` is `"agent"`. |
| `request_as`       | string                          | _(optional)_ Selects which user the AI completion request is attributed to. One of `"triggerer"` (default — the user who triggered the automation, falling back to the flow creator) or `"creator"` (always the flow creator). Any other value is rejected at create/update time. |

Requires the AI plugin (`mattermost-plugin-agents`) to be installed and active.

#### Guardrails

| Field          | Type     | Description                                                                                                                                 |
| -------------- | -------- | ------------------------------------------------------------------------------------------------------------------------------------------- |
| `channel_ids`  | string[] | Mattermost channel IDs (26 characters each). When non-empty (and `allowed_tools` is set), the automation registers per-tool before hooks so only these channels are exposed to the agent via supported tools. Duplicate or empty entries are rejected. |

Hook handlers maintain an explicit catalog of **production** Mattermost built-in MCP tool names from the agents plugin (dev-only tools such as `create_user` / `create_post_as_user` / `create_team` / `add_user_to_team` are excluded). The bridge is told about a tool's `before` callback only when the catalog declares one. Tools not in the catalog get no callbacks at all and ride the agent's normal allowed_tools path. As defense in depth, the hook HTTP handler still rejects any callback that arrives for a tool that is not in the catalog.

`allowed_tools` is also re-validated at execution time (not just on flow create/update), so catalog updates that demote a tool to disallowed, or agent changes that remove a tool, take effect on already-saved automations without requiring a re-save. The validator additionally rejects `get_user_channels` whenever guardrails are configured for the same `ai_prompt` action.

When a hook rejects a tool call (missing or disallowed `channel_id`, or a resolved channel that is not permitted), the error returned to the agent includes the rejected ID and the list of allowed `channel_ids` so the model can self-correct. The list is capped at 10 IDs followed by `(+N more)` to keep the payload bounded.

For `get_team_info` and `get_team_members`, guardrails restrict `team_id` to the **allowed-team set**: the union of (a) the automation's trigger team (the `channel_created` or `user_joined_team` trigger's `team_id`, or the team of the trigger channel for channel-scoped trigger types) and (b) the team of every channel listed in `channel_ids`. This means a guardrail set may span multiple teams and team tools work for all of them. Direct- and group-message channels in `channel_ids` contribute nothing to the allowed-team set (they have no team) but still work for channel-scoped tools. For `get_team_info`, only an explicit `team_id` is allowed under guardrails (no `team_name`-only lookup). For `get_channel_info`, only an explicit `channel_id` is allowed under guardrails (the `channel_name` + `team_id` resolution path is rejected).

Channel → team lookups for the allowed-team set are memoized per-channel inside the hook handler (channel → team is immutable in Mattermost), so each unique guardrail channel triggers at most one `GetChannel` call per plugin run.

Omit `guardrails` or use an empty `channel_ids` list for no channel restriction.

---

## Trigger types

### `message_posted`

Fires when a new message is posted in the specified channel. By default, posts that are thread replies (i.e. have a `root_id`) are ignored; set `include_thread_replies: true` to fire on replies as well.

When `include_thread_replies: true` is set and the firing post is itself a reply, the trigger handler additionally fetches the parent thread (root + replies, sorted oldest first, with each author's user pre-resolved) and exposes it at `{{.Trigger.Thread}}`. The `ai_prompt` action automatically renders this thread as a transcript inside its `<user_data>` block; other actions can consume it via templates. The thread fetch is best-effort: a failure logs a warning and the flow continues with `{{.Trigger.Thread}}` left nil. Root-post fires never trigger a thread fetch.

To bound work item size for the Mattermost KV store, threads larger than 61 messages are truncated to the root post plus the most recent 60 replies. `{{.Trigger.Thread.PostCount}}` always reflects the original full thread size, and `{{.Trigger.Thread.Truncated}}` reports whether older replies were dropped. The `ai_prompt` action surfaces the truncation in its trigger-context system message so the model knows it is not seeing the full conversation.

### `schedule`

Fires on a recurring interval. The `interval` field accepts any Go `time.ParseDuration` string (e.g. `"1h"`, `"24h"`). The minimum interval is 1 hour.

If `start_at` is provided, it must be a future UTC timestamp in milliseconds. The first execution is scheduled at that time. Otherwise, the schedule starts immediately.

### `membership_changed`

Fires when a user joins or leaves the specified channel. Bot users are automatically excluded. The membership action (`"joined"` or `"left"`) is available in the trigger data via `{{.Trigger.Membership.Action}}`.

### `channel_created`

Fires when a new public channel (type `"O"`) is created on the specified `team_id`. DMs, group messages, and private channels are excluded. Authorization for this trigger is team-scoped: the creating user must be a team admin on `team_id` (or a system admin), and any literal action channel references must belong to the same team.

### `user_joined_team`

Fires when a user joins the configured team. Bot users are automatically excluded. The optional `user_type` field filters by user role: `"user"` matches only regular users, `"guest"` matches only guests, and `""` (default) matches both. The user creating the flow must be a team admin or a channel admin on the team's default channel (town-square). Team information is available via `{{.Trigger.Team.Id}}`, `{{.Trigger.Team.Name}}`, and `{{.Trigger.Team.DisplayName}}`. The team's default channel ID is available via `{{.Trigger.Team.DefaultChannelId}}`. The user's guest status is available via `{{.Trigger.User.IsGuest}}`.

---

## Action types

### `send_message`

Posts a message to a channel as the plugin bot user.

The `body`, `channel_id`, and `reply_to_post_id` fields are rendered as Go templates with the flow context.

### `ai_prompt`

Sends a rendered prompt to an AI agent or service via the Mattermost AI plugin bridge and stores the response.

By default, the completion request is attributed to the user who triggered the automation (`{{.Trigger.User.Id}}`). When the trigger has no associated user (e.g. `schedule`), the request falls back to the flow creator (`{{.CreatedBy}}`). The optional `request_as` config field lets the automation switch attribution to the flow creator unconditionally:

- `"triggerer"` (or unset, default) — use the triggering user, falling back to the creator.
- `"creator"` — always use the flow creator, even when a triggering user is available.

The set of attributable identities is bounded to these two principals; arbitrary user IDs cannot be configured. The resolved user and its source (`triggerer` or `creator`) are emitted on the plugin's debug log alongside `action_id` and `provider_id`.

Requires the AI plugin (`mattermost-plugin-agents`) to be installed and active.

In addition to the configured `system_prompt` and `prompt`, the action automatically
injects a trusted trigger-context system message that includes the current date as
an RFC 3339 timestamp (UTC) annotated with the weekday, plus the equivalent Unix
timestamp in milliseconds (matching the format used for timestamps stored by the plugin):

```text
Current Date: 2026-04-22T14:30:45Z (Wednesday)
Current Unix Timestamp (ms): 1776868245000
```

These are sourced from the plugin server clock at execution time (not template-accessible).

---

## Template context

Action templates receive a `FlowContext` object with the following structure:

```
{{.FlowID}}             — id of the flow being executed
{{.CreatedBy}}          — user ID of the flow creator
{{.Trigger}}            — trigger event data
{{.Trigger.Post}}       — the post that triggered the flow (message_posted only)
{{.Trigger.Channel}}    — the channel where the event occurred (message_posted, membership_changed, channel_created)
{{.Trigger.User}}       — the user who triggered the event (message_posted, membership_changed, channel_created, user_joined_team)
{{.Trigger.Team}}       — the team the user joined (user_joined_team only)
{{.Trigger.Schedule}}   — schedule metadata (schedule only)
{{.Trigger.Membership}} — membership change metadata (membership_changed only)
{{.Trigger.Thread}}     — parent thread of the triggering reply (message_posted with include_thread_replies, reply posts only)
{{.Steps.<action_id>}}  — output from a previous action step
```

### Trigger data fields

**Post** _(message_posted trigger; same shape used inside each `Trigger.Thread.Messages` element):_

| Field      | Access                        | Notes                                                                                                                                                            |
| ---------- | ----------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| ID         | `{{.Trigger.Post.Id}}`        |                                                                                                                                                                  |
| Channel ID | `{{.Trigger.Post.ChannelId}}` |                                                                                                                                                                  |
| Thread ID  | `{{.Trigger.Post.ThreadId}}`  |                                                                                                                                                                  |
| Message    | `{{.Trigger.Post.Message}}`   | The raw post body. Slack-style attachment text is not appended.                                                                                                  |
| User       | `{{.Trigger.Post.User}}`      | Populated only on thread-message posts (`{{range .Trigger.Thread.Messages}}{{.User.AuthorDisplay}}{{end}}`). Nil on the top-level triggering post — use `{{.Trigger.User}}` there. |
| Create At  | `{{.Trigger.Post.CreateAt}}`  | Unix milliseconds. Populated only on thread-message posts.                                                                                                       |

**Channel** _(message_posted, membership_changed, channel_created):_

| Field        | Access                             |
| ------------ | ---------------------------------- |
| ID           | `{{.Trigger.Channel.Id}}`          |
| Name         | `{{.Trigger.Channel.Name}}`        |
| Display Name | `{{.Trigger.Channel.DisplayName}}` |

**User** _(message_posted, membership_changed, channel_created, user_joined_team):_

> **Note:** For `user_joined_team` triggers, `{{.Trigger.Channel}}` is **not** available. Use `{{.Trigger.Team.DefaultChannelId}}` to reference the team's default channel.

| Field          | Access                            | Notes                                                                                                |
| -------------- | --------------------------------- | ---------------------------------------------------------------------------------------------------- |
| ID             | `{{.Trigger.User.Id}}`            |                                                                                                      |
| Username       | `{{.Trigger.User.Username}}`      |                                                                                                      |
| First Name     | `{{.Trigger.User.FirstName}}`     |                                                                                                      |
| Last Name      | `{{.Trigger.User.LastName}}`      |                                                                                                      |
| Is Guest       | `{{.Trigger.User.IsGuest}}`       |                                                                                                      |
| Author Display | `{{.Trigger.User.AuthorDisplay}}` | Renders `@username (First Last)` with graceful fallbacks (down to user ID, then `"unknown"` on a nil receiver). Useful inside `{{range .Trigger.Thread.Messages}}` blocks. |

**Membership** _(membership_changed trigger only):_

| Field  | Access                           |
| ------ | -------------------------------- |
| Action | `{{.Trigger.Membership.Action}}` |

**Team** _(user_joined_team trigger only):_

| Field              | Access                               |
| ------------------ | ------------------------------------ |
| ID                 | `{{.Trigger.Team.Id}}`               |
| Name               | `{{.Trigger.Team.Name}}`             |
| Display Name       | `{{.Trigger.Team.DisplayName}}`      |
| Default Channel ID | `{{.Trigger.Team.DefaultChannelId}}` |

**Schedule** _(schedule trigger only):_

| Field    | Access                           |
| -------- | -------------------------------- |
| Fired At | `{{.Trigger.Schedule.FiredAt}}`  |
| Interval | `{{.Trigger.Schedule.Interval}}` |

**Thread** _(message_posted trigger with `include_thread_replies`, only when the firing post is a reply):_

| Field             | Access                                       | Notes                                                                                                                                                                                                                                                                                                                       |
| ----------------- | -------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Root ID           | `{{.Trigger.Thread.RootID}}`                 | ID of the thread root post.                                                                                                                                                                                                                                                                                                 |
| Post Count        | `{{.Trigger.Thread.PostCount}}`              | Number of posts in the original full thread (root included). When `Truncated` is `true`, this is larger than `len .Messages`.                                                                                                                                                                                               |
| Truncated         | `{{.Trigger.Thread.Truncated}}`              | `true` when older replies were dropped to keep the work item under the KV-store size cap. The root post is always retained alongside the most recent 60 replies.                                                                                                                                                            |
| Messages          | `{{range .Trigger.Thread.Messages}}…{{end}}` | Ordered oldest first. Each element is a [Post](#trigger-data-fields) with `User` and `CreateAt` populated. Capped at root + 60 most-recent replies; check `Truncated` to see if older replies were dropped.                                                                                                                 |
| Transcript        | `{{.Trigger.Thread.TranscriptDisplay}}`      | Plaintext transcript in `authorDisplay: message` form, with each post separated by a blank line so multi-line message bodies stay readable. Empty for a nil receiver. The `ai_prompt` action renders this automatically inside its `<user_data>` block and also discloses `Truncated` in its trigger-context system message. |

### Step output fields

Previous action outputs are available via `{{.Steps.<action_id>}}`.

> **Note:** Trigger data fields use `.Id` (e.g. `{{.Trigger.Post.Id}}`) following Mattermost model conventions, while step output fields use `.PostID` / `.ChannelID` following Go naming conventions. This difference reflects the underlying Go struct definitions.

| Field      | Access                             |
| ---------- | ---------------------------------- |
| Post ID    | `{{.Steps.<action_id>.PostID}}`    |
| Channel ID | `{{.Steps.<action_id>.ChannelID}}` |
| Message    | `{{.Steps.<action_id>.Message}}`   |
| Truncated  | `{{.Steps.<action_id>.Truncated}}` |
