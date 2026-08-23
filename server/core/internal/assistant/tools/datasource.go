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
	_descChartType = "Optional. One of line_chart, bar_chart, gauge_chart. When set, the result is the transformed series the metric block renders, plus a status, instead of the raw result — use it to check a query before putting it in a metric block."
)

// errUnknownDataSource is what a lookup reports for an id that names
// nothing in the session's organisation. Another organisation's id
// lands here too, which is the point: the tools cannot be used to
// discover that a data source exists elsewhere.
var errUnknownDataSource = errors.New("no data source with that id in this organisation; call list_data_sources for the ids that exist")

// timeRangeArgs is the range every data-source read can be narrowed
// with. Both ends are optional: the tools serve a model that usually
// means "recently" and should not have to compute timestamps to say so.
type timeRangeArgs struct {
	// From is the range start as an RFC3339 timestamp.
	From string `json:"from"`

	// To is the range end as an RFC3339 timestamp.
	To string `json:"to"`
}

// resolve turns the pair into a time range, defaulting the end to now
// and the start to an hour before it.
func (a timeRangeArgs) resolve() (processor.TimeRange, error) {
	out := processor.TimeRange{To: timeutil.Now()}

	if a.To != "" {
		to, err := time.Parse(time.RFC3339, a.To)
		if err != nil {
			return processor.TimeRange{}, fmt.Errorf("to must be an RFC3339 timestamp: %w", err)
		}

		out.To = to
	}

	out.From = out.To.Add(-_defaultQueryWindow)

	if a.From != "" {
		from, err := time.Parse(time.RFC3339, a.From)
		if err != nil {
			return processor.TimeRange{}, fmt.Errorf("from must be an RFC3339 timestamp: %w", err)
		}

		out.From = from
	}

	return out, nil
}

// timeRangeProps returns the shared from/to schema properties.
func timeRangeProps() map[string]any {
	return map[string]any{
		_keyFrom: stringProp(_descFrom),
		_keyTo:   stringProp(_descTo),
	}
}

// dataSourceProps builds a data-source tool's schema from the shared
// data-source-id property plus whatever else the tool takes.
func dataSourceProps(extra map[string]any) map[string]any {
	out := map[string]any{_keyDataSourceID: stringProp(_descDataSourceID)}

	maps.Copy(out, extra)

	return out
}

// chartType parses the optional chart_type argument. An empty value
// means the caller wants the raw result.
func chartType(raw string) (processor.ChartType, error) {
	if raw == "" {
		return "", nil
	}

	ct := processor.ChartType(raw)
	if !ct.IsValid() {
		return "", fmt.Errorf("chart_type must be one of line_chart, bar_chart, gauge_chart, got %q", raw)
	}

	return ct, nil
}

// dataSourceTitle names the data source a call is about to read, for
// the status line.
//
// An id that resolves to nothing is announced by id rather than
// abandoned: the call is about to fail on that id, and naming it is
// what makes the failure legible. That is why the lookup's error is
// answered here instead of being passed on — the label is not the place
// the failure gets reported.
func dataSourceTitle(inp DescribeInput, verb, id string) string {
	if id == "" {
		return ""
	}

	name := id

	if ds, err := inp.DataSource(id); err == nil && ds != nil {
		name = ds.Name
	}

	return fmt.Sprintf("%s %q", verb, name)
}

// runnerFor resolves the data source the call names to the runner that
// reads it. The id arrives as text from the model, so parsing it is
// part of resolving it, and every failure is phrased for the caller
// that can act on it.
func runnerFor(inp Input, name Name, rawID string) (datasource.Runner, error) {
	if rawID == "" {
		return nil, fmt.Errorf("%s: %s is required", name, _keyDataSourceID)
	}

	id, err := xid.FromString(rawID)
	if err != nil {
		return nil, fmt.Errorf("%s: %s is not a valid data source id: %w", name, _keyDataSourceID, err)
	}

	runner, err := inp.DataSourceRunner(id)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}

	return runner, nil
}

// prometheusClient resolves the call's data source and takes its
// Prometheus client, which is what every Prometheus tool starts with.
func prometheusClient(inp Input, name Name, rawID string) (datasource.Prometheus, error) {
	runner, err := runnerFor(inp, name, rawID)
	if err != nil {
		return nil, err
	}

	prom, err := runner.Prometheus(inp.Context())
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}

	return prom, nil
}

// sqlClient resolves the call's data source and takes its SQL client,
// whichever dialect it speaks.
func sqlClient(inp Input, name Name, rawID string) (datasource.SQL, error) {
	runner, err := runnerFor(inp, name, rawID)
	if err != nil {
		return nil, err
	}

	sql, err := runner.SQL(inp.Context())
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}

	return sql, nil
}

// dataSourceInfo is one row of list_data_sources. It is deliberately
// narrower than the stored data source: the URL and the credentials
// never reach the model.
type dataSourceInfo struct {
	// ID addresses the data source in every other data-source tool.
	ID string `json:"id"`

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
			ID:     ds.ID.String(),
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
	DataSourceID string `json:"data_source_id"`
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

	return dataSourceTitle(inp, "Reading metric metadata of", in.DataSourceID), nil
}

// Execute fetches the data source's metric metadata.
func (getPrometheusMetadata) Execute(inp Input) (string, error) {
	var in getPrometheusMetadataArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	prom, err := prometheusClient(inp, NameGetPrometheusMetadata, in.DataSourceID)
	if err != nil {
		return "", err
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
	DataSourceID string `json:"data_source_id"`

	// Matchers narrows the label names to the series they select.
	Matchers []string `json:"matchers"`
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

	return dataSourceTitle(inp, "Listing label names of", in.DataSourceID), nil
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

	prom, err := prometheusClient(inp, NameListPrometheusLabelNames, in.DataSourceID)
	if err != nil {
		return "", err
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
	DataSourceID string `json:"data_source_id"`

	// Label is the label whose values are being listed. Required.
	Label string `json:"label"`

	// Matchers narrows the values to the series they select.
	Matchers []string `json:"matchers"`
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

	if in.Label == "" {
		return dataSourceTitle(inp, "Listing label values of", in.DataSourceID), nil
	}

	return dataSourceTitle(inp, fmt.Sprintf("Listing values of label %q in", in.Label), in.DataSourceID), nil
}

// Execute fetches the label's values.
func (listPrometheusLabelValues) Execute(inp Input) (string, error) {
	var in prometheusLabelValuesArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	if in.Label == "" {
		return "", fmt.Errorf("%s: label is required", NameListPrometheusLabelValues)
	}

	tr, err := in.resolve()
	if err != nil {
		return "", fmt.Errorf("%s: %w", NameListPrometheusLabelValues, err)
	}

	prom, err := prometheusClient(inp, NameListPrometheusLabelValues, in.DataSourceID)
	if err != nil {
		return "", err
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
	DataSourceID string `json:"data_source_id"`

	// Matchers select the series to return. Required.
	Matchers []string `json:"matchers"`
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

	return dataSourceTitle(inp, "Listing series of", in.DataSourceID), nil
}

// Execute fetches the matching series.
func (listPrometheusSeries) Execute(inp Input) (string, error) {
	var in prometheusSeriesArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	// Prometheus rejects a series query with no selector, so the model
	// is told what is missing rather than handed the upstream error.
	if len(in.Matchers) == 0 {
		return "", fmt.Errorf("%s: at least one matcher is required", NameListPrometheusSeries)
	}

	tr, err := in.resolve()
	if err != nil {
		return "", fmt.Errorf("%s: %w", NameListPrometheusSeries, err)
	}

	prom, err := prometheusClient(inp, NameListPrometheusSeries, in.DataSourceID)
	if err != nil {
		return "", err
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
	DataSourceID string `json:"data_source_id"`

	// Query is the PromQL expression to run. Required.
	Query string `json:"query"`

	// ChartType, when set, asks for the transformed series a metric
	// block of that chart type would render instead of the raw result.
	ChartType string `json:"chart_type"`
}

// queryPrometheus runs a PromQL range query.
type queryPrometheus struct {
	plainSummary
}

// Info returns the tool's model-facing description.
func (queryPrometheus) Info() Info {
	return Info{
		Name:        NameQueryPrometheus,
		Description: "Run a PromQL range query against a Prometheus data source over the given window. Returns the raw result by default, or the series a metric block would render when chart_type is set.",
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

	return dataSourceTitle(inp, "Querying", in.DataSourceID), nil
}

// Execute runs the query, transforming the result when a chart type
// was asked for.
func (queryPrometheus) Execute(inp Input) (string, error) {
	var in queryPrometheusArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	if in.Query == "" {
		return "", fmt.Errorf("%s: query is required", NameQueryPrometheus)
	}

	ct, err := chartType(in.ChartType)
	if err != nil {
		return "", fmt.Errorf("%s: %w", NameQueryPrometheus, err)
	}

	tr, err := in.resolve()
	if err != nil {
		return "", fmt.Errorf("%s: %w", NameQueryPrometheus, err)
	}

	prom, err := prometheusClient(inp, NameQueryPrometheus, in.DataSourceID)
	if err != nil {
		return "", err
	}

	res, err := prom.QueryRange(inp.Context(), in.Query, tr)
	if err != nil {
		return "", fmt.Errorf("%s: %w", NameQueryPrometheus, err)
	}

	if ct == "" {
		return result(res)
	}

	// a query that returned nothing has no result to transform, and the
	// metric block renders it as no-data — which is the answer the
	// model asked for by naming a chart type.
	if res == nil {
		return result(&processor.QueryResult{Status: processor.QueryStatusNoData})
	}

	return result(res.Transform(ct))
}

// getSQLMetadataArgs is what get_sql_metadata is called with.
type getSQLMetadataArgs struct {
	// DataSourceID names the SQL data source.
	DataSourceID string `json:"data_source_id"`
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

	return dataSourceTitle(inp, "Reading tables of", in.DataSourceID), nil
}

// Execute fetches the data source's tables and columns.
func (getSQLMetadata) Execute(inp Input) (string, error) {
	var in getSQLMetadataArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	sql, err := sqlClient(inp, NameGetSQLMetadata, in.DataSourceID)
	if err != nil {
		return "", err
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
	DataSourceID string `json:"data_source_id"`

	// Query is the SQL to probe. Required.
	Query string `json:"query"`
}

// getSQLQueryLabels probes a query for its string columns.
type getSQLQueryLabels struct {
	plainSummary
}

// Info returns the tool's model-facing description.
func (getSQLQueryLabels) Info() Info {
	return Info{
		Name:        NameGetSQLQueryLabels,
		Description: "Run a SQL query limited to one row and return its string columns with an example value each — the columns a chart would treat as series labels. Cheaper than query_sql for checking what a query returns.",
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

	return dataSourceTitle(inp, "Probing query labels of", in.DataSourceID), nil
}

// Execute probes the query for its string columns.
func (getSQLQueryLabels) Execute(inp Input) (string, error) {
	var in sqlQueryLabelsArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	if in.Query == "" {
		return "", fmt.Errorf("%s: query is required", NameGetSQLQueryLabels)
	}

	tr, err := in.resolve()
	if err != nil {
		return "", fmt.Errorf("%s: %w", NameGetSQLQueryLabels, err)
	}

	sql, err := sqlClient(inp, NameGetSQLQueryLabels, in.DataSourceID)
	if err != nil {
		return "", err
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
	DataSourceID string `json:"data_source_id"`

	// Query is the SQL to run. Required.
	Query string `json:"query"`

	// ChartType, when set, asks for the transformed series a metric
	// block of that chart type would render instead of the raw rows.
	ChartType string `json:"chart_type"`
}

// querySQL runs a query against a SQL data source.
type querySQL struct {
	plainSummary
}

// Info returns the tool's model-facing description.
func (querySQL) Info() Info {
	return Info{
		Name:        NameQuerySQL,
		Description: "Run a read-only query against a PostgreSQL, MariaDB or MySQL data source. Returns columns and rows by default, or the series a metric block would render when chart_type is set. $__ macros ($__timeFilter, $__timeGroupAlias, …) are expanded against the window, and the row count is capped.",
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

	return dataSourceTitle(inp, "Querying", in.DataSourceID), nil
}

// Execute runs the query against whichever SQL dialect the data source
// speaks, transforming the result when a chart type was asked for.
func (querySQL) Execute(inp Input) (string, error) {
	var in querySQLArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	if in.Query == "" {
		return "", fmt.Errorf("%s: query is required", NameQuerySQL)
	}

	ct, err := chartType(in.ChartType)
	if err != nil {
		return "", fmt.Errorf("%s: %w", NameQuerySQL, err)
	}

	tr, err := in.resolve()
	if err != nil {
		return "", fmt.Errorf("%s: %w", NameQuerySQL, err)
	}

	runner, err := runnerFor(inp, NameQuerySQL, in.DataSourceID)
	if err != nil {
		return "", err
	}

	// the two dialects return different result shapes, so this is the
	// one place a tool has to know which one it is talking to.
	if runner.Type() == datasource.TypePostgreSQL {
		return runPostgreSQLQuery(inp, runner, in.Query, tr, ct)
	}

	return runMySQLQuery(inp, runner, in.Query, tr, ct)
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

	return result(res.Transform(ct))
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

	return result(res.Transform(ct))
}
