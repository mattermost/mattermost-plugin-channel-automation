# Channel Automation Plugin for Mattermost

[![Build Status](https://github.com/mattermost/mattermost-plugin-channel-automation/actions/workflows/ci.yml/badge.svg)](https://github.com/mattermost/mattermost-plugin-channel-automation/actions/workflows/ci.yml)
[![E2E Status](https://github.com/mattermost/mattermost-plugin-channel-automation/actions/workflows/e2e.yml/badge.svg)](https://github.com/mattermost/mattermost-plugin-channel-automation/actions/workflows/e2e.yml)

A Mattermost plugin that lets system admins build automated workflows triggered by channel events. Define flows that react to messages, post responses, and optionally call AI agents — all configured through a built-in management UI.

## Features

- **Flow-based automation** — Create named flows with a trigger and a sequence of actions that execute in order.
- **Go template engine** — Action fields support `text/template` syntax with access to trigger context (post, channel, user) and outputs from previous actions.
- **Persistent work queue** — Flow executions are durably queued in the KV store with bounded concurrency and automatic crash recovery.
- **Failure notifications** — When an action in a flow fails (e.g. an `ai_prompt` returns an error), the plugin DMs the flow's creator with the failing action ID and error message, rate-limited to once per hour per flow and coordinated cluster-wide via the KV store.
- **Management UI** — A dedicated webapp section for listing, creating, editing, enabling/disabling, and deleting flows.

### Triggers

- **Message Posted** — Fire a flow when a new message appears in a specific channel. Bot posts, system messages, and webhook posts are automatically excluded.
- **Schedule** — Fire a flow on a recurring interval (e.g. every 30 minutes, every 24 hours).
- **Membership Changed** — Fire a flow when a user joins or leaves a specific channel.
- **Channel Created** — Fire a flow when a new public channel is created anywhere on the server.

### Actions

- **Send Message** — Post a message as the plugin bot, with optional threading support.
- **AI Prompt** — Send a rendered prompt to an AI agent provided by the [Mattermost AI Plugin](https://github.com/mattermost/mattermost-plugin-ai) and capture the response.

## Getting Started

### Installation

1. Download the latest release from the [Releases](https://github.com/mattermost/mattermost-plugin-channel-automation/releases) page.
2. Upload the `.tar.gz` file through **System Console > Plugins > Plugin Management**.
3. Enable the plugin.

### Creating a Flow

1. Open the **Channel Automation** section from the product menu.
2. Click **Create Flow**.
3. Give the flow a name, select a trigger type and channel, then add one or more actions.
4. Save and enable the flow.

### Example: Echo Bot

A simple flow that replies in-thread whenever someone posts in a channel:

| Field            | Value                                     |
| ---------------- | ----------------------------------------- |
| **Trigger**      | `message_posted` on channel `town-square` |
| **Action 1**     | `send_message`                            |
| Channel ID       | `{{.Trigger.Channel.Id}}`                 |
| Reply To Post ID | `{{.Trigger.Post.Id}}`                    |
| Body             | `Echo: {{.Trigger.Post.Message}}`         |

### Example: AI Triage

A flow that asks an AI agent to classify incoming messages and posts the result:

| Field                     | Value                                                                                       |
| ------------------------- | ------------------------------------------------------------------------------------------- |
| **Trigger**               | `message_posted` on a support channel                                                       |
| **Action 1** (`classify`) | `ai_prompt` — Agent: your-agent, Prompt: `Classify this message: {{.Trigger.Post.Message}}` |
| **Action 2**              | `send_message` — Body: `Classification: {{(index .Steps "classify").Message}}`              |

## Trigger Types

| Type                 | Description                                                                                                             |
| -------------------- | ----------------------------------------------------------------------------------------------------------------------- |
| `message_posted`     | Fires when a user posts a message in the configured channel. Bot posts, system messages, and webhook posts are ignored. |
| `schedule`           | Fires on a recurring interval. Minimum interval is 1 hour.                                                              |
| `membership_changed` | Fires when a user joins or leaves the configured channel. Bot users are excluded.                                       |
| `channel_created`    | Fires when a new public channel is created. No channel configuration needed.                                            |

## Action Types

| Type           | Description                                                                                                                           |
| -------------- | ------------------------------------------------------------------------------------------------------------------------------------- |
| `send_message` | Posts a message as the plugin bot. Supports `channel_id`, `reply_to_post_id`, `as_bot_id`, and `body` — all templated.                |
| `ai_prompt`    | Sends a rendered prompt to an AI agent via the Mattermost AI Plugin. Requires `provider_type` and `provider_id` in the action config. |

## Template Context

All action fields that support templates receive a `FlowContext` with:

| Variable                                   | Description                                         |
| ------------------------------------------ | --------------------------------------------------- |
| `{{.Trigger.Post.Id}}`                     | ID of the triggering post                           |
| `{{.Trigger.Post.Message}}`                | Message text                                        |
| `{{.Trigger.Post.ChannelId}}`              | Channel where the post was created                  |
| `{{.Trigger.Post.ThreadId}}`               | Thread/root post ID                                 |
| `{{.Trigger.Channel.Id}}`                  | Channel ID                                          |
| `{{.Trigger.Channel.Name}}`                | Channel name                                        |
| `{{.Trigger.Channel.DisplayName}}`         | Channel display name                                |
| `{{.Trigger.User.Id}}`                     | User ID of the post author                          |
| `{{.Trigger.User.Username}}`               | Username                                            |
| `{{.Trigger.User.FirstName}}`              | First name                                          |
| `{{.Trigger.User.LastName}}`               | Last name                                           |
| `{{.CreatedBy}}`                           | User ID of the flow creator                         |
| `{{(index .Steps "<action_id>").Message}}` | Output message from a previous action               |
| `{{(index .Steps "<action_id>").PostID}}`  | Post ID created by a previous `send_message` action |
| `{{(index .Steps "<action_id>").Truncated}}`| Whether the output message was truncated           |

Sensitive user fields (email, password, auth data) are stripped from the template context. Nickname is not available.

## Configuration

| Setting                    | Default | Description                                                                                                 |
| -------------------------- | ------- | ----------------------------------------------------------------------------------------------------------- |
| **Max Concurrent Flows**   | `4`     | Maximum flow executions running concurrently per plugin instance. Requires a plugin restart to take effect. |
| **Max Flows Per Channel**  | `0`     | Maximum number of flows that can target a single channel. Set to 0 for unlimited.                           |

## API

The plugin exposes a REST API under `/plugins/com.mattermost.channel-automation/api/v1`. System admins are always allowed. Otherwise the user must be a channel admin on every channel referenced by the flow.

| Method   | Path                            | Description                         |
| -------- | ------------------------------- | ----------------------------------- |
| `GET`    | `/flows`                        | List all flows                      |
| `POST`   | `/flows`                        | Create a new flow                   |
| `GET`    | `/flows/{id}`                   | Get a flow by ID                    |
| `PUT`    | `/flows/{id}`                   | Update a flow                       |
| `DELETE` | `/flows/{id}`                   | Delete a flow                       |
| `GET`    | `/flows/{flow_id}/executions`   | List executions for a flow          |
| `GET`    | `/executions/{id}`              | Get a single execution record       |
| `GET`    | `/executions`                   | List recent executions (admin only) |

See [docs/api.md](docs/api.md) for the full API reference with request/response schemas.

## Development

### Prerequisites

- Go
- Node.js (see `.nvmrc`)
- Make

### Building

```bash
make all        # lint + test + build
make dist       # build plugin bundle only
make check-style # run all linters
make test       # run all tests
```

### Deploying locally

Enable plugin uploads and optionally [local mode](https://docs.mattermost.com/administration/mmctl-cli-tool.html#local-mode), then:

```bash
make deploy
```

Or with credentials:

```bash
export MM_SERVICESETTINGS_SITEURL=http://localhost:8065
export MM_ADMIN_TOKEN=<your-token>
make deploy
```

To watch for webapp changes and auto-deploy:

```bash
export MM_SERVICESETTINGS_SITEURL=http://localhost:8065
export MM_ADMIN_TOKEN=<your-token>
make watch
```

### Releasing

Versions are determined at compile time from git tags. To cut a release:

```bash
make patch       # patch release (e.g. 1.0.1)
make minor       # minor release (e.g. 1.1.0)
make major       # major release (e.g. 2.0.0)
```

Append `-rc` for release candidates (`make patch-rc`, `make minor-rc`, `make major-rc`).
