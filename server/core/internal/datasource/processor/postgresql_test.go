package processor

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_PostgreSQLQueryResult_Transform(t *testing.T) {
	cc := map[string]struct {
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

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			result := c.Result.Transform(c.ChartType)
			require.Equal(t, c.Expected.Status, result.Status)
			assert.Equal(t, c.Expected.Data, result.Data)
		})
	}
}

func Test_PostgreSQLQueryResult_HasData(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Result *PostgreSQLQueryResult
		Has    bool
	}{
		"No result": {},
		"No rows":   {Result: &PostgreSQLQueryResult{Columns: []string{"value"}}},
		"A row":     {Result: &PostgreSQLQueryResult{Columns: []string{"value"}, Rows: [][]any{{1.0}}}, Has: true},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Has, c.Result.HasData())
		})
	}
}

func Test_sqlSetLimit(t *testing.T) {
	// the query is bounded by wrapping rather than by rewriting its tail,
	// so every shape produces the same envelope.
	cc := map[string]struct {
		Query    string
		Limit    int
		Expected string
	}{
		"Simple query is wrapped": {
			Query:    "SELECT * FROM users",
			Limit:    1,
			Expected: "SELECT * FROM (\nSELECT * FROM users\n) AS oxynote_limited LIMIT 1",
		},
		"Trailing semicolon and whitespace are stripped": {
			Query:    "SELECT * FROM users ;  ",
			Limit:    1,
			Expected: "SELECT * FROM (\nSELECT * FROM users\n) AS oxynote_limited LIMIT 1",
		},
		// the inner limit stays: it is the user's, and the wrapper only
		// caps what comes out of it.
		"Existing LIMIT is kept inside the wrapper": {
			Query:    "SELECT * FROM users LIMIT 100",
			Limit:    1,
			Expected: "SELECT * FROM (\nSELECT * FROM users LIMIT 100\n) AS oxynote_limited LIMIT 1",
		},
		"LIMIT with OFFSET is left alone": {
			Query:    "SELECT * FROM users LIMIT 100 OFFSET 20",
			Limit:    1,
			Expected: "SELECT * FROM (\nSELECT * FROM users LIMIT 100 OFFSET 20\n) AS oxynote_limited LIMIT 1",
		},
		"MySQL two-argument LIMIT is left alone": {
			Query:    "SELECT * FROM users LIMIT 5, 10",
			Limit:    1,
			Expected: "SELECT * FROM (\nSELECT * FROM users LIMIT 5, 10\n) AS oxynote_limited LIMIT 1",
		},
		// the closing parenthesis has to land on its own line, or the
		// comment would swallow it and the statement would not parse.
		"Query ending in a line comment stays bounded": {
			Query:    "SELECT * FROM users -- only the active ones",
			Limit:    1,
			Expected: "SELECT * FROM (\nSELECT * FROM users -- only the active ones\n) AS oxynote_limited LIMIT 1",
		},
		"CTE is wrapped whole": {
			Query:    "WITH top AS (SELECT id FROM users LIMIT 10) SELECT * FROM top",
			Limit:    1,
			Expected: "SELECT * FROM (\nWITH top AS (SELECT id FROM users LIMIT 10) SELECT * FROM top\n) AS oxynote_limited LIMIT 1",
		},
		"Multiline query keeps its shape": {
			Query:    "SELECT *\nFROM users\nWHERE active = true",
			Limit:    1,
			Expected: "SELECT * FROM (\nSELECT *\nFROM users\nWHERE active = true\n) AS oxynote_limited LIMIT 1",
		},
		"Limit value is carried through": {
			Query:    "SELECT * FROM users",
			Limit:    25,
			Expected: "SELECT * FROM (\nSELECT * FROM users\n) AS oxynote_limited LIMIT 25",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			result := sqlSetLimit(c.Query, c.Limit)
			assert.Equal(t, c.Expected, result)
		})
	}
}

func Test_pgNormalizeValue(t *testing.T) {
	numeric := pgtype.Numeric{}
	require.NoError(t, numeric.Scan("42.5"))

	cc := map[string]struct {
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
		"Oid becomes a decimal string": {
			In:       uint32(16384),
			Expected: "16384",
		},
		"Xid8 becomes a decimal string": {
			In:       uint64(18446744073709551615),
			Expected: "18446744073709551615",
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

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			result := pgNormalizeValue(c.In)
			assert.Equal(t, c.Expected, result)
		})
	}
}

func Test_NewPostgreSQL(t *testing.T) {
	t.Parallel()

	inp := &InputMock{}

	p := NewPostgreSQL(inp)
	require.NotNil(t, p)
	assert.Same(t, inp, p.inp)
	assert.Nil(t, p.pool)
	assert.False(t, p.closed)
}

func Test_NewSupabase(t *testing.T) {
	t.Parallel()

	inp := &InputMock{}

	p := NewSupabase(inp)
	require.NotNil(t, p)
	assert.Same(t, inp, p.inp)
	assert.Equal(t, _supabaseSchemas, p.hiddenSchemas)
	assert.Nil(t, p.pool)
	assert.False(t, p.closed)
}

func Test_PostgreSQL_Close(t *testing.T) {
	cc := map[string]struct {
		// Used indicates whether a query built the pool before the close.
		Used bool
	}{
		"Pool never built": {},
		"Pool in use":      {Used: true},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			p := NewPostgreSQL(&InputMock{
				URLFunc: func() string {
					return fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable", _pgUser, _pgPass, _pgAddr, _testDB)
				},
			})

			if c.Used {
				_, err := p.Query(context.Background(), "SELECT 1", _testTimeRange)
				require.NoError(t, err)
			}

			require.NoError(t, p.Close())
			assert.True(t, p.closed)

			// a closed processor hands out no connection, and builds no new
			// pool that nothing would close.
			_, err := p.connect(context.Background())
			testutil.AssertEqualError(t, _errClosed, err)

			if c.Used {
				require.NotNil(t, p.pool)
				assert.Error(t, p.pool.Ping(context.Background()))
			}
		})
	}
}

func Test_PostgreSQL_TestConnection(t *testing.T) {
	cc := map[string]struct {
		URL              string
		Creds            Credentials
		Result           ConnectionStatus
		CredentialsCalls int
	}{
		"Error returned by connect": {
			URL:              "://",
			Result:           ConnectionStatusUnreachable,
			CredentialsCalls: 1,
		},
		"Unreachable database": {
			URL:              "postgres://127.0.0.1:1/db",
			Result:           ConnectionStatusUnreachable,
			CredentialsCalls: 1,
		},
		// a refused password is not the same as a host that never answers.
		"Wrong credentials": {
			URL:              fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable", _readerUser, "wrong-password", _pgAddr, _testDB),
			Result:           ConnectionStatusUnauthorized,
			CredentialsCalls: 1,
		},
		"Not read-only user": {
			URL:              fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable", _pgUser, _pgPass, _pgAddr, _testDB),
			Result:           ConnectionStatusNotReadOnly,
			CredentialsCalls: 1,
		},
		"Successful read-only connection": {
			URL:              fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable", _readerUser, _readerPass, _pgAddr, _testDB),
			Result:           ConnectionStatusSuccess,
			CredentialsCalls: 1,
		},
		"Invalid CA certificate": {
			URL:              fmt.Sprintf("postgres://%s:%s@%s/%s", _readerUser, _readerPass, _pgAddr, _testDB),
			Creds:            caCredentials("not a certificate"),
			Result:           ConnectionStatusInvalidCACertificate,
			CredentialsCalls: 1,
		},
		// the server has no TLS. A CA certificate allows no plaintext
		// fallback, whatever sslmode the URL asks for.
		"CA certificate against a server without TLS": {
			URL:              fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable", _readerUser, _readerPass, _pgAddr, _testDB),
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

			p := NewPostgreSQL(inp)

			t.Cleanup(func() { assert.NoError(t, p.Close()) })

			cs, err := p.TestConnection(context.Background())
			require.NoError(t, err)
			assert.Equal(t, c.Result, cs)
			assert.Len(t, inp.URLCalls(), 1)
			assert.Len(t, inp.CredentialsCalls(), c.CredentialsCalls)
		})
	}
}

func Test_pgConnectionStatus(t *testing.T) {
	cc := map[string]struct {
		Err    error
		Result ConnectionStatus
	}{
		"Invalid CA certificate": {
			Err:    fmt.Errorf("connecting: %w", _errInvalidCACertificate),
			Result: ConnectionStatusInvalidCACertificate,
		},
		"Error that is not from the server": {
			Err:    assert.AnError,
			Result: ConnectionStatusUnreachable,
		},
		"Refused password": {
			Err:    &pgconn.PgError{Code: "28P01", Message: `password authentication failed for user "reader"`},
			Result: ConnectionStatusUnauthorized,
		},
		"Unknown database": {
			Err:    &pgconn.PgError{Code: "3D000", Message: `database "nosuchdb" does not exist`},
			Result: ConnectionStatusUnreachable,
		},
		"Supavisor without the project": {
			Err:    &pgconn.PgError{Code: "XX000", Message: "(ENOTFOUND) tenant/user reader.ref not found"},
			Result: ConnectionStatusUnauthorized,
		},
		"Supavisor given a user without a project": {
			Err:    &pgconn.PgError{Code: "XX000", Message: "(ENOIDENTIFIER) no tenant identifier provided (external_id or sni_hostname required)"},
			Result: ConnectionStatusUnauthorized,
		},
		"Supavisor given a role the database lacks": {
			Err:    &pgconn.PgError{Code: "XX000", Message: "(EAUTHQUERY) user not found in the database"},
			Result: ConnectionStatusUnauthorized,
		},
		// the database behind the pooler is down or paused.
		"Supavisor unable to reach the database": {
			Err:    &pgconn.PgError{Code: "XX000", Message: "(EAUTHQUERY) auth_query connection failed: timeout"},
			Result: ConnectionStatusUnreachable,
		},
		"Supavisor pool full": {
			Err:    &pgconn.PgError{Code: "XX000", Message: "(EMAXCONNSESSION) max clients reached in session mode - max clients are limited to pool_size: 15"},
			Result: ConnectionStatusUnreachable,
		},
		"Supavisor blocking after failed logins": {
			Err:    &pgconn.PgError{Code: "XX000", Message: "(ECIRCUITBREAKER) too many authentication failures, new connections are temporarily blocked"},
			Result: ConnectionStatusUnreachable,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, pgConnectionStatus(c.Err))
		})
	}
}

func Test_PostgreSQL_Metadata(t *testing.T) {
	ownerURL := fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable", _pgUser, _pgPass, _pgAddr, _testDB)
	publicTables := map[string]SQLTable{
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
	}

	cc := map[string]struct {
		URL string

		// Supabase indicates whether the processor is the one for a
		// Supabase project.
		Supabase bool

		Result           *SQLMetadataResult
		Err              error
		CredentialsCalls int
	}{
		"Error returned by connect": {
			URL:              "://",
			Err:              assert.AnError,
			CredentialsCalls: 1,
		},
		"Error returned by pgxpool.Pool.Query": {
			URL:              "postgres://127.0.0.1:1/db",
			Err:              assert.AnError,
			CredentialsCalls: 1,
		},
		"Successful retrieval": {
			URL: ownerURL,
			Result: &SQLMetadataResult{
				Tables: func() map[string]SQLTable {
					tables := maps.Clone(publicTables)
					tables["auth.users"] = SQLTable{
						Columns: []SQLColumn{
							{Name: "id"},
							{Name: "email"},
						},
					}

					return tables
				}(),
				DefaultSchema: "public",
			},
			CredentialsCalls: 1,
		},
		// the schemas Supabase keeps for its own services are left out.
		"Successful retrieval from a Supabase project": {
			URL:      ownerURL,
			Supabase: true,
			Result: &SQLMetadataResult{
				Tables:        publicTables,
				DefaultSchema: "public",
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

			p := NewPostgreSQL(inp)
			if c.Supabase {
				p = NewSupabase(inp)
			}

			t.Cleanup(func() { assert.NoError(t, p.Close()) })

			result, err := p.Metadata(context.Background())
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

func Test_PostgreSQL_QueryLabels(t *testing.T) {
	cc := map[string]struct {
		URL              string
		Query            string
		Result           map[string]string
		ErrStatus        int
		Err              error
		CredentialsCalls int
	}{
		"Error returned by connect": {
			URL:              "://",
			Query:            "SELECT 1",
			Err:              assert.AnError,
			CredentialsCalls: 1,
		},
		"Error returned by pgxpool.Pool.Query": {
			URL:              "postgres://127.0.0.1:1/db",
			Query:            "SELECT 1",
			Err:              assert.AnError,
			CredentialsCalls: 1,
		},
		"Invalid query error": {
			URL:              fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable", _pgUser, _pgPass, _pgAddr, _testDB),
			Query:            "SELECT bogus FROM",
			ErrStatus:        http.StatusBadRequest,
			Err:              assert.AnError,
			CredentialsCalls: 1,
		},
		// a runtime failure is reported through rows.Err rather than the
		// Query call, so an empty result must not read as success.
		"Execution failure on an empty result": {
			URL:              fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable", _pgUser, _pgPass, _pgAddr, _testDB),
			Query:            "SELECT 1/0 WHERE false OR 1/0 = 0",
			Err:              assert.AnError,
			CredentialsCalls: 1,
		},
		"Successful retrieval": {
			URL:   fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable", _pgUser, _pgPass, _pgAddr, _testDB),
			Query: "SELECT time, host, value FROM metrics ORDER BY time",
			// only the text column produces a label; the numeric
			// columns come back as float64 and are skipped.
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

			p := NewPostgreSQL(inp)

			t.Cleanup(func() { assert.NoError(t, p.Close()) })

			labels, err := p.QueryLabels(context.Background(), c.Query, _testTimeRange)
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

func Test_PostgreSQL_Query(t *testing.T) {
	ownerURL := fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable", _pgUser, _pgPass, _pgAddr, _testDB)

	cc := map[string]struct {
		URL string

		// Before are the queries run first. A case with Before or Check
		// runs every query on one connection, so each sees what the one
		// before it left behind.
		Before []string

		Query            string
		Result           *PostgreSQLQueryResult
		ErrStatus        int
		Err              error
		CredentialsCalls int
		Check            func(t *testing.T, p *PostgreSQL)
	}{
		"Error returned by connect": {
			URL:              "://",
			Query:            "SELECT 1",
			Err:              assert.AnError,
			CredentialsCalls: 1,
		},
		"Error returned by pgxpool.Pool.Query": {
			URL:              "postgres://127.0.0.1:1/db",
			Query:            "SELECT 1",
			Err:              assert.AnError,
			CredentialsCalls: 1,
		},
		// a real column decodes as float32 and a numeric as pgtype.Numeric;
		// both have to arrive as float64 or the chart reads them as labels.
		"Real and numeric columns become float64": {
			URL:   fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable", _pgUser, _pgPass, _pgAddr, _testDB),
			Query: "SELECT ratio, total FROM typed_metrics",
			Result: &PostgreSQLQueryResult{
				Columns: []string{"ratio", "total"},
				Rows:    [][]any{{0.25, 10.5}},
			},
			CredentialsCalls: 1,
		},
		// pgx decodes oid as uint32 and xid8 as uint64. As strings they
		// stay labels, and a cached copy reads the same.
		"Oid and xid8 columns become decimal strings": {
			URL:   fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable", _pgUser, _pgPass, _pgAddr, _testDB),
			Query: "SELECT 16384::oid AS datid, '7'::xid8 AS xmin",
			Result: &PostgreSQLQueryResult{
				Columns: []string{"datid", "xmin"},
				Rows:    [][]any{{"16384", "7"}},
			},
			CredentialsCalls: 1,
		},
		"Invalid query error": {
			URL:              fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable", _pgUser, _pgPass, _pgAddr, _testDB),
			Query:            "SELECT bogus FROM",
			ErrStatus:        http.StatusBadRequest,
			Err:              assert.AnError,
			CredentialsCalls: 1,
		},
		// pgx defers execution, so this failure surfaces through rows.Err
		// during iteration rather than at the Query call.
		"Runtime failure raised by the query text": {
			URL:              fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable", _pgUser, _pgPass, _pgAddr, _testDB),
			Query:            "SELECT value / 0 FROM metrics",
			ErrStatus:        http.StatusBadRequest,
			Err:              assert.AnError,
			CredentialsCalls: 1,
		},
		"Successful query": {
			URL:   fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable", _pgUser, _pgPass, _pgAddr, _testDB),
			Query: "SELECT time, host, value FROM metrics ORDER BY time",
			Result: &PostgreSQLQueryResult{
				Columns: []string{"time", "host", "value"},
				Rows: [][]any{
					{float64(1700000000), "web-1", 10.5},
					{float64(1700000060), "web-2", 20.5},
				},
			},
			CredentialsCalls: 1,
		},
		// the table changes shape between two runs of the same query on one
		// connection. A named statement would still describe the old shape.
		"Query after its table changed shape": {
			URL: ownerURL,
			Before: []string{
				"DROP TABLE IF EXISTS reshaped",
				"CREATE TABLE reshaped (a int)",
				"INSERT INTO reshaped VALUES (1)",
				"SELECT * FROM reshaped",
				"ALTER TABLE reshaped ADD COLUMN b text DEFAULT 'x'",
			},
			Query: "SELECT * FROM reshaped",
			Result: &PostgreSQLQueryResult{
				Columns: []string{"a", "b"},
				Rows:    [][]any{{float64(1), "x"}},
			},
			CredentialsCalls: 1,
		},
		"A released connection serves the next query": {
			URL:    ownerURL,
			Before: []string{"SELECT time FROM metrics ORDER BY time", "SELECT time FROM metrics ORDER BY time"},
			Query:  "SELECT time FROM metrics ORDER BY time",
			Result: &PostgreSQLQueryResult{
				Columns: []string{"time"},
				Rows:    [][]any{{float64(1700000000)}, {float64(1700000060)}},
			},
			CredentialsCalls: 1,
			// the held connections plus the one every query shared.
			Check: func(t *testing.T, p *PostgreSQL) {
				t.Helper()

				assert.Equal(t, int64(_pgMaxConns), p.pool.Stat().NewConnsCount())
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			inp := &InputMock{
				URLFunc: func() string { return c.URL },
			}

			p := NewPostgreSQL(inp)

			t.Cleanup(func() { assert.NoError(t, p.Close()) })

			// with all but one connection held until the test ends, every
			// query lands on the one left.
			if len(c.Before) > 0 || c.Check != nil {
				pool, perr := p.connect(context.Background())
				require.NoError(t, perr)

				for range _pgMaxConns - 1 {
					held, aerr := pool.Acquire(context.Background())
					require.NoError(t, aerr)

					t.Cleanup(held.Release)
				}
			}

			for _, q := range c.Before {
				_, berr := p.Query(context.Background(), q, _testTimeRange)
				require.NoError(t, berr)
			}

			result, err := p.Query(context.Background(), c.Query, _testTimeRange)
			testutil.AssertEqualError(t, c.Err, err)
			assert.Len(t, inp.URLCalls(), 1)
			assert.Len(t, inp.CredentialsCalls(), c.CredentialsCalls)

			if c.ErrStatus != 0 {
				assert.Equal(t, c.ErrStatus, errutil.StatusCode(err, false))
			}

			if c.Check != nil {
				c.Check(t, p)
			}

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, result)
		})
	}
}

func Test_PostgreSQL_connect(t *testing.T) {
	cc := map[string]struct {
		URL              string
		Creds            Credentials
		Closed           bool
		Err              error
		CredentialsCalls int

		// RootCAs indicates whether every TLS config trusts the CA
		// certificate alone.
		RootCAs bool
	}{
		"Error returned by Credentials.basic": {
			URL:              "postgres://127.0.0.1:1/db",
			Creds:            NewCredentials([]byte(`{`)),
			Err:              assert.AnError,
			CredentialsCalls: 1,
		},
		"Error returned by buildConnectionString": {
			URL:              "://",
			Err:              assert.AnError,
			CredentialsCalls: 1,
		},
		"Error returned by BasicCredentials.rootCAs": {
			URL:              "postgres://127.0.0.1:1/db",
			Creds:            caCredentials("not a certificate"),
			Err:              _errInvalidCACertificate,
			CredentialsCalls: 1,
		},
		// a URL without a host falls back to a Unix socket directory such
		// as /tmp, which connects without TLS.
		"CA certificate on a Unix socket": {
			URL:              "postgres:///db",
			Creds:            caCredentials(_testCA),
			Err:              _errInvalidCACertificate,
			CredentialsCalls: 1,
		},
		"CA certificate on a host list ending in a Unix socket": {
			URL:              "postgres:///db?host=db1.test,/var/run/postgresql",
			Creds:            caCredentials(_testCA),
			Err:              _errInvalidCACertificate,
			CredentialsCalls: 1,
		},
		"Closed processor": {
			URL:    "postgres://127.0.0.1:1/db",
			Closed: true,
			Err:    _errClosed,
		},
		// connection-string keys outside the allowlist are rejected at
		// parse, so the simple protocol cannot be re-enabled through the
		// data source URL to chain a second statement, and the URL cannot
		// reach the local filesystem.
		"Disallowed default_query_exec_mode key": {
			URL:              fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable&default_query_exec_mode=simple_protocol", _pgUser, _pgPass, _pgAddr, _testDB),
			Err:              assert.AnError,
			CredentialsCalls: 1,
		},
		"Disallowed sslkey key": {
			URL:              fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable&sslkey=/tmp/key", _pgUser, _pgPass, _pgAddr, _testDB),
			Err:              assert.AnError,
			CredentialsCalls: 1,
		},
		"Disallowed servicefile key": {
			URL:              fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable&servicefile=/tmp/service", _pgUser, _pgPass, _pgAddr, _testDB),
			Err:              assert.AnError,
			CredentialsCalls: 1,
		},
		"Disallowed options key": {
			URL:              fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable&options=-csearch_path%%3Dpublic", _pgUser, _pgPass, _pgAddr, _testDB),
			Err:              assert.AnError,
			CredentialsCalls: 1,
		},
		"Successful pool": {
			URL:              fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable", _pgUser, _pgPass, _pgAddr, _testDB),
			CredentialsCalls: 1,
		},
		// each host gets a TLS config of its own.
		"Successful pool verified with a CA certificate": {
			URL:              "postgres://db1.test:5432,db2.test:5433/db?sslmode=disable",
			Creds:            caCredentials(_testCA),
			RootCAs:          true,
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

			p := NewPostgreSQL(inp)

			t.Cleanup(func() { assert.NoError(t, p.Close()) })

			if c.Closed {
				require.NoError(t, p.Close())
			}

			pool, err := p.connect(context.Background())
			testutil.AssertEqualError(t, c.Err, err)
			assert.Len(t, inp.CredentialsCalls(), c.CredentialsCalls)

			if err != nil {
				assert.Nil(t, p.pool)
				return
			}

			require.NotNil(t, pool)

			cfg := pool.Config()
			assert.Equal(t, int32(_pgMaxConns), cfg.MaxConns)
			assert.Equal(t, pgx.QueryExecModeCacheDescribe, cfg.ConnConfig.DefaultQueryExecMode)
			assert.Equal(t, _connectTimeout, cfg.ConnConfig.ConnectTimeout)

			if c.RootCAs {
				require.NotNil(t, cfg.ConnConfig.TLSConfig)
				assert.NotNil(t, cfg.ConnConfig.TLSConfig.RootCAs)
				assert.False(t, cfg.ConnConfig.TLSConfig.InsecureSkipVerify)
				require.Len(t, cfg.ConnConfig.Fallbacks, 1)
				require.NotNil(t, cfg.ConnConfig.Fallbacks[0].TLSConfig)
				assert.NotNil(t, cfg.ConnConfig.Fallbacks[0].TLSConfig.RootCAs)
			}

			// the pool is built once and handed to every later call.
			again, err := p.connect(context.Background())
			require.NoError(t, err)
			assert.Same(t, pool, again)
			assert.Len(t, inp.URLCalls(), 1)
		})
	}
}

func Test_PostgreSQL_buildConnectionString(t *testing.T) {
	cc := map[string]struct {
		URL    string
		Creds  BasicCredentials
		Result string
		Err    error
	}{
		"Error returned by url.Parse": {
			URL: "://",
			Err: assert.AnError,
		},
		"Credentials override url user": { //nolint:gosec // static test credentials
			URL: "postgres://olduser:oldpass@dbhost:5432/mydb",
			Creds: BasicCredentials{
				Username: "credsuser",
				Password: "credspass",
			},
			Result: "postgres://credsuser:credspass@dbhost:5432/mydb",
		},
		"Password keeps the url user": { //nolint:gosec // static test credentials
			URL:    "postgres://reader.ref@dbhost:6543/postgres",
			Creds:  BasicCredentials{Password: "credspass"},
			Result: "postgres://reader.ref:credspass@dbhost:6543/postgres",
		},
		"Username keeps the url password": { //nolint:gosec // static test credentials
			URL:    "postgres://olduser:oldpass@dbhost:5432/mydb",
			Creds:  BasicCredentials{Username: "credsuser"},
			Result: "postgres://credsuser:oldpass@dbhost:5432/mydb",
		},
		"Empty credentials keep url user": { //nolint:gosec // static test credentials
			URL:    "postgres://olduser:oldpass@dbhost:5432/mydb",
			Result: "postgres://olduser:oldpass@dbhost:5432/mydb",
		},
		"CA certificate forces verify-full": { //nolint:gosec // static test credentials
			URL:    "postgres://user:pass@dbhost:5432/mydb?connect_timeout=5&sslmode=disable",
			Creds:  BasicCredentials{CACertificate: "pem"},
			Result: "postgres://user:pass@dbhost:5432/mydb?connect_timeout=5&sslmode=verify-full",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			inp := &InputMock{
				URLFunc: func() string { return c.URL },
			}

			p := NewPostgreSQL(inp)

			connStr, err := p.buildConnectionString(c.Creds)
			testutil.AssertEqualError(t, c.Err, err)
			assert.Len(t, inp.URLCalls(), 1)
			assert.Empty(t, inp.CredentialsCalls())

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
