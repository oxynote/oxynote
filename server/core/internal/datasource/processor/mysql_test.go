package processor

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/go-sql-driver/mysql"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_MySQLQueryResult_Transform(t *testing.T) {
	cc := map[string]struct {
		Result    MySQLQueryResult
		ChartType ChartType
		Expected  QueryResult
	}{
		"Empty chart type returns type-not-selected": {
			Result: MySQLQueryResult{
				Columns: []string{"time", "value"},
				Rows:    [][]any{{int64(1700000000), float64(42)}},
			},
			ChartType: "",
			Expected:  QueryResult{Status: QueryStatusTypeNotSelected},
		},
		"No rows returns no-data": {
			Result: MySQLQueryResult{
				Columns: []string{"time", "value"},
				Rows:    nil,
			},
			ChartType: ChartTypeLine,
			Expected:  QueryResult{Status: QueryStatusNoData},
		},
		"No columns returns no-data": {
			Result: MySQLQueryResult{
				Columns: nil,
				Rows:    [][]any{{int64(1700000000), float64(42)}},
			},
			ChartType: ChartTypeLine,
			Expected:  QueryResult{Status: QueryStatusNoData},
		},
		"No time column returns chart-and-data-mismatch": {
			Result: MySQLQueryResult{
				Columns: []string{"name", "value"},
				Rows:    [][]any{{"foo", float64(42)}},
			},
			ChartType: ChartTypeLine,
			Expected:  QueryResult{Status: QueryStatusChartAndDataMismatch},
		},
		"No numeric column returns chart-and-data-mismatch": {
			Result: MySQLQueryResult{
				Columns: []string{"time", "name"},
				Rows:    [][]any{{int64(1700000000), "foo"}},
			},
			ChartType: ChartTypeLine,
			Expected:  QueryResult{Status: QueryStatusChartAndDataMismatch},
		},
		"Single value column with int64 timestamps": {
			Result: MySQLQueryResult{
				Columns: []string{"time", "value"},
				Rows: [][]any{
					{int64(1700000000), float64(10)},
					{int64(1700000060), float64(20)},
					{int64(1700000120), float64(30)},
				},
			},
			ChartType: ChartTypeLine,
			Expected: QueryResult{
				Status: QueryStatusOK,
				Data: []QueryResultSeries{
					{
						Labels: map[string]string{},
						Metrics: [][2]any{
							{int64(1700000000), float64(10)},
							{int64(1700000060), float64(20)},
							{int64(1700000120), float64(30)},
						},
					},
				},
			},
		},
		"Single value column with float64 timestamps": {
			Result: MySQLQueryResult{
				Columns: []string{"time", "value"},
				Rows: [][]any{
					{float64(1700000000), float64(10)},
					{float64(1700000060), float64(20)},
				},
			},
			ChartType: ChartTypeLine,
			Expected: QueryResult{
				Status: QueryStatusOK,
				Data: []QueryResultSeries{
					{
						Labels: map[string]string{},
						Metrics: [][2]any{
							{int64(1700000000), float64(10)},
							{int64(1700000060), float64(20)},
						},
					},
				},
			},
		},
		"Single value column with []byte numeric DECIMAL values": {
			Result: MySQLQueryResult{
				Columns: []string{"time", "value"},
				Rows: [][]any{
					{int64(1700000000), []byte("10.5")},
					{int64(1700000060), []byte("20.3")},
				},
			},
			ChartType: ChartTypeLine,
			Expected: QueryResult{
				Status: QueryStatusOK,
				Data: []QueryResultSeries{
					{
						Labels: map[string]string{},
						Metrics: [][2]any{
							{int64(1700000000), 10.5},
							{int64(1700000060), 20.3},
						},
					},
				},
			},
		},
		// MySQL types a $__timeGroupAlias bucket as DECIMAL, so the time
		// arrives as a numeric string.
		"Time column as DECIMAL string": {
			Result: MySQLQueryResult{
				Columns:     []string{"time", "value"},
				ColumnTypes: []string{"DECIMAL", "BIGINT"},
				Rows: [][]any{
					{"1700000000", int64(10)},
					{"1700003600", int64(20)},
				},
			},
			ChartType: ChartTypeLine,
			Expected: QueryResult{
				Status: QueryStatusOK,
				Data: []QueryResultSeries{
					{
						Labels: map[string]string{},
						Metrics: [][2]any{
							{int64(1700000000), float64(10)},
							{int64(1700003600), float64(20)},
						},
					},
				},
			},
		},
		"Single value column with labels": {
			Result: MySQLQueryResult{
				Columns: []string{"time", "host", "value"},
				Rows: [][]any{
					{int64(1700000000), "host-a", float64(10)},
					{int64(1700000000), "host-b", float64(20)},
					{int64(1700000060), "host-a", float64(15)},
					{int64(1700000060), "host-b", float64(25)},
				},
			},
			ChartType: ChartTypeLine,
			Expected: QueryResult{
				Status: QueryStatusOK,
				Data: []QueryResultSeries{
					{
						Labels: map[string]string{"host": "host-a"},
						Metrics: [][2]any{
							{int64(1700000000), float64(10)},
							{int64(1700000060), float64(15)},
						},
					},
					{
						Labels: map[string]string{"host": "host-b"},
						Metrics: [][2]any{
							{int64(1700000000), float64(20)},
							{int64(1700000060), float64(25)},
						},
					},
				},
			},
		},
		"Multiple value columns without labels": {
			Result: MySQLQueryResult{
				Columns: []string{"time", "cpu", "memory"},
				Rows: [][]any{
					{int64(1700000000), float64(0.41), float64(0.67)},
					{int64(1700000060), float64(0.43), float64(0.65)},
				},
			},
			ChartType: ChartTypeLine,
			Expected: QueryResult{
				Status: QueryStatusOK,
				Data: []QueryResultSeries{
					{
						Labels: map[string]string{"__name__": "cpu"},
						Metrics: [][2]any{
							{int64(1700000000), 0.41},
							{int64(1700000060), 0.43},
						},
					},
					{
						Labels: map[string]string{"__name__": "memory"},
						Metrics: [][2]any{
							{int64(1700000000), 0.67},
							{int64(1700000060), 0.65},
						},
					},
				},
			},
		},
		"Multiple value columns with labels": {
			Result: MySQLQueryResult{
				Columns: []string{"time", "cpu", "memory", "hostname"},
				Rows: [][]any{
					{int64(1700000000), float64(0.41), float64(0.67), "web-1"},
					{int64(1700000060), float64(0.43), float64(0.65), "web-1"},
					{int64(1700000000), float64(0.55), float64(0.71), "web-2"},
					{int64(1700000060), float64(0.51), float64(0.70), "web-2"},
				},
			},
			ChartType: ChartTypeLine,
			Expected: QueryResult{
				Status: QueryStatusOK,
				Data: []QueryResultSeries{
					{
						Labels:  map[string]string{"__name__": "cpu", "hostname": "web-1"},
						Metrics: [][2]any{{int64(1700000000), 0.41}, {int64(1700000060), 0.43}},
					},
					{
						Labels:  map[string]string{"__name__": "memory", "hostname": "web-1"},
						Metrics: [][2]any{{int64(1700000000), 0.67}, {int64(1700000060), 0.65}},
					},
					{
						Labels:  map[string]string{"__name__": "cpu", "hostname": "web-2"},
						Metrics: [][2]any{{int64(1700000000), 0.55}, {int64(1700000060), 0.51}},
					},
					{
						Labels:  map[string]string{"__name__": "memory", "hostname": "web-2"},
						Metrics: [][2]any{{int64(1700000000), 0.71}, {int64(1700000060), 0.70}},
					},
				},
			},
		},
		"Gauge keeps only last value per series": {
			Result: MySQLQueryResult{
				Columns: []string{"time", "cpu", "memory"},
				Rows: [][]any{
					{int64(1700000000), float64(0.41), float64(0.67)},
					{int64(1700000060), float64(0.43), float64(0.65)},
				},
			},
			ChartType: ChartTypeGauge,
			Expected: QueryResult{
				Status: QueryStatusOK,
				Data: []QueryResultSeries{
					{
						Labels:  map[string]string{"__name__": "cpu"},
						Metrics: [][2]any{{int64(1700000060), 0.43}},
					},
					{
						Labels:  map[string]string{"__name__": "memory"},
						Metrics: [][2]any{{int64(1700000060), 0.65}},
					},
				},
			},
		},
		"Single value column does not add __name__ label": {
			Result: MySQLQueryResult{
				Columns: []string{"time", "count"},
				Rows:    [][]any{{int64(1700000000), float64(42)}},
			},
			ChartType: ChartTypeBar,
			Expected: QueryResult{
				Status: QueryStatusOK,
				Data: []QueryResultSeries{
					{
						Labels:  map[string]string{},
						Metrics: [][2]any{{int64(1700000000), float64(42)}},
					},
				},
			},
		},
		"Skips rows with invalid values": {
			Result: MySQLQueryResult{
				Columns: []string{"time", "value"},
				Rows: [][]any{
					{int64(1700000000), float64(10)},
					{int64(1700000060), "not-a-number"},
					{int64(1700000120), float64(30)},
				},
			},
			ChartType: ChartTypeLine,
			Expected: QueryResult{
				Status: QueryStatusOK,
				Data: []QueryResultSeries{
					{
						Labels: map[string]string{},
						Metrics: [][2]any{
							{int64(1700000000), float64(10)},
							{int64(1700000120), float64(30)},
						},
					},
				},
			},
		},
		"All invalid timestamps returns no-data": {
			Result: MySQLQueryResult{
				Columns: []string{"time", "value"},
				Rows: [][]any{
					{"bad-time", float64(10)},
					{"bad-time", float64(20)},
				},
			},
			ChartType: ChartTypeLine,
			Expected:  QueryResult{Status: QueryStatusNoData},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			result := c.Result.Transform(c.ChartType)
			require.Equal(t, c.Expected.Status, result.Status)
			assert.Equal(t, c.Expected.Data, result.Data)
		})
	}
}

func Test_MySQLQueryResult_HasData(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Result *MySQLQueryResult
		Has    bool
	}{
		"No result": {},
		"No rows":   {Result: &MySQLQueryResult{Columns: []string{"value"}}},
		"A row":     {Result: &MySQLQueryResult{Columns: []string{"value"}, Rows: [][]any{{int64(1)}}}, Has: true},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Has, c.Result.HasData())
		})
	}
}

func Test_NewMySQL(t *testing.T) {
	t.Parallel()

	inp := &InputMock{}

	m := NewMySQL(inp)
	require.NotNil(t, m)
	assert.Same(t, inp, m.inp)
	assert.Nil(t, m.db)
	assert.False(t, m.closed)
}

func Test_MySQL_Close(t *testing.T) {
	cc := map[string]struct {
		// Used indicates whether a query opened the pool before the close.
		Used bool
	}{
		"Pool never opened": {},
		"Pool in use":       {Used: true},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			m := NewMySQL(&InputMock{
				URLFunc: func() string {
					return fmt.Sprintf("mysql://%s:%s@%s/%s", _mysqlRootUser, _mysqlRootPass, _mysqlAddr, _testDB)
				},
			})

			if c.Used {
				_, err := m.Query(context.Background(), "SELECT 1", _testTimeRange)
				require.NoError(t, err)
			}

			require.NoError(t, m.Close())
			assert.True(t, m.closed)

			// a closed processor hands out no connection, and opens no new
			// pool that nothing would close.
			_, err := m.connect()
			testutil.AssertEqualError(t, _errClosed, err)

			if c.Used {
				require.NotNil(t, m.db)
				assert.Error(t, m.db.PingContext(context.Background()))
			}
		})
	}
}

func Test_MySQL_TestConnection(t *testing.T) {
	cc := map[string]struct {
		URL              string
		Creds            Credentials
		Result           ConnectionStatus
		CredentialsCalls int
	}{
		"Error returned by connect": {
			URL:    "://",
			Result: ConnectionStatusUnreachable,
		},
		"Unreachable database": {
			URL:              "mysql://127.0.0.1:1/db",
			Result:           ConnectionStatusUnreachable,
			CredentialsCalls: 1,
		},
		// a refused password is not the same as a host that never answers.
		"Wrong credentials": {
			URL:              fmt.Sprintf("mysql://%s:%s@%s/%s", _readerUser, "wrong-password", _mysqlAddr, _testDB),
			Result:           ConnectionStatusUnauthorized,
			CredentialsCalls: 1,
		},
		"Not read-only user": {
			URL:              fmt.Sprintf("mysql://%s:%s@%s/%s", _mysqlRootUser, _mysqlRootPass, _mysqlAddr, _testDB),
			Result:           ConnectionStatusNotReadOnly,
			CredentialsCalls: 1,
		},
		"Successful read-only connection": {
			URL:              fmt.Sprintf("mysql://%s:%s@%s/%s", _readerUser, _readerPass, _mysqlAddr, _testDB),
			Result:           ConnectionStatusSuccess,
			CredentialsCalls: 1,
		},
		"Invalid CA certificate": {
			URL:              fmt.Sprintf("mysql://%s:%s@%s/%s", _readerUser, _readerPass, _mysqlAddr, _testDB),
			Creds:            caCredentials("not a certificate"),
			Result:           ConnectionStatusInvalidCACertificate,
			CredentialsCalls: 1,
		},
		"Server the CA certificate did not sign": {
			URL:              fmt.Sprintf("mysql://%s:%s@%s/%s", _readerUser, _readerPass, _mysqlAddr, _testDB),
			Creds:            caCredentials(_testCA),
			Result:           ConnectionStatusUnreachable,
			CredentialsCalls: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			inp := &InputMock{
				URLFunc:         func() string { return c.URL },
				CredentialsFunc: func() Credentials { return c.Creds },
			}

			m := NewMySQL(inp)

			t.Cleanup(func() { assert.NoError(t, m.Close()) })

			cs, err := m.TestConnection(context.Background())
			require.NoError(t, err)
			assert.Equal(t, c.Result, cs)
			assert.Len(t, inp.URLCalls(), 1)
			assert.Len(t, inp.CredentialsCalls(), c.CredentialsCalls)
		})
	}
}

func Test_MySQL_Metadata(t *testing.T) {
	cc := map[string]struct {
		URL              string
		Result           *SQLMetadataResult
		Err              error
		CredentialsCalls int
	}{
		"Error returned by connect": {
			URL: "://",
			Err: assert.AnError,
		},
		"Unreachable database": {
			URL:              "mysql://127.0.0.1:1/db",
			Err:              assert.AnError,
			CredentialsCalls: 1,
		},
		"Successful retrieval": {
			URL: fmt.Sprintf("mysql://%s:%s@%s/%s", _mysqlRootUser, _mysqlRootPass, _mysqlAddr, _testDB),
			Result: &SQLMetadataResult{
				Tables: map[string]SQLTable{
					"testdb.metrics": {
						Columns: []SQLColumn{
							{Name: "time"},
							{Name: "host"},
							{Name: "value"},
						},
					},
					"testdb.typed_metrics": {
						Columns: []SQLColumn{
							{Name: "time"},
							{Name: "code"},
							{Name: "total"},
						},
					},
				},
				DefaultSchema: "testdb",
			},
			CredentialsCalls: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			inp := &InputMock{
				URLFunc: func() string { return c.URL },
			}

			m := NewMySQL(inp)

			t.Cleanup(func() { assert.NoError(t, m.Close()) })

			result, err := m.Metadata(context.Background())
			testutil.AssertEqualError(t, c.Err, err)
			assert.Len(t, inp.URLCalls(), 1)
			assert.Len(t, inp.CredentialsCalls(), c.CredentialsCalls)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, result)
		})
	}
}

func Test_MySQL_QueryLabels(t *testing.T) {
	cc := map[string]struct {
		URL              string
		Query            string
		Result           map[string]string
		ErrStatus        int
		Err              error
		CredentialsCalls int
	}{
		"Error returned by connect": {
			URL:   "://",
			Query: "SELECT 1",
			Err:   assert.AnError,
		},
		"Unreachable database": {
			URL:              "mysql://127.0.0.1:1/db",
			Query:            "SELECT 1",
			Err:              assert.AnError,
			CredentialsCalls: 1,
		},
		"Invalid query error": {
			URL:              fmt.Sprintf("mysql://%s:%s@%s/%s", _mysqlRootUser, _mysqlRootPass, _mysqlAddr, _testDB),
			Query:            "SELECT bogus FROM",
			ErrStatus:        http.StatusBadRequest,
			Err:              assert.AnError,
			CredentialsCalls: 1,
		},
		"Successful retrieval": {
			URL:   fmt.Sprintf("mysql://%s:%s@%s/%s", _mysqlRootUser, _mysqlRootPass, _mysqlAddr, _testDB),
			Query: "SELECT time, host, value FROM metrics ORDER BY time",
			// only the text column produces a label; the numeric
			// columns come back typed and are skipped.
			Result:           map[string]string{"host": "web-1"},
			CredentialsCalls: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			inp := &InputMock{
				URLFunc: func() string { return c.URL },
			}

			m := NewMySQL(inp)

			t.Cleanup(func() { assert.NoError(t, m.Close()) })

			labels, err := m.QueryLabels(context.Background(), c.Query, _testTimeRange)
			testutil.AssertEqualError(t, c.Err, err)
			assert.Len(t, inp.URLCalls(), 1)
			assert.Len(t, inp.CredentialsCalls(), c.CredentialsCalls)

			if c.ErrStatus != 0 {
				assert.Equal(t, c.ErrStatus, errutil.StatusCode(err, false))
			}

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, labels)
		})
	}
}

func Test_MySQL_Query(t *testing.T) {
	rootURL := fmt.Sprintf("mysql://%s:%s@%s/%s", _mysqlRootUser, _mysqlRootPass, _mysqlAddr, _testDB)

	// connectionID reads the id of the connection a query runs on.
	connectionID := func(t *testing.T, m *MySQL) any {
		t.Helper()

		res, err := m.Query(context.Background(), "SELECT CONNECTION_ID()", _testTimeRange)
		require.NoError(t, err)
		require.Len(t, res.Rows, 1)

		return res.Rows[0][0]
	}

	cc := map[string]struct {
		URL string

		Query            string
		Result           *MySQLQueryResult
		ErrStatus        int
		Err              error
		CredentialsCalls int
		Check            func(t *testing.T, m *MySQL)
	}{
		"Error returned by connect": {
			URL:   "://",
			Query: "SELECT 1",
			Err:   assert.AnError,
		},
		"Unreachable database": {
			URL:              "mysql://127.0.0.1:1/db",
			Query:            "SELECT 1",
			Err:              assert.AnError,
			CredentialsCalls: 1,
		},
		"Invalid query error": {
			URL:              fmt.Sprintf("mysql://%s:%s@%s/%s", _mysqlRootUser, _mysqlRootPass, _mysqlAddr, _testDB),
			Query:            "SELECT bogus FROM",
			ErrStatus:        http.StatusBadRequest,
			Err:              assert.AnError,
			CredentialsCalls: 1,
		},
		// a DECIMAL arrives as bytes exactly like a VARCHAR does, so only
		// the declared type separates the value column from the label.
		"Decimal column keeps its declared type": {
			URL:   fmt.Sprintf("mysql://%s:%s@%s/%s", _mysqlRootUser, _mysqlRootPass, _mysqlAddr, _testDB),
			Query: "SELECT time, code, total FROM typed_metrics",
			Result: &MySQLQueryResult{
				Columns:     []string{"time", "code", "total"},
				ColumnTypes: []string{"BIGINT", "VARCHAR", "DECIMAL"},
				Rows:        [][]any{{float64(1700000000), "200", "10.50"}},
			},
			CredentialsCalls: 1,
		},
		// rows come back in the shape JSON gives them, so a cached copy
		// draws the same chart as the fresh one.
		"FLOAT, unsigned BIGINT and DATE columns": {
			URL:   fmt.Sprintf("mysql://%s:%s@%s/%s", _mysqlRootUser, _mysqlRootPass, _mysqlAddr, _testDB),
			Query: "SELECT CAST(1.5 AS FLOAT) AS ratio, CAST(9223372036854775808 AS UNSIGNED) AS hits, DATE('2024-01-01') AS day",
			Result: &MySQLQueryResult{
				Columns:     []string{"ratio", "hits", "day"},
				ColumnTypes: []string{"FLOAT", "UNSIGNED BIGINT", "DATE"},
				Rows:        [][]any{{1.5, float64(9223372036854775808), "2024-01-01T00:00:00Z"}},
			},
			CredentialsCalls: 1,
		},
		"Successful query": {
			URL:   fmt.Sprintf("mysql://%s:%s@%s/%s", _mysqlRootUser, _mysqlRootPass, _mysqlAddr, _testDB),
			Query: "SELECT time, host, value FROM metrics ORDER BY time",
			Result: &MySQLQueryResult{
				Columns:     []string{"time", "host", "value"},
				ColumnTypes: []string{"BIGINT", "VARCHAR", "DOUBLE"},
				Rows: [][]any{
					{float64(1700000000), "web-1", 10.5},
					{float64(1700000060), "web-2", 20.5},
				},
			},
			CredentialsCalls: 1,
		},
		"A plain read keeps its connection": {
			URL:   rootURL,
			Query: "SELECT 1 AS one",
			Result: &MySQLQueryResult{
				Columns:     []string{"one"},
				ColumnTypes: []string{"INT"},
				Rows:        [][]any{{float64(1)}},
			},
			CredentialsCalls: 1,
			Check: func(t *testing.T, m *MySQL) {
				t.Helper()

				first := connectionID(t, m)
				assert.Equal(t, first, connectionID(t, m))
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			inp := &InputMock{
				URLFunc: func() string { return c.URL },
			}

			m := NewMySQL(inp)

			t.Cleanup(func() { assert.NoError(t, m.Close()) })

			// with all but one connection held until the test ends, every
			// query lands on the one left.
			if c.Check != nil {
				db, derr := m.connect()
				require.NoError(t, derr)

				for range _mysqlMaxConns - 1 {
					held, cerr := db.Conn(context.Background())
					require.NoError(t, cerr)

					t.Cleanup(func() { assert.NoError(t, held.Close()) })
				}
			}

			result, err := m.Query(context.Background(), c.Query, _testTimeRange)
			testutil.AssertEqualError(t, c.Err, err)
			assert.Len(t, inp.URLCalls(), 1)
			assert.Len(t, inp.CredentialsCalls(), c.CredentialsCalls)

			if c.ErrStatus != 0 {
				assert.Equal(t, c.ErrStatus, errutil.StatusCode(err, false))
			}

			if c.Check != nil {
				c.Check(t, m)
			}

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, result)
		})
	}
}

func Test_MySQL_connect(t *testing.T) {
	cc := map[string]struct {
		URL              string
		Closed           bool
		Err              error
		CredentialsCalls int
	}{
		"Error returned by buildConfig": {
			URL: "://",
			Err: assert.AnError,
		},
		"Error returned by mysql.NewConnector": {
			URL:              "mysql://127.0.0.1:1/db?tls=bogus",
			Err:              assert.AnError,
			CredentialsCalls: 1,
		},
		"Closed processor": {
			URL:    "mysql://127.0.0.1:1/db",
			Closed: true,
			Err:    _errClosed,
		},
		"Successful pool": {
			URL:              fmt.Sprintf("mysql://%s:%s@%s/%s", _mysqlRootUser, _mysqlRootPass, _mysqlAddr, _testDB),
			CredentialsCalls: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			inp := &InputMock{
				URLFunc: func() string { return c.URL },
			}

			m := NewMySQL(inp)

			t.Cleanup(func() { assert.NoError(t, m.Close()) })

			if c.Closed {
				require.NoError(t, m.Close())
			}

			db, err := m.connect()
			testutil.AssertEqualError(t, c.Err, err)
			assert.Len(t, inp.CredentialsCalls(), c.CredentialsCalls)

			if err != nil {
				assert.Nil(t, m.db)
				return
			}

			require.NotNil(t, db)
			assert.Equal(t, _mysqlMaxConns, db.Stats().MaxOpenConnections)

			// the pool is opened once and handed to every later call.
			again, err := m.connect()
			require.NoError(t, err)
			assert.Same(t, db, again)
			assert.Len(t, inp.URLCalls(), 1)
		})
	}
}

func Test_MySQL_buildConfig(t *testing.T) {
	cc := map[string]struct {
		URL              string
		Creds            Credentials
		User             string
		Password         string
		Addr             string
		DBName           string
		TLS              string
		RootCAs          bool
		Err              error
		CredentialsCalls int
	}{
		"Error returned by url.Parse": {
			URL: "://",
			Err: assert.AnError,
		},
		"Error returned by Credentials.basic": {
			URL:              "mysql://dbhost/mydb",
			Creds:            NewCredentials([]byte(`{`)),
			Err:              assert.AnError,
			CredentialsCalls: 1,
		},
		"Error returned by BasicCredentials.rootCAs": {
			URL:              "mysql://dbhost/mydb",
			Creds:            caCredentials("not a certificate"),
			Err:              _errInvalidCACertificate,
			CredentialsCalls: 1,
		},
		"URL credentials with default port": { //nolint:gosec // static test credentials
			URL:              "mysql://urluser:urlpass@dbhost/mydb",
			User:             "urluser",
			Password:         "urlpass",
			Addr:             "dbhost:3306",
			DBName:           "mydb",
			CredentialsCalls: 1,
		},
		"Credentials override url user": { //nolint:gosec // static test credentials
			URL:              "mysql://urluser:urlpass@dbhost:3307/mydb",
			Creds:            NewCredentials([]byte(`{"username":"credsuser","password":"credspass"}`)),
			User:             "credsuser",
			Password:         "credspass",
			Addr:             "dbhost:3307",
			DBName:           "mydb",
			CredentialsCalls: 1,
		},
		"Password keeps the url user": {
			URL:              "mysql://urluser@dbhost:3307/mydb",
			Creds:            NewCredentials([]byte(`{"password":"credspass"}`)),
			User:             "urluser",
			Password:         "credspass",
			Addr:             "dbhost:3307",
			DBName:           "mydb",
			CredentialsCalls: 1,
		},
		"Empty credentials keep url user": { //nolint:gosec // static test credentials
			URL:              "mysql://urluser:urlpass@dbhost:3307/mydb",
			Creds:            NewCredentials([]byte(`{"username":"","password":""}`)),
			User:             "urluser",
			Password:         "urlpass",
			Addr:             "dbhost:3307",
			DBName:           "mydb",
			CredentialsCalls: 1,
		},
		"TLS parameter is carried over": {
			URL:              "mysql://dbhost/mydb?tls=true",
			Addr:             "dbhost:3306",
			DBName:           "mydb",
			TLS:              "true",
			CredentialsCalls: 1,
		},
		// a TLS parameter could skip the certificate the CA is there to
		// check.
		"CA certificate replaces the TLS parameter": {
			URL:              "mysql://dbhost/mydb?tls=skip-verify",
			Creds:            caCredentials(_testCA),
			Addr:             "dbhost:3306",
			DBName:           "mydb",
			RootCAs:          true,
			CredentialsCalls: 1,
		},
		"No user info": {
			URL:              "mysql://dbhost/mydb",
			Addr:             "dbhost:3306",
			DBName:           "mydb",
			CredentialsCalls: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			inp := &InputMock{
				URLFunc:         func() string { return c.URL },
				CredentialsFunc: func() Credentials { return c.Creds },
			}

			m := NewMySQL(inp)

			cfg, err := m.buildConfig()
			testutil.AssertEqualError(t, c.Err, err)
			assert.Len(t, inp.URLCalls(), 1)
			assert.Len(t, inp.CredentialsCalls(), c.CredentialsCalls)

			if err != nil {
				return
			}

			assert.Equal(t, c.User, cfg.User)
			assert.Equal(t, c.Password, cfg.Passwd)
			assert.Equal(t, c.Addr, cfg.Addr)
			assert.Equal(t, c.DBName, cfg.DBName)
			assert.True(t, cfg.ParseTime)
			assert.Equal(t, 10*time.Second, cfg.Timeout)
			assert.Equal(t, c.TLS, cfg.TLSConfig)
			assert.Equal(t, c.RootCAs, cfg.TLS != nil && cfg.TLS.RootCAs != nil)
		})
	}
}

func Test_mysqlCheckReadOnly(t *testing.T) {
	cc := map[string]struct {
		Rows     *sqlmock.Rows
		QueryErr error
		Result   bool
		Err      error
	}{
		"Error returned by db.QueryContext": {
			QueryErr: assert.AnError,
			Err:      assert.AnError,
		},
		"Error returned by rows.Scan": {
			Rows: sqlmock.NewRows([]string{"grants"}).AddRow(nil),
			Err:  assert.AnError,
		},
		"Error returned by rows.Err": {
			Rows: sqlmock.NewRows([]string{"grants"}).
				AddRow("GRANT SELECT ON *.* TO 'u'@'%'").
				AddRow("GRANT SELECT ON *.* TO 'u'@'%'").
				RowError(1, assert.AnError),
			Err: assert.AnError,
		},
		"All privileges grant": {
			Rows: sqlmock.NewRows([]string{"grants"}).
				AddRow("GRANT ALL PRIVILEGES ON *.* TO 'u'@'%'"),
		},
		"Write privilege": {
			Rows: sqlmock.NewRows([]string{"grants"}).
				AddRow("GRANT SELECT, INSERT ON *.* TO 'u'@'%'"),
		},
		"Role grant hides its privileges": {
			Rows: sqlmock.NewRows([]string{"grants"}).
				AddRow("GRANT SELECT ON *.* TO `u`@`%`").
				AddRow("GRANT `writer_role`@`%` TO `u`@`%`"),
		},
		"Read-only privileges": {
			Rows: sqlmock.NewRows([]string{"grants"}).
				AddRow("GRANT SELECT, USAGE ON *.* TO 'u'@'%'"),
			Result: true,
		},
		"Read-only privileges with empty entry": {
			Rows: sqlmock.NewRows([]string{"grants"}).
				AddRow("GRANT SELECT,, USAGE ON *.* TO 'u'@'%'"),
			Result: true,
		},
		"Column-level read-only grant": {
			Rows: sqlmock.NewRows([]string{"grants"}).
				AddRow("GRANT SELECT (a, b), SHOW VIEW ON `db`.`t` TO 'u'@'%'"),
			Result: true,
		},
		"Column-level write grant": {
			Rows: sqlmock.NewRows([]string{"grants"}).
				AddRow("GRANT SELECT (a, b), UPDATE (c) ON `db`.`t` TO 'u'@'%'"),
		},
		"Malformed grant is skipped": {
			Rows: sqlmock.NewRows([]string{"grants"}).
				AddRow("SOMETHING WEIRD"),
			Result: true,
		},
		"No grants": {
			Rows:   sqlmock.NewRows([]string{"grants"}),
			Result: true,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
			require.NoError(t, err)
			t.Cleanup(func() { db.Close() }) //nolint:errcheck,gosec // error provides no meaningful info

			exp := mock.ExpectQuery("SHOW GRANTS FOR CURRENT_USER()")

			if c.QueryErr != nil {
				exp.WillReturnError(c.QueryErr)
			} else {
				exp.WillReturnRows(c.Rows)
			}

			readOnly, err := mysqlCheckReadOnly(context.Background(), db)
			testutil.AssertEqualError(t, c.Err, err)
			assert.NoError(t, mock.ExpectationsWereMet())

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, readOnly)
		})
	}
}

func Test_mysqlNormalizeValue(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Value  any
		Result any
	}{
		"Bytes become a string":           {Value: []byte("web-1"), Result: "web-1"},
		"FLOAT becomes float64":           {Value: float32(1.5), Result: 1.5},
		"BIGINT becomes float64":          {Value: int64(7), Result: float64(7)},
		"Unsigned BIGINT becomes float64": {Value: uint64(5), Result: float64(5)},
		"Date becomes an RFC3339 string": {
			Value:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			Result: "2024-01-01T00:00:00Z",
		},
		"Anything else is kept": {Value: "web-1", Result: "web-1"},
		"Null is kept":          {},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, mysqlNormalizeValue(c.Value))
		})
	}
}

func Test_mysqlParseTimestamp(t *testing.T) {
	cc := map[string]struct {
		Value  any
		Result int64
		OK     bool
	}{
		"Float64 unix seconds": {
			Value:  float64(1700000000.9),
			Result: 1700000000,
			OK:     true,
		},
		"Int64 unix seconds": {
			Value:  int64(1700000000),
			Result: 1700000000,
			OK:     true,
		},
		"RFC3339 string": {
			Value:  "2023-11-14T22:13:20Z",
			Result: 1700000000,
			OK:     true,
		},
		"RFC3339 string with fraction": {
			Value:  "2023-11-14T22:13:20.5Z",
			Result: 1700000000,
			OK:     true,
		},
		"Numeric string": {
			Value:  "1700000000",
			Result: 1700000000,
			OK:     true,
		},
		"Invalid string": {
			Value: "not-a-time",
		},
		"Numeric bytes": {
			Value:  []byte("1700000000.5"),
			Result: 1700000000,
			OK:     true,
		},
		"RFC3339 bytes": {
			Value:  []byte("2023-11-14T22:13:20Z"),
			Result: 1700000000,
			OK:     true,
		},
		"Invalid bytes": {
			Value: []byte("not-a-time"),
		},
		"Unsupported type": {
			Value: true,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			ts, ok := mysqlParseTimestamp(c.Value)
			assert.Equal(t, c.OK, ok)
			assert.Equal(t, c.Result, ts)
		})
	}
}

func Test_mysqlParseNumericValue(t *testing.T) {
	cc := map[string]struct {
		Value  any
		Result float64
		OK     bool
	}{
		"Float64 value": {
			Value:  float64(10.5),
			Result: 10.5,
			OK:     true,
		},
		"Int64 value": {
			Value:  int64(10),
			Result: 10,
			OK:     true,
		},
		"Numeric bytes": {
			Value:  []byte("10.5"),
			Result: 10.5,
			OK:     true,
		},
		"Invalid bytes": {
			Value: []byte("not-a-number"),
		},
		// Query converts every []byte to a string before Transform runs,
		// which is the form a DECIMAL value arrives in.
		"Numeric string": {
			Value:  "10.5",
			Result: 10.5,
			OK:     true,
		},
		"Non-numeric string": {
			Value: "web-01",
		},
		"Unsupported type": {
			Value: true,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			v, ok := mysqlParseNumericValue(c.Value)
			assert.Equal(t, c.OK, ok)
			assert.InDelta(t, c.Result, v, 0.0001)
		})
	}
}

func Test_mysqlQueryError(t *testing.T) {
	type tcase struct {
		Inp error
		Err error
	}

	cc := map[string]tcase{
		"Invalid-query MySQLError": {
			Inp: &mysql.MySQLError{Number: 1064, Message: "syntax error"},
			Err: NewInvalidQueryError("syntax error"),
		},
		"MySQLError outside the invalid-query range": func() tcase {
			err := &mysql.MySQLError{Number: 1045, Message: "access denied"}

			return tcase{
				Inp: err,
				Err: fmt.Errorf("error executing query: %w", err),
			}
		}(),
		"Non-mysql error": func() tcase {
			err := errors.New("dial failure")

			return tcase{
				Inp: err,
				Err: fmt.Errorf("error executing query: %w", err),
			}
		}(),
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, mysqlQueryError(c.Inp))
		})
	}
}
