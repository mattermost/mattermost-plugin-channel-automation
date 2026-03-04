package pluginbridge

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockPluginAPI records the request and returns a pre-built response.
type mockPluginAPI struct {
	lastRequest *http.Request
	response    *http.Response
}

func (m *mockPluginAPI) PluginHTTP(req *http.Request) *http.Response {
	m.lastRequest = req
	return m.response
}

func jsonResponse(statusCode int, body any) *http.Response {
	data, _ := json.Marshal(body)
	return &http.Response{
		StatusCode: statusCode,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(data)),
	}
}

func textResponse(statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Header:     http.Header{"Content-Type": []string{"text/plain"}},
		Body:       io.NopCloser(bytes.NewBufferString(body)),
	}
}

func sampleFlow() *Flow {
	return &Flow{
		ID:      "flow123",
		Name:    "test flow",
		Enabled: true,
		Trigger: Trigger{
			MessagePosted: &MessagePostedConfig{ChannelID: "ch1"},
		},
		Actions: []Action{
			{
				ID:          "act1",
				SendMessage: &SendMessageActionConfig{ChannelID: "ch2", Body: "hello"},
			},
		},
	}
}

func TestListFlows(t *testing.T) {
	flows := []*Flow{sampleFlow()}
	mock := &mockPluginAPI{response: jsonResponse(http.StatusOK, flows)}
	client := NewClient(mock)

	result, err := client.ListFlows("user1")
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, "flow123", result[0].ID)
	assert.Equal(t, "test flow", result[0].Name)

	assert.Equal(t, http.MethodGet, mock.lastRequest.Method)
	assert.Equal(t, "/plugins/com.mattermost.channel-automation/api/v1/flows", mock.lastRequest.URL.Path)
	assert.Equal(t, "user1", mock.lastRequest.Header.Get("Mattermost-User-ID"))
}

func TestGetFlow(t *testing.T) {
	flow := sampleFlow()
	mock := &mockPluginAPI{response: jsonResponse(http.StatusOK, flow)}
	client := NewClient(mock)

	result, err := client.GetFlow("user1", "flow123")
	require.NoError(t, err)
	assert.Equal(t, "flow123", result.ID)

	assert.Equal(t, http.MethodGet, mock.lastRequest.Method)
	assert.Equal(t, "/plugins/com.mattermost.channel-automation/api/v1/flows/flow123", mock.lastRequest.URL.Path)
}

func TestCreateFlow(t *testing.T) {
	created := sampleFlow()
	created.CreatedAt = 1000
	created.CreatedBy = "user1"
	mock := &mockPluginAPI{response: jsonResponse(http.StatusCreated, created)}
	client := NewClient(mock)

	input := &Flow{
		Name:    "test flow",
		Enabled: true,
		Trigger: Trigger{
			MessagePosted: &MessagePostedConfig{ChannelID: "ch1"},
		},
	}
	result, err := client.CreateFlow("user1", input)
	require.NoError(t, err)
	assert.Equal(t, "flow123", result.ID)
	assert.Equal(t, int64(1000), result.CreatedAt)

	assert.Equal(t, http.MethodPost, mock.lastRequest.Method)
	assert.Equal(t, "/plugins/com.mattermost.channel-automation/api/v1/flows", mock.lastRequest.URL.Path)
	assert.Equal(t, "application/json", mock.lastRequest.Header.Get("Content-Type"))
}

func TestUpdateFlow(t *testing.T) {
	updated := sampleFlow()
	updated.UpdatedAt = 2000
	mock := &mockPluginAPI{response: jsonResponse(http.StatusOK, updated)}
	client := NewClient(mock)

	input := sampleFlow()
	result, err := client.UpdateFlow("user1", input)
	require.NoError(t, err)
	assert.Equal(t, int64(2000), result.UpdatedAt)

	assert.Equal(t, http.MethodPut, mock.lastRequest.Method)
	assert.Equal(t, "/plugins/com.mattermost.channel-automation/api/v1/flows/flow123", mock.lastRequest.URL.Path)
}

func TestUpdateFlowEmptyID(t *testing.T) {
	client := NewClient(&mockPluginAPI{})

	_, err := client.UpdateFlow("user1", &Flow{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "flow ID must be set")
}

func TestDeleteFlow(t *testing.T) {
	mock := &mockPluginAPI{response: &http.Response{
		StatusCode: http.StatusNoContent,
		Body:       io.NopCloser(bytes.NewReader(nil)),
	}}
	client := NewClient(mock)

	err := client.DeleteFlow("user1", "flow123")
	require.NoError(t, err)

	assert.Equal(t, http.MethodDelete, mock.lastRequest.Method)
	assert.Equal(t, "/plugins/com.mattermost.channel-automation/api/v1/flows/flow123", mock.lastRequest.URL.Path)
}

func TestErrorNotFound(t *testing.T) {
	mock := &mockPluginAPI{response: textResponse(http.StatusNotFound, "flow not found\n")}
	client := NewClient(mock)

	_, err := client.GetFlow("user1", "missing")
	require.Error(t, err)
	assert.True(t, IsNotFound(err))
	assert.False(t, IsForbidden(err))
	assert.False(t, IsBadRequest(err))

	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusNotFound, apiErr.StatusCode)
	assert.Equal(t, "flow not found", apiErr.Message)
}

func TestErrorForbidden(t *testing.T) {
	mock := &mockPluginAPI{response: textResponse(http.StatusForbidden, "you do not have channel admin permissions on channel ch1\n")}
	client := NewClient(mock)

	_, err := client.CreateFlow("user1", sampleFlow())
	require.Error(t, err)
	assert.True(t, IsForbidden(err))
	assert.False(t, IsNotFound(err))
}

func TestErrorBadRequest(t *testing.T) {
	mock := &mockPluginAPI{response: textResponse(http.StatusBadRequest, "invalid request body\n")}
	client := NewClient(mock)

	_, err := client.CreateFlow("user1", sampleFlow())
	require.Error(t, err)
	assert.True(t, IsBadRequest(err))
}

func TestNilResponse(t *testing.T) {
	mock := &mockPluginAPI{response: nil}
	client := NewClient(mock)

	_, err := client.ListFlows("user1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "PluginHTTP returned nil response")
}
