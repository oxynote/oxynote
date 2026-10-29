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

	"github.com/guregu/null/v5"
	datasourceCore "github.com/oxynote/oxynote/server/core/internal/datasource"
	datasourceMock "github.com/oxynote/oxynote/server/core/internal/datasource/_mock"
	"github.com/oxynote/oxynote/server/core/internal/datasource/processor"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/auth"
	"github.com/oxynote/oxynote/server/core/internal/variable"
	"github.com/oxynote/oxynote/server/core/pkg/metricutil"
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
type check func(t *testing.T, db *DBMock, exec *runnerMock, vars *VariableManagerMock, rec *httptest.ResponseRecorder, logs *bytes.Buffer)

// checks combines the provided checks into a slice.
func checks(cc ...check) []check { return cc }

// hasResp returns a check asserting the response code and JSON body; an
// empty body asserts an empty response.
func hasResp(code int, body string) check {
	return func(t *testing.T, _ *DBMock, _ *runnerMock, _ *VariableManagerMock, rec *httptest.ResponseRecorder, _ *bytes.Buffer) {
		testutil.AssertResponse(t, rec, code, body)
	}
}

// hasJSONResp returns a check asserting that the response matches the
// JSON representation of the provided value.
func hasJSONResp(code int, v any) check {
	return func(t *testing.T, _ *DBMock, _ *runnerMock, _ *VariableManagerMock, rec *httptest.ResponseRecorder, _ *bytes.Buffer) {
		assert.Equal(t, code, rec.Code)

		exp, err := json.Marshal(v)
		require.NoError(t, err)
		assert.JSONEq(t, string(exp), rec.Body.String())
	}
}

// hasUpdateFailedLog returns a check asserting that the handler logged
// the data source status update failure.
func hasUpdateFailedLog() check {
	return func(t *testing.T, _ *DBMock, _ *runnerMock, _ *VariableManagerMock, _ *httptest.ResponseRecorder, logs *bytes.Buffer) {
		assert.Contains(t, logs.String(), "cannot update data source status")
	}
}

// wasDBFetchDataSourceCalled returns a check asserting the number of
// FetchDataSource calls and their parameters.
func wasDBFetchDataSourceCalled(count int) check {
	return func(t *testing.T, db *DBMock, _ *runnerMock, _ *VariableManagerMock, _ *httptest.ResponseRecorder, _ *bytes.Buffer) {
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
	return func(t *testing.T, db *DBMock, _ *runnerMock, _ *VariableManagerMock, _ *httptest.ResponseRecorder, _ *bytes.Buffer) {
		require.Len(t, db.InsertDataSourceCalls(), count)
	}
}

// wasDBUpdateDataSourceCalled returns a check asserting the number of
// UpdateDataSource calls.
func wasDBUpdateDataSourceCalled(count int) check {
	return func(t *testing.T, db *DBMock, _ *runnerMock, _ *VariableManagerMock, _ *httptest.ResponseRecorder, _ *bytes.Buffer) {
		require.Len(t, db.UpdateDataSourceCalls(), count)
	}
}

// wasDBDeleteDataSourceCalled returns a check asserting the number of
// DeleteDataSource calls and their parameters.
func wasDBDeleteDataSourceCalled(count int) check {
	return func(t *testing.T, db *DBMock, _ *runnerMock, _ *VariableManagerMock, _ *httptest.ResponseRecorder, _ *bytes.Buffer) {
		ff := db.DeleteDataSourceCalls()
		require.Len(t, ff, count)

		if count == 0 {
			return
		}

		assert.Equal(t, _testID, ff[0].ID)
		assert.Equal(t, _testSession.ActiveOrganizationID, ff[0].OrganizationID)
	}
}

// wasVariableManagerFetchTreeCalled returns a check asserting the number
// of FetchTree calls and that each was for the session's organization and
// user.
func wasVariableManagerFetchTreeCalled(count int) check {
	return func(t *testing.T, _ *DBMock, _ *runnerMock, vars *VariableManagerMock, _ *httptest.ResponseRecorder, _ *bytes.Buffer) {
		ff := vars.FetchTreeCalls()
		require.Len(t, ff, count)

		for _, f := range ff {
			assert.Equal(t, _testSession.ActiveOrganizationID, f.OrganizationID)
			assert.Equal(t, _testSession.UserID, f.UserID)
		}
	}
}

// wasExecutorTestConnectionCalled returns a check asserting the number of
// TestConnection calls.
func wasExecutorTestConnectionCalled(count int) check {
	return func(t *testing.T, _ *DBMock, exec *runnerMock, _ *VariableManagerMock, _ *httptest.ResponseRecorder, _ *bytes.Buffer) {
		require.Len(t, exec.runner.TestConnectionCalls(), count)
	}
}

// wasRunnersRunnerCalled returns a check asserting the number of Runner
// calls.
func wasRunnersRunnerCalled(count int) check {
	return func(t *testing.T, _ *DBMock, exec *runnerMock, _ *VariableManagerMock, _ *httptest.ResponseRecorder, _ *bytes.Buffer) {
		require.Len(t, exec.dataSources, count)
	}
}

// wasRunnersEvictCalled returns a check asserting the number of
// Runners.Evict calls, each for the test data source.
func wasRunnersEvictCalled(count int) check {
	return func(t *testing.T, _ *DBMock, exec *runnerMock, _ *VariableManagerMock, _ *httptest.ResponseRecorder, _ *bytes.Buffer) {
		require.Len(t, exec.evicted, count)

		for _, id := range exec.evicted {
			assert.Equal(t, _testID, id)
		}
	}
}

// wasExecutorPrometheusCalled returns a check asserting the number of
// Runner.Prometheus calls.
func wasExecutorPrometheusCalled(count int) check {
	return func(t *testing.T, _ *DBMock, exec *runnerMock, _ *VariableManagerMock, _ *httptest.ResponseRecorder, _ *bytes.Buffer) {
		require.Len(t, exec.runner.PrometheusCalls(), count)
	}
}

// wasExecutorSQLCalled returns a check asserting the number of Runner.SQL
// calls.
func wasExecutorSQLCalled(count int) check {
	return func(t *testing.T, _ *DBMock, exec *runnerMock, _ *VariableManagerMock, _ *httptest.ResponseRecorder, _ *bytes.Buffer) {
		require.Len(t, exec.runner.SQLCalls(), count)
	}
}

// wasExecutorLogsCalled returns a check asserting the number of
// Runner.Logs calls.
func wasExecutorLogsCalled(count int) check {
	return func(t *testing.T, _ *DBMock, exec *runnerMock, _ *VariableManagerMock, _ *httptest.ResponseRecorder, _ *bytes.Buffer) {
		require.Len(t, exec.runner.LogsCalls(), count)
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

// stubDataSourceWithCredentials returns a PostgreSQL data source holding
// the given credentials JSON.
func stubDataSourceWithCredentials(creds string) *datasourceCore.DataSource {
	ds := stubDataSource(datasourceCore.TypePostgreSQL)
	ds.Credentials = processor.NewCredentials([]byte(creds))

	return ds
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
	// prometheus, sql, postgreSQL, mySQL and logSource are the clients
	// the accessors return. A check reads back what the handler asked
	// them.
	prometheus *datasourceMock.Prometheus
	sql        *datasourceMock.SQL
	postgreSQL *datasourceMock.PostgreSQL
	mySQL      *datasourceMock.MySQL
	logSource  *datasourceMock.Logs

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

	// evicted are the data source ids whose connections the handler had
	// closed, in call order.
	evicted []xid.ID
}

// Runner records the data source and hands back the one runner.
func (m *runnerMock) Runner(ds datasourceCore.DataSource) datasourceCore.Runner {
	m.dataSources = append(m.dataSources, ds)

	return m.runner
}

// Evict records the data source id.
func (m *runnerMock) Evict(id xid.ID) {
	m.evicted = append(m.evicted, id)
}

// stubVariables returns a variable manager whose tree resolves $metric to
// up, but only for the test session's organization and user.
func stubVariables(err error) *VariableManagerMock {
	return &VariableManagerMock{
		FetchTreeFunc: func(_ context.Context, organizationID, userID string) (variable.Tree, error) {
			if err != nil {
				return nil, err
			}

			if organizationID != _testSession.ActiveOrganizationID || userID != _testSession.UserID {
				return variable.Tree{}, nil
			}

			return variable.Tree{{Variables: []variable.Entry{{
				Key:   "metric",
				Type:  variable.TypeText,
				Value: null.StringFrom("up"),
			}}}}, nil
		},
	}
}

// prepHandler builds a handler around the provided mocks, wiring one
// runner over the clients and defaulting nil mocks to empty stubs, and
// returns the buffer its log writes to.
func prepHandler(db *DBMock, clients *clientMocks, vars *VariableManagerMock) (*Handler, *DBMock, *runnerMock, *VariableManagerMock, *bytes.Buffer) {
	if db == nil {
		db = &DBMock{}
	}

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

	if clients.logSource == nil {
		clients.logSource = &datasourceMock.Logs{}
	}

	if vars == nil {
		vars = &VariableManagerMock{}
	}

	exec := &runnerMock{clientMocks: clients}

	exec.runner = &datasourceMock.Runner{
		TypeFunc: func() datasourceCore.Type {
			return exec.dataSources[len(exec.dataSources)-1].Type
		},
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
		LogsFunc: func(context.Context) (datasourceCore.Logs, error) {
			if clients.err != nil {
				return nil, clients.err
			}

			return clients.logSource, nil
		},
	}

	logs := &bytes.Buffer{}

	hdl := &Handler{
		log:       slog.New(slog.NewTextHandler(logs, nil)),
		db:        db,
		runners:   exec,
		variables: vars,
	}

	return hdl, db, exec, vars, logs
}

func Test_NewHandler(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.DiscardHandler)
	db := &DBMock{}

	runners := datasourceCore.NewManager(log, &datasourceMock.StatusStore{}, nil, metricutil.NewFactory("test", nil))

	variables := &VariableManagerMock{}

	hdl := NewHandler(log, db, runners, variables)
	require.NotNil(t, hdl)
	assert.Equal(t, log, hdl.log)
	assert.Same(t, db, hdl.db)
	assert.Same(t, runners, hdl.runners)
	assert.Same(t, variables, hdl.variables)
}

func Test_Handler_CreateDataSource(t *testing.T) {
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
				wasRunnersRunnerCalled(0),
				wasExecutorTestConnectionCalled(0),
				wasDBInsertDataSourceCalled(0),
			),
		},
		"Invalid JSON body": {
			Body: "{",
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"request.invalid_json","message":"invalid JSON body"}`),
				wasRunnersRunnerCalled(0),
				wasExecutorTestConnectionCalled(0),
				wasDBInsertDataSourceCalled(0),
			),
		},
		"Error returned by Runner.TestConnection": {
			Clients: &clientMocks{status: "", statusErr: assert.AnError},
			Body:    body,
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasRunnersRunnerCalled(1),
				wasExecutorTestConnectionCalled(1),
				wasDBInsertDataSourceCalled(0),
			),
		},
		"Unsuccessful connection status": {
			Clients: &clientMocks{status: processor.ConnectionStatusUnauthorized, statusErr: nil},
			Body:    body,
			Checks: checks(
				hasResp(http.StatusUnauthorized, `{"code":"data_source.unauthorized","message":"Unauthorized access to the data source."}`),
				wasRunnersRunnerCalled(1),
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
				wasRunnersRunnerCalled(1),
				wasExecutorTestConnectionCalled(1),
				wasDBInsertDataSourceCalled(1),
			),
		},
		"Successful creation": {
			Clients: &clientMocks{status: processor.ConnectionStatusSuccess, statusErr: nil},
			Body:    body,
			Checks: checks(
				func(t *testing.T, db *DBMock, exec *runnerMock, _ *VariableManagerMock, rec *httptest.ResponseRecorder, _ *bytes.Buffer) {
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

					exp, err := json.Marshal(dataSourceResponse{
						DataSource: *ds,
						Configured: map[string]bool{"username": true},
					})
					require.NoError(t, err)
					assert.JSONEq(t, string(exp), rec.Body.String())
				},
			),
		},
		// nothing is stored, so a list of the organisation's data sources
		// never meets credentials it cannot read.
		"Error returned by CreateInput.Validate": {
			Clients: &clientMocks{status: processor.ConnectionStatusSuccess, statusErr: nil},
			Body:    `{"type":"prometheus","name":"test-source","url":"http://source.test","credentials":[1]}`,
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"data_source.invalid_credentials","message":"credentials must be an object of strings"}`),
				func(t *testing.T, db *DBMock, exec *runnerMock, _ *VariableManagerMock, _ *httptest.ResponseRecorder, _ *bytes.Buffer) {
					t.Helper()

					assert.Empty(t, exec.runner.TestConnectionCalls())
					assert.Empty(t, db.InsertDataSourceCalls())
				},
			),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl, db, exec, vars, logs := prepHandler(c.DB, c.Clients, nil)

			req := prepRequest("POST", "http://test.com/", c.Body, !c.OmitSession, "")
			rec := httptest.NewRecorder()

			hdl.CreateDataSource(rec, req)

			for _, ch := range c.Checks {
				ch(t, db, exec, vars, rec, logs)
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
		"Error returned by Credentials.Configured": {
			DB: stubDB(stubDataSourceWithCredentials(`{`), nil),
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
		// which credentials are set is shown, never their values.
		"Successful fetch with credentials": {
			DB: stubDB(stubDataSourceWithCredentials(`{"username":"user","password":"pass","caCertificate":"pem"}`), nil),
			ID: _testID.String(),
			Checks: checks(
				hasJSONResp(http.StatusOK, dataSourceResponse{
					DataSource: *stubDataSource(datasourceCore.TypePostgreSQL),
					Configured: map[string]bool{"username": true, "password": true, "caCertificate": true},
				}),
				wasDBFetchDataSourceCalled(1),
			),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl, db, exec, vars, logs := prepHandler(c.DB, nil, nil)

			req := prepRequest("GET", "http://test.com/", "", !c.OmitSession, c.ID)
			rec := httptest.NewRecorder()

			hdl.FetchDataSource(rec, req)

			for _, ch := range c.Checks {
				ch(t, db, exec, vars, rec, logs)
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
				wasDBFetchDataSourceCalled(0),
				wasRunnersRunnerCalled(0),
				wasExecutorTestConnectionCalled(0),
				wasDBUpdateDataSourceCalled(0),
				wasRunnersEvictCalled(0),
			),
		},
		"Missing data source id": {
			Checks: checks(
				hasResp(http.StatusNotFound, `{"code":"general","message":"not found"}`),
				wasDBFetchDataSourceCalled(0),
				wasRunnersRunnerCalled(0),
				wasExecutorTestConnectionCalled(0),
				wasDBUpdateDataSourceCalled(0),
				wasRunnersEvictCalled(0),
			),
		},
		"Error returned by db.FetchDataSource": {
			DB: stubDB(nil, assert.AnError),
			ID: _testID.String(),
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasDBFetchDataSourceCalled(1),
				wasRunnersRunnerCalled(0),
				wasExecutorTestConnectionCalled(0),
				wasDBUpdateDataSourceCalled(0),
				wasRunnersEvictCalled(0),
			),
		},
		"Error returned by Runner.TestConnection": {
			DB:      stubDB(stubDataSource(datasourceCore.TypePrometheus), nil),
			Clients: &clientMocks{status: "", statusErr: assert.AnError},
			ID:      _testID.String(),
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasDBFetchDataSourceCalled(1),
				wasRunnersRunnerCalled(1),
				wasExecutorTestConnectionCalled(1),
				wasDBUpdateDataSourceCalled(0),
				wasRunnersEvictCalled(0),
			),
		},
		"Unchanged status skips the update": {
			DB:      stubDB(stubDataSource(datasourceCore.TypePrometheus), nil),
			Clients: &clientMocks{status: processor.ConnectionStatusSuccess, statusErr: nil},
			ID:      _testID.String(),
			Checks: checks(
				hasResp(http.StatusOK, `{"status":"success"}`),
				wasDBFetchDataSourceCalled(1),
				wasRunnersRunnerCalled(1),
				wasExecutorTestConnectionCalled(1),
				wasDBUpdateDataSourceCalled(0),
				wasRunnersEvictCalled(0),
			),
		},
		"Changed status updates the data source": {
			DB:      stubDB(stubDataSource(datasourceCore.TypePrometheus), nil),
			Clients: &clientMocks{status: processor.ConnectionStatusUnreachable, statusErr: nil},
			ID:      _testID.String(),
			Checks: checks(
				hasResp(http.StatusOK, `{"status":"unreachable"}`),
				wasDBFetchDataSourceCalled(1),
				wasRunnersRunnerCalled(1),
				wasExecutorTestConnectionCalled(1),
				wasDBUpdateDataSourceCalled(1),
				wasRunnersEvictCalled(1),
			),
		},
		"Credentials that cannot be decrypted are reported": {
			DB: func() *DBMock {
				ds := stubDataSource(datasourceCore.TypePrometheus)
				ds.Status = processor.ConnectionStatusInvalidEncryptionKey

				return stubDB(ds, nil)
			}(),
			Clients: &clientMocks{status: processor.ConnectionStatusInvalidEncryptionKey, statusErr: nil},
			ID:      _testID.String(),
			Checks: checks(
				hasResp(http.StatusOK, `{"status":"invalid_encryption_key"}`),
				wasDBFetchDataSourceCalled(1),
				wasRunnersRunnerCalled(1),
				wasExecutorTestConnectionCalled(1),
				// the read already recorded the status, and a write here
				// would store an encryption of the credentials it could
				// not read in place of the ones it could not.
				wasDBUpdateDataSourceCalled(0),
				wasRunnersEvictCalled(0),
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
				wasDBFetchDataSourceCalled(1),
				wasRunnersRunnerCalled(1),
				wasExecutorTestConnectionCalled(1),
				wasDBUpdateDataSourceCalled(1),
				wasRunnersEvictCalled(1),
				hasUpdateFailedLog(),
			),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl, db, exec, vars, logs := prepHandler(c.DB, c.Clients, nil)

			req := prepRequest("POST", "http://test.com/", "", !c.OmitSession, c.ID)
			rec := httptest.NewRecorder()

			hdl.TestDataSourceConnection(rec, req)

			for _, ch := range c.Checks {
				ch(t, db, exec, vars, rec, logs)
			}
		})
	}
}

func Test_Handler_FetchDataSources(t *testing.T) {
	wasDBFetchDataSourcesCalled := func(count int) check {
		return func(t *testing.T, db *DBMock, _ *runnerMock, _ *VariableManagerMock, _ *httptest.ResponseRecorder, _ *bytes.Buffer) {
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
		"Error returned by Credentials.Configured": {
			DB: &DBMock{
				FetchDataSourcesFunc: func(_ context.Context, _ string) ([]datasourceCore.DataSource, error) {
					return []datasourceCore.DataSource{*stubDataSourceWithCredentials(`{`)}, nil
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
					return []datasourceCore.DataSource{
						*stubDataSource(datasourceCore.TypePrometheus),
						*stubDataSourceWithCredentials(`{"caCertificate":"pem"}`),
					}, nil
				},
			},
			Checks: checks(
				hasJSONResp(http.StatusOK, []dataSourceResponse{
					{DataSource: *stubDataSource(datasourceCore.TypePrometheus)},
					{
						DataSource: *stubDataSource(datasourceCore.TypePostgreSQL),
						Configured: map[string]bool{"caCertificate": true},
					},
				}),
				wasDBFetchDataSourcesCalled(1),
			),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl, db, exec, vars, logs := prepHandler(c.DB, nil, nil)

			req := prepRequest("GET", "http://test.com/", "", !c.OmitSession, "")
			rec := httptest.NewRecorder()

			hdl.FetchDataSources(rec, req)

			for _, ch := range c.Checks {
				ch(t, db, exec, vars, rec, logs)
			}
		})
	}
}

func Test_Handler_UpdateDataSource(t *testing.T) {
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
				wasRunnersRunnerCalled(0),
				wasExecutorTestConnectionCalled(0),
				wasDBUpdateDataSourceCalled(0),
				wasRunnersEvictCalled(0),
			),
		},
		"Missing data source id": {
			Body: body,
			Checks: checks(
				hasResp(http.StatusNotFound, `{"code":"general","message":"not found"}`),
				wasDBFetchDataSourceCalled(0),
				wasRunnersRunnerCalled(0),
				wasExecutorTestConnectionCalled(0),
				wasDBUpdateDataSourceCalled(0),
				wasRunnersEvictCalled(0),
			),
		},
		"Invalid JSON body": {
			Body: "{",
			ID:   _testID.String(),
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"request.invalid_json","message":"invalid JSON body"}`),
				wasDBFetchDataSourceCalled(0),
				wasRunnersRunnerCalled(0),
				wasExecutorTestConnectionCalled(0),
				wasDBUpdateDataSourceCalled(0),
				wasRunnersEvictCalled(0),
			),
		},
		"Error returned by db.FetchDataSource": {
			DB:   stubDB(nil, assert.AnError),
			Body: body,
			ID:   _testID.String(),
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasDBFetchDataSourceCalled(1),
				wasRunnersRunnerCalled(0),
				wasExecutorTestConnectionCalled(0),
				wasDBUpdateDataSourceCalled(0),
				wasRunnersEvictCalled(0),
			),
		},
		"Error returned by DataSource.ApplyUpdate": {
			DB:   stubDB(stubDataSource(datasourceCore.Type("bogus")), nil),
			Body: `{"credentials":{"username":"user"}}`,
			ID:   _testID.String(),
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasDBFetchDataSourceCalled(1),
				wasRunnersRunnerCalled(0),
				wasExecutorTestConnectionCalled(0),
				wasDBUpdateDataSourceCalled(0),
				wasRunnersEvictCalled(0),
			),
		},
		"Error returned by Runner.TestConnection": {
			DB:      stubDB(stubDataSource(datasourceCore.TypePrometheus), nil),
			Clients: &clientMocks{status: "", statusErr: assert.AnError},
			Body:    body,
			ID:      _testID.String(),
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasDBFetchDataSourceCalled(1),
				wasRunnersRunnerCalled(1),
				wasExecutorTestConnectionCalled(1),
				wasDBUpdateDataSourceCalled(0),
				wasRunnersEvictCalled(0),
			),
		},
		"Unsuccessful connection status": {
			DB:      stubDB(stubDataSource(datasourceCore.TypePrometheus), nil),
			Clients: &clientMocks{status: processor.ConnectionStatusUnreachable, statusErr: nil},
			Body:    body,
			ID:      _testID.String(),
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"data_source.unreachable","message":"The data source is unreachable."}`),
				wasDBFetchDataSourceCalled(1),
				wasRunnersRunnerCalled(1),
				wasExecutorTestConnectionCalled(1),
				wasDBUpdateDataSourceCalled(0),
				wasRunnersEvictCalled(0),
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
				wasDBFetchDataSourceCalled(1),
				wasRunnersRunnerCalled(1),
				wasExecutorTestConnectionCalled(1),
				wasDBUpdateDataSourceCalled(1),
				wasRunnersEvictCalled(0),
			),
		},
		"Error returned by Credentials.Configured": {
			DB:      stubDB(stubDataSourceWithCredentials(`[1]`), nil),
			Clients: &clientMocks{status: processor.ConnectionStatusSuccess, statusErr: nil},
			Body:    body,
			ID:      _testID.String(),
			Checks: checks(
				hasResp(http.StatusInternalServerError, `{"code":"general","message":"internal server error"}`),
				wasDBFetchDataSourceCalled(1),
				wasRunnersRunnerCalled(1),
				wasExecutorTestConnectionCalled(1),
				wasDBUpdateDataSourceCalled(1),
				wasRunnersEvictCalled(1),
			),
		},
		"Successful update": {
			DB:      stubDB(stubDataSource(datasourceCore.TypePrometheus), nil),
			Clients: &clientMocks{status: processor.ConnectionStatusSuccess, statusErr: nil},
			Body:    body,
			ID:      _testID.String(),
			Checks: checks(
				func(t *testing.T, db *DBMock, _ *runnerMock, _ *VariableManagerMock, rec *httptest.ResponseRecorder, _ *bytes.Buffer) {
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
				},
				wasDBFetchDataSourceCalled(1),
				wasRunnersRunnerCalled(1),
				wasExecutorTestConnectionCalled(1),
				wasRunnersEvictCalled(1),
			),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl, db, exec, vars, logs := prepHandler(c.DB, c.Clients, nil)

			req := prepRequest("PUT", "http://test.com/", c.Body, !c.OmitSession, c.ID)
			rec := httptest.NewRecorder()

			hdl.UpdateDataSource(rec, req)

			for _, ch := range c.Checks {
				ch(t, db, exec, vars, rec, logs)
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
				wasRunnersEvictCalled(0),
			),
		},
		"Missing data source id": {
			Checks: checks(
				hasResp(http.StatusNotFound, `{"code":"general","message":"not found"}`),
				wasDBDeleteDataSourceCalled(0),
				wasRunnersEvictCalled(0),
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
				wasRunnersEvictCalled(0),
			),
		},
		"Successful deletion": {
			ID: _testID.String(),
			Checks: checks(
				hasResp(http.StatusNoContent, ""),
				wasDBDeleteDataSourceCalled(1),
				wasRunnersEvictCalled(1),
			),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl, db, exec, vars, logs := prepHandler(c.DB, nil, nil)

			req := prepRequest("DELETE", "http://test.com/", "", !c.OmitSession, c.ID)
			rec := httptest.NewRecorder()

			hdl.DeleteDataSource(rec, req)

			for _, ch := range c.Checks {
				ch(t, db, exec, vars, rec, logs)
			}
		})
	}
}

func Test_Handler_substitute(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Variables *VariableManagerMock
		Target    string
		Texts     []string
		Result    []string
		Err       error
		Checks    []check
	}{
		"No reference skips the tree": {
			Texts:  []string{"up", "$__from"},
			Result: []string{"up", "$__from"},
			Checks: checks(wasVariableManagerFetchTreeCalled(0)),
		},
		"Error returned by VariableManager.FetchTree": {
			Variables: stubVariables(assert.AnError),
			Texts:     []string{"$metric"},
			Err:       assert.AnError,
			Checks:    checks(wasVariableManagerFetchTreeCalled(1)),
		},
		"Successful substitution": {
			Variables: stubVariables(nil),
			Texts:     []string{"sum($metric)", "up"},
			Result:    []string{"sum(up)", "up"},
			Checks:    checks(wasVariableManagerFetchTreeCalled(1)),
		},
		"Successful substitution with an override": {
			Variables: stubVariables(nil),
			Target:    "http://test.com/?var-metric=down&var-other=x&q=$metric",
			Texts:     []string{"sum($metric)", "$other"},
			Result:    []string{"sum(down)", "$other"},
			Checks:    checks(wasVariableManagerFetchTreeCalled(1)),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl, db, exec, vars, logs := prepHandler(nil, nil, c.Variables)

			target := c.Target
			if target == "" {
				target = "http://test.com/"
			}

			res, err := hdl.substitute(httptest.NewRequest(http.MethodGet, target, http.NoBody), _testSession, c.Texts...)

			for _, ch := range c.Checks {
				ch(t, db, exec, vars, httptest.NewRecorder(), logs)
			}

			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_newDataSourceResponse(t *testing.T) {
	cc := map[string]struct {
		DataSource *datasourceCore.DataSource
		Result     dataSourceResponse
		Err        error
	}{
		"Error returned by Credentials.Configured": {
			DataSource: stubDataSourceWithCredentials(`[1]`),
			Err:        assert.AnError,
		},
		"No credentials": {
			DataSource: stubDataSource(datasourceCore.TypePrometheus),
			Result: dataSourceResponse{
				DataSource: *stubDataSource(datasourceCore.TypePrometheus),
				Configured: map[string]bool{},
			},
		},
		"Some credentials set": {
			DataSource: stubDataSourceWithCredentials(`{"username":"user","caCertificate":"pem"}`),
			Result: dataSourceResponse{
				DataSource: *stubDataSourceWithCredentials(`{"username":"user","caCertificate":"pem"}`),
				Configured: map[string]bool{"username": true, "caCertificate": true},
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := newDataSourceResponse(c.DataSource)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)
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

			hdl, db, exec, vars, logs := prepHandler(c.DB, nil, nil)
			ds := stubDataSource(datasourceCore.TypePrometheus)

			hdl.persistDataSourceStatus(prepRequest("GET", "http://test.com/", "", true, ""), ds, c.Status)

			assert.Equal(t, c.Result, ds.Status)

			for _, ch := range c.Checks {
				ch(t, db, exec, vars, httptest.NewRecorder(), logs)
			}
		})
	}
}
