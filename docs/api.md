# Management API

Base URL: `{siteUrl}/plugins/com.mattermost.channel-automation/api/v1`

## Authentication

All endpoints require a valid Mattermost session — the `Mattermost-User-ID` header must be present. Returns `401 Unauthorized` if missing.

All endpoints additionally check permissions: **System Admins** (`manage_system`) are always allowed. Otherwise the user must be a **channel admin** (`SchemeAdmin`) on every channel referenced in the flow (trigger and action channel IDs). Returns `403 Forbidden` with the failing channel ID if neither condition is met. The list endpoint filters results to only flows the user has permission to view.

## Endpoints

### List flows

```
GET /flows
GET /flows?channel_id=<channel-id>
```

Returns all flows visible to the requesting user. System admins see all flows; other users only see flows where they have channel admin permissions on all referenced channels.

**Query parameters:**

| Parameter    | Type   | Description                                                                                                                               |
| ------------ | ------ | ----------------------------------------------------------------------------------------------------------------------------------------- |
| `channel_id` | string | _(optional)_ Filter to flows whose trigger targets this channel. Applies to `message_posted`, `schedule`, and `membership_changed` triggers. |

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

| Status | Body                                                        |
| ------ | ----------------------------------------------------------- |
| 400    | `invalid request body`                                      |
| 400    | Action validation error (missing/invalid/duplicate ID)      |
| 400    | Trigger validation error (missing/invalid fields)           |
| 403    | `you do not have channel admin permissions on channel <id>` |
| 500    | `failed to create flow`                                     |

---

### Get flow

```
GET /flows/{id}
```

Returns a single flow by ID.

**Response:** `200 OK`

The flow object.

**Errors:**

| Status | Body                                                        |
| ------ | ----------------------------------------------------------- |
| 403    | `you do not have channel admin permissions on channel <id>` |
| 404    | `flow not found`                                            |
| 500    | `failed to get flow`                                        |

---

### Update flow

```
PUT /flows/{id}
```

Replaces a flow. The server preserves immutable fields (`id`, `created_at`, `created_by`) and updates `updated_at`. Each action must include a user-specified `id` (lowercase slug format).

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

| Status | Body                                                        |
| ------ | ----------------------------------------------------------- |
| 400    | `invalid request body`                                      |
| 400    | Action validation error (missing/invalid/duplicate ID)      |
| 400    | Trigger validation error (missing/invalid fields)           |
| 403    | `you do not have channel admin permissions on channel <id>` |
| 404    | `flow not found`                                            |
| 500    | `failed to update flow`                                     |

---

### Delete flow

```
DELETE /flows/{id}
```

Deletes a flow by ID.

**Response:** `204 No Content`

**Errors:**

| Status | Body                                                        |
| ------ | ----------------------------------------------------------- |
| 403    | `you do not have channel admin permissions on channel <id>` |
| 404    | `flow not found`                                            |
| 500    | `failed to delete flow`                                     |

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

### Trigger

Exactly one key should be set, indicating the trigger type:

| Field                | Type                                                  | Description                                                       |
| -------------------- | ----------------------------------------------------- | ----------------------------------------------------------------- |
| `message_posted`     | [MessagePostedConfig](#messagepostedconfig)           | _(optional)_ Fires on new messages in a channel                   |
| `schedule`           | [ScheduleConfig](#scheduleconfig)                     | _(optional)_ Fires on a recurring schedule                        |
| `membership_changed` | [MembershipChangedConfig](#membershipchangedconfig)   | _(optional)_ Fires when a user joins or leaves a channel          |

#### MessagePostedConfig

| Field        | Type   | Description                 |
| ------------ | ------ | --------------------------- |
| `channel_id` | string | Channel to watch (required) |

#### ScheduleConfig

| Field        | Type    | Description                                                                          |
| ------------ | ------- | ------------------------------------------------------------------------------------ |
| `channel_id` | string  | Channel associated with the schedule (required)                                      |
| `interval`   | string  | Go duration string, e.g. `"1h"`, `"30m"` (required, minimum 5m)                     |
| `start_at`   | integer | _(optional)_ Start time in milliseconds since epoch. Defaults to flow creation time. |

#### MembershipChangedConfig

| Field        | Type   | Description                 |
| ------------ | ------ | --------------------------- |
| `channel_id` | string | Channel to watch (required) |

### Action

Exactly one type-specific config key should be set alongside `id`:

| Field          | Type                                                | Description                                                                                                                        |
| -------------- | --------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------- |
| `id`           | string                                              | User-specified slug ID (required, lowercase alphanumeric with hyphens, e.g. `"send-greeting"`). Must be unique within the flow.   |
| `send_message` | [SendMessageActionConfig](#sendmessageactionconfig) | _(optional)_ Posts a message                                                                                                       |
| `ai_prompt`    | [AIPromptActionConfig](#aipromptactionconfig)       | _(optional)_ Sends a prompt to an AI service                                                                                       |

#### SendMessageActionConfig

| Field              | Type   | Description                                                                 |
| ------------------ | ------ | --------------------------------------------------------------------------- |
| `channel_id`       | string | Target channel ID. Supports Go templates.                                   |
| `reply_to_post_id` | string | _(optional)_ Post ID to reply to, creating a thread. Supports Go templates. |
| `body`             | string | Message body as a Go `text/template` string                                 |

#### AIPromptActionConfig

| Field              | Type                            | Description                                                                                                             |
| ------------------ | ------------------------------- | ----------------------------------------------------------------------------------------------------------------------- |
| `system_prompt`    | string                          | _(optional)_ System prompt template (Go `text/template` syntax). Sent as the first message with role `"system"`.        |
| `prompt`           | string                          | The prompt template (Go `text/template` syntax)                                                                         |
| `provider_type`    | string                          | Either `"agent"` or `"service"`                                                                                         |
| `provider_id`      | string                          | ID of the agent or service to use                                                                                       |
| `allowed_tools`    | string[]                        | _(optional)_ Allowlist of tool names the agent may use without approval                                                 |
| `tool_constraints` | map[string]map[string]string[]  | _(optional)_ Restricts tool parameter values. Maps tool name → param name → allowed values. Requires `allowed_tools`.   |

Requires the AI plugin (`mattermost-plugin-ai`) to be installed and active.

---

## Trigger types

### `message_posted`

Fires when a new message is posted in the specified channel.

### `schedule`

Fires on a recurring interval. The `interval` field accepts any Go `time.ParseDuration` string (e.g. `"5m"`, `"1h"`, `"24h"`). The minimum interval is 5 minutes.

If `start_at` is provided, the first execution is scheduled at that time. Otherwise, the schedule starts from the flow's creation time.

### `membership_changed`

Fires when a user joins or leaves the specified channel. Bot users are automatically excluded. The membership action (`"joined"` or `"left"`) is available in the trigger data via `{{.Trigger.Membership.Action}}`.

---

## Action types

### `send_message`

Posts a message to a channel as the plugin bot user.

The `body`, `channel_id`, and `reply_to_post_id` fields are rendered as Go templates with the flow context.

### `ai_prompt`

Sends a rendered prompt to an AI agent or service via the Mattermost AI plugin bridge and stores the response.

Requires the AI plugin (`mattermost-plugin-ai`) to be installed and active.

---

## Template context

Action templates receive a `FlowContext` object with the following structure:

```
{{.Trigger}}            — trigger event data
{{.Trigger.Post}}       — the post that triggered the flow (message_posted only)
{{.Trigger.Channel}}    — the channel where the event occurred (message_posted, membership_changed)
{{.Trigger.User}}       — the user who triggered the event (message_posted, membership_changed)
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

**Channel** _(message_posted, membership_changed):_

| Field        | Access                             |
| ------------ | ---------------------------------- |
| ID           | `{{.Trigger.Channel.Id}}`          |
| Name         | `{{.Trigger.Channel.Name}}`        |
| Display Name | `{{.Trigger.Channel.DisplayName}}` |

**User** _(message_posted, membership_changed):_

| Field      | Access                        |
| ---------- | ----------------------------- |
| ID         | `{{.Trigger.User.Id}}`        |
| Username   | `{{.Trigger.User.Username}}`  |
| First Name | `{{.Trigger.User.FirstName}}` |
| Last Name  | `{{.Trigger.User.LastName}}`  |

**Membership** _(membership_changed trigger only):_

| Field  | Access                            |
| ------ | --------------------------------- |
| Action | `{{.Trigger.Membership.Action}}`  |

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
