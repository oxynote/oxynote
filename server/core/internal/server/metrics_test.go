package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/go-chi/chi/v5"
	"github.com/oxynote/oxynote/server/core/pkg/metricutil"
	"github.com/oxynote/wetsocks/wsserver"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// gatherValue extracts a metric value with matching labels from the
// registry.
func gatherValue(t *testing.T, rg prometheus.Gatherer, name string, want map[string]string) float64 {
	t.Helper()

	families, err := rg.Gather()
	require.NoError(t, err)

	for _, mf := range families {
		if mf.GetName() != name {
			continue
		}

	metric:
		for _, m := range mf.GetMetric() {
			labels := map[string]string{}

			for _, lp := range m.GetLabel() {
				labels[lp.GetName()] = lp.GetValue()
			}

			for k, v := range want {
				if labels[k] != v {
					continue metric
				}
			}

			if m.GetCounter() != nil {
				return m.GetCounter().GetValue()
			}

			return m.GetHistogram().GetSampleSum()
		}
	}

	t.Fatalf("metric %s with labels %v not found", name, want)

	return 0
}

func Test_wrapMetrics(t *testing.T) {
	t.Parallel()

	rg := prometheus.NewRegistry()

	r := chi.NewRouter()
	r.Use(wrapMetrics(metricutil.NewFactory("test", rg)))
	r.Get("/items/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	for range 3 {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://test.com/items/42", http.NoBody))
		require.Equal(t, http.StatusNoContent, rec.Code)
	}

	// requests are counted by route pattern, not the raw URL.
	assert.InEpsilon(t, 3.0, gatherValue(t, rg, "test_server_http_requests_total", map[string]string{
		_pathLabel:   "/items/{id}",
		_methodLabel: http.MethodGet,
	}), 0.0001)
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
	pool := wsserver.New(discardLog(), r, wsserver.Options{
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

	assert.InEpsilon(t, 1.0, gatherValue(t, rg, "test_server_ws_messages_total", map[string]string{
		_topicTemplateLabel: "test@test",
	}), 0.0001)
}
