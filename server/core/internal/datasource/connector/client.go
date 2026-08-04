// Package connector provides a client for communicating with the connector service.
package connector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/oxynote/oxynote/server/core/pkg/errutil"

	"github.com/oxynote/oxynote/server/core/internal/datasource"
	"github.com/oxynote/oxynote/server/core/internal/datasource/processor"
)

// Client communicates with the connector service for data source operations.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a new connector client.
func NewClient(baseURL string, httpClient *http.Client) *Client {
	return &Client{
		baseURL:    baseURL,
		httpClient: httpClient,
	}
}

// TestConnection tests the connection to a data source via the connector.
func (c *Client) TestConnection(ctx context.Context, ds datasource.DataSource) (processor.ConnectionStatus, error) {
	resp, err := c.sendRequest(ctx,
		"/api/data-sources/connection",
		NewTestConnectionRequest(
			datasource.NewRunner(ds),
		),
	)
	if err != nil {
		return "", err
	}

	return resp.Status, nil
}

// sendRequest sends a request to the connector service and returns the response.
func (c *Client) sendRequest(ctx context.Context, path string, body any) (*Response, error) {
	rawBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("error marshaling connector request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(rawBody))
	if err != nil {
		return nil, fmt.Errorf("error creating connector request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	httpResp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error sending connector request: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		respBody, err := io.ReadAll(httpResp.Body)
		if err != nil {
			return nil, fmt.Errorf("error reading connector error response: %w", err)
		}

		var errResp struct {
			Message string `json:"message"`
			Code    string `json:"code"`
		}

		err = json.Unmarshal(respBody, &errResp)
		if err != nil {
			return nil, fmt.Errorf("error decoding connector error response: %w", err)
		}

		if errResp.Code == "" {
			errResp.Code = "unknown_error"
			errResp.Message = "An unknown error occurred while processing the connector request."
		}

		return nil, errutil.New(httpResp.StatusCode, errResp.Code, "%s", errResp.Message)
	}

	var resp Response

	if err := json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
		return nil, fmt.Errorf("error decoding connector response: %w", err)
	}

	return &resp, nil
}
