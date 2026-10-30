package datasource

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/guregu/null/v5"
	"github.com/jellydator/xync"
	"github.com/oxynote/oxynote/server/core/internal/datasource/demo"
	"github.com/oxynote/oxynote/server/core/internal/datasource/processor"
	"github.com/oxynote/oxynote/server/core/pkg/memkit"
	"github.com/oxynote/oxynote/server/core/pkg/metricutil"
	"github.com/oxynote/oxynote/server/core/pkg/redkit"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
	"github.com/prometheus/client_golang/api"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// _testTimeRange is a fixed time range reused across the client tests.
var _testTimeRange = processor.TimeRange{
	From: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	To:   time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
}

// stubClient is a processor whose connection test answers status and err.
// It counts its tests and closes, and runs onTest inside each test when
// set.
type stubClient struct {
	status   processor.ConnectionStatus
	err      error
	closeErr error
	onTest   func()
	tests    atomic.Int32
	closes   atomic.Int32
}

func (s *stubClient) TestConnection(context.Context) (processor.ConnectionStatus, error) {
	s.tests.Add(1)

	if s.onTest != nil {
		s.onTest()
	}

	return s.status, s.err
}

func (s *stubClient) Close() error {
	s.closes.Add(1)

	return s.closeErr
}

// passingPostgreSQL is a PostgreSQL client whose connection test passes.
// Anything else asked of it is a test bug.
type passingPostgreSQL struct {
	PostgreSQL
}

func (passingPostgreSQL) TestConnection(context.Context) (processor.ConnectionStatus, error) {
	return processor.ConnectionStatusSuccess, nil
}

func (passingPostgreSQL) Close() error {
	return nil
}

// passingMySQL is a MySQL client whose connection test passes. Anything
// else asked of it is a test bug.
type passingMySQL struct {
	MySQL
}

func (passingMySQL) TestConnection(context.Context) (processor.ConnectionStatus, error) {
	return processor.ConnectionStatusSuccess, nil
}

func (passingMySQL) Close() error {
	return nil
}

// countNewConns returns a server hook counting in conns the connections
// clients open to it.
func countNewConns(conns *atomic.Int32) func(net.Conn, http.ConnState) {
	return func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			conns.Add(1)
		}
	}
}

// prepPrometheusServer starts a fake Prometheus API server serving the
// provided handler, counting in conns the connections clients open to it.
// Its cleanup also drops the Prometheus client's shared keep-alive
// connections so goleak stays clean.
func prepPrometheusServer(t *testing.T, h http.Handler, conns *atomic.Int32) *httptest.Server {
	t.Helper()

	srv := httptest.NewUnstartedServer(h)
	srv.Config.ConnState = countNewConns(conns)
	srv.Start()

	t.Cleanup(func() {
		srv.Close()
		api.DefaultRoundTripper.(*http.Transport).CloseIdleConnections()
	})

	return srv
}

// prometheusAPIHandler serves canned successful responses for every
// Prometheus API endpoint a client exercises.
func prometheusAPIHandler() http.Handler {
	respond := func(body string) http.HandlerFunc {
		return func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(body)) //nolint:errcheck,gosec // error provides no meaningful info
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/status/buildinfo", respond(`{"status":"success","data":{"version":"2.50.0"}}`))
	mux.HandleFunc("/api/v1/query_range", respond(`{"status":"success","data":{"resultType":"matrix","result":[{"metric":{"__name__":"up"},"values":[[1700000000,"1"]]}]}}`))
	mux.HandleFunc("/api/v1/metadata", respond(`{"status":"success","data":{"go_goroutines":[{"type":"gauge","help":"Number of goroutines.","unit":""}]}}`))
	mux.HandleFunc("/api/v1/labels", respond(`{"status":"success","data":["__name__","job"]}`))
	mux.HandleFunc("/api/v1/label/instance/values", respond(`{"status":"success","data":["a","b"]}`))
	mux.HandleFunc("/api/v1/series", respond(`{"status":"success","data":[{"__name__":"up","job":"prometheus"}]}`))

	return mux
}

// lokiAPIHandler serves canned successful responses for every Loki API
// endpoint a log client exercises. A query_range for a volume query
// answers with series, any other with streams.
func lokiAPIHandler() http.Handler {
	respond := func(body string) http.HandlerFunc {
		return func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(body)) //nolint:errcheck,gosec // error provides no meaningful info
		}
	}

	streams := respond(`{"status":"success","data":{"resultType":"streams","encodingFlags":["categorize-labels"],"result":[{"stream":{"app":"api"},"values":[["1704067200000000000","started",{}]]}]}}`)
	matrix := respond(`{"status":"success","data":{"resultType":"matrix","result":[{"metric":{"detected_level":"info"},"values":[[1704067260,"3"]]}]}}`)

	mux := http.NewServeMux()
	mux.HandleFunc("/loki/api/v1/status/buildinfo", respond(`{"version":"3.7.9"}`))
	mux.HandleFunc("/loki/api/v1/query_range", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Query().Get("query"), "sum by") {
			matrix(w, r)

			return
		}

		streams(w, r)
	})
	mux.HandleFunc("/loki/api/v1/labels", respond(`{"status":"success","data":["app"]}`))
	mux.HandleFunc("/loki/api/v1/label/app/values", respond(`{"status":"success","data":["api"]}`))

	return mux
}

// prepLokiServer starts a fake Loki serving the provided handler, counting
// in conns the connections clients open to it. Its cleanup also drops the
// shared transport's keep-alive connections so goleak stays clean.
func prepLokiServer(t *testing.T, h http.Handler, conns *atomic.Int32) *httptest.Server {
	t.Helper()

	srv := httptest.NewUnstartedServer(h)
	srv.Config.ConnState = countNewConns(conns)
	srv.Start()

	t.Cleanup(func() {
		srv.Close()
		http.DefaultTransport.(*http.Transport).CloseIdleConnections()
	})

	return srv
}

// countBuildinfo wraps h, counting in hits the connection tests it
// answers.
func countBuildinfo(h http.Handler, hits *atomic.Int32) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/status/buildinfo") {
			hits.Add(1)
		}

		h.ServeHTTP(w, r)
	})
}

// prepManager builds a manager over the given store, closed when the test
// ends so no connection outlives it.
func prepManager(t *testing.T, store *StatusStoreMock) *Manager {
	t.Helper()

	if store == nil {
		store = &StatusStoreMock{}
	}

	m := NewManager(slog.New(slog.DiscardHandler), store, nil, metricutil.NewFactory("test", nil))

	t.Cleanup(func() { assert.NoError(t, m.Close()) })

	return m
}

// prepRunner builds the runner for a data source of the given type,
// pointed at the given URL, over a store that records what it was told.
func prepRunner(t *testing.T, typ Type, url string, store *StatusStoreMock) *runner {
	t.Helper()

	r, ok := prepManager(t, store).Runner(DataSource{
		ID:             xid.New(),
		OrganizationID: "org",
		Name:           "prod",
		Type:           typ,
		URL:            url,
		Status:         processor.ConnectionStatusSuccess,
	}).(*runner)
	require.True(t, ok)

	return r
}

// prepStubRunner puts a runner whose client is the given stub into the
// manager, last asked for at usedAt.
func prepStubRunner(m *Manager, client *stubClient, usedAt time.Time) *runner {
	r := &runner{
		m: m,
		ds: DataSource{
			ID:             xid.New(),
			OrganizationID: "org",
			Type:           TypePrometheus,
		},
		client: client,
		status: processor.ConnectionStatusSuccess,
		usedAt: usedAt,
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.runners[r.ds.ID] = r

	return r
}

func Test_NewManager(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Pool  *redis.Pool
		Cache queryCache
	}{
		"Without valkey the answers stay in this process": {
			Cache: &memkit.BytesStore{},
		},
		"With valkey the answers are kept there": {
			Pool:  &redis.Pool{},
			Cache: &redkit.BytesStore{},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			store := &StatusStoreMock{}

			m := NewManager(slog.New(slog.DiscardHandler), store, c.Pool, metricutil.NewFactory("test", nil))
			require.NotNil(t, m)

			t.Cleanup(func() { assert.NoError(t, m.Close()) })

			assert.NotNil(t, m.log)
			assert.Same(t, store, m.store)
			assert.IsType(t, c.Cache, m.cache)
			assert.NotNil(t, m.demo)
			assert.NotNil(t, m.metrics)
			assert.NotNil(t, m.tests)
			assert.Empty(t, m.runners)
			assert.NotNil(t, m.supv)
		})
	}
}

func Test_Manager_Start(t *testing.T) {
	t.Parallel()

	// the bubble's clock is virtual, so the sleep below passes the idle
	// limit and one more sweep at once.
	synctest.Test(t, func(t *testing.T) {
		m := prepManager(t, nil)
		cache := &queryCacheMock{}
		m.cache = cache

		idle := &stubClient{}
		prepStubRunner(m, idle, timeutil.Now())

		ctx, cancel := context.WithCancel(context.Background())

		supv := xync.NewSupervisor()
		supv.Go(func(context.Context) { m.Start(ctx) })

		time.Sleep(_idleAfter + _sweepInterval)
		synctest.Wait()

		cancel()
		supv.Wait()
		m.supv.Wait()

		// Start runs the cache's own upkeep and the idle sweep until ctx
		// ends.
		assert.Len(t, cache.StartCalls(), 1)
		assert.Equal(t, int32(1), idle.closes.Load())
	})
}

func Test_Manager_Close(t *testing.T) {
	t.Parallel()

	m := prepManager(t, nil)

	closing := &stubClient{}
	failing := &stubClient{closeErr: assert.AnError}

	prepStubRunner(m, closing, timeutil.Now())
	prepStubRunner(m, failing, timeutil.Now())

	require.NoError(t, m.Close())

	// every runner leaves, and Close waits for each close, a failing one
	// included.
	assert.Empty(t, m.runners)
	assert.Equal(t, int32(1), closing.closes.Load())
	assert.Equal(t, int32(1), failing.closes.Load())
}

func Test_Manager_Runner(t *testing.T) {
	t.Parallel()

	id := xid.New()
	edited := null.TimeFrom(time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC))

	cc := map[string]struct {
		// Kept is the data source the manager already keeps a runner for,
		// if any.
		Kept *DataSource
		DS   DataSource

		// Same indicates whether the kept runner is handed back.
		Same      bool
		ClientErr error
	}{
		"First call hands out a runner the manager does not keep yet": {
			DS: DataSource{ID: id, Type: TypePrometheus, URL: "http://prometheus.test", Status: processor.ConnectionStatusSuccess},
		},
		// the caller read the stored row just now, so its status is the
		// one a later observation is compared with.
		"Unchanged data source gets the kept runner": {
			Kept: &DataSource{ID: id, Type: TypePrometheus, URL: "http://prometheus.test", Status: processor.ConnectionStatusSuccess},
			DS:   DataSource{ID: id, Type: TypePrometheus, URL: "http://prometheus.test", Status: processor.ConnectionStatusUnreachable},
			Same: true,
		},
		// testing an edit must not close the connections its readers use.
		"Edited data source gets a runner of its own": {
			Kept: &DataSource{ID: id, Type: TypePrometheus, URL: "http://prometheus.test", Status: processor.ConnectionStatusSuccess},
			DS:   DataSource{ID: id, Type: TypePrometheus, URL: "http://prometheus.new", Status: processor.ConnectionStatusSuccess, UpdatedAt: edited},
		},
		"Data source whose client cannot be built": {
			DS:        DataSource{ID: id, Type: TypePrometheus, URL: demo.Scheme + "nope"},
			ClientErr: demo.ErrUnknownSource,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			m := prepManager(t, nil)
			asked := timeutil.Now()

			var (
				kept *runner
				stub = &stubClient{}
			)

			if c.Kept != nil {
				kept = prepStubRunner(m, stub, asked.Add(-time.Hour))
				kept.ds = *c.Kept
				m.runners = map[xid.ID]*runner{id: kept}
			}

			r, ok := m.Runner(c.DS).(*runner)
			require.True(t, ok)

			m.supv.Wait()

			assert.Equal(t, c.DS.Status, r.status)
			assert.False(t, r.usedAt.Before(asked))
			testutil.AssertEqualError(t, c.ClientErr, r.clientErr)
			assert.Zero(t, stub.closes.Load())

			if c.Same {
				assert.Same(t, kept, r)
				return
			}

			assert.NotSame(t, kept, r)
			assert.Equal(t, c.DS, r.ds)
			assert.Same(t, kept, m.runners[id])

			if c.ClientErr == nil {
				assert.IsType(t, &processor.Prometheus{}, r.client)
			}
		})
	}
}

func Test_Manager_adopt(t *testing.T) {
	t.Parallel()

	edited := null.TimeFrom(time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC))

	cc := map[string]struct {
		// Kept is what the manager keeps for the data source before the
		// call: nothing, the runner itself, another runner for the same
		// version, or one for an earlier version.
		Kept string

		// Released indicates whether the manager let the runner go before
		// the call.
		Released bool

		// Result is the runner handed back and Held the one kept after the
		// call: the runner itself, the other one, or nothing.
		Result string
		Held   string
		Closes int32
	}{
		"First runner for the data source": {Result: "self", Held: "self"},
		"Runner already kept":              {Kept: "self", Result: "self", Held: "self"},
		// the call that adopted first wins, and the loser never connected.
		"Same version kept by another call": {Kept: "same", Result: "other", Held: "other"},
		"Edited data source replaces the kept runner": {
			Kept:   "older",
			Result: "self",
			Held:   "self",
			Closes: 1,
		},
		// its connections are closed, so keeping it again would hand them
		// out.
		"Runner the manager let go": {Released: true, Result: "self", Held: "none"},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			m := prepManager(t, nil)

			r := &runner{
				m:        m,
				ds:       DataSource{ID: xid.New(), UpdatedAt: edited},
				status:   processor.ConnectionStatusUnreachable,
				released: c.Released,
			}

			stub := &stubClient{}
			other := &runner{m: m, ds: r.ds, client: stub}

			switch c.Kept {
			case "self":
				m.runners[r.ds.ID] = r
			case "same":
				m.runners[r.ds.ID] = other
			case "older":
				other.ds.UpdatedAt = null.Time{}
				m.runners[r.ds.ID] = other
			}

			got := m.adopt(r)
			m.supv.Wait()

			runners := map[string]*runner{"self": r, "other": other}

			assert.Same(t, runners[c.Result], got)
			assert.Same(t, runners[c.Held], m.runners[r.ds.ID])
			assert.Equal(t, c.Closes, stub.closes.Load())
			assert.Equal(t, processor.ConnectionStatusUnreachable, got.status)
		})
	}
}

func Test_Manager_Evict(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		// Known indicates whether the evicted id is one the manager holds.
		Known  bool
		Closes int32
	}{
		"Known data source":   {Known: true, Closes: 1},
		"Unknown data source": {},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			m := prepManager(t, nil)
			stub := &stubClient{}
			r := prepStubRunner(m, stub, timeutil.Now())

			id := xid.New()
			if c.Known {
				id = r.ds.ID
			}

			m.Evict(id)
			m.supv.Wait()

			assert.Equal(t, c.Closes, stub.closes.Load())

			_, held := m.runners[r.ds.ID]
			assert.Equal(t, !c.Known, held)
		})
	}
}

func Test_Manager_renew(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		// Held is what the manager keeps for the data source before the
		// call: the runner itself, another one, or nothing.
		Held    string
		Renewed bool
	}{
		"Runner kept":           {Held: "self", Renewed: true},
		"Another runner kept":   {Held: "other"},
		"No runner kept at all": {},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			m := prepManager(t, nil)
			stub := &stubClient{}

			r := &runner{
				m: m,
				ds: DataSource{
					ID:             xid.New(),
					OrganizationID: "org",
					Type:           TypePrometheus,
				},
				client:   stub,
				status:   processor.ConnectionStatusNotReadOnly,
				usedAt:   timeutil.Now(),
				testedAt: timeutil.Now(),
				tested:   processor.ConnectionStatusNotReadOnly,
			}

			runners := map[string]*runner{"self": r, "other": {m: m, ds: r.ds}}

			if kept := runners[c.Held]; kept != nil {
				m.runners[r.ds.ID] = kept
			}

			m.renew(r)
			m.supv.Wait()

			held := m.runners[r.ds.ID]

			if !c.Renewed {
				assert.Same(t, runners[c.Held], held)
				assert.Zero(t, stub.closes.Load())

				return
			}

			require.NotNil(t, held)
			assert.NotSame(t, r, held)
			assert.NotNil(t, held.client)
			assert.Equal(t, r.ds, held.ds)
			assert.Equal(t, r.status, held.status)
			assert.Equal(t, r.usedAt, held.usedAt)
			assert.Equal(t, r.testedAt, held.testedAt)
			assert.Equal(t, r.tested, held.tested)
			assert.Equal(t, int32(1), stub.closes.Load())
		})
	}
}

func Test_Manager_keeps(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		// Held is what the manager keeps for the data source: the runner
		// itself, another one, or nothing.
		Held   string
		Result bool
	}{
		"Runner kept":           {Held: "self", Result: true},
		"Another runner kept":   {Held: "other"},
		"No runner kept at all": {},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			m := prepManager(t, nil)
			r := &runner{m: m, ds: DataSource{ID: xid.New()}}

			switch c.Held {
			case "self":
				m.runners[r.ds.ID] = r
			case "other":
				m.runners[r.ds.ID] = &runner{m: m, ds: r.ds}
			}

			assert.Equal(t, c.Result, m.keeps(r))
		})
	}
}

func Test_Manager_closeIdle(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		UsedAgo time.Duration
		Closed  bool
	}{
		"Runner unused for _idleAfter": {UsedAgo: _idleAfter, Closed: true},
		"Runner used since":            {UsedAgo: _idleAfter - time.Minute},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			m := prepManager(t, nil)
			stub := &stubClient{}
			r := prepStubRunner(m, stub, timeutil.Now().Add(-c.UsedAgo))

			m.closeIdle(context.Background())
			m.supv.Wait()

			_, held := m.runners[r.ds.ID]
			assert.Equal(t, !c.Closed, held)

			if c.Closed {
				assert.Equal(t, int32(1), stub.closes.Load())
				return
			}

			assert.Zero(t, stub.closes.Load())
		})
	}
}

func Test_Manager_release(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Client ownedClient
		Log    string
	}{
		"Client that holds nothing": {
			Client: &demo.Client{},
		},
		"Runner whose client was never built": {},
		"Client closed": {
			Client: &stubClient{},
		},
		"Close that fails is logged": {
			Client: &stubClient{closeErr: assert.AnError},
			Log:    "cannot close data source connections",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			w, buf := testutil.NewBuffer()

			m := &Manager{
				log:  slog.New(slog.NewTextHandler(w, nil)),
				supv: xync.NewSupervisor(),
			}

			m.release(&runner{m: m, ds: DataSource{ID: xid.New()}, client: c.Client})
			m.supv.Wait()

			require.NoError(t, w.Flush())

			if stub, ok := c.Client.(*stubClient); ok {
				assert.Equal(t, int32(1), stub.closes.Load())
			}

			if c.Log == "" {
				assert.Empty(t, buf.String())
				return
			}

			assert.Contains(t, buf.String(), c.Log)
		})
	}
}

func Test_Manager_newClient(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Type   Type
		URL    string
		Client ownedClient
		Err    error
	}{
		"Prometheus":          {Type: TypePrometheus, Client: &processor.Prometheus{}},
		"Demo Prometheus":     {Type: TypePrometheus, URL: demo.URL, Client: &demo.Client{}},
		"Unknown demo source": {Type: TypePrometheus, URL: demo.Scheme + "nope", Err: demo.ErrUnknownSource},
		"PostgreSQL":          {Type: TypePostgreSQL, Client: &processor.PostgreSQL{}},
		"Supabase":            {Type: TypeSupabase, Client: &processor.PostgreSQL{}},
		"MySQL":               {Type: TypeMySQL, Client: &processor.MySQL{}},
		"MariaDB":             {Type: TypeMariaDB, Client: &processor.MySQL{}},
		"Loki":                {Type: TypeLoki, Client: &processor.Loki{}},
		"Demo Loki":           {Type: TypeLoki, URL: demo.URL, Client: &demo.Client{}},
		"Unknown demo Loki":   {Type: TypeLoki, URL: demo.Scheme + "nope", Err: demo.ErrUnknownSource},
		"Unknown type":        {Type: Type("bogus"), Err: assert.AnError},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			client, err := prepManager(t, nil).newClient(DataSource{Type: c.Type, URL: c.URL})
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				assert.Nil(t, client)
				return
			}

			assert.IsType(t, c.Client, client)
		})
	}
}

func Test_Manager_prometheusClient(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		URL    string
		Client ownedClient
		Err    error
	}{
		"Demo data source": {
			URL:    demo.URL,
			Client: &demo.Client{},
		},
		"Demo scheme naming a source that does not exist": {
			URL: demo.Scheme + "marketing",
			Err: demo.ErrUnknownSource,
		},
		"Demo scheme with no source at all": {
			URL: demo.Scheme,
			Err: demo.ErrUnknownSource,
		},
		"Real server": {
			URL:    "http://prom.test:9090",
			Client: &processor.Prometheus{},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			m := prepManager(t, nil)

			client, err := m.prometheusClient(DataSource{Type: TypePrometheus, URL: c.URL})
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				assert.Nil(t, client)
				return
			}

			assert.IsType(t, c.Client, client)

			// the demo client caches the history it replays, so every
			// runner reads the manager's rather than building its own.
			if c.URL == demo.URL {
				assert.Same(t, m.demo, client)
			}
		})
	}
}

func Test_Manager_lokiClient(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		URL    string
		Client ownedClient
		Err    error
	}{
		"Demo data source": {
			URL:    demo.URL,
			Client: &demo.Client{},
		},
		"Demo scheme naming a source that does not exist": {
			URL: demo.Scheme + "marketing",
			Err: demo.ErrUnknownSource,
		},
		"Real server": {
			URL:    "http://loki.test:3100",
			Client: &processor.Loki{},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			m := prepManager(t, nil)

			client, err := m.lokiClient(DataSource{Type: TypeLoki, URL: c.URL})
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				assert.Nil(t, client)
				return
			}

			assert.IsType(t, c.Client, client)

			if c.URL == demo.URL {
				assert.Same(t, m.demo, client, "the demo answers as Loki out of the client the manager shares")
			}
		})
	}
}

func Test_runner_Type(t *testing.T) {
	t.Parallel()

	assert.Equal(t, TypeMariaDB, prepRunner(t, TypeMariaDB, "", nil).Type())
}

func Test_runner_TestConnection(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Type          Type
		URL           string
		Handler       http.Handler
		Undecryptable bool
		Kept          bool

		// Tests is how many connection tests the two calls reach the data
		// source with.
		Tests  int32
		Result processor.ConnectionStatus
		Err    error
	}{
		"A type with no processor behind it": {
			Type: Type("bogus"),
			Err:  assert.AnError,
		},
		"Credentials that cannot be decrypted": {
			Type:          TypePrometheus,
			Undecryptable: true,
			Result:        processor.ConnectionStatusInvalidEncryptionKey,
		},
		// a test someone asked for is never answered from one remembered
		// a moment ago.
		"A connection that answers": {
			Type:    TypePrometheus,
			Handler: testutil.JSONHandler("/api/v1/status/buildinfo", http.StatusOK, `{"status":"success","data":{"version":"2.50.0"}}`),
			Tests:   2,
			Result:  processor.ConnectionStatusSuccess,
		},
		"A connection that does not": {
			Type:    TypePrometheus,
			Handler: testutil.JSONHandler("/api/v1/status/buildinfo", http.StatusInternalServerError, `{}`),
			Tests:   2,
			Result:  processor.ConnectionStatusUnreachable,
		},
		// readers share the kept runner's client, so a test dials through
		// one of its own.
		"A runner the manager keeps": {
			Type:    TypePrometheus,
			Handler: testutil.JSONHandler("/api/v1/status/buildinfo", http.StatusOK, `{"status":"success","data":{"version":"2.50.0"}}`),
			Kept:    true,
			Tests:   2,
			Result:  processor.ConnectionStatusSuccess,
		},
		// the throwaway client's pool is closed after the test, which
		// goleak would otherwise catch.
		"A PostgreSQL connection that does not answer": {
			Type:   TypePostgreSQL,
			URL:    "postgres://127.0.0.1:1/db",
			Result: processor.ConnectionStatusUnreachable,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var hits atomic.Int32

			url := c.URL
			if c.Handler != nil {
				url = prepPrometheusServer(t, countBuildinfo(c.Handler, &hits), &atomic.Int32{}).URL
			}

			store := &StatusStoreMock{}
			r := prepRunner(t, c.Type, url, store)

			shared := &stubClient{}
			if c.Kept {
				r = prepStubRunner(r.m, shared, timeutil.Now())
				r.ds.URL = url
			}

			if c.Undecryptable {
				r.ds.Credentials = undecryptableCredentials(t)
			}

			cs, err := r.TestConnection(context.Background())
			testutil.AssertEqualError(t, c.Err, err)

			again, aerr := r.TestConnection(context.Background())
			testutil.AssertEqualError(t, c.Err, aerr)

			// an explicit test is asked about a data source that is
			// being created or changed, so what it finds is reported and
			// not recorded, and the runner is not kept for it. It never
			// touches the client a kept runner shares.
			assert.Empty(t, store.UpdateDataSourceStatusCalls())
			assert.Equal(t, c.Tests, hits.Load())
			assert.True(t, r.testedAt.IsZero())
			assert.Equal(t, c.Kept, r.m.keeps(r))
			assert.Zero(t, shared.tests.Load())
			assert.Zero(t, shared.closes.Load())

			if c.Err != nil {
				return
			}

			assert.Equal(t, c.Result, cs)
			assert.Equal(t, c.Result, again)
		})
	}
}

// Test_runner_Prometheus runs its cases one after another, and Go finishes
// it before resuming any parallel test: the clients keep idle connections
// in a transport every test shares, and another test's cleanup closing
// them would throw off the connection count.
func Test_runner_Prometheus(t *testing.T) {
	cc := map[string]struct {
		Type    Type
		Handler http.Handler
		Written int
		Tests   int32
		Conns   int32
		Err     error
	}{
		// every read, and the next call's, shares one connection.
		"A Prometheus data source hands out its client": {
			Type:    TypePrometheus,
			Handler: prometheusAPIHandler(),
			Tests:   1,
			Conns:   1,
		},
		"A data source of another type does not": {
			Type: TypePostgreSQL,
			Err:  assert.AnError,
		},
		"A Prometheus data source with no address": {
			Type:    TypePrometheus,
			Written: 1,
			Err:     assert.AnError,
		},
		"A connection the data source refused": {
			Type:    TypePrometheus,
			Handler: testutil.JSONHandler("/api/v1/status/buildinfo", http.StatusInternalServerError, `{}`),
			Written: 1,
			Tests:   1,
			Conns:   1,
			Err:     assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			var hits, conns atomic.Int32

			var url string
			if c.Handler != nil {
				url = prepPrometheusServer(t, countBuildinfo(c.Handler, &hits), &conns).URL
			}

			store := &StatusStoreMock{}
			r := prepRunner(t, c.Type, url, store)

			client, err := r.Prometheus(context.Background())

			testutil.AssertEqualError(t, c.Err, err)
			assert.Len(t, store.UpdateDataSourceStatusCalls(), c.Written)
			assert.Equal(t, c.Tests, hits.Load())

			if c.Err != nil {
				assert.Nil(t, client)

				// the next call gets the same answer without a new login.
				_, aerr := r.m.Runner(r.ds).Prometheus(context.Background())
				testutil.AssertEqualError(t, c.Err, aerr)
				assert.Len(t, store.UpdateDataSourceStatusCalls(), c.Written)
				assert.Equal(t, c.Tests, hits.Load())
				assert.Equal(t, c.Conns, conns.Load())

				return
			}

			require.NotNil(t, client)
			assert.IsType(t, cachedPrometheus{}, client)

			// the client the accessor hands back is the live one: every
			// read it offers reaches the data source.
			res, qerr := client.QueryRange(context.Background(), "up", _testTimeRange)
			require.NoError(t, qerr)
			assert.NotNil(t, res)

			meta, merr := client.Metadata(context.Background())
			require.NoError(t, merr)
			assert.NotNil(t, meta)

			names, nerr := client.LabelNames(context.Background(), nil, _testTimeRange)
			require.NoError(t, nerr)
			assert.NotNil(t, names)

			values, verr := client.LabelValues(context.Background(), "instance", nil, _testTimeRange)
			require.NoError(t, verr)
			assert.NotNil(t, values)

			series, serr := client.Series(context.Background(), []string{"up"}, _testTimeRange)
			require.NoError(t, serr)
			assert.NotNil(t, series)

			// the next call for the data source gets the same client, and a
			// test that passed a moment ago is not run again.
			again, aerr := r.m.Runner(r.ds).Prometheus(context.Background())
			require.NoError(t, aerr)
			assert.Equal(t, client, again)

			res, qerr = again.QueryRange(context.Background(), "up", _testTimeRange)
			require.NoError(t, qerr)
			assert.NotNil(t, res)

			assert.Equal(t, c.Tests, hits.Load())
			assert.Equal(t, c.Conns, conns.Load())
		})
	}
}

// Test_runner_Logs runs its cases one after another for the reason
// Test_runner_Prometheus does.
func Test_runner_Logs(t *testing.T) {
	cc := map[string]struct {
		Type    Type
		URL     string
		Handler http.Handler
		Written int
		Tests   int32
		Conns   int32
		Err     error
	}{
		// every read, and the next call's, shares one connection.
		"A Loki data source hands out its client": {
			Type:    TypeLoki,
			Handler: lokiAPIHandler(),
			Tests:   1,
			Conns:   1,
		},
		"The demo answers as Loki": {
			Type: TypeLoki,
			URL:  demo.URL,
		},
		"A data source of another type does not": {
			Type: TypePrometheus,
			Err:  assert.AnError,
		},
		"A Loki data source with no address": {
			Type:    TypeLoki,
			Written: 1,
			Err:     assert.AnError,
		},
		"A connection the data source refused": {
			Type:    TypeLoki,
			Handler: testutil.JSONHandler("/loki/api/v1/status/buildinfo", http.StatusUnauthorized, `{}`),
			Written: 1,
			Tests:   1,
			Conns:   1,
			Err:     assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			var hits, conns atomic.Int32

			url := c.URL
			if c.Handler != nil {
				url = prepLokiServer(t, countBuildinfo(c.Handler, &hits), &conns).URL
			}

			store := &StatusStoreMock{}
			r := prepRunner(t, c.Type, url, store)

			client, err := r.Logs(context.Background())

			testutil.AssertEqualError(t, c.Err, err)
			assert.Len(t, store.UpdateDataSourceStatusCalls(), c.Written)
			assert.Equal(t, c.Tests, hits.Load())

			if c.Err != nil {
				assert.Nil(t, client)
				assert.Equal(t, c.Conns, conns.Load())

				return
			}

			require.NotNil(t, client)
			assert.IsType(t, cachedLogs{}, client)

			// the client the accessor hands back is the live one: every
			// read it offers reaches the data source.
			lines, lerr := client.Logs(context.Background(), `{app="api"}`, _testTimeRange, processor.LogsPage{Limit: 10})
			require.NoError(t, lerr)
			assert.NotNil(t, lines)

			volume, verr := client.LogsVolume(context.Background(), `{app="api"}`, _testTimeRange)
			require.NoError(t, verr)
			assert.NotNil(t, volume)

			fields, ferr := client.LogFields(context.Background(), "", _testTimeRange)
			require.NoError(t, ferr)
			assert.NotEmpty(t, fields.Fields)

			values, aerr := client.LogFieldValues(context.Background(), "app", "", _testTimeRange)
			require.NoError(t, aerr)
			assert.NotNil(t, values)

			// the next call for the data source gets the same client, and a
			// test that passed a moment ago is not run again.
			again, gerr := r.m.Runner(r.ds).Logs(context.Background())
			require.NoError(t, gerr)
			assert.Equal(t, client, again)

			lines, lerr = again.Logs(context.Background(), `{app="api"}`, _testTimeRange, processor.LogsPage{Limit: 10})
			require.NoError(t, lerr)
			assert.NotNil(t, lines)

			assert.Equal(t, c.Tests, hits.Load())
			assert.Equal(t, c.Conns, conns.Load())
		})
	}
}

func Test_runner_PostgreSQL(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Type Type
		URL  string

		// Client, when set, is the client of a runner the manager already
		// keeps for the data source.
		Client  ownedClient
		Written int
		Err     error
	}{
		"A PostgreSQL data source hands out its cached client": {
			Type:   TypePostgreSQL,
			Client: passingPostgreSQL{},
		},
		"A Supabase data source hands out its cached client": {
			Type:   TypeSupabase,
			Client: passingPostgreSQL{},
		},
		// a PostgreSQL data source pointed nowhere still resolves its
		// processor; what fails is the connection, which the accessor
		// reports as the error it is.
		"A connection the data source refused": { //nolint:gosec // static test credentials
			Type:    TypePostgreSQL,
			URL:     "postgres://user:pass@127.0.0.1:1/db",
			Written: 1,
			Err:     assert.AnError,
		},
		"A data source of another type": {
			Type: TypePrometheus,
			Err:  assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			store := &StatusStoreMock{}
			r := prepRunner(t, c.Type, c.URL, store)

			if c.Client != nil {
				r.client = c.Client
			}

			client, err := r.PostgreSQL(context.Background())
			testutil.AssertEqualError(t, c.Err, err)
			assert.Len(t, store.UpdateDataSourceStatusCalls(), c.Written)

			if err != nil {
				return
			}

			assert.IsType(t, cachedPostgreSQL{}, client)
		})
	}
}

func Test_runner_MySQL(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Type Type
		URL  string

		// Client, when set, is the client of a runner the manager already
		// keeps for the data source.
		Client  ownedClient
		Written int
		Err     error
	}{
		"A MySQL data source hands out its cached client": {
			Type:   TypeMySQL,
			Client: passingMySQL{},
		},
		"A connection the MySQL data source refused": {
			Type:    TypeMySQL,
			URL:     "user:pass@tcp(127.0.0.1:1)/db",
			Written: 1,
			Err:     assert.AnError,
		},
		"A connection the MariaDB data source refused": {
			Type:    TypeMariaDB,
			URL:     "user:pass@tcp(127.0.0.1:1)/db",
			Written: 1,
			Err:     assert.AnError,
		},
		"A data source of another type": {
			Type: TypePrometheus,
			Err:  assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			store := &StatusStoreMock{}
			r := prepRunner(t, c.Type, c.URL, store)

			if c.Client != nil {
				r.client = c.Client
			}

			client, err := r.MySQL(context.Background())
			testutil.AssertEqualError(t, c.Err, err)
			assert.Len(t, store.UpdateDataSourceStatusCalls(), c.Written)

			if err != nil {
				return
			}

			assert.IsType(t, cachedMySQL{}, client)
		})
	}
}

func Test_runner_SQL(t *testing.T) {
	t.Parallel()

	// every SQL dialect resolves through the same accessor
	cc := map[string]struct {
		Type    Type
		Written int
		Err     error
	}{
		"A PostgreSQL data source with no address": {
			Type:    TypePostgreSQL,
			Written: 1,
			Err:     assert.AnError,
		},
		"A Supabase data source with no address": {
			Type:    TypeSupabase,
			Written: 1,
			Err:     assert.AnError,
		},
		"A MySQL data source with no address": {
			Type:    TypeMySQL,
			Written: 1,
			Err:     assert.AnError,
		},
		"A MariaDB data source with no address": {
			Type:    TypeMariaDB,
			Written: 1,
			Err:     assert.AnError,
		},
		"A data source of another type": {
			Type: TypePrometheus,
			Err:  assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			store := &StatusStoreMock{}

			_, err := prepRunner(t, c.Type, "", store).SQL(context.Background())
			testutil.AssertEqualError(t, c.Err, err)
			assert.Len(t, store.UpdateDataSourceStatusCalls(), c.Written)
		})
	}
}

func Test_connect(t *testing.T) {
	t.Parallel()

	srv := prepPrometheusServer(t, prometheusAPIHandler(), &atomic.Int32{})

	cc := map[string]struct {
		Type          Type
		URL           string
		Allowed       []Type
		Stored        processor.ConnectionStatus
		Undecryptable bool
		Rival         bool
		Recorded      []processor.ConnectionStatus
		Err           error
	}{
		// a status the connection reported is recorded on the way
		// through, so a data source read only by the assistant still has
		// an honest row.
		"A status the connection reported is recorded": {
			Type:     TypePrometheus,
			URL:      srv.URL,
			Allowed:  []Type{TypePrometheus},
			Stored:   processor.ConnectionStatusUnreachable,
			Recorded: []processor.ConnectionStatus{processor.ConnectionStatusSuccess},
		},
		// credentials that cannot be decrypted end the call with the
		// reason, which is recorded on the way out like any other status.
		"Credentials that cannot be decrypted": {
			Type:          TypePrometheus,
			URL:           srv.URL,
			Allowed:       []Type{TypePrometheus},
			Stored:        processor.ConnectionStatusSuccess,
			Undecryptable: true,
			Recorded:      []processor.ConnectionStatus{processor.ConnectionStatusInvalidEncryptionKey},
			Err:           processor.ConnectionStatusInvalidEncryptionKey.Error(),
		},
		// two calls for one version got runners before either connected,
		// and the one to connect second reads through the one kept first.
		"A call that lost the race to keep its runner": {
			Type:    TypePrometheus,
			URL:     srv.URL,
			Allowed: []Type{TypePrometheus},
			Stored:  processor.ConnectionStatusSuccess,
			Rival:   true,
		},
		"The wanted client is not one this data source serves": {
			Type:    TypePostgreSQL,
			Allowed: []Type{TypePrometheus},
			Stored:  processor.ConnectionStatusSuccess,
			Err:     assert.AnError,
		},
		"The data source has no processor at all": {
			Type:    Type("bogus"),
			Allowed: []Type{Type("bogus")},
			Stored:  processor.ConnectionStatusSuccess,
			Err:     assert.AnError,
		},
		"The client could not be built": {
			Type:    TypePrometheus,
			URL:     demo.Scheme + "nope",
			Allowed: []Type{TypePrometheus},
			Stored:  processor.ConnectionStatusSuccess,
			Err:     demo.ErrUnknownSource,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			store := &StatusStoreMock{}
			r := prepRunner(t, c.Type, c.URL, store)
			r.status = c.Stored

			if c.Undecryptable {
				r.ds.Credentials = undecryptableCredentials(t)
			}

			kept := r
			if c.Rival {
				rival, ok := r.m.Runner(r.ds).(*runner)
				require.True(t, ok)

				_, rerr := connect[Prometheus](context.Background(), rival, c.Allowed...)
				require.NoError(t, rerr)

				kept = rival
			}

			client, err := connect[Prometheus](context.Background(), r, c.Allowed...)
			testutil.AssertEqualError(t, c.Err, err)

			ff := store.UpdateDataSourceStatusCalls()
			require.Len(t, ff, len(c.Recorded))

			for i, status := range c.Recorded {
				assert.Equal(t, r.ds.ID, ff[i].ID)
				assert.Equal(t, "org", ff[i].OrganizationID)
				assert.Equal(t, status, ff[i].Status)
			}

			if err != nil {
				return
			}

			assert.NotNil(t, client)
			assert.Same(t, kept, r.m.runners[r.ds.ID])
		})
	}
}

func Test_runner_ensureTested(t *testing.T) {
	t.Parallel()

	type tcase struct {
		Client *stubClient

		// TestedAgo is how long before now the last test ran, or zero for
		// never, and Found is what it found.
		TestedAgo time.Duration
		Found     processor.ConnectionStatus

		// Released indicates whether the manager let go of the runner
		// before the call, and EvictedInTest whether it does so while the
		// call tests the connection.
		Released      bool
		EvictedInTest bool

		// Context is the caller's context, ended by Client when it tests
		// if Cancel is set.
		Context context.Context
		Cancel  context.CancelFunc

		// Held indicates whether another call holds the test lock until
		// the call gives up.
		Held bool

		// Callers is how many calls test at once.
		Callers int

		Tests   int32
		Written int
		Tested  bool
		Dropped bool
		Renewed bool
		Closes  int32
		Err     error
	}

	cc := map[string]tcase{
		"A runner never tested is tested": {
			Client:  &stubClient{status: processor.ConnectionStatusSuccess},
			Callers: 1,
			Tests:   1,
			Tested:  true,
		},
		"A test that passed a moment ago is not repeated": {
			Client:    &stubClient{status: processor.ConnectionStatusSuccess},
			TestedAgo: _statusTTL / 2,
			Found:     processor.ConnectionStatusSuccess,
			Callers:   1,
			Tested:    true,
		},
		"A test that failed a moment ago is not repeated": {
			Client:    &stubClient{status: processor.ConnectionStatusSuccess},
			TestedAgo: _statusTTL / 2,
			Found:     processor.ConnectionStatusUnreachable,
			Callers:   1,
			Tested:    true,
			Err:       processor.ConnectionStatusUnreachable.Error(),
		},
		"A test older than _statusTTL is repeated": {
			Client:    &stubClient{status: processor.ConnectionStatusSuccess},
			TestedAgo: _statusTTL,
			Found:     processor.ConnectionStatusSuccess,
			Callers:   1,
			Tests:     1,
			Tested:    true,
		},
		// each test of a misconfigured data source is a failed login.
		"A failure older than _statusTTL is not repeated": {
			Client:    &stubClient{status: processor.ConnectionStatusSuccess},
			TestedAgo: _statusTTL * 2,
			Found:     processor.ConnectionStatusUnreachable,
			Callers:   1,
			Tested:    true,
			Err:       processor.ConnectionStatusUnreachable.Error(),
		},
		"A failure older than _failureTTL is repeated": {
			Client:    &stubClient{status: processor.ConnectionStatusSuccess},
			TestedAgo: _failureTTL,
			Found:     processor.ConnectionStatusUnreachable,
			Callers:   1,
			Tests:     1,
			Tested:    true,
		},
		// the calls waiting behind the first test reuse what it found.
		"Calls at once share one test": {
			Client:  &stubClient{status: processor.ConnectionStatusSuccess},
			Callers: 5,
			Tests:   1,
			Tested:  true,
		},
		// the new runner remembers the failure, so it answers the calls that
		// follow, and its next test opens new connections.
		"A failing test renews the runner": {
			Client:  &stubClient{status: processor.ConnectionStatusUnreachable},
			Callers: 1,
			Tests:   1,
			Written: 1,
			Tested:  true,
			Renewed: true,
			Closes:  1,
			Err:     processor.ConnectionStatusUnreachable.Error(),
		},
		// the calls waiting behind a failing test get its answer rather
		// than each logging in again.
		"Calls at once share one failing test": {
			Client:  &stubClient{status: processor.ConnectionStatusUnreachable},
			Callers: 5,
			Tests:   1,
			Written: 1,
			Tested:  true,
			Renewed: true,
			Closes:  1,
			Err:     processor.ConnectionStatusUnreachable.Error(),
		},
		// the calls that waited behind a failing test still hold the
		// runner, and the failure is their answer.
		"A runner let go after a failing test": {
			Client:    &stubClient{status: processor.ConnectionStatusSuccess},
			TestedAgo: _statusTTL / 2,
			Found:     processor.ConnectionStatusUnreachable,
			Released:  true,
			Callers:   1,
			Tested:    true,
			Dropped:   true,
			Err:       processor.ConnectionStatusUnreachable.Error(),
		},
		// its connections are closed, so a pass from before must not hand
		// them out.
		"A runner let go after a passing test": {
			Client:    &stubClient{status: processor.ConnectionStatusSuccess},
			TestedAgo: _statusTTL / 2,
			Found:     processor.ConnectionStatusSuccess,
			Released:  true,
			Callers:   1,
			Tested:    true,
			Dropped:   true,
			Err:       _errDataSourceChanged,
		},
		// the test failed only because its connections were closed under
		// it, which says nothing about the data source.
		"A runner let go during its test": {
			Client:        &stubClient{status: processor.ConnectionStatusUnreachable},
			EvictedInTest: true,
			Callers:       1,
			Tests:         1,
			Dropped:       true,
			Closes:        1,
			Err:           _errDataSourceChanged,
		},
		// its connections are closed, so a test would only report them as
		// unreachable.
		"A runner the manager let go is not tested": {
			Client:   &stubClient{status: processor.ConnectionStatusSuccess},
			Released: true,
			Callers:  1,
			Dropped:  true,
			Err:      _errDataSourceChanged,
		},
		"Error returned by TestConnection": {
			Client:  &stubClient{err: assert.AnError},
			Callers: 1,
			Tests:   1,
			Err:     assert.AnError,
		},
		"A caller that gave up waiting for the test lock": func() tcase {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			return tcase{
				Client:  &stubClient{status: processor.ConnectionStatusSuccess},
				Context: ctx,
				Held:    true,
				Callers: 1,
				Err:     context.Canceled,
			}
		}(),
		// a test the caller gave up on says nothing about the data
		// source, so nothing is recorded or dropped.
		"A caller that gave up during the test": func() tcase {
			ctx, cancel := context.WithCancel(context.Background())

			return tcase{
				Client:  &stubClient{status: processor.ConnectionStatusUnreachable, onTest: cancel},
				Context: ctx,
				Cancel:  cancel,
				Callers: 1,
				Tests:   1,
				Err:     context.Canceled,
			}
		}(),
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			if c.Cancel != nil {
				t.Cleanup(c.Cancel)
			}

			ctx := c.Context
			if ctx == nil {
				ctx = context.Background()
			}

			store := &StatusStoreMock{}

			m := prepManager(t, store)
			r := prepStubRunner(m, c.Client, timeutil.Now())

			if c.TestedAgo > 0 {
				r.testedAt = timeutil.Now().Add(-c.TestedAgo)
				r.tested = c.Found
			}

			if c.Released {
				delete(m.runners, r.ds.ID)
			}

			if c.EvictedInTest {
				c.Client.onTest = func() { m.Evict(r.ds.ID) }
			}

			// holding the test lock lines every caller up behind it.
			unlock, err := m.tests.Lock(context.Background(), r.ds.ID)
			require.NoError(t, err)

			supv := xync.NewSupervisor()

			for range c.Callers {
				supv.Go(func(context.Context) {
					testutil.AssertEqualError(t, c.Err, r.ensureTested(ctx))
				})
			}

			if c.Held {
				supv.Wait()
			}

			unlock()
			supv.Wait()
			m.supv.Wait()

			assert.Equal(t, c.Tests, c.Client.tests.Load())
			assert.Len(t, store.UpdateDataSourceStatusCalls(), c.Written)
			assert.Equal(t, c.Tested, !r.testedAt.IsZero())

			held, ok := m.runners[r.ds.ID]
			assert.Equal(t, !c.Dropped, ok)
			assert.Equal(t, c.Closes, c.Client.closes.Load())

			if !ok {
				return
			}

			if !c.Renewed {
				assert.Same(t, r, held)

				return
			}

			assert.NotSame(t, r, held)
			assert.Equal(t, r.testedAt, held.testedAt)
			assert.Equal(t, r.tested, held.tested)
		})
	}
}

func Test_runner_recordStatus(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Stored   processor.ConnectionStatus
		Observed processor.ConnectionStatus
		Store    *StatusStoreMock
		NoStore  bool
		Written  int
	}{
		"A status that changed is written": {
			Stored:   processor.ConnectionStatusSuccess,
			Observed: processor.ConnectionStatusUnreachable,
			Written:  1,
		},
		"A status that did not change is not": {
			Stored:   processor.ConnectionStatusSuccess,
			Observed: processor.ConnectionStatusSuccess,
		},
		// the caller asked to read the data source, and what its row
		// says about connectivity is not the answer they waited for.
		"A write that fails is carried through": {
			Stored:   processor.ConnectionStatusSuccess,
			Observed: processor.ConnectionStatusUnauthorized,
			Store: &StatusStoreMock{
				UpdateDataSourceStatusFunc: func(context.Context, xid.ID, string, processor.ConnectionStatus) error {
					return assert.AnError
				},
			},
			Written: 1,
		},
		// a manager built without a store has nowhere to record and says
		// so by doing nothing.
		"A runner without a store records nothing": {
			Stored:   processor.ConnectionStatusSuccess,
			Observed: processor.ConnectionStatusUnreachable,
			NoStore:  true,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			store := c.Store
			if store == nil {
				store = &StatusStoreMock{}
			}

			r := prepRunner(t, TypePrometheus, "", store)
			r.status = c.Stored

			if c.NoStore {
				r.m.store = nil
			}

			r.recordStatus(context.Background(), c.Observed)

			assert.Len(t, store.UpdateDataSourceStatusCalls(), c.Written)
			assert.Equal(t, c.Observed, r.status)
		})
	}
}
