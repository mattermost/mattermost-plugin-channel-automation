package pluginbridge

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const basePath = "/plugins/" + pluginID + "/api/v1"

func (c *Client) newRequest(method, path, userID string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, "http://localhost"+basePath+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Mattermost-User-ID", userID)
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

// ListFlows returns all flows visible to the given user.
func (c *Client) ListFlows(userID string) ([]*Flow, error) {
	req, err := c.newRequest(http.MethodGet, "/flows", userID, nil)
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

	var flows []*Flow
	if err := json.NewDecoder(resp.Body).Decode(&flows); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return flows, nil
}

// GetFlow returns a single flow by ID.
func (c *Client) GetFlow(userID, flowID string) (*Flow, error) {
	req, err := c.newRequest(http.MethodGet, "/flows/"+flowID, userID, nil)
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

	var flow Flow
	if err := json.NewDecoder(resp.Body).Decode(&flow); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &flow, nil
}

// CreateFlow creates a new flow and returns the created flow with server-assigned fields.
func (c *Client) CreateFlow(userID string, flow *Flow) (*Flow, error) {
	body, err := json.Marshal(flow)
	if err != nil {
		return nil, fmt.Errorf("encoding request: %w", err)
	}

	req, err := c.newRequest(http.MethodPost, "/flows", userID, bytes.NewReader(body))
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

	var created Flow
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &created, nil
}

// UpdateFlow updates an existing flow. The flow's ID field must be set.
func (c *Client) UpdateFlow(userID string, flow *Flow) (*Flow, error) {
	if flow.ID == "" {
		return nil, fmt.Errorf("flow ID must be set for update")
	}

	body, err := json.Marshal(flow)
	if err != nil {
		return nil, fmt.Errorf("encoding request: %w", err)
	}

	req, err := c.newRequest(http.MethodPut, "/flows/"+flow.ID, userID, bytes.NewReader(body))
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

	var updated Flow
	if err := json.NewDecoder(resp.Body).Decode(&updated); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &updated, nil
}

// DeleteFlow deletes a flow by ID.
func (c *Client) DeleteFlow(userID, flowID string) error {
	req, err := c.newRequest(http.MethodDelete, "/flows/"+flowID, userID, nil)
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
