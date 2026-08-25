// Package webchange provides a client for the changedetection.io API.
package webchange

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/oxynote/oxynote/server/core/pkg/errutil"
)

// ErrWatcherNotFound is returned when the watcher no longer exists on the
// changedetection.io side, which a caller can recover from by creating it
// again rather than failing forever.
var ErrWatcherNotFound = errors.New("watcher not found")

// ErrNotConfigured is returned when the changedetection.io integration is not
// configured on this deployment.
var ErrNotConfigured = errutil.New(http.StatusConflict, "changedetection.not_configured", "changedetection is not configured")

// _maxErrorBody caps how much of an error response is echoed into the
// returned error. A misbehaving deployment or a proxy in front of it can
// answer with an arbitrarily large HTML page, which no error message
// needs in full.
const _maxErrorBody = 4 << 10

// _requestTimeout bounds a single changedetection.io request. The callers
// are hook processors running under the periodic executor's long-lived
// context, so without a deadline of its own a hung connection would stall
// the whole processing pass rather than just its own call.
const _requestTimeout = 30 * time.Second

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
		client:  &http.Client{Timeout: _requestTimeout},
	}
}

// Configured reports whether the changedetection.io integration is
// configured on this deployment. An empty base URL disables it.
func (c *Client) Configured() bool {
	return c.baseURL != ""
}

// FetchWatcher fetches the watch with the given UUID.
func (c *Client) FetchWatcher(ctx context.Context, uuid string) (*Watch, error) {
	if !c.Configured() {
		return nil, ErrNotConfigured
	}

	url := c.baseURL + "/api/v1/watch/" + uuid

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("X-Api-Key", c.apiKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // error provides no meaningful info

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrWatcherNotFound
	}

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
	if !c.Configured() {
		return "", ErrNotConfigured
	}

	body, err := json.Marshal(watchRequest{
		URL:          url,
		FetchBackend: _fetchBackendWebDriver,
		TimeBetweenCheck: &timeBetweenCheck{
			Minutes: _checkIntervalMinutes,
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/watch", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("X-Api-Key", c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // error provides no meaningful info

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
	if !c.Configured() {
		return ErrNotConfigured
	}

	body, err := json.Marshal(watchRequest{
		URL:          url,
		FetchBackend: _fetchBackendWebDriver,
		TimeBetweenCheck: &timeBetweenCheck{
			Minutes: _checkIntervalMinutes,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.baseURL+"/api/v1/watch/"+uuid, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("X-Api-Key", c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // error provides no meaningful info

	if resp.StatusCode != http.StatusOK {
		return c.parseErrorResponse(resp)
	}

	return nil
}

// DeleteWatcher deletes the watch with the given UUID. On an
// unconfigured deployment there is nothing to tear down, so the delete
// succeeds as a no-op rather than blocking whatever removal asked for it.
func (c *Client) DeleteWatcher(ctx context.Context, uuid string) error {
	if !c.Configured() {
		return nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.baseURL+"/api/v1/watch/"+uuid, http.NoBody)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("X-Api-Key", c.apiKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // error provides no meaningful info

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
		return c.parseErrorResponse(resp)
	}

	return nil
}

// parseErrorResponse parses an error response from the API.
func (c *Client) parseErrorResponse(resp *http.Response) error {
	body, err := io.ReadAll(io.LimitReader(resp.Body, _maxErrorBody))
	if err != nil {
		return fmt.Errorf("failed to read error response: %w", err)
	}

	return fmt.Errorf("api error (status %d): %s", resp.StatusCode, body)
}
