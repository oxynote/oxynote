package datasource

import (
	"context"
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

	m := NewManager(slog.New(slog.DiscardHandler), store)

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

	m := NewManager(slog.New(slog.DiscardHandler), store)
	require.NotNil(t, m)
	assert.NotNil(t, m.log)
	assert.Same(t, store, m.store)
	assert.NotNil(t, m.demo)
}

func Test_Manager_Runner(t *testing.T) {
	t.Parallel()

	store := &StatusStoreMock{}
	ds := DataSource{Type: TypePrometheus, URL: "http://prometheus.test"}

	m := NewManager(slog.New(slog.DiscardHandler), store)

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
			Result:        processor.ConnectionStatusInvalidEncryptionKey,
		},
		"A connection that answers": {
			Type:    TypePrometheus,
			Handler: testutil.JSONHandler("/api/v1/status/buildinfo", http.StatusOK, `{"status":"success","data":{"version":"2.50.0"}}`),
			Result:  processor.ConnectionStatusSuccess,
		},
		"A connection that does not": {
			Type:    TypePrometheus,
			Handler: testutil.JSONHandler("/api/v1/status/buildinfo", http.StatusInternalServerError, `{}`),
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

			// an explicit test is asked about a data source that is
			// being created or changed, so what it finds is reported and
			// not recorded.
			assert.Empty(t, store.UpdateDataSourceStatusCalls())

			if c.Err != nil {
				return
			}

			assert.Equal(t, c.Result, cs)
		})
	}
}

func Test_runner_Prometheus(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Type    Type
		Handler http.Handler
		Written int
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
		"A Prometheus data source with no address": {
			Type:    TypePrometheus,
			Written: 1,
			Err:     assert.AnError,
		},
		"A connection the data source refused": {
			Type:    TypePrometheus,
			Handler: testutil.JSONHandler("/api/v1/status/buildinfo", http.StatusInternalServerError, `{}`),
			Written: 1,
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

			store := &StatusStoreMock{}

			client, err := prepRunner(c.Type, url, store).Prometheus(context.Background())

			testutil.AssertEqualError(t, c.Err, err)
			assert.Len(t, store.UpdateDataSourceStatusCalls(), c.Written)

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

	cc := map[string]struct {
		Type    Type
		URL     string
		Written int
		Err     error
	}{
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

			_, err := prepRunner(c.Type, c.URL, store).PostgreSQL(context.Background())
			testutil.AssertEqualError(t, c.Err, err)
			assert.Len(t, store.UpdateDataSourceStatusCalls(), c.Written)
		})
	}
}

func Test_runner_MySQL(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Type    Type
		URL     string
		Written int
		Err     error
	}{
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

			_, err := prepRunner(c.Type, c.URL, store).MySQL(context.Background())
			testutil.AssertEqualError(t, c.Err, err)
			assert.Len(t, store.UpdateDataSourceStatusCalls(), c.Written)
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

			_, err := prepRunner(c.Type, "", store).SQL(context.Background())
			testutil.AssertEqualError(t, c.Err, err)
			assert.Len(t, store.UpdateDataSourceStatusCalls(), c.Written)
		})
	}
}

func Test_connect(t *testing.T) {
	t.Parallel()

	srv := prepPrometheusServer(t, prometheusAPIHandler())

	cc := map[string]struct {
		Type          Type
		URL           string
		Allowed       []Type
		Stored        processor.ConnectionStatus
		Undecryptable bool
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
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			store := &StatusStoreMock{}
			r := prepRunner(c.Type, c.URL, store)
			r.ds.Status = c.Stored

			if c.Undecryptable {
				r.ds.Credentials = undecryptableCredentials(t)
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
		// a runner built without a store has nowhere to record and says
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

			r := prepRunner(TypePrometheus, "", store)
			r.ds.Status = c.Stored

			if c.NoStore {
				r.store = nil
			}

			r.recordStatus(context.Background(), c.Observed)

			assert.Len(t, store.UpdateDataSourceStatusCalls(), c.Written)
		})
	}
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
