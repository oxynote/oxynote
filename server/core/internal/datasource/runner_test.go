package datasource

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/oxynote/oxynote/server/core/internal/datasource/demo"
	"github.com/oxynote/oxynote/server/core/internal/datasource/processor"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
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

// discardLog returns a logger that writes nowhere.
func discardLog() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

// prepPrometheusServer starts a fake Prometheus API server serving the
// provided handler. Its cleanup also drops the Prometheus client's shared
// keep-alive connections so goleak stays clean.
func prepPrometheusServer(t *testing.T, h http.Handler) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(h)

	t.Cleanup(func() {
		srv.Close()
		api.DefaultRoundTripper.(*http.Transport).CloseIdleConnections()
	})

	return srv
}

// prometheusBuildinfoHandler serves the Prometheus buildinfo endpoint with
// the given status code and version, 404-ing every other path.
func prometheusBuildinfoHandler(code int, version string) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/v1/status/buildinfo", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		fmt.Fprintf(w, `{"status":"success","data":{"version":"%s"}}`, version) //nolint:errcheck // error provides no meaningful info
	})

	return mux
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

// prepRunner builds the runner for a data source of the given type,
// pointed at the given URL, over a store that records what it was told.
func prepRunner(typ Type, url string, store *StatusStoreMock) *runner {
	if store == nil {
		store = &StatusStoreMock{}
	}

	m := NewManager(discardLog(), store)

	r, ok := m.Runner(DataSource{
		ID:             xid.New(),
		OrganizationID: "org",
		Name:           "prod",
		Type:           typ,
		URL:            url,
		Status:         processor.ConnectionStatusSuccess,
	}).(*runner)
	if !ok {
		panic("the manager built something other than a runner")
	}

	return r
}

func Test_NewManager(t *testing.T) {
	t.Parallel()

	store := &StatusStoreMock{}

	m := NewManager(discardLog(), store)
	require.NotNil(t, m)
	assert.NotNil(t, m.log)
	assert.Same(t, store, m.store)
}

func Test_Manager_Runner(t *testing.T) {
	t.Parallel()

	store := &StatusStoreMock{}
	ds := DataSource{Type: TypePrometheus, URL: "http://prometheus.test"}

	m := NewManager(discardLog(), store)

	r, ok := m.Runner(ds).(*runner)
	require.True(t, ok)

	// the runner carries the data source it operates and shares the
	// manager's store; anything longer-lived than one call stays there.
	assert.Equal(t, ds, r.ds)
	assert.Same(t, store, r.store)
	assert.False(t, r.prepared)
	assert.Nil(t, r.client)

	// the demo client is one of those longer-lived things: it caches the
	// history it replays, so every runner has to read the manager's
	// rather than build one of its own.
	require.NotNil(t, r.demo)

	second, ok := m.Runner(ds).(*runner)
	require.True(t, ok)
	assert.Same(t, r.demo, second.demo)
}

func Test_runner_Type(t *testing.T) {
	t.Parallel()

	assert.Equal(t, TypeMariaDB, prepRunner(TypeMariaDB, "", nil).Type())
}

func Test_runner_TestConnection(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Type          Type
		Handler       http.Handler
		Undecryptable bool
		Result        processor.ConnectionStatus
		Err           error
	}{
		"A type with no processor behind it": {
			Type: Type("bogus"),
			Err:  assert.AnError,
		},
		"Credentials that cannot be decrypted": {
			Type:          TypePrometheus,
			Undecryptable: true,
			Result:        processor.ConnectionStatusInvalidSigningSecret,
		},
		"A connection that answers": {
			Type:    TypePrometheus,
			Handler: prometheusBuildinfoHandler(http.StatusOK, "2.50.0"),
			Result:  processor.ConnectionStatusSuccess,
		},
		"A connection that does not": {
			Type:    TypePrometheus,
			Handler: prometheusBuildinfoHandler(http.StatusInternalServerError, ""),
			Result:  processor.ConnectionStatusUnreachable,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var url string
			if c.Handler != nil {
				url = prepPrometheusServer(t, c.Handler).URL
			}

			store := &StatusStoreMock{}

			r := prepRunner(c.Type, url, store)

			if c.Undecryptable {
				r.ds.Credentials = undecryptableCredentials(t)
			}

			cs, err := r.TestConnection(context.Background())

			testutil.AssertEqualError(t, c.Err, err)

			if c.Err != nil {
				return
			}

			assert.Equal(t, c.Result, cs)

			// an explicit test is asked about a data source that is
			// being created or changed, so what it finds is reported and
			// not recorded.
			assert.Empty(t, store.UpdateDataSourceStatusCalls())
		})
	}
}

func Test_runner_Prometheus(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Type    Type
		Handler http.Handler
		Err     error
	}{
		"A Prometheus data source hands out its client": {
			Type:    TypePrometheus,
			Handler: prometheusAPIHandler(),
		},
		"A data source of another type does not": {
			Type: TypePostgreSQL,
			Err:  assert.AnError,
		},
		"A type with no processor behind it": {
			Type: TypePrometheus,
			Err:  assert.AnError,
		},
		"A connection the data source refused": {
			Type:    TypePrometheus,
			Handler: prometheusBuildinfoHandler(http.StatusInternalServerError, ""),
			Err:     assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var url string
			if c.Handler != nil {
				url = prepPrometheusServer(t, c.Handler).URL
			}

			client, err := prepRunner(c.Type, url, nil).Prometheus(context.Background())

			testutil.AssertEqualError(t, c.Err, err)

			if c.Err != nil {
				assert.Nil(t, client)
				return
			}

			require.NotNil(t, client)

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
		})
	}
}

func Test_runner_PostgreSQL(t *testing.T) {
	t.Parallel()

	// a PostgreSQL data source pointed nowhere still resolves its
	// processor; what fails is the connection, which the accessor
	// reports as the error it is.
	_, err := prepRunner(TypePostgreSQL, "postgres://user:pass@127.0.0.1:1/db", nil).
		PostgreSQL(context.Background())
	require.Error(t, err)

	_, err = prepRunner(TypePrometheus, "", nil).PostgreSQL(context.Background())
	require.Error(t, err)
}

func Test_runner_MySQL(t *testing.T) {
	t.Parallel()

	for _, typ := range []Type{TypeMySQL, TypeMariaDB} {
		_, err := prepRunner(typ, "user:pass@tcp(127.0.0.1:1)/db", nil).MySQL(context.Background())
		require.Error(t, err, "type %s", typ)
	}

	_, err := prepRunner(TypePrometheus, "", nil).MySQL(context.Background())
	require.Error(t, err)
}

func Test_runner_SQL(t *testing.T) {
	t.Parallel()

	// every SQL dialect resolves through the same accessor
	for _, typ := range []Type{TypePostgreSQL, TypeMySQL, TypeMariaDB} {
		_, err := prepRunner(typ, "", nil).SQL(context.Background())
		require.Error(t, err, "type %s", typ)
	}

	_, err := prepRunner(TypePrometheus, "", nil).SQL(context.Background())
	require.Error(t, err)
}

func Test_connect(t *testing.T) {
	t.Parallel()

	srv := prepPrometheusServer(t, prometheusAPIHandler())

	// a status the connection reported is recorded on the way through,
	// so a data source read only by the assistant still has an honest
	// row.
	store := &StatusStoreMock{}
	r := prepRunner(TypePrometheus, srv.URL, store)
	r.ds.Status = processor.ConnectionStatusUnreachable

	client, err := connect[Prometheus](context.Background(), r, TypePrometheus)
	require.NoError(t, err)
	assert.NotNil(t, client)

	ff := store.UpdateDataSourceStatusCalls()
	require.Len(t, ff, 1)
	assert.Equal(t, r.ds.ID, ff[0].Id)
	assert.Equal(t, "org", ff[0].OrganizationID)
	assert.Equal(t, processor.ConnectionStatusSuccess, ff[0].Status)

	// credentials that cannot be decrypted end the call with the reason,
	// which is recorded on the way out like any other status.
	store = &StatusStoreMock{}
	r = prepRunner(TypePrometheus, srv.URL, store)
	r.ds.Credentials = undecryptableCredentials(t)

	_, err = connect[Prometheus](context.Background(), r, TypePrometheus)
	assert.Equal(t, processor.ConnectionStatusInvalidSigningSecret.Error(), err)

	ff = store.UpdateDataSourceStatusCalls()
	require.Len(t, ff, 1)
	assert.Equal(t, processor.ConnectionStatusInvalidSigningSecret, ff[0].Status)

	// the wanted client is not one this data source serves
	_, err = connect[Prometheus](context.Background(), prepRunner(TypePostgreSQL, "", nil), TypePrometheus)
	require.Error(t, err)

	// the data source has no processor at all
	_, err = connect[Prometheus](context.Background(), prepRunner(Type("bogus"), "", nil), Type("bogus"))
	require.Error(t, err)
}

func Test_runner_recordStatus(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Stored   processor.ConnectionStatus
		Observed processor.ConnectionStatus
		Store    *StatusStoreMock
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
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			store := c.Store
			if store == nil {
				store = &StatusStoreMock{}
			}

			r := prepRunner(TypePrometheus, "", store)
			r.ds.Status = c.Stored

			r.recordStatus(context.Background(), c.Observed)

			assert.Len(t, store.UpdateDataSourceStatusCalls(), c.Written)
		})
	}

	// a runner built without a store has nowhere to record and says so
	// by doing nothing.
	r := &runner{log: discardLog(), ds: DataSource{Status: processor.ConnectionStatusSuccess}}
	r.recordStatus(context.Background(), processor.ConnectionStatusUnreachable)
}

func Test_runner_ensurePrepared(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Type Type
		URL  string
		Err  error
	}{
		"Prometheus":          {Type: TypePrometheus},
		"Demo Prometheus":     {Type: TypePrometheus, URL: demo.URL},
		"Unknown demo source": {Type: TypePrometheus, URL: demo.Scheme + "nope", Err: demo.ErrUnknownSource},
		"PostgreSQL":          {Type: TypePostgreSQL},
		"MySQL":               {Type: TypeMySQL},
		"MariaDB":             {Type: TypeMariaDB},
		"Unknown type":        {Type: Type("bogus"), Err: assert.AnError},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			r := prepRunner(c.Type, c.URL, nil)

			err := r.ensurePrepared()

			testutil.AssertEqualError(t, c.Err, err)

			if c.Err != nil {
				assert.False(t, r.prepared)
				return
			}

			require.True(t, r.prepared)
			require.NotNil(t, r.client)

			// preparing twice keeps the processor already built
			client := r.client
			require.NoError(t, r.ensurePrepared())
			assert.Same(t, client, r.client)
		})
	}
}

func Test_runner_prometheusClient(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		URL    string
		Client connectionTester
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

			client, err := prepRunner(TypePrometheus, c.URL, nil).prometheusClient()
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				assert.Nil(t, client)
				return
			}

			assert.IsType(t, c.Client, client)
		})
	}
}
