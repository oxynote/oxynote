package processor

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_PostgreSQLQueryResult_Transform(t *testing.T) {
	tests := map[string]struct {
		Result    PostgreSQLQueryResult
		ChartType ChartType
		Expected  QueryResult
	}{
		"Empty chart type returns type-not-selected": {
			Result: PostgreSQLQueryResult{
				Columns: []string{"time", "value"},
				Rows:    [][]any{{1700000000.0, 42.0}},
			},
			ChartType: "",
			Expected:  QueryResult{Status: QueryStatusTypeNotSelected},
		},
		"No rows returns no-data": {
			Result: PostgreSQLQueryResult{
				Columns: []string{"time", "value"},
				Rows:    nil,
			},
			ChartType: ChartTypeLine,
			Expected:  QueryResult{Status: QueryStatusNoData},
		},
		"No columns returns no-data": {
			Result: PostgreSQLQueryResult{
				Columns: nil,
				Rows:    [][]any{{1700000000.0, 42.0}},
			},
			ChartType: ChartTypeLine,
			Expected:  QueryResult{Status: QueryStatusNoData},
		},
		"No time column returns chart-and-data-mismatch": {
			Result: PostgreSQLQueryResult{
				Columns: []string{"name", "value"},
				Rows:    [][]any{{"foo", 42.0}},
			},
			ChartType: ChartTypeLine,
			Expected:  QueryResult{Status: QueryStatusChartAndDataMismatch},
		},
		"No numeric column returns chart-and-data-mismatch": {
			Result: PostgreSQLQueryResult{
				Columns: []string{"time", "name"},
				Rows:    [][]any{{1700000000.0, "foo"}},
			},
			ChartType: ChartTypeLine,
			Expected:  QueryResult{Status: QueryStatusChartAndDataMismatch},
		},
		"Single value column": {
			Result: PostgreSQLQueryResult{
				Columns: []string{"time", "value"},
				Rows: [][]any{
					{1700000000.0, 10.0},
					{1700000060.0, 20.0},
					{1700000120.0, 30.0},
				},
			},
			ChartType: ChartTypeLine,
			Expected: QueryResult{
				Status: QueryStatusOK,
				Data: []QueryResultSeries{
					{
						Labels: map[string]string{},
						Metrics: [][2]any{
							{int64(1700000000), 10.0},
							{int64(1700000060), 20.0},
							{int64(1700000120), 30.0},
						},
					},
				},
			},
		},
		"RFC3339 timestamps": {
			Result: PostgreSQLQueryResult{
				Columns: []string{"time", "value"},
				Rows: [][]any{
					{"2023-11-14T22:13:20Z", 10.0},
					{"2023-11-14T22:14:20Z", 20.0},
				},
			},
			ChartType: ChartTypeLine,
			Expected: QueryResult{
				Status: QueryStatusOK,
				Data: []QueryResultSeries{
					{
						Labels: map[string]string{},
						Metrics: [][2]any{
							{int64(1700000000), 10.0},
							{int64(1700000060), 20.0},
						},
					},
				},
			},
		},
		"Single value column with labels": {
			Result: PostgreSQLQueryResult{
				Columns: []string{"time", "host", "value"},
				Rows: [][]any{
					{1700000000.0, "host-a", 10.0},
					{1700000000.0, "host-b", 20.0},
					{1700000060.0, "host-a", 15.0},
					{1700000060.0, "host-b", 25.0},
				},
			},
			ChartType: ChartTypeLine,
			Expected: QueryResult{
				Status: QueryStatusOK,
				Data: []QueryResultSeries{
					{
						Labels: map[string]string{"host": "host-a"},
						Metrics: [][2]any{
							{int64(1700000000), 10.0},
							{int64(1700000060), 15.0},
						},
					},
					{
						Labels: map[string]string{"host": "host-b"},
						Metrics: [][2]any{
							{int64(1700000000), 20.0},
							{int64(1700000060), 25.0},
						},
					},
				},
			},
		},
		"Multiple value columns without labels": {
			Result: PostgreSQLQueryResult{
				Columns: []string{"time", "cpu", "memory"},
				Rows: [][]any{
					{1700000000.0, 0.41, 0.67},
					{1700000060.0, 0.43, 0.65},
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
			Result: PostgreSQLQueryResult{
				Columns: []string{"time", "cpu", "memory", "hostname"},
				Rows: [][]any{
					{1700000000.0, 0.41, 0.67, "web-1"},
					{1700000060.0, 0.43, 0.65, "web-1"},
					{1700000000.0, 0.55, 0.71, "web-2"},
					{1700000060.0, 0.51, 0.70, "web-2"},
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
			Result: PostgreSQLQueryResult{
				Columns: []string{"time", "cpu", "memory"},
				Rows: [][]any{
					{1700000000.0, 0.41, 0.67},
					{1700000060.0, 0.43, 0.65},
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
			Result: PostgreSQLQueryResult{
				Columns: []string{"time", "count"},
				Rows:    [][]any{{1700000000.0, 42.0}},
			},
			ChartType: ChartTypeBar,
			Expected: QueryResult{
				Status: QueryStatusOK,
				Data: []QueryResultSeries{
					{
						Labels:  map[string]string{},
						Metrics: [][2]any{{int64(1700000000), 42.0}},
					},
				},
			},
		},
		"Skips rows with invalid values": {
			Result: PostgreSQLQueryResult{
				Columns: []string{"time", "value"},
				Rows: [][]any{
					{1700000000.0, 10.0},
					{1700000060.0, "not-a-number"},
					{1700000120.0, 30.0},
				},
			},
			ChartType: ChartTypeLine,
			Expected: QueryResult{
				Status: QueryStatusOK,
				Data: []QueryResultSeries{
					{
						Labels: map[string]string{},
						Metrics: [][2]any{
							{int64(1700000000), 10.0},
							{int64(1700000120), 30.0},
						},
					},
				},
			},
		},
		"All invalid timestamps returns no-data": {
			Result: PostgreSQLQueryResult{
				Columns: []string{"time", "value"},
				Rows: [][]any{
					{"bad-time", 10.0},
					{"bad-time", 20.0},
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

func Test_sqlSetLimit(t *testing.T) {
	// the query is bounded by wrapping rather than by rewriting its tail,
	// so every shape produces the same envelope.
	wrapped := func(q string, n int) string {
		return "SELECT * FROM (\n" + q + "\n) AS oxynote_limited LIMIT " + strconv.Itoa(n)
	}

	tests := map[string]struct {
		Query    string
		Limit    int
		Expected string
	}{
		"Simple query is wrapped": {
			Query:    "SELECT * FROM users",
			Limit:    1,
			Expected: wrapped("SELECT * FROM users", 1),
		},
		"Trailing semicolon and whitespace are stripped": {
			Query:    "SELECT * FROM users ;  ",
			Limit:    1,
			Expected: wrapped("SELECT * FROM users", 1),
		},
		// the inner limit stays: it is the user's, and the wrapper only
		// caps what comes out of it.
		"Existing LIMIT is kept inside the wrapper": {
			Query:    "SELECT * FROM users LIMIT 100",
			Limit:    1,
			Expected: wrapped("SELECT * FROM users LIMIT 100", 1),
		},
		"LIMIT with OFFSET is left alone": {
			Query:    "SELECT * FROM users LIMIT 100 OFFSET 20",
			Limit:    1,
			Expected: wrapped("SELECT * FROM users LIMIT 100 OFFSET 20", 1),
		},
		"MySQL two-argument LIMIT is left alone": {
			Query:    "SELECT * FROM users LIMIT 5, 10",
			Limit:    1,
			Expected: wrapped("SELECT * FROM users LIMIT 5, 10", 1),
		},
		// the closing parenthesis has to land on its own line, or the
		// comment would swallow it and the statement would not parse.
		"Query ending in a line comment stays bounded": {
			Query:    "SELECT * FROM users -- only the active ones",
			Limit:    1,
			Expected: wrapped("SELECT * FROM users -- only the active ones", 1),
		},
		"CTE is wrapped whole": {
			Query:    "WITH top AS (SELECT id FROM users LIMIT 10) SELECT * FROM top",
			Limit:    1,
			Expected: wrapped("WITH top AS (SELECT id FROM users LIMIT 10) SELECT * FROM top", 1),
		},
		"Multiline query keeps its shape": {
			Query:    "SELECT *\nFROM users\nWHERE active = true",
			Limit:    1,
			Expected: wrapped("SELECT *\nFROM users\nWHERE active = true", 1),
		},
		"Limit value is carried through": {
			Query:    "SELECT * FROM users",
			Limit:    25,
			Expected: wrapped("SELECT * FROM users", 25),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			result := sqlSetLimit(tc.Query, tc.Limit)
			assert.Equal(t, tc.Expected, result)
		})
	}
}

func Test_pgNormalizeValue(t *testing.T) {
	numeric := pgtype.Numeric{}
	require.NoError(t, numeric.Scan("42.5"))

	tests := map[string]struct {
		In       any
		Expected any
	}{
		"Timestamp becomes RFC3339 string": {
			In:       time.Date(2026, 8, 5, 12, 30, 0, 0, time.UTC),
			Expected: "2026-08-05T12:30:00Z",
		},
		"Int64 becomes float64": {
			In:       int64(42),
			Expected: 42.0,
		},
		"Int32 becomes float64": {
			In:       int32(7),
			Expected: 7.0,
		},
		// pgx decodes real as float32; a column left unconverted reads as a
		// label and the chart reports a data mismatch.
		"Float32 becomes float64": {
			In:       float32(0.25),
			Expected: 0.25,
		},
		"Int16 becomes float64": {
			In:       int16(3),
			Expected: 3.0,
		},
		"Numeric becomes float64": {
			In:       numeric,
			Expected: 42.5,
		},
		"Float64 passes through": {
			In:       42.5,
			Expected: 42.5,
		},
		"String passes through": {
			In:       "foo",
			Expected: "foo",
		},
		"Nil passes through": {
			In:       nil,
			Expected: nil,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			result := pgNormalizeValue(tc.In)
			assert.Equal(t, tc.Expected, result)
		})
	}
}

func Test_NewPostgreSQL(t *testing.T) {
	t.Parallel()

	inp := &InputMock{}

	p := NewPostgreSQL(inp)
	require.NotNil(t, p)
	assert.Same(t, inp, p.inp)
}

func Test_PostgreSQL_TestConnection(t *testing.T) {
	cc := map[string]struct {
		URL    string
		Result ConnectionStatus
	}{
		"Error returned by connect": {
			URL:    "://",
			Result: ConnectionStatusUnreachable,
		},
		"Unreachable database": {
			URL:    "postgres://127.0.0.1:1/db",
			Result: ConnectionStatusUnreachable,
		},
		// a refused password is not the same as a host that never answers.
		"Wrong credentials": {
			URL:    pgTestURL(_readerUser, "wrong-password"),
			Result: ConnectionStatusUnauthorized,
		},
		"Not read-only user": {
			URL:    pgTestURL(_pgUser, _pgPass),
			Result: ConnectionStatusNotReadOnly,
		},
		"Successful read-only connection": {
			URL:    pgTestURL(_readerUser, _readerPass),
			Result: ConnectionStatusSuccess,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			p := NewPostgreSQL(&InputMock{
				URLFunc: func() string { return c.URL },
			})

			cs, err := p.TestConnection(context.Background())
			require.NoError(t, err)
			assert.Equal(t, c.Result, cs)
		})
	}
}

func Test_PostgreSQL_Metadata(t *testing.T) {
	cc := map[string]struct {
		URL    string
		Result *SQLMetadataResult
		Err    error
	}{
		"Error returned by buildConnectionString": {
			URL: "://",
			Err: assert.AnError,
		},
		"Error returned by pgx.Connect": {
			URL: "postgres://127.0.0.1:1/db",
			Err: assert.AnError,
		},
		"Successful retrieval": {
			URL: pgTestURL(_pgUser, _pgPass),
			Result: &SQLMetadataResult{
				Tables: map[string]SQLTable{
					"public.metrics": {
						Columns: []SQLColumn{
							{Name: "time"},
							{Name: "host"},
							{Name: "value"},
						},
					},
					"public.typed_metrics": {
						Columns: []SQLColumn{
							{Name: "time"},
							{Name: "host"},
							{Name: "ratio"},
							{Name: "total"},
						},
					},
				},
				DefaultSchema: "public",
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			p := NewPostgreSQL(&InputMock{
				URLFunc: func() string { return c.URL },
			})

			result, err := p.Metadata(context.Background())
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, result)
		})
	}
}

func Test_PostgreSQL_QueryLabels(t *testing.T) {
	cc := map[string]struct {
		URL       string
		Query     string
		Result    map[string]string
		ErrStatus int
		Err       error
	}{
		"Error returned by buildConnectionString": {
			URL:   "://",
			Query: "SELECT 1",
			Err:   assert.AnError,
		},
		"Error returned by pgx.Connect": {
			URL:   "postgres://127.0.0.1:1/db",
			Query: "SELECT 1",
			Err:   assert.AnError,
		},
		"Invalid query error": {
			URL:       pgTestURL(_pgUser, _pgPass),
			Query:     "SELECT bogus FROM",
			ErrStatus: http.StatusBadRequest,
			Err:       assert.AnError,
		},
		// a runtime failure is reported through rows.Err rather than the
		// Query call, so an empty result must not read as success.
		"Execution failure on an empty result": {
			URL:   pgTestURL(_pgUser, _pgPass),
			Query: "SELECT 1/0 WHERE false OR 1/0 = 0",
			Err:   assert.AnError,
		},
		"Successful retrieval": {
			URL:   pgTestURL(_pgUser, _pgPass),
			Query: "SELECT time, host, value FROM metrics ORDER BY time",
			// only the text column produces a label; the numeric
			// columns come back as float64 and are skipped.
			Result: map[string]string{"host": "web-1"},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			p := NewPostgreSQL(&InputMock{
				URLFunc: func() string { return c.URL },
			})

			labels, err := p.QueryLabels(context.Background(), c.Query, _testTimeRange)
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

func Test_PostgreSQL_Query(t *testing.T) {
	cc := map[string]struct {
		URL       string
		Query     string
		Result    *PostgreSQLQueryResult
		ErrStatus int
		Err       error
	}{
		"Error returned by buildConnectionString": {
			URL:   "://",
			Query: "SELECT 1",
			Err:   assert.AnError,
		},
		"Error returned by pgx.Connect": {
			URL:   "postgres://127.0.0.1:1/db",
			Query: "SELECT 1",
			Err:   assert.AnError,
		},
		// a real column decodes as float32 and a numeric as pgtype.Numeric;
		// both have to arrive as float64 or the chart reads them as labels.
		"Real and numeric columns become float64": {
			URL:   pgTestURL(_pgUser, _pgPass),
			Query: "SELECT ratio, total FROM typed_metrics",
			Result: &PostgreSQLQueryResult{
				Columns: []string{"ratio", "total"},
				Rows:    [][]any{{0.25, 10.5}},
			},
		},
		"Invalid query error": {
			URL:       pgTestURL(_pgUser, _pgPass),
			Query:     "SELECT bogus FROM",
			ErrStatus: http.StatusBadRequest,
			Err:       assert.AnError,
		},
		// pgx defers execution, so this failure surfaces through rows.Err
		// during iteration rather than at the Query call.
		"Runtime failure raised by the query text": {
			URL:       pgTestURL(_pgUser, _pgPass),
			Query:     "SELECT value / 0 FROM metrics",
			ErrStatus: http.StatusBadRequest,
			Err:       assert.AnError,
		},
		"Successful query": {
			URL:   pgTestURL(_pgUser, _pgPass),
			Query: "SELECT time, host, value FROM metrics ORDER BY time",
			Result: &PostgreSQLQueryResult{
				Columns: []string{"time", "host", "value"},
				Rows: [][]any{
					{float64(1700000000), "web-1", 10.5},
					{float64(1700000060), "web-2", 20.5},
				},
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			p := NewPostgreSQL(&InputMock{
				URLFunc: func() string { return c.URL },
			})

			result, err := p.Query(context.Background(), c.Query, _testTimeRange)
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

func Test_PostgreSQL_connect(t *testing.T) {
	t.Parallel()

	// buildConnectionString error
	p := NewPostgreSQL(&InputMock{
		URLFunc: func() string { return "://" },
	})

	_, err := p.connect(context.Background())
	assert.Error(t, err)

	// connection error
	p = NewPostgreSQL(&InputMock{
		URLFunc: func() string { return "postgres://127.0.0.1:1/db" },
	})

	_, err = p.connect(context.Background())
	assert.Error(t, err)

	// the session refuses writes, and connection-string keys outside the
	// allowlist are rejected at parse, so the simple protocol cannot be
	// re-enabled through the data source URL to smuggle a second statement
	// past the gate, and the URL cannot reach the local filesystem.
	cc := map[string]struct {
		URL   string
		Query string
	}{
		"Write statement": {
			URL:   pgTestURL(_pgUser, _pgPass),
			Query: "DELETE FROM metrics",
		},
		"Write smuggled through the simple protocol": {
			URL:   pgTestURL(_pgUser, _pgPass) + "&default_query_exec_mode=simple_protocol",
			Query: "SET transaction_read_only = off; DELETE FROM metrics",
		},
		"Disallowed sslkey key": {
			URL:   pgTestURL(_pgUser, _pgPass) + "&sslkey=/tmp/key",
			Query: "SELECT time FROM metrics",
		},
		"Disallowed servicefile key": {
			URL:   pgTestURL(_pgUser, _pgPass) + "&servicefile=/tmp/service",
			Query: "SELECT time FROM metrics",
		},
		"Disallowed options key": {
			URL:   pgTestURL(_pgUser, _pgPass) + "&options=-csearch_path%3Dpublic",
			Query: "SELECT time FROM metrics",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			p := NewPostgreSQL(&InputMock{
				URLFunc: func() string { return c.URL },
			})

			_, err := p.Query(context.Background(), c.Query, _testTimeRange)
			assert.Error(t, err)

			// the rows are still there, checked over a clean URL since the
			// case's own URL may be the thing being rejected.
			p = NewPostgreSQL(&InputMock{
				URLFunc: func() string { return pgTestURL(_pgUser, _pgPass) },
			})

			res, err := p.Query(context.Background(), "SELECT time FROM metrics ORDER BY time", _testTimeRange)
			require.NoError(t, err)
			assert.Len(t, res.Rows, 2)
		})
	}
}

func Test_PostgreSQL_buildConnectionString(t *testing.T) {
	cc := map[string]struct {
		URL    string
		Creds  Credentials
		Result string
		Err    error
	}{
		"Error returned by url.Parse": {
			URL: "://",
			Err: assert.AnError,
		},
		"Error returned by unmarshaling credentials": {
			URL:   "postgres://dbhost:5432/mydb",
			Creds: NewCredentials([]byte(`{`)),
			Err:   assert.AnError,
		},
		"Credentials override url user": { //nolint:gosec // static test credentials
			URL:    "postgres://olduser:oldpass@dbhost:5432/mydb",
			Creds:  NewCredentials([]byte(`{"username":"credsuser","password":"credspass"}`)),
			Result: "postgres://credsuser:credspass@dbhost:5432/mydb",
		},
		"Empty credentials keep url user": { //nolint:gosec // static test credentials
			URL:    "postgres://olduser:oldpass@dbhost:5432/mydb",
			Creds:  NewCredentials([]byte(`{"username":"","password":""}`)),
			Result: "postgres://olduser:oldpass@dbhost:5432/mydb",
		},
		"No credentials keep url unchanged": {
			URL:    "postgres://dbhost:5432/mydb",
			Result: "postgres://dbhost:5432/mydb",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			p := NewPostgreSQL(&InputMock{
				URLFunc:         func() string { return c.URL },
				CredentialsFunc: func() Credentials { return c.Creds },
			})

			connStr, err := p.buildConnectionString()
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, connStr)
		})
	}
}

func Test_pgParseTimestamp(t *testing.T) {
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
		"Unsupported type": {
			Value: int64(1700000000),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			ts, ok := pgParseTimestamp(c.Value)
			assert.Equal(t, c.OK, ok)
			assert.Equal(t, c.Result, ts)
		})
	}
}

func Test_pgParseNumericValue(t *testing.T) {
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
		"Float32 value": {
			Value:  float32(0.25),
			Result: 0.25,
			OK:     true,
		},
		"Unsupported type": {
			Value: "10.5",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			v, ok := pgParseNumericValue(c.Value)
			assert.Equal(t, c.OK, ok)
			assert.InDelta(t, c.Result, v, 0.0001)
		})
	}
}

func Test_pgQueryError(t *testing.T) {
	type tcase struct {
		Inp error
		Err error
	}

	cc := map[string]tcase{
		"Invalid-query PgError": {
			Inp: &pgconn.PgError{Code: "42601", Message: "syntax error"},
			Err: NewInvalidQueryError("syntax error"),
		},
		"Data-exception PgError": {
			Inp: &pgconn.PgError{Code: "22012", Message: "division by zero"},
			Err: NewInvalidQueryError("division by zero"),
		},
		"Query-cancelled PgError": {
			Inp: &pgconn.PgError{Code: "57014", Message: "canceling statement"},
			Err: NewInvalidQueryError("canceling statement"),
		},
		"PgError with a short code": func() tcase {
			err := &pgconn.PgError{Code: "4", Message: "broken"}

			return tcase{
				Inp: err,
				Err: fmt.Errorf("error executing query: %w", err),
			}
		}(),
		"Non-postgres error": func() tcase {
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

			testutil.AssertEqualError(t, c.Err, pgQueryError(c.Inp))
		})
	}
}
