package processor

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_sqlQueryResult_metrics(t *testing.T) {
	points := []sqlPoint{{time: 0, value: 1}, {time: 60, value: 2}}

	cc := map[string]struct {
		Fill      *sqlFill
		ChartType ChartType
		Result    [][2]any
	}{
		"Line":                       {ChartType: ChartTypeLine, Result: [][2]any{{int64(0), 1.0}, {int64(60), 2.0}}},
		"Gauge keeps the last point": {ChartType: ChartTypeGauge, Result: [][2]any{{int64(60), 2.0}}},
		"Filled line":                {Fill: &sqlFill{Interval: 60, From: 0, To: 120}, ChartType: ChartTypeLine, Result: [][2]any{{int64(0), 1.0}, {int64(60), 2.0}, {int64(120), nil}}},
		"Gauge skips a null point":   {Fill: &sqlFill{Interval: 60, From: 0, To: 120}, ChartType: ChartTypeGauge, Result: [][2]any{{int64(60), 2.0}}},
		"Gauge of only null points":  {Fill: &sqlFill{Interval: 60, From: 600, To: 660}, ChartType: ChartTypeGauge, Result: [][2]any{}},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			sqr := sqlQueryResult{Fill: c.Fill}

			assert.Equal(t, c.Result, sqr.metrics(slices.Clone(points), c.ChartType))
		})
	}
}

func Test_sqlQueryResult_identifyColumns(t *testing.T) {
	cc := map[string]struct {
		Result    sqlQueryResult
		Parse     func(any) (float64, bool)
		TimeIdx   int
		ValueIdxs []int
		LabelIdxs []int
	}{
		"No rows": {
			Result: sqlQueryResult{
				Columns: []string{"time", "value"},
			},
			Parse:   mysqlParseNumericValue,
			TimeIdx: 0,
		},
		"No time column": {
			Result: sqlQueryResult{
				Columns: []string{"value", "host"},
				Rows:    [][]any{{float64(1), "a"}},
			},
			Parse:     mysqlParseNumericValue,
			TimeIdx:   -1,
			ValueIdxs: []int{0},
			LabelIdxs: []int{1},
		},
		// the declared type decides: a DECIMAL is a value even though it
		// arrives as text, and a VARCHAR stays a label even when its text
		// parses as a number.
		"Declared types classify the columns": {
			Result: sqlQueryResult{
				Columns:     []string{"time", "code", "total"},
				ColumnTypes: []string{"BIGINT", "VARCHAR", "DECIMAL"},
				Rows:        [][]any{{int64(1700000000), "200", "10.50"}},
			},
			Parse:     mysqlParseNumericValue,
			TimeIdx:   0,
			ValueIdxs: []int{2},
			LabelIdxs: []int{1},
		},
		"Time value and label columns": {
			Result: sqlQueryResult{
				Columns: []string{"time", "value", "host"},
				Rows:    [][]any{{int64(1700000000), float64(1), "a"}},
			},
			Parse:     mysqlParseNumericValue,
			TimeIdx:   0,
			ValueIdxs: []int{1},
			LabelIdxs: []int{2},
		},
		"Short first row treats missing columns as labels": {
			Result: sqlQueryResult{
				Columns: []string{"time", "value", "host"},
				Rows:    [][]any{{int64(1700000000)}},
			},
			Parse:     mysqlParseNumericValue,
			TimeIdx:   0,
			LabelIdxs: []int{1, 2},
		},
		// a column whose first row is NULL is still a value column; reading
		// only the first row drops the series from the chart.
		"Leading NULL does not turn a value column into a label": {
			Result: sqlQueryResult{
				Columns: []string{"time", "value"},
				Rows: [][]any{
					{float64(1700000000), nil},
					{float64(1700000060), float64(10)},
				},
			},
			Parse:     pgParseNumericValue,
			TimeIdx:   0,
			ValueIdxs: []int{1},
		},
		"Undeclared types fall back to the shape of the values": {
			Result: sqlQueryResult{
				Columns: []string{"time", "value", "host"},
				Rows:    [][]any{{"2023-11-14T22:13:20Z", float64(1), "a"}},
			},
			Parse:     pgParseNumericValue,
			TimeIdx:   0,
			ValueIdxs: []int{1},
			LabelIdxs: []int{2},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			timeIdx, valueIdxs, labelIdxs := c.Result.identifyColumns(c.Parse)
			assert.Equal(t, c.TimeIdx, timeIdx)
			assert.Equal(t, c.ValueIdxs, valueIdxs)
			assert.Equal(t, c.LabelIdxs, labelIdxs)
		})
	}
}

func Test_estimateValueSize(t *testing.T) {
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

			assert.Equal(t, c.Result, estimateValueSize(c.Value))
		})
	}
}
