package datasourcehandler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/datasource"
	"github.com/oxynote/oxynote/server/core/internal/datasource/processor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_Handler_FetchSQLQueryLabels(t *testing.T) {
	wasExecutorSQLQueryLabelsCalled := func(count int) check {
		return func(t *testing.T, _ *DBMock, exec *ExecutorMock, _ *httptest.ResponseRecorder, _ *bytes.Buffer) {
			ff := exec.SQLQueryLabelsCalls()
			require.Len(t, ff, count)

			if count == 0 {
				return
			}

			assert.Equal(t, _testID, ff[0].Ds.ID)
			assert.Equal(t, "up", ff[0].Query)
			assert.Equal(t, _testParsedTimeRange, ff[0].Tr)
		}
	}

	// stubExec returns an executor mock whose SQLQueryLabels yields the
	// provided values.
	stubExec := func(cs processor.ConnectionStatus, labels map[string]string, err error) *ExecutorMock {
		return &ExecutorMock{
			SQLQueryLabelsFunc: func(_ context.Context, _ datasource.DataSource, _ string, _ processor.TimeRange) (processor.ConnectionStatus, map[string]string, error) {
				return cs, labels, err
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
				wasExecutorSQLQueryLabelsCalled(0),
			),
		},
		"Missing data source id": {
			Target: _testQueryTarget,
			Checks: checks(
				hasResp(http.StatusNotFound, `{"code":"general","message":"not found"}`),
				wasExecutorSQLQueryLabelsCalled(0),
			),
		},
		"Error returned by db.FetchDataSource": {
			DB:     stubDB(nil, assert.AnError),
			Target: _testQueryTarget,
			ID:     _testID.String(),
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasExecutorSQLQueryLabelsCalled(0),
			),
		},
		"Missing query parameter": {
			DB:     stubDB(stubDataSource(datasource.TypePostgreSQL), nil),
			Target: "http://test.com/?from=2024-01-01T00:00:00Z",
			ID:     _testID.String(),
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"query.required","message":"Query parameter is required."}`),
				wasExecutorSQLQueryLabelsCalled(0),
			),
		},
		"Invalid time range": {
			DB:     stubDB(stubDataSource(datasource.TypePostgreSQL), nil),
			Target: "http://test.com/?q=up&from=bogus",
			ID:     _testID.String(),
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"from.invalid","message":"From parameter must be a valid RFC3339 timestamp."}`),
				wasExecutorSQLQueryLabelsCalled(0),
			),
		},
		"Error returned by executor.SQLQueryLabels": {
			DB:     stubDB(stubDataSource(datasource.TypePostgreSQL), nil),
			Exec:   stubExec("", nil, assert.AnError),
			Target: _testQueryTarget,
			ID:     _testID.String(),
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasDBUpdateDataSourceCalled(0),
			),
		},
		"Changed status updates the data source": {
			DB:     stubDB(stubDataSource(datasource.TypePostgreSQL), nil),
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
				db := stubDB(stubDataSource(datasource.TypePostgreSQL), nil)
				db.UpdateDataSourceFunc = func(_ context.Context, _ *datasource.DataSource) error {
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
		"Nil labels return an empty map": {
			DB:     stubDB(stubDataSource(datasource.TypePostgreSQL), nil),
			Exec:   stubExec(processor.ConnectionStatusSuccess, nil, nil),
			Target: _testQueryTarget,
			ID:     _testID.String(),
			Checks: checks(
				hasResp(http.StatusOK, `{"labels":{}}`),
				wasDBUpdateDataSourceCalled(0),
			),
		},
		"Successful retrieval": {
			DB:     stubDB(stubDataSource(datasource.TypePostgreSQL), nil),
			Exec:   stubExec(processor.ConnectionStatusSuccess, map[string]string{"host": "a"}, nil),
			Target: _testQueryTarget,
			ID:     _testID.String(),
			Checks: checks(
				hasResp(http.StatusOK, `{"labels":{"host":"a"}}`),
				wasDBUpdateDataSourceCalled(0),
				wasExecutorSQLQueryLabelsCalled(1),
			),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl, db, exec, logs := prepHandler(c.DB, c.Exec)

			req := prepRequest("GET", c.Target, "", !c.OmitSession, c.ID)
			rec := httptest.NewRecorder()

			hdl.FetchSQLQueryLabels(rec, req)

			for _, ch := range c.Checks {
				ch(t, db, exec, rec, logs)
			}
		})
	}
}

func Test_Handler_FetchSQLMetadata(t *testing.T) {
	wasExecutorSQLMetadataCalled := func(count int) check {
		return func(t *testing.T, _ *DBMock, exec *ExecutorMock, _ *httptest.ResponseRecorder, _ *bytes.Buffer) {
			ff := exec.SQLMetadataCalls()
			require.Len(t, ff, count)

			if count == 0 {
				return
			}

			assert.Equal(t, _testID, ff[0].Ds.ID)
		}
	}

	// stubExec returns an executor mock whose SQLMetadata yields the
	// provided values.
	stubExec := func(cs processor.ConnectionStatus, result *processor.SQLMetadataResult, err error) *ExecutorMock {
		return &ExecutorMock{
			SQLMetadataFunc: func(_ context.Context, _ datasource.DataSource) (processor.ConnectionStatus, *processor.SQLMetadataResult, error) {
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
				wasExecutorSQLMetadataCalled(0),
			),
		},
		"Missing data source id": {
			Checks: checks(
				hasResp(http.StatusNotFound, `{"code":"general","message":"not found"}`),
				wasExecutorSQLMetadataCalled(0),
			),
		},
		"Error returned by db.FetchDataSource": {
			DB: stubDB(nil, assert.AnError),
			ID: _testID.String(),
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasExecutorSQLMetadataCalled(0),
			),
		},
		"Error returned by executor.SQLMetadata": {
			DB:   stubDB(stubDataSource(datasource.TypePostgreSQL), nil),
			Exec: stubExec("", nil, assert.AnError),
			ID:   _testID.String(),
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasDBUpdateDataSourceCalled(0),
			),
		},
		"Changed status updates the data source": {
			DB:   stubDB(stubDataSource(datasource.TypePostgreSQL), nil),
			Exec: stubExec(processor.ConnectionStatusUnreachable, nil, nil),
			ID:   _testID.String(),
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"data_source.unreachable","message":"The data source is unreachable."}`),
				wasDBUpdateDataSourceCalled(1),
			),
		},
		"Error returned by db.UpdateDataSource": {
			DB: func() *DBMock {
				db := stubDB(stubDataSource(datasource.TypePostgreSQL), nil)
				db.UpdateDataSourceFunc = func(_ context.Context, _ *datasource.DataSource) error {
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
			DB: stubDB(stubDataSource(datasource.TypePostgreSQL), nil),
			Exec: stubExec(processor.ConnectionStatusSuccess, &processor.SQLMetadataResult{
				Tables: map[string]processor.SQLTable{
					"public.users": {Columns: []processor.SQLColumn{{Name: "id"}}},
				},
				DefaultSchema: "public",
			}, nil),
			ID: _testID.String(),
			Checks: checks(
				hasResp(http.StatusOK, `{"tables":{"public.users":{"columns":[{"name":"id"}]}},"defaultSchema":"public"}`),
				wasDBUpdateDataSourceCalled(0),
				wasExecutorSQLMetadataCalled(1),
			),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl, db, exec, logs := prepHandler(c.DB, c.Exec)

			req := prepRequest("GET", "http://test.com/", "", !c.OmitSession, c.ID)
			rec := httptest.NewRecorder()

			hdl.FetchSQLMetadata(rec, req)

			for _, ch := range c.Checks {
				ch(t, db, exec, rec, logs)
			}
		})
	}
}
