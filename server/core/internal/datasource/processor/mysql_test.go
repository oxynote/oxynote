package processor

import (
	"testing"

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
