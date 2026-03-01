# Management API

Base URL: `{siteUrl}/plugins/com.mattermost.channel-automation/api/v1`

## Authentication

All endpoints require two layers of authentication, enforced via middleware:

1. **Mattermost session** — The `Mattermost-User-ID` header must be present (set automatically by the Mattermost server for authenticated plugin requests). Returns `401 Unauthorized` if missing.
2. **System Admin role** — The authenticated user must have the `manage_system` permission. Returns `403 Forbidden` otherwise.

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
      "type": "message_posted",
      "channel_id": "channel-id-1"
    },
    "actions": [
      {
        "id": "def456...",
        "name": "Send notification",
        "type": "send_message",
        "channel_id": "channel-id-2",
        "body": "New post by {{.Trigger.User.Username}}"
      }
    ],
    "created_at": 1735689600000,
    "updated_at": 1735689600000,
    "created_by": "user-id"
  }
]
```

**Errors:**

| Status | Body |
|--------|------|
| 500 | `failed to list flows` |

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
    "type": "message_posted",
    "channel_id": "channel-id-1"
  },
  "actions": [
    {
      "name": "Send notification",
      "type": "send_message",
      "channel_id": "channel-id-2",
      "body": "New post by {{.Trigger.User.Username}}"
    }
  ]
}
```

**Response:** `201 Created`

The created flow object with all server-assigned fields populated.

**Errors:**

| Status | Body |
|--------|------|
| 400 | `invalid request body` |
| 500 | `failed to create flow` |

---

### Get flow

```
GET /flows/{id}
```

Returns a single flow by ID.

**Response:** `200 OK`

The flow object.

**Errors:**

| Status | Body |
|--------|------|
| 404 | `flow not found` |
| 500 | `failed to get flow` |

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
    "type": "message_posted",
    "channel_id": "channel-id-2"
  },
  "actions": [
    {
      "name": "New Action",
      "type": "send_message",
      "channel_id": "channel-id-3",
      "body": "updated message"
    }
  ]
}
```

**Response:** `200 OK`

The updated flow object.

**Errors:**

| Status | Body |
|--------|------|
| 400 | `invalid request body` |
| 404 | `flow not found` |
| 500 | `failed to update flow` |

---

### Delete flow

```
DELETE /flows/{id}
```

Deletes a flow. Returns success even if the flow doesn't exist.

**Response:** `204 No Content`

**Errors:**

| Status | Body |
|--------|------|
| 500 | `failed to delete flow` |

---

## Data types

### Flow

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | 26-character unique ID (server-assigned) |
| `name` | string | Display name |
| `enabled` | boolean | Whether the flow is active |
| `trigger` | [Trigger](#trigger) | When the flow fires |
| `actions` | [Action](#action)[] | Steps to execute |
| `created_at` | integer | Creation time in milliseconds since epoch (server-assigned) |
| `updated_at` | integer | Last update time in milliseconds since epoch (server-assigned) |
| `created_by` | string | User ID of the creator (server-assigned) |

### Trigger

| Field | Type | Description |
|-------|------|-------------|
| `type` | string | Trigger type (see [Trigger types](#trigger-types)) |
| `channel_id` | string | Channel to watch |

### Action

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Unique ID (auto-generated if omitted) |
| `name` | string | Display name |
| `type` | string | Action type (see [Action types](#action-types)) |
| `channel_id` | string | Target channel ID. Supports Go templates. |
| `reply_to_post_id` | string | *(optional)* Post ID to reply to, creating a thread. Supports Go templates. |
| `body` | string | Message body as a Go `text/template` string |
| `config` | object | *(optional)* Action-type-specific configuration |

---

## Trigger types

### `message_posted`

Fires when a new message is posted in the specified channel.

---

## Action types

### `send_message`

Posts a message to a channel as the plugin bot user.

The `body`, `channel_id`, and `reply_to_post_id` fields are rendered as Go templates with the flow context.

### `ai_prompt`

Sends a rendered prompt to an AI agent or service via the Mattermost AI plugin bridge and stores the response.

Requires the AI plugin (`mattermost-plugin-ai`) to be installed and active.

**Required `config` keys:**

| Key | Type | Description |
|-----|------|-------------|
| `prompt` | string | The prompt template (Go `text/template` syntax) |
| `provider_type` | string | Either `"agent"` or `"service"` |
| `provider_id` | string | ID of the agent or service to use |

---

## Template context

Action templates receive a `FlowContext` object with the following structure:

```
{{.Trigger}}        — trigger event data
{{.Trigger.Post}}   — the post that triggered the flow
{{.Trigger.Channel}} — the channel where the event occurred
{{.Trigger.User}}   — the user who triggered the event
{{.Steps.<action_id>}} — output from a previous action step
```

### Trigger data fields

**Post:**

| Field | Access |
|-------|--------|
| ID | `{{.Trigger.Post.Id}}` |
| Channel ID | `{{.Trigger.Post.ChannelId}}` |
| Message | `{{.Trigger.Post.Message}}` |

**Channel:**

| Field | Access |
|-------|--------|
| ID | `{{.Trigger.Channel.Id}}` |
| Name | `{{.Trigger.Channel.Name}}` |
| Display Name | `{{.Trigger.Channel.DisplayName}}` |

**User:**

| Field | Access |
|-------|--------|
| ID | `{{.Trigger.User.Id}}` |
| Username | `{{.Trigger.User.Username}}` |
| First Name | `{{.Trigger.User.FirstName}}` |
| Last Name | `{{.Trigger.User.LastName}}` |

### Step output fields

Previous action outputs are available via `{{.Steps.<action_id>}}`:

| Field | Access |
|-------|--------|
| Post ID | `{{.Steps.<action_id>.PostID}}` |
| Channel ID | `{{.Steps.<action_id>.ChannelID}}` |
| Message | `{{.Steps.<action_id>.Message}}` |
