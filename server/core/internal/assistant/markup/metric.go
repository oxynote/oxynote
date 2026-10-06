package markup

import (
	jsonv2 "encoding/json/v2"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/oxynote/oxynote/server/core/internal/document"
)

// Metric enum values. They mirror the editor's enums in
// web/app/components/editor/blocks/metrics/utils.ts (TimeRangePreset,
// RefreshInterval, the Visualization*Unit enums, MetricBlockWidth,
// MetricSimulationPreset) and web/app/utils/api/data-source/generic-query.ts
// (GenericQueryChartType); a value added there has to be added here or the
// assistant cannot author it.
var (
	// _metricVisualizationTypes are the chart kinds; they equal the
	// datasource processor's ChartType values, which only the tests
	// import, so this package stays free of the processors.
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

	// _metricSimulationPresets are the generated series a block draws
	// while the metric it documents has no real data to answer with.
	_metricSimulationPresets = []string{
		"cpu_usage", "memory_usage", "disk_usage",
		"http_requests", "http_latency", "error_rate",
	}

	// _metricEnums maps each enum-valued attribute to its values, which
	// is the table decodeMetricAttrs checks against and MetricEnums
	// publishes.
	_metricEnums = map[string][]string{
		document.AttrVisualizationType: _metricVisualizationTypes,
		document.AttrTimeRange:         _metricTimeRanges,
		document.AttrRefreshInterval:   _metricRefreshIntervals,
		document.AttrUnitType:          _metricUnitTypes,
		document.AttrWidth:             _metricWidths,
		document.AttrSimulationPreset:  _metricSimulationPresets,
	}
)

// MetricEnums returns every enum-valued metric attribute with its
// allowed values, keyed by attribute name.
func MetricEnums() map[string][]string {
	out := make(map[string][]string, len(_metricEnums))

	for k, v := range _metricEnums {
		out[k] = slices.Clone(v)
	}

	return out
}

// MetricReference describes the attributes a <metric> holds, its enum
// values taken from the validator so the two cannot drift apart.
func MetricReference() string {
	var sb strings.Builder

	sb.WriteString("dataSourceId (an id from list_data_sources), queries ([{name, query, legendFormat}], PromQL or SQL as the data source takes)")

	for _, key := range slices.Sorted(maps.Keys(_metricEnums)) {
		sb.WriteString(", " + key + " (" + strings.Join(_metricEnums[key], "|") + ")")
	}

	sb.WriteString(", and optionally title, unitCustom (the label when unitType is custom), decimals, thresholds ([{value, label, color}]), baseThresholdColor, axisBoundsMin, axisBoundsMax")

	return sb.String()
}

// metricAttrs holds the metric attributes that are not enums. Decoding
// a metric's JSON into it checks the type of each one.
type metricAttrs struct {
	// Title is the metric's heading.
	Title string `json:"title"`

	// DataSourceID names the data source the queries run against.
	DataSourceID string `json:"dataSourceId"`

	// UnitCustom is the unit label when unitType is custom.
	UnitCustom string `json:"unitCustom"`

	// BaseThresholdColor is the colour below the first threshold.
	BaseThresholdColor string `json:"baseThresholdColor"`

	// Decimals is how many decimals values show with.
	Decimals float64 `json:"decimals"`

	// AxisBoundsMin is the lower bound of the value axis.
	AxisBoundsMin float64 `json:"axisBoundsMin"`

	// AxisBoundsMax is the upper bound of the value axis.
	AxisBoundsMax float64 `json:"axisBoundsMax"`

	// Queries are the queries the metric draws.
	Queries []metricQuery `json:"queries"`

	// Thresholds are the values the chart marks.
	Thresholds []metricThreshold `json:"thresholds"`
}

// metricQuery is one row of a metric's queries.
type metricQuery struct {
	// Name is the query's display name. Required.
	Name *string `json:"name"`

	// Query is the query text. Required.
	Query *string `json:"query"`

	// LegendFormat is the legend template.
	LegendFormat string `json:"legendFormat"`
}

// metricThreshold is one row of a metric's thresholds.
type metricThreshold struct {
	// Value is where the threshold sits.
	Value float64 `json:"value"`

	// Label names the threshold.
	Label string `json:"label"`

	// Color is the colour above it.
	Color string `json:"color"`
}

// decodeMetricAttrs reads a metric's JSON attributes and checks them:
// every enum holds one of its values, every other attribute has its
// type, and every query has a name and a query.
func decodeMetricAttrs(text string) (document.Attributes, error) {
	attrs := document.Attributes{}

	if strings.TrimSpace(text) == "" {
		return attrs, nil
	}

	if err := jsonv2.Unmarshal([]byte(text), &attrs); err != nil {
		return nil, fmt.Errorf("<metric> holds a JSON object of its attributes: %w", err)
	}

	for _, key := range slices.Sorted(maps.Keys(_metricEnums)) {
		v, ok := attrs.Value(key)
		if !ok {
			continue
		}

		if s, isString := v.(string); !isString || !slices.Contains(_metricEnums[key], s) {
			return nil, fmt.Errorf("metric %s must be one of: %s", key, strings.Join(_metricEnums[key], ", "))
		}
	}

	var typed metricAttrs

	if err := jsonv2.Unmarshal([]byte(text), &typed); err != nil {
		return nil, fmt.Errorf("metric attributes: %w", err)
	}

	for i, q := range typed.Queries {
		if q.Name == nil || q.Query == nil {
			return nil, fmt.Errorf("metric query %d: name and query are required", i+1)
		}
	}

	return attrs, nil
}

// setSimulation sets the simulation flag of a metric built from markup.
// A metric that keeps its stored preset keeps its stored flag, so one
// re-sent after its data arrived does not simulate again; otherwise the
// flag is on exactly when a preset is named. stored is the metric the
// element's id names, or the zero block.
func setSimulation(attrs document.Attributes, stored document.Block) {
	preset, named := attrs.Value(document.AttrSimulationPreset)

	if stored.Type == document.BlockNodeMetricBlock && named &&
		preset == stored.Attrs[document.AttrSimulationPreset] {
		if active, set := stored.Attrs.Value(document.AttrSimulationActive); set {
			attrs[document.AttrSimulationActive] = active
		}

		return
	}

	if named {
		attrs[document.AttrSimulationActive] = true
	}
}
