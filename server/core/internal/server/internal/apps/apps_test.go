package apps

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func Test_RequireConfigured(t *testing.T) {
	cc := map[string]struct {
		Configured bool
		Err        error
		NextCalls  int
		RespCode   int
		RespJSON   string
	}{
		"Not configured": {
			Err:      errutil.New(http.StatusConflict, "app.not_configured", "app is not configured"),
			RespCode: http.StatusConflict,
			RespJSON: `{"code":"app.not_configured","message":"app is not configured"}`,
		},
		"Not configured with a hidden route": {
			Err:      errutil.NewPlain(http.StatusNotFound),
			RespCode: http.StatusNotFound,
			RespJSON: `{"code":"general","message":"not found"}`,
		},
		"Configured": {
			Configured: true,
			Err:        errutil.NewPlain(http.StatusNotFound),
			NextCalls:  1,
			RespCode:   http.StatusOK,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var configuredCalls, nextCalls int

			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				nextCalls++

				w.WriteHeader(http.StatusOK)
			})

			rec := httptest.NewRecorder()

			RequireConfigured(
				slog.New(slog.DiscardHandler),
				func() bool {
					configuredCalls++

					return c.Configured
				},
				c.Err,
			)(next).ServeHTTP(rec, httptest.NewRequest("GET", "http://test.com/", http.NoBody))

			assert.Equal(t, 1, configuredCalls)
			assert.Equal(t, c.NextCalls, nextCalls)
			testutil.AssertResponse(t, rec, c.RespCode, c.RespJSON)
		})
	}
}

func Test_Connected(t *testing.T) {
	cc := map[string]struct {
		Configured bool
		FetchErr   error
		FetchCalls int
		Result     bool
		Err        error
	}{
		"Not configured skips the lookup": {
			FetchErr: assert.AnError,
		},
		"Error returned by fetch": {
			Configured: true,
			FetchErr:   assert.AnError,
			FetchCalls: 1,
			Err:        assert.AnError,
		},
		"Missing row is not connected": {
			Configured: true,
			FetchErr:   errutil.ErrNotFound,
			FetchCalls: 1,
		},
		"Connected": {
			Configured: true,
			FetchCalls: 1,
			Result:     true,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var fetchCalls int

			res, err := Connected(context.Background(), c.Configured, func(_ context.Context) error {
				fetchCalls++

				return c.FetchErr
			})

			testutil.AssertEqualError(t, c.Err, err)
			assert.Equal(t, c.FetchCalls, fetchCalls)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_RespondCapability(t *testing.T) {
	cc := map[string]struct {
		Configured bool
		Connected  bool
		RespJSON   string
	}{
		"Unavailable integration": {
			RespJSON: `{"configured":false,"connected":false}`,
		},
		"Available but unconnected integration": {
			Configured: true,
			RespJSON:   `{"configured":true,"connected":false}`,
		},
		"Connected integration": {
			Configured: true,
			Connected:  true,
			RespJSON:   `{"configured":true,"connected":true}`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			rec := httptest.NewRecorder()

			RespondCapability(slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)), rec, c.Configured, c.Connected)

			assert.Equal(t, http.StatusOK, rec.Code)
			assert.JSONEq(t, c.RespJSON, rec.Body.String())
		})
	}
}
