package datasource

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/oxynote/oxynote/server/core/internal/datasource/processor"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// _testTimeRange is a fixed time range reused across executor tests.
var _testTimeRange = processor.TimeRange{
	From: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	To:   time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
}

// prometheusAPIHandler serves canned successful responses for every
// Prometheus API endpoint the executor exercises.
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

func Test_NewExecutor(t *testing.T) {
	t.Parallel()

	require.NotNil(t, NewExecutor())
}

func Test_Executor_TestConnection(t *testing.T) {
	cc := map[string]struct {
		Type    Type
		Handler http.Handler
		Result  processor.ConnectionStatus
		Err     error
	}{
		"Error returned by runner.TestConnection": {
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

			ds := DataSource{Type: c.Type}

			if c.Handler != nil {
				ds.URL = prepPrometheusServer(t, c.Handler).URL
			}

			cs, err := NewExecutor().TestConnection(context.Background(), ds)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, cs)
		})
	}
}

func Test_Executor_PrometheusQuery(t *testing.T) {
	cc := map[string]struct {
		Type      Type
		Handler   http.Handler
		Status    processor.ConnectionStatus
		NilResult bool
		Err       error
	}{
		"Error returned by runner.Prometheus": {
			Type: TypeMySQL,
			Err:  errutil.ErrNotFound,
		},
		"Unsuccessful connection test": {
			Type:      TypePrometheus,
			Handler:   prometheusBuildinfoHandler(http.StatusInternalServerError, ""),
			Status:    processor.ConnectionStatusUnreachable,
			NilResult: true,
		},
		"Error returned by prom.QueryRange": {
			Type:    TypePrometheus,
			Handler: prometheusBuildinfoHandler(http.StatusOK, "2.50.0"),
			Err:     assert.AnError,
		},
		"Successful query": {
			Type:    TypePrometheus,
			Handler: prometheusAPIHandler(),
			Status:  processor.ConnectionStatusSuccess,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			ds := DataSource{Type: c.Type}

			if c.Handler != nil {
				ds.URL = prepPrometheusServer(t, c.Handler).URL
			}

			cs, result, err := NewExecutor().PrometheusQuery(context.Background(), ds, "up", _testTimeRange)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Status, cs)

			if c.NilResult {
				assert.Nil(t, result)
				return
			}

			require.NotNil(t, result)
			assert.Equal(t, model.ValMatrix, result.Type)
			assert.NotNil(t, result.Result)
		})
	}
}

func Test_Executor_PrometheusMetadata(t *testing.T) {
	cc := map[string]struct {
		Type      Type
		Handler   http.Handler
		Status    processor.ConnectionStatus
		NilResult bool
		Err       error
	}{
		"Error returned by runner.Prometheus": {
			Type: TypeMySQL,
			Err:  errutil.ErrNotFound,
		},
		"Unsuccessful connection test": {
			Type:      TypePrometheus,
			Handler:   prometheusBuildinfoHandler(http.StatusInternalServerError, ""),
			Status:    processor.ConnectionStatusUnreachable,
			NilResult: true,
		},
		"Error returned by prom.Metadata": {
			Type:    TypePrometheus,
			Handler: prometheusBuildinfoHandler(http.StatusOK, "2.50.0"),
			Err:     assert.AnError,
		},
		"Successful metadata retrieval": {
			Type:    TypePrometheus,
			Handler: prometheusAPIHandler(),
			Status:  processor.ConnectionStatusSuccess,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			ds := DataSource{Type: c.Type}

			if c.Handler != nil {
				ds.URL = prepPrometheusServer(t, c.Handler).URL
			}

			cs, result, err := NewExecutor().PrometheusMetadata(context.Background(), ds)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Status, cs)

			if c.NilResult {
				assert.Nil(t, result)
				return
			}

			require.NotNil(t, result)
			assert.NotNil(t, result.Result)
		})
	}
}

func Test_Executor_PrometheusLabelNames(t *testing.T) {
	cc := map[string]struct {
		Type      Type
		Handler   http.Handler
		Status    processor.ConnectionStatus
		NilResult bool
		Result    []string
		Err       error
	}{
		"Error returned by runner.Prometheus": {
			Type: TypeMySQL,
			Err:  errutil.ErrNotFound,
		},
		"Unsuccessful connection test": {
			Type:      TypePrometheus,
			Handler:   prometheusBuildinfoHandler(http.StatusInternalServerError, ""),
			Status:    processor.ConnectionStatusUnreachable,
			NilResult: true,
		},
		"Error returned by prom.LabelNames": {
			Type:    TypePrometheus,
			Handler: prometheusBuildinfoHandler(http.StatusOK, "2.50.0"),
			Err:     assert.AnError,
		},
		"Successful label names retrieval": {
			Type:    TypePrometheus,
			Handler: prometheusAPIHandler(),
			Status:  processor.ConnectionStatusSuccess,
			Result:  []string{"__name__", "job"},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			ds := DataSource{Type: c.Type}

			if c.Handler != nil {
				ds.URL = prepPrometheusServer(t, c.Handler).URL
			}

			cs, result, err := NewExecutor().PrometheusLabelNames(context.Background(), ds, []string{"up"}, _testTimeRange)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Status, cs)

			if c.NilResult {
				assert.Nil(t, result)
				return
			}

			require.NotNil(t, result)
			assert.Equal(t, c.Result, result.Result)
		})
	}
}

func Test_Executor_PrometheusLabelValues(t *testing.T) {
	cc := map[string]struct {
		Type      Type
		Handler   http.Handler
		Status    processor.ConnectionStatus
		NilResult bool
		Result    []string
		Err       error
	}{
		"Error returned by runner.Prometheus": {
			Type: TypeMySQL,
			Err:  errutil.ErrNotFound,
		},
		"Unsuccessful connection test": {
			Type:      TypePrometheus,
			Handler:   prometheusBuildinfoHandler(http.StatusInternalServerError, ""),
			Status:    processor.ConnectionStatusUnreachable,
			NilResult: true,
		},
		"Error returned by prom.LabelValues": {
			Type:    TypePrometheus,
			Handler: prometheusBuildinfoHandler(http.StatusOK, "2.50.0"),
			Err:     assert.AnError,
		},
		"Successful label values retrieval": {
			Type:    TypePrometheus,
			Handler: prometheusAPIHandler(),
			Status:  processor.ConnectionStatusSuccess,
			Result:  []string{"a", "b"},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			ds := DataSource{Type: c.Type}

			if c.Handler != nil {
				ds.URL = prepPrometheusServer(t, c.Handler).URL
			}

			cs, result, err := NewExecutor().PrometheusLabelValues(context.Background(), ds, "instance", []string{"up"}, _testTimeRange)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Status, cs)

			if c.NilResult {
				assert.Nil(t, result)
				return
			}

			require.NotNil(t, result)
			assert.Equal(t, c.Result, result.Result)
		})
	}
}

func Test_Executor_PrometheusSeries(t *testing.T) {
	cc := map[string]struct {
		Type      Type
		Handler   http.Handler
		Status    processor.ConnectionStatus
		NilResult bool
		Result    []model.LabelSet
		Err       error
	}{
		"Error returned by runner.Prometheus": {
			Type: TypeMySQL,
			Err:  errutil.ErrNotFound,
		},
		"Unsuccessful connection test": {
			Type:      TypePrometheus,
			Handler:   prometheusBuildinfoHandler(http.StatusInternalServerError, ""),
			Status:    processor.ConnectionStatusUnreachable,
			NilResult: true,
		},
		"Error returned by prom.Series": {
			Type:    TypePrometheus,
			Handler: prometheusBuildinfoHandler(http.StatusOK, "2.50.0"),
			Err:     assert.AnError,
		},
		"Successful series retrieval": {
			Type:    TypePrometheus,
			Handler: prometheusAPIHandler(),
			Status:  processor.ConnectionStatusSuccess,
			Result: []model.LabelSet{
				{"__name__": "up", "job": "prometheus"},
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			ds := DataSource{Type: c.Type}

			if c.Handler != nil {
				ds.URL = prepPrometheusServer(t, c.Handler).URL
			}

			cs, result, err := NewExecutor().PrometheusSeries(context.Background(), ds, []string{"up"}, _testTimeRange)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Status, cs)

			if c.NilResult {
				assert.Nil(t, result)
				return
			}

			require.NotNil(t, result)
			assert.Equal(t, c.Result, result.Result)
		})
	}
}

func Test_Executor_MySQLQuery(t *testing.T) {
	cc := map[string]struct {
		Type   Type
		URL    string
		Status processor.ConnectionStatus
		Err    error
	}{
		"Error returned by runner.MySQL": {
			Type: TypePrometheus,
			Err:  errutil.ErrNotFound,
		},
		"Unsuccessful connection test": {
			Type:   TypeMySQL,
			URL:    "mysql://127.0.0.1:1/db",
			Status: processor.ConnectionStatusUnreachable,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			ds := DataSource{Type: c.Type, URL: c.URL}

			cs, result, err := NewExecutor().MySQLQuery(context.Background(), ds, "SELECT 1", _testTimeRange)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Status, cs)
			assert.Nil(t, result)
		})
	}
}

func Test_Executor_PostgreSQLQuery(t *testing.T) {
	cc := map[string]struct {
		Type   Type
		URL    string
		Status processor.ConnectionStatus
		Err    error
	}{
		"Error returned by runner.PostgreSQL": {
			Type: TypePrometheus,
			Err:  errutil.ErrNotFound,
		},
		"Unsuccessful connection test": {
			Type:   TypePostgreSQL,
			URL:    "postgres://127.0.0.1:1/db",
			Status: processor.ConnectionStatusUnreachable,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			ds := DataSource{Type: c.Type, URL: c.URL}

			cs, result, err := NewExecutor().PostgreSQLQuery(context.Background(), ds, "SELECT 1", _testTimeRange)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Status, cs)
			assert.Nil(t, result)
		})
	}
}

func Test_Executor_SQLQueryLabels(t *testing.T) {
	cc := map[string]struct {
		Type   Type
		URL    string
		Status processor.ConnectionStatus
		Err    error
	}{
		"Error returned by runner.SQL": {
			Type: TypePrometheus,
			Err:  errutil.ErrNotFound,
		},
		"Unsuccessful connection test": {
			Type:   TypeMySQL,
			URL:    "mysql://127.0.0.1:1/db",
			Status: processor.ConnectionStatusUnreachable,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			ds := DataSource{Type: c.Type, URL: c.URL}

			cs, result, err := NewExecutor().SQLQueryLabels(context.Background(), ds, "SELECT 1", _testTimeRange)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Status, cs)
			assert.Nil(t, result)
		})
	}
}

func Test_Executor_SQLMetadata(t *testing.T) {
	cc := map[string]struct {
		Type   Type
		URL    string
		Status processor.ConnectionStatus
		Err    error
	}{
		"Error returned by runner.SQL": {
			Type: TypePrometheus,
			Err:  errutil.ErrNotFound,
		},
		"Unsuccessful connection test": {
			Type:   TypePostgreSQL,
			URL:    "postgres://127.0.0.1:1/db",
			Status: processor.ConnectionStatusUnreachable,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			ds := DataSource{Type: c.Type, URL: c.URL}

			cs, result, err := NewExecutor().SQLMetadata(context.Background(), ds)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Status, cs)
			assert.Nil(t, result)
		})
	}
}
