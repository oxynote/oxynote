package datasourcehandler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/datasource"
	"github.com/oxynote/oxynote/server/core/internal/datasource/processor"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"
)

// _testGenericQueryTarget is the request target carrying a query, a chart
// type, and a complete time range.
const _testGenericQueryTarget = "http://test.com/?q=up&chartType=line_chart&from=2024-01-01T00:00:00Z&to=2024-01-02T00:00:00Z"

// stubPrometheusQueryExec returns an executor mock whose PrometheusQuery
// yields the provided values.
func stubPrometheusQueryExec(cs processor.ConnectionStatus, result *processor.PrometheusQueryResult, err error) *ExecutorMock {
	return &ExecutorMock{
		PrometheusQueryFunc: func(_ context.Context, _ datasource.DataSource, _ string, _ processor.TimeRange) (processor.ConnectionStatus, *processor.PrometheusQueryResult, error) {
			return cs, result, err
		},
	}
}

// stubMySQLQueryExec returns an executor mock whose MySQLQuery yields the
// provided values.
func stubMySQLQueryExec(cs processor.ConnectionStatus, result *processor.MySQLQueryResult, err error) *ExecutorMock {
	return &ExecutorMock{
		MySQLQueryFunc: func(_ context.Context, _ datasource.DataSource, _ string, _ processor.TimeRange) (processor.ConnectionStatus, *processor.MySQLQueryResult, error) {
			return cs, result, err
		},
	}
}

// stubPostgreSQLQueryExec returns an executor mock whose PostgreSQLQuery
// yields the provided values.
func stubPostgreSQLQueryExec(cs processor.ConnectionStatus, result *processor.PostgreSQLQueryResult, err error) *ExecutorMock {
	return &ExecutorMock{
		PostgreSQLQueryFunc: func(_ context.Context, _ datasource.DataSource, _ string, _ processor.TimeRange) (processor.ConnectionStatus, *processor.PostgreSQLQueryResult, error) {
			return cs, result, err
		},
	}
}

func Test_Handler_QueryDataSource(t *testing.T) {
	wasExecutorQueryCalled := func(prometheus, mysql, postgresql int) check {
		return func(t *testing.T, _ *DBMock, exec *ExecutorMock, _ *httptest.ResponseRecorder, _ *bytes.Buffer) {
			assert.Len(t, exec.PrometheusQueryCalls(), prometheus)
			assert.Len(t, exec.MySQLQueryCalls(), mysql)
			assert.Len(t, exec.PostgreSQLQueryCalls(), postgresql)
		}
	}

	cc := map[string]struct {
		DB          *DBMock
		Exec        *ExecutorMock
		Target      string
		OmitSession bool
		ID          string
		Checks      []check
	}{
		"Not authenticated": {
			Target:      _testGenericQueryTarget,
			OmitSession: true,
			ID:          _testID.String(),
			Checks: checks(
				hasResp(http.StatusUnauthorized, `{"code":"account.not_authenticated","message":"not authenticated"}`),
				wasExecutorQueryCalled(0, 0, 0),
			),
		},
		"Missing data source id": {
			Target: _testGenericQueryTarget,
			Checks: checks(
				hasResp(http.StatusNotFound, `{"code":"general","message":"not found"}`),
				wasExecutorQueryCalled(0, 0, 0),
			),
		},
		"Error returned by db.FetchDataSource": {
			DB:     stubDB(nil, assert.AnError),
			Target: _testGenericQueryTarget,
			ID:     _testID.String(),
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasExecutorQueryCalled(0, 0, 0),
			),
		},
		"Missing query parameter": {
			DB:     stubDB(stubDataSource(datasource.TypePrometheus), nil),
			Target: "http://test.com/?chartType=line_chart",
			ID:     _testID.String(),
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"query.required","message":"Query parameter is required."}`),
				wasExecutorQueryCalled(0, 0, 0),
			),
		},
		"Invalid chart type": {
			DB:     stubDB(stubDataSource(datasource.TypePrometheus), nil),
			Target: "http://test.com/?q=up&chartType=bogus",
			ID:     _testID.String(),
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"chart_type.invalid","message":"Invalid chart type. Must be one of: line, bar, gauge."}`),
				wasExecutorQueryCalled(0, 0, 0),
			),
		},
		"Unsupported data source type": {
			DB:     stubDB(stubDataSource(datasource.Type("bogus")), nil),
			Target: _testGenericQueryTarget,
			ID:     _testID.String(),
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"data_source.type_not_supported","message":"Generic query is not supported for this data source type."}`),
				wasExecutorQueryCalled(0, 0, 0),
			),
		},
		"Prometheus dispatch": {
			DB:     stubDB(stubDataSource(datasource.TypePrometheus), nil),
			Exec:   stubPrometheusQueryExec(processor.ConnectionStatusSuccess, nil, nil),
			Target: _testGenericQueryTarget,
			ID:     _testID.String(),
			Checks: checks(
				hasResp(http.StatusOK, `{"status":"no-data"}`),
				wasExecutorQueryCalled(1, 0, 0),
			),
		},
		"PostgreSQL dispatch": {
			DB:     stubDB(stubDataSource(datasource.TypePostgreSQL), nil),
			Exec:   stubPostgreSQLQueryExec(processor.ConnectionStatusSuccess, nil, nil),
			Target: _testGenericQueryTarget,
			ID:     _testID.String(),
			Checks: checks(
				hasResp(http.StatusOK, `{"status":"no-data"}`),
				wasExecutorQueryCalled(0, 0, 1),
			),
		},
		"MariaDB dispatch": {
			DB:     stubDB(stubDataSource(datasource.TypeMariaDB), nil),
			Exec:   stubMySQLQueryExec(processor.ConnectionStatusSuccess, nil, nil),
			Target: _testGenericQueryTarget,
			ID:     _testID.String(),
			Checks: checks(
				hasResp(http.StatusOK, `{"status":"no-data"}`),
				wasExecutorQueryCalled(0, 1, 0),
			),
		},
		"MySQL dispatch": {
			DB:     stubDB(stubDataSource(datasource.TypeMySQL), nil),
			Exec:   stubMySQLQueryExec(processor.ConnectionStatusSuccess, nil, nil),
			Target: _testGenericQueryTarget,
			ID:     _testID.String(),
			Checks: checks(
				hasResp(http.StatusOK, `{"status":"no-data"}`),
				wasExecutorQueryCalled(0, 1, 0),
			),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl, db, exec, logs := prepHandler(c.DB, c.Exec)

			req := prepRequest("GET", c.Target, "", !c.OmitSession, c.ID)
			rec := httptest.NewRecorder()

			hdl.QueryDataSource(rec, req)

			for _, ch := range c.Checks {
				ch(t, db, exec, rec, logs)
			}
		})
	}
}

func Test_Handler_queryPrometheusGeneric(t *testing.T) {
	cc := map[string]struct {
		DB     *DBMock
		Exec   *ExecutorMock
		Target string
		Checks []check
	}{
		"Invalid time range": {
			Target: "http://test.com/?from=bogus",
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"from.invalid","message":"From parameter must be a valid RFC3339 timestamp."}`),
			),
		},
		"Error returned by executor.PrometheusQuery": {
			Exec:   stubPrometheusQueryExec("", nil, assert.AnError),
			Target: _testQueryTarget,
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasDBUpdateDataSourceCalled(0),
			),
		},
		"Changed status updates the data source": {
			Exec:   stubPrometheusQueryExec(processor.ConnectionStatusUnreachable, nil, nil),
			Target: _testQueryTarget,
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"data_source.unreachable","message":"The data source is unreachable."}`),
				wasDBUpdateDataSourceCalled(1),
			),
		},
		"Error returned by db.UpdateDataSource": {
			DB: &DBMock{
				UpdateDataSourceFunc: func(_ context.Context, _ *datasource.DataSource) error {
					return assert.AnError
				},
			},
			Exec:   stubPrometheusQueryExec(processor.ConnectionStatusUnreachable, nil, nil),
			Target: _testQueryTarget,
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"data_source.unreachable","message":"The data source is unreachable."}`),
				wasDBUpdateDataSourceCalled(1),
				hasUpdateFailedLog(),
			),
		},
		"Nil result returns no-data": {
			Exec:   stubPrometheusQueryExec(processor.ConnectionStatusSuccess, nil, nil),
			Target: _testQueryTarget,
			Checks: checks(
				hasResp(http.StatusOK, `{"status":"no-data"}`),
				wasDBUpdateDataSourceCalled(0),
			),
		},
		"Successful query": {
			Exec: stubPrometheusQueryExec(processor.ConnectionStatusSuccess, &processor.PrometheusQueryResult{
				Type: model.ValMatrix,
				Result: model.Matrix{
					&model.SampleStream{
						Metric: model.Metric{"job": "a"},
						Values: []model.SamplePair{
							{Timestamp: model.Time(1700000000000), Value: 10},
						},
					},
				},
			}, nil),
			Target: _testQueryTarget,
			Checks: checks(
				hasResp(http.StatusOK, `{"status":"ok","data":[{"labels":{"job":"a"},"metrics":[[1700000000,10]]}]}`),
				wasDBUpdateDataSourceCalled(0),
			),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl, db, exec, logs := prepHandler(c.DB, c.Exec)

			req := prepRequest("GET", c.Target, "", true, "")
			rec := httptest.NewRecorder()

			hdl.queryPrometheusGeneric(rec, req, stubDataSource(datasource.TypePrometheus), "up", processor.ChartTypeLine)

			for _, ch := range c.Checks {
				ch(t, db, exec, rec, logs)
			}
		})
	}
}

func Test_Handler_queryMySQLGeneric(t *testing.T) {
	cc := map[string]struct {
		DB     *DBMock
		Exec   *ExecutorMock
		Target string
		Checks []check
	}{
		"Invalid time range": {
			Target: "http://test.com/?from=bogus",
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"from.invalid","message":"From parameter must be a valid RFC3339 timestamp."}`),
			),
		},
		"Error returned by executor.MySQLQuery": {
			Exec:   stubMySQLQueryExec("", nil, assert.AnError),
			Target: _testQueryTarget,
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasDBUpdateDataSourceCalled(0),
			),
		},
		"Changed status updates the data source": {
			Exec:   stubMySQLQueryExec(processor.ConnectionStatusUnreachable, nil, nil),
			Target: _testQueryTarget,
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"data_source.unreachable","message":"The data source is unreachable."}`),
				wasDBUpdateDataSourceCalled(1),
			),
		},
		"Error returned by db.UpdateDataSource": {
			DB: &DBMock{
				UpdateDataSourceFunc: func(_ context.Context, _ *datasource.DataSource) error {
					return assert.AnError
				},
			},
			Exec:   stubMySQLQueryExec(processor.ConnectionStatusUnreachable, nil, nil),
			Target: _testQueryTarget,
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"data_source.unreachable","message":"The data source is unreachable."}`),
				wasDBUpdateDataSourceCalled(1),
				hasUpdateFailedLog(),
			),
		},
		"Nil result returns no-data": {
			Exec:   stubMySQLQueryExec(processor.ConnectionStatusSuccess, nil, nil),
			Target: _testQueryTarget,
			Checks: checks(
				hasResp(http.StatusOK, `{"status":"no-data"}`),
				wasDBUpdateDataSourceCalled(0),
			),
		},
		"Successful query": {
			Exec: stubMySQLQueryExec(processor.ConnectionStatusSuccess, &processor.MySQLQueryResult{
				Columns: []string{"time", "value"},
				Rows:    [][]any{{int64(1700000000), float64(10)}},
			}, nil),
			Target: _testQueryTarget,
			Checks: checks(
				hasResp(http.StatusOK, `{"status":"ok","data":[{"labels":{},"metrics":[[1700000000,10]]}]}`),
				wasDBUpdateDataSourceCalled(0),
			),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl, db, exec, logs := prepHandler(c.DB, c.Exec)

			req := prepRequest("GET", c.Target, "", true, "")
			rec := httptest.NewRecorder()

			hdl.queryMySQLGeneric(rec, req, stubDataSource(datasource.TypeMySQL), "SELECT 1", processor.ChartTypeLine)

			for _, ch := range c.Checks {
				ch(t, db, exec, rec, logs)
			}
		})
	}
}

func Test_Handler_queryPostgreSQLGeneric(t *testing.T) {
	cc := map[string]struct {
		DB     *DBMock
		Exec   *ExecutorMock
		Target string
		Checks []check
	}{
		"Invalid time range": {
			Target: "http://test.com/?from=bogus",
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"from.invalid","message":"From parameter must be a valid RFC3339 timestamp."}`),
			),
		},
		"Error returned by executor.PostgreSQLQuery": {
			Exec:   stubPostgreSQLQueryExec("", nil, assert.AnError),
			Target: _testQueryTarget,
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasDBUpdateDataSourceCalled(0),
			),
		},
		"Changed status updates the data source": {
			Exec:   stubPostgreSQLQueryExec(processor.ConnectionStatusUnreachable, nil, nil),
			Target: _testQueryTarget,
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"data_source.unreachable","message":"The data source is unreachable."}`),
				wasDBUpdateDataSourceCalled(1),
			),
		},
		"Error returned by db.UpdateDataSource": {
			DB: &DBMock{
				UpdateDataSourceFunc: func(_ context.Context, _ *datasource.DataSource) error {
					return assert.AnError
				},
			},
			Exec:   stubPostgreSQLQueryExec(processor.ConnectionStatusUnreachable, nil, nil),
			Target: _testQueryTarget,
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"data_source.unreachable","message":"The data source is unreachable."}`),
				wasDBUpdateDataSourceCalled(1),
				hasUpdateFailedLog(),
			),
		},
		"Nil result returns no-data": {
			Exec:   stubPostgreSQLQueryExec(processor.ConnectionStatusSuccess, nil, nil),
			Target: _testQueryTarget,
			Checks: checks(
				hasResp(http.StatusOK, `{"status":"no-data"}`),
				wasDBUpdateDataSourceCalled(0),
			),
		},
		"Successful query": {
			Exec: stubPostgreSQLQueryExec(processor.ConnectionStatusSuccess, &processor.PostgreSQLQueryResult{
				Columns: []string{"time", "value"},
				Rows:    [][]any{{1700000000.0, 10.0}},
			}, nil),
			Target: _testQueryTarget,
			Checks: checks(
				hasResp(http.StatusOK, `{"status":"ok","data":[{"labels":{},"metrics":[[1700000000,10]]}]}`),
				wasDBUpdateDataSourceCalled(0),
			),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl, db, exec, logs := prepHandler(c.DB, c.Exec)

			req := prepRequest("GET", c.Target, "", true, "")
			rec := httptest.NewRecorder()

			hdl.queryPostgreSQLGeneric(rec, req, stubDataSource(datasource.TypePostgreSQL), "SELECT 1", processor.ChartTypeLine)

			for _, ch := range c.Checks {
				ch(t, db, exec, rec, logs)
			}
		})
	}
}
