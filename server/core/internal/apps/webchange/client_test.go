package webchange

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// capture records the request the test server received.
type capture struct {
	method string
	path   string
	apiKey string
	body   []byte
}

// newTestClient starts a test server responding with the given status and
// body, and returns a Client pointed at it plus the captured request.
func newTestClient(t *testing.T, status int, respBody string) (*Client, *capture) {
	t.Helper()

	captured := &capture{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

func Test_NewClient(t *testing.T) {
	t.Parallel()

	c := NewClient("http://test.com", "key")
	require.NotNil(t, c)
	assert.Equal(t, "http://test.com", c.baseURL)
	assert.Equal(t, "key", c.apiKey)
	require.NotNil(t, c.client)
	assert.Equal(t, _requestTimeout, c.client.Timeout)
}

func Test_Client_FetchWatcher(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		Status        int
		Body          string
		ExpectErr     bool
		ExpectedErr   error
		ExpectedWatch *Watch
	}{
		// a watcher deleted on the changedetection.io side is recoverable —
		// the caller recreates it — so it gets a sentinel of its own instead
		// of the generic api error.
		"Missing watcher reports the sentinel": {
			Status:      http.StatusNotFound,
			Body:        `{"detail": "not found"}`,
			ExpectErr:   true,
			ExpectedErr: ErrWatcherNotFound,
		},
		"Watch without errors is reachable": {
			Status: http.StatusOK,
			Body:   `{"url": "https://example.com", "last_changed": 1700000000, "last_error": false}`,
			ExpectedWatch: &Watch{
				URL:           "https://example.com",
				Unreachable:   false,
				LastChangedAt: time.Unix(1700000000, 0),
			},
		},
		"Boolean error flag marks the watch unreachable": {
			Status: http.StatusOK,
			Body:   `{"url": "https://example.com", "last_error": true}`,
			ExpectedWatch: &Watch{
				URL:           "https://example.com",
				Unreachable:   true,
				LastChangedAt: time.Unix(0, 0),
			},
		},
		"Non-empty error string marks the watch unreachable": {
			Status: http.StatusOK,
			Body:   `{"url": "https://example.com", "last_error": "connection refused"}`,
			ExpectedWatch: &Watch{
				URL:           "https://example.com",
				Unreachable:   true,
				LastChangedAt: time.Unix(0, 0),
			},
		},
		"Empty error string keeps the watch reachable": {
			Status: http.StatusOK,
			Body:   `{"url": "https://example.com", "last_error": ""}`,
			ExpectedWatch: &Watch{
				URL:           "https://example.com",
				Unreachable:   false,
				LastChangedAt: time.Unix(0, 0),
			},
		},
		"Null error keeps the watch reachable": {
			Status: http.StatusOK,
			Body:   `{"url": "https://example.com", "last_error": null}`,
			ExpectedWatch: &Watch{
				URL:           "https://example.com",
				Unreachable:   false,
				LastChangedAt: time.Unix(0, 0),
			},
		},
		"Unknown error type marks the watch unreachable": {
			Status: http.StatusOK,
			Body:   `{"url": "https://example.com", "last_error": {"code": 1}}`,
			ExpectedWatch: &Watch{
				URL:           "https://example.com",
				Unreachable:   true,
				LastChangedAt: time.Unix(0, 0),
			},
		},
		"Non-200 status surfaces the API error body": {
			Status:    http.StatusNotFound,
			Body:      `No watch exists with the UUID`,
			ExpectErr: true,
		},
		"Malformed JSON fails decoding": {
			Status:    http.StatusOK,
			Body:      `{not json`,
			ExpectErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c, captured := newTestClient(t, tc.Status, tc.Body)

			watch, err := c.FetchWatcher(context.Background(), "uuid-1")

			assert.Equal(t, http.MethodGet, captured.method)
			assert.Equal(t, "/api/v1/watch/uuid-1", captured.path)
			assert.Equal(t, "test-key", captured.apiKey)

			if tc.ExpectErr {
				require.Error(t, err)

				if tc.ExpectedErr != nil {
					assert.Equal(t, tc.ExpectedErr, err)
				}

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.ExpectedWatch, watch)
		})
	}
}

func Test_Client_CreateWatcher(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		Status       int
		Body         string
		ExpectErr    bool
		ExpectedUUID string
	}{
		"Created watch returns its UUID": {
			Status:       http.StatusCreated,
			Body:         `{"uuid": "uuid-1"}`,
			ExpectedUUID: "uuid-1",
		},
		"Non-201 status surfaces the API error body": {
			Status:    http.StatusBadRequest,
			Body:      `Invalid URL`,
			ExpectErr: true,
		},
		"Malformed JSON fails decoding": {
			Status:    http.StatusCreated,
			Body:      `{not json`,
			ExpectErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c, captured := newTestClient(t, tc.Status, tc.Body)

			uuid, err := c.CreateWatcher(context.Background(), "https://example.com")

			assert.Equal(t, http.MethodPost, captured.method)
			assert.Equal(t, "/api/v1/watch", captured.path)
			assert.Equal(t, "test-key", captured.apiKey)
			assertWatchRequestBody(t, captured.body)

			if tc.ExpectErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.ExpectedUUID, uuid)
		})
	}
}

func Test_Client_UpdateWatcher(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		Status    int
		Body      string
		ExpectErr bool
	}{
		"Updated watch succeeds": {
			Status: http.StatusOK,
		},
		"Non-200 status surfaces the API error body": {
			Status:    http.StatusBadRequest,
			Body:      `Invalid URL`,
			ExpectErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c, captured := newTestClient(t, tc.Status, tc.Body)

			err := c.UpdateWatcher(context.Background(), "uuid-1", "https://example.com")

			assert.Equal(t, http.MethodPut, captured.method)
			assert.Equal(t, "/api/v1/watch/uuid-1", captured.path)
			assert.Equal(t, "test-key", captured.apiKey)
			assertWatchRequestBody(t, captured.body)

			if tc.ExpectErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
		})
	}
}

func Test_Client_DeleteWatcher(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		Status    int
		Body      string
		ExpectErr bool
	}{
		"Deleted watch succeeds": {
			Status: http.StatusNoContent,
		},
		"Missing watch is tolerated": {
			Status: http.StatusNotFound,
		},
		"Other statuses surface the API error body": {
			Status:    http.StatusInternalServerError,
			Body:      `boom`,
			ExpectErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c, captured := newTestClient(t, tc.Status, tc.Body)

			err := c.DeleteWatcher(context.Background(), "uuid-1")

			assert.Equal(t, http.MethodDelete, captured.method)
			assert.Equal(t, "/api/v1/watch/uuid-1", captured.path)
			assert.Equal(t, "test-key", captured.apiKey)

			if tc.ExpectErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
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
