package processor

import (
	"context"
	"net/http"
	"testing"
	"time"

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
	tests := map[string]struct {
		Query    string
		Limit    int
		Expected string
	}{
		"Appends LIMIT to simple query": {
			Query:    "SELECT * FROM users",
			Limit:    1,
			Expected: "SELECT * FROM users LIMIT 1",
		},
		"Replaces existing LIMIT": {
			Query:    "SELECT * FROM users LIMIT 100",
			Limit:    1,
			Expected: "SELECT * FROM users LIMIT 1",
		},
		"Replaces existing LIMIT with different value": {
			Query:    "SELECT * FROM users LIMIT 50",
			Limit:    5,
			Expected: "SELECT * FROM users LIMIT 5",
		},
		"Case insensitive - lowercase limit": {
			Query:    "SELECT * FROM users limit 100",
			Limit:    1,
			Expected: "SELECT * FROM users LIMIT 1",
		},
		"Case insensitive - mixed case": {
			Query:    "SELECT * FROM users Limit 100",
			Limit:    1,
			Expected: "SELECT * FROM users LIMIT 1",
		},
		"Strips trailing semicolon": {
			Query:    "SELECT * FROM users;",
			Limit:    1,
			Expected: "SELECT * FROM users LIMIT 1",
		},
		"Strips trailing semicolon with spaces": {
			Query:    "SELECT * FROM users ;  ",
			Limit:    1,
			Expected: "SELECT * FROM users LIMIT 1",
		},
		"Replaces LIMIT with trailing whitespace": {
			Query:    "SELECT * FROM users LIMIT 100  ",
			Limit:    1,
			Expected: "SELECT * FROM users LIMIT 1",
		},
		"Does not affect LIMIT inside subquery": {
			Query:    "SELECT * FROM (SELECT id FROM users LIMIT 10) sub",
			Limit:    1,
			Expected: "SELECT * FROM (SELECT id FROM users LIMIT 10) sub LIMIT 1",
		},
		"Does not affect LIMIT inside subquery in WHERE clause": {
			Query:    "SELECT * FROM orders WHERE user_id IN (SELECT id FROM users LIMIT 5) ORDER BY created_at",
			Limit:    1,
			Expected: "SELECT * FROM orders WHERE user_id IN (SELECT id FROM users LIMIT 5) ORDER BY created_at LIMIT 1",
		},
		"Replaces outer LIMIT with subquery LIMIT present": {
			Query:    "SELECT * FROM (SELECT id FROM users LIMIT 10) sub LIMIT 50",
			Limit:    1,
			Expected: "SELECT * FROM (SELECT id FROM users LIMIT 10) sub LIMIT 1",
		},
		"Handles query with ORDER BY": {
			Query:    "SELECT * FROM users ORDER BY name",
			Limit:    1,
			Expected: "SELECT * FROM users ORDER BY name LIMIT 1",
		},
		"Replaces LIMIT after ORDER BY": {
			Query:    "SELECT * FROM users ORDER BY name LIMIT 25",
			Limit:    1,
			Expected: "SELECT * FROM users ORDER BY name LIMIT 1",
		},
		"Handles query with GROUP BY": {
			Query:    "SELECT host, COUNT(*) FROM logs GROUP BY host LIMIT 20",
			Limit:    1,
			Expected: "SELECT host, COUNT(*) FROM logs GROUP BY host LIMIT 1",
		},
		"Handles multiline query": {
			Query:    "SELECT *\nFROM users\nWHERE active = true",
			Limit:    1,
			Expected: "SELECT *\nFROM users\nWHERE active = true LIMIT 1",
		},
		"Replaces LIMIT in multiline query": {
			Query:    "SELECT *\nFROM users\nLIMIT 100",
			Limit:    1,
			Expected: "SELECT *\nFROM users\nLIMIT 1",
		},
		"Handles CTE with subquery LIMIT": {
			Query:    "WITH top_users AS (SELECT id FROM users LIMIT 10) SELECT * FROM top_users JOIN orders ON orders.user_id = top_users.id",
			Limit:    1,
			Expected: "WITH top_users AS (SELECT id FROM users LIMIT 10) SELECT * FROM top_users JOIN orders ON orders.user_id = top_users.id LIMIT 1",
		},
		"Handles OFFSET after LIMIT": {
			Query:    "SELECT * FROM users LIMIT 100 OFFSET 20",
			Limit:    1,
			Expected: "SELECT * FROM users LIMIT 100 OFFSET 20 LIMIT 1",
		},
		"Handles empty query": {
			Query:    "",
			Limit:    1,
			Expected: " LIMIT 1",
		},
		"Does not match LIMIT in string literal at end": {
			Query:    "SELECT * FROM users WHERE name = 'LIMIT 100'",
			Limit:    1,
			Expected: "SELECT * FROM users WHERE name = 'LIMIT 100' LIMIT 1",
		},
		"Does not match NOLIMIT or similar": {
			Query:    "SELECT * FROM users WHERE nolimit = true",
			Limit:    1,
			Expected: "SELECT * FROM users WHERE nolimit = true LIMIT 1",
		},
		"Multiline query with GROUP BY and ORDER BY": {
			Query:    "SELECT\n\t$__timeGroupAlias(\"time\", \"35m\"),\n\tservice,\n\tCOUNT(*) AS deployments\nFROM deployments\nWHERE $__timeFilter(\"time\")\nGROUP BY 1, 2\nORDER BY 1, 2",
			Limit:    1,
			Expected: "SELECT\n\t$__timeGroupAlias(\"time\", \"35m\"),\n\tservice,\n\tCOUNT(*) AS deployments\nFROM deployments\nWHERE $__timeFilter(\"time\")\nGROUP BY 1, 2\nORDER BY 1, 2 LIMIT 1",
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
		"Invalid query error": {
			URL:       pgTestURL(_pgUser, _pgPass),
			Query:     "SELECT bogus FROM",
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
			Creds: Credentials(`{`),
			Err:   assert.AnError,
		},
		"Credentials override url user": { //nolint:gosec // static test credentials
			URL:    "postgres://olduser:oldpass@dbhost:5432/mydb",
			Creds:  Credentials(`{"username":"credsuser","password":"credspass"}`),
			Result: "postgres://credsuser:credspass@dbhost:5432/mydb",
		},
		"Empty credentials keep url user": { //nolint:gosec // static test credentials
			URL:    "postgres://olduser:oldpass@dbhost:5432/mydb",
			Creds:  Credentials(`{"username":"","password":""}`),
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

func Test_UpdatePostgreSQLCredentials(t *testing.T) {
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

			creds, err := UpdatePostgreSQLCredentials(c.Creds, c.Inp)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, creds)
		})
	}
}

func Test_PostgreSQLQueryResult_identifyColumns(t *testing.T) {
	cc := map[string]struct {
		Result    PostgreSQLQueryResult
		TimeIdx   int
		ValueIdxs []int
		LabelIdxs []int
	}{
		"No rows": {
			Result: PostgreSQLQueryResult{
				Columns: []string{"time", "value"},
			},
			TimeIdx: 0,
		},
		"No time column": {
			Result: PostgreSQLQueryResult{
				Columns: []string{"value", "host"},
				Rows:    [][]any{{float64(1), "a"}},
			},
			TimeIdx:   -1,
			ValueIdxs: []int{0},
			LabelIdxs: []int{1},
		},
		"Time value and label columns": {
			Result: PostgreSQLQueryResult{
				Columns: []string{"time", "value", "host"},
				Rows:    [][]any{{"2023-11-14T22:13:20Z", float64(1), "a"}},
			},
			TimeIdx:   0,
			ValueIdxs: []int{1},
			LabelIdxs: []int{2},
		},
		"Short first row treats missing columns as labels": {
			Result: PostgreSQLQueryResult{
				Columns: []string{"time", "value", "host"},
				Rows:    [][]any{{"2023-11-14T22:13:20Z"}},
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

func Test_pgEstimateValueSize(t *testing.T) {
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
			Value:  float64(42),
			Result: 8,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, pgEstimateValueSize(c.Value))
		})
	}
}
