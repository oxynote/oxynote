package processor

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"strings"

	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/strutil"
)

const (
	// _maxErrorMessageSize caps how much of an error body is shown.
	_maxErrorMessageSize = 1024

	// _maxHTTPResponseSize caps the bytes read from one answer: log lines
	// may be up to 256 KiB each, and a page may ask for thousands.
	_maxHTTPResponseSize = 32 << 20
)

// ErrResponseTooLarge is returned when an answer holds more than
// _maxHTTPResponseSize bytes.
var ErrResponseTooLarge = NewInvalidQueryError("the data source answered with more than 32 MiB")

// _errRedirected is returned when the server answers with a redirect,
// which the client does not follow.
var _errRedirected = errutil.New(
	http.StatusBadRequest,
	"data_source.redirected",
	"the data source answered with a redirect",
)

// _errEndpointNotFound is returned when the server has no route for a
// request, which is what a URL that points at another server answers.
var _errEndpointNotFound = errutil.New(
	http.StatusBadRequest,
	"data_source.endpoint_not_found",
	"the data source does not serve this request",
)

// httpClient sends the requests of a data source that answers over
// HTTP. It authenticates with the data source's credentials and never
// follows a redirect.
type httpClient struct {
	inp       Input
	transport ownTransport
}

// fetchJSON sends a GET to the path and decodes the JSON answer as T.
func (hc *httpClient) fetchJSON[T any](ctx context.Context, path string, params url.Values, header http.Header) (T, error) {
	var res T

	body, err := hc.fetch(ctx, http.MethodGet, path, params, header)
	if err != nil {
		return res, err
	}

	if err := json.Unmarshal(body, &res); err != nil {
		var zero T

		return zero, fmt.Errorf("decoding response: %w", err)
	}

	return res, nil
}

// fetch sends a request to the path and returns the body of a 200 answer.
// Any other answer is the error it describes.
func (hc *httpClient) fetch(ctx context.Context, method, path string, params url.Values, header http.Header) ([]byte, error) {
	client, err := hc.authClient()
	if err != nil {
		return nil, err
	}

	status, body, err := hc.doRequest(ctx, client, method, path, params, header)
	if err != nil {
		return nil, err
	}

	switch {
	case status == http.StatusOK:
		return body, nil
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return nil, ConnectionStatusUnauthorized.Error()
	case status >= http.StatusMultipleChoices && status < http.StatusBadRequest:
		return nil, _errRedirected
	case status == http.StatusNotFound:
		return nil, _errEndpointNotFound
	case status == http.StatusTooManyRequests:
		return nil, _errRateLimited
	case status >= http.StatusInternalServerError:
		return nil, ConnectionStatusUnreachable.Error()
	}

	// servers answer an error in plain text, but the proxies of hosted
	// services wrap it as JSON. Either is cut to _maxErrorMessageSize runes.
	var wrapped struct {
		// Message specifies the error, as proxies write it.
		Message string `json:"message"`

		// Error specifies the error, as Prometheus-style APIs write it.
		Error string `json:"error"`
	}

	msg := strings.TrimSpace(string(body))

	if json.Unmarshal(body, &wrapped) == nil {
		msg = cmp.Or(wrapped.Message, wrapped.Error, msg)
	}

	msg = strutil.Ellipsize(msg, _maxErrorMessageSize)

	return nil, NewInvalidQueryError(cmp.Or(msg, "the data source rejected the query"))
}

// doRequest sends a request to the path through client and returns the
// status and body. A POST carries params as a form body, for a query too
// long for a URL.
func (hc *httpClient) doRequest(
	ctx context.Context,
	client *http.Client,
	method string,
	path string,
	params url.Values,
	header http.Header,
) (int, []byte, error) {
	target, err := url.JoinPath(hc.inp.URL(), path)
	if err != nil {
		return 0, nil, fmt.Errorf("building url: %w", err)
	}

	reqBody := io.Reader(http.NoBody)

	switch {
	case method == http.MethodPost:
		reqBody = strings.NewReader(params.Encode())
	case len(params) > 0:
		target += "?" + params.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, method, target, reqBody)
	if err != nil {
		// NOCOV: url.JoinPath already parsed the same target.
		return 0, nil, fmt.Errorf("creating request: %w", err)
	}

	maps.Copy(req.Header, header)

	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // error provides no meaningful info

	body, err := io.ReadAll(io.LimitReader(resp.Body, _maxHTTPResponseSize+1))
	if err != nil {
		return 0, nil, fmt.Errorf("reading response: %w", err)
	}

	if len(body) > _maxHTTPResponseSize {
		return 0, nil, ErrResponseTooLarge
	}

	return resp.StatusCode, body, nil
}

// authClient returns the HTTP client that authenticates with the data
// source's credentials and stays on the data source's own address.
func (hc *httpClient) authClient() (*http.Client, error) {
	rawCreds := hc.inp.Credentials()

	auth, err := rawCreds.Read()
	if err != nil {
		return nil, err
	}

	transport, err := hc.transport.roundTripper(http.DefaultTransport, hc.inp.URL(), auth)
	if err != nil {
		return nil, err
	}

	if auth != nil {
		transport = auth.Authorize(transport)
	}

	return &http.Client{
		Transport: transport,
		// a followed redirect could lead to any host, whose error answers
		// would then reach the user.
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}, nil
}
