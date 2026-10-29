package datasource

import (
	"bytes"
	"context"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/guregu/null/v5"
	datasourceCore "github.com/oxynote/oxynote/server/core/internal/datasource"
	datasourceMock "github.com/oxynote/oxynote/server/core/internal/datasource/_mock"
	"github.com/oxynote/oxynote/server/core/internal/datasource/processor"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// _testLogsQuery is the log query the logs targets carry; stubVariables
// resolves its variable to up.
const _testLogsQuery = `{app="$metric"}`

// _testLogsWindow is the window the logs targets carry.
const _testLogsWindow = "from=2024-01-01T00:00:00Z&to=2024-01-02T00:00:00Z"

var (
	// _testLogsTarget is a logs request carrying a query, a window, a
	// limit and a cursor.
	_testLogsTarget = "http://test.com/?" + url.Values{
		"q":      {_testLogsQuery},
		"from":   {"2024-01-01T00:00:00Z"},
		"to":     {"2024-01-02T00:00:00Z"},
		"limit":  {"2"},
		"cursor": {"ns:1704110400000000000"},
	}.Encode()

	// _testLogsCursor is the cursor _testLogsTarget carries.
	_testLogsCursor = func() processor.Cursor {
		var c processor.Cursor
		if err := c.UnmarshalText([]byte("ns:1704110400000000000")); err != nil {
			panic(err)
		}

		return c
	}()
)

// stubLogClients returns the clients a runner hands out, whose log client
// answers every read with the given values.
func stubLogClients(cs processor.ConnectionStatus, err error) *clientMocks {
	page := &processor.LogsResult{
		Status: processor.QueryStatusOK,
		Lines: []processor.LogLine{{
			ID:     "0d6c1b9e44f2a871",
			Time:   time.Date(2024, 1, 1, 12, 58, 14, 670388731, time.UTC),
			Level:  processor.LogLevelError,
			Line:   "panic: runtime error\n\tmain.go:17",
			Labels: map[string]string{"app": "up"},
			Fields: map[string]string{"trace_id": "e1a3f33c25c5eecd"},
		}},
		Cursor: null.ValueFrom(_testLogsCursor),
	}

	return &clientMocks{
		err: cs.Error(),
		logSource: &datasourceMock.Logs{
			LogsFunc: func(context.Context, string, processor.TimeRange, processor.LogsPage) (*processor.LogsResult, error) {
				return page, err
			},
			LogsVolumeFunc: func(context.Context, string, processor.TimeRange) (*processor.QueryResult, error) {
				return &processor.QueryResult{
					Status: processor.QueryStatusOK,
					Data: []processor.QueryResultSeries{{
						Labels:  map[string]string{"level": "error"},
						Metrics: [][2]any{{int64(1704067200), float64(84)}},
					}},
				}, err
			},
			LogFieldsFunc: func(context.Context, string, processor.TimeRange) (*processor.LogFieldsResult, error) {
				return &processor.LogFieldsResult{Fields: []processor.LogField{
					{Name: "app", Kind: processor.LogFieldKindLabel},
					{Name: "trace_id", Kind: processor.LogFieldKindField},
				}}, err
			},
			LogFieldValuesFunc: func(context.Context, string, string, processor.TimeRange) (*processor.LogFieldValuesResult, error) {
				return &processor.LogFieldValuesResult{Values: []processor.LogFieldValue{
					{Value: "up", Count: null.IntFrom(2038)},
					{Value: "down", Count: null.Int{}},
				}}, err
			},
		},
	}
}

// commonLogsCases are the cases every logs handler answers the same way
// before it reaches the log client. field is the field URL parameter the
// handler needs, if any.
func commonLogsCases(field string) map[string]logsCase {
	return map[string]logsCase{
		"Not authenticated": {
			OmitSession: true,
			Target:      _testLogsTarget,
			Field:       field,
			Checks: checks(
				hasResp(http.StatusUnauthorized, `{"code":"account.not_authenticated","message":"not authenticated"}`),
				wasDBFetchDataSourceCalled(0),
				wasRunnersRunnerCalled(0),
				wasExecutorLogsCalled(0),
			),
		},
		"Missing data source id": {
			Target: _testLogsTarget,
			Field:  field,
			OmitID: true,
			Checks: checks(
				hasResp(http.StatusNotFound, `{"code":"general","message":"not found"}`),
				wasDBFetchDataSourceCalled(0),
				wasRunnersRunnerCalled(0),
				wasExecutorLogsCalled(0),
			),
		},
		"Error returned by db.FetchDataSource": {
			DB:     stubDB(nil, assert.AnError),
			Target: _testLogsTarget,
			Field:  field,
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasDBFetchDataSourceCalled(1),
				wasRunnersRunnerCalled(0),
				wasExecutorLogsCalled(0),
			),
		},
		"Invalid window": {
			DB:     stubDB(stubDataSource(datasourceCore.TypeLoki), nil),
			Target: "http://test.com/?q=x&from=yesterday",
			Field:  field,
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"from.invalid","message":"From parameter must be a valid RFC3339 timestamp."}`),
				wasRunnersRunnerCalled(0),
				wasExecutorLogsCalled(0),
			),
		},
		"Error returned by VariableManager.FetchTree": {
			DB:        stubDB(stubDataSource(datasourceCore.TypeLoki), nil),
			Variables: stubVariables(assert.AnError),
			Target:    _testLogsTarget,
			Field:     field,
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasVariableManagerFetchTreeCalled(1),
				wasRunnersRunnerCalled(0),
				wasExecutorLogsCalled(0),
			),
		},
		"Error returned by Runner.Logs": {
			DB:        stubDB(stubDataSource(datasourceCore.TypePrometheus), nil),
			Clients:   stubLogClients(processor.ConnectionStatusUnreachable, nil),
			Variables: stubVariables(nil),
			Target:    _testLogsTarget,
			Field:     field,
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"data_source.unreachable","message":"The data source is unreachable."}`),
				wasRunnersRunnerCalled(1),
				wasExecutorLogsCalled(1),
			),
		},
	}
}

// logsCase is one case of a logs handler test.
type logsCase struct {
	DB          *DBMock
	Clients     *clientMocks
	Variables   *VariableManagerMock
	OmitSession bool
	OmitID      bool
	Target      string
	Field       string
	Checks      []check
}

// runLogsCase runs one case of a logs handler test against handle.
func runLogsCase(t *testing.T, c logsCase, handle func(*Handler, http.ResponseWriter, *http.Request)) {
	t.Helper()

	hdl, db, exec, vars, logs := prepHandler(c.DB, c.Clients, c.Variables)

	id := _testID.String()
	if c.OmitID {
		id = ""
	}

	req := prepRequest(http.MethodGet, c.Target, "", !c.OmitSession, id)
	if c.Field != "" {
		req = req.WithContext(testutil.AddChiCtx(req.Context(), "field", c.Field))
	}

	rec := httptest.NewRecorder()

	handle(hdl, rec, req)

	for _, ch := range c.Checks {
		ch(t, db, exec, vars, rec, logs)
	}
}

func Test_Handler_FetchDataSourceLogs(t *testing.T) {
	wasLogSourceLogsCalled := func(count int) check {
		return func(t *testing.T, _ *DBMock, exec *runnerMock, _ *VariableManagerMock, _ *httptest.ResponseRecorder, _ *bytes.Buffer) {
			ff := exec.logSource.LogsCalls()
			require.Len(t, ff, count)

			if count == 0 {
				return
			}

			assert.Equal(t, `{app="up"}`, ff[0].Q)
			assert.Equal(t, _testParsedTimeRange, ff[0].Tr)
			assert.Equal(t, processor.LogsPage{Limit: 2, Cursor: null.ValueFrom(_testLogsCursor)}, ff[0].Page)
		}
	}

	cc := map[string]logsCase{
		"Missing query": {
			DB:     stubDB(stubDataSource(datasourceCore.TypeLoki), nil),
			Target: "http://test.com/?" + _testLogsWindow,
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"query.required","message":"Query parameter is required."}`),
				wasRunnersRunnerCalled(0),
			),
		},
		"Invalid limit": {
			DB:     stubDB(stubDataSource(datasourceCore.TypeLoki), nil),
			Target: "http://test.com/?q=x&limit=5000&" + _testLogsWindow,
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"limit.invalid","message":"limit must be a whole number from 1 to 1000"}`),
				wasRunnersRunnerCalled(0),
			),
		},
		"Invalid cursor": {
			DB:     stubDB(stubDataSource(datasourceCore.TypeLoki), nil),
			Target: "http://test.com/?q=x&cursor=later&" + _testLogsWindow,
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"cursor.invalid","message":"cursor must come from a previous logs response"}`),
				wasRunnersRunnerCalled(0),
			),
		},
		"Error returned by Logs.Logs": {
			DB:        stubDB(stubDataSource(datasourceCore.TypeLoki), nil),
			Clients:   stubLogClients(processor.ConnectionStatusSuccess, processor.NewInvalidQueryError("parse error at line 1")),
			Variables: stubVariables(nil),
			Target:    _testLogsTarget,
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"query.error","message":"parse error at line 1"}`),
				wasLogSourceLogsCalled(1),
			),
		},
		"Successful page": {
			DB:        stubDB(stubDataSource(datasourceCore.TypeLoki), nil),
			Clients:   stubLogClients(processor.ConnectionStatusSuccess, nil),
			Variables: stubVariables(nil),
			Target:    _testLogsTarget,
			Checks: checks(
				hasResp(http.StatusOK, `{
					"status": "ok",
					"lines": [{
						"id": "0d6c1b9e44f2a871",
						"time": "2024-01-01T12:58:14.670388731Z",
						"level": "error",
						"line": "panic: runtime error\n\tmain.go:17",
						"labels": {"app": "up"},
						"fields": {"trace_id": "e1a3f33c25c5eecd"}
					}],
					"cursor": "ns:1704110400000000000",
					"partial": false
				}`),
				wasDBFetchDataSourceCalled(1),
				wasVariableManagerFetchTreeCalled(1),
				wasRunnersRunnerCalled(1),
				wasExecutorLogsCalled(1),
				wasLogSourceLogsCalled(1),
			),
		},
	}

	maps.Copy(cc, commonLogsCases(""))

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			runLogsCase(t, c, (*Handler).FetchDataSourceLogs)
		})
	}
}

func Test_Handler_FetchDataSourceLogVolume(t *testing.T) {
	wasLogSourceLogsVolumeCalled := func(count int) check {
		return func(t *testing.T, _ *DBMock, exec *runnerMock, _ *VariableManagerMock, _ *httptest.ResponseRecorder, _ *bytes.Buffer) {
			ff := exec.logSource.LogsVolumeCalls()
			require.Len(t, ff, count)

			if count == 0 {
				return
			}

			assert.Equal(t, `{app="up"}`, ff[0].Q)
			assert.Equal(t, _testParsedTimeRange, ff[0].Tr)
		}
	}

	cc := map[string]logsCase{
		"Missing query": {
			DB:     stubDB(stubDataSource(datasourceCore.TypeLoki), nil),
			Target: "http://test.com/?" + _testLogsWindow,
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"query.required","message":"Query parameter is required."}`),
				wasRunnersRunnerCalled(0),
			),
		},
		"Error returned by Logs.LogsVolume": {
			DB:        stubDB(stubDataSource(datasourceCore.TypeLoki), nil),
			Clients:   stubLogClients(processor.ConnectionStatusSuccess, processor.NewInvalidQueryError("parse error at line 1")),
			Variables: stubVariables(nil),
			Target:    _testLogsTarget,
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"query.error","message":"parse error at line 1"}`),
				wasLogSourceLogsVolumeCalled(1),
			),
		},
		"Successful volume": {
			DB:        stubDB(stubDataSource(datasourceCore.TypeLoki), nil),
			Clients:   stubLogClients(processor.ConnectionStatusSuccess, nil),
			Variables: stubVariables(nil),
			Target:    _testLogsTarget,
			Checks: checks(
				hasResp(http.StatusOK, `{"status":"ok","data":[{"labels":{"level":"error"},"metrics":[[1704067200,84]]}]}`),
				wasDBFetchDataSourceCalled(1),
				wasRunnersRunnerCalled(1),
				wasExecutorLogsCalled(1),
				wasLogSourceLogsVolumeCalled(1),
			),
		},
	}

	maps.Copy(cc, commonLogsCases(""))

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			runLogsCase(t, c, (*Handler).FetchDataSourceLogVolume)
		})
	}
}

func Test_Handler_FetchDataSourceLogFields(t *testing.T) {
	wasLogSourceLogFieldsCalled := func(count int, q string) check {
		return func(t *testing.T, _ *DBMock, exec *runnerMock, _ *VariableManagerMock, _ *httptest.ResponseRecorder, _ *bytes.Buffer) {
			ff := exec.logSource.LogFieldsCalls()
			require.Len(t, ff, count)

			if count == 0 {
				return
			}

			assert.Equal(t, q, ff[0].Q)
			assert.Equal(t, _testParsedTimeRange, ff[0].Tr)
		}
	}

	cc := map[string]logsCase{
		"Error returned by Logs.LogFields": {
			DB:        stubDB(stubDataSource(datasourceCore.TypeLoki), nil),
			Clients:   stubLogClients(processor.ConnectionStatusSuccess, assert.AnError),
			Variables: stubVariables(nil),
			Target:    _testLogsTarget,
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasLogSourceLogFieldsCalled(1, `{app="up"}`),
			),
		},
		"Successful fields without a query": {
			DB:      stubDB(stubDataSource(datasourceCore.TypeLoki), nil),
			Clients: stubLogClients(processor.ConnectionStatusSuccess, nil),
			Target:  "http://test.com/?" + _testLogsWindow,
			Checks: checks(
				hasResp(http.StatusOK, `{"fields":[{"name":"app","kind":"label"},{"name":"trace_id","kind":"field"}]}`),
				wasVariableManagerFetchTreeCalled(0),
				wasExecutorLogsCalled(1),
				wasLogSourceLogFieldsCalled(1, ""),
			),
		},
		"Successful fields narrowed by a query": {
			DB:        stubDB(stubDataSource(datasourceCore.TypeLoki), nil),
			Clients:   stubLogClients(processor.ConnectionStatusSuccess, nil),
			Variables: stubVariables(nil),
			Target:    _testLogsTarget,
			Checks: checks(
				hasResp(http.StatusOK, `{"fields":[{"name":"app","kind":"label"},{"name":"trace_id","kind":"field"}]}`),
				wasVariableManagerFetchTreeCalled(1),
				wasLogSourceLogFieldsCalled(1, `{app="up"}`),
			),
		},
	}

	maps.Copy(cc, commonLogsCases(""))

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			runLogsCase(t, c, (*Handler).FetchDataSourceLogFields)
		})
	}
}

func Test_Handler_FetchDataSourceLogFieldValues(t *testing.T) {
	wasLogSourceLogFieldValuesCalled := func(count int) check {
		return func(t *testing.T, _ *DBMock, exec *runnerMock, _ *VariableManagerMock, _ *httptest.ResponseRecorder, _ *bytes.Buffer) {
			ff := exec.logSource.LogFieldValuesCalls()
			require.Len(t, ff, count)

			if count == 0 {
				return
			}

			assert.Equal(t, "app", ff[0].Field)
			assert.Equal(t, `{app="up"}`, ff[0].Q)
			assert.Equal(t, _testParsedTimeRange, ff[0].Tr)
		}
	}

	cc := map[string]logsCase{
		"Missing field": {
			DB:     stubDB(stubDataSource(datasourceCore.TypeLoki), nil),
			Target: _testLogsTarget,
			Checks: checks(
				hasResp(http.StatusNotFound, `{"code":"general","message":"not found"}`),
				wasDBFetchDataSourceCalled(0),
				wasRunnersRunnerCalled(0),
			),
		},
		"Error returned by Logs.LogFieldValues": {
			DB:        stubDB(stubDataSource(datasourceCore.TypeLoki), nil),
			Clients:   stubLogClients(processor.ConnectionStatusSuccess, assert.AnError),
			Variables: stubVariables(nil),
			Target:    _testLogsTarget,
			Field:     "app",
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasLogSourceLogFieldValuesCalled(1),
			),
		},
		"Successful values": {
			DB:        stubDB(stubDataSource(datasourceCore.TypeLoki), nil),
			Clients:   stubLogClients(processor.ConnectionStatusSuccess, nil),
			Variables: stubVariables(nil),
			Target:    _testLogsTarget,
			Field:     "app",
			Checks: checks(
				hasResp(http.StatusOK, `{"values":[{"value":"up","count":2038},{"value":"down","count":null}]}`),
				wasDBFetchDataSourceCalled(1),
				wasRunnersRunnerCalled(1),
				wasExecutorLogsCalled(1),
				wasLogSourceLogFieldValuesCalled(1),
			),
		},
	}

	maps.Copy(cc, commonLogsCases("app"))

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			runLogsCase(t, c, (*Handler).FetchDataSourceLogFieldValues)
		})
	}
}
