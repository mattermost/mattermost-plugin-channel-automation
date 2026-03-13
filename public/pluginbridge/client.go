package pluginbridge

import "net/http"

const pluginID = "com.mattermost.channel-automation"

// PluginAPI is the subset of the Mattermost plugin API needed by the client.
type PluginAPI interface {
	PluginHTTP(request *http.Request) *http.Response
}

// ClientOption configures a Client.
type ClientOption func(*Client)

// WithUser sets the user ID that the client will use for all requests.
func WithUser(userID string) ClientOption {
	return func(c *Client) {
		c.userID = userID
	}
}

// Client provides typed access to the Channel Automation plugin API
// via inter-plugin HTTP calls.
type Client struct {
	httpClient *http.Client
	userID     string
}

// NewClient creates a new Client that routes requests through the given
// plugin API. Use WithUser to bind the client to a specific user.
func NewClient(api PluginAPI, opts ...ClientOption) *Client {
	c := &Client{
		httpClient: &http.Client{
			Transport: &pluginAPIRoundTripper{api: api},
		},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// AsUser returns a shallow copy of the client that makes requests on
// behalf of a different user. The underlying HTTP transport is shared.
func (c *Client) AsUser(userID string) *Client {
	return &Client{
		httpClient: c.httpClient,
		userID:     userID,
	}
}
