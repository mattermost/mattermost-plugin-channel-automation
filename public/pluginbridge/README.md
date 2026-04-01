# pluginbridge

`pluginbridge` provides a typed Go client for the Channel Automation plugin API.
It is designed for use by other Mattermost plugins that need to create, read,
update, or delete automation flows via inter-plugin HTTP calls.

## Usage

```go
import "github.com/mattermost/mattermost-plugin-channel-automation/public/pluginbridge"

// Create a client bound to a specific user.
client := pluginbridge.NewClient(pluginAPI, pluginbridge.WithUser("user-id"))

// List all flows (no filters).
flows, err := client.ListFlows(pluginbridge.ListFlowsOptions{})

// List flows filtered by channel.
flows, err := client.ListFlows(pluginbridge.ListFlowsOptions{
    ChannelID: "channel-id",
})

// CRUD operations.
// On `ai_prompt` actions, optional `MattermostAccessScope` maps to API JSON `mattermost_access_scope`
// (team/channel guardrails for the AI plugin bridge).
created, err := client.CreateFlow(&pluginbridge.Flow{ /* ... */ })
flow, err    := client.GetFlow("flow-id")
updated, err := client.UpdateFlow(flow)
err           = client.DeleteFlow("flow-id")
```

### Switching users

`AsUser` returns a copy of the client that makes requests on
behalf of a different user. The underlying HTTP transport is shared.

```go
admin := pluginbridge.NewClient(pluginAPI, pluginbridge.WithUser(adminID))
other := admin.AsUser(regularUserID)

// 'other' makes requests as regularUserID.
flows, err := other.ListFlows(pluginbridge.ListFlowsOptions{})
```

### Error handling

```go
_, err := client.GetFlow("missing-id")
if pluginbridge.IsNotFound(err) {
    // handle 404
}
if pluginbridge.IsForbidden(err) {
    // handle 403
}
if pluginbridge.IsBadRequest(err) {
    // handle 400
}
```
