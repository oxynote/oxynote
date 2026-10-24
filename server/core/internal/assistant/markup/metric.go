package markup

import (
	jsonv2 "encoding/json/v2"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/oxynote/oxynote/server/core/internal/document"
	tagCore "github.com/oxynote/oxynote/server/core/internal/tag"
	"github.com/oxynote/oxynote/server/core/internal/variable"
)

// Metric enum values. They mirror the editor's enums in
// web/app/components/editor/blocks/metrics/utils.ts (TimeRangePreset,
// RefreshInterval, the Visualization*Unit enums, MetricBlockWidth,
// MetricSimulationPreset) and web/app/utils/api/data-source/generic-query.ts
// (GenericQueryChartType); a value added there has to be added here, or to
// the document package for time ranges and refresh intervals, or the
// assistant cannot author it.
var (
	// _metricVisualizationTypes are the chart kinds; they equal the
	// datasource processor's ChartType values, which only the tests
	// import, so this package stays free of the processors.
	_metricVisualizationTypes = []string{"line_chart", "bar_chart", "gauge_chart"}

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

	// _metricEnums maps each enum-valued attribute to its presets,
	// without the built-in variable references. metricValues adds those
	// for the decoder.
	_metricEnums = map[string][]string{
		document.AttrVisualizationType: _metricVisualizationTypes,
		document.AttrTimeRange:         document.MetricTimeRanges(),
		document.AttrRefreshInterval:   document.MetricRefreshIntervals(),
		document.AttrUnitType:          _metricUnitTypes,
		document.AttrWidth:             _metricWidths,
		document.AttrSimulationPreset:  _metricSimulationPresets,
	}

	// _metricReferences maps each attribute that can follow a built-in
	// variable to the reference it holds then.
	_metricReferences = map[string]string{
		document.AttrTimeRange:       "$" + variable.KeyMetricRange,
		document.AttrRefreshInterval: "$" + variable.KeyMetricRefresh,
	}
)

// MetricEnums returns every enum-valued metric attribute with its
// presets, keyed by attribute name. The built-in variable references
// the decoder also accepts are left out.
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
		sb.WriteString(", " + key + " (" + strings.Join(metricValues(key), "|") + ")")
	}

	sb.WriteString(", and optionally title, unitCustom (the label when unitType is custom), decimals, thresholds ([{value, label, color}]), baseThresholdColor, axisBoundsMin, axisBoundsMax. " +
		"A threshold colour and baseThresholdColor are one of " + strings.Join(tagCore.ColorNames(), "|") + ". " +
		"timeRange and refreshInterval default to " + _metricReferences[document.AttrTimeRange] + " and " + _metricReferences[document.AttrRefreshInterval] +
		", the built-in variables each user sets; give a preset only to pin one chart, and the reference to unpin it. " +
		"A <metrics> row fits 3 compact, 2 standard or 1 wide, never a standard beside a compact: fill every row, with the text about the charts before the <metrics>")

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

	// BaseThresholdColor is the colour below the first threshold, a
	// palette name.
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

	// Color is the colour above it, a palette name.
	Color string `json:"color"`
}

// decodeMetricAttrs reads a metric's JSON attributes and checks them:
// every enum holds one of its values, every other attribute has its
// type, every query has a name and a query, and every threshold colour
// is a palette name. A colour the stored metric already carries passes
// too, since one saved before thresholds stored names holds css and a
// re-sent chart keeps it. stored is the metric the element's id names,
// or the zero block.
func decodeMetricAttrs(text string, stored document.Block) (document.Attributes, error) {
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

		if s, isString := v.(string); !isString || !slices.Contains(metricValues(key), s) {
			return nil, fmt.Errorf("metric %s must be one of: %s", key, strings.Join(metricValues(key), ", "))
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

	colors := append(tagCore.ColorNames(), storedMetricColors(stored)...)

	if c := typed.BaseThresholdColor; c != "" && !slices.Contains(colors, c) {
		return nil, fmt.Errorf("metric baseThresholdColor must be one of: %s", strings.Join(tagCore.ColorNames(), ", "))
	}

	for i, th := range typed.Thresholds {
		if th.Color != "" && !slices.Contains(colors, th.Color) {
			return nil, fmt.Errorf("metric threshold %d: color must be one of: %s", i+1, strings.Join(tagCore.ColorNames(), ", "))
		}
	}

	return attrs, nil
}

// storedMetricColors returns every colour the stored metric carries: the
// base threshold colour and each threshold's.
func storedMetricColors(stored document.Block) []string {
	out := []string{stored.Attrs.Get("baseThresholdColor").String()}

	rows, ok := stored.Attrs["thresholds"].([]any)
	if !ok {
		return out
	}

	for _, row := range rows {
		if r, isRow := row.(map[string]any); isRow {
			out = append(out, document.Attributes(r).Get("color").String())
		}
	}

	return out
}

// metricValues returns every value the attribute takes: its enum and,
// when it can follow a built-in variable, the reference to it.
func metricValues(key string) []string {
	ref, ok := _metricReferences[key]
	if !ok {
		return _metricEnums[key]
	}

	return append([]string{ref}, _metricEnums[key]...)
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

// setMetricWindow fills the time range and refresh interval the element
// leaves out. A metric the id names keeps what it stores, so a re-sent
// chart keeps its window; a new one follows the built-in variables.
// stored is the metric the element's id names, or the zero block.
func setMetricWindow(attrs document.Attributes, stored document.Block) {
	isNew := stored.Type != document.BlockNodeMetricBlock

	for key, ref := range _metricReferences {
		if _, set := attrs.Value(key); set {
			continue
		}

		if isNew {
			attrs[key] = ref

			continue
		}

		if v, kept := stored.Attrs.Value(key); kept {
			attrs[key] = v
		}
	}
}
