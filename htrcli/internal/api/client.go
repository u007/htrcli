package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Client is an HTTP client for the HTR NControl server.
type Client struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
}

// NewClient creates a new API client.
func NewClient(baseURL, token string) *Client {
	return &Client{
		BaseURL: baseURL,
		Token:   token,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// doRequestClient executes an HTTP request with the given client and returns the response body.
func (c *Client) doRequestClient(client *http.Client, method, path string, body any) ([]byte, error) {
	url := c.BaseURL + path

	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, &ConnectionError{Message: fmt.Sprintf("failed to create request: %v", err)}
	}

	req.Header.Set("Content-Type", "application/json")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, &ConnectionError{Message: fmt.Sprintf("failed to connect to server: %v", err)}
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode == 403 {
		return nil, &AuthError{Message: "authentication failed: invalid or missing token"}
	}

	if resp.StatusCode == 404 {
		var apiResp ApiResponse
		if json.Unmarshal(data, &apiResp) == nil && apiResp.Error != "" {
			return nil, &NotFoundError{Message: apiResp.Error}
		}
		return nil, &NotFoundError{Message: "resource not found"}
	}

	if resp.StatusCode >= 400 {
		return nil, &APIError{StatusCode: resp.StatusCode, Message: string(data)}
	}

	return data, nil
}

// doRequest executes an HTTP request using the default client and returns the response body.
func (c *Client) doRequest(method, path string, body any) ([]byte, error) {
	return c.doRequestClient(c.HTTPClient, method, path, body)
}

// doRequestWithEnvelope executes a request and unpacks the ApiResponse envelope.
func (c *Client) doRequestWithEnvelope(method, path string, body any) (any, error) {
	data, err := c.doRequest(method, path, body)
	if err != nil {
		return nil, err
	}

	var apiResp ApiResponse
	if err := json.Unmarshal(data, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if !apiResp.OK {
		return nil, &APIError{Message: apiResp.Error}
	}

	return apiResp.Data, nil
}

// PostEvents forwards captured events to the daemon.
func (c *Client) PostEvents(tabID int, kind string, entries []EventEntry) error {
	_, err := c.doRequestWithEnvelope("POST", "/api/events/ingest", IngestEventsRequest{
		TabID:   tabID,
		Kind:    kind,
		Entries: entries,
	})
	return err
}

// GetEvents polls captured events since a cursor.
func (c *Client) GetEvents(tabID *int, kind string, since int) (*EventsResponse, error) {
	path := fmt.Sprintf("/api/events?kind=%s&since=%d", kind, since)
	if tabID != nil {
		path += "&tab=" + strconv.Itoa(*tabID)
	}

	data, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}

	var apiResp ApiResponse
	if err := json.Unmarshal(data, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	if !apiResp.OK {
		return nil, &APIError{Message: apiResp.Error}
	}

	dataBytes, err := json.Marshal(apiResp.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal data: %w", err)
	}

	var events EventsResponse
	if err := json.Unmarshal(dataBytes, &events); err != nil {
		return nil, fmt.Errorf("failed to parse events response: %w", err)
	}

	return &events, nil
}

// GetHealth checks server health.
func (c *Client) GetHealth() (*HealthResponse, error) {
	data, err := c.doRequest("GET", "/api/health", nil)
	if err != nil {
		return nil, err
	}

	var apiResp ApiResponse
	if err := json.Unmarshal(data, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if !apiResp.OK {
		return nil, &APIError{Message: apiResp.Error}
	}

	dataBytes, err := json.Marshal(apiResp.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal data: %w", err)
	}

	var health HealthResponse
	if err := json.Unmarshal(dataBytes, &health); err != nil {
		return nil, fmt.Errorf("failed to parse health response: %w", err)
	}

	return &health, nil
}

// ListTabs returns all connected browser tabs.
func (c *Client) ListTabs() ([]TabInfo, error) {
	data, err := c.doRequest("GET", "/api/tabs", nil)
	if err != nil {
		return nil, err
	}

	var apiResp ApiResponse
	if err := json.Unmarshal(data, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if !apiResp.OK {
		return nil, &APIError{Message: apiResp.Error}
	}

	dataBytes, err := json.Marshal(apiResp.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal data: %w", err)
	}

	var tabs []TabInfo
	if err := json.Unmarshal(dataBytes, &tabs); err != nil {
		return nil, fmt.Errorf("failed to parse tabs: %w", err)
	}

	return tabs, nil
}

// GetTab returns information about a specific tab.
func (c *Client) GetTab(id int) (*TabInfo, error) {
	data, err := c.doRequest("GET", "/api/tabs/"+strconv.Itoa(id), nil)
	if err != nil {
		return nil, err
	}

	var apiResp ApiResponse
	if err := json.Unmarshal(data, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if !apiResp.OK {
		return nil, &APIError{Message: apiResp.Error}
	}

	dataBytes, err := json.Marshal(apiResp.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal data: %w", err)
	}

	var tab TabInfo
	if err := json.Unmarshal(dataBytes, &tab); err != nil {
		return nil, fmt.Errorf("failed to parse tab: %w", err)
	}

	return &tab, nil
}

// ExecuteCommand sends a command to a specific tab.
func (c *Client) ExecuteCommand(tabID *int, cmd Command) (*CommandResult, error) {
	req := CommandRequest{Command: cmd}

	var path string
	if tabID != nil {
		path = "/api/tabs/" + strconv.Itoa(*tabID) + "/command"
	} else {
		path = "/api/command"
	}

	data, err := c.doRequest("POST", path, req)
	if err != nil {
		return nil, err
	}

	var apiResp ApiResponse
	if err := json.Unmarshal(data, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if !apiResp.OK {
		return nil, &APIError{Message: apiResp.Error}
	}

	dataBytes, err := json.Marshal(apiResp.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal data: %w", err)
	}

	var result CommandResult
	if err := json.Unmarshal(dataBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to parse command result: %w", err)
	}

	return &result, nil
}

// GetPageInfo returns information about the current page. A non-nil tabID
// targets that tab (the --tab flag); nil falls back to the server's default.
func (c *Client) GetPageInfo(tabID *int) (*PageInfo, error) {
	path := "/api/page"
	if tabID != nil {
		path += "?tab=" + strconv.Itoa(*tabID)
	}
	data, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}

	var apiResp ApiResponse
	if err := json.Unmarshal(data, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if !apiResp.OK {
		return nil, &APIError{Message: apiResp.Error}
	}

	dataBytes, err := json.Marshal(apiResp.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal data: %w", err)
	}

	var page PageInfo
	if err := json.Unmarshal(dataBytes, &page); err != nil {
		return nil, fmt.Errorf("failed to parse page info: %w", err)
	}

	return &page, nil
}

// GetScreenshot captures a viewport screenshot and returns the base64 PNG data.
func (c *Client) GetScreenshot() (string, error) {
	return c.GetScreenshotOpts(ScreenshotOptions{})
}

// GetScreenshotOpts captures a screenshot with the given options and returns the
// base64 PNG data. When FullPage is true, the HTTP timeout is extended to 90s.
func (c *Client) GetScreenshotOpts(opts ScreenshotOptions) (string, error) {
	q := url.Values{}
	if opts.FullPage {
		q.Set("fullPage", "true")
	}
	if len(opts.Annotate) > 0 {
		raw, err := json.Marshal(opts.Annotate)
		if err != nil {
			return "", fmt.Errorf("failed to marshal annotate selectors: %w", err)
		}
		q.Set("annotate", string(raw))
	}
	if opts.TabID != nil {
		q.Set("tab", strconv.Itoa(*opts.TabID))
	}

	path := "/api/screenshot"
	if enc := q.Encode(); enc != "" {
		path += "?" + enc
	}

	client := c.HTTPClient
	if opts.FullPage {
		client = &http.Client{Timeout: 90 * time.Second}
	}

	data, err := c.doRequestClient(client, "GET", path, nil)
	if err != nil {
		return "", err
	}

	var apiResp ApiResponse
	if err := json.Unmarshal(data, &apiResp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if !apiResp.OK {
		return "", &APIError{Message: apiResp.Error}
	}

	// The screenshot data is in apiResp.Data as a string (base64)
	if apiResp.Data == nil {
		return "", fmt.Errorf("no screenshot data received")
	}

	// Marshal and unmarshal to get the string value
	dataBytes, err := json.Marshal(apiResp.Data)
	if err != nil {
		return "", fmt.Errorf("failed to marshal data: %w", err)
	}

	var screenshot string
	if err := json.Unmarshal(dataBytes, &screenshot); err != nil {
		return "", fmt.Errorf("failed to parse screenshot: %w", err)
	}

	return screenshot, nil
}
