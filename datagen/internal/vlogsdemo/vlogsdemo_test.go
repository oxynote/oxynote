package vlogsdemo

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// fakeStore stands in for a VictoriaLogs server.
type fakeStore struct {
	// queryBody is the answer to a query.
	queryBody string

	// queryStatus is the status of a query, 200 when zero.
	queryStatus int

	// ingestStatus is the status of an ingest, 200 when zero.
	ingestStatus int

	mu       sync.Mutex
	requests int
	ticks    int
	lines    []string
}

// ServeHTTP answers a query or records the ingested lines.
func (f *fakeStore) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body) //nolint:errcheck // a short body fails the test's assertions

	if r.URL.Path == _queryPath {
		if f.queryStatus != 0 {
			w.WriteHeader(f.queryStatus)
		}

		_, _ = w.Write([]byte(f.queryBody)) //nolint:errcheck // the client reports it

		return
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	f.requests++
	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	f.lines = append(f.lines, lines...)

	if len(lines) <= _maxTickLogs {
		f.ticks++
	}

	if f.ingestStatus != 0 {
		w.WriteHeader(f.ingestStatus)
	}
}

// ingested returns the number of ingest requests and lines so far.
func (f *fakeStore) ingested() (int, int) {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.requests, len(f.lines)
}

// tickCount returns how many ingests were the size of a tick.
func (f *fakeStore) tickCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.ticks
}

// prepGenerator starts the fake store and points a generator at it.
func prepGenerator(t *testing.T, store *fakeStore) *Generator {
	t.Helper()

	srv := httptest.NewServer(store)
	t.Cleanup(srv.Close)

	g := NewGenerator(slog.New(slog.DiscardHandler), srv.URL+"/")
	g.httpClient = srv.Client()
	g.readyDelay = time.Millisecond

	return g
}

func Test_NewGenerator(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.DiscardHandler)

	g := NewGenerator(log, "http://victorialogs:9428/")
	require.NotNil(t, g)

	assert.Equal(t, log, g.log)
	assert.Equal(t, "http://victorialogs:9428", g.url)
	assert.Equal(t, _requestTimeout, g.httpClient.Timeout)
	assert.NotNil(t, g.r)
	assert.Equal(t, _tickInterval, g.interval)
	assert.Equal(t, _backfillDays, g.backfillDays)
	assert.Equal(t, _readyDelay, g.readyDelay)
}

func Test_Generator_Run(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Store    *fakeStore
		Err      error
		Backfill bool
	}{
		"Error returned by awaitStore": {
			Store: &fakeStore{queryStatus: http.StatusInternalServerError},
			Err:   assert.AnError,
		},
		"Error returned by backfill": {
			Store: &fakeStore{ingestStatus: http.StatusBadRequest},
			Err:   assert.AnError,
		},
		"Successful run on an empty store": {
			Store:    &fakeStore{},
			Backfill: true,
		},
		"Successful run on a populated store": {
			Store: &fakeStore{queryBody: `{"_msg":"x"}`},
		},
		"Failed tick keeps the run going": {
			Store: &fakeStore{queryBody: `{"_msg":"x"}`, ingestStatus: http.StatusBadRequest},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			store := c.Store
			g := prepGenerator(t, store)
			g.interval = time.Millisecond
			g.backfillDays = 1

			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()

			done := make(chan error, 1)

			go func() {
				done <- g.Run(ctx)
			}()

			if c.Err != nil {
				testutil.AssertEqualError(t, c.Err, <-done)

				return
			}

			require.Eventually(t, func() bool {
				return store.tickCount() > 1
			}, 5*time.Second, time.Millisecond)

			cancel()
			require.NoError(t, <-done)

			_, lines := store.ingested()
			if c.Backfill {
				assert.GreaterOrEqual(t, lines, 24*60)
			}
		})
	}
}

func Test_Generator_awaitStore(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Store    *fakeStore
		Canceled bool
		Result   bool
		Err      error
	}{
		"Store never answers": {
			Store: &fakeStore{queryStatus: http.StatusServiceUnavailable},
			Err:   assert.AnError,
		},
		"Context cancelled": {
			Store:    &fakeStore{queryStatus: http.StatusServiceUnavailable},
			Canceled: true,
			Err:      assert.AnError,
		},
		"Store answers": {Store: &fakeStore{}, Result: true},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()

			if c.Canceled {
				cancel()
			}

			res, err := prepGenerator(t, c.Store).awaitStore(ctx)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_Generator_isEmpty(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Store  *fakeStore
		Result bool
		Err    error
	}{
		"Error returned by post": {
			Store: &fakeStore{queryStatus: http.StatusBadRequest},
			Err:   assert.AnError,
		},
		"Empty store":     {Store: &fakeStore{queryBody: "\n"}, Result: true},
		"Populated store": {Store: &fakeStore{queryBody: `{"_msg":"x"}`}},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := prepGenerator(t, c.Store).isEmpty(t.Context())
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_Generator_backfill(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Store *fakeStore
		Err   error
	}{
		"Error returned by ingest": {
			Store: &fakeStore{ingestStatus: http.StatusBadRequest},
			Err:   assert.AnError,
		},
		"Successful backfill": {Store: &fakeStore{}},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			g := prepGenerator(t, c.Store)
			g.backfillDays = 2

			err := g.backfill(t.Context(), time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC))
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			requests, lines := c.Store.ingested()
			assert.Greater(t, requests, 1)
			assert.GreaterOrEqual(t, lines, 2*24*60)
			assert.LessOrEqual(t, lines, 2*24*60*_maxTickLogs)
		})
	}
}

func Test_Generator_newTick(t *testing.T) {
	t.Parallel()

	var (
		g    = NewGenerator(slog.New(slog.DiscardHandler), "http://x")
		now  = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
		span = time.Minute
	)

	for range 100 {
		entries := g.newTick(now, span)
		require.NotEmpty(t, entries)
		require.LessOrEqual(t, len(entries), _maxTickLogs)

		for _, e := range entries {
			at, err := time.Parse(time.RFC3339Nano, e.Time)
			require.NoError(t, err)
			assert.False(t, at.After(now))
			assert.True(t, at.After(now.Add(-span)))
		}
	}
}

func Test_Generator_newEntry(t *testing.T) {
	t.Parallel()

	var (
		g   = NewGenerator(slog.New(slog.DiscardHandler), "http://x")
		now = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	)

	services := map[string]bool{}
	envs := map[string]bool{}

	for range 500 {
		e := g.newEntry(now)

		services[e.Service] = true
		envs[e.Env] = true

		assert.Equal(t, now.Format(time.RFC3339Nano), e.Time)
		assert.True(t, strings.HasPrefix(e.Host, e.Service+"-"))
		assert.NotEmpty(t, e.Message)
	}

	assert.Len(t, services, len(_services))
	assert.Len(t, envs, 2)
}

func Test_Generator_newLevel(t *testing.T) {
	t.Parallel()

	g := NewGenerator(slog.New(slog.DiscardHandler), "http://x")
	counts := map[string]int{}

	for range 5000 {
		counts[g.newLevel()]++
	}

	assert.Len(t, counts, 4)
	assert.Greater(t, counts["info"], counts["warn"])
	assert.Greater(t, counts["warn"], counts["error"])
}

func Test_Generator_newMessage(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Service string
		Check   func(t *testing.T, message string)
	}{
		"JSON line": {
			Service: _serviceAPI,
			Check: func(t *testing.T, message string) {
				t.Helper()

				var line map[string]any

				require.NoError(t, json.Unmarshal([]byte(message), &line))
				assert.Equal(t, "error", line["level"])
				assert.InDelta(t, 501, line["status"], 2)
			},
		},
		"Logfmt line": {
			Service: _serviceWorker,
			Check: func(t *testing.T, message string) {
				t.Helper()

				assert.Contains(t, message, `level=error msg="job finished" job=`)
			},
		},
		"Plain line": {
			Service: _serviceGateway,
			Check: func(t *testing.T, message string) {
				t.Helper()

				assert.Contains(t, message, `"GET /`)
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			g := NewGenerator(slog.New(slog.DiscardHandler), "http://x")

			c.Check(t, g.newMessage(c.Service, "error"))
		})
	}
}

func Test_Generator_newStatus(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Level  string
		Result []int
	}{
		"Error": {Level: "error", Result: []int{500, 503}},
		"Warn":  {Level: "warn", Result: []int{404, 429}},
		"Info":  {Level: "info", Result: []int{200, 201}},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			g := NewGenerator(slog.New(slog.DiscardHandler), "http://x")

			for range 50 {
				assert.Contains(t, c.Result, g.newStatus(c.Level))
			}
		})
	}
}

func Test_Generator_ingest(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Store *fakeStore
		Err   error
	}{
		"Error returned by post": {
			Store: &fakeStore{ingestStatus: http.StatusBadRequest},
			Err:   assert.AnError,
		},
		"Successful ingest": {Store: &fakeStore{ingestStatus: http.StatusNoContent}},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			err := prepGenerator(t, c.Store).ingest(t.Context(), []entry{
				{Time: "2026-01-01T00:00:00Z", Message: "<a>", Service: "api"},
				{Time: "2026-01-01T00:00:01Z", Message: "b", Service: "api"},
			})
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			require.Len(t, c.Store.lines, 2)
			assert.Contains(t, c.Store.lines[0], `"_msg":"<a>"`)
		})
	}
}

func Test_Generator_post(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		URL    string
		Store  *fakeStore
		Result string
		Err    error
	}{
		"Error returned by http.NewRequestWithContext": {
			URL:   "http://bad host",
			Store: &fakeStore{},
			Err:   assert.AnError,
		},
		"Error returned by httpClient.Do": {
			URL:   "http://127.0.0.1:1",
			Store: &fakeStore{},
			Err:   assert.AnError,
		},
		"Unexpected status": {
			Store: &fakeStore{queryStatus: http.StatusBadGateway},
			Err:   assert.AnError,
		},
		"Successful request": {
			Store:  &fakeStore{queryBody: "ok"},
			Result: "ok",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			g := prepGenerator(t, c.Store)
			if c.URL != "" {
				g.url = c.URL
			}

			res, err := g.post(t.Context(), _queryPath, "text/plain", strings.NewReader("x"))
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, string(res))
		})
	}
}
