package webchanges

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client is a client for the changedetection.io API.
type Client struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

// NewClient creates a new changedetection.io API client.
func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		client:  http.DefaultClient,
	}
}

// FetchWatcher fetches the watch with the given UUID.
func (c *Client) FetchWatcher(ctx context.Context, uuid string) (*Watch, error) {
	url := c.baseURL + "/api/v1/watch/" + uuid

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("x-api-key", c.apiKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, c.parseErrorResponse(resp)
	}

	var watch rawWatch

	if err := json.NewDecoder(resp.Body).Decode(&watch); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	var unreachable bool

	switch werr := watch.LastError.(type) {
	case bool:
		unreachable = werr
	case string:
		unreachable = werr != ""
	default:
		// Unknown type, assume unreachable if not nil.
		unreachable = werr != nil
	}

	return &Watch{
		URL:           watch.URL,
		Unreachable:   unreachable,
		LastChangedAt: time.Unix(watch.LastChanged, 0),
	}, nil
}

// CreateWatcher creates a new watch and returns its uuid.
func (c *Client) CreateWatcher(ctx context.Context, url string) (string, error) {
	body, err := json.Marshal(CreateWatchRequest{
		URL:          url,
		FetchBackend: _fetchBackendWebDriver,
		TimeBetweenCheck: &TimeBetweenCheck{
			Minutes: 3,
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/watch", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return "", c.parseErrorResponse(resp)
	}

	var result struct {
		UUID string `json:"uuid"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	return result.UUID, nil
}

// UpdateWatcher updates an existing watch.
func (c *Client) UpdateWatcher(ctx context.Context, uuid, url string) error {
	body, err := json.Marshal(UpdateWatchRequest{
		URL:          url,
		FetchBackend: _fetchBackendWebDriver,
		TimeBetweenCheck: &TimeBetweenCheck{
			Minutes: 3,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.baseURL+"/api/v1/watch/"+uuid, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return c.parseErrorResponse(resp)
	}

	return nil
}

// DeleteWatcher deletes the watch with the given UUID.
func (c *Client) DeleteWatcher(ctx context.Context, uuid string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.baseURL+"/api/v1/watch/"+uuid, http.NoBody)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("x-api-key", c.apiKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
		return c.parseErrorResponse(resp)
	}

	return nil
}

// parseErrorResponse parses an error response from the API.
func (c *Client) parseErrorResponse(resp *http.Response) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read error response: %w", err)
	}

	return fmt.Errorf("api error: %s", string(body))
}
