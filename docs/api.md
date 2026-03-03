# Management API

Base URL: `{siteUrl}/plugins/com.mattermost.channel-automation/api/v1`

## Authentication

All endpoints require a valid Mattermost session — the `Mattermost-User-ID` header must be present. Returns `401 Unauthorized` if missing.

Write operations (create, update) additionally check permissions: **System Admins** (`manage_system`) are always allowed. Otherwise the user must be a **channel admin** (`SchemeAdmin`) on every channel referenced in the flow (trigger and action channel IDs). Returns `403 Forbidden` with the failing channel ID if neither condition is met.

## Endpoints

### List flows

```
GET /flows
```

Returns all flows.

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
                "id": "def456...",
                "name": "Send notification",
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

Creates a new flow. The server assigns `id`, `created_at`, `updated_at`, and `created_by`. Action IDs are auto-generated for actions that omit them.

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
            "name": "Send notification",
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

| Status | Body                 |
| ------ | -------------------- |
| 404    | `flow not found`     |
| 500    | `failed to get flow` |

---

### Update flow

```
PUT /flows/{id}
```

Replaces a flow. The server preserves immutable fields (`id`, `created_at`, `created_by`) and updates `updated_at`. Action IDs are auto-generated for actions that omit them.

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
            "name": "New Action",
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
| 403    | `you do not have channel admin permissions on channel <id>` |
| 404    | `flow not found`                                            |
| 500    | `failed to update flow`                                     |

---

### Delete flow

```
DELETE /flows/{id}
```

Deletes a flow. Returns success even if the flow doesn't exist.

**Response:** `204 No Content`

**Errors:**

| Status | Body                    |
| ------ | ----------------------- |
| 500    | `failed to delete flow` |

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

| Field            | Type                                        | Description                                     |
| ---------------- | ------------------------------------------- | ----------------------------------------------- |
| `message_posted` | [MessagePostedConfig](#messagepostedconfig) | _(optional)_ Fires on new messages in a channel |
| `schedule`       | [ScheduleConfig](#scheduleconfig)           | _(optional)_ Fires on a recurring schedule      |

#### MessagePostedConfig

| Field        | Type   | Description                 |
| ------------ | ------ | --------------------------- |
| `channel_id` | string | Channel to watch (required) |

#### ScheduleConfig

| Field      | Type    | Description                                                                          |
| ---------- | ------- | ------------------------------------------------------------------------------------ |
| `interval` | string  | Go duration string, e.g. `"1h"`, `"30m"` (required, minimum 5m)                      |
| `start_at` | integer | _(optional)_ Start time in milliseconds since epoch. Defaults to flow creation time. |

### Action

Exactly one type-specific config key should be set alongside `id` and `name`:

| Field          | Type                                                | Description                                  |
| -------------- | --------------------------------------------------- | -------------------------------------------- |
| `id`           | string                                              | Unique ID (auto-generated if omitted)        |
| `name`         | string                                              | Display name                                 |
| `send_message` | [SendMessageActionConfig](#sendmessageactionconfig) | _(optional)_ Posts a message                 |
| `ai_prompt`    | [AIPromptActionConfig](#aipromptactionconfig)       | _(optional)_ Sends a prompt to an AI service |

#### SendMessageActionConfig

| Field              | Type   | Description                                                                 |
| ------------------ | ------ | --------------------------------------------------------------------------- |
| `channel_id`       | string | Target channel ID. Supports Go templates.                                   |
| `reply_to_post_id` | string | _(optional)_ Post ID to reply to, creating a thread. Supports Go templates. |
| `body`             | string | Message body as a Go `text/template` string                                 |

#### AIPromptActionConfig

| Field           | Type   | Description                                     |
| --------------- | ------ | ----------------------------------------------- |
| `prompt`        | string | The prompt template (Go `text/template` syntax) |
| `provider_type` | string | Either `"agent"` or `"service"`                 |
| `provider_id`   | string | ID of the agent or service to use               |

Requires the AI plugin (`mattermost-plugin-ai`) to be installed and active.

---

## Trigger types

### `message_posted`

Fires when a new message is posted in the specified channel.

### `schedule`

Fires on a recurring interval. The `interval` field accepts any Go `time.ParseDuration` string (e.g. `"5m"`, `"1h"`, `"24h"`). The minimum interval is 5 minutes.

If `start_at` is provided, the first execution is scheduled at that time. Otherwise, the schedule starts from the flow's creation time.

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
{{.Trigger}}           — trigger event data
{{.Trigger.Post}}      — the post that triggered the flow (message_posted only)
{{.Trigger.Channel}}   — the channel where the event occurred (message_posted only)
{{.Trigger.User}}      — the user who triggered the event (message_posted only)
{{.Trigger.Schedule}}  — schedule metadata (schedule only)
{{.Steps.<action_id>}} — output from a previous action step
```

### Trigger data fields

**Post** _(message_posted trigger only):_

| Field      | Access                        |
| ---------- | ----------------------------- |
| ID         | `{{.Trigger.Post.Id}}`        |
| Channel ID | `{{.Trigger.Post.ChannelId}}` |
| Thread ID  | `{{.Trigger.Post.ThreadId}}`  |
| Message    | `{{.Trigger.Post.Message}}`   |

**Channel** _(message_posted trigger only):_

| Field        | Access                             |
| ------------ | ---------------------------------- |
| ID           | `{{.Trigger.Channel.Id}}`          |
| Name         | `{{.Trigger.Channel.Name}}`        |
| Display Name | `{{.Trigger.Channel.DisplayName}}` |

**User** _(message_posted trigger only):_

| Field      | Access                        |
| ---------- | ----------------------------- |
| ID         | `{{.Trigger.User.Id}}`        |
| Username   | `{{.Trigger.User.Username}}`  |
| First Name | `{{.Trigger.User.FirstName}}` |
| Last Name  | `{{.Trigger.User.LastName}}`  |

**Schedule** _(schedule trigger only):_

| Field    | Access                           |
| -------- | -------------------------------- |
| Fired At | `{{.Trigger.Schedule.FiredAt}}`  |
| Interval | `{{.Trigger.Schedule.Interval}}` |

### Step output fields

Previous action outputs are available via `{{.Steps.<action_id>}}`:

| Field      | Access                             |
| ---------- | ---------------------------------- |
| Post ID    | `{{.Steps.<action_id>.PostID}}`    |
| Channel ID | `{{.Steps.<action_id>.ChannelID}}` |
| Message    | `{{.Steps.<action_id>.Message}}`   |
