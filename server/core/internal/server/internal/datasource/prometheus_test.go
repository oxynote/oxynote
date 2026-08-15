package datasource

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	datasourceCore "github.com/oxynote/oxynote/server/core/internal/datasource"
	"github.com/oxynote/oxynote/server/core/internal/datasource/processor"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// _testQueryTarget is the request target carrying a query and a complete
// time range.
const _testQueryTarget = "http://test.com/?q=up&from=2024-01-01T00:00:00Z&to=2024-01-02T00:00:00Z"

// _testParsedTimeRange is the time range parsed from _testQueryTarget.
var _testParsedTimeRange = processor.TimeRange{
	From: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	To:   time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
}

func Test_Handler_QueryPrometheusDataSource(t *testing.T) {
	wasExecutorPrometheusQueryCalled := func(count int) check {
		return func(t *testing.T, _ *DBMock, exec *ExecutorMock, _ *httptest.ResponseRecorder, _ *bytes.Buffer) {
			ff := exec.PrometheusQueryCalls()
			require.Len(t, ff, count)

			if count == 0 {
				return
			}

			assert.Equal(t, _testID, ff[0].Ds.ID)
			assert.Equal(t, "up", ff[0].Query)
			assert.Equal(t, _testParsedTimeRange, ff[0].Tr)
		}
	}

	// stubExec returns an executor mock whose PrometheusQuery yields the
	// provided values.
	stubExec := func(cs processor.ConnectionStatus, result *processor.PrometheusQueryResult, err error) *ExecutorMock {
		return &ExecutorMock{
			PrometheusQueryFunc: func(_ context.Context, _ datasourceCore.DataSource, _ string, _ processor.TimeRange) (processor.ConnectionStatus, *processor.PrometheusQueryResult, error) {
				return cs, result, err
			},
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
			Target:      _testQueryTarget,
			OmitSession: true,
			ID:          _testID.String(),
			Checks: checks(
				hasResp(http.StatusUnauthorized, `{"code":"account.not_authenticated","message":"not authenticated"}`),
				wasExecutorPrometheusQueryCalled(0),
			),
		},
		"Missing data source id": {
			Target: _testQueryTarget,
			Checks: checks(
				hasResp(http.StatusNotFound, `{"code":"general","message":"not found"}`),
				wasExecutorPrometheusQueryCalled(0),
			),
		},
		"Error returned by db.FetchDataSource": {
			DB:     stubDB(nil, assert.AnError),
			Target: _testQueryTarget,
			ID:     _testID.String(),
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasExecutorPrometheusQueryCalled(0),
			),
		},
		"Missing query parameter": {
			DB:     stubDB(stubDataSource(datasourceCore.TypePrometheus), nil),
			Target: "http://test.com/?from=2024-01-01T00:00:00Z",
			ID:     _testID.String(),
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"query.required","message":"Query parameter is required."}`),
				wasExecutorPrometheusQueryCalled(0),
			),
		},
		"Invalid time range": {
			DB:     stubDB(stubDataSource(datasourceCore.TypePrometheus), nil),
			Target: "http://test.com/?q=up&from=bogus",
			ID:     _testID.String(),
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"from.invalid","message":"From parameter must be a valid RFC3339 timestamp."}`),
				wasExecutorPrometheusQueryCalled(0),
			),
		},
		"Error returned by executor.PrometheusQuery": {
			DB:     stubDB(stubDataSource(datasourceCore.TypePrometheus), nil),
			Exec:   stubExec("", nil, assert.AnError),
			Target: _testQueryTarget,
			ID:     _testID.String(),
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasDBUpdateDataSourceCalled(0),
			),
		},
		"Changed status updates the data source": {
			DB:     stubDB(stubDataSource(datasourceCore.TypePrometheus), nil),
			Exec:   stubExec(processor.ConnectionStatusUnreachable, nil, nil),
			Target: _testQueryTarget,
			ID:     _testID.String(),
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"data_source.unreachable","message":"The data source is unreachable."}`),
				wasDBUpdateDataSourceCalled(1),
			),
		},
		"Error returned by db.UpdateDataSource": {
			DB: func() *DBMock {
				db := stubDB(stubDataSource(datasourceCore.TypePrometheus), nil)
				db.UpdateDataSourceFunc = func(_ context.Context, _ *datasourceCore.DataSource) error {
					return assert.AnError
				}

				return db
			}(),
			Exec:   stubExec(processor.ConnectionStatusUnreachable, nil, nil),
			Target: _testQueryTarget,
			ID:     _testID.String(),
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"data_source.unreachable","message":"The data source is unreachable."}`),
				wasDBUpdateDataSourceCalled(1),
				hasUpdateFailedLog(),
			),
		},
		"Successful query": {
			DB:     stubDB(stubDataSource(datasourceCore.TypePrometheus), nil),
			Exec:   stubExec(processor.ConnectionStatusSuccess, &processor.PrometheusQueryResult{Type: model.ValMatrix}, nil),
			Target: _testQueryTarget,
			ID:     _testID.String(),
			Checks: checks(
				hasResp(http.StatusOK, `{"type":"matrix"}`),
				wasDBUpdateDataSourceCalled(0),
				wasExecutorPrometheusQueryCalled(1),
			),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl, db, exec, logs := prepHandler(c.DB, c.Exec)

			req := prepRequest("GET", c.Target, "", !c.OmitSession, c.ID)
			rec := httptest.NewRecorder()

			hdl.QueryPrometheusDataSource(rec, req)

			for _, ch := range c.Checks {
				ch(t, db, exec, rec, logs)
			}
		})
	}
}

func Test_Handler_FetchPrometheusDataSourceMetadata(t *testing.T) {
	// stubExec returns an executor mock whose PrometheusMetadata yields
	// the provided values.
	stubExec := func(cs processor.ConnectionStatus, result *processor.PrometheusMetadataResult, err error) *ExecutorMock {
		return &ExecutorMock{
			PrometheusMetadataFunc: func(_ context.Context, _ datasourceCore.DataSource) (processor.ConnectionStatus, *processor.PrometheusMetadataResult, error) {
				return cs, result, err
			},
		}
	}

	cc := map[string]struct {
		DB          *DBMock
		Exec        *ExecutorMock
		OmitSession bool
		ID          string
		Checks      []check
	}{
		"Not authenticated": {
			OmitSession: true,
			ID:          _testID.String(),
			Checks: checks(
				hasResp(http.StatusUnauthorized, `{"code":"account.not_authenticated","message":"not authenticated"}`),
			),
		},
		"Missing data source id": {
			Checks: checks(
				hasResp(http.StatusNotFound, `{"code":"general","message":"not found"}`),
			),
		},
		"Error returned by db.FetchDataSource": {
			DB: stubDB(nil, assert.AnError),
			ID: _testID.String(),
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
			),
		},
		"Error returned by executor.PrometheusMetadata": {
			DB:   stubDB(stubDataSource(datasourceCore.TypePrometheus), nil),
			Exec: stubExec("", nil, assert.AnError),
			ID:   _testID.String(),
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasDBUpdateDataSourceCalled(0),
			),
		},
		"Changed status updates the data source": {
			DB:   stubDB(stubDataSource(datasourceCore.TypePrometheus), nil),
			Exec: stubExec(processor.ConnectionStatusUnreachable, nil, nil),
			ID:   _testID.String(),
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"data_source.unreachable","message":"The data source is unreachable."}`),
				wasDBUpdateDataSourceCalled(1),
			),
		},
		"Error returned by db.UpdateDataSource": {
			DB: func() *DBMock {
				db := stubDB(stubDataSource(datasourceCore.TypePrometheus), nil)
				db.UpdateDataSourceFunc = func(_ context.Context, _ *datasourceCore.DataSource) error {
					return assert.AnError
				}

				return db
			}(),
			Exec: stubExec(processor.ConnectionStatusUnreachable, nil, nil),
			ID:   _testID.String(),
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"data_source.unreachable","message":"The data source is unreachable."}`),
				wasDBUpdateDataSourceCalled(1),
				hasUpdateFailedLog(),
			),
		},
		"Successful metadata retrieval": {
			DB:   stubDB(stubDataSource(datasourceCore.TypePrometheus), nil),
			Exec: stubExec(processor.ConnectionStatusSuccess, &processor.PrometheusMetadataResult{Result: map[string]string{"metric": "info"}}, nil),
			ID:   _testID.String(),
			Checks: checks(
				hasResp(http.StatusOK, `{"result":{"metric":"info"}}`),
				wasDBUpdateDataSourceCalled(0),
			),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl, db, exec, logs := prepHandler(c.DB, c.Exec)

			req := prepRequest("GET", "http://test.com/", "", !c.OmitSession, c.ID)
			rec := httptest.NewRecorder()

			hdl.FetchPrometheusDataSourceMetadata(rec, req)

			for _, ch := range c.Checks {
				ch(t, db, exec, rec, logs)
			}
		})
	}
}

func Test_Handler_FetchPrometheusDataSourceLabelNames(t *testing.T) {
	wasExecutorPrometheusLabelNamesCalled := func(count int) check {
		return func(t *testing.T, _ *DBMock, exec *ExecutorMock, _ *httptest.ResponseRecorder, _ *bytes.Buffer) {
			ff := exec.PrometheusLabelNamesCalls()
			require.Len(t, ff, count)

			if count == 0 {
				return
			}

			assert.Equal(t, _testID, ff[0].Ds.ID)
			assert.Equal(t, []string{"up"}, ff[0].Matchers)
		}
	}

	// stubExec returns an executor mock whose PrometheusLabelNames yields
	// the provided values.
	stubExec := func(cs processor.ConnectionStatus, result *processor.PrometheusLabelNamesResult, err error) *ExecutorMock {
		return &ExecutorMock{
			PrometheusLabelNamesFunc: func(_ context.Context, _ datasourceCore.DataSource, _ []string, _ processor.TimeRange) (processor.ConnectionStatus, *processor.PrometheusLabelNamesResult, error) {
				return cs, result, err
			},
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
			Target:      "http://test.com/?matchers=up",
			OmitSession: true,
			ID:          _testID.String(),
			Checks: checks(
				hasResp(http.StatusUnauthorized, `{"code":"account.not_authenticated","message":"not authenticated"}`),
				wasExecutorPrometheusLabelNamesCalled(0),
			),
		},
		"Missing data source id": {
			Target: "http://test.com/?matchers=up",
			Checks: checks(
				hasResp(http.StatusNotFound, `{"code":"general","message":"not found"}`),
				wasExecutorPrometheusLabelNamesCalled(0),
			),
		},
		"Error returned by db.FetchDataSource": {
			DB:     stubDB(nil, assert.AnError),
			Target: "http://test.com/?matchers=up",
			ID:     _testID.String(),
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasExecutorPrometheusLabelNamesCalled(0),
			),
		},
		"Invalid time range": {
			DB:     stubDB(stubDataSource(datasourceCore.TypePrometheus), nil),
			Target: "http://test.com/?matchers=up&from=bogus",
			ID:     _testID.String(),
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"from.invalid","message":"From parameter must be a valid RFC3339 timestamp."}`),
				wasExecutorPrometheusLabelNamesCalled(0),
			),
		},
		"Error returned by executor.PrometheusLabelNames": {
			DB:     stubDB(stubDataSource(datasourceCore.TypePrometheus), nil),
			Exec:   stubExec("", nil, assert.AnError),
			Target: "http://test.com/?matchers=up",
			ID:     _testID.String(),
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasDBUpdateDataSourceCalled(0),
			),
		},
		"Changed status updates the data source": {
			DB:     stubDB(stubDataSource(datasourceCore.TypePrometheus), nil),
			Exec:   stubExec(processor.ConnectionStatusUnreachable, nil, nil),
			Target: "http://test.com/?matchers=up",
			ID:     _testID.String(),
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"data_source.unreachable","message":"The data source is unreachable."}`),
				wasDBUpdateDataSourceCalled(1),
			),
		},
		"Error returned by db.UpdateDataSource": {
			DB: func() *DBMock {
				db := stubDB(stubDataSource(datasourceCore.TypePrometheus), nil)
				db.UpdateDataSourceFunc = func(_ context.Context, _ *datasourceCore.DataSource) error {
					return assert.AnError
				}

				return db
			}(),
			Exec:   stubExec(processor.ConnectionStatusUnreachable, nil, nil),
			Target: "http://test.com/?matchers=up",
			ID:     _testID.String(),
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"data_source.unreachable","message":"The data source is unreachable."}`),
				wasDBUpdateDataSourceCalled(1),
				hasUpdateFailedLog(),
			),
		},
		"Successful label names retrieval": {
			DB:     stubDB(stubDataSource(datasourceCore.TypePrometheus), nil),
			Exec:   stubExec(processor.ConnectionStatusSuccess, &processor.PrometheusLabelNamesResult{Result: []string{"__name__"}}, nil),
			Target: "http://test.com/?matchers=up",
			ID:     _testID.String(),
			Checks: checks(
				hasResp(http.StatusOK, `{"result":["__name__"]}`),
				wasDBUpdateDataSourceCalled(0),
				wasExecutorPrometheusLabelNamesCalled(1),
			),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl, db, exec, logs := prepHandler(c.DB, c.Exec)

			req := prepRequest("GET", c.Target, "", !c.OmitSession, c.ID)
			rec := httptest.NewRecorder()

			hdl.FetchPrometheusDataSourceLabelNames(rec, req)

			for _, ch := range c.Checks {
				ch(t, db, exec, rec, logs)
			}
		})
	}
}

func Test_Handler_FetchPrometheusDataSourceLabelValues(t *testing.T) {
	wasExecutorPrometheusLabelValuesCalled := func(count int) check {
		return func(t *testing.T, _ *DBMock, exec *ExecutorMock, _ *httptest.ResponseRecorder, _ *bytes.Buffer) {
			ff := exec.PrometheusLabelValuesCalls()
			require.Len(t, ff, count)

			if count == 0 {
				return
			}

			assert.Equal(t, _testID, ff[0].Ds.ID)
			assert.Equal(t, "instance", ff[0].Label)
			assert.Equal(t, []string{"up"}, ff[0].Matchers)
		}
	}

	// stubExec returns an executor mock whose PrometheusLabelValues
	// yields the provided values.
	stubExec := func(cs processor.ConnectionStatus, result *processor.PrometheusLabelValuesResult, err error) *ExecutorMock {
		return &ExecutorMock{
			PrometheusLabelValuesFunc: func(_ context.Context, _ datasourceCore.DataSource, _ string, _ []string, _ processor.TimeRange) (processor.ConnectionStatus, *processor.PrometheusLabelValuesResult, error) {
				return cs, result, err
			},
		}
	}

	cc := map[string]struct {
		DB          *DBMock
		Exec        *ExecutorMock
		Target      string
		OmitSession bool
		ID          string
		OmitLabel   bool
		Checks      []check
	}{
		"Not authenticated": {
			Target:      "http://test.com/?matchers=up",
			OmitSession: true,
			ID:          _testID.String(),
			Checks: checks(
				hasResp(http.StatusUnauthorized, `{"code":"account.not_authenticated","message":"not authenticated"}`),
				wasExecutorPrometheusLabelValuesCalled(0),
			),
		},
		"Missing data source id": {
			Target: "http://test.com/?matchers=up",
			Checks: checks(
				hasResp(http.StatusNotFound, `{"code":"general","message":"not found"}`),
				wasExecutorPrometheusLabelValuesCalled(0),
			),
		},
		"Missing label parameter": {
			Target:    "http://test.com/?matchers=up",
			ID:        _testID.String(),
			OmitLabel: true,
			Checks: checks(
				hasResp(http.StatusNotFound, `{"code":"general","message":"not found"}`),
				wasExecutorPrometheusLabelValuesCalled(0),
			),
		},
		"Error returned by db.FetchDataSource": {
			DB:     stubDB(nil, assert.AnError),
			Target: "http://test.com/?matchers=up",
			ID:     _testID.String(),
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasExecutorPrometheusLabelValuesCalled(0),
			),
		},
		"Invalid time range": {
			DB:     stubDB(stubDataSource(datasourceCore.TypePrometheus), nil),
			Target: "http://test.com/?matchers=up&from=bogus",
			ID:     _testID.String(),
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"from.invalid","message":"From parameter must be a valid RFC3339 timestamp."}`),
				wasExecutorPrometheusLabelValuesCalled(0),
			),
		},
		"Error returned by executor.PrometheusLabelValues": {
			DB:     stubDB(stubDataSource(datasourceCore.TypePrometheus), nil),
			Exec:   stubExec("", nil, assert.AnError),
			Target: "http://test.com/?matchers=up",
			ID:     _testID.String(),
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasDBUpdateDataSourceCalled(0),
			),
		},
		"Changed status updates the data source": {
			DB:     stubDB(stubDataSource(datasourceCore.TypePrometheus), nil),
			Exec:   stubExec(processor.ConnectionStatusUnreachable, nil, nil),
			Target: "http://test.com/?matchers=up",
			ID:     _testID.String(),
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"data_source.unreachable","message":"The data source is unreachable."}`),
				wasDBUpdateDataSourceCalled(1),
			),
		},
		"Error returned by db.UpdateDataSource": {
			DB: func() *DBMock {
				db := stubDB(stubDataSource(datasourceCore.TypePrometheus), nil)
				db.UpdateDataSourceFunc = func(_ context.Context, _ *datasourceCore.DataSource) error {
					return assert.AnError
				}

				return db
			}(),
			Exec:   stubExec(processor.ConnectionStatusUnreachable, nil, nil),
			Target: "http://test.com/?matchers=up",
			ID:     _testID.String(),
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"data_source.unreachable","message":"The data source is unreachable."}`),
				wasDBUpdateDataSourceCalled(1),
				hasUpdateFailedLog(),
			),
		},
		"Successful label values retrieval": {
			DB:     stubDB(stubDataSource(datasourceCore.TypePrometheus), nil),
			Exec:   stubExec(processor.ConnectionStatusSuccess, &processor.PrometheusLabelValuesResult{Result: []string{"a", "b"}}, nil),
			Target: "http://test.com/?matchers=up",
			ID:     _testID.String(),
			Checks: checks(
				hasResp(http.StatusOK, `{"result":["a","b"]}`),
				wasDBUpdateDataSourceCalled(0),
				wasExecutorPrometheusLabelValuesCalled(1),
			),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl, db, exec, logs := prepHandler(c.DB, c.Exec)

			req := prepRequest("GET", c.Target, "", !c.OmitSession, c.ID)

			if !c.OmitLabel {
				req = req.WithContext(testutil.AddChiCtx(req.Context(), "label", "instance"))
			}

			rec := httptest.NewRecorder()

			hdl.FetchPrometheusDataSourceLabelValues(rec, req)

			for _, ch := range c.Checks {
				ch(t, db, exec, rec, logs)
			}
		})
	}
}

func Test_Handler_FetchPrometheusDataSourceSeries(t *testing.T) {
	wasExecutorPrometheusSeriesCalled := func(count int) check {
		return func(t *testing.T, _ *DBMock, exec *ExecutorMock, _ *httptest.ResponseRecorder, _ *bytes.Buffer) {
			ff := exec.PrometheusSeriesCalls()
			require.Len(t, ff, count)

			if count == 0 {
				return
			}

			assert.Equal(t, _testID, ff[0].Ds.ID)
			assert.Equal(t, []string{"up"}, ff[0].Matchers)
		}
	}

	// stubExec returns an executor mock whose PrometheusSeries yields the
	// provided values.
	stubExec := func(cs processor.ConnectionStatus, result *processor.PrometheusSeriesResult, err error) *ExecutorMock {
		return &ExecutorMock{
			PrometheusSeriesFunc: func(_ context.Context, _ datasourceCore.DataSource, _ []string, _ processor.TimeRange) (processor.ConnectionStatus, *processor.PrometheusSeriesResult, error) {
				return cs, result, err
			},
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
			Target:      "http://test.com/?matchers=up",
			OmitSession: true,
			ID:          _testID.String(),
			Checks: checks(
				hasResp(http.StatusUnauthorized, `{"code":"account.not_authenticated","message":"not authenticated"}`),
				wasExecutorPrometheusSeriesCalled(0),
			),
		},
		"Missing data source id": {
			Target: "http://test.com/?matchers=up",
			Checks: checks(
				hasResp(http.StatusNotFound, `{"code":"general","message":"not found"}`),
				wasExecutorPrometheusSeriesCalled(0),
			),
		},
		"Error returned by db.FetchDataSource": {
			DB:     stubDB(nil, assert.AnError),
			Target: "http://test.com/?matchers=up",
			ID:     _testID.String(),
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasExecutorPrometheusSeriesCalled(0),
			),
		},
		"Invalid time range": {
			DB:     stubDB(stubDataSource(datasourceCore.TypePrometheus), nil),
			Target: "http://test.com/?matchers=up&from=bogus",
			ID:     _testID.String(),
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"from.invalid","message":"From parameter must be a valid RFC3339 timestamp."}`),
				wasExecutorPrometheusSeriesCalled(0),
			),
		},
		"Error returned by executor.PrometheusSeries": {
			DB:     stubDB(stubDataSource(datasourceCore.TypePrometheus), nil),
			Exec:   stubExec("", nil, assert.AnError),
			Target: "http://test.com/?matchers=up",
			ID:     _testID.String(),
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasDBUpdateDataSourceCalled(0),
			),
		},
		"Changed status updates the data source": {
			DB:     stubDB(stubDataSource(datasourceCore.TypePrometheus), nil),
			Exec:   stubExec(processor.ConnectionStatusUnreachable, nil, nil),
			Target: "http://test.com/?matchers=up",
			ID:     _testID.String(),
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"data_source.unreachable","message":"The data source is unreachable."}`),
				wasDBUpdateDataSourceCalled(1),
			),
		},
		"Error returned by db.UpdateDataSource": {
			DB: func() *DBMock {
				db := stubDB(stubDataSource(datasourceCore.TypePrometheus), nil)
				db.UpdateDataSourceFunc = func(_ context.Context, _ *datasourceCore.DataSource) error {
					return assert.AnError
				}

				return db
			}(),
			Exec:   stubExec(processor.ConnectionStatusUnreachable, nil, nil),
			Target: "http://test.com/?matchers=up",
			ID:     _testID.String(),
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"data_source.unreachable","message":"The data source is unreachable."}`),
				wasDBUpdateDataSourceCalled(1),
				hasUpdateFailedLog(),
			),
		},
		"Successful series retrieval": {
			DB:     stubDB(stubDataSource(datasourceCore.TypePrometheus), nil),
			Exec:   stubExec(processor.ConnectionStatusSuccess, &processor.PrometheusSeriesResult{Result: []model.LabelSet{{"job": "prometheus"}}}, nil),
			Target: "http://test.com/?matchers=up",
			ID:     _testID.String(),
			Checks: checks(
				hasResp(http.StatusOK, `{"result":[{"job":"prometheus"}]}`),
				wasDBUpdateDataSourceCalled(0),
				wasExecutorPrometheusSeriesCalled(1),
			),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl, db, exec, logs := prepHandler(c.DB, c.Exec)

			req := prepRequest("GET", c.Target, "", !c.OmitSession, c.ID)
			rec := httptest.NewRecorder()

			hdl.FetchPrometheusDataSourceSeries(rec, req)

			for _, ch := range c.Checks {
				ch(t, db, exec, rec, logs)
			}
		})
	}
}
