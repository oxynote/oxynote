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
		fc  = metricutil.NewFactory("engineering", prometheus.NewRegistry(), metricutil.WithCustomHost("demo"))
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

	cc := map[string]struct {
		Addr      string
		Shutdown  bool
		ErrLogged bool
	}{
		// an invalid port makes ListenAndServe fail immediately.
		"Error returned by http.Server.ListenAndServe": {
			Addr:      ":a",
			ErrLogged: true,
		},
		// a graceful shutdown ends Listen without the error log.
		"Successful graceful shutdown": {
			Addr:     "127.0.0.1:0",
			Shutdown: true,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			wr, buf := testutil.NewBuffer()

			s := Server{
				log:  slog.New(slog.NewJSONHandler(wr, nil)),
				http: &http.Server{Addr: c.Addr, ReadHeaderTimeout: time.Minute},
			}

			stopCh := make(chan struct{})

			go func() {
				defer close(stopCh)

				s.Listen()
			}()

			if c.Shutdown {
				require.Eventually(t, func() bool {
					require.NoError(t, wr.Flush())

					return buf.String() != ""
				}, 5*time.Second, 10*time.Millisecond)

				require.NoError(t, s.http.Shutdown(context.Background()))
			}

			<-stopCh

			require.NoError(t, wr.Flush())
			assert.Contains(t, buf.String(), `"msg":"listening"`)

			if c.ErrLogged {
				assert.Contains(t, buf.String(), "cannot terminate gracefully")
			} else {
				assert.NotContains(t, buf.String(), "cannot terminate gracefully")
			}
		})
	}
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

			// each subtest registers the same gauge, so each needs its own
			// registry.
			fc := metricutil.NewFactory("engineering", prometheus.NewRegistry(), metricutil.WithCustomHost("demo"))

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
