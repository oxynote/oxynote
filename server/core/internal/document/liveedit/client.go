package liveedit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// Client posts batched operations to the Node hocuspocus service's
// internal operations endpoint. The base URL points at the Node
// service (e.g. http://auth-realtime:8081); the path to the
// endpoint is appended by Apply.
type Client struct {
	// httpClient is the underlying HTTP client. Callers should
	// pass one with a reasonable timeout configured.
	httpClient *http.Client

	// baseURL is the Node service root, without a trailing slash.
	baseURL string
}

// NewClient constructs an edit-RPC client. The baseURL must include
// the scheme and host of the Node service; the operations path is
// appended per request.
func NewClient(httpClient *http.Client, baseURL string) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	return &Client{
		httpClient: httpClient,
		baseURL:    baseURL,
	}
}

// Result is the outcome of an Apply call. Applied counts the
// operations Node committed to the Y.Doc; Errors lists per-op
// failures (e.g. uid not found, malformed block). A partial-success
// response has Applied < len(ops) and at least one entry in Errors.
type Result struct {
	// Applied is the number of operations the Node side committed
	// to the live Y.Doc.
	Applied int `json:"applied"`

	// Errors contains one entry per failed operation, in input
	// order. An empty slice means every operation succeeded.
	Errors []OpError `json:"errors"`
}

// OpError describes one operation's failure on the Node side.
type OpError struct {
	// Index is the position of the failing op in the request's
	// operations array.
	Index int `json:"index"`

	// Message is the short reason from Node.
	Message string `json:"message"`
}

// Apply submits the given operations to the Node service for the
// (documentID, branchID) document and returns the per-op outcome.
// Any transport-level failure (bad status, malformed response,
// canonical expansion error) is returned as an error and no
// operations are considered applied; failures of individual
// operations within an otherwise successful HTTP response are
// surfaced via Result.Errors.
func (c *Client) Apply(ctx context.Context, documentID, branchID string, ops []Operation) (Result, error) {
	if len(ops) == 0 {
		return Result{}, nil
	}

	wires := make([]wireOp, 0, len(ops))

	for i, op := range ops {
		w, err := op.toWire()
		if err != nil {
			return Result{}, fmt.Errorf("operation %d: %w", i, err)
		}

		wires = append(wires, w)
	}

	body, err := json.Marshal(struct {
		Operations []wireOp `json:"operations"`
	}{Operations: wires})
	if err != nil {
		return Result{}, fmt.Errorf("marshal operations: %w", err)
	}

	endpoint, err := c.endpoint(documentID, branchID)
	if err != nil {
		return Result{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return Result{}, fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("post operations: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Surface up to ~1 KiB of the response body so callers can
		// see the Node-side error without us pre-parsing.
		preview, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))

		return Result{}, fmt.Errorf("node operations endpoint: status %d: %s", resp.StatusCode, string(preview))
	}

	var out Result
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return Result{}, fmt.Errorf("decode response: %w", err)
	}

	return out, nil
}

// endpoint builds the per-document operations URL.
func (c *Client) endpoint(documentID, branchID string) (string, error) {
	base, err := url.Parse(c.baseURL)
	if err != nil {
		return "", fmt.Errorf("parse base url: %w", err)
	}

	base.Path = fmt.Sprintf("/api/internal/documents/%s/branches/%s/operations",
		url.PathEscape(documentID),
		url.PathEscape(branchID),
	)

	return base.String(), nil
}
