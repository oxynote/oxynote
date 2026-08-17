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
		Called     bool
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
			Called:     true,
			RespCode:   http.StatusOK,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var called bool

			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				called = true

				w.WriteHeader(http.StatusOK)
			})

			rec := httptest.NewRecorder()

			RequireConfigured(
				slog.New(slog.DiscardHandler),
				func() bool { return c.Configured },
				c.Err,
			)(next).ServeHTTP(rec, httptest.NewRequest("GET", "http://test.com/", http.NoBody))

			assert.Equal(t, c.Called, called)
			assert.Equal(t, c.RespCode, rec.Code)

			if c.RespJSON == "" {
				assert.Zero(t, rec.Body.Len(), rec.Body.String())
				return
			}

			assert.JSONEq(t, c.RespJSON, rec.Body.String())
		})
	}
}

func Test_Connected(t *testing.T) {
	cc := map[string]struct {
		Configured bool
		FetchErr   error
		Fetched    bool
		Result     bool
		Err        error
	}{
		"Not configured skips the lookup": {
			FetchErr: assert.AnError,
		},
		"Error returned by the lookup": {
			Configured: true,
			FetchErr:   assert.AnError,
			Fetched:    true,
			Err:        assert.AnError,
		},
		"Missing row is not connected": {
			Configured: true,
			FetchErr:   errutil.ErrNotFound,
			Fetched:    true,
		},
		"Connected": {
			Configured: true,
			Fetched:    true,
			Result:     true,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var fetched bool

			res, err := Connected(context.Background(), c.Configured, func(_ context.Context) error {
				fetched = true

				return c.FetchErr
			})

			testutil.AssertEqualError(t, c.Err, err)
			assert.Equal(t, c.Fetched, fetched)

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
