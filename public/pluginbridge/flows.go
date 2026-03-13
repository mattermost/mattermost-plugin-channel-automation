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

// ListFlowsOptions contains optional filters for ListFlows.
type ListFlowsOptions struct {
	// ChannelID filters flows to those triggered by the given channel.
	ChannelID string
}

// ListFlows returns all flows visible to the caller.
// Use the opts fields to filter the results (pass an empty struct for no filters).
func (c *Client) ListFlows(opts ListFlowsOptions) ([]*Flow, error) {
	path := "/flows"
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

	var flows []*Flow
	if err := json.NewDecoder(resp.Body).Decode(&flows); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return flows, nil
}

// GetFlow returns a single flow by ID.
func (c *Client) GetFlow(flowID string) (*Flow, error) {
	if flowID == "" {
		return nil, fmt.Errorf("flow ID must not be empty")
	}
	req, err := c.newRequest(http.MethodGet, "/flows/"+url.PathEscape(flowID), nil)
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
func (c *Client) CreateFlow(flow *Flow) (*Flow, error) {
	if flow == nil {
		return nil, fmt.Errorf("flow must not be nil")
	}
	body, err := json.Marshal(flow)
	if err != nil {
		return nil, fmt.Errorf("encoding request: %w", err)
	}

	req, err := c.newRequest(http.MethodPost, "/flows", bytes.NewReader(body))
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
func (c *Client) UpdateFlow(flow *Flow) (*Flow, error) {
	if flow == nil {
		return nil, fmt.Errorf("flow must not be nil")
	}
	if flow.ID == "" {
		return nil, fmt.Errorf("flow ID must be set for update")
	}

	body, err := json.Marshal(flow)
	if err != nil {
		return nil, fmt.Errorf("encoding request: %w", err)
	}

	req, err := c.newRequest(http.MethodPut, "/flows/"+url.PathEscape(flow.ID), bytes.NewReader(body))
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
func (c *Client) DeleteFlow(flowID string) error {
	if flowID == "" {
		return fmt.Errorf("flow ID must not be empty")
	}
	req, err := c.newRequest(http.MethodDelete, "/flows/"+url.PathEscape(flowID), nil)
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
