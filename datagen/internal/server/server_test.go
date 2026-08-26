package server

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/oxynote/oxynote/server/core/pkg/metricutil"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func Test_New(t *testing.T) {
	t.Parallel()

	var (
		log = slog.New(slog.DiscardHandler)
		fc  = newFactory()
	)

	s := New(log, fc)
	require.NotNil(t, s)

	assert.Equal(t, log, s.log)
	assert.Equal(t, fc, s.fc)

	require.NotNil(t, s.http)
	assert.Equal(t, ":8080", s.http.Addr)
	assert.Equal(t, time.Minute, s.http.ReadTimeout)
	assert.Equal(t, time.Minute, s.http.ReadHeaderTimeout)
	assert.NotNil(t, s.http.Handler)
	assert.NotNil(t, s.http.ErrorLog)
}

func Test_Server_Listen(t *testing.T) {
	t.Parallel()

	log, wr, content := bufferLog()

	s := Server{
		log:  log,
		http: &http.Server{Addr: ":a", ReadHeaderTimeout: time.Minute},
	}

	// an invalid port makes ListenAndServe fail immediately.
	s.Listen()

	require.NoError(t, wr.Flush())
	assert.Contains(t, content(), `"msg":"listening"`)
	assert.Contains(t, content(), "cannot terminate gracefully")

	// a graceful shutdown ends Listen without the error log.
	log2, wr2, content2 := bufferLog()

	s2 := Server{
		log:  log2,
		http: &http.Server{Addr: "127.0.0.1:0", ReadHeaderTimeout: time.Minute},
	}

	stopCh := make(chan struct{})

	go func() {
		defer close(stopCh)

		s2.Listen()
	}()

	require.Eventually(t, func() bool {
		require.NoError(t, wr2.Flush())

		return content2() != ""
	}, 5*time.Second, 10*time.Millisecond)

	require.NoError(t, s2.http.Shutdown(context.Background()))

	<-stopCh

	require.NoError(t, wr2.Flush())
	assert.Contains(t, content2(), `"msg":"listening"`)
	assert.NotContains(t, content2(), "cannot terminate gracefully")
}

func Test_Server_Address(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Addr   string
		Result string
	}{
		"Default HTTP port is omitted": {
			Addr:   ":80",
			Result: "http://localhost",
		},
		"Custom port is included": {
			Addr:   ":1234",
			Result: "http://localhost:1234",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			s := Server{http: &http.Server{Addr: c.Addr, ReadHeaderTimeout: time.Minute}}

			assert.Equal(t, c.Result, s.Address())
		})
	}
}

func Test_Server_Close(t *testing.T) {
	t.Parallel()

	s := Server{
		log:  slog.New(slog.DiscardHandler),
		http: &http.Server{ReadHeaderTimeout: time.Minute},
	}

	assert.NoError(t, s.Close())
}

func Test_Server_router(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Method   string
		Path     string
		RespCode int

		// Contains is a fragment the response body must carry, used where
		// the body is a metrics dump rather than a fixed payload.
		Contains string
	}{
		"Metrics are served": {
			Method:   http.MethodGet,
			Path:     "/api/metrics",
			RespCode: http.StatusOK,
			Contains: "engineering_test_gauge",
		},
		"Unknown path is not found": {
			Method:   http.MethodGet,
			Path:     "/api/nothing",
			RespCode: http.StatusNotFound,
		},
		// the metrics route is registered for every method, so a scrape
		// with an odd verb is answered rather than rejected.
		"Metrics answer any method": {
			Method:   http.MethodDelete,
			Path:     "/api/metrics",
			RespCode: http.StatusOK,
			Contains: "engineering_test_gauge",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			fc := newFactory()

			// a registered gauge gives the scrape something to report.
			fc.NewGauge(metricutil.Options{Name: "test_gauge", Help: "Tracks nothing."}).Set(1)

			s := Server{log: slog.New(slog.DiscardHandler), fc: fc}

			req := httptest.NewRequest(c.Method, "http://test.com"+c.Path, http.NoBody)
			rec := httptest.NewRecorder()

			s.router().ServeHTTP(rec, req)

			assert.Equal(t, c.RespCode, rec.Code)

			if c.Contains != "" {
				assert.Contains(t, rec.Body.String(), c.Contains)
			}
		})
	}
}

// newFactory creates a metrics factory backed by a registry of its own, so
// parallel tests never collide over metric names.
func newFactory() metricutil.Factory {
	return metricutil.NewFactory(
		"engineering",
		prometheus.NewRegistry(),
		metricutil.WithCustomHost("demo"),
	)
}

// bufferLog creates a logger writing into a buffer whose contents the caller
// can read back once flushed.
func bufferLog() (*slog.Logger, *testutil.Writer, func() string) {
	wr, buf := testutil.NewBuffer()

	return slog.New(slog.NewJSONHandler(wr, nil)), wr, buf.String
}
