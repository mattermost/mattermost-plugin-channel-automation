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

func sampleAutomation() *Automation {
	return &Automation{
		ID:      "automation123",
		Name:    "test automation",
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

func TestListAutomations(t *testing.T) {
	automations := []*Automation{sampleAutomation()}
	mock := &mockPluginAPI{response: jsonResponse(http.StatusOK, automations)}
	client := NewClient(mock, WithUser("user1"))

	result, err := client.ListAutomations(ListAutomationsOptions{})
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, "automation123", result[0].ID)
	assert.Equal(t, "test automation", result[0].Name)

	assert.Equal(t, http.MethodGet, mock.lastRequest.Method)
	assert.Equal(t, "/plugins/com.mattermost.channel-automation/api/v1/automations", mock.lastRequest.URL.Path)
	assert.Equal(t, "user1", mock.lastRequest.Header.Get("Mattermost-User-ID"))
}

func TestListAutomationsByChannel(t *testing.T) {
	automations := []*Automation{sampleAutomation()}
	mock := &mockPluginAPI{response: jsonResponse(http.StatusOK, automations)}
	client := NewClient(mock, WithUser("user1"))

	result, err := client.ListAutomations(ListAutomationsOptions{ChannelID: "ch1"})
	require.NoError(t, err)
	require.Len(t, result, 1)

	assert.Equal(t, http.MethodGet, mock.lastRequest.Method)
	assert.Equal(t, "/plugins/com.mattermost.channel-automation/api/v1/automations", mock.lastRequest.URL.Path)
	assert.Equal(t, "ch1", mock.lastRequest.URL.Query().Get("channel_id"))
}

func TestGetAutomation(t *testing.T) {
	automation := sampleAutomation()
	mock := &mockPluginAPI{response: jsonResponse(http.StatusOK, automation)}
	client := NewClient(mock, WithUser("user1"))

	result, err := client.GetAutomation("automation123")
	require.NoError(t, err)
	assert.Equal(t, "automation123", result.ID)

	assert.Equal(t, http.MethodGet, mock.lastRequest.Method)
	assert.Equal(t, "/plugins/com.mattermost.channel-automation/api/v1/automations/automation123", mock.lastRequest.URL.Path)
}

func TestCreateAutomation(t *testing.T) {
	created := sampleAutomation()
	created.CreatedAt = 1000
	created.CreatedBy = "user1"
	mock := &mockPluginAPI{response: jsonResponse(http.StatusCreated, created)}
	client := NewClient(mock, WithUser("user1"))

	input := &Automation{
		Name:    "test automation",
		Enabled: true,
		Trigger: Trigger{
			MessagePosted: &MessagePostedConfig{ChannelID: "ch1"},
		},
	}
	result, err := client.CreateAutomation(input)
	require.NoError(t, err)
	assert.Equal(t, "automation123", result.ID)
	assert.Equal(t, int64(1000), result.CreatedAt)

	assert.Equal(t, http.MethodPost, mock.lastRequest.Method)
	assert.Equal(t, "/plugins/com.mattermost.channel-automation/api/v1/automations", mock.lastRequest.URL.Path)
	assert.Equal(t, "application/json", mock.lastRequest.Header.Get("Content-Type"))
}

func TestUpdateAutomation(t *testing.T) {
	updated := sampleAutomation()
	updated.UpdatedAt = 2000
	mock := &mockPluginAPI{response: jsonResponse(http.StatusOK, updated)}
	client := NewClient(mock, WithUser("user1"))

	input := sampleAutomation()
	result, err := client.UpdateAutomation(input)
	require.NoError(t, err)
	assert.Equal(t, int64(2000), result.UpdatedAt)

	assert.Equal(t, http.MethodPut, mock.lastRequest.Method)
	assert.Equal(t, "/plugins/com.mattermost.channel-automation/api/v1/automations/automation123", mock.lastRequest.URL.Path)
}

func TestUpdateAutomationEmptyID(t *testing.T) {
	client := NewClient(&mockPluginAPI{}, WithUser("user1"))

	_, err := client.UpdateAutomation(&Automation{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "automation ID must be set")
}

func TestGetAutomationEmptyID(t *testing.T) {
	client := NewClient(&mockPluginAPI{}, WithUser("user1"))

	_, err := client.GetAutomation("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "automation ID must not be empty")
}

func TestDeleteAutomationEmptyID(t *testing.T) {
	client := NewClient(&mockPluginAPI{}, WithUser("user1"))

	err := client.DeleteAutomation("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "automation ID must not be empty")
}

func TestDeleteAutomation(t *testing.T) {
	mock := &mockPluginAPI{response: &http.Response{
		StatusCode: http.StatusNoContent,
		Body:       io.NopCloser(bytes.NewReader(nil)),
	}}
	client := NewClient(mock, WithUser("user1"))

	err := client.DeleteAutomation("automation123")
	require.NoError(t, err)

	assert.Equal(t, http.MethodDelete, mock.lastRequest.Method)
	assert.Equal(t, "/plugins/com.mattermost.channel-automation/api/v1/automations/automation123", mock.lastRequest.URL.Path)
}

func TestErrorNotFound(t *testing.T) {
	mock := &mockPluginAPI{response: textResponse(http.StatusNotFound, "automation not found\n")}
	client := NewClient(mock, WithUser("user1"))

	_, err := client.GetAutomation("missing")
	require.Error(t, err)
	assert.True(t, IsNotFound(err))
	assert.False(t, IsForbidden(err))
	assert.False(t, IsBadRequest(err))

	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusNotFound, apiErr.StatusCode)
	assert.Equal(t, "automation not found", apiErr.Message)
}

func TestErrorForbidden(t *testing.T) {
	mock := &mockPluginAPI{response: textResponse(http.StatusForbidden, "you do not have channel admin permissions on channel ch1\n")}
	client := NewClient(mock, WithUser("user1"))

	_, err := client.CreateAutomation(sampleAutomation())
	require.Error(t, err)
	assert.True(t, IsForbidden(err))
	assert.False(t, IsNotFound(err))
}

func TestErrorBadRequest(t *testing.T) {
	mock := &mockPluginAPI{response: textResponse(http.StatusBadRequest, "invalid request body\n")}
	client := NewClient(mock, WithUser("user1"))

	_, err := client.CreateAutomation(sampleAutomation())
	require.Error(t, err)
	assert.True(t, IsBadRequest(err))
}

func TestNilResponse(t *testing.T) {
	mock := &mockPluginAPI{response: nil}
	client := NewClient(mock, WithUser("user1"))

	_, err := client.ListAutomations(ListAutomationsOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "PluginHTTP returned nil response")
}

func TestCreateAutomationWithChannelCreatedTrigger(t *testing.T) {
	created := &Automation{
		ID:      "automation456",
		Name:    "channel created automation",
		Enabled: true,
		Trigger: Trigger{
			ChannelCreated: &ChannelCreatedConfig{},
		},
		Actions: []Action{
			{
				ID:          "act1",
				SendMessage: &SendMessageActionConfig{ChannelID: "ch2", Body: "welcome"},
			},
		},
		CreatedAt: 1000,
		CreatedBy: "user1",
	}
	mock := &mockPluginAPI{response: jsonResponse(http.StatusCreated, created)}
	client := NewClient(mock, WithUser("user1"))

	input := &Automation{
		Name:    "channel created automation",
		Enabled: true,
		Trigger: Trigger{
			ChannelCreated: &ChannelCreatedConfig{},
		},
		Actions: []Action{
			{
				ID:          "act1",
				SendMessage: &SendMessageActionConfig{ChannelID: "ch2", Body: "welcome"},
			},
		},
	}
	result, err := client.CreateAutomation(input)
	require.NoError(t, err)
	assert.Equal(t, "automation456", result.ID)
	assert.NotNil(t, result.Trigger.ChannelCreated)
	assert.Nil(t, result.Trigger.MessagePosted)

	// Verify the request body round-trips the channel_created trigger.
	var sent Automation
	require.NoError(t, json.NewDecoder(mock.lastRequest.Body).Decode(&sent))
	assert.NotNil(t, sent.Trigger.ChannelCreated)
}

func TestCreateAutomationWithAsBotID(t *testing.T) {
	created := &Automation{
		ID:      "automation789",
		Name:    "bot automation",
		Enabled: true,
		Trigger: Trigger{
			MessagePosted: &MessagePostedConfig{ChannelID: "ch1"},
		},
		Actions: []Action{
			{
				ID:          "act1",
				SendMessage: &SendMessageActionConfig{ChannelID: "ch2", AsBotID: "bot123", Body: "hello from bot"},
			},
		},
		CreatedAt: 1000,
		CreatedBy: "user1",
	}
	mock := &mockPluginAPI{response: jsonResponse(http.StatusCreated, created)}
	client := NewClient(mock, WithUser("user1"))

	input := &Automation{
		Name:    "bot automation",
		Enabled: true,
		Trigger: Trigger{
			MessagePosted: &MessagePostedConfig{ChannelID: "ch1"},
		},
		Actions: []Action{
			{
				ID:          "act1",
				SendMessage: &SendMessageActionConfig{ChannelID: "ch2", AsBotID: "bot123", Body: "hello from bot"},
			},
		},
	}
	result, err := client.CreateAutomation(input)
	require.NoError(t, err)
	assert.Equal(t, "automation789", result.ID)
	assert.Equal(t, "bot123", result.Actions[0].SendMessage.AsBotID)

	// Verify the request body round-trips the as_bot_id field.
	var sent Automation
	require.NoError(t, json.NewDecoder(mock.lastRequest.Body).Decode(&sent))
	assert.Equal(t, "bot123", sent.Actions[0].SendMessage.AsBotID)
}

func TestAsUser(t *testing.T) {
	mock := &mockPluginAPI{response: jsonResponse(http.StatusOK, []*Automation{})}
	client := NewClient(mock, WithUser("user1"))
	other := client.AsUser("user2")

	_, err := other.ListAutomations(ListAutomationsOptions{})
	require.NoError(t, err)
	assert.Equal(t, "user2", mock.lastRequest.Header.Get("Mattermost-User-ID"))

	// Provide a fresh response for the second call (body is consumed after each request).
	mock.response = jsonResponse(http.StatusOK, []*Automation{})

	// Original client is unaffected.
	_, err = client.ListAutomations(ListAutomationsOptions{})
	require.NoError(t, err)
	assert.Equal(t, "user1", mock.lastRequest.Header.Get("Mattermost-User-ID"))
}
