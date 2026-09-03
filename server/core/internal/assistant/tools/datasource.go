package tools

import (
	"errors"
	"fmt"
	"maps"
	"time"

	"github.com/oxynote/oxynote/server/core/internal/datasource"
	"github.com/oxynote/oxynote/server/core/internal/datasource/processor"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
	"github.com/rs/xid"
)

// _defaultQueryWindow is the window a data-source tool reads when the
// model names neither end of the range. An hour is what the metric
// block's own default preset covers.
const _defaultQueryWindow = time.Hour

// Shared property names and descriptions for the data-source tools.
const (
	// _keyDataSourceID is the shared data-source-id property name.
	_keyDataSourceID = "data_source_id"

	// _descDataSourceID describes the data-source-id property.
	_descDataSourceID = "The data source id, from list_data_sources."

	// _descFrom describes the range-start property.
	_descFrom = "Optional. Range start as an RFC3339 timestamp. Defaults to an hour before 'to'."

	// _descTo describes the range-end property.
	_descTo = "Optional. Range end as an RFC3339 timestamp. Defaults to now."

	// _descMatchers describes the Prometheus series-selector property.
	_descMatchers = "PromQL series selectors, e.g. [\"up\", \"{job=\\\"api\\\"}\"]."

	// _descChartType describes the optional chart-type property shared
	// by the two query tools.
	_descChartType = "Optional. One of line_chart, bar_chart, gauge_chart. When set, the result describes what the metric block would draw (render status, series count, and each series' labels, point count and endpoints) instead of the raw data. Use it to check a query before putting it in a metric block; omit it when you need the values themselves."
)

// errUnknownDataSource is what a lookup reports for an id that names
// nothing in the session's organisation. Another organisation's id
// lands here too, which is the point: the tools cannot be used to
// discover that a data source exists elsewhere.
var errUnknownDataSource = errors.New("no data source with that id in this organisation; call list_data_sources for the ids that exist")

// errInvertedTimeRange reports a range whose start falls after its end,
// once either absent end has been defaulted.
var errInvertedTimeRange = errors.New("'from' is after 'to'; the range start must be the earlier timestamp")

// timeRangeArgs is the range every data-source read can be narrowed
// with. Both ends are optional: the tools serve a model that usually
// means "recently" and should not have to compute timestamps to say so.
type timeRangeArgs struct {
	// From is the range start. Zero means an hour before the end.
	From time.Time `json:"from"`

	// To is the range end. Zero means now.
	To time.Time `json:"to"`
}

// resolve turns the pair into a time range, defaulting the end to now
// and the start to an hour before it. An inverted range is rejected
// here rather than handed to a backend, so the model is told the
// arguments are backwards instead of seeing a backend-shaped failure.
func (a timeRangeArgs) resolve() (processor.TimeRange, error) {
	out := processor.TimeRange{From: a.From, To: a.To}

	if out.To.IsZero() {
		out.To = timeutil.Now()
	}

	if out.From.IsZero() {
		out.From = out.To.Add(-_defaultQueryWindow)
	}

	if out.From.After(out.To) {
		return processor.TimeRange{}, errInvertedTimeRange
	}

	return out, nil
}

// dataSourceProps builds a data-source tool's schema from the shared
// data-source-id property plus whatever else the tool takes.
func dataSourceProps(extra map[string]any) map[string]any {
	out := map[string]any{_keyDataSourceID: stringProp(_descDataSourceID)}

	maps.Copy(out, extra)

	return out
}

// dataSourceInfo is one row of list_data_sources. It is deliberately
// narrower than the stored data source: the URL and the credentials
// never reach the model.
type dataSourceInfo struct {
	// ID addresses the data source in every other data-source tool.
	ID xid.ID `json:"id"`

	// Name is the data source's display name.
	Name string `json:"name"`

	// Type is what the data source speaks (prometheus, postgresql,
	// mariadb, mysql), which decides the tools that serve it.
	Type datasource.Type `json:"type"`

	// Status is the connection status recorded for it.
	Status processor.ConnectionStatus `json:"status"`
}

// dataSourcesResult is what list_data_sources returns.
type dataSourcesResult struct {
	// DataSources are the organisation's data sources.
	DataSources []dataSourceInfo `json:"data_sources"`
}

// sqlQueryLabelsResult is what get_sql_query_labels returns.
type sqlQueryLabelsResult struct {
	// Labels maps each string column the query returned to an example
	// value from its first row.
	Labels map[string]string `json:"labels"`
}

// listDataSources lists the organisation's data sources.
type listDataSources struct {
	plainSummary
}

// Info returns the tool's model-facing description.
func (listDataSources) Info() Info {
	return Info{
		Name:        NameListDataSources,
		Description: "List the organisation's data sources. Returns id, name, type (prometheus, postgresql, mariadb, mysql) and connection status. Start here: every other data-source tool takes an id from this list, and the type decides which of them serves it.",
		Properties:  map[string]any{},
	}
}

// Traits reports a plain read.
func (listDataSources) Traits() Traits {
	return Traits{DataSource: true}
}

// Title returns no status line: listing is too generic to announce.
func (listDataSources) Title(_ DescribeInput) (string, error) {
	return "", nil
}

// Execute lists the data sources the organisation owns.
func (listDataSources) Execute(inp Input) (string, error) {
	sources, err := inp.DataSources()
	if err != nil {
		return "", fmt.Errorf("%s: %w", NameListDataSources, err)
	}

	out := make([]dataSourceInfo, 0, len(sources))

	for _, ds := range sources {
		out = append(out, dataSourceInfo{
			ID:     ds.ID,
			Name:   ds.Name,
			Type:   ds.Type,
			Status: ds.Status,
		})
	}

	return result(dataSourcesResult{
		DataSources: out,
	})
}

// getPrometheusMetadataArgs is what get_prometheus_metadata is called
// with.
type getPrometheusMetadataArgs struct {
	// DataSourceID names the Prometheus data source.
	DataSourceID xid.ID `json:"data_source_id"`
}

// Validate checks the arguments are complete.
func (a getPrometheusMetadataArgs) Validate() error {
	if a.DataSourceID.IsNil() {
		return errRequired(_keyDataSourceID)
	}

	return nil
}

// getPrometheusMetadata lists the metrics a Prometheus data source
// exposes.
type getPrometheusMetadata struct {
	plainSummary
}

// Info returns the tool's model-facing description.
func (getPrometheusMetadata) Info() Info {
	return Info{
		Name:        NameGetPrometheusMetadata,
		Description: "List the metrics a Prometheus data source exposes, with each metric's type, help text and unit. Use it to find the metric names to write a PromQL query against instead of guessing them.",
		Properties:  dataSourceProps(nil),
		Required:    []string{_keyDataSourceID},
	}
}

// Traits reports a plain read.
func (getPrometheusMetadata) Traits() Traits {
	return Traits{DataSource: true}
}

// Title announces the data source being read.
func (getPrometheusMetadata) Title(inp DescribeInput) (string, error) {
	var in getPrometheusMetadataArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	ds, err := inp.DataSource(in.DataSourceID)
	if err != nil {
		return "", fmt.Errorf("%s: %w", NameGetPrometheusMetadata, err)
	}

	return fmt.Sprintf("Reading metric metadata of %q", ds.Name), nil
}

// Execute fetches the data source's metric metadata.
func (getPrometheusMetadata) Execute(inp Input) (string, error) {
	var in getPrometheusMetadataArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	runner, err := inp.DataSourceRunner(in.DataSourceID)
	if err != nil {
		return "", fmt.Errorf("%s: %w", NameGetPrometheusMetadata, err)
	}

	prom, err := runner.Prometheus(inp.Context())
	if err != nil {
		return "", fmt.Errorf("%s: %w", NameGetPrometheusMetadata, err)
	}

	res, err := prom.Metadata(inp.Context())
	if err != nil {
		return "", fmt.Errorf("%s: %w", NameGetPrometheusMetadata, err)
	}

	return result(res)
}

// prometheusLabelNamesArgs is what list_prometheus_label_names is
// called with.
type prometheusLabelNamesArgs struct {
	timeRangeArgs

	// DataSourceID names the Prometheus data source.
	DataSourceID xid.ID `json:"data_source_id"`

	// Matchers narrows the label names to the series they select.
	Matchers []string `json:"matchers"`
}

// Validate checks the arguments are complete.
func (a prometheusLabelNamesArgs) Validate() error {
	if a.DataSourceID.IsNil() {
		return errRequired(_keyDataSourceID)
	}

	return nil
}

// listPrometheusLabelNames lists the label names present in a
// Prometheus data source.
type listPrometheusLabelNames struct {
	plainSummary
}

// Info returns the tool's model-facing description.
func (listPrometheusLabelNames) Info() Info {
	return Info{
		Name:        NameListPrometheusLabelNames,
		Description: "List the label names present in a Prometheus data source, optionally only those on the series the matchers select. Pair it with list_prometheus_label_values to build a filtered PromQL query.",
		Properties: dataSourceProps(map[string]any{
			_keyMatchers: map[string]any{
				_keyType:        _typeArray,
				_keyDescription: "Optional. " + _descMatchers,
				_keyItems: map[string]any{
					_keyType: _typeString,
				},
			},
			_keyFrom: stringProp(_descFrom),
			_keyTo:   stringProp(_descTo),
		}),
		Required: []string{_keyDataSourceID},
	}
}

// Traits reports a plain read.
func (listPrometheusLabelNames) Traits() Traits {
	return Traits{DataSource: true}
}

// Title announces the data source being read.
func (listPrometheusLabelNames) Title(inp DescribeInput) (string, error) {
	var in prometheusLabelNamesArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	ds, err := inp.DataSource(in.DataSourceID)
	if err != nil {
		return "", fmt.Errorf("%s: %w", NameListPrometheusLabelNames, err)
	}

	return fmt.Sprintf("Listing label names of %q", ds.Name), nil
}

// Execute fetches the data source's label names.
func (listPrometheusLabelNames) Execute(inp Input) (string, error) {
	var in prometheusLabelNamesArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	tr, err := in.resolve()
	if err != nil {
		return "", fmt.Errorf("%s: %w", NameListPrometheusLabelNames, err)
	}

	runner, err := inp.DataSourceRunner(in.DataSourceID)
	if err != nil {
		return "", fmt.Errorf("%s: %w", NameListPrometheusLabelNames, err)
	}

	prom, err := runner.Prometheus(inp.Context())
	if err != nil {
		return "", fmt.Errorf("%s: %w", NameListPrometheusLabelNames, err)
	}

	res, err := prom.LabelNames(inp.Context(), in.Matchers, tr)
	if err != nil {
		return "", fmt.Errorf("%s: %w", NameListPrometheusLabelNames, err)
	}

	return result(res)
}

// prometheusLabelValuesArgs is what list_prometheus_label_values is
// called with.
type prometheusLabelValuesArgs struct {
	timeRangeArgs

	// DataSourceID names the Prometheus data source.
	DataSourceID xid.ID `json:"data_source_id"`

	// Label is the label whose values are being listed. Required.
	Label string `json:"label"`

	// Matchers narrows the values to the series they select.
	Matchers []string `json:"matchers"`
}

// Validate checks the arguments are complete.
func (a prometheusLabelValuesArgs) Validate() error {
	if a.DataSourceID.IsNil() {
		return errRequired(_keyDataSourceID)
	}

	if a.Label == "" {
		return errRequired(_keyLabel)
	}

	return nil
}

// listPrometheusLabelValues lists the values one label takes in a
// Prometheus data source.
type listPrometheusLabelValues struct {
	plainSummary
}

// Info returns the tool's model-facing description.
func (listPrometheusLabelValues) Info() Info {
	return Info{
		Name:        NameListPrometheusLabelValues,
		Description: "List the values a label takes in a Prometheus data source, optionally only on the series the matchers select. Use it to discover the concrete label values a query should filter on.",
		Properties: dataSourceProps(map[string]any{
			_keyLabel: stringProp("The label whose values to list, e.g. \"job\"."),
			_keyMatchers: map[string]any{
				_keyType:        _typeArray,
				_keyDescription: "Optional. " + _descMatchers,
				_keyItems: map[string]any{
					_keyType: _typeString,
				},
			},
			_keyFrom: stringProp(_descFrom),
			_keyTo:   stringProp(_descTo),
		}),
		Required: []string{_keyDataSourceID, _keyLabel},
	}
}

// Traits reports a plain read.
func (listPrometheusLabelValues) Traits() Traits {
	return Traits{DataSource: true}
}

// Title announces the label and the data source being read.
func (listPrometheusLabelValues) Title(inp DescribeInput) (string, error) {
	var in prometheusLabelValuesArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	ds, err := inp.DataSource(in.DataSourceID)
	if err != nil {
		return "", fmt.Errorf("%s: %w", NameListPrometheusLabelValues, err)
	}

	return fmt.Sprintf("Listing values of label %q in %q", in.Label, ds.Name), nil
}

// Execute fetches the label's values.
func (listPrometheusLabelValues) Execute(inp Input) (string, error) {
	var in prometheusLabelValuesArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	tr, err := in.resolve()
	if err != nil {
		return "", fmt.Errorf("%s: %w", NameListPrometheusLabelValues, err)
	}

	runner, err := inp.DataSourceRunner(in.DataSourceID)
	if err != nil {
		return "", fmt.Errorf("%s: %w", NameListPrometheusLabelValues, err)
	}

	prom, err := runner.Prometheus(inp.Context())
	if err != nil {
		return "", fmt.Errorf("%s: %w", NameListPrometheusLabelValues, err)
	}

	res, err := prom.LabelValues(inp.Context(), in.Label, in.Matchers, tr)
	if err != nil {
		return "", fmt.Errorf("%s: %w", NameListPrometheusLabelValues, err)
	}

	return result(res)
}

// prometheusSeriesArgs is what list_prometheus_series is called with.
type prometheusSeriesArgs struct {
	timeRangeArgs

	// DataSourceID names the Prometheus data source.
	DataSourceID xid.ID `json:"data_source_id"`

	// Matchers select the series to return. Required.
	Matchers []string `json:"matchers"`
}

// Validate checks the arguments are complete.
func (a prometheusSeriesArgs) Validate() error {
	if a.DataSourceID.IsNil() {
		return errRequired(_keyDataSourceID)
	}

	if len(a.Matchers) == 0 {
		return errRequired(_keyMatchers)
	}

	return nil
}

// listPrometheusSeries lists the series matching a set of selectors.
type listPrometheusSeries struct {
	plainSummary
}

// Info returns the tool's model-facing description.
func (listPrometheusSeries) Info() Info {
	return Info{
		Name:        NameListPrometheusSeries,
		Description: "List the series matching a set of selectors in a Prometheus data source, each as its full label set. Use it to see which label combinations a metric actually has before querying it.",
		Properties: dataSourceProps(map[string]any{
			_keyMatchers: map[string]any{
				_keyType:        _typeArray,
				_keyDescription: _descMatchers,
				_keyItems: map[string]any{
					_keyType: _typeString,
				},
			},
			_keyFrom: stringProp(_descFrom),
			_keyTo:   stringProp(_descTo),
		}),
		Required: []string{_keyDataSourceID, _keyMatchers},
	}
}

// Traits reports a plain read.
func (listPrometheusSeries) Traits() Traits {
	return Traits{DataSource: true}
}

// Title announces the data source being read.
func (listPrometheusSeries) Title(inp DescribeInput) (string, error) {
	var in prometheusSeriesArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	ds, err := inp.DataSource(in.DataSourceID)
	if err != nil {
		return "", fmt.Errorf("%s: %w", NameListPrometheusSeries, err)
	}

	return fmt.Sprintf("Listing series of %q", ds.Name), nil
}

// Execute fetches the matching series.
func (listPrometheusSeries) Execute(inp Input) (string, error) {
	var in prometheusSeriesArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	tr, err := in.resolve()
	if err != nil {
		return "", fmt.Errorf("%s: %w", NameListPrometheusSeries, err)
	}

	runner, err := inp.DataSourceRunner(in.DataSourceID)
	if err != nil {
		return "", fmt.Errorf("%s: %w", NameListPrometheusSeries, err)
	}

	prom, err := runner.Prometheus(inp.Context())
	if err != nil {
		return "", fmt.Errorf("%s: %w", NameListPrometheusSeries, err)
	}

	res, err := prom.Series(inp.Context(), in.Matchers, tr)
	if err != nil {
		return "", fmt.Errorf("%s: %w", NameListPrometheusSeries, err)
	}

	return result(res)
}

// queryPrometheusArgs is what query_prometheus is called with.
type queryPrometheusArgs struct {
	timeRangeArgs

	// DataSourceID names the Prometheus data source.
	DataSourceID xid.ID `json:"data_source_id"`

	// Query is the PromQL expression to run. Required.
	Query string `json:"query"`

	// ChartType, when set, asks for the transformed series a metric
	// block of that chart type would render instead of the raw result.
	ChartType processor.ChartType `json:"chart_type"`
}

// Validate checks the arguments are complete.
func (a queryPrometheusArgs) Validate() error {
	if a.DataSourceID.IsNil() {
		return errRequired(_keyDataSourceID)
	}

	if a.Query == "" {
		return errRequired(_keyQuery)
	}

	return nil
}

// queryPrometheus runs a PromQL range query.
type queryPrometheus struct {
	plainSummary
}

// Info returns the tool's model-facing description.
func (queryPrometheus) Info() Info {
	return Info{
		Name:        NameQueryPrometheus,
		Description: "Run a PromQL range query against a Prometheus data source over the given window. Returns the raw result by default; with chart_type set it instead describes what a metric block would render, which is how to check a query before writing it into a block.",
		Properties: dataSourceProps(map[string]any{
			_keyQuery:     stringProp("The PromQL expression to run."),
			_keyChartType: stringProp(_descChartType),
			_keyFrom:      stringProp(_descFrom),
			_keyTo:        stringProp(_descTo),
		}),
		Required: []string{_keyDataSourceID, _keyQuery},
	}
}

// Traits reports a plain read.
func (queryPrometheus) Traits() Traits {
	return Traits{DataSource: true}
}

// Title announces the data source being queried.
func (queryPrometheus) Title(inp DescribeInput) (string, error) {
	var in queryPrometheusArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	ds, err := inp.DataSource(in.DataSourceID)
	if err != nil {
		return "", fmt.Errorf("%s: %w", NameQueryPrometheus, err)
	}

	return fmt.Sprintf("Querying %q", ds.Name), nil
}

// Execute runs the query, transforming the result when a chart type
// was asked for.
func (queryPrometheus) Execute(inp Input) (string, error) {
	var in queryPrometheusArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	tr, err := in.resolve()
	if err != nil {
		return "", fmt.Errorf("%s: %w", NameQueryPrometheus, err)
	}

	runner, err := inp.DataSourceRunner(in.DataSourceID)
	if err != nil {
		return "", fmt.Errorf("%s: %w", NameQueryPrometheus, err)
	}

	prom, err := runner.Prometheus(inp.Context())
	if err != nil {
		return "", fmt.Errorf("%s: %w", NameQueryPrometheus, err)
	}

	res, err := prom.QueryRange(inp.Context(), in.Query, tr)
	if err != nil {
		return "", fmt.Errorf("%s: %w", NameQueryPrometheus, err)
	}

	if in.ChartType == "" {
		return result(res)
	}

	// a query that returned nothing has no result to transform, and the
	// metric block renders it as no-data — which is the answer the
	// model asked for by naming a chart type.
	if res == nil {
		return result(&processor.QueryResult{Status: processor.QueryStatusNoData})
	}

	return result(newChartPreview(res.Transform(in.ChartType)))
}

// _maxPreviewSeries caps how many series a chart check describes. A
// query behind a metric block draws a handful; one answering with more
// is already the wrong query, and listing them all would cost more than
// the check saves.
const _maxPreviewSeries = 10

// chartPreview is what a query answers with when a chart type was
// named. Naming one asks whether the query renders, not what it
// contains — so the shape of the answer is reported and the points
// behind it are not. A caller that wants the data omits chart_type and
// gets the raw result.
type chartPreview struct {
	// Status is the render outcome: whether the data fits the chart.
	Status processor.QueryStatus `json:"status"`

	// SeriesCount is how many series the chart would draw, including
	// any beyond the ones described.
	SeriesCount int `json:"series_count"`

	// Series describes the first few, each by its labels and extent.
	Series []chartPreviewSeries `json:"series,omitempty"`
}

// chartPreviewSeries is one series as a chart check reports it.
type chartPreviewSeries struct {
	// Labels are the series' labels, which become its legend entry.
	Labels map[string]string `json:"labels,omitempty"`

	// PointCount is how many points the series carries.
	PointCount int `json:"point_count"`

	// First and Last are its endpoints as [timestamp, value], enough
	// to see the series covers the window and holds real values.
	First [2]any `json:"first,omitempty"`
	Last  [2]any `json:"last,omitempty"`
}

// newChartPreview summarises a transformed result for a chart check.
func newChartPreview(qr *processor.QueryResult) chartPreview {
	if qr == nil {
		// NOCOV: Transform never returns nil; the guard keeps a future
		// caller from panicking on one.
		return chartPreview{Status: processor.QueryStatusNoData}
	}

	out := chartPreview{
		Status:      qr.Status,
		SeriesCount: len(qr.Data),
	}

	for _, sr := range qr.Data[:min(len(qr.Data), _maxPreviewSeries)] {
		row := chartPreviewSeries{
			Labels:     sr.Labels,
			PointCount: len(sr.Metrics),
		}

		if len(sr.Metrics) > 0 {
			row.First = sr.Metrics[0]
			row.Last = sr.Metrics[len(sr.Metrics)-1]
		}

		out.Series = append(out.Series, row)
	}

	return out
}

// getSQLMetadataArgs is what get_sql_metadata is called with.
type getSQLMetadataArgs struct {
	// DataSourceID names the SQL data source.
	DataSourceID xid.ID `json:"data_source_id"`
}

// Validate checks the arguments are complete.
func (a getSQLMetadataArgs) Validate() error {
	if a.DataSourceID.IsNil() {
		return errRequired(_keyDataSourceID)
	}

	return nil
}

// getSQLMetadata lists the tables and columns of a SQL data source.
type getSQLMetadata struct {
	plainSummary
}

// Info returns the tool's model-facing description.
func (getSQLMetadata) Info() Info {
	return Info{
		Name:        NameGetSQLMetadata,
		Description: "List the tables and their columns in a PostgreSQL, MariaDB or MySQL data source, plus the default schema. Read it before writing a query so the table and column names are the real ones.",
		Properties:  dataSourceProps(nil),
		Required:    []string{_keyDataSourceID},
	}
}

// Traits reports a plain read.
func (getSQLMetadata) Traits() Traits {
	return Traits{DataSource: true}
}

// Title announces the data source being read.
func (getSQLMetadata) Title(inp DescribeInput) (string, error) {
	var in getSQLMetadataArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	ds, err := inp.DataSource(in.DataSourceID)
	if err != nil {
		return "", fmt.Errorf("%s: %w", NameGetSQLMetadata, err)
	}

	return fmt.Sprintf("Reading tables of %q", ds.Name), nil
}

// Execute fetches the data source's tables and columns.
func (getSQLMetadata) Execute(inp Input) (string, error) {
	var in getSQLMetadataArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	runner, err := inp.DataSourceRunner(in.DataSourceID)
	if err != nil {
		return "", fmt.Errorf("%s: %w", NameGetSQLMetadata, err)
	}

	sql, err := runner.SQL(inp.Context())
	if err != nil {
		return "", fmt.Errorf("%s: %w", NameGetSQLMetadata, err)
	}

	res, err := sql.Metadata(inp.Context())
	if err != nil {
		return "", fmt.Errorf("%s: %w", NameGetSQLMetadata, err)
	}

	return result(res)
}

// sqlQueryLabelsArgs is what get_sql_query_labels is called with.
type sqlQueryLabelsArgs struct {
	timeRangeArgs

	// DataSourceID names the SQL data source.
	DataSourceID xid.ID `json:"data_source_id"`

	// Query is the SQL to probe. Required.
	Query string `json:"query"`
}

// Validate checks the arguments are complete.
func (a sqlQueryLabelsArgs) Validate() error {
	if a.DataSourceID.IsNil() {
		return errRequired(_keyDataSourceID)
	}

	if a.Query == "" {
		return errRequired(_keyQuery)
	}

	return nil
}

// getSQLQueryLabels probes a query for its string columns.
type getSQLQueryLabels struct {
	plainSummary
}

// Info returns the tool's model-facing description.
func (getSQLQueryLabels) Info() Info {
	return Info{
		Name:        NameGetSQLQueryLabels,
		Description: "Run a SQL query limited to one row and return its string columns with an example value each, which are the columns a chart would treat as series labels. Use it to check what a query returns before charting it; it is cheaper than query_sql.",
		Properties: dataSourceProps(map[string]any{
			_keyQuery: stringProp("The SQL query to probe. $__ macros are expanded as they are for a metric block."),
			_keyFrom:  stringProp(_descFrom),
			_keyTo:    stringProp(_descTo),
		}),
		Required: []string{_keyDataSourceID, _keyQuery},
	}
}

// Traits reports a plain read.
func (getSQLQueryLabels) Traits() Traits {
	return Traits{DataSource: true}
}

// Title announces the data source being probed.
func (getSQLQueryLabels) Title(inp DescribeInput) (string, error) {
	var in sqlQueryLabelsArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	ds, err := inp.DataSource(in.DataSourceID)
	if err != nil {
		return "", fmt.Errorf("%s: %w", NameGetSQLQueryLabels, err)
	}

	return fmt.Sprintf("Probing query labels of %q", ds.Name), nil
}

// Execute probes the query for its string columns.
func (getSQLQueryLabels) Execute(inp Input) (string, error) {
	var in sqlQueryLabelsArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	tr, err := in.resolve()
	if err != nil {
		return "", fmt.Errorf("%s: %w", NameGetSQLQueryLabels, err)
	}

	runner, err := inp.DataSourceRunner(in.DataSourceID)
	if err != nil {
		return "", fmt.Errorf("%s: %w", NameGetSQLQueryLabels, err)
	}

	sql, err := runner.SQL(inp.Context())
	if err != nil {
		return "", fmt.Errorf("%s: %w", NameGetSQLQueryLabels, err)
	}

	res, err := sql.QueryLabels(inp.Context(), in.Query, tr)
	if err != nil {
		return "", fmt.Errorf("%s: %w", NameGetSQLQueryLabels, err)
	}

	return result(sqlQueryLabelsResult{
		Labels: res,
	})
}

// querySQLArgs is what query_sql is called with.
type querySQLArgs struct {
	timeRangeArgs

	// DataSourceID names the SQL data source.
	DataSourceID xid.ID `json:"data_source_id"`

	// Query is the SQL to run. Required.
	Query string `json:"query"`

	// ChartType, when set, asks for the transformed series a metric
	// block of that chart type would render instead of the raw rows.
	ChartType processor.ChartType `json:"chart_type"`
}

// Validate checks the arguments are complete.
func (a querySQLArgs) Validate() error {
	if a.DataSourceID.IsNil() {
		return errRequired(_keyDataSourceID)
	}

	if a.Query == "" {
		return errRequired(_keyQuery)
	}

	return nil
}

// querySQL runs a query against a SQL data source.
type querySQL struct {
	plainSummary
}

// Info returns the tool's model-facing description.
func (querySQL) Info() Info {
	return Info{
		Name:        NameQuerySQL,
		Description: "Run a read-only query against a PostgreSQL, MariaDB or MySQL data source. Returns columns and rows by default, with the row count capped; with chart_type set it instead describes what a metric block would render, which is how to check a query before writing it into a block. $__ macros ($__timeFilter, $__timeGroupAlias and the rest) are expanded against the window.",
		Properties: dataSourceProps(map[string]any{
			_keyQuery:     stringProp("The SQL query to run. For a chart, select a time column aliased \"time\" plus one or more numeric columns."),
			_keyChartType: stringProp(_descChartType),
			_keyFrom:      stringProp(_descFrom),
			_keyTo:        stringProp(_descTo),
		}),
		Required: []string{_keyDataSourceID, _keyQuery},
	}
}

// Traits reports a plain read.
func (querySQL) Traits() Traits {
	return Traits{DataSource: true}
}

// Title announces the data source being queried.
func (querySQL) Title(inp DescribeInput) (string, error) {
	var in querySQLArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	ds, err := inp.DataSource(in.DataSourceID)
	if err != nil {
		return "", fmt.Errorf("%s: %w", NameQuerySQL, err)
	}

	return fmt.Sprintf("Querying %q", ds.Name), nil
}

// Execute runs the query against whichever SQL dialect the data source
// speaks, transforming the result when a chart type was asked for.
func (querySQL) Execute(inp Input) (string, error) {
	var in querySQLArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	tr, err := in.resolve()
	if err != nil {
		return "", fmt.Errorf("%s: %w", NameQuerySQL, err)
	}

	runner, err := inp.DataSourceRunner(in.DataSourceID)
	if err != nil {
		return "", fmt.Errorf("%s: %w", NameQuerySQL, err)
	}

	// the two dialects return different result shapes, so this is the
	// one place a tool has to know which one it is talking to.
	if runner.Type() == datasource.TypePostgreSQL {
		return runPostgreSQLQuery(inp, runner, in.Query, tr, in.ChartType)
	}

	return runMySQLQuery(inp, runner, in.Query, tr, in.ChartType)
}

// runPostgreSQLQuery serves query_sql for a PostgreSQL data source.
func runPostgreSQLQuery(
	inp Input,
	runner datasource.Runner,
	query string,
	tr processor.TimeRange,
	ct processor.ChartType,
) (string, error) {
	pg, err := runner.PostgreSQL(inp.Context())
	if err != nil {
		return "", fmt.Errorf("%s: %w", NameQuerySQL, err)
	}

	res, err := pg.Query(inp.Context(), query, tr)
	if err != nil {
		return "", fmt.Errorf("%s: %w", NameQuerySQL, err)
	}

	if ct == "" {
		return result(res)
	}

	if res == nil {
		return result(&processor.QueryResult{Status: processor.QueryStatusNoData})
	}

	return result(newChartPreview(res.Transform(ct)))
}

// runMySQLQuery serves query_sql for a MySQL or MariaDB data source.
func runMySQLQuery(
	inp Input,
	runner datasource.Runner,
	query string,
	tr processor.TimeRange,
	ct processor.ChartType,
) (string, error) {
	my, err := runner.MySQL(inp.Context())
	if err != nil {
		return "", fmt.Errorf("%s: %w", NameQuerySQL, err)
	}

	res, err := my.Query(inp.Context(), query, tr)
	if err != nil {
		return "", fmt.Errorf("%s: %w", NameQuerySQL, err)
	}

	if ct == "" {
		return result(res)
	}

	if res == nil {
		return result(&processor.QueryResult{Status: processor.QueryStatusNoData})
	}

	return result(newChartPreview(res.Transform(ct)))
}
