package pluginbridge

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const basePath = "/plugins/" + pluginID + "/api/v1"

func (c *Client) newRequest(method, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, "http://localhost"+basePath+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Mattermost-User-ID", c.userID)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

func checkResponse(resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	body, _ := io.ReadAll(resp.Body)
	return &APIError{
		StatusCode: resp.StatusCode,
		Message:    strings.TrimSpace(string(body)),
	}
}

// ListAutomationsOptions contains optional filters for ListAutomations.
type ListAutomationsOptions struct {
	// ChannelID filters automations to those triggered by the given channel.
	ChannelID string
}

// ListAutomations returns all automations visible to the caller.
// Use the opts fields to filter the results (pass an empty struct for no filters).
func (c *Client) ListAutomations(opts ListAutomationsOptions) ([]*Automation, error) {
	path := "/automations"
	if opts.ChannelID != "" {
		path += "?channel_id=" + url.QueryEscape(opts.ChannelID)
	}

	req, err := c.newRequest(http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if err := checkResponse(resp); err != nil {
		return nil, err
	}

	var automations []*Automation
	if err := json.NewDecoder(resp.Body).Decode(&automations); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return automations, nil
}

// GetAutomation returns a single automation by ID.
func (c *Client) GetAutomation(automationID string) (*Automation, error) {
	if automationID == "" {
		return nil, fmt.Errorf("automation ID must not be empty")
	}
	req, err := c.newRequest(http.MethodGet, "/automations/"+url.PathEscape(automationID), nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if err := checkResponse(resp); err != nil {
		return nil, err
	}

	var automation Automation
	if err := json.NewDecoder(resp.Body).Decode(&automation); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &automation, nil
}

// CreateAutomation creates a new automation and returns the created automation with server-assigned fields.
func (c *Client) CreateAutomation(automation *Automation) (*Automation, error) {
	if automation == nil {
		return nil, fmt.Errorf("automation must not be nil")
	}
	body, err := json.Marshal(automation)
	if err != nil {
		return nil, fmt.Errorf("encoding request: %w", err)
	}

	req, err := c.newRequest(http.MethodPost, "/automations", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if err := checkResponse(resp); err != nil {
		return nil, err
	}

	var created Automation
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &created, nil
}

// UpdateAutomation updates an existing automation. The automation's ID field must be set.
func (c *Client) UpdateAutomation(automation *Automation) (*Automation, error) {
	if automation == nil {
		return nil, fmt.Errorf("automation must not be nil")
	}
	if automation.ID == "" {
		return nil, fmt.Errorf("automation ID must be set for update")
	}

	body, err := json.Marshal(automation)
	if err != nil {
		return nil, fmt.Errorf("encoding request: %w", err)
	}

	req, err := c.newRequest(http.MethodPut, "/automations/"+url.PathEscape(automation.ID), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if err := checkResponse(resp); err != nil {
		return nil, err
	}

	var updated Automation
	if err := json.NewDecoder(resp.Body).Decode(&updated); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &updated, nil
}

// DeleteAutomation deletes an automation by ID.
func (c *Client) DeleteAutomation(automationID string) error {
	if automationID == "" {
		return fmt.Errorf("automation ID must not be empty")
	}
	req, err := c.newRequest(http.MethodDelete, "/automations/"+url.PathEscape(automationID), nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("executing request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	return checkResponse(resp)
}
