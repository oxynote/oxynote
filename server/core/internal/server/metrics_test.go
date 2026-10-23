package server

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/synctest"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/go-chi/chi/v5"
	"github.com/oxynote/oxynote/server/core/pkg/metricutil"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/oxynote/wetsocks/wsserver"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_wrapMetrics(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Delay    time.Duration
		Observed float64
	}{
		"Fast requests are only counted": {
			// MetricValue reports -1 for a series that was never observed.
			Observed: -1,
		},
		"Slow requests also record their duration": {
			Delay:    _durationThreshold + time.Millisecond,
			Observed: 3,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			// the bubble's clock moves only while every goroutine in it is
			// blocked. So the handler takes exactly Delay and no real time
			// passes.
			synctest.Test(t, func(t *testing.T) {
				rg := prometheus.NewRegistry()

				r := chi.NewRouter()
				r.Use(wrapMetrics(metricutil.NewFactory("test", rg)))
				r.Get("/items/{id}", func(w http.ResponseWriter, _ *http.Request) {
					time.Sleep(c.Delay)
					w.WriteHeader(http.StatusNoContent)
				})

				for range 3 {
					rec := httptest.NewRecorder()
					r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://test.com/items/42", http.NoBody))
					require.Equal(t, http.StatusNoContent, rec.Code)
				}

				labels := map[string]string{
					_pathLabel:   "/items/{id}",
					_methodLabel: http.MethodGet,
				}

				// requests are counted by route pattern, not the raw URL.
				assert.InEpsilon(t, 3.0, testutil.MetricValue(t, rg, "test_server_http_requests_total", labels), 0.0001)
				assert.InDelta(t, c.Observed, testutil.MetricValue(t, rg, "test_server_http_request_duration_seconds", labels), 0.0001)
			})
		})
	}
}

func Test_wsMetricsBinder(t *testing.T) {
	t.Parallel()

	rg := prometheus.NewRegistry()

	r := wsserver.NewRouter()
	binderFn := wsMetricsBinder(r, metricutil.NewFactory("test", rg))

	var tpc wsserver.Topic

	binderFn("test@test", func(ntpc wsserver.Topic) {
		tpc = ntpc
	})

	require.NotNil(t, tpc)

	// the middleware only fires while publishing to a live
	// subscriber, so run the real transport once.
	pool := wsserver.New(slog.New(slog.DiscardHandler), r, wsserver.Options{
		InsecureSkipVerify: true,
	})

	hs := httptest.NewServer(pool)

	t.Cleanup(func() {
		pool.Close()
		hs.Close()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, resp, err := websocket.Dial(ctx, hs.URL, &websocket.DialOptions{HTTPClient: hs.Client()})
	require.NoError(t, err)

	if resp != nil && resp.Body != nil {
		resp.Body.Close() //nolint:errcheck,gosec // error provides no meaningful info
	}

	defer conn.Close(websocket.StatusNormalClosure, "bye") //nolint:errcheck // error provides no meaningful info

	require.NoError(t, wsjson.Write(ctx, conn, struct {
		ID    uint64 `json:"id"`
		Topic string `json:"topic"`
	}{ID: 5, Topic: "sub~test@test"}))

	_, _, err = conn.Read(ctx)
	require.NoError(t, err)

	tpc.PublishMany(ctx, "payload", nil)

	_, _, err = conn.Read(ctx)
	require.NoError(t, err)

	assert.InEpsilon(t, 1.0, testutil.MetricValue(t, rg, "test_server_ws_messages_total", map[string]string{
		_topicTemplateLabel: "test@test",
	}), 0.0001)
}
