package webchange

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

// capture records how many requests the test server received and the last
// one of them.
type capture struct {
	requests int
	method   string
	path     string
	apiKey   string
	body     []byte
}

// newTestClient starts a test server responding with the given status and
// body, and returns a Client pointed at it plus the captured request.
func newTestClient(t *testing.T, status int, respBody string) (*Client, *capture) {
	t.Helper()

	captured := &capture{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured.requests++
		captured.method = r.Method
		captured.path = r.URL.Path
		captured.apiKey = r.Header.Get("X-Api-Key")

		raw, err := io.ReadAll(r.Body)
		assert.NoError(t, err)

		captured.body = raw

		w.WriteHeader(status)

		_, err = w.Write([]byte(respBody))
		assert.NoError(t, err)
	}))
	t.Cleanup(srv.Close)

	return NewClient(srv.URL, "test-key"), captured
}

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func Test_NewClient(t *testing.T) {
	t.Parallel()

	c := NewClient("http://test.com", "key")
	require.NotNil(t, c)
	assert.Equal(t, "http://test.com", c.baseURL)
	assert.Equal(t, "key", c.apiKey)
	require.NotNil(t, c.client)
	assert.Equal(t, _requestTimeout, c.client.Timeout)
}

func Test_Client_Configured(t *testing.T) {
	t.Parallel()

	assert.True(t, NewClient("http://test.com", "key").Configured())
	assert.False(t, NewClient("", "key").Configured())
}

func Test_Client_FetchWatcher(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Unconfigured  bool
		BaseURL       string
		Status        int
		Body          string
		Requests      int
		ExpectedWatch *Watch
		Err           error
	}{
		"Unconfigured client fails": {
			Unconfigured: true,
			Err:          ErrNotConfigured,
		},
		"Error returned by http.NewRequestWithContext": {
			BaseURL: "http://\x7f",
			Err:     assert.AnError,
		},
		"Error returned by http.Client.Do": {
			BaseURL: "unsupported://test.com",
			Err:     assert.AnError,
		},
		// a watcher deleted on the changedetection.io side is recoverable —
		// the caller recreates it — so it gets a sentinel of its own instead
		// of the generic api error.
		"Missing watcher reports the sentinel": {
			Status:   http.StatusNotFound,
			Body:     `{"detail": "not found"}`,
			Requests: 1,
			Err:      ErrWatcherNotFound,
		},
		"Watch without errors is reachable": {
			Status:   http.StatusOK,
			Body:     `{"url": "https://example.com", "last_changed": 1700000000, "last_error": false}`,
			Requests: 1,
			ExpectedWatch: &Watch{
				URL:           "https://example.com",
				Unreachable:   false,
				LastChangedAt: time.Unix(1700000000, 0),
			},
		},
		"Boolean error flag marks the watch unreachable": {
			Status:   http.StatusOK,
			Body:     `{"url": "https://example.com", "last_error": true}`,
			Requests: 1,
			ExpectedWatch: &Watch{
				URL:           "https://example.com",
				Unreachable:   true,
				LastChangedAt: time.Unix(0, 0),
			},
		},
		"Non-empty error string marks the watch unreachable": {
			Status:   http.StatusOK,
			Body:     `{"url": "https://example.com", "last_error": "connection refused"}`,
			Requests: 1,
			ExpectedWatch: &Watch{
				URL:           "https://example.com",
				Unreachable:   true,
				LastChangedAt: time.Unix(0, 0),
			},
		},
		"Empty error string keeps the watch reachable": {
			Status:   http.StatusOK,
			Body:     `{"url": "https://example.com", "last_error": ""}`,
			Requests: 1,
			ExpectedWatch: &Watch{
				URL:           "https://example.com",
				Unreachable:   false,
				LastChangedAt: time.Unix(0, 0),
			},
		},
		"Null error keeps the watch reachable": {
			Status:   http.StatusOK,
			Body:     `{"url": "https://example.com", "last_error": null}`,
			Requests: 1,
			ExpectedWatch: &Watch{
				URL:           "https://example.com",
				Unreachable:   false,
				LastChangedAt: time.Unix(0, 0),
			},
		},
		"Unknown error type marks the watch unreachable": {
			Status:   http.StatusOK,
			Body:     `{"url": "https://example.com", "last_error": {"code": 1}}`,
			Requests: 1,
			ExpectedWatch: &Watch{
				URL:           "https://example.com",
				Unreachable:   true,
				LastChangedAt: time.Unix(0, 0),
			},
		},
		"Non-200 status surfaces the API error body": {
			Status:   http.StatusInternalServerError,
			Body:     `boom`,
			Requests: 1,
			Err:      errors.New("api error (status 500): boom"),
		},
		"Malformed JSON fails decoding": {
			Status:   http.StatusOK,
			Body:     `{not json`,
			Requests: 1,
			Err:      assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			client, captured := newTestClient(t, c.Status, c.Body)

			if c.Unconfigured {
				client.baseURL = ""
			}

			if c.BaseURL != "" {
				client.baseURL = c.BaseURL
			}

			watch, err := client.FetchWatcher(context.Background(), "uuid-1")

			assert.Equal(t, c.Requests, captured.requests)

			if c.Requests > 0 {
				assert.Equal(t, http.MethodGet, captured.method)
				assert.Equal(t, "/api/v1/watch/uuid-1", captured.path)
				assert.Equal(t, "test-key", captured.apiKey)
			}

			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				assert.Nil(t, watch)

				return
			}

			assert.Equal(t, c.ExpectedWatch, watch)
		})
	}
}

func Test_Client_CreateWatcher(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Unconfigured bool
		BaseURL      string
		Status       int
		Body         string
		Requests     int
		ExpectedUUID string
		Err          error
	}{
		"Unconfigured client fails": {
			Unconfigured: true,
			Err:          ErrNotConfigured,
		},
		"Error returned by http.NewRequestWithContext": {
			BaseURL: "http://\x7f",
			Err:     assert.AnError,
		},
		"Error returned by http.Client.Do": {
			BaseURL: "unsupported://test.com",
			Err:     assert.AnError,
		},
		"Created watch returns its UUID": {
			Status:       http.StatusCreated,
			Body:         `{"uuid": "uuid-1"}`,
			Requests:     1,
			ExpectedUUID: "uuid-1",
		},
		"Non-201 status surfaces the API error body": {
			Status:   http.StatusBadRequest,
			Body:     `Invalid URL`,
			Requests: 1,
			Err:      errors.New("api error (status 400): Invalid URL"),
		},
		"Malformed JSON fails decoding": {
			Status:   http.StatusCreated,
			Body:     `{not json`,
			Requests: 1,
			Err:      assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			client, captured := newTestClient(t, c.Status, c.Body)

			if c.Unconfigured {
				client.baseURL = ""
			}

			if c.BaseURL != "" {
				client.baseURL = c.BaseURL
			}

			uuid, err := client.CreateWatcher(context.Background(), "https://example.com")

			assert.Equal(t, c.Requests, captured.requests)

			if c.Requests > 0 {
				assert.Equal(t, http.MethodPost, captured.method)
				assert.Equal(t, "/api/v1/watch", captured.path)
				assert.Equal(t, "test-key", captured.apiKey)
				assertWatchRequestBody(t, captured.body)
			}

			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				assert.Empty(t, uuid)

				return
			}

			assert.Equal(t, c.ExpectedUUID, uuid)
		})
	}
}

func Test_Client_UpdateWatcher(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Unconfigured bool
		BaseURL      string
		Status       int
		Body         string
		Requests     int
		Err          error
	}{
		"Unconfigured client fails": {
			Unconfigured: true,
			Err:          ErrNotConfigured,
		},
		"Error returned by http.NewRequestWithContext": {
			BaseURL: "http://\x7f",
			Err:     assert.AnError,
		},
		"Error returned by http.Client.Do": {
			BaseURL: "unsupported://test.com",
			Err:     assert.AnError,
		},
		"Updated watch succeeds": {
			Status:   http.StatusOK,
			Requests: 1,
		},
		"Non-200 status surfaces the API error body": {
			Status:   http.StatusBadRequest,
			Body:     `Invalid URL`,
			Requests: 1,
			Err:      errors.New("api error (status 400): Invalid URL"),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			client, captured := newTestClient(t, c.Status, c.Body)

			if c.Unconfigured {
				client.baseURL = ""
			}

			if c.BaseURL != "" {
				client.baseURL = c.BaseURL
			}

			err := client.UpdateWatcher(context.Background(), "uuid-1", "https://example.com")

			assert.Equal(t, c.Requests, captured.requests)

			if c.Requests > 0 {
				assert.Equal(t, http.MethodPut, captured.method)
				assert.Equal(t, "/api/v1/watch/uuid-1", captured.path)
				assert.Equal(t, "test-key", captured.apiKey)
				assertWatchRequestBody(t, captured.body)
			}

			testutil.AssertEqualError(t, c.Err, err)
		})
	}
}

func Test_Client_DeleteWatcher(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Unconfigured bool
		BaseURL      string
		Status       int
		Body         string
		Requests     int
		Err          error
	}{
		"Unconfigured client has nothing to delete": {
			Unconfigured: true,
		},
		"Error returned by http.NewRequestWithContext": {
			BaseURL: "http://\x7f",
			Err:     assert.AnError,
		},
		"Error returned by http.Client.Do": {
			BaseURL: "unsupported://test.com",
			Err:     assert.AnError,
		},
		"Deleted watch succeeds": {
			Status:   http.StatusNoContent,
			Requests: 1,
		},
		"Missing watch is tolerated": {
			Status:   http.StatusNotFound,
			Requests: 1,
		},
		"Other statuses surface the API error body": {
			Status:   http.StatusInternalServerError,
			Body:     `boom`,
			Requests: 1,
			Err:      errors.New("api error (status 500): boom"),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			client, captured := newTestClient(t, c.Status, c.Body)

			if c.Unconfigured {
				client.baseURL = ""
			}

			if c.BaseURL != "" {
				client.baseURL = c.BaseURL
			}

			err := client.DeleteWatcher(context.Background(), "uuid-1")

			assert.Equal(t, c.Requests, captured.requests)

			if c.Requests > 0 {
				assert.Equal(t, http.MethodDelete, captured.method)
				assert.Equal(t, "/api/v1/watch/uuid-1", captured.path)
				assert.Equal(t, "test-key", captured.apiKey)
			}

			testutil.AssertEqualError(t, c.Err, err)
		})
	}
}

func Test_Client_parseErrorResponse(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Body io.Reader
		Err  error
	}{
		"Error returned by Body.Read": {
			Body: iotest.ErrReader(assert.AnError),
			Err:  fmt.Errorf("failed to read error response: %w", assert.AnError),
		},
		"Body is surfaced with the status": {
			Body: strings.NewReader("boom"),
			Err:  errors.New("api error (status 500): boom"),
		},
		"Oversized body is truncated": {
			Body: strings.NewReader(strings.Repeat("a", _maxErrorBody+1)),
			Err:  errors.New("api error (status 500): " + strings.Repeat("a", _maxErrorBody)),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			err := (&Client{}).parseErrorResponse(&http.Response{
				StatusCode: http.StatusInternalServerError,
				Body:       io.NopCloser(c.Body),
			})

			testutil.AssertEqualError(t, c.Err, err)
		})
	}
}

// assertWatchRequestBody checks the shared create/update request payload:
// the target URL, the WebDriver fetch backend, and the check interval.
func assertWatchRequestBody(t *testing.T, raw []byte) {
	t.Helper()

	var body watchRequest

	require.NoError(t, json.Unmarshal(raw, &body))
	assert.Equal(t, "https://example.com", body.URL)
	assert.Equal(t, _fetchBackendWebDriver, body.FetchBackend)
	require.NotNil(t, body.TimeBetweenCheck)
	assert.Equal(t, _checkIntervalMinutes, body.TimeBetweenCheck.Minutes)
	assert.False(t, body.TimeBetweenCheckUseDefault)
}
