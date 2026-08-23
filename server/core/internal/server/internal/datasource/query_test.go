package datasource

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	datasourceCore "github.com/oxynote/oxynote/server/core/internal/datasource"
	datasourceMock "github.com/oxynote/oxynote/server/core/internal/datasource/_mock"
	"github.com/oxynote/oxynote/server/core/internal/datasource/processor"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"
)

// _testGenericQueryTarget is the request target carrying a query, a chart
// type, and a complete time range.
const _testGenericQueryTarget = "http://test.com/?q=up&chartType=line_chart&from=2024-01-01T00:00:00Z&to=2024-01-02T00:00:00Z"

// stubPrometheusQueryClients returns the clients a runner hands out,
// whose Prometheus query yields the provided values. A status the
// connection refused is how the accessor fails rather than something the
// query returns.
func stubPrometheusQueryClients(cs processor.ConnectionStatus, result *processor.PrometheusQueryResult, err error) *clientMocks {
	return &clientMocks{
		err: cs.Error(),
		prometheus: &datasourceMock.Prometheus{
			QueryRangeFunc: func(_ context.Context, _ string, _ processor.TimeRange) (*processor.PrometheusQueryResult, error) {
				return result, err
			},
		},
	}
}

// stubMySQLQueryClients returns the clients a runner hands out, whose
// MySQL query yields the provided values.
func stubMySQLQueryClients(cs processor.ConnectionStatus, result *processor.MySQLQueryResult, err error) *clientMocks {
	return &clientMocks{
		err: cs.Error(),
		mySQL: &datasourceMock.MySQL{
			QueryFunc: func(_ context.Context, _ string, _ processor.TimeRange) (*processor.MySQLQueryResult, error) {
				return result, err
			},
		},
	}
}

// stubPostgreSQLQueryClients returns the clients a runner hands out,
// whose PostgreSQL query yields the provided values.
func stubPostgreSQLQueryClients(cs processor.ConnectionStatus, result *processor.PostgreSQLQueryResult, err error) *clientMocks {
	return &clientMocks{
		err: cs.Error(),
		postgreSQL: &datasourceMock.PostgreSQL{
			QueryFunc: func(_ context.Context, _ string, _ processor.TimeRange) (*processor.PostgreSQLQueryResult, error) {
				return result, err
			},
		},
	}
}

func Test_Handler_QueryDataSource(t *testing.T) {
	wasExecutorQueryCalled := func(prometheus, mysql, postgresql int) check {
		return func(t *testing.T, _ *DBMock, exec *runnerMock, _ *httptest.ResponseRecorder, _ *bytes.Buffer) {
			assert.Len(t, exec.prometheus.QueryRangeCalls(), prometheus)
			assert.Len(t, exec.mySQL.QueryCalls(), mysql)
			assert.Len(t, exec.postgreSQL.QueryCalls(), postgresql)
		}
	}

	cc := map[string]struct {
		DB          *DBMock
		Clients     *clientMocks
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
			DB:     stubDB(stubDataSource(datasourceCore.TypePrometheus), nil),
			Target: "http://test.com/?chartType=line_chart",
			ID:     _testID.String(),
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"query.required","message":"Query parameter is required."}`),
				wasExecutorQueryCalled(0, 0, 0),
			),
		},
		"Invalid chart type": {
			DB:     stubDB(stubDataSource(datasourceCore.TypePrometheus), nil),
			Target: "http://test.com/?q=up&chartType=bogus",
			ID:     _testID.String(),
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"chart_type.invalid","message":"Invalid chart type. Must be one of: line, bar, gauge."}`),
				wasExecutorQueryCalled(0, 0, 0),
			),
		},
		"Unsupported data source type": {
			DB:     stubDB(stubDataSource(datasourceCore.Type("bogus")), nil),
			Target: _testGenericQueryTarget,
			ID:     _testID.String(),
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"data_source.type_not_supported","message":"Generic query is not supported for this data source type."}`),
				wasExecutorQueryCalled(0, 0, 0),
			),
		},
		"Prometheus dispatch": {
			DB:      stubDB(stubDataSource(datasourceCore.TypePrometheus), nil),
			Clients: stubPrometheusQueryClients(processor.ConnectionStatusSuccess, nil, nil),
			Target:  _testGenericQueryTarget,
			ID:      _testID.String(),
			Checks: checks(
				hasResp(http.StatusOK, `{"status":"no-data"}`),
				wasExecutorQueryCalled(1, 0, 0),
			),
		},
		"PostgreSQL dispatch": {
			DB:      stubDB(stubDataSource(datasourceCore.TypePostgreSQL), nil),
			Clients: stubPostgreSQLQueryClients(processor.ConnectionStatusSuccess, nil, nil),
			Target:  _testGenericQueryTarget,
			ID:      _testID.String(),
			Checks: checks(
				hasResp(http.StatusOK, `{"status":"no-data"}`),
				wasExecutorQueryCalled(0, 0, 1),
			),
		},
		"MariaDB dispatch": {
			DB:      stubDB(stubDataSource(datasourceCore.TypeMariaDB), nil),
			Clients: stubMySQLQueryClients(processor.ConnectionStatusSuccess, nil, nil),
			Target:  _testGenericQueryTarget,
			ID:      _testID.String(),
			Checks: checks(
				hasResp(http.StatusOK, `{"status":"no-data"}`),
				wasExecutorQueryCalled(0, 1, 0),
			),
		},
		"MySQL dispatch": {
			DB:      stubDB(stubDataSource(datasourceCore.TypeMySQL), nil),
			Clients: stubMySQLQueryClients(processor.ConnectionStatusSuccess, nil, nil),
			Target:  _testGenericQueryTarget,
			ID:      _testID.String(),
			Checks: checks(
				hasResp(http.StatusOK, `{"status":"no-data"}`),
				wasExecutorQueryCalled(0, 1, 0),
			),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl, db, exec, logs := prepHandler(c.DB, c.Clients)

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
		DB      *DBMock
		Clients *clientMocks
		Target  string
		Checks  []check
	}{
		"Invalid time range": {
			Target: "http://test.com/?from=bogus",
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"from.invalid","message":"From parameter must be a valid RFC3339 timestamp."}`),
			),
		},
		"Error returned by executor.PrometheusQuery": {
			Clients: stubPrometheusQueryClients("", nil, assert.AnError),
			Target:  _testQueryTarget,
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasDBUpdateDataSourceCalled(0),
			),
		},
		"A connection the data source refused": {
			Clients: stubPrometheusQueryClients(processor.ConnectionStatusUnreachable, nil, nil),
			Target:  _testQueryTarget,
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"data_source.unreachable","message":"The data source is unreachable."}`),
				wasDBUpdateDataSourceCalled(0),
			),
		},
		"Nil result returns no-data": {
			Clients: stubPrometheusQueryClients(processor.ConnectionStatusSuccess, nil, nil),
			Target:  _testQueryTarget,
			Checks: checks(
				hasResp(http.StatusOK, `{"status":"no-data"}`),
				wasDBUpdateDataSourceCalled(0),
			),
		},
		"Successful query": {
			Clients: stubPrometheusQueryClients(processor.ConnectionStatusSuccess, &processor.PrometheusQueryResult{
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

			hdl, db, exec, logs := prepHandler(c.DB, c.Clients)

			req := prepRequest("GET", c.Target, "", true, "")
			rec := httptest.NewRecorder()

			hdl.queryPrometheusGeneric(rec, req, stubDataSource(datasourceCore.TypePrometheus), "up", processor.ChartTypeLine)

			for _, ch := range c.Checks {
				ch(t, db, exec, rec, logs)
			}
		})
	}
}

func Test_Handler_queryMySQLGeneric(t *testing.T) {
	cc := map[string]struct {
		DB      *DBMock
		Clients *clientMocks
		Target  string
		Checks  []check
	}{
		"Invalid time range": {
			Target: "http://test.com/?from=bogus",
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"from.invalid","message":"From parameter must be a valid RFC3339 timestamp."}`),
			),
		},
		"Error returned by executor.MySQLQuery": {
			Clients: stubMySQLQueryClients("", nil, assert.AnError),
			Target:  _testQueryTarget,
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasDBUpdateDataSourceCalled(0),
			),
		},
		"A connection the data source refused": {
			Clients: stubMySQLQueryClients(processor.ConnectionStatusUnreachable, nil, nil),
			Target:  _testQueryTarget,
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"data_source.unreachable","message":"The data source is unreachable."}`),
				wasDBUpdateDataSourceCalled(0),
			),
		},
		"Nil result returns no-data": {
			Clients: stubMySQLQueryClients(processor.ConnectionStatusSuccess, nil, nil),
			Target:  _testQueryTarget,
			Checks: checks(
				hasResp(http.StatusOK, `{"status":"no-data"}`),
				wasDBUpdateDataSourceCalled(0),
			),
		},
		"Successful query": {
			Clients: stubMySQLQueryClients(processor.ConnectionStatusSuccess, &processor.MySQLQueryResult{
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

			hdl, db, exec, logs := prepHandler(c.DB, c.Clients)

			req := prepRequest("GET", c.Target, "", true, "")
			rec := httptest.NewRecorder()

			hdl.queryMySQLGeneric(rec, req, stubDataSource(datasourceCore.TypeMySQL), "SELECT 1", processor.ChartTypeLine)

			for _, ch := range c.Checks {
				ch(t, db, exec, rec, logs)
			}
		})
	}
}

func Test_Handler_queryPostgreSQLGeneric(t *testing.T) {
	cc := map[string]struct {
		DB      *DBMock
		Clients *clientMocks
		Target  string
		Checks  []check
	}{
		"Invalid time range": {
			Target: "http://test.com/?from=bogus",
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"from.invalid","message":"From parameter must be a valid RFC3339 timestamp."}`),
			),
		},
		"Error returned by executor.PostgreSQLQuery": {
			Clients: stubPostgreSQLQueryClients("", nil, assert.AnError),
			Target:  _testQueryTarget,
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasDBUpdateDataSourceCalled(0),
			),
		},
		"A connection the data source refused": {
			Clients: stubPostgreSQLQueryClients(processor.ConnectionStatusUnreachable, nil, nil),
			Target:  _testQueryTarget,
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"data_source.unreachable","message":"The data source is unreachable."}`),
				wasDBUpdateDataSourceCalled(0),
			),
		},
		"Nil result returns no-data": {
			Clients: stubPostgreSQLQueryClients(processor.ConnectionStatusSuccess, nil, nil),
			Target:  _testQueryTarget,
			Checks: checks(
				hasResp(http.StatusOK, `{"status":"no-data"}`),
				wasDBUpdateDataSourceCalled(0),
			),
		},
		"Successful query": {
			Clients: stubPostgreSQLQueryClients(processor.ConnectionStatusSuccess, &processor.PostgreSQLQueryResult{
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

			hdl, db, exec, logs := prepHandler(c.DB, c.Clients)

			req := prepRequest("GET", c.Target, "", true, "")
			rec := httptest.NewRecorder()

			hdl.queryPostgreSQLGeneric(rec, req, stubDataSource(datasourceCore.TypePostgreSQL), "SELECT 1", processor.ChartTypeLine)

			for _, ch := range c.Checks {
				ch(t, db, exec, rec, logs)
			}
		})
	}
}
