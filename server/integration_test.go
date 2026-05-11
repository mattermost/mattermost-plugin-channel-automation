package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/mattermost/mattermost-plugin-starter-template/server/testhelper"
	mmmodel "github.com/mattermost/mattermost/server/public/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

// pluginURL builds the full URL for a plugin API endpoint.
func pluginURL(serverURL, path string) string {
	return serverURL + "/plugins/com.mattermost.channel-automation/api/v1" + path
}

// doRequest constructs and executes an HTTP request with an optional auth token and body.
func doRequest(t *testing.T, method, url, authToken string, body any) *http.Response {
	t.Helper()

	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, url, bodyReader)
	require.NoError(t, err)

	if authToken != "" {
		req.Header.Set("Authorization", "Bearer "+authToken)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)

	return resp
}

// decodeAutomation reads and decodes a single Automation from the response body.
func decodeAutomation(t *testing.T, resp *http.Response) model.Automation {
	t.Helper()
	defer func() { _ = resp.Body.Close() }()

	var f model.Automation
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&f))
	return f
}

// decodeAutomations reads and decodes a slice of Automations from the response body.
func decodeAutomations(t *testing.T, resp *http.Response) []*model.Automation {
	t.Helper()
	defer func() { _ = resp.Body.Close() }()

	var automations []*model.Automation
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&automations))
	return automations
}

// validAutomationBody returns a minimal valid automation creation body using a message_posted trigger.
func validAutomationBody(channelID string) model.Automation {
	return model.Automation{
		Name:    "test-automation",
		Enabled: true,
		Trigger: model.Trigger{
			MessagePosted: &model.MessagePostedConfig{
				ChannelID: channelID,
			},
		},
		Actions: []model.Action{
			{
				ID: "send-greeting",
				SendMessage: &model.SendMessageActionConfig{
					ChannelID: channelID,
					Body:      "Hello!",
				},
			},
		},
	}
}

// deleteAutomation is a helper that deletes a automation and asserts success.
func deleteAutomation(t *testing.T, serverURL, token, automationID string) {
	t.Helper()
	resp := doRequest(t, http.MethodDelete, pluginURL(serverURL, "/automations/"+automationID), token, nil)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
}

// TestIntegration runs all integration tests as subtests sharing a single
// testhelper.Setup call. This avoids repeated database resets between tests
// which are unreliable with the current testhelper implementation.
func TestIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	th := testhelper.Setup(t)

	t.Run("PluginActivation", func(t *testing.T) {
		pluginStatuses, _, err := th.AdminClient.GetPluginStatuses(context.Background())
		require.NoError(t, err)

		var found bool
		for _, ps := range pluginStatuses {
			if ps.PluginId == testhelper.PluginID() {
				assert.Equal(t, mmmodel.PluginStateRunning, ps.State)
				found = true
				break
			}
		}
		require.True(t, found, "plugin %s not found in plugin statuses", testhelper.PluginID())
	})

	t.Run("ListAutomations_Empty", func(t *testing.T) {
		token := th.AdminClient.AuthToken

		resp := doRequest(t, http.MethodGet, pluginURL(th.ServerURL, "/automations"), token, nil)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		automations := decodeAutomations(t, resp)
		require.NotNil(t, automations, "empty list should be [], not null")
		assert.Empty(t, automations)
	})

	t.Run("GetAutomation_NotFound", func(t *testing.T) {
		token := th.AdminClient.AuthToken

		resp := doRequest(t, http.MethodGet, pluginURL(th.ServerURL, "/automations/nonexistent-id"), token, nil)
		_ = resp.Body.Close()
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("DeleteAutomation_NotFound", func(t *testing.T) {
		token := th.AdminClient.AuthToken

		resp := doRequest(t, http.MethodDelete, pluginURL(th.ServerURL, "/automations/nonexistent-id"), token, nil)
		_ = resp.Body.Close()
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("CreateAutomation_Unauthenticated", func(t *testing.T) {
		body := validAutomationBody(th.Channel.Id)
		resp := doRequest(t, http.MethodPost, pluginURL(th.ServerURL, "/automations"), "", body)
		_ = resp.Body.Close()
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("CreateAutomation_ValidationErrors", func(t *testing.T) {
		token := th.AdminClient.AuthToken

		t.Run("empty trigger", func(t *testing.T) {
			body := model.Automation{
				Name:    "bad-trigger",
				Trigger: model.Trigger{},
				Actions: []model.Action{
					{
						ID: "act-one",
						SendMessage: &model.SendMessageActionConfig{
							ChannelID: th.Channel.Id,
							Body:      "hi",
						},
					},
				},
			}
			resp := doRequest(t, http.MethodPost, pluginURL(th.ServerURL, "/automations"), token, body)
			_ = resp.Body.Close()
			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		})

		t.Run("missing action ID", func(t *testing.T) {
			body := model.Automation{
				Name: "no-action-id",
				Trigger: model.Trigger{
					MessagePosted: &model.MessagePostedConfig{ChannelID: th.Channel.Id},
				},
				Actions: []model.Action{
					{
						SendMessage: &model.SendMessageActionConfig{
							ChannelID: th.Channel.Id,
							Body:      "hi",
						},
					},
				},
			}
			resp := doRequest(t, http.MethodPost, pluginURL(th.ServerURL, "/automations"), token, body)
			_ = resp.Body.Close()
			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		})

		t.Run("invalid action ID", func(t *testing.T) {
			body := model.Automation{
				Name: "bad-action-id",
				Trigger: model.Trigger{
					MessagePosted: &model.MessagePostedConfig{ChannelID: th.Channel.Id},
				},
				Actions: []model.Action{
					{
						ID: "Invalid_ID!",
						SendMessage: &model.SendMessageActionConfig{
							ChannelID: th.Channel.Id,
							Body:      "hi",
						},
					},
				},
			}
			resp := doRequest(t, http.MethodPost, pluginURL(th.ServerURL, "/automations"), token, body)
			_ = resp.Body.Close()
			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		})

		t.Run("duplicate action IDs", func(t *testing.T) {
			body := model.Automation{
				Name: "dup-ids",
				Trigger: model.Trigger{
					MessagePosted: &model.MessagePostedConfig{ChannelID: th.Channel.Id},
				},
				Actions: []model.Action{
					{
						ID: "same-id",
						SendMessage: &model.SendMessageActionConfig{
							ChannelID: th.Channel.Id,
							Body:      "first",
						},
					},
					{
						ID: "same-id",
						SendMessage: &model.SendMessageActionConfig{
							ChannelID: th.Channel.Id,
							Body:      "second",
						},
					},
				},
			}
			resp := doRequest(t, http.MethodPost, pluginURL(th.ServerURL, "/automations"), token, body)
			_ = resp.Body.Close()
			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		})

		t.Run("schedule missing interval", func(t *testing.T) {
			body := model.Automation{
				Name: "no-interval",
				Trigger: model.Trigger{
					Schedule: &model.ScheduleConfig{
						ChannelID: th.Channel.Id,
					},
				},
				Actions: []model.Action{
					{
						ID: "act-one",
						SendMessage: &model.SendMessageActionConfig{
							ChannelID: th.Channel.Id,
							Body:      "hi",
						},
					},
				},
			}
			resp := doRequest(t, http.MethodPost, pluginURL(th.ServerURL, "/automations"), token, body)
			_ = resp.Body.Close()
			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		})

		t.Run("schedule interval too small", func(t *testing.T) {
			body := model.Automation{
				Name: "short-interval",
				Trigger: model.Trigger{
					Schedule: &model.ScheduleConfig{
						ChannelID: th.Channel.Id,
						Interval:  "30m",
					},
				},
				Actions: []model.Action{
					{
						ID: "act-one",
						SendMessage: &model.SendMessageActionConfig{
							ChannelID: th.Channel.Id,
							Body:      "hi",
						},
					},
				},
			}
			resp := doRequest(t, http.MethodPost, pluginURL(th.ServerURL, "/automations"), token, body)
			_ = resp.Body.Close()
			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		})
	})

	t.Run("AutomationCRUDLifecycle", func(t *testing.T) {
		token := th.AdminClient.AuthToken

		// Create
		body := validAutomationBody(th.Channel.Id)
		resp := doRequest(t, http.MethodPost, pluginURL(th.ServerURL, "/automations"), token, body)
		require.Equal(t, http.StatusCreated, resp.StatusCode)
		created := decodeAutomation(t, resp)

		assert.NotEmpty(t, created.ID)
		assert.Equal(t, "test-automation", created.Name)
		assert.True(t, created.Enabled)
		assert.NotZero(t, created.CreatedAt)
		assert.Equal(t, created.CreatedAt, created.UpdatedAt)
		assert.Equal(t, th.AdminUser.Id, created.CreatedBy)
		assert.Equal(t, "send-greeting", created.Actions[0].ID)

		// Get
		resp = doRequest(t, http.MethodGet, pluginURL(th.ServerURL, "/automations/"+created.ID), token, nil)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		fetched := decodeAutomation(t, resp)
		assert.Equal(t, created.ID, fetched.ID)
		assert.Equal(t, created.CreatedAt, fetched.CreatedAt)

		// List
		resp = doRequest(t, http.MethodGet, pluginURL(th.ServerURL, "/automations"), token, nil)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		automations := decodeAutomations(t, resp)
		require.Len(t, automations, 1)
		assert.Equal(t, created.ID, automations[0].ID)

		// Update
		updated := body
		updated.Name = "updated-automation"
		updated.Actions = []model.Action{
			{
				ID: "send-farewell",
				SendMessage: &model.SendMessageActionConfig{
					ChannelID: th.Channel.Id,
					Body:      "Goodbye!",
				},
			},
		}
		resp = doRequest(t, http.MethodPut, pluginURL(th.ServerURL, "/automations/"+created.ID), token, updated)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		got := decodeAutomation(t, resp)
		assert.Equal(t, created.ID, got.ID)
		assert.Equal(t, "updated-automation", got.Name)
		assert.Equal(t, created.CreatedAt, got.CreatedAt, "CreatedAt must be immutable")
		assert.Equal(t, created.CreatedBy, got.CreatedBy, "CreatedBy must be immutable")
		assert.GreaterOrEqual(t, got.UpdatedAt, created.UpdatedAt)
		assert.Equal(t, "send-farewell", got.Actions[0].ID)

		// Delete
		deleteAutomation(t, th.ServerURL, token, created.ID)

		// Verify deletion
		resp = doRequest(t, http.MethodGet, pluginURL(th.ServerURL, "/automations/"+created.ID), token, nil)
		_ = resp.Body.Close()
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("CreateAutomation_ScheduleTrigger", func(t *testing.T) {
		token := th.AdminClient.AuthToken

		body := model.Automation{
			Name:    "scheduled-automation",
			Enabled: true,
			Trigger: model.Trigger{
				Schedule: &model.ScheduleConfig{
					ChannelID: th.Channel.Id,
					Interval:  "1h",
				},
			},
			Actions: []model.Action{
				{
					ID: "send-update",
					SendMessage: &model.SendMessageActionConfig{
						ChannelID: th.Channel.Id,
						Body:      "Scheduled update",
					},
				},
			},
		}

		resp := doRequest(t, http.MethodPost, pluginURL(th.ServerURL, "/automations"), token, body)
		require.Equal(t, http.StatusCreated, resp.StatusCode)
		created := decodeAutomation(t, resp)

		assert.NotEmpty(t, created.ID)
		assert.Equal(t, "scheduled-automation", created.Name)
		require.NotNil(t, created.Trigger.Schedule)
		assert.Equal(t, "1h", created.Trigger.Schedule.Interval)
		assert.Equal(t, th.Channel.Id, created.Trigger.Schedule.ChannelID)

		deleteAutomation(t, th.ServerURL, token, created.ID)
	})

	t.Run("Permissions_ChannelAdminAllowed", func(t *testing.T) {
		// Create a regular user and log them in to get a token.
		user := th.CreateUser()
		client := mmmodel.NewAPIv4Client(th.ServerURL)
		_, _, err := client.Login(context.Background(), user.Username, "Password1!")
		require.NoError(t, err)
		token := client.AuthToken

		// Add the user to the channel, then promote to channel admin.
		_, _, err = th.AdminClient.AddChannelMember(context.Background(), th.Channel.Id, user.Id)
		require.NoError(t, err)
		_, err = th.AdminClient.UpdateChannelMemberSchemeRoles(context.Background(), th.Channel.Id, user.Id, &mmmodel.SchemeRoles{
			SchemeUser:  true,
			SchemeAdmin: true,
		})
		require.NoError(t, err)

		body := validAutomationBody(th.Channel.Id)
		resp := doRequest(t, http.MethodPost, pluginURL(th.ServerURL, "/automations"), token, body)
		require.Equal(t, http.StatusCreated, resp.StatusCode)
		created := decodeAutomation(t, resp)
		assert.NotEmpty(t, created.ID)
		assert.Equal(t, user.Id, created.CreatedBy)

		deleteAutomation(t, th.ServerURL, th.AdminClient.AuthToken, created.ID)
	})

	t.Run("Permissions_NonAdminDenied", func(t *testing.T) {
		// Create a regular user (not channel admin) and log them in.
		user := th.CreateUser()
		client := mmmodel.NewAPIv4Client(th.ServerURL)
		_, _, err := client.Login(context.Background(), user.Username, "Password1!")
		require.NoError(t, err)
		token := client.AuthToken

		// User is a team member but NOT a channel admin. The automation references
		// the test channel, so permission check should fail.
		body := validAutomationBody(th.Channel.Id)
		resp := doRequest(t, http.MethodPost, pluginURL(th.ServerURL, "/automations"), token, body)
		_ = resp.Body.Close()
		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})
}
