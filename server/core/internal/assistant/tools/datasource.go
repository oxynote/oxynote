package tools

import (
	"errors"
	"fmt"
	"time"

	"github.com/oxynote/oxynote/server/core/internal/datasource"
	"github.com/oxynote/oxynote/server/core/internal/datasource/processor"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
	"github.com/prometheus/common/model"
	"github.com/rs/xid"
)

// _defaultQueryWindow is the window a data-source tool reads when the
// model names neither end of the range. An hour is what the metric
// block's own default preset covers.
const _defaultQueryWindow = time.Hour

// Caps on a raw query answer. A chart check describes any answer in a
// few lines; the raw data is for reading values, and past these sizes
// it costs more than it tells.
const (
	// _maxRawSeries caps the series of a raw Prometheus answer.
	_maxRawSeries = 20

	// _maxRawPoints caps the points of each series in a raw Prometheus
	// answer, keeping the latest.
	_maxRawPoints = 100

	// _maxRawRows caps the rows of a raw SQL answer.
	_maxRawRows = 200
)

// _maxPreviewSeries caps how many series a chart check describes. A
// query behind a metric block draws a handful; one answering with more
// is already the wrong query, and listing them all would cost more than
// the check saves.
const _maxPreviewSeries = 10

// errUnknownDataSource is what a lookup reports for an id that names
// nothing in the session's organisation. Another organisation's id
// lands here too, which is the point: the tools cannot be used to
// discover that a data source exists elsewhere.
var errUnknownDataSource = errors.New("no data source with that id in this organisation; call list_data_sources for the ids that exist")

// errInvertedTimeRange reports a range whose start falls after its end,
// once either absent end has been defaulted.
var errInvertedTimeRange = errors.New("'from' is after 'to'; the range start must be the earlier timestamp")

// _timeRangeProps are the from and to arguments every ranged read takes.
var _timeRangeProps = map[string]any{
	"from": map[string]any{"type": "string", "description": _fromDescription},
	"to":   map[string]any{"type": "string", "description": _toDescription},
}

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

// listDataSources lists the organisation's data sources.
type listDataSources struct{}

// Info returns the tool's model-facing description.
func (listDataSources) Info() Info {
	return Info{
		Name:        NameListDataSources,
		Traits:      Traits{DataSource: true},
		Description: "List the organisation's data sources as {id, name, type, status}. type (prometheus, postgresql, mariadb or mysql) decides the query language and which tools serve it. Start here: the other data source tools take an id from this list.",
		Properties:  map[string]any{},
	}
}

// Execute lists the data sources the organisation owns.
func (listDataSources) Execute(inp *input) (string, error) {
	sources, err := inp.FetchDataSources()
	if err != nil {
		return "", err
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

// dataSourceArgs is what get_data_source_metadata is called with.
type dataSourceArgs struct {
	// DataSourceID names the data source.
	DataSourceID xid.ID `json:"data_source_id"`
}

// Validate checks the arguments are complete.
func (a dataSourceArgs) Validate() error {
	if a.DataSourceID.IsNil() {
		return errRequired("data_source_id")
	}

	return nil
}

// getDataSourceMetadata describes what a data source holds: the metrics
// of a Prometheus one, the tables and columns of a SQL one.
type getDataSourceMetadata struct{}

// Info returns the tool's model-facing description.
func (getDataSourceMetadata) Info() Info {
	return Info{
		Name:        NameGetDataSourceMetadata,
		Traits:      Traits{DataSource: true},
		Description: "Describe what a data source holds: for Prometheus, each metric with its type, help and unit; for SQL, {default_schema, tables: {table: [columns]}}. Read it before writing a query, so the names in it are real ones.",
		Properties:  map[string]any{"data_source_id": map[string]any{"type": "string", "description": _dataSourceIDDescription}},
		Required:    []string{"data_source_id"},
	}
}

// Title announces the data source being read.
func (getDataSourceMetadata) Title(inp DescribeInput) string {
	var in dataSourceArgs

	if err := inp.Decode(&in); err != nil {
		return ""
	}

	ds, err := inp.FetchDataSource(in.DataSourceID)
	if err != nil {
		return ""
	}

	return fmt.Sprintf("Reading what %q holds", ds.Name)
}

// Execute fetches the metadata of whichever kind of data source the id
// names.
func (getDataSourceMetadata) Execute(inp *input) (string, error) {
	var in dataSourceArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	runner, err := inp.DataSourceRunner(in.DataSourceID)
	if err != nil {
		return "", err
	}

	if runner.Type() == datasource.TypePrometheus {
		prom, perr := runner.Prometheus(inp.Context())
		if perr != nil {
			return "", perr
		}

		res, merr := prom.Metadata(inp.Context())
		if merr != nil {
			return "", merr
		}

		return result(res)
	}

	sql, err := runner.SQL(inp.Context())
	if err != nil {
		return "", err
	}

	res, err := sql.Metadata(inp.Context())
	if err != nil {
		return "", err
	}

	out := sqlMetadataResult{
		DefaultSchema: res.DefaultSchema,
		Tables:        make(map[string][]string, len(res.Tables)),
	}

	for name, table := range res.Tables {
		columns := make([]string, 0, len(table.Columns))

		for _, c := range table.Columns {
			columns = append(columns, c.Name)
		}

		out.Tables[name] = columns
	}

	return result(out)
}

// sqlMetadataResult is what get_data_source_metadata returns for a SQL
// data source.
type sqlMetadataResult struct {
	// DefaultSchema is the schema a table name without one resolves to.
	DefaultSchema string `json:"default_schema"`

	// Tables maps each schema-qualified table name to its column names.
	Tables map[string][]string `json:"tables"`
}

// prometheusLabelsArgs is what list_prometheus_labels is called with.
type prometheusLabelsArgs struct {
	timeRangeArgs

	// DataSourceID names the Prometheus data source.
	DataSourceID xid.ID `json:"data_source_id"`

	// Label, when set, asks for the values of that label instead of
	// the label names.
	Label string `json:"label"`

	// Matchers narrows the answer to the series they select.
	Matchers []string `json:"matchers"`
}

// Validate checks the arguments are complete.
func (a prometheusLabelsArgs) Validate() error {
	if a.DataSourceID.IsNil() {
		return errRequired("data_source_id")
	}

	return nil
}

// listPrometheusLabels lists the label names of a Prometheus data
// source, or the values one label takes.
type listPrometheusLabels struct{}

// Info returns the tool's model-facing description.
func (listPrometheusLabels) Info() Info {
	return Info{
		Name:        NameListPrometheusLabels,
		Traits:      Traits{DataSource: true},
		Description: "List the label names of a Prometheus data source or, with label set, the values that label takes. matchers narrows either to the series they select. Use it to find the labels and values a query should filter on.",
		Properties: map[string]any{
			"data_source_id": map[string]any{"type": "string", "description": _dataSourceIDDescription},
			"label":          map[string]any{"type": "string", "description": "Optional. A label, such as job, whose values to list instead of the names."},
			"matchers":       map[string]any{"type": "array", "description": "Optional. " + _matchersDescription, "items": map[string]any{"type": "string"}},
			"from":           _timeRangeProps["from"],
			"to":             _timeRangeProps["to"],
		},
		Required: []string{"data_source_id"},
	}
}

// Title announces the data source being read, and the label when one
// was named.
func (listPrometheusLabels) Title(inp DescribeInput) string {
	var in prometheusLabelsArgs

	if err := inp.Decode(&in); err != nil {
		return ""
	}

	ds, err := inp.FetchDataSource(in.DataSourceID)
	if err != nil {
		return ""
	}

	if in.Label != "" {
		return fmt.Sprintf("Listing values of label %q in %q", in.Label, ds.Name)
	}

	return fmt.Sprintf("Listing labels of %q", ds.Name)
}

// Execute fetches the label names, or the named label's values.
func (listPrometheusLabels) Execute(inp *input) (string, error) {
	var in prometheusLabelsArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	tr, err := in.resolve()
	if err != nil {
		return "", err
	}

	runner, err := inp.DataSourceRunner(in.DataSourceID)
	if err != nil {
		return "", err
	}

	prom, err := runner.Prometheus(inp.Context())
	if err != nil {
		return "", err
	}

	if in.Label != "" {
		res, verr := prom.LabelValues(inp.Context(), in.Label, in.Matchers, tr)
		if verr != nil {
			return "", verr
		}

		return result(res)
	}

	res, err := prom.LabelNames(inp.Context(), in.Matchers, tr)
	if err != nil {
		return "", err
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
		return errRequired("data_source_id")
	}

	if len(a.Matchers) == 0 {
		return errRequired("matchers")
	}

	return nil
}

// listPrometheusSeries lists the series matching a set of selectors.
type listPrometheusSeries struct{}

// Info returns the tool's model-facing description.
func (listPrometheusSeries) Info() Info {
	return Info{
		Name:        NameListPrometheusSeries,
		Traits:      Traits{DataSource: true},
		Description: "List the series that matchers select in a Prometheus data source, each as its full label set. Use it to see which label combinations a metric has before querying it.",
		Properties: map[string]any{
			"data_source_id": map[string]any{"type": "string", "description": _dataSourceIDDescription},
			"matchers":       map[string]any{"type": "array", "description": _matchersDescription, "items": map[string]any{"type": "string"}},
			"from":           _timeRangeProps["from"],
			"to":             _timeRangeProps["to"],
		},
		Required: []string{"data_source_id", "matchers"},
	}
}

// Title announces the data source being read.
func (listPrometheusSeries) Title(inp DescribeInput) string {
	var in prometheusSeriesArgs

	if err := inp.Decode(&in); err != nil {
		return ""
	}

	ds, err := inp.FetchDataSource(in.DataSourceID)
	if err != nil {
		return ""
	}

	return fmt.Sprintf("Listing series of %q", ds.Name)
}

// Execute fetches the matching series.
func (listPrometheusSeries) Execute(inp *input) (string, error) {
	var in prometheusSeriesArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	tr, err := in.resolve()
	if err != nil {
		return "", err
	}

	runner, err := inp.DataSourceRunner(in.DataSourceID)
	if err != nil {
		return "", err
	}

	prom, err := runner.Prometheus(inp.Context())
	if err != nil {
		return "", err
	}

	res, err := prom.Series(inp.Context(), in.Matchers, tr)
	if err != nil {
		return "", err
	}

	return result(res)
}

// queryDataSourceArgs is what query_data_source is called with.
type queryDataSourceArgs struct {
	timeRangeArgs

	// DataSourceID names the data source.
	DataSourceID xid.ID `json:"data_source_id"`

	// Query is the PromQL or SQL to run. Required.
	Query string `json:"query"`

	// ChartType, when set, asks for what a metric block of that chart
	// type would draw instead of the raw answer.
	ChartType processor.ChartType `json:"chart_type"`
}

// Validate checks the arguments are complete.
func (a queryDataSourceArgs) Validate() error {
	if a.DataSourceID.IsNil() {
		return errRequired("data_source_id")
	}

	if a.Query == "" {
		return errRequired("query")
	}

	return nil
}

// queryDataSource runs a query against a data source in the language it
// speaks.
type queryDataSource struct{}

// Info returns the tool's model-facing description.
func (queryDataSource) Info() Info {
	return Info{
		Name:        NameQueryDataSource,
		Traits:      Traits{DataSource: true},
		Description: fmt.Sprintf("Run a query over a time range: PromQL for Prometheus, read-only SQL for the others, where $__ macros such as $__timeFilter expand against the range. Without chart_type it returns the raw answer, capped at %d series of %d points or %d rows. With chart_type it returns what a metric block would draw: status, series count, and each series' labels, point count and endpoints. Check a query that way before putting it in a block.", _maxRawSeries, _maxRawPoints, _maxRawRows),
		Properties: map[string]any{
			"data_source_id": map[string]any{"type": "string", "description": _dataSourceIDDescription},
			"query":          map[string]any{"type": "string", "description": "The PromQL or SQL to run. For a SQL chart, select a time column aliased time and one or more numeric columns."},
			"chart_type": map[string]any{
				"type":        "string",
				"enum":        []processor.ChartType{processor.ChartTypeLine, processor.ChartTypeBar, processor.ChartTypeGauge},
				"description": "Optional. The chart to check the query against; omit it for the values themselves.",
			},
			"from": _timeRangeProps["from"],
			"to":   _timeRangeProps["to"],
		},
		Required: []string{"data_source_id", "query"},
	}
}

// Title announces the data source being queried.
func (queryDataSource) Title(inp DescribeInput) string {
	var in queryDataSourceArgs

	if err := inp.Decode(&in); err != nil {
		return ""
	}

	ds, err := inp.FetchDataSource(in.DataSourceID)
	if err != nil {
		return ""
	}

	return fmt.Sprintf("Querying %q", ds.Name)
}

// Execute runs the query in whichever language the data source speaks,
// answering with the raw data or, when a chart type was named, with what
// the chart would draw.
func (queryDataSource) Execute(inp *input) (string, error) {
	var in queryDataSourceArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	tr, err := in.resolve()
	if err != nil {
		return "", err
	}

	runner, err := inp.DataSourceRunner(in.DataSourceID)
	if err != nil {
		return "", err
	}

	// each dialect answers in its own shape, so this is the one place a
	// tool has to know which one it is talking to.
	switch runner.Type() {
	case datasource.TypePrometheus:
		prom, err := runner.Prometheus(inp.Context())
		if err != nil {
			return "", err
		}

		res, err := prom.QueryRange(inp.Context(), in.Query, tr)
		if err != nil {
			return "", fmt.Errorf("%w; check the query against get_data_source_metadata", err)
		}

		switch {
		case res == nil:
			return result(&processor.QueryResult{Status: processor.QueryStatusNoData})
		case in.ChartType == "":
			return result(newRawSeries(res))
		default:
			return result(newChartPreview(res.Transform(in.ChartType)))
		}
	case datasource.TypePostgreSQL:
		pg, err := runner.PostgreSQL(inp.Context())
		if err != nil {
			return "", err
		}

		res, err := pg.Query(inp.Context(), in.Query, tr)
		if err != nil {
			return "", fmt.Errorf("%w; check the query against get_data_source_metadata", err)
		}

		switch {
		case res == nil:
			return result(&processor.QueryResult{Status: processor.QueryStatusNoData})
		case in.ChartType == "":
			return result(newRawRows(res.Columns, res.Rows))
		default:
			return result(newChartPreview(res.Transform(in.ChartType)))
		}
	default:
		my, err := runner.MySQL(inp.Context())
		if err != nil {
			return "", err
		}

		res, err := my.Query(inp.Context(), in.Query, tr)
		if err != nil {
			return "", fmt.Errorf("%w; check the query against get_data_source_metadata", err)
		}

		switch {
		case res == nil:
			return result(&processor.QueryResult{Status: processor.QueryStatusNoData})
		case in.ChartType == "":
			return result(newRawRows(res.Columns, res.Rows))
		default:
			return result(newChartPreview(res.Transform(in.ChartType)))
		}
	}
}

// rawSeries is a raw Prometheus answer, cut down to _maxRawSeries series
// of the latest _maxRawPoints points each.
type rawSeries struct {
	// Warnings are what Prometheus warned about while answering.
	Warnings []string `json:"warnings,omitempty"`

	// Type is the kind of value the query returned.
	Type model.ValueType `json:"type"`

	// Result is the answer itself.
	Result any `json:"result,omitempty"`

	// Truncated says what was left out, when anything was.
	Truncated string `json:"truncated,omitempty"`
}

// newRawSeries caps a raw Prometheus answer. Only a range of series can
// grow past the caps; any other answer is returned as it is.
func newRawSeries(res *processor.PrometheusQueryResult) rawSeries {
	out := rawSeries{Warnings: res.Warnings, Type: res.Type, Result: res.Result}

	matrix, ok := res.Result.(model.Matrix)
	if !ok {
		return out
	}

	capped := make(model.Matrix, 0, min(len(matrix), _maxRawSeries))
	cut := len(matrix) > _maxRawSeries

	for _, s := range matrix[:min(len(matrix), _maxRawSeries)] {
		if len(s.Values) > _maxRawPoints {
			s = &model.SampleStream{Metric: s.Metric, Values: s.Values[len(s.Values)-_maxRawPoints:]}
			cut = true
		}

		capped = append(capped, s)
	}

	out.Result = capped

	if cut {
		out.Truncated = fmt.Sprintf("%d of %d series, each its latest %d points at most; set chart_type to describe them all", len(capped), len(matrix), _maxRawPoints)
	}

	return out
}

// rawRows is a raw SQL answer, cut down to its first _maxRawRows rows.
type rawRows struct {
	// Columns are the column names.
	Columns []string `json:"columns"`

	// Rows are the rows, each a value per column.
	Rows [][]any `json:"rows"`

	// TotalRows is how many rows the query returned, set only when more
	// than the ones listed.
	TotalRows int `json:"total_rows,omitempty"`
}

// newRawRows caps a raw SQL answer.
func newRawRows(columns []string, rows [][]any) rawRows {
	out := rawRows{Columns: columns, Rows: rows[:min(len(rows), _maxRawRows)]}

	if len(rows) > _maxRawRows {
		out.TotalRows = len(rows)
	}

	return out
}

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
