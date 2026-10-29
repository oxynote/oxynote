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

// _testVariableQueryTarget is the generic query target whose query is a
// variable that stubVariables resolves to up.
const _testVariableQueryTarget = "http://test.com/?q=$metric&chartType=line_chart&from=2024-01-01T00:00:00Z&to=2024-01-02T00:00:00Z"

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
		return func(t *testing.T, _ *DBMock, exec *runnerMock, _ *VariableManagerMock, _ *httptest.ResponseRecorder, _ *bytes.Buffer) {
			assert.Len(t, exec.prometheus.QueryRangeCalls(), prometheus)
			assert.Len(t, exec.mySQL.QueryCalls(), mysql)
			assert.Len(t, exec.postgreSQL.QueryCalls(), postgresql)

			for _, f := range exec.prometheus.QueryRangeCalls() {
				assert.Equal(t, "up", f.Q)
			}

			for _, f := range exec.mySQL.QueryCalls() {
				assert.Equal(t, "up", f.Q)
			}

			for _, f := range exec.postgreSQL.QueryCalls() {
				assert.Equal(t, "up", f.Q)
			}
		}
	}

	wasExecutorTypeCalled := func(count int) check {
		return func(t *testing.T, _ *DBMock, exec *runnerMock, _ *VariableManagerMock, _ *httptest.ResponseRecorder, _ *bytes.Buffer) {
			assert.Len(t, exec.runner.TypeCalls(), count)
		}
	}

	wasExecutorClientCalled := func(prometheus, mysql, postgresql int) check {
		return func(t *testing.T, _ *DBMock, exec *runnerMock, _ *VariableManagerMock, _ *httptest.ResponseRecorder, _ *bytes.Buffer) {
			assert.Len(t, exec.runner.PrometheusCalls(), prometheus)
			assert.Len(t, exec.runner.MySQLCalls(), mysql)
			assert.Len(t, exec.runner.PostgreSQLCalls(), postgresql)
		}
	}

	cc := map[string]struct {
		DB          *DBMock
		Clients     *clientMocks
		Variables   *VariableManagerMock
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
				wasDBFetchDataSourceCalled(0),
				wasVariableManagerFetchTreeCalled(0),
				wasRunnersRunnerCalled(0),
				wasExecutorTypeCalled(0),
				wasExecutorClientCalled(0, 0, 0),
			),
		},
		"Missing data source id": {
			Target: _testGenericQueryTarget,
			Checks: checks(
				hasResp(http.StatusNotFound, `{"code":"general","message":"not found"}`),
				wasExecutorQueryCalled(0, 0, 0),
				wasDBFetchDataSourceCalled(0),
				wasVariableManagerFetchTreeCalled(0),
				wasRunnersRunnerCalled(0),
				wasExecutorTypeCalled(0),
				wasExecutorClientCalled(0, 0, 0),
			),
		},
		"Error returned by db.FetchDataSource": {
			DB:     stubDB(nil, assert.AnError),
			Target: _testGenericQueryTarget,
			ID:     _testID.String(),
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasExecutorQueryCalled(0, 0, 0),
				wasDBFetchDataSourceCalled(1),
				wasVariableManagerFetchTreeCalled(0),
				wasRunnersRunnerCalled(0),
				wasExecutorTypeCalled(0),
				wasExecutorClientCalled(0, 0, 0),
			),
		},
		"Missing query parameter": {
			DB:     stubDB(stubDataSource(datasourceCore.TypePrometheus), nil),
			Target: "http://test.com/?chartType=line_chart",
			ID:     _testID.String(),
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"query.required","message":"Query parameter is required."}`),
				wasExecutorQueryCalled(0, 0, 0),
				wasDBFetchDataSourceCalled(1),
				wasVariableManagerFetchTreeCalled(0),
				wasRunnersRunnerCalled(0),
				wasExecutorTypeCalled(0),
				wasExecutorClientCalled(0, 0, 0),
			),
		},
		"Invalid chart type": {
			DB:     stubDB(stubDataSource(datasourceCore.TypePrometheus), nil),
			Target: "http://test.com/?q=up&chartType=bogus",
			ID:     _testID.String(),
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"chart_type.invalid","message":"Invalid chart type. Must be one of: line, bar, gauge."}`),
				wasExecutorQueryCalled(0, 0, 0),
				wasDBFetchDataSourceCalled(1),
				wasVariableManagerFetchTreeCalled(0),
				wasRunnersRunnerCalled(0),
				wasExecutorTypeCalled(0),
				wasExecutorClientCalled(0, 0, 0),
			),
		},
		"Invalid time range": {
			DB:     stubDB(stubDataSource(datasourceCore.TypePrometheus), nil),
			Target: "http://test.com/?q=up&chartType=line_chart&from=bogus",
			ID:     _testID.String(),
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"from.invalid","message":"From parameter must be a valid RFC3339 timestamp."}`),
				wasExecutorQueryCalled(0, 0, 0),
				wasDBFetchDataSourceCalled(1),
				wasVariableManagerFetchTreeCalled(0),
				wasRunnersRunnerCalled(0),
				wasExecutorTypeCalled(0),
				wasExecutorClientCalled(0, 0, 0),
			),
		},
		"Unsupported data source type": {
			DB:     stubDB(stubDataSource(datasourceCore.Type("bogus")), nil),
			Target: _testGenericQueryTarget,
			ID:     _testID.String(),
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"data_source.type_not_supported","message":"Generic query is not supported for this data source type."}`),
				wasExecutorQueryCalled(0, 0, 0),
				wasDBFetchDataSourceCalled(1),
				wasVariableManagerFetchTreeCalled(0),
				wasRunnersRunnerCalled(1),
				wasExecutorTypeCalled(1),
				wasExecutorClientCalled(0, 0, 0),
			),
		},
		"Error returned by Prometheus.QueryRange": {
			DB:      stubDB(stubDataSource(datasourceCore.TypePrometheus), nil),
			Clients: stubPrometheusQueryClients("", nil, assert.AnError),
			Target:  _testGenericQueryTarget,
			ID:      _testID.String(),
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasExecutorQueryCalled(1, 0, 0),
				wasDBFetchDataSourceCalled(1),
				wasVariableManagerFetchTreeCalled(0),
				wasRunnersRunnerCalled(1),
				wasExecutorTypeCalled(1),
				wasExecutorClientCalled(1, 0, 0),
			),
		},
		"Error returned by Runner.Prometheus": {
			DB:      stubDB(stubDataSource(datasourceCore.TypePrometheus), nil),
			Clients: stubPrometheusQueryClients(processor.ConnectionStatusUnreachable, nil, nil),
			Target:  _testGenericQueryTarget,
			ID:      _testID.String(),
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"data_source.unreachable","message":"The data source is unreachable."}`),
				wasExecutorQueryCalled(0, 0, 0),
				wasDBFetchDataSourceCalled(1),
				wasVariableManagerFetchTreeCalled(0),
				wasRunnersRunnerCalled(1),
				wasExecutorTypeCalled(1),
				wasExecutorClientCalled(1, 0, 0),
			),
		},
		"Successful query": {
			DB: stubDB(stubDataSource(datasourceCore.TypePrometheus), nil),
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
			Target: _testGenericQueryTarget,
			ID:     _testID.String(),
			Checks: checks(
				hasResp(http.StatusOK, `{"status":"ok","data":[{"labels":{"job":"a"},"metrics":[[1700000000,10]]}]}`),
				wasExecutorQueryCalled(1, 0, 0),
				wasVariableManagerFetchTreeCalled(0),
				wasDBFetchDataSourceCalled(1),
				wasRunnersRunnerCalled(1),
				wasExecutorTypeCalled(1),
				wasExecutorClientCalled(1, 0, 0),
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
				wasVariableManagerFetchTreeCalled(0),
				wasDBFetchDataSourceCalled(1),
				wasRunnersRunnerCalled(1),
				wasExecutorTypeCalled(1),
				wasExecutorClientCalled(1, 0, 0),
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
				wasVariableManagerFetchTreeCalled(0),
				wasDBFetchDataSourceCalled(1),
				wasRunnersRunnerCalled(1),
				wasExecutorTypeCalled(1),
				wasExecutorClientCalled(0, 0, 1),
			),
		},
		"Supabase dispatch": {
			DB:      stubDB(stubDataSource(datasourceCore.TypeSupabase), nil),
			Clients: stubPostgreSQLQueryClients(processor.ConnectionStatusSuccess, nil, nil),
			Target:  _testGenericQueryTarget,
			ID:      _testID.String(),
			Checks: checks(
				hasResp(http.StatusOK, `{"status":"no-data"}`),
				wasExecutorQueryCalled(0, 0, 1),
				wasVariableManagerFetchTreeCalled(0),
				wasDBFetchDataSourceCalled(1),
				wasRunnersRunnerCalled(1),
				wasExecutorTypeCalled(1),
				wasExecutorClientCalled(0, 0, 1),
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
				wasVariableManagerFetchTreeCalled(0),
				wasDBFetchDataSourceCalled(1),
				wasRunnersRunnerCalled(1),
				wasExecutorTypeCalled(1),
				wasExecutorClientCalled(0, 1, 0),
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
				wasVariableManagerFetchTreeCalled(0),
				wasDBFetchDataSourceCalled(1),
				wasRunnersRunnerCalled(1),
				wasExecutorTypeCalled(1),
				wasExecutorClientCalled(0, 1, 0),
			),
		},
		"Error returned by VariableManager.FetchTree": {
			DB:        stubDB(stubDataSource(datasourceCore.TypePrometheus), nil),
			Variables: stubVariables(assert.AnError),
			Target:    _testVariableQueryTarget,
			ID:        _testID.String(),
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasExecutorQueryCalled(0, 0, 0),
				wasVariableManagerFetchTreeCalled(1),
				wasDBFetchDataSourceCalled(1),
				wasRunnersRunnerCalled(0),
				wasExecutorTypeCalled(0),
				wasExecutorClientCalled(0, 0, 0),
			),
		},
		"Prometheus substituted variables": {
			DB:        stubDB(stubDataSource(datasourceCore.TypePrometheus), nil),
			Clients:   stubPrometheusQueryClients(processor.ConnectionStatusSuccess, nil, nil),
			Variables: stubVariables(nil),
			Target:    _testVariableQueryTarget,
			ID:        _testID.String(),
			Checks: checks(
				hasResp(http.StatusOK, `{"status":"no-data"}`),
				wasExecutorQueryCalled(1, 0, 0),
				wasVariableManagerFetchTreeCalled(1),
				wasDBFetchDataSourceCalled(1),
				wasRunnersRunnerCalled(1),
				wasExecutorTypeCalled(1),
				wasExecutorClientCalled(1, 0, 0),
			),
		},
		"PostgreSQL substituted variables": {
			DB:        stubDB(stubDataSource(datasourceCore.TypePostgreSQL), nil),
			Clients:   stubPostgreSQLQueryClients(processor.ConnectionStatusSuccess, nil, nil),
			Variables: stubVariables(nil),
			Target:    _testVariableQueryTarget,
			ID:        _testID.String(),
			Checks: checks(
				hasResp(http.StatusOK, `{"status":"no-data"}`),
				wasExecutorQueryCalled(0, 0, 1),
				wasVariableManagerFetchTreeCalled(1),
				wasDBFetchDataSourceCalled(1),
				wasRunnersRunnerCalled(1),
				wasExecutorTypeCalled(1),
				wasExecutorClientCalled(0, 0, 1),
			),
		},
		"MySQL substituted variables": {
			DB:        stubDB(stubDataSource(datasourceCore.TypeMySQL), nil),
			Clients:   stubMySQLQueryClients(processor.ConnectionStatusSuccess, nil, nil),
			Variables: stubVariables(nil),
			Target:    _testVariableQueryTarget,
			ID:        _testID.String(),
			Checks: checks(
				hasResp(http.StatusOK, `{"status":"no-data"}`),
				wasExecutorQueryCalled(0, 1, 0),
				wasVariableManagerFetchTreeCalled(1),
				wasDBFetchDataSourceCalled(1),
				wasRunnersRunnerCalled(1),
				wasExecutorTypeCalled(1),
				wasExecutorClientCalled(0, 1, 0),
			),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl, db, exec, vars, logs := prepHandler(c.DB, c.Clients, c.Variables)

			req := prepRequest("GET", c.Target, "", !c.OmitSession, c.ID)
			rec := httptest.NewRecorder()

			hdl.QueryDataSource(rec, req)

			for _, ch := range c.Checks {
				ch(t, db, exec, vars, rec, logs)
			}
		})
	}
}
