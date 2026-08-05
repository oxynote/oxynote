package processor

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
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
