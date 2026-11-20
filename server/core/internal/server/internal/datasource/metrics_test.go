package datasource

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	datasourceCore "github.com/oxynote/oxynote/server/core/internal/datasource"
	datasourceMock "github.com/oxynote/oxynote/server/core/internal/datasource/_mock"
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

func Test_Handler_FetchDataSourceMetricMetadata(t *testing.T) {
	wasExecutorMetricsMetadataCalled := func(count int) check {
		return func(t *testing.T, _ *DBMock, exec *runnerMock, _ *VariableManagerMock, _ *httptest.ResponseRecorder, _ *bytes.Buffer) {
			ff := exec.metrics.MetadataCalls()
			require.Len(t, ff, count)

			if count == 0 {
				return
			}

			assert.Equal(t, _testID, exec.dataSources[0].ID)
		}
	}

	// stubClients returns clients whose accessors fail with the error of cs
	// and whose Metadata returns the given values.
	stubClients := func(cs processor.ConnectionStatus, result *processor.PrometheusMetadataResult, err error) *clientMocks {
		return &clientMocks{
			err: cs.Error(),
			metrics: &datasourceMock.Metrics{
				MetadataFunc: func(_ context.Context) (*processor.PrometheusMetadataResult, error) {
					return result, err
				},
			},
		}
	}

	cc := map[string]struct {
		DB          *DBMock
		Clients     *clientMocks
		OmitSession bool
		ID          string
		Checks      []check
	}{
		"Not authenticated": {
			OmitSession: true,
			ID:          _testID.String(),
			Checks: checks(
				hasResp(http.StatusUnauthorized, `{"code":"account.not_authenticated","message":"not authenticated"}`),
				wasDBFetchDataSourceCalled(0),
				wasRunnersRunnerCalled(0),
				wasExecutorMetricsCalled(0),
				wasExecutorMetricsMetadataCalled(0),
			),
		},
		"Missing data source id": {
			Checks: checks(
				hasResp(http.StatusNotFound, `{"code":"general","message":"not found"}`),
				wasDBFetchDataSourceCalled(0),
				wasRunnersRunnerCalled(0),
				wasExecutorMetricsCalled(0),
				wasExecutorMetricsMetadataCalled(0),
			),
		},
		"Error returned by db.FetchDataSource": {
			DB: stubDB(nil, assert.AnError),
			ID: _testID.String(),
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasDBFetchDataSourceCalled(1),
				wasRunnersRunnerCalled(0),
				wasExecutorMetricsCalled(0),
				wasExecutorMetricsMetadataCalled(0),
			),
		},
		"Error returned by Metrics.Metadata": {
			DB:      stubDB(stubDataSource(datasourceCore.TypePrometheus), nil),
			Clients: stubClients("", nil, assert.AnError),
			ID:      _testID.String(),
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasDBUpdateDataSourceCalled(0),
				wasDBFetchDataSourceCalled(1),
				wasRunnersRunnerCalled(1),
				wasExecutorMetricsCalled(1),
				wasExecutorMetricsMetadataCalled(1),
			),
		},
		"Error returned by Runner.Metrics": {
			DB:      stubDB(stubDataSource(datasourceCore.TypePrometheus), nil),
			Clients: stubClients(processor.ConnectionStatusUnreachable, nil, nil),
			ID:      _testID.String(),
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"data_source.unreachable","message":"The data source is unreachable."}`),
				wasDBUpdateDataSourceCalled(0),
				wasDBFetchDataSourceCalled(1),
				wasRunnersRunnerCalled(1),
				wasExecutorMetricsCalled(1),
				wasExecutorMetricsMetadataCalled(0),
			),
		},
		"Successful metadata retrieval": {
			DB:      stubDB(stubDataSource(datasourceCore.TypePrometheus), nil),
			Clients: stubClients(processor.ConnectionStatusSuccess, &processor.PrometheusMetadataResult{Result: map[string]string{"metric": "info"}}, nil),
			ID:      _testID.String(),
			Checks: checks(
				hasResp(http.StatusOK, `{"result":{"metric":"info"}}`),
				wasDBUpdateDataSourceCalled(0),
				wasDBFetchDataSourceCalled(1),
				wasRunnersRunnerCalled(1),
				wasExecutorMetricsCalled(1),
				wasExecutorMetricsMetadataCalled(1),
			),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl, db, exec, vars, logs := prepHandler(c.DB, c.Clients, nil)

			req := prepRequest("GET", "http://test.com/", "", !c.OmitSession, c.ID)
			rec := httptest.NewRecorder()

			hdl.FetchDataSourceMetricMetadata(rec, req)

			for _, ch := range c.Checks {
				ch(t, db, exec, vars, rec, logs)
			}
		})
	}
}

func Test_Handler_FetchDataSourceMetricLabelNames(t *testing.T) {
	wasExecutorMetricsLabelNamesCalled := func(count int) check {
		return func(t *testing.T, _ *DBMock, exec *runnerMock, _ *VariableManagerMock, _ *httptest.ResponseRecorder, _ *bytes.Buffer) {
			ff := exec.metrics.LabelNamesCalls()
			require.Len(t, ff, count)

			if count == 0 {
				return
			}

			assert.Equal(t, _testID, exec.dataSources[0].ID)
			assert.Equal(t, []string{"up"}, ff[0].Matchers)
		}
	}

	// stubClients returns clients whose accessors fail with the error of cs
	// and whose Prometheus LabelNames returns the given values.
	stubClients := func(cs processor.ConnectionStatus, result *processor.PrometheusLabelNamesResult, err error) *clientMocks {
		return &clientMocks{
			err: cs.Error(),
			metrics: &datasourceMock.Metrics{
				LabelNamesFunc: func(_ context.Context, _ []string, _ processor.TimeRange) (*processor.PrometheusLabelNamesResult, error) {
					return result, err
				},
			},
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
			Target:      "http://test.com/?matchers=up",
			OmitSession: true,
			ID:          _testID.String(),
			Checks: checks(
				hasResp(http.StatusUnauthorized, `{"code":"account.not_authenticated","message":"not authenticated"}`),
				wasDBFetchDataSourceCalled(0),
				wasVariableManagerFetchTreeCalled(0),
				wasRunnersRunnerCalled(0),
				wasExecutorMetricsCalled(0),
				wasExecutorMetricsLabelNamesCalled(0),
			),
		},
		"Missing data source id": {
			Target: "http://test.com/?matchers=up",
			Checks: checks(
				hasResp(http.StatusNotFound, `{"code":"general","message":"not found"}`),
				wasDBFetchDataSourceCalled(0),
				wasVariableManagerFetchTreeCalled(0),
				wasRunnersRunnerCalled(0),
				wasExecutorMetricsCalled(0),
				wasExecutorMetricsLabelNamesCalled(0),
			),
		},
		"Error returned by db.FetchDataSource": {
			DB:     stubDB(nil, assert.AnError),
			Target: "http://test.com/?matchers=up",
			ID:     _testID.String(),
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasDBFetchDataSourceCalled(1),
				wasVariableManagerFetchTreeCalled(0),
				wasRunnersRunnerCalled(0),
				wasExecutorMetricsCalled(0),
				wasExecutorMetricsLabelNamesCalled(0),
			),
		},
		"Invalid time range": {
			DB:     stubDB(stubDataSource(datasourceCore.TypePrometheus), nil),
			Target: "http://test.com/?matchers=up&from=bogus",
			ID:     _testID.String(),
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"from.invalid","message":"from parameter must be a valid RFC3339 timestamp"}`),
				wasDBFetchDataSourceCalled(1),
				wasVariableManagerFetchTreeCalled(0),
				wasRunnersRunnerCalled(0),
				wasExecutorMetricsCalled(0),
				wasExecutorMetricsLabelNamesCalled(0),
			),
		},
		"Error returned by Metrics.LabelNames": {
			DB:      stubDB(stubDataSource(datasourceCore.TypePrometheus), nil),
			Clients: stubClients("", nil, assert.AnError),
			Target:  "http://test.com/?matchers=up",
			ID:      _testID.String(),
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasDBUpdateDataSourceCalled(0),
				wasDBFetchDataSourceCalled(1),
				wasVariableManagerFetchTreeCalled(0),
				wasRunnersRunnerCalled(1),
				wasExecutorMetricsCalled(1),
				wasExecutorMetricsLabelNamesCalled(1),
			),
		},
		"Error returned by Runner.Metrics": {
			DB:      stubDB(stubDataSource(datasourceCore.TypePrometheus), nil),
			Clients: stubClients(processor.ConnectionStatusUnreachable, nil, nil),
			Target:  "http://test.com/?matchers=up",
			ID:      _testID.String(),
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"data_source.unreachable","message":"The data source is unreachable."}`),
				wasDBUpdateDataSourceCalled(0),
				wasDBFetchDataSourceCalled(1),
				wasVariableManagerFetchTreeCalled(0),
				wasRunnersRunnerCalled(1),
				wasExecutorMetricsCalled(1),
				wasExecutorMetricsLabelNamesCalled(0),
			),
		},
		"Successful label names retrieval": {
			DB:      stubDB(stubDataSource(datasourceCore.TypePrometheus), nil),
			Clients: stubClients(processor.ConnectionStatusSuccess, &processor.PrometheusLabelNamesResult{Result: []string{"__name__"}}, nil),
			Target:  "http://test.com/?matchers=up",
			ID:      _testID.String(),
			Checks: checks(
				hasResp(http.StatusOK, `{"result":["__name__"]}`),
				wasDBUpdateDataSourceCalled(0),
				wasExecutorMetricsLabelNamesCalled(1),
				wasVariableManagerFetchTreeCalled(0),
				wasDBFetchDataSourceCalled(1),
				wasRunnersRunnerCalled(1),
				wasExecutorMetricsCalled(1),
			),
		},
		"Error returned by VariableManager.FetchTree": {
			DB:        stubDB(stubDataSource(datasourceCore.TypePrometheus), nil),
			Variables: stubVariables(assert.AnError),
			Target:    "http://test.com/?matchers=$metric",
			ID:        _testID.String(),
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasExecutorMetricsLabelNamesCalled(0),
				wasVariableManagerFetchTreeCalled(1),
				wasDBFetchDataSourceCalled(1),
				wasRunnersRunnerCalled(0),
				wasExecutorMetricsCalled(0),
			),
		},
		"Substituted variables": {
			DB:        stubDB(stubDataSource(datasourceCore.TypePrometheus), nil),
			Clients:   stubClients(processor.ConnectionStatusSuccess, &processor.PrometheusLabelNamesResult{Result: []string{"__name__"}}, nil),
			Variables: stubVariables(nil),
			Target:    "http://test.com/?matchers=$metric",
			ID:        _testID.String(),
			Checks: checks(
				hasResp(http.StatusOK, `{"result":["__name__"]}`),
				wasDBUpdateDataSourceCalled(0),
				wasExecutorMetricsLabelNamesCalled(1),
				wasVariableManagerFetchTreeCalled(1),
				wasDBFetchDataSourceCalled(1),
				wasRunnersRunnerCalled(1),
				wasExecutorMetricsCalled(1),
			),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl, db, exec, vars, logs := prepHandler(c.DB, c.Clients, c.Variables)

			req := prepRequest("GET", c.Target, "", !c.OmitSession, c.ID)
			rec := httptest.NewRecorder()

			hdl.FetchDataSourceMetricLabelNames(rec, req)

			for _, ch := range c.Checks {
				ch(t, db, exec, vars, rec, logs)
			}
		})
	}
}

func Test_Handler_FetchDataSourceMetricLabelValues(t *testing.T) {
	wasExecutorMetricsLabelValuesCalled := func(count int) check {
		return func(t *testing.T, _ *DBMock, exec *runnerMock, _ *VariableManagerMock, _ *httptest.ResponseRecorder, _ *bytes.Buffer) {
			ff := exec.metrics.LabelValuesCalls()
			require.Len(t, ff, count)

			if count == 0 {
				return
			}

			assert.Equal(t, _testID, exec.dataSources[0].ID)
			assert.Equal(t, "instance", ff[0].Label)
			assert.Equal(t, []string{"up"}, ff[0].Matchers)
		}
	}

	// stubClients returns clients whose accessors fail with the error of cs
	// and whose Prometheus LabelValues returns the given values.
	stubClients := func(cs processor.ConnectionStatus, result *processor.PrometheusLabelValuesResult, err error) *clientMocks {
		return &clientMocks{
			err: cs.Error(),
			metrics: &datasourceMock.Metrics{
				LabelValuesFunc: func(_ context.Context, _ string, _ []string, _ processor.TimeRange) (*processor.PrometheusLabelValuesResult, error) {
					return result, err
				},
			},
		}
	}

	cc := map[string]struct {
		DB          *DBMock
		Clients     *clientMocks
		Variables   *VariableManagerMock
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
				wasDBFetchDataSourceCalled(0),
				wasVariableManagerFetchTreeCalled(0),
				wasRunnersRunnerCalled(0),
				wasExecutorMetricsCalled(0),
				wasExecutorMetricsLabelValuesCalled(0),
			),
		},
		"Missing data source id": {
			Target: "http://test.com/?matchers=up",
			Checks: checks(
				hasResp(http.StatusNotFound, `{"code":"general","message":"not found"}`),
				wasDBFetchDataSourceCalled(0),
				wasVariableManagerFetchTreeCalled(0),
				wasRunnersRunnerCalled(0),
				wasExecutorMetricsCalled(0),
				wasExecutorMetricsLabelValuesCalled(0),
			),
		},
		"Missing label parameter": {
			Target:    "http://test.com/?matchers=up",
			ID:        _testID.String(),
			OmitLabel: true,
			Checks: checks(
				hasResp(http.StatusNotFound, `{"code":"general","message":"not found"}`),
				wasDBFetchDataSourceCalled(0),
				wasVariableManagerFetchTreeCalled(0),
				wasRunnersRunnerCalled(0),
				wasExecutorMetricsCalled(0),
				wasExecutorMetricsLabelValuesCalled(0),
			),
		},
		"Error returned by db.FetchDataSource": {
			DB:     stubDB(nil, assert.AnError),
			Target: "http://test.com/?matchers=up",
			ID:     _testID.String(),
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasDBFetchDataSourceCalled(1),
				wasVariableManagerFetchTreeCalled(0),
				wasRunnersRunnerCalled(0),
				wasExecutorMetricsCalled(0),
				wasExecutorMetricsLabelValuesCalled(0),
			),
		},
		"Invalid time range": {
			DB:     stubDB(stubDataSource(datasourceCore.TypePrometheus), nil),
			Target: "http://test.com/?matchers=up&from=bogus",
			ID:     _testID.String(),
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"from.invalid","message":"from parameter must be a valid RFC3339 timestamp"}`),
				wasDBFetchDataSourceCalled(1),
				wasVariableManagerFetchTreeCalled(0),
				wasRunnersRunnerCalled(0),
				wasExecutorMetricsCalled(0),
				wasExecutorMetricsLabelValuesCalled(0),
			),
		},
		"Error returned by Metrics.LabelValues": {
			DB:      stubDB(stubDataSource(datasourceCore.TypePrometheus), nil),
			Clients: stubClients("", nil, assert.AnError),
			Target:  "http://test.com/?matchers=up",
			ID:      _testID.String(),
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasDBUpdateDataSourceCalled(0),
				wasDBFetchDataSourceCalled(1),
				wasVariableManagerFetchTreeCalled(0),
				wasRunnersRunnerCalled(1),
				wasExecutorMetricsCalled(1),
				wasExecutorMetricsLabelValuesCalled(1),
			),
		},
		"Error returned by Runner.Metrics": {
			DB:      stubDB(stubDataSource(datasourceCore.TypePrometheus), nil),
			Clients: stubClients(processor.ConnectionStatusUnreachable, nil, nil),
			Target:  "http://test.com/?matchers=up",
			ID:      _testID.String(),
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"data_source.unreachable","message":"The data source is unreachable."}`),
				wasDBUpdateDataSourceCalled(0),
				wasDBFetchDataSourceCalled(1),
				wasVariableManagerFetchTreeCalled(0),
				wasRunnersRunnerCalled(1),
				wasExecutorMetricsCalled(1),
				wasExecutorMetricsLabelValuesCalled(0),
			),
		},
		"Successful label values retrieval": {
			DB:      stubDB(stubDataSource(datasourceCore.TypePrometheus), nil),
			Clients: stubClients(processor.ConnectionStatusSuccess, &processor.PrometheusLabelValuesResult{Result: []string{"a", "b"}}, nil),
			Target:  "http://test.com/?matchers=up",
			ID:      _testID.String(),
			Checks: checks(
				hasResp(http.StatusOK, `{"result":["a","b"]}`),
				wasDBUpdateDataSourceCalled(0),
				wasExecutorMetricsLabelValuesCalled(1),
				wasVariableManagerFetchTreeCalled(0),
				wasDBFetchDataSourceCalled(1),
				wasRunnersRunnerCalled(1),
				wasExecutorMetricsCalled(1),
			),
		},
		"Error returned by VariableManager.FetchTree": {
			DB:        stubDB(stubDataSource(datasourceCore.TypePrometheus), nil),
			Variables: stubVariables(assert.AnError),
			Target:    "http://test.com/?matchers=$metric",
			ID:        _testID.String(),
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasExecutorMetricsLabelValuesCalled(0),
				wasVariableManagerFetchTreeCalled(1),
				wasDBFetchDataSourceCalled(1),
				wasRunnersRunnerCalled(0),
				wasExecutorMetricsCalled(0),
			),
		},
		"Substituted variables": {
			DB:        stubDB(stubDataSource(datasourceCore.TypePrometheus), nil),
			Clients:   stubClients(processor.ConnectionStatusSuccess, &processor.PrometheusLabelValuesResult{Result: []string{"a", "b"}}, nil),
			Variables: stubVariables(nil),
			Target:    "http://test.com/?matchers=$metric",
			ID:        _testID.String(),
			Checks: checks(
				hasResp(http.StatusOK, `{"result":["a","b"]}`),
				wasDBUpdateDataSourceCalled(0),
				wasExecutorMetricsLabelValuesCalled(1),
				wasVariableManagerFetchTreeCalled(1),
				wasDBFetchDataSourceCalled(1),
				wasRunnersRunnerCalled(1),
				wasExecutorMetricsCalled(1),
			),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl, db, exec, vars, logs := prepHandler(c.DB, c.Clients, c.Variables)

			req := prepRequest("GET", c.Target, "", !c.OmitSession, c.ID)

			if !c.OmitLabel {
				req = req.WithContext(testutil.AddChiCtx(req.Context(), "label", "instance"))
			}

			rec := httptest.NewRecorder()

			hdl.FetchDataSourceMetricLabelValues(rec, req)

			for _, ch := range c.Checks {
				ch(t, db, exec, vars, rec, logs)
			}
		})
	}
}

func Test_Handler_FetchDataSourceMetricSeries(t *testing.T) {
	wasExecutorMetricsSeriesCalled := func(count int) check {
		return func(t *testing.T, _ *DBMock, exec *runnerMock, _ *VariableManagerMock, _ *httptest.ResponseRecorder, _ *bytes.Buffer) {
			ff := exec.metrics.SeriesCalls()
			require.Len(t, ff, count)

			if count == 0 {
				return
			}

			assert.Equal(t, _testID, exec.dataSources[0].ID)
			assert.Equal(t, []string{"up"}, ff[0].Matchers)
		}
	}

	// stubClients returns clients whose accessors fail with the error of cs
	// and whose Prometheus Series returns the given values.
	stubClients := func(cs processor.ConnectionStatus, result *processor.PrometheusSeriesResult, err error) *clientMocks {
		return &clientMocks{
			err: cs.Error(),
			metrics: &datasourceMock.Metrics{
				SeriesFunc: func(_ context.Context, _ []string, _ processor.TimeRange) (*processor.PrometheusSeriesResult, error) {
					return result, err
				},
			},
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
			Target:      "http://test.com/?matchers=up",
			OmitSession: true,
			ID:          _testID.String(),
			Checks: checks(
				hasResp(http.StatusUnauthorized, `{"code":"account.not_authenticated","message":"not authenticated"}`),
				wasDBFetchDataSourceCalled(0),
				wasVariableManagerFetchTreeCalled(0),
				wasRunnersRunnerCalled(0),
				wasExecutorMetricsCalled(0),
				wasExecutorMetricsSeriesCalled(0),
			),
		},
		"Missing data source id": {
			Target: "http://test.com/?matchers=up",
			Checks: checks(
				hasResp(http.StatusNotFound, `{"code":"general","message":"not found"}`),
				wasDBFetchDataSourceCalled(0),
				wasVariableManagerFetchTreeCalled(0),
				wasRunnersRunnerCalled(0),
				wasExecutorMetricsCalled(0),
				wasExecutorMetricsSeriesCalled(0),
			),
		},
		"Error returned by db.FetchDataSource": {
			DB:     stubDB(nil, assert.AnError),
			Target: "http://test.com/?matchers=up",
			ID:     _testID.String(),
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasDBFetchDataSourceCalled(1),
				wasVariableManagerFetchTreeCalled(0),
				wasRunnersRunnerCalled(0),
				wasExecutorMetricsCalled(0),
				wasExecutorMetricsSeriesCalled(0),
			),
		},
		"Invalid time range": {
			DB:     stubDB(stubDataSource(datasourceCore.TypePrometheus), nil),
			Target: "http://test.com/?matchers=up&from=bogus",
			ID:     _testID.String(),
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"from.invalid","message":"from parameter must be a valid RFC3339 timestamp"}`),
				wasDBFetchDataSourceCalled(1),
				wasVariableManagerFetchTreeCalled(0),
				wasRunnersRunnerCalled(0),
				wasExecutorMetricsCalled(0),
				wasExecutorMetricsSeriesCalled(0),
			),
		},
		"Error returned by Metrics.Series": {
			DB:      stubDB(stubDataSource(datasourceCore.TypePrometheus), nil),
			Clients: stubClients("", nil, assert.AnError),
			Target:  "http://test.com/?matchers=up",
			ID:      _testID.String(),
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasDBUpdateDataSourceCalled(0),
				wasDBFetchDataSourceCalled(1),
				wasVariableManagerFetchTreeCalled(0),
				wasRunnersRunnerCalled(1),
				wasExecutorMetricsCalled(1),
				wasExecutorMetricsSeriesCalled(1),
			),
		},
		"Error returned by Runner.Metrics": {
			DB:      stubDB(stubDataSource(datasourceCore.TypePrometheus), nil),
			Clients: stubClients(processor.ConnectionStatusUnreachable, nil, nil),
			Target:  "http://test.com/?matchers=up",
			ID:      _testID.String(),
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"data_source.unreachable","message":"The data source is unreachable."}`),
				wasDBUpdateDataSourceCalled(0),
				wasDBFetchDataSourceCalled(1),
				wasVariableManagerFetchTreeCalled(0),
				wasRunnersRunnerCalled(1),
				wasExecutorMetricsCalled(1),
				wasExecutorMetricsSeriesCalled(0),
			),
		},
		"Successful series retrieval": {
			DB:      stubDB(stubDataSource(datasourceCore.TypePrometheus), nil),
			Clients: stubClients(processor.ConnectionStatusSuccess, &processor.PrometheusSeriesResult{Result: []model.LabelSet{{"job": "prometheus"}}}, nil),
			Target:  "http://test.com/?matchers=up",
			ID:      _testID.String(),
			Checks: checks(
				hasResp(http.StatusOK, `{"result":[{"job":"prometheus"}]}`),
				wasDBUpdateDataSourceCalled(0),
				wasExecutorMetricsSeriesCalled(1),
				wasVariableManagerFetchTreeCalled(0),
				wasDBFetchDataSourceCalled(1),
				wasRunnersRunnerCalled(1),
				wasExecutorMetricsCalled(1),
			),
		},
		"Error returned by VariableManager.FetchTree": {
			DB:        stubDB(stubDataSource(datasourceCore.TypePrometheus), nil),
			Variables: stubVariables(assert.AnError),
			Target:    "http://test.com/?matchers=$metric",
			ID:        _testID.String(),
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasExecutorMetricsSeriesCalled(0),
				wasVariableManagerFetchTreeCalled(1),
				wasDBFetchDataSourceCalled(1),
				wasRunnersRunnerCalled(0),
				wasExecutorMetricsCalled(0),
			),
		},
		"Substituted variables": {
			DB:        stubDB(stubDataSource(datasourceCore.TypePrometheus), nil),
			Clients:   stubClients(processor.ConnectionStatusSuccess, &processor.PrometheusSeriesResult{Result: []model.LabelSet{{"job": "prometheus"}}}, nil),
			Variables: stubVariables(nil),
			Target:    "http://test.com/?matchers=$metric",
			ID:        _testID.String(),
			Checks: checks(
				hasResp(http.StatusOK, `{"result":[{"job":"prometheus"}]}`),
				wasDBUpdateDataSourceCalled(0),
				wasExecutorMetricsSeriesCalled(1),
				wasVariableManagerFetchTreeCalled(1),
				wasDBFetchDataSourceCalled(1),
				wasRunnersRunnerCalled(1),
				wasExecutorMetricsCalled(1),
			),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl, db, exec, vars, logs := prepHandler(c.DB, c.Clients, c.Variables)

			req := prepRequest("GET", c.Target, "", !c.OmitSession, c.ID)
			rec := httptest.NewRecorder()

			hdl.FetchDataSourceMetricSeries(rec, req)

			for _, ch := range c.Checks {
				ch(t, db, exec, vars, rec, logs)
			}
		})
	}
}
