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

	body, err := hc.fetchRaw(ctx, path, params, header)
	if err != nil {
		return res, err
	}

	if err := json.Unmarshal(body, &res); err != nil {
		var zero T

		return zero, fmt.Errorf("decoding response: %w", err)
	}

	return res, nil
}

// fetchRaw sends a GET to the path and returns the body of a 200 answer, as
// fetch does.
func (hc *httpClient) fetchRaw(ctx context.Context, path string, params url.Values, header http.Header) ([]byte, error) {
	return hc.fetch(ctx, path, params, nil, header)
}

// postForm sends a POST to the path with form as its body and returns the
// body of a 200 answer, as fetch does. It is for a query too long for a
// URL, which many proxies cut off at about 8 KB. A GET cannot carry it
// either, because Go servers read form values from the body only for
// POST, PUT and PATCH, and proxies often drop the body of a GET. Use it
// only for a server that reads such a POST like the same GET, as
// VictoriaLogs does.
func (hc *httpClient) postForm(ctx context.Context, path string, form url.Values) ([]byte, error) {
	return hc.fetch(ctx, path, nil, form, nil)
}

// fetch sends a request to the path and returns the body of a 200 answer.
// It is a POST of form when form is set, and a GET otherwise. Any other
// answer is the error it describes.
func (hc *httpClient) fetch(ctx context.Context, path string, params, form url.Values, header http.Header) ([]byte, error) {
	client, err := hc.authClient()
	if err != nil {
		return nil, err
	}

	status, body, err := hc.doRequest(ctx, client, path, params, form, header)
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
// status and body. It is a POST of form when form is set, and a GET
// otherwise.
func (hc *httpClient) doRequest(
	ctx context.Context,
	client *http.Client,
	path string,
	params url.Values,
	form url.Values,
	header http.Header,
) (int, []byte, error) {
	target, err := url.JoinPath(hc.inp.URL(), path)
	if err != nil {
		return 0, nil, fmt.Errorf("building url: %w", err)
	}

	if len(params) > 0 {
		target += "?" + params.Encode()
	}

	method, reqBody := http.MethodGet, io.Reader(http.NoBody)
	if form != nil {
		method, reqBody = http.MethodPost, strings.NewReader(form.Encode())
	}

	req, err := http.NewRequestWithContext(ctx, method, target, reqBody)
	if err != nil {
		// NOCOV: url.JoinPath already parsed the same target.
		return 0, nil, fmt.Errorf("creating request: %w", err)
	}

	maps.Copy(req.Header, header)

	if form != nil {
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
