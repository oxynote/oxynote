package tools

import (
	"errors"
	"fmt"
	"time"

	"github.com/oxynote/oxynote/server/core/internal/datasource"
	"github.com/oxynote/oxynote/server/core/internal/datasource/processor"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/variable"
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

	// _maxRawLogs caps the logs of a raw Loki answer, keeping the newest.
	_maxRawLogs = 100
)

// _maxPreviewSeries caps how many series a chart check describes. A
// query behind a metric block draws a handful; one answering with more
// is already the wrong query, and listing them all would cost more than
// the check saves.
const _maxPreviewSeries = 10

// _errUnknownDataSource is what a lookup reports for an id that names
// nothing in the session's organisation. Another organisation's id
// lands here too, which is the point: the tools cannot be used to
// discover that a data source exists elsewhere.
var _errUnknownDataSource = errors.New("no data source with that id in this organisation; call list_data_sources for the ids that exist")

// _errInvertedTimeRange reports a range whose start falls after its end,
// once either absent end has been defaulted.
var _errInvertedTimeRange = errors.New("'from' is after 'to'; the range start must be the earlier timestamp")

// _errLogsChartType is returned when a chart check is asked of a Loki
// data source, whose logs no chart draws.
var _errLogsChartType = errors.New("a loki data source draws no chart; omit chart_type, and show the logs in a <logs> block")

// _timeRangeProps are the from and to arguments every ranged read takes.
var _timeRangeProps = map[string]any{
	"from": map[string]any{"type": "string", "description": _fromDescription},
	"to":   map[string]any{"type": "string", "description": _toDescription},
}

// listDataSources lists the organisation's data sources.
type listDataSources struct{}

// Info returns the tool's model-facing description.
func (listDataSources) Info() Info {
	return Info{
		Name:        NameListDataSources,
		Traits:      Traits{Group: GroupDataSources, DataSource: true},
		Description: "List the organisation's data sources as {id, name, type, status}. type is prometheus, loki, postgresql, supabase, mariadb or mysql. It decides the query language and which tools serve it, and loki holds logs. Start here, since the other data source tools take an id from this list.",
		Properties:  map[string]any{},
	}
}

// Validate accepts every payload: nothing is required.
func (listDataSources) Validate() error {
	return nil
}

// Exec lists the data sources the organisation owns.
func (listDataSources) Exec(inp Input) (any, error) {
	sources, err := inp.FetchDataSources()
	if err != nil {
		return nil, err
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

	return listDataSourcesResult{
		DataSources: out,
	}, nil
}

// listDataSourcesResult is what list_data_sources returns.
type listDataSourcesResult struct {
	// DataSources are the organisation's data sources.
	DataSources []dataSourceInfo `json:"data_sources"`
}

// dataSourceInfo is one row of list_data_sources. It is deliberately
// narrower than the stored data source: the URL and the credentials
// never reach the model.
type dataSourceInfo struct {
	// ID addresses the data source in every other data-source tool.
	ID xid.ID `json:"id"`

	// Name is the data source's display name.
	Name string `json:"name"`

	// Type is what the data source speaks (prometheus, loki, postgresql,
	// supabase, mariadb, mysql), which decides the tools that serve it.
	Type datasource.Type `json:"type"`

	// Status is the connection status recorded for it.
	Status processor.ConnectionStatus `json:"status"`
}

// getDataSourceMetadata describes what a data source holds: the metrics
// of a Prometheus one, the tables and columns of a SQL one.
type getDataSourceMetadata struct {
	// DataSourceID names the data source.
	DataSourceID xid.ID `json:"data_source_id"`
}

// Info returns the tool's model-facing description.
func (getDataSourceMetadata) Info() Info {
	return Info{
		Name:        NameGetDataSourceMetadata,
		Traits:      Traits{Group: GroupDataSources, DataSource: true},
		Description: "Describe what a data source holds. Prometheus gives each metric with its type, help and unit. Loki gives the labels its logs carried over the last hour, and list_log_fields with a query finds their other fields. SQL gives {default_schema, tables: {table: [columns]}}. Read it before writing a query, so the names in it are real.",
		Properties:  map[string]any{"data_source_id": map[string]any{"type": "string", "description": _dataSourceIDDescription}},
		Required:    []string{"data_source_id"},
	}
}

// Validate checks the arguments are complete.
func (gm getDataSourceMetadata) Validate() error {
	if gm.DataSourceID.IsNil() {
		return errRequired("data_source_id")
	}

	return nil
}

// Title announces the data source being read.
func (gm getDataSourceMetadata) Title(inp DescribeInput) string {
	ds, err := inp.FetchDataSource(gm.DataSourceID)
	if err != nil {
		return ""
	}

	return fmt.Sprintf("Reading what %q holds", ds.Name)
}

// Exec fetches the metadata of whichever kind of data source the id
// names.
func (gm getDataSourceMetadata) Exec(inp Input) (any, error) {
	runner, err := inp.DataSourceRunner(gm.DataSourceID)
	if err != nil {
		return nil, err
	}

	typ := runner.Type()
	if typ == datasource.TypePrometheus {
		prom, perr := runner.Prometheus(inp.Context())
		if perr != nil {
			return nil, perr
		}

		res, merr := prom.Metadata(inp.Context())
		if merr != nil {
			return nil, merr
		}

		return res, nil
	}

	if typ == datasource.TypeLoki {
		logs, lerr := runner.Logs(inp.Context())
		if lerr != nil {
			return nil, lerr
		}

		to := timeutil.Now()

		res, ferr := logs.LogFields(inp.Context(), "", processor.TimeRange{From: to.Add(-_defaultQueryWindow), To: to})
		if ferr != nil {
			return nil, ferr
		}

		return res, nil
	}

	sql, err := runner.SQL(inp.Context())
	if err != nil {
		return nil, err
	}

	res, err := sql.Metadata(inp.Context())
	if err != nil {
		return nil, err
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

	return out, nil
}

// sqlMetadataResult is what get_data_source_metadata returns for a SQL
// data source.
type sqlMetadataResult struct {
	// DefaultSchema is the schema a table name without one resolves to.
	DefaultSchema string `json:"default_schema"`

	// Tables maps each schema-qualified table name to its column names.
	Tables map[string][]string `json:"tables"`
}

// listPrometheusLabels lists the label names of a Prometheus data
// source, or the values one label takes.
type listPrometheusLabels struct {
	timeRange

	// DataSourceID names the Prometheus data source.
	DataSourceID xid.ID `json:"data_source_id"`

	// Label, when set, asks for the values of that label instead of
	// the label names.
	Label string `json:"label"`

	// Matchers narrows the answer to the series they select.
	Matchers []string `json:"matchers"`
}

// Info returns the tool's model-facing description.
func (listPrometheusLabels) Info() Info {
	return Info{
		Name:        NameListPrometheusLabels,
		Traits:      Traits{Group: GroupDataSources, DataSource: true},
		Description: "List the label names of a Prometheus data source or, with label, that label's values. matchers narrows either list to the series they select. Use it to find what a query should filter on.",
		Properties: map[string]any{
			"data_source_id": map[string]any{"type": "string", "description": _dataSourceIDDescription},
			"label":          map[string]any{"type": "string", "description": "A label, such as job, whose values to list."},
			"matchers":       map[string]any{"type": "array", "description": _matchersDescription, "items": map[string]any{"type": "string"}},
			"from":           _timeRangeProps["from"],
			"to":             _timeRangeProps["to"],
		},
		Required: []string{"data_source_id"},
	}
}

// Validate checks the arguments are complete.
func (lpl listPrometheusLabels) Validate() error {
	if lpl.DataSourceID.IsNil() {
		return errRequired("data_source_id")
	}

	return nil
}

// Title announces the data source being read, and the label when one
// was named.
func (lpl listPrometheusLabels) Title(inp DescribeInput) string {
	ds, err := inp.FetchDataSource(lpl.DataSourceID)
	if err != nil {
		return ""
	}

	if lpl.Label != "" {
		return fmt.Sprintf("Listing values of label %q in %q", lpl.Label, ds.Name)
	}

	return fmt.Sprintf("Listing labels of %q", ds.Name)
}

// Exec fetches the label names, or the named label's values.
func (lpl listPrometheusLabels) Exec(inp Input) (any, error) {
	tr, err := lpl.resolve()
	if err != nil {
		return nil, err
	}

	runner, err := inp.DataSourceRunner(lpl.DataSourceID)
	if err != nil {
		return nil, err
	}

	matchers, err := inp.SubstituteVariables(lpl.Matchers...)
	if err != nil {
		return nil, err
	}

	prom, err := runner.Prometheus(inp.Context())
	if err != nil {
		return nil, err
	}

	if lpl.Label != "" {
		res, verr := prom.LabelValues(inp.Context(), lpl.Label, matchers, tr)
		if verr != nil {
			return nil, verr
		}

		return res, nil
	}

	res, err := prom.LabelNames(inp.Context(), matchers, tr)
	if err != nil {
		return nil, err
	}

	return res, nil
}

// listPrometheusSeries lists the series matching a set of selectors.
type listPrometheusSeries struct {
	timeRange

	// DataSourceID names the Prometheus data source.
	DataSourceID xid.ID `json:"data_source_id"`

	// Matchers select the series to return. Required.
	Matchers []string `json:"matchers"`
}

// Info returns the tool's model-facing description.
func (listPrometheusSeries) Info() Info {
	return Info{
		Name:        NameListPrometheusSeries,
		Traits:      Traits{Group: GroupDataSources, DataSource: true},
		Description: "List the series that matchers select in a Prometheus data source, each as its full label set. Use it to see which label combinations a metric has before querying it.",
		Properties: map[string]any{
			"data_source_id": map[string]any{"type": "string", "description": _dataSourceIDDescription},
			"matchers":       map[string]any{"type": "array", "description": _matchersDescription, "items": map[string]any{"type": "string"}, "minItems": 1},
			"from":           _timeRangeProps["from"],
			"to":             _timeRangeProps["to"],
		},
		Required: []string{"data_source_id", "matchers"},
	}
}

// Validate checks the arguments are complete.
func (lps listPrometheusSeries) Validate() error {
	if lps.DataSourceID.IsNil() {
		return errRequired("data_source_id")
	}

	if len(lps.Matchers) == 0 {
		return errRequired("matchers")
	}

	return nil
}

// Title announces the data source being read.
func (lps listPrometheusSeries) Title(inp DescribeInput) string {
	ds, err := inp.FetchDataSource(lps.DataSourceID)
	if err != nil {
		return ""
	}

	return fmt.Sprintf("Listing series of %q", ds.Name)
}

// Exec fetches the matching series.
func (lps listPrometheusSeries) Exec(inp Input) (any, error) {
	tr, err := lps.resolve()
	if err != nil {
		return nil, err
	}

	runner, err := inp.DataSourceRunner(lps.DataSourceID)
	if err != nil {
		return nil, err
	}

	matchers, err := inp.SubstituteVariables(lps.Matchers...)
	if err != nil {
		return nil, err
	}

	prom, err := runner.Prometheus(inp.Context())
	if err != nil {
		return nil, err
	}

	res, err := prom.Series(inp.Context(), matchers, tr)
	if err != nil {
		return nil, err
	}

	return res, nil
}

// listLogFields lists the field names the logs of a Loki data source
// carry, or the values one field takes.
type listLogFields struct {
	timeRange

	// DataSourceID names the Loki data source.
	DataSourceID xid.ID `json:"data_source_id"`

	// Field, when set, asks for the values of that field instead of the
	// field names.
	Field string `json:"field"`

	// Query narrows the answer to the logs it matches.
	Query string `json:"query"`
}

// Info returns the tool's model-facing description.
func (listLogFields) Info() Info {
	return Info{
		Name:        NameListLogFields,
		Traits:      Traits{Group: GroupDataSources, DataSource: true},
		Description: "List the field names the logs of a Loki data source carry. With field, it lists that field's values instead, counted when the source counts them. Without query only labels and their values come back. A LogQL query narrows either list to the logs it matches and adds their other fields. Use it to find what a log query should filter on. A filter on a field parsed from a log needs a parser stage first: | json for JSON logs, | logfmt for key=value logs. detected_level and other structured metadata need no parser, but they are not labels, so filter them after the selector, as in | detected_level=\"error\".",
		Properties: map[string]any{
			"data_source_id": map[string]any{"type": "string", "description": _dataSourceIDDescription},
			"field":          map[string]any{"type": "string", "description": "A field, such as detected_level, whose values to list."},
			"query":          map[string]any{"type": "string", "description": "A LogQL log query, such as {app=\"api\"}, whose logs to read the fields from."},
			"from":           _timeRangeProps["from"],
			"to":             _timeRangeProps["to"],
		},
		Required: []string{"data_source_id"},
	}
}

// Validate checks the arguments are complete.
func (llf listLogFields) Validate() error {
	if llf.DataSourceID.IsNil() {
		return errRequired("data_source_id")
	}

	return nil
}

// Title announces the data source being read, and the field when one
// was named.
func (llf listLogFields) Title(inp DescribeInput) string {
	ds, err := inp.FetchDataSource(llf.DataSourceID)
	if err != nil {
		return ""
	}

	if llf.Field != "" {
		return fmt.Sprintf("Listing values of field %q in %q", llf.Field, ds.Name)
	}

	return fmt.Sprintf("Listing fields of %q", ds.Name)
}

// Exec fetches the field names, or the named field's values.
func (llf listLogFields) Exec(inp Input) (any, error) {
	tr, err := llf.resolve()
	if err != nil {
		return nil, err
	}

	runner, err := inp.DataSourceRunner(llf.DataSourceID)
	if err != nil {
		return nil, err
	}

	qq, err := inp.SubstituteVariables(llf.Query)
	if err != nil {
		return nil, err
	}

	logs, err := runner.Logs(inp.Context())
	if err != nil {
		return nil, err
	}

	if llf.Field != "" {
		res, verr := logs.LogFieldValues(inp.Context(), llf.Field, qq[0], tr)
		if verr != nil {
			return nil, verr
		}

		return res, nil
	}

	res, err := logs.LogFields(inp.Context(), qq[0], tr)
	if err != nil {
		return nil, err
	}

	return res, nil
}

// queryDataSource runs a query against a data source in the language it
// speaks.
type queryDataSource struct {
	timeRange

	// DataSourceID names the data source.
	DataSourceID xid.ID `json:"data_source_id"`

	// Query is the PromQL, LogQL or SQL to run. Required.
	Query string `json:"query"`

	// ChartType, when set, asks for what a metric block of that chart
	// type would draw instead of the raw answer.
	ChartType processor.ChartType `json:"chart_type"`
}

// Info returns the tool's model-facing description.
func (queryDataSource) Info() Info {
	return Info{
		Name:        NameQueryDataSource,
		Traits:      Traits{Group: GroupDataSources, DataSource: true},
		Description: fmt.Sprintf("Run a query over a time range. Use PromQL for Prometheus, a LogQL log query for Loki and read-only SQL for the others. In SQL, $__ macros such as $__timeFilter expand against the range. Without chart_type it returns the raw answer: at most %d series of %d points, %d rows, or the newest %d logs. Each log has its time, level, message, labels and fields. has_more says older logs matched, so narrow the query or the range to see them. With chart_type it returns what a metric block would draw: status, series count, and each series' labels, point count and endpoints. Check a query that way before you put it in a block.", _maxRawSeries, _maxRawPoints, _maxRawRows, _maxRawLogs),
		Properties: map[string]any{
			"data_source_id": map[string]any{"type": "string", "description": _dataSourceIDDescription},
			"query":          map[string]any{"type": "string", "description": "The PromQL, LogQL or SQL to run. In LogQL, only labels go inside {}. A field parsed from the line needs | json or | logfmt before its filter, and detected_level goes after the selector, as in | detected_level=\"error\". For a SQL chart, select a time column aliased time and one or more numeric columns."},
			"chart_type": map[string]any{
				"type":        "string",
				"enum":        []processor.ChartType{processor.ChartTypeLine, processor.ChartTypeBar, processor.ChartTypeGauge},
				"description": "The chart to check the query against. Leave it out for the values themselves. Not for Loki.",
			},
			"from": _timeRangeProps["from"],
			"to":   _timeRangeProps["to"],
		},
		Required: []string{"data_source_id", "query"},
	}
}

// Validate checks the arguments are complete.
func (qd queryDataSource) Validate() error {
	if qd.DataSourceID.IsNil() {
		return errRequired("data_source_id")
	}

	if qd.Query == "" {
		return errRequired("query")
	}

	return nil
}

// Title announces the data source being queried.
func (qd queryDataSource) Title(inp DescribeInput) string {
	ds, err := inp.FetchDataSource(qd.DataSourceID)
	if err != nil {
		return ""
	}

	return fmt.Sprintf("Querying %q", ds.Name)
}

// Exec runs the query in whichever language the data source speaks,
// answering with the raw data or, when a chart type was named, with what
// the chart would draw.
func (qd queryDataSource) Exec(inp Input) (any, error) {
	tr, err := qd.resolve()
	if err != nil {
		return nil, err
	}

	runner, err := inp.DataSourceRunner(qd.DataSourceID)
	if err != nil {
		return nil, err
	}

	vars, err := inp.VariableValues(qd.Query)
	if err != nil {
		return nil, err
	}

	q := qd.Query

	// each dialect answers in its own shape, so this is the one place a
	// tool has to know which one it is talking to.
	switch runner.Type() {
	case datasource.TypePrometheus:
		prom, err := runner.Prometheus(inp.Context())
		if err != nil {
			return nil, err
		}

		res, err := prom.QueryRange(inp.Context(), variable.Substitute(vars, q)[0], tr)
		if err != nil {
			return nil, queryError(err)
		}

		switch {
		case res == nil:
			return &processor.QueryResult{Status: processor.QueryStatusNoData}, nil
		case qd.ChartType == "":
			return newRawSeries(res), nil
		default:
			return newChartPreview(res.Transform(qd.ChartType)), nil
		}
	case datasource.TypeLoki:
		if qd.ChartType != "" {
			return nil, _errLogsChartType
		}

		return queryLogs(inp, runner, variable.Substitute(vars, q)[0], tr)
	case datasource.TypePostgreSQL, datasource.TypeSupabase:
		pg, err := runner.PostgreSQL(inp.Context())
		if err != nil {
			return nil, err
		}

		res, err := pg.Query(inp.Context(), q, tr, vars)
		if err != nil {
			return nil, queryError(err)
		}

		switch {
		case res == nil:
			return &processor.QueryResult{Status: processor.QueryStatusNoData}, nil
		case qd.ChartType == "":
			return newRawRows(res.Columns, res.Rows), nil
		}

		qr, err := res.Transform(qd.ChartType)
		if err != nil {
			return nil, err
		}

		return newChartPreview(qr), nil
	default:
		my, err := runner.MySQL(inp.Context())
		if err != nil {
			return nil, err
		}

		res, err := my.Query(inp.Context(), q, tr, vars)
		if err != nil {
			return nil, queryError(err)
		}

		switch {
		case res == nil:
			return &processor.QueryResult{Status: processor.QueryStatusNoData}, nil
		case qd.ChartType == "":
			return newRawRows(res.Columns, res.Rows), nil
		}

		qr, err := res.Transform(qd.ChartType)
		if err != nil {
			return nil, err
		}

		return newChartPreview(qr), nil
	}
}

// queryLogs reads the newest logs a LogQL query matches, as
// query_data_source answers for a Loki data source.
func queryLogs(inp Input, runner datasource.Runner, q string, tr processor.TimeRange) (any, error) {
	logs, err := runner.Logs(inp.Context())
	if err != nil {
		return nil, err
	}

	page := processor.LogsPage{Limit: _maxRawLogs, Order: processor.LogsOrderDesc}

	res, err := logs.Logs(inp.Context(), q, tr, page)
	if err != nil {
		if processor.IsOutage(err) {
			// an outage or a rate limit is not fixed by a different query.
			return nil, err
		}

		return nil, fmt.Errorf("%w; check the fields with list_log_fields, and use a log query, since metric queries such as rate() are not supported", err)
	}

	if !res.HasData() {
		return &processor.QueryResult{Status: processor.QueryStatusNoData}, nil
	}

	return newRawLogs(res), nil
}

// queryError returns a failed query's error with a hint to check the query
// when the data source rejected it. Any other failure, such as an outage or
// a rate limit, is returned as it is: a different query does not fix it.
func queryError(err error) error {
	if !processor.IsInvalidQuery(err) {
		return err
	}

	return fmt.Errorf("%w; check the query against get_data_source_metadata", err)
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

// rawLogs is a raw Loki answer, cut down to its newest _maxRawLogs logs.
type rawLogs struct {
	// Status is whether the query matched any log.
	Status processor.QueryStatus `json:"status"`

	// Logs are the newest logs the query matched, newest first.
	Logs []rawLog `json:"logs"`

	// HasMore is whether more logs matched than the ones shown.
	HasMore bool `json:"has_more"`
}

// rawLog is one log of a raw Loki answer.
type rawLog struct {
	// Time is when the log was written, in UTC.
	Time time.Time `json:"time"`

	// Level is the log's normalised severity.
	Level processor.LogLevel `json:"level"`

	// Message is the log itself.
	Message string `json:"message"`

	// Labels are what the source indexes the log by.
	Labels map[string]string `json:"labels,omitempty"`

	// Fields are everything else the log carries, such as structured
	// metadata.
	Fields map[string]string `json:"fields,omitempty"`

	// Parsed are the keys a parser stage of the query took from the log.
	Parsed map[string]string `json:"parsed,omitempty"`
}

// newRawLogs renames a page of logs for the model. The page was read at
// _maxRawLogs, but a page whose logs share one timestamp holds that whole
// group, so it is cut to the cap again.
func newRawLogs(res *processor.LogsResult) rawLogs {
	lines := res.Lines[:min(len(res.Lines), _maxRawLogs)]

	out := rawLogs{
		Status:  res.Status,
		Logs:    make([]rawLog, 0, len(lines)),
		HasMore: res.NextCursor.Valid || len(res.Lines) > len(lines),
	}

	for _, l := range lines {
		out.Logs = append(out.Logs, rawLog{
			Time:    l.Time,
			Level:   l.Level,
			Message: l.Line,
			Labels:  l.Labels,
			Fields:  l.Fields,
			Parsed:  l.Parsed,
		})
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
// named: the shape of what the chart would draw, without its points.
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

// timeRange is the range every data-source read can be narrowed
// with. Both ends are optional: the tools serve a model that usually
// means "recently" and should not have to compute timestamps to say so.
type timeRange struct {
	// From is the range start. Zero means an hour before the end.
	From time.Time `json:"from"`

	// To is the range end. Zero means now.
	To time.Time `json:"to"`
}

// resolve turns the pair into a time range, defaulting the end to now
// and the start to an hour before it. An inverted range is rejected
// here rather than handed to a backend, so the model is told the
// arguments are backwards instead of seeing a backend-shaped failure.
func (a timeRange) resolve() (processor.TimeRange, error) {
	out := processor.TimeRange{From: a.From, To: a.To}

	if out.To.IsZero() {
		out.To = timeutil.Now()
	}

	if out.From.IsZero() {
		out.From = out.To.Add(-_defaultQueryWindow)
	}

	if out.From.After(out.To) {
		return processor.TimeRange{}, _errInvertedTimeRange
	}

	return out, nil
}

// DataSourceInput is what the data-source tools reach on top of
// DescribeInput.
type DataSourceInput interface {
	// FetchDataSources should return the organisation's data sources.
	FetchDataSources() ([]datasource.DataSource, error)

	// DataSourceRunner should return the runner that reads the data
	// source.
	DataSourceRunner(id xid.ID) (datasource.Runner, error)

	// CheckDataSources should refuse blocks naming a data source the
	// organisation does not have.
	CheckDataSources(blocks []document.Block) error

	// VariableValues should return the value of every variable the texts
	// name, keyed by name.
	VariableValues(texts ...string) (map[string]string, error)

	// SubstituteVariables should return the texts with the variables
	// they name replaced by their values.
	SubstituteVariables(texts ...string) ([]string, error)
}
