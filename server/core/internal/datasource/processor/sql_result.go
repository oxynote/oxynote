package processor

import (
	"fmt"
	"maps"
	"strings"
)

// sqlQueryResult is the dialect-agnostic view of a SQL result set, so that
// turning rows into series is written once. The dialects differ only in how
// they read a timestamp and a number out of a value, which they hand over as
// functions.
type sqlQueryResult struct {
	// Columns contains the column names of the result set.
	Columns []string

	// Rows contains the result rows, each as a slice of values.
	Rows [][]any

	// ColumnTypes contains the database type of each column, when the
	// driver reported them. It is empty for dialects that do not, which
	// falls the classification back to the values' own shape.
	ColumnTypes []string
}

// transform turns the result set into a unified QueryResult based on the
// requested chart type.
//
// The SQL query is expected to return:
//   - A time column named "time" (as RFC3339 string or unix seconds)
//   - One or more numeric columns, each producing a separate series
//   - Any non-numeric, non-time columns are treated as series labels
//
// Each numeric column generates its own set of series. When there are multiple
// numeric columns, the column name is added as the "__name__" label to
// distinguish them.
func (sqr sqlQueryResult) transform( //nolint:gocognit // this method is complex, however, it's well-structured
	ct ChartType,
	parseTimestamp func(any) (int64, bool),
	parseNumeric func(any) (float64, bool),
) *QueryResult {
	if ct == "" {
		return &QueryResult{Status: QueryStatusTypeNotSelected}
	}

	if len(sqr.Columns) == 0 || len(sqr.Rows) == 0 {
		return &QueryResult{Status: QueryStatusNoData}
	}

	timeIdx, valueIdxs, labelIdxs := sqr.identifyColumns(parseNumeric)
	if timeIdx < 0 || len(valueIdxs) == 0 {
		return &QueryResult{Status: QueryStatusChartAndDataMismatch}
	}

	multipleValues := len(valueIdxs) > 1

	// group rows by (value column, label combination) into series,
	// preserving insertion order.
	seriesMap := make(map[string]*QueryResultSeries)

	var seriesOrder []string

	for _, row := range sqr.Rows {
		if len(row) <= timeIdx {
			continue
		}

		ts, ok := parseTimestamp(row[timeIdx])
		if !ok {
			continue
		}

		// build the label key once per row, shared across value columns.
		labels := make(map[string]string, len(labelIdxs))

		var labelKeyParts []string

		for _, li := range labelIdxs {
			var lbl string
			if li < len(row) && row[li] != nil {
				lbl = fmt.Sprintf("%v", row[li])
			}

			labels[sqr.Columns[li]] = lbl
			labelKeyParts = append(labelKeyParts, sqr.Columns[li]+"="+lbl)
		}

		labelKey := strings.Join(labelKeyParts, "\x00")

		for _, vi := range valueIdxs {
			if vi >= len(row) {
				continue
			}

			val, ok := parseNumeric(row[vi])
			if !ok || !isValidValue(val) {
				continue
			}

			key := sqr.Columns[vi] + "\x00" + labelKey

			if _, exists := seriesMap[key]; !exists {
				seriesLabels := make(map[string]string, len(labels)+1)
				maps.Copy(seriesLabels, labels)

				if multipleValues {
					seriesLabels["__name__"] = sqr.Columns[vi]
				}

				seriesMap[key] = &QueryResultSeries{
					Labels: seriesLabels,
				}
				seriesOrder = append(seriesOrder, key)
			}

			seriesMap[key].Metrics = append(seriesMap[key].Metrics, [2]any{ts, val})
		}
	}

	if len(seriesMap) == 0 {
		return &QueryResult{Status: QueryStatusNoData}
	}

	series := make([]QueryResultSeries, 0, len(seriesOrder))

	for _, key := range seriesOrder {
		s := seriesMap[key]

		// a gauge shows the present, so only the last value survives.
		if ct == ChartTypeGauge && len(s.Metrics) > 0 {
			s.Metrics = s.Metrics[len(s.Metrics)-1:]
		}

		series = append(series, *s)
	}

	return &QueryResult{
		Status: QueryStatusOK,
		Data:   series,
	}
}

// identifyColumns detects which columns are time, value (numeric), and
// labels (non-numeric).
func (sqr sqlQueryResult) identifyColumns(
	parseNumeric func(any) (float64, bool),
) (timeIdx int, valueIdxs, labelIdxs []int) {
	timeIdx = -1

	for i, col := range sqr.Columns {
		if strings.EqualFold(col, _timeColumn) {
			timeIdx = i
			break
		}
	}

	if len(sqr.Rows) == 0 {
		return timeIdx, valueIdxs, labelIdxs
	}

	for i := range sqr.Columns {
		if i == timeIdx {
			continue
		}

		if i < len(sqr.ColumnTypes) {
			if _sqlNumericTypes[strings.ToUpper(sqr.ColumnTypes[i])] {
				valueIdxs = append(valueIdxs, i)

				continue
			}

			labelIdxs = append(labelIdxs, i)

			continue
		}

		// without the declared types — a result assembled by hand rather
		// than scanned — the values' own shape is all there is to go on.
		if columnIsNumeric(sqr.Rows, i, parseNumeric) {
			valueIdxs = append(valueIdxs, i)

			continue
		}

		labelIdxs = append(labelIdxs, i)
	}

	return timeIdx, valueIdxs, labelIdxs
}

// estimateValueSize returns a rough byte-size estimate for a single value
// as it would be rendered into the JSON payload.
func estimateValueSize(v any) int {
	switch val := v.(type) {
	case nil:
		return _jsonNullSize
	case bool:
		return _jsonBoolSize
	case string:
		return len(val) + _jsonQuotesSize
	case []byte:
		return len(val)
	default:
		return _jsonNumericSize
	}
}
