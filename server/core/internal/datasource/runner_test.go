package datasource

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/datasource/processor"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/prometheus/client_golang/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func Test_NewRunner(t *testing.T) {
	t.Parallel()

	r := NewRunner(DataSource{
		Type:        TypePrometheus,
		URL:         "http://prometheus.test",
		Credentials: processor.Credentials(`{"username":"user"}`),
	})

	assert.Equal(t, TypePrometheus, r.Type)
	assert.Equal(t, "http://prometheus.test", r.URL)
	assert.Equal(t, processor.Credentials(`{"username":"user"}`), r.Credentials)
	assert.False(t, r.prepared)
	assert.Nil(t, r.runner)
}

func Test_Runner_TestConnection(t *testing.T) {
	cc := map[string]struct {
		Type    Type
		Handler http.Handler
		Result  processor.ConnectionStatus
		Err     error
	}{
		"Error returned by ensurePrepared": {
			Type: Type("bogus"),
			Err:  assert.AnError,
		},
		"Successful connection": {
			Type:    TypePrometheus,
			Handler: prometheusBuildinfoHandler(http.StatusOK, "2.50.0"),
			Result:  processor.ConnectionStatusSuccess,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			r := &Runner{Type: c.Type}

			if c.Handler != nil {
				r.URL = prepPrometheusServer(t, c.Handler).URL
			}

			cs, err := r.TestConnection(context.Background())
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, cs)
		})
	}
}

func Test_Runner_Prometheus(t *testing.T) {
	cc := map[string]struct {
		Type    Type
		URL     string
		Handler http.Handler
		Result  processor.ConnectionStatus
		Err     error
	}{
		"Invalid data source type": {
			Type: TypePostgreSQL,
			Err:  errutil.ErrNotFound,
		},
		"Error returned by runner.TestConnection": {
			Type: TypePrometheus,
			URL:  "://",
			Err:  assert.AnError,
		},
		"Unreachable data source": {
			Type:    TypePrometheus,
			Handler: prometheusBuildinfoHandler(http.StatusInternalServerError, ""),
			Result:  processor.ConnectionStatusUnreachable,
		},
		"Successful client creation": {
			Type:    TypePrometheus,
			Handler: prometheusBuildinfoHandler(http.StatusOK, "2.50.0"),
			Result:  processor.ConnectionStatusSuccess,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			r := &Runner{Type: c.Type, URL: c.URL}

			if c.Handler != nil {
				r.URL = prepPrometheusServer(t, c.Handler).URL
			}

			prom, cs, err := r.Prometheus(context.Background())
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.IsType(t, &processor.Prometheus{}, prom)
			assert.Equal(t, c.Result, cs)
		})
	}
}

func Test_Runner_PostgreSQL(t *testing.T) {
	cc := map[string]struct {
		Type   Type
		URL    string
		Result processor.ConnectionStatus
		Err    error
	}{
		"Invalid data source type": {
			Type: TypePrometheus,
			Err:  errutil.ErrNotFound,
		},
		"Unreachable data source": {
			Type:   TypePostgreSQL,
			URL:    "postgres://127.0.0.1:1/db",
			Result: processor.ConnectionStatusUnreachable,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			r := &Runner{Type: c.Type, URL: c.URL}

			pg, cs, err := r.PostgreSQL(context.Background())
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.IsType(t, &processor.PostgreSQL{}, pg)
			assert.Equal(t, c.Result, cs)
		})
	}
}

func Test_Runner_MySQL(t *testing.T) {
	cc := map[string]struct {
		Type   Type
		URL    string
		Result processor.ConnectionStatus
		Err    error
	}{
		"Invalid data source type": {
			Type: TypePrometheus,
			Err:  errutil.ErrNotFound,
		},
		"Unreachable mariadb data source": {
			Type:   TypeMariaDB,
			URL:    "mysql://127.0.0.1:1/db",
			Result: processor.ConnectionStatusUnreachable,
		},
		"Unreachable mysql data source": {
			Type:   TypeMySQL,
			URL:    "mysql://127.0.0.1:1/db",
			Result: processor.ConnectionStatusUnreachable,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			r := &Runner{Type: c.Type, URL: c.URL}

			mdb, cs, err := r.MySQL(context.Background())
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.IsType(t, &processor.MySQL{}, mdb)
			assert.Equal(t, c.Result, cs)
		})
	}
}

func Test_Runner_SQL(t *testing.T) {
	cc := map[string]struct {
		Type   Type
		URL    string
		Runner any
		Result processor.ConnectionStatus
		Err    error
	}{
		"Invalid data source type": {
			Type: TypePrometheus,
			Err:  errutil.ErrNotFound,
		},
		"PostgreSQL data source": {
			Type:   TypePostgreSQL,
			URL:    "postgres://127.0.0.1:1/db",
			Runner: &processor.PostgreSQL{},
			Result: processor.ConnectionStatusUnreachable,
		},
		"MariaDB data source": {
			Type:   TypeMariaDB,
			URL:    "mysql://127.0.0.1:1/db",
			Runner: &processor.MySQL{},
			Result: processor.ConnectionStatusUnreachable,
		},
		"MySQL data source": {
			Type:   TypeMySQL,
			URL:    "mysql://127.0.0.1:1/db",
			Runner: &processor.MySQL{},
			Result: processor.ConnectionStatusUnreachable,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			r := &Runner{Type: c.Type, URL: c.URL}

			sqlDS, cs, err := r.SQL(context.Background())
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.IsType(t, c.Runner, sqlDS)
			assert.Equal(t, c.Result, cs)
		})
	}
}

func Test_Runner_ensurePrepared(t *testing.T) {
	cc := map[string]struct {
		Runner *Runner
		Result any
		Err    error
	}{
		"Invalid data source type": {
			Runner: &Runner{Type: Type("bogus")},
			Err:    assert.AnError,
		},
		"Already prepared": {
			Runner: &Runner{Type: TypePrometheus, prepared: true},
		},
		"Prometheus data source": {
			Runner: &Runner{Type: TypePrometheus},
			Result: &processor.Prometheus{},
		},
		"PostgreSQL data source": {
			Runner: &Runner{Type: TypePostgreSQL},
			Result: &processor.PostgreSQL{},
		},
		"MariaDB data source": {
			Runner: &Runner{Type: TypeMariaDB},
			Result: &processor.MySQL{},
		},
		"MySQL data source": {
			Runner: &Runner{Type: TypeMySQL},
			Result: &processor.MySQL{},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			err := c.Runner.ensurePrepared()
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				assert.False(t, c.Runner.prepared)
				return
			}

			assert.True(t, c.Runner.prepared)

			if c.Result == nil {
				// the already-prepared runner must be left untouched.
				assert.Nil(t, c.Runner.runner)
				return
			}

			require.NotNil(t, c.Runner.runner)
			assert.IsType(t, c.Result, c.Runner.runner)
		})
	}
}
