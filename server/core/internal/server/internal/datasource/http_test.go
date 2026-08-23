package datasource

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	datasourceCore "github.com/oxynote/oxynote/server/core/internal/datasource"
	datasourceMock "github.com/oxynote/oxynote/server/core/internal/datasource/_mock"
	"github.com/oxynote/oxynote/server/core/internal/datasource/processor"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/auth"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

var (
	// _testID is the data source ID used across handler tests.
	_testID = xid.New()

	// _testSession is the authenticated session used across handler tests.
	_testSession = auth.Session{
		UserID:               "user-1",
		ActiveOrganizationID: "org-1",
	}
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// check inspects the collaborators' state after a handler call.
type check func(t *testing.T, db *DBMock, exec *runnerMock, rec *httptest.ResponseRecorder, logs *bytes.Buffer)

// checks combines the provided checks into a slice.
func checks(cc ...check) []check { return cc }

// hasResp returns a check asserting the response code and JSON body; an
// empty body asserts an empty response.
func hasResp(code int, body string) check {
	return func(t *testing.T, _ *DBMock, _ *runnerMock, rec *httptest.ResponseRecorder, _ *bytes.Buffer) {
		assert.Equal(t, code, rec.Code)

		if body == "" {
			assert.Zero(t, rec.Body.Len(), rec.Body.String())
			return
		}

		assert.JSONEq(t, body, rec.Body.String())
	}
}

// hasJSONResp returns a check asserting that the response matches the
// JSON representation of the provided value.
func hasJSONResp(code int, v any) check {
	return func(t *testing.T, _ *DBMock, _ *runnerMock, rec *httptest.ResponseRecorder, _ *bytes.Buffer) {
		assert.Equal(t, code, rec.Code)

		exp, err := json.Marshal(v)
		require.NoError(t, err)
		assert.JSONEq(t, string(exp), rec.Body.String())
	}
}

// hasUpdateFailedLog returns a check asserting that the handler logged
// the data source status update failure.
func hasUpdateFailedLog() check {
	return func(t *testing.T, _ *DBMock, _ *runnerMock, _ *httptest.ResponseRecorder, logs *bytes.Buffer) {
		assert.Contains(t, logs.String(), "cannot update data source status")
	}
}

// wasDBFetchDataSourceCalled returns a check asserting the number of
// FetchDataSource calls and their parameters.
func wasDBFetchDataSourceCalled(count int) check {
	return func(t *testing.T, db *DBMock, _ *runnerMock, _ *httptest.ResponseRecorder, _ *bytes.Buffer) {
		ff := db.FetchDataSourceCalls()
		require.Len(t, ff, count)

		if count == 0 {
			return
		}

		assert.NotNil(t, ff[0].Ctx)
		assert.Equal(t, _testID, ff[0].ID)
		assert.Equal(t, _testSession.ActiveOrganizationID, ff[0].OrganizationID)
	}
}

// wasDBInsertDataSourceCalled returns a check asserting the number of
// InsertDataSource calls.
func wasDBInsertDataSourceCalled(count int) check {
	return func(t *testing.T, db *DBMock, _ *runnerMock, _ *httptest.ResponseRecorder, _ *bytes.Buffer) {
		require.Len(t, db.InsertDataSourceCalls(), count)
	}
}

// wasDBUpdateDataSourceCalled returns a check asserting the number of
// UpdateDataSource calls.
func wasDBUpdateDataSourceCalled(count int) check {
	return func(t *testing.T, db *DBMock, _ *runnerMock, _ *httptest.ResponseRecorder, _ *bytes.Buffer) {
		require.Len(t, db.UpdateDataSourceCalls(), count)
	}
}

// wasDBDeleteDataSourceCalled returns a check asserting the number of
// DeleteDataSource calls and their parameters.
func wasDBDeleteDataSourceCalled(count int) check {
	return func(t *testing.T, db *DBMock, _ *runnerMock, _ *httptest.ResponseRecorder, _ *bytes.Buffer) {
		ff := db.DeleteDataSourceCalls()
		require.Len(t, ff, count)

		if count == 0 {
			return
		}

		assert.Equal(t, _testID, ff[0].ID)
		assert.Equal(t, _testSession.ActiveOrganizationID, ff[0].OrganizationID)
	}
}

// wasExecutorTestConnectionCalled returns a check asserting the number of
// TestConnection calls.
func wasExecutorTestConnectionCalled(count int) check {
	return func(t *testing.T, _ *DBMock, exec *runnerMock, _ *httptest.ResponseRecorder, _ *bytes.Buffer) {
		require.Len(t, exec.runner.TestConnectionCalls(), count)
	}
}

// stubDataSource returns a data source owned by the test organization.
func stubDataSource(typ datasourceCore.Type) *datasourceCore.DataSource {
	return &datasourceCore.DataSource{
		ID:             _testID,
		OrganizationID: _testSession.ActiveOrganizationID,
		Name:           "test-source",
		Type:           typ,
		URL:            "http://source.test",
		Status:         processor.ConnectionStatusSuccess,
		CreatedAt:      time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	}
}

// stubDB returns a DB mock whose FetchDataSource returns the provided
// data source or error.
func stubDB(ds *datasourceCore.DataSource, fetchErr error) *DBMock {
	return &DBMock{
		FetchDataSourceFunc: func(_ context.Context, _ xid.ID, _ string) (*datasourceCore.DataSource, error) {
			if fetchErr != nil {
				return nil, fetchErr
			}

			return ds, nil
		},
	}
}

// prepRequest builds a handler test request. The session is dropped when
// session is false and the dataSourceId URL parameter when id is empty.
func prepRequest(method, target, body string, session bool, id string) *http.Request {
	rdr := io.Reader(http.NoBody)
	if body != "" {
		rdr = strings.NewReader(body)
	}

	req := httptest.NewRequest(method, target, rdr)

	if session {
		req = req.WithContext(auth.AddSessionToContext(req.Context(), _testSession))
	}

	if id != "" {
		req = req.WithContext(testutil.AddChiCtx(req.Context(), "dataSourceId", id))
	}

	return req
}

// clientMocks are what a case's runner hands out: the typed clients the
// handler operates through, what its connection test reports, and the
// refusal that stands in for a data source that cannot serve any of
// them.
type clientMocks struct {
	// prometheus, sql, postgreSQL and mySQL are the clients the
	// accessors return. A check reads back what the handler asked them.
	prometheus *datasourceMock.Prometheus
	sql        *datasourceMock.SQL
	postgreSQL *datasourceMock.PostgreSQL
	mySQL      *datasourceMock.MySQL

	// status and statusErr are what TestConnection reports.
	status    processor.ConnectionStatus
	statusErr error

	// err, when set, is how every accessor refuses — a data source of
	// the wrong type, or a connection it would not open.
	err error
}

// runnerMock is what the handler is wired with: one runner over those
// clients, plus the data sources it was asked to operate. That list is
// where "the handler ran against the data source it fetched" is
// asserted, the runner no longer being told which one it serves on
// every call.
type runnerMock struct {
	*clientMocks

	// runner is the runner every call is handed, so what the handler
	// asked it accumulates in one place.
	runner *datasourceMock.Runner

	// dataSources are the data sources a runner was asked for, in call
	// order.
	dataSources []datasourceCore.DataSource
}

// newRunnerMock wires one runner over the given clients.
func newRunnerMock(clients *clientMocks) *runnerMock {
	if clients == nil {
		clients = &clientMocks{}
	}

	if clients.prometheus == nil {
		clients.prometheus = &datasourceMock.Prometheus{}
	}

	if clients.sql == nil {
		clients.sql = &datasourceMock.SQL{}
	}

	if clients.postgreSQL == nil {
		clients.postgreSQL = &datasourceMock.PostgreSQL{}
	}

	if clients.mySQL == nil {
		clients.mySQL = &datasourceMock.MySQL{}
	}

	m := &runnerMock{clientMocks: clients}

	m.runner = &datasourceMock.Runner{
		TestConnectionFunc: func(context.Context) (processor.ConnectionStatus, error) {
			return clients.status, clients.statusErr
		},
		PrometheusFunc: func(context.Context) (datasourceCore.Prometheus, error) {
			if clients.err != nil {
				return nil, clients.err
			}

			return clients.prometheus, nil
		},
		SQLFunc: func(context.Context) (datasourceCore.SQL, error) {
			if clients.err != nil {
				return nil, clients.err
			}

			return clients.sql, nil
		},
		PostgreSQLFunc: func(context.Context) (datasourceCore.PostgreSQL, error) {
			if clients.err != nil {
				return nil, clients.err
			}

			return clients.postgreSQL, nil
		},
		MySQLFunc: func(context.Context) (datasourceCore.MySQL, error) {
			if clients.err != nil {
				return nil, clients.err
			}

			return clients.mySQL, nil
		},
	}

	return m
}

// Runner records the data source and hands back the one runner.
func (m *runnerMock) Runner(ds datasourceCore.DataSource) datasourceCore.Runner {
	m.dataSources = append(m.dataSources, ds)

	return m.runner
}

// prepHandler builds a handler around the provided mocks, defaulting nil
// mocks to empty stubs, and returns the buffer its log writes to.
func prepHandler(db *DBMock, clients *clientMocks) (*Handler, *DBMock, *runnerMock, *bytes.Buffer) {
	if db == nil {
		db = &DBMock{}
	}

	runner := newRunnerMock(clients)
	logs := &bytes.Buffer{}

	hdl := &Handler{
		log:     slog.New(slog.NewTextHandler(logs, nil)),
		db:      db,
		runners: runner,
	}

	return hdl, db, runner, logs
}

func Test_NewHandler(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.DiscardHandler)
	db := &DBMock{}

	runners := datasourceCore.NewManager(log, &datasourceMock.StatusStore{})

	hdl := NewHandler(log, db, runners)
	require.NotNil(t, hdl)
	assert.Equal(t, log, hdl.log)
	assert.Same(t, db, hdl.db)
	assert.Same(t, runners, hdl.runners)
}

func Test_Handler_CreateDataSource(t *testing.T) {
	hasCreatedDataSourceResp := func() check {
		return func(t *testing.T, db *DBMock, exec *runnerMock, rec *httptest.ResponseRecorder, _ *bytes.Buffer) {
			ff := db.InsertDataSourceCalls()
			require.Len(t, ff, 1)

			ds := ff[0].Ds
			require.NotNil(t, ds)
			assert.False(t, ds.ID.IsNil())
			assert.Equal(t, _testSession.ActiveOrganizationID, ds.OrganizationID)
			assert.Equal(t, "test-source", ds.Name)
			assert.Equal(t, datasourceCore.TypePrometheus, ds.Type)
			assert.Equal(t, "http://source.test", ds.URL)
			assert.Equal(t, processor.ConnectionStatusSuccess, ds.Status)

			require.Len(t, exec.runner.TestConnectionCalls(), 1)
			require.Len(t, exec.dataSources, 1)
			assert.Equal(t, ds.Name, exec.dataSources[0].Name)

			assert.Equal(t, http.StatusCreated, rec.Code)

			exp, err := json.Marshal(ds)
			require.NoError(t, err)
			assert.JSONEq(t, string(exp), rec.Body.String())
		}
	}

	body := `{"type":"prometheus","name":"test-source","url":"http://source.test","credentials":{"username":"user"}}`

	cc := map[string]struct {
		DB          *DBMock
		Clients     *clientMocks
		Body        string
		OmitSession bool
		Checks      []check
	}{
		"Not authenticated": {
			Body:        body,
			OmitSession: true,
			Checks: checks(
				hasResp(http.StatusUnauthorized, `{"code":"account.not_authenticated","message":"not authenticated"}`),
				wasExecutorTestConnectionCalled(0),
				wasDBInsertDataSourceCalled(0),
			),
		},
		"Invalid JSON body": {
			Body: "{",
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"request.invalid_json","message":"invalid JSON body"}`),
				wasExecutorTestConnectionCalled(0),
				wasDBInsertDataSourceCalled(0),
			),
		},
		"Error returned by executor.TestConnection": {
			Clients: &clientMocks{status: "", statusErr: assert.AnError},
			Body:    body,
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasDBInsertDataSourceCalled(0),
			),
		},
		"Unsuccessful connection status": {
			Clients: &clientMocks{status: processor.ConnectionStatusUnauthorized, statusErr: nil},
			Body:    body,
			Checks: checks(
				hasResp(http.StatusUnauthorized, `{"code":"data_source.unauthorized","message":"Unauthorized access to the data source."}`),
				wasExecutorTestConnectionCalled(1),
				wasDBInsertDataSourceCalled(0),
			),
		},
		"Error returned by db.InsertDataSource": {
			DB: &DBMock{
				InsertDataSourceFunc: func(_ context.Context, _ *datasourceCore.DataSource) error {
					return assert.AnError
				},
			},
			Clients: &clientMocks{status: processor.ConnectionStatusSuccess, statusErr: nil},
			Body:    body,
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasDBInsertDataSourceCalled(1),
			),
		},
		"Successful creation": {
			Clients: &clientMocks{status: processor.ConnectionStatusSuccess, statusErr: nil},
			Body:    body,
			Checks: checks(
				hasCreatedDataSourceResp(),
			),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl, db, exec, logs := prepHandler(c.DB, c.Clients)

			req := prepRequest("POST", "http://test.com/", c.Body, !c.OmitSession, "")
			rec := httptest.NewRecorder()

			hdl.CreateDataSource(rec, req)

			for _, ch := range c.Checks {
				ch(t, db, exec, rec, logs)
			}
		})
	}
}

func Test_Handler_FetchDataSource(t *testing.T) {
	cc := map[string]struct {
		DB          *DBMock
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
			),
		},
		"Missing data source id": {
			Checks: checks(
				hasResp(http.StatusNotFound, `{"code":"general","message":"not found"}`),
				wasDBFetchDataSourceCalled(0),
			),
		},
		"Invalid data source id": {
			ID: "bogus",
			Checks: checks(
				hasResp(http.StatusNotFound, `{"code":"general","message":"not found"}`),
				wasDBFetchDataSourceCalled(0),
			),
		},
		"Error returned by db.FetchDataSource": {
			DB: stubDB(nil, assert.AnError),
			ID: _testID.String(),
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasDBFetchDataSourceCalled(1),
			),
		},
		"Successful fetch": {
			DB: stubDB(stubDataSource(datasourceCore.TypePrometheus), nil),
			ID: _testID.String(),
			Checks: checks(
				hasJSONResp(http.StatusOK, stubDataSource(datasourceCore.TypePrometheus)),
				wasDBFetchDataSourceCalled(1),
			),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl, db, exec, logs := prepHandler(c.DB, nil)

			req := prepRequest("GET", "http://test.com/", "", !c.OmitSession, c.ID)
			rec := httptest.NewRecorder()

			hdl.FetchDataSource(rec, req)

			for _, ch := range c.Checks {
				ch(t, db, exec, rec, logs)
			}
		})
	}
}

func Test_Handler_TestDataSourceConnection(t *testing.T) {
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
				wasExecutorTestConnectionCalled(0),
			),
		},
		"Missing data source id": {
			Checks: checks(
				hasResp(http.StatusNotFound, `{"code":"general","message":"not found"}`),
				wasExecutorTestConnectionCalled(0),
			),
		},
		"Error returned by db.FetchDataSource": {
			DB: stubDB(nil, assert.AnError),
			ID: _testID.String(),
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasExecutorTestConnectionCalled(0),
			),
		},
		"Error returned by executor.TestConnection": {
			DB:      stubDB(stubDataSource(datasourceCore.TypePrometheus), nil),
			Clients: &clientMocks{status: "", statusErr: assert.AnError},
			ID:      _testID.String(),
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasDBUpdateDataSourceCalled(0),
			),
		},
		"Unchanged status skips the update": {
			DB:      stubDB(stubDataSource(datasourceCore.TypePrometheus), nil),
			Clients: &clientMocks{status: processor.ConnectionStatusSuccess, statusErr: nil},
			ID:      _testID.String(),
			Checks: checks(
				hasResp(http.StatusOK, `{"status":"success"}`),
				wasDBUpdateDataSourceCalled(0),
			),
		},
		"Changed status updates the data source": {
			DB:      stubDB(stubDataSource(datasourceCore.TypePrometheus), nil),
			Clients: &clientMocks{status: processor.ConnectionStatusUnreachable, statusErr: nil},
			ID:      _testID.String(),
			Checks: checks(
				hasResp(http.StatusOK, `{"status":"unreachable"}`),
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
			Clients: &clientMocks{status: processor.ConnectionStatusUnreachable, statusErr: nil},
			ID:      _testID.String(),
			Checks: checks(
				hasResp(http.StatusOK, `{"status":"unreachable"}`),
				wasDBUpdateDataSourceCalled(1),
				hasUpdateFailedLog(),
			),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl, db, exec, logs := prepHandler(c.DB, c.Clients)

			req := prepRequest("POST", "http://test.com/", "", !c.OmitSession, c.ID)
			rec := httptest.NewRecorder()

			hdl.TestDataSourceConnection(rec, req)

			for _, ch := range c.Checks {
				ch(t, db, exec, rec, logs)
			}
		})
	}
}

func Test_Handler_FetchDataSources(t *testing.T) {
	wasDBFetchDataSourcesCalled := func(count int) check {
		return func(t *testing.T, db *DBMock, _ *runnerMock, _ *httptest.ResponseRecorder, _ *bytes.Buffer) {
			ff := db.FetchDataSourcesCalls()
			require.Len(t, ff, count)

			if count == 0 {
				return
			}

			assert.Equal(t, _testSession.ActiveOrganizationID, ff[0].OrganizationID)
		}
	}

	cc := map[string]struct {
		DB          *DBMock
		OmitSession bool
		Checks      []check
	}{
		"Not authenticated": {
			OmitSession: true,
			Checks: checks(
				hasResp(http.StatusUnauthorized, `{"code":"account.not_authenticated","message":"not authenticated"}`),
				wasDBFetchDataSourcesCalled(0),
			),
		},
		"Error returned by db.FetchDataSources": {
			DB: &DBMock{
				FetchDataSourcesFunc: func(_ context.Context, _ string) ([]datasourceCore.DataSource, error) {
					return nil, assert.AnError
				},
			},
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasDBFetchDataSourcesCalled(1),
			),
		},
		"Successful fetch": {
			DB: &DBMock{
				FetchDataSourcesFunc: func(_ context.Context, _ string) ([]datasourceCore.DataSource, error) {
					return []datasourceCore.DataSource{*stubDataSource(datasourceCore.TypePrometheus)}, nil
				},
			},
			Checks: checks(
				hasJSONResp(http.StatusOK, []datasourceCore.DataSource{*stubDataSource(datasourceCore.TypePrometheus)}),
				wasDBFetchDataSourcesCalled(1),
			),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl, db, exec, logs := prepHandler(c.DB, nil)

			req := prepRequest("GET", "http://test.com/", "", !c.OmitSession, "")
			rec := httptest.NewRecorder()

			hdl.FetchDataSources(rec, req)

			for _, ch := range c.Checks {
				ch(t, db, exec, rec, logs)
			}
		})
	}
}

func Test_Handler_UpdateDataSource(t *testing.T) {
	hasUpdatedDataSourceResp := func() check {
		return func(t *testing.T, db *DBMock, _ *runnerMock, rec *httptest.ResponseRecorder, _ *bytes.Buffer) {
			ff := db.UpdateDataSourceCalls()
			require.Len(t, ff, 1)

			ds := ff[0].Ds
			require.NotNil(t, ds)
			assert.Equal(t, "renamed-source", ds.Name)
			assert.True(t, ds.UpdatedAt.Valid)

			assert.Equal(t, http.StatusOK, rec.Code)

			exp, err := json.Marshal(ds)
			require.NoError(t, err)
			assert.JSONEq(t, string(exp), rec.Body.String())
		}
	}

	body := `{"name":"renamed-source"}`

	cc := map[string]struct {
		DB          *DBMock
		Clients     *clientMocks
		Body        string
		OmitSession bool
		ID          string
		Checks      []check
	}{
		"Not authenticated": {
			Body:        body,
			OmitSession: true,
			ID:          _testID.String(),
			Checks: checks(
				hasResp(http.StatusUnauthorized, `{"code":"account.not_authenticated","message":"not authenticated"}`),
				wasDBFetchDataSourceCalled(0),
			),
		},
		"Missing data source id": {
			Body: body,
			Checks: checks(
				hasResp(http.StatusNotFound, `{"code":"general","message":"not found"}`),
				wasDBFetchDataSourceCalled(0),
			),
		},
		"Invalid JSON body": {
			Body: "{",
			ID:   _testID.String(),
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"request.invalid_json","message":"invalid JSON body"}`),
				wasDBFetchDataSourceCalled(0),
			),
		},
		"Error returned by db.FetchDataSource": {
			DB:   stubDB(nil, assert.AnError),
			Body: body,
			ID:   _testID.String(),
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasExecutorTestConnectionCalled(0),
			),
		},
		"Error returned by DataSource.ApplyUpdate": {
			DB:   stubDB(stubDataSource(datasourceCore.Type("bogus")), nil),
			Body: `{"credentials":{"username":"user"}}`,
			ID:   _testID.String(),
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasExecutorTestConnectionCalled(0),
			),
		},
		"Error returned by executor.TestConnection": {
			DB:      stubDB(stubDataSource(datasourceCore.TypePrometheus), nil),
			Clients: &clientMocks{status: "", statusErr: assert.AnError},
			Body:    body,
			ID:      _testID.String(),
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasDBUpdateDataSourceCalled(0),
			),
		},
		"Unsuccessful connection status": {
			DB:      stubDB(stubDataSource(datasourceCore.TypePrometheus), nil),
			Clients: &clientMocks{status: processor.ConnectionStatusUnreachable, statusErr: nil},
			Body:    body,
			ID:      _testID.String(),
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"data_source.unreachable","message":"The data source is unreachable."}`),
				wasDBUpdateDataSourceCalled(0),
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
			Clients: &clientMocks{status: processor.ConnectionStatusSuccess, statusErr: nil},
			Body:    body,
			ID:      _testID.String(),
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasDBUpdateDataSourceCalled(1),
			),
		},
		"Successful update": {
			DB:      stubDB(stubDataSource(datasourceCore.TypePrometheus), nil),
			Clients: &clientMocks{status: processor.ConnectionStatusSuccess, statusErr: nil},
			Body:    body,
			ID:      _testID.String(),
			Checks: checks(
				hasUpdatedDataSourceResp(),
			),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl, db, exec, logs := prepHandler(c.DB, c.Clients)

			req := prepRequest("PUT", "http://test.com/", c.Body, !c.OmitSession, c.ID)
			rec := httptest.NewRecorder()

			hdl.UpdateDataSource(rec, req)

			for _, ch := range c.Checks {
				ch(t, db, exec, rec, logs)
			}
		})
	}
}

func Test_Handler_DeleteDataSource(t *testing.T) {
	cc := map[string]struct {
		DB          *DBMock
		OmitSession bool
		ID          string
		Checks      []check
	}{
		"Not authenticated": {
			OmitSession: true,
			ID:          _testID.String(),
			Checks: checks(
				hasResp(http.StatusUnauthorized, `{"code":"account.not_authenticated","message":"not authenticated"}`),
				wasDBDeleteDataSourceCalled(0),
			),
		},
		"Missing data source id": {
			Checks: checks(
				hasResp(http.StatusNotFound, `{"code":"general","message":"not found"}`),
				wasDBDeleteDataSourceCalled(0),
			),
		},
		"Error returned by db.DeleteDataSource": {
			DB: &DBMock{
				DeleteDataSourceFunc: func(_ context.Context, _ xid.ID, _ string) error {
					return assert.AnError
				},
			},
			ID: _testID.String(),
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasDBDeleteDataSourceCalled(1),
			),
		},
		"Successful deletion": {
			ID: _testID.String(),
			Checks: checks(
				hasResp(http.StatusNoContent, ""),
				wasDBDeleteDataSourceCalled(1),
			),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl, db, exec, logs := prepHandler(c.DB, nil)

			req := prepRequest("DELETE", "http://test.com/", "", !c.OmitSession, c.ID)
			rec := httptest.NewRecorder()

			hdl.DeleteDataSource(rec, req)

			for _, ch := range c.Checks {
				ch(t, db, exec, rec, logs)
			}
		})
	}
}

func Test_Handler_persistDataSourceStatus(t *testing.T) {
	cc := map[string]struct {
		DB     *DBMock
		Status processor.ConnectionStatus
		Result processor.ConnectionStatus
		Checks []check
	}{
		"Unchanged status skips the update": {
			Status: processor.ConnectionStatusSuccess,
			Result: processor.ConnectionStatusSuccess,
			Checks: checks(wasDBUpdateDataSourceCalled(0)),
		},
		"Error returned by db.UpdateDataSource": {
			DB: &DBMock{
				UpdateDataSourceFunc: func(_ context.Context, _ *datasourceCore.DataSource) error {
					return assert.AnError
				},
			},
			Status: processor.ConnectionStatusUnreachable,
			Result: processor.ConnectionStatusUnreachable,
			Checks: checks(
				wasDBUpdateDataSourceCalled(1),
				hasUpdateFailedLog(),
			),
		},
		"Changed status is stored": {
			Status: processor.ConnectionStatusUnreachable,
			Result: processor.ConnectionStatusUnreachable,
			Checks: checks(wasDBUpdateDataSourceCalled(1)),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl, db, exec, logs := prepHandler(c.DB, nil)
			ds := stubDataSource(datasourceCore.TypePrometheus)

			hdl.persistDataSourceStatus(prepRequest("GET", "http://test.com/", "", true, ""), ds, c.Status)

			assert.Equal(t, c.Result, ds.Status)

			for _, ch := range c.Checks {
				ch(t, db, exec, httptest.NewRecorder(), logs)
			}
		})
	}
}
