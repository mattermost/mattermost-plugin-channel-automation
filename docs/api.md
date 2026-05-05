# Management API

Base URL: `{siteUrl}/plugins/com.mattermost.channel-automation/api/v1`

## Authentication

All endpoints require a valid Mattermost session — the `Mattermost-User-ID` header must be present. Returns `401 Unauthorized` if missing.

All endpoints additionally check permissions. **System admins** (`manage_system`) are always allowed. For non-admins, authorization depends on the flow's trigger type:

- **Channel-scoped triggers** (`message_posted`, `schedule`, `membership_changed`): the user must be a **channel admin** (`SchemeAdmin`) on every literal channel referenced in the flow (the trigger channel and any literal `send_message.channel_id`). Returns `403 Forbidden` with `"you do not have channel admin permissions on one or more channels referenced by this flow"`.
- **Team-scoped triggers** (`channel_created`): the user must be a **team admin** (`manage_team`) on the trigger's `team_id`, and every literal channel referenced in the flow must belong to that team. Returns `403 Forbidden` with either `"you must be a team admin on the team specified in the channel_created trigger"` or `"channel <id> does not belong to the team specified in the channel_created trigger"`.

In practice, validation (`ValidateSendMessageChannel`) already requires `send_message.channel_id` to be either the literal trigger channel ID or the template `{{.Trigger.Channel.Id}}`, so for channel-scoped triggers the set of literal channels checked collapses to the trigger channel, and for `channel_created` any literal `send_message.channel_id` must belong to `team_id`.

The list endpoint filters results to only flows the user has permission to view under the rules above.

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

For `ai_prompt` actions with `allowed_tools`, the server rejects the tool names `create_post`, `dm`, and `group_message`. Comparison is case-insensitive and ignores surrounding whitespace.

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
| 403    | `you do not have channel admin permissions on one or more channels referenced by this flow`                 |
| 409    | `channel has reached the maximum of <N> flow(s)`                                                            |
| 500    | `failed to create flow`                                                                                     |

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

Replaces a flow. The server preserves immutable fields (`id`, `created_at`, `created_by`) and updates `updated_at`. Each action must include a user-specified `id` (lowercase slug format). `allowed_tools` validation matches [Create flow](#create-flow).

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
| 403    | `you do not have channel admin permissions on one or more channels referenced by this flow`                 |
| 404    | `flow not found`                                                                                            |
| 409    | `channel has reached the maximum of <N> flow(s)`                                                            |
| 500    | `failed to update flow`                                                                                     |

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
| `allowed_tools`    | string[]                        | _(optional)_ Allowlist of tool names the agent may use without approval. May not include `create_post`, `dm`, or `group_message`. |
| `request_as`       | string                          | _(optional)_ Selects which user the AI completion request is attributed to. One of `"triggerer"` (default — the user who triggered the automation, falling back to the flow creator) or `"creator"` (always the flow creator). Any other value is rejected at create/update time. |

Requires the AI plugin (`mattermost-plugin-agents`) to be installed and active.

---

## Trigger types

### `message_posted`

Fires when a new message is posted in the specified channel. By default, posts that are thread replies (i.e. have a `root_id`) are ignored; set `include_thread_replies: true` to fire on replies as well.

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
{{.CreatedBy}}          — user ID of the flow creator
{{.Trigger}}            — trigger event data
{{.Trigger.Post}}       — the post that triggered the flow (message_posted only)
{{.Trigger.Channel}}    — the channel where the event occurred (message_posted, membership_changed, channel_created)
{{.Trigger.User}}       — the user who triggered the event (message_posted, membership_changed, channel_created, user_joined_team)
{{.Trigger.Team}}       — the team the user joined (user_joined_team only)
{{.Trigger.Schedule}}   — schedule metadata (schedule only)
{{.Trigger.Membership}} — membership change metadata (membership_changed only)
{{.Steps.<action_id>}}  — output from a previous action step
```

### Trigger data fields

**Post** _(message_posted trigger only):_

| Field      | Access                        |
| ---------- | ----------------------------- |
| ID         | `{{.Trigger.Post.Id}}`        |
| Channel ID | `{{.Trigger.Post.ChannelId}}` |
| Thread ID  | `{{.Trigger.Post.ThreadId}}`  |
| Message    | `{{.Trigger.Post.Message}}`   |

**Channel** _(message_posted, membership_changed, channel_created):_

| Field        | Access                             |
| ------------ | ---------------------------------- |
| ID           | `{{.Trigger.Channel.Id}}`          |
| Name         | `{{.Trigger.Channel.Name}}`        |
| Display Name | `{{.Trigger.Channel.DisplayName}}` |

**User** _(message_posted, membership_changed, channel_created, user_joined_team):_

> **Note:** For `user_joined_team` triggers, `{{.Trigger.Channel}}` is **not** available. Use `{{.Trigger.Team.DefaultChannelId}}` to reference the team's default channel.

| Field      | Access                        |
| ---------- | ----------------------------- |
| ID         | `{{.Trigger.User.Id}}`        |
| Username   | `{{.Trigger.User.Username}}`  |
| First Name | `{{.Trigger.User.FirstName}}` |
| Last Name  | `{{.Trigger.User.LastName}}`  |
| Is Guest   | `{{.Trigger.User.IsGuest}}`   |

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

### Step output fields

Previous action outputs are available via `{{.Steps.<action_id>}}`.

> **Note:** Trigger data fields use `.Id` (e.g. `{{.Trigger.Post.Id}}`) following Mattermost model conventions, while step output fields use `.PostID` / `.ChannelID` following Go naming conventions. This difference reflects the underlying Go struct definitions.

| Field      | Access                             |
| ---------- | ---------------------------------- |
| Post ID    | `{{.Steps.<action_id>.PostID}}`    |
| Channel ID | `{{.Steps.<action_id>.ChannelID}}` |
| Message    | `{{.Steps.<action_id>.Message}}`   |
| Truncated  | `{{.Steps.<action_id>.Truncated}}` |
