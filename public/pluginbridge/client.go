package pluginbridge

import "net/http"

const pluginID = "com.mattermost.channel-automation"

// PluginAPI is the subset of the Mattermost plugin API needed by the client.
type PluginAPI interface {
	PluginHTTP(request *http.Request) *http.Response
}

// Client provides typed access to the Channel Automation plugin API
// via inter-plugin HTTP calls.
type Client struct {
	httpClient *http.Client
}

// NewClient creates a new Client that routes requests through the given plugin API.
func NewClient(api PluginAPI) *Client {
	return &Client{
		httpClient: &http.Client{
			Transport: &pluginAPIRoundTripper{api: api},
		},
	}
}
