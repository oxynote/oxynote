package block

import (
	"fmt"
	"slices"
	"strings"

	"github.com/oxynote/oxynote/server/core/internal/document"
)

// Metric query-row field names. They name the fields inside the
// queries array a metric block carries, not the block's own attributes,
// so they stay with the validation that reads them.
const (
	// _metricQueryName is the display name of one query row.
	_metricQueryName = "name"

	// _metricQueryQuery is the query text of one query row.
	_metricQueryQuery = "query"

	// _metricQueryLegendFormat is the optional legend template of one
	// query row.
	_metricQueryLegendFormat = "legendFormat"
)

// Metric enum values. They mirror the editor's enums in
// web/app/components/editor/blocks/metrics/utils.ts (TimeRangePreset,
// RefreshInterval, the Visualization*Unit enums, MetricBlockWidth) and
// web/app/utils/api/data-source/generic-query.ts (GenericQueryChartType);
// a value added there has to be added here or the assistant cannot author
// it.
var (
	// _metricVisualizationTypes are the chart kinds; they equal the
	// datasource processor's ChartType values, which the tools package
	// pins with a test since this package must not import it.
	_metricVisualizationTypes = []string{"line_chart", "bar_chart", "gauge_chart"}

	// _metricTimeRanges are the time-window presets.
	_metricTimeRanges = []string{
		"last_5_minutes", "last_15_minutes", "last_30_minutes",
		"last_1_hour", "last_3_hours", "last_6_hours", "last_12_hours", "last_24_hours",
		"last_2_days", "last_7_days", "last_30_days", "last_90_days",
		"last_6_months", "last_1_year", "last_2_years", "last_5_years",
		"today", "yesterday", "today_so_far",
		"this_week", "this_week_so_far", "this_month", "this_month_so_far",
		"this_year", "this_year_so_far",
		"previous_week", "previous_month", "previous_year",
	}

	// _metricRefreshIntervals are the re-query periods.
	_metricRefreshIntervals = []string{"5s", "10s", "30s", "1m", "5m", "15m", "30m", "1h", "2h", "1d"}

	// _metricUnitTypes are the value units: custom, time, data and percent.
	_metricUnitTypes = []string{
		"custom",
		"nanoseconds", "microseconds", "milliseconds", "seconds", "minutes", "hours", "days",
		"bytes", "kilobytes", "megabytes", "gigabytes", "terabytes",
		"bits", "kilobits", "megabits", "gigabits", "terabits",
		"percent0to100", "percent0to1",
	}

	// _metricWidths are the block widths inside a metric_grid.
	_metricWidths = []string{"compact", "standard", "wide"}

	// _metricEnums maps each enum-valued attribute to its values, which
	// is the table validateMetric checks against and MetricEnums
	// publishes.
	_metricEnums = map[string][]string{
		document.AttrVisualizationType: _metricVisualizationTypes,
		document.AttrTimeRange:         _metricTimeRanges,
		document.AttrRefreshInterval:   _metricRefreshIntervals,
		document.AttrUnitType:          _metricUnitTypes,
		document.AttrWidth:             _metricWidths,
	}
)

// MetricEnums returns every enum-valued metric attribute with its
// allowed values, keyed by attribute name. The prompt pins itself to it
// so the model is told exactly the values Validate accepts, and nothing
// else.
func MetricEnums() map[string][]string {
	out := make(map[string][]string, len(_metricEnums))

	for k, v := range _metricEnums {
		out[k] = slices.Clone(v)
	}

	return out
}

// validateMetric checks a metric block: no content, and every known
// attribute it does carry is well-formed. Only present, non-null values
// are checked — the editor stores null for "unset" and an older block
// may carry attributes this layer does not name — so a block read back
// and re-sent always passes.
func validateMetric(b Block, path string) error {
	if err := validateContentless(b, path); err != nil {
		return err
	}

	for _, key := range []string{
		document.AttrVisualizationType,
		document.AttrTimeRange,
		document.AttrRefreshInterval,
		document.AttrUnitType,
		document.AttrWidth,
	} {
		if err := validateMetricEnum(b.Attrs, key, path); err != nil {
			return err
		}
	}

	for _, key := range []string{
		document.AttrTitle,
		document.AttrDataSourceID,
		document.AttrUnitCustom,
		document.AttrBaseThresholdColor,
		document.AttrSimulationPreset,
	} {
		if err := validateMetricString(b.Attrs, key, path); err != nil {
			return err
		}
	}

	for _, key := range []string{
		document.AttrDecimals,
		document.AttrAxisBoundsMin,
		document.AttrAxisBoundsMax,
	} {
		if err := validateMetricNumber(b.Attrs, key, path); err != nil {
			return err
		}
	}

	if err := validateMetricQueries(b.Attrs, path); err != nil {
		return err
	}

	return validateMetricThresholds(b.Attrs, path)
}

// validateMetricEnum checks that the named attribute, when set, is one
// of its allowed values.
func validateMetricEnum(attrs document.Attributes, key, path string) error {
	v, ok := attrs.Value(key)
	if !ok {
		return nil
	}

	s, isString := v.(string)
	if !isString || !slices.Contains(_metricEnums[key], s) {
		return verr(
			joinPath(path, "attrs."+key),
			fmt.Sprintf("metric %s must be one of: %s", key, strings.Join(_metricEnums[key], ", ")),
		)
	}

	return nil
}

// validateMetricString checks that the named attribute, when set, is a
// string.
func validateMetricString(attrs document.Attributes, key, path string) error {
	v, ok := attrs.Value(key)
	if !ok {
		return nil
	}

	if _, isString := v.(string); !isString {
		return verr(joinPath(path, "attrs."+key), fmt.Sprintf("metric %s must be a string", key))
	}

	return nil
}

// validateMetricNumber checks that the named attribute, when set, is a
// number.
func validateMetricNumber(attrs document.Attributes, key, path string) error {
	v, ok := attrs.Value(key)
	if !ok {
		return nil
	}

	if !isNumber(v) {
		return verr(joinPath(path, "attrs."+key), fmt.Sprintf("metric %s must be a number", key))
	}

	return nil
}

// validateMetricQueries checks that queries, when set, is an array of
// objects each carrying a string name and query, and an optional string
// legendFormat.
func validateMetricQueries(attrs document.Attributes, path string) error {
	v, ok := attrs.Value(document.AttrQueries)
	if !ok {
		return nil
	}

	rows, err := objectRows(v)
	if err != nil {
		return verr(joinPath(path, "attrs."+document.AttrQueries), "metric queries must be an array of {name, query, legendFormat} objects")
	}

	for i, row := range rows {
		rowPath := joinPath(path, fmt.Sprintf("attrs.%s[%d]", document.AttrQueries, i))

		for _, key := range []string{_metricQueryName, _metricQueryQuery} {
			if _, isString := row[key].(string); !isString {
				return verr(rowPath, fmt.Sprintf("metric query %s must be a string", key))
			}
		}

		if lf, present := row.Value(_metricQueryLegendFormat); present {
			if _, isString := lf.(string); !isString {
				return verr(rowPath, "metric query legendFormat must be a string")
			}
		}
	}

	return nil
}

// validateMetricThresholds checks that thresholds, when set, is an array
// of objects whose value, when set, is a number and whose label and
// color, when set, are strings.
func validateMetricThresholds(attrs document.Attributes, path string) error {
	v, ok := attrs.Value(document.AttrThresholds)
	if !ok {
		return nil
	}

	rows, err := objectRows(v)
	if err != nil {
		return verr(joinPath(path, "attrs."+document.AttrThresholds), "metric thresholds must be an array of {value, label, color} objects")
	}

	for i, row := range rows {
		rowPath := joinPath(path, fmt.Sprintf("attrs.%s[%d]", document.AttrThresholds, i))

		if val, present := row.Value("value"); present && !isNumber(val) {
			return verr(rowPath, "metric threshold value must be a number")
		}

		for _, key := range []string{"label", "color"} {
			if s, present := row.Value(key); present {
				if _, isString := s.(string); !isString {
					return verr(rowPath, fmt.Sprintf("metric threshold %s must be a string", key))
				}
			}
		}
	}

	return nil
}

// objectRows reads an attribute value as a list of objects.
//
// The value reaches this either from the model's JSON or from the
// content column, and both decode an array of objects to []any of
// map[string]any, so that is the only shape there is to read.
func objectRows(v any) ([]document.Attributes, error) {
	rows, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("value is %T, not an array", v)
	}

	out := make([]document.Attributes, 0, len(rows))

	for _, r := range rows {
		m, isObject := r.(map[string]any)
		if !isObject {
			return nil, fmt.Errorf("element is %T, not an object", r)
		}

		out = append(out, m)
	}

	return out, nil
}

// isNumber reports whether v is one of the numeric types a decoded JSON
// payload or a hand-built attribute map can carry.
func isNumber(v any) bool {
	switch v.(type) {
	case float64, float32, int, int64, int32:
		return true
	default:
		return false
	}
}
