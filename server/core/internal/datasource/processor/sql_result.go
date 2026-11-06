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

	// Fill specifies the fill mode of the query's group macro, or nil when
	// it has none.
	Fill *sqlFill
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
//
// With a fill mode every series gets a point on each bucket of the range.
func (sqr sqlQueryResult) transform( //nolint:gocognit // this method is complex, however, it's well-structured
	ct ChartType,
	parseTimestamp func(any) (int64, bool),
	parseNumeric func(any) (float64, bool),
) (*QueryResult, error) {
	if ct == "" {
		return &QueryResult{Status: QueryStatusTypeNotSelected}, nil
	}

	if len(sqr.Columns) == 0 || len(sqr.Rows) == 0 {
		return &QueryResult{Status: QueryStatusNoData}, nil
	}

	timeIdx, valueIdxs, labelIdxs := sqr.identifyColumns(parseNumeric)
	if timeIdx < 0 || len(valueIdxs) == 0 {
		return &QueryResult{Status: QueryStatusChartAndDataMismatch}, nil
	}

	multipleValues := len(valueIdxs) > 1

	// group rows by (value column, label combination) into series,
	// preserving insertion order.
	var (
		seriesMap   = make(map[string]*QueryResultSeries)
		points      = make(map[string][]sqlPoint)
		seriesOrder []string
	)

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

			points[key] = append(points[key], sqlPoint{time: ts, value: val})
		}
	}

	if len(seriesMap) == 0 {
		return &QueryResult{Status: QueryStatusNoData}, nil
	}

	if sqr.Fill != nil && int64(len(seriesOrder))*sqr.Fill.buckets() > _maxFilledPoints {
		return nil, NewInvalidQueryError(fmt.Sprintf("the fill gives more than %d points; use a longer interval", _maxFilledPoints))
	}

	series := make([]QueryResultSeries, 0, len(seriesOrder))

	for _, key := range seriesOrder {
		s := seriesMap[key]
		s.Metrics = sqr.metrics(points[key], ct)
		series = append(series, *s)
	}

	return &QueryResult{
		Status: QueryStatusOK,
		Data:   series,
	}, nil
}

// metrics returns the points of one series as chart type ct draws them,
// on the fill's buckets when there is a fill.
func (sqr sqlQueryResult) metrics(points []sqlPoint, ct ChartType) [][2]any {
	var out [][2]any

	if sqr.Fill != nil {
		out = sqr.Fill.apply(points)
	} else {
		for _, p := range points {
			out = append(out, [2]any{p.time, p.value})
		}
	}

	if ct != ChartTypeGauge {
		return out
	}

	// a gauge shows the present, so only the last value survives. A null
	// one is skipped, because it has no value to show.
	n := len(out)
	for n > 0 && out[n-1][1] == nil {
		n--
	}

	return out[max(n-1, 0):n]
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
