package processor

import (
	"context"
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
	tests := map[string]struct {
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
		"Single value column with []byte numeric values (DECIMAL)": {
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

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			result := tc.Result.Transform(tc.ChartType)
			require.Equal(t, tc.Expected.Status, result.Status)
			assert.Equal(t, tc.Expected.Data, result.Data)
		})
	}
}

func Test_NewMySQL(t *testing.T) {
	t.Parallel()

	inp := &InputMock{}

	m := NewMySQL(inp)
	require.NotNil(t, m)
	assert.Same(t, inp, m.inp)
}

func Test_MySQL_TestConnection(t *testing.T) {
	cc := map[string]struct {
		URL    string
		Result ConnectionStatus
	}{
		"Error returned by connect": {
			URL:    "://",
			Result: ConnectionStatusUnreachable,
		},
		"Unreachable database": {
			URL:    "mysql://127.0.0.1:1/db",
			Result: ConnectionStatusUnreachable,
		},
		// a refused password is not the same as a host that never answers.
		"Wrong credentials": {
			URL:    mysqlTestURL(_readerUser, "wrong-password"),
			Result: ConnectionStatusUnauthorized,
		},
		"Not read-only user": {
			URL:    mysqlTestURL(_mysqlRootUser, _mysqlRootPass),
			Result: ConnectionStatusNotReadOnly,
		},
		"Successful read-only connection": {
			URL:    mysqlTestURL(_readerUser, _readerPass),
			Result: ConnectionStatusSuccess,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			m := NewMySQL(&InputMock{
				URLFunc: func() string { return c.URL },
			})

			cs, err := m.TestConnection(context.Background())
			require.NoError(t, err)
			assert.Equal(t, c.Result, cs)
		})
	}
}

func Test_MySQL_Metadata(t *testing.T) {
	cc := map[string]struct {
		URL    string
		Result *SQLMetadataResult
		Err    error
	}{
		"Error returned by connect": {
			URL: "://",
			Err: assert.AnError,
		},
		"Error returned by db.QueryContext": {
			URL: "mysql://127.0.0.1:1/db",
			Err: assert.AnError,
		},
		"Successful retrieval": {
			URL: mysqlTestURL(_mysqlRootUser, _mysqlRootPass),
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
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			m := NewMySQL(&InputMock{
				URLFunc: func() string { return c.URL },
			})

			result, err := m.Metadata(context.Background())
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, result)
		})
	}
}

func Test_MySQL_QueryLabels(t *testing.T) {
	cc := map[string]struct {
		URL       string
		Query     string
		Result    map[string]string
		ErrStatus int
		Err       error
	}{
		"Error returned by connect": {
			URL:   "://",
			Query: "SELECT 1",
			Err:   assert.AnError,
		},
		"Error returned by db.QueryContext": {
			URL:   "mysql://127.0.0.1:1/db",
			Query: "SELECT 1",
			Err:   assert.AnError,
		},
		"Invalid query error": {
			URL:       mysqlTestURL(_mysqlRootUser, _mysqlRootPass),
			Query:     "SELECT bogus FROM",
			ErrStatus: http.StatusBadRequest,
			Err:       assert.AnError,
		},
		"Successful retrieval": {
			URL:   mysqlTestURL(_mysqlRootUser, _mysqlRootPass),
			Query: "SELECT time, host, value FROM metrics ORDER BY time",
			// only the text column produces a label; the numeric
			// columns come back typed and are skipped.
			Result: map[string]string{"host": "web-1"},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			m := NewMySQL(&InputMock{
				URLFunc: func() string { return c.URL },
			})

			labels, err := m.QueryLabels(context.Background(), c.Query, _testTimeRange)
			testutil.AssertEqualError(t, c.Err, err)

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
	cc := map[string]struct {
		URL       string
		Query     string
		Result    *MySQLQueryResult
		ErrStatus int
		Err       error
	}{
		"Error returned by connect": {
			URL:   "://",
			Query: "SELECT 1",
			Err:   assert.AnError,
		},
		"Error returned by db.QueryContext": {
			URL:   "mysql://127.0.0.1:1/db",
			Query: "SELECT 1",
			Err:   assert.AnError,
		},
		"Invalid query error": {
			URL:       mysqlTestURL(_mysqlRootUser, _mysqlRootPass),
			Query:     "SELECT bogus FROM",
			ErrStatus: http.StatusBadRequest,
			Err:       assert.AnError,
		},
		// a DECIMAL arrives as bytes exactly like a VARCHAR does, so only
		// the declared type separates the value column from the label.
		"Decimal column keeps its declared type": {
			URL:   mysqlTestURL(_mysqlRootUser, _mysqlRootPass),
			Query: "SELECT time, code, total FROM typed_metrics",
			Result: &MySQLQueryResult{
				Columns:     []string{"time", "code", "total"},
				ColumnTypes: []string{"BIGINT", "VARCHAR", "DECIMAL"},
				Rows:        [][]any{{int64(1700000000), "200", "10.50"}},
			},
		},
		"Successful query": {
			URL:   mysqlTestURL(_mysqlRootUser, _mysqlRootPass),
			Query: "SELECT time, host, value FROM metrics ORDER BY time",
			Result: &MySQLQueryResult{
				Columns:     []string{"time", "host", "value"},
				ColumnTypes: []string{"BIGINT", "VARCHAR", "DOUBLE"},
				Rows: [][]any{
					{int64(1700000000), "web-1", 10.5},
					{int64(1700000060), "web-2", 20.5},
				},
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			m := NewMySQL(&InputMock{
				URLFunc: func() string { return c.URL },
			})

			result, err := m.Query(context.Background(), c.Query, _testTimeRange)
			testutil.AssertEqualError(t, c.Err, err)

			if c.ErrStatus != 0 {
				assert.Equal(t, c.ErrStatus, errutil.StatusCode(err, false))
			}

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, result)
		})
	}
}

func Test_MySQL_connect(t *testing.T) {
	t.Parallel()

	// error
	m := NewMySQL(&InputMock{
		URLFunc: func() string { return "://" },
	})

	_, err := m.connect()
	assert.Error(t, err)

	// success
	m = NewMySQL(&InputMock{
		URLFunc: func() string { return "mysql://localhost:3306/db" },
	})

	db, err := m.connect()
	require.NoError(t, err)
	require.NotNil(t, db)
	require.NoError(t, db.Close())
}

func Test_MySQL_buildDSN(t *testing.T) {
	cc := map[string]struct {
		URL      string
		Creds    Credentials
		User     string
		Password string
		Addr     string
		DBName   string
		TLS      string
		Err      error
	}{
		"Error returned by url.Parse": {
			URL: "://",
			Err: assert.AnError,
		},
		"Error returned by unmarshaling credentials": {
			URL:   "mysql://dbhost/mydb",
			Creds: Credentials(`{`),
			Err:   assert.AnError,
		},
		"URL credentials with default port": { //nolint:gosec // static test credentials
			URL:      "mysql://urluser:urlpass@dbhost/mydb",
			User:     "urluser",
			Password: "urlpass",
			Addr:     "dbhost:3306",
			DBName:   "mydb",
		},
		"Credentials override url user": { //nolint:gosec // static test credentials
			URL:      "mysql://urluser:urlpass@dbhost:3307/mydb",
			Creds:    Credentials(`{"username":"credsuser","password":"credspass"}`),
			User:     "credsuser",
			Password: "credspass",
			Addr:     "dbhost:3307",
			DBName:   "mydb",
		},
		"Empty credentials keep url user": { //nolint:gosec // static test credentials
			URL:      "mysql://urluser:urlpass@dbhost:3307/mydb",
			Creds:    Credentials(`{"username":"","password":""}`),
			User:     "urluser",
			Password: "urlpass",
			Addr:     "dbhost:3307",
			DBName:   "mydb",
		},
		"TLS parameter is carried over": {
			URL:    "mysql://dbhost/mydb?tls=true",
			Addr:   "dbhost:3306",
			DBName: "mydb",
			TLS:    "true",
		},
		"No user info": {
			URL:    "mysql://dbhost/mydb",
			Addr:   "dbhost:3306",
			DBName: "mydb",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			m := NewMySQL(&InputMock{
				URLFunc:         func() string { return c.URL },
				CredentialsFunc: func() Credentials { return c.Creds },
			})

			dsn, err := m.buildDSN()
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			cfg, err := mysql.ParseDSN(dsn)
			require.NoError(t, err)
			assert.Equal(t, c.User, cfg.User)
			assert.Equal(t, c.Password, cfg.Passwd)
			assert.Equal(t, c.Addr, cfg.Addr)
			assert.Equal(t, c.DBName, cfg.DBName)
			assert.True(t, cfg.ParseTime)
			assert.Equal(t, 10*time.Second, cfg.Timeout)
			assert.Equal(t, c.TLS, cfg.TLSConfig)
		})
	}
}

func Test_UpdateMySQLCredentials(t *testing.T) {
	cc := map[string]struct {
		Creds  Credentials
		Inp    CredentialsUpdateInput
		Result Credentials
		Err    error
	}{
		"Error returned by unmarshaling credentials": {
			Creds: Credentials(`{`),
			Inp:   CredentialsUpdateInput(`{"username":"user"}`),
			Err:   assert.AnError,
		},
		"Error returned by unmarshaling update input": {
			Inp: CredentialsUpdateInput(`{`),
			Err: assert.AnError,
		},
		"Updated username retains password": {
			Creds:  Credentials(`{"username":"old","password":"secret"}`),
			Inp:    CredentialsUpdateInput(`{"username":"new"}`),
			Result: Credentials(`{"username":"new","password":"secret"}`),
		},
		"Updated password retains username": {
			Creds:  Credentials(`{"username":"user","password":"old"}`),
			Inp:    CredentialsUpdateInput(`{"password":"new"}`),
			Result: Credentials(`{"username":"user","password":"new"}`),
		},
		"Cleared credentials": {
			Creds: Credentials(`{"username":"user","password":"pass"}`),
			Inp:   CredentialsUpdateInput(`{"username":"","password":""}`),
		},
		"Created credentials from scratch": {
			Inp:    CredentialsUpdateInput(`{"username":"user","password":"pass"}`),
			Result: Credentials(`{"username":"user","password":"pass"}`),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			creds, err := UpdateMySQLCredentials(c.Creds, c.Inp)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, creds)
		})
	}
}

func Test_MySQLQueryResult_identifyColumns(t *testing.T) {
	cc := map[string]struct {
		Result    MySQLQueryResult
		TimeIdx   int
		ValueIdxs []int
		LabelIdxs []int
	}{
		"No rows": {
			Result: MySQLQueryResult{
				Columns: []string{"time", "value"},
			},
			TimeIdx: 0,
		},
		"No time column": {
			Result: MySQLQueryResult{
				Columns: []string{"value", "host"},
				Rows:    [][]any{{float64(1), "a"}},
			},
			TimeIdx:   -1,
			ValueIdxs: []int{0},
			LabelIdxs: []int{1},
		},
		// the declared type decides: a DECIMAL is a value even though it
		// arrives as text, and a VARCHAR stays a label even when its text
		// parses as a number.
		"Declared types classify the columns": {
			Result: MySQLQueryResult{
				Columns:     []string{"time", "code", "total"},
				ColumnTypes: []string{"BIGINT", "VARCHAR", "DECIMAL"},
				Rows:        [][]any{{int64(1700000000), "200", "10.50"}},
			},
			TimeIdx:   0,
			ValueIdxs: []int{2},
			LabelIdxs: []int{1},
		},
		"Time value and label columns": {
			Result: MySQLQueryResult{
				Columns: []string{"time", "value", "host"},
				Rows:    [][]any{{int64(1700000000), float64(1), "a"}},
			},
			TimeIdx:   0,
			ValueIdxs: []int{1},
			LabelIdxs: []int{2},
		},
		"Short first row treats missing columns as labels": {
			Result: MySQLQueryResult{
				Columns: []string{"time", "value", "host"},
				Rows:    [][]any{{int64(1700000000)}},
			},
			TimeIdx:   0,
			LabelIdxs: []int{1, 2},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			timeIdx, valueIdxs, labelIdxs := c.Result.identifyColumns()
			assert.Equal(t, c.TimeIdx, timeIdx)
			assert.Equal(t, c.ValueIdxs, valueIdxs)
			assert.Equal(t, c.LabelIdxs, labelIdxs)
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
		"Error returned by rows iteration": {
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

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, readOnly)
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
		"Time value": {
			Value:  time.Date(2023, 11, 14, 22, 13, 20, 0, time.UTC),
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

func Test_mysqlEstimateValueSize(t *testing.T) {
	cc := map[string]struct {
		Value  any
		Result int
	}{
		"Nil value": {
			Result: 4,
		},
		"Bool value": {
			Value:  true,
			Result: 5,
		},
		"String value": {
			Value:  "hello",
			Result: 7,
		},
		"Bytes value": {
			Value:  []byte("hello"),
			Result: 5,
		},
		"Numeric value": {
			Value:  int64(42),
			Result: 8,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, mysqlEstimateValueSize(c.Value))
		})
	}
}
