# pluginbridge

`pluginbridge` provides a typed Go client for the Channel Automation plugin API.
It is designed for use by other Mattermost plugins that need to create, read,
update, or delete automations via inter-plugin HTTP calls.

## Usage

```go
import "github.com/mattermost/mattermost-plugin-channel-automation/public/pluginbridge"

// Create a client bound to a specific user.
client := pluginbridge.NewClient(pluginAPI, pluginbridge.WithUser("user-id"))

// List all automations (no filters).
automations, err := client.ListAutomations(pluginbridge.ListAutomationsOptions{})

// List automations filtered by channel.
automations, err := client.ListAutomations(pluginbridge.ListAutomationsOptions{
    ChannelID: "channel-id",
})

// CRUD operations.
created, err     := client.CreateAutomation(&pluginbridge.Automation{ /* ... */ })
automation, err  := client.GetAutomation("automation-id")
updated, err     := client.UpdateAutomation(automation)
err               = client.DeleteAutomation("automation-id")
```

### Switching users

`AsUser` returns a copy of the client that makes requests on
behalf of a different user. The underlying HTTP transport is shared.

```go
admin := pluginbridge.NewClient(pluginAPI, pluginbridge.WithUser(adminID))
other := admin.AsUser(regularUserID)

// 'other' makes requests as regularUserID.
automations, err := other.ListAutomations(pluginbridge.ListAutomationsOptions{})
```

### Error handling

```go
_, err := client.GetAutomation("missing-id")
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
