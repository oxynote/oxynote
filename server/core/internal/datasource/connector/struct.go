package connector

import (
	"encoding/json"

	"github.com/oxynote/oxynote/server/core/internal/datasource"
	"github.com/oxynote/oxynote/server/core/internal/datasource/processor"
)

// TestConnectionRequest represents a request to test the connection to a data source.
type TestConnectionRequest struct {
	// Runner contains the necessary information to test the connection to the data source.
	Runner datasource.Runner `json:"runner"`
}

// NewTestConnectionRequest creates a new TestConnectionRequest from the given Runner.
func NewTestConnectionRequest(runner datasource.Runner) *TestConnectionRequest {
	return &TestConnectionRequest{
		Runner: runner,
	}
}

// PrometheusQueryRequest represents a request to execute a Prometheus query against a data source.
type PrometheusQueryRequest struct {
	// Runner contains the necessary information to execute the query against the data source.
	Runner datasource.Runner `json:"runner"`

	// Query is the Prometheus query to be executed.
	Query string `json:"query"`

	// TimeRange specifies the time range for the query, if applicable.
	TimeRange *processor.TimeRange `json:"timeRange,omitempty"`
}

// NewPrometheusQueryRequest creates a new PrometheusQueryRequest from the given Runner, query, and time range.
func NewPrometheusQueryRequest(runner datasource.Runner, query string, timeRange *processor.TimeRange) *PrometheusQueryRequest {
	return &PrometheusQueryRequest{
		Runner:    runner,
		Query:     query,
		TimeRange: timeRange,
	}
}

// PrometheusMetadataRequest represents a request to fetch metadata from a Prometheus data source.
type PrometheusMetadataRequest struct {
	// Runner contains the necessary information to fetch metadata from the data source.
	Runner datasource.Runner `json:"runner"`
}

// NewPrometheusMetadataRequest creates a new PrometheusMetadataRequest from the given Runner.
func NewPrometheusMetadataRequest(runner datasource.Runner) *PrometheusMetadataRequest {
	return &PrometheusMetadataRequest{
		Runner: runner,
	}
}

// PrometheusLabelNamesRequest represents a request to fetch label names from a Prometheus data source.
type PrometheusLabelNamesRequest struct {
	// Runner contains the necessary information to fetch label names from the data source.
	Runner datasource.Runner `json:"runner"`

	// Matchers is an optional list of label matchers to filter the label names.
	Matchers []string `json:"matchers,omitempty"`

	// TimeRange specifies the time range for fetching label names, if applicable.
	TimeRange *processor.TimeRange `json:"timeRange,omitempty"`
}

// NewPrometheusLabelNamesRequest creates a new PrometheusLabelNamesRequest from the given Runner, matchers, and time range.
func NewPrometheusLabelNamesRequest(runner datasource.Runner, matchers []string, timeRange *processor.TimeRange) *PrometheusLabelNamesRequest {
	return &PrometheusLabelNamesRequest{
		Runner:    runner,
		Matchers:  matchers,
		TimeRange: timeRange,
	}
}

// PrometheusLabelValuesRequest represents a request to fetch label values for a specific label from a Prometheus data source.
type PrometheusLabelValuesRequest struct {
	// Runner contains the necessary information to fetch label values from the data source.
	Runner datasource.Runner `json:"runner"`

	// Label is the name of the label for which to fetch values.
	Label string `json:"label"`

	// Matchers is an optional list of label matchers to filter the label values.
	Matchers []string `json:"matchers,omitempty"`

	// TimeRange specifies the time range for fetching label values, if applicable.
	TimeRange *processor.TimeRange `json:"timeRange,omitempty"`
}

// NewPrometheusLabelValuesRequest creates a new PrometheusLabelValuesRequest from the given Runner, label, matchers, and time range.
func NewPrometheusLabelValuesRequest(runner datasource.Runner, label string, matchers []string, timeRange *processor.TimeRange) *PrometheusLabelValuesRequest {
	return &PrometheusLabelValuesRequest{
		Runner:    runner,
		Label:     label,
		Matchers:  matchers,
		TimeRange: timeRange,
	}
}

// PrometheusSeriesRequest represents a request to fetch series matching the given selectors from a Prometheus data source.
type PrometheusSeriesRequest struct {
	// Runner contains the necessary information to fetch series from the data source.
	Runner datasource.Runner `json:"runner"`

	// Matchers is a list of label matchers to filter the series.
	Matchers []string `json:"matchers"`

	// TimeRange specifies the time range for fetching series, if applicable.
	TimeRange *processor.TimeRange `json:"timeRange,omitempty"`
}

// NewPrometheusSeriesRequest creates a new PrometheusSeriesRequest from the given Runner, matchers, and time range.
func NewPrometheusSeriesRequest(runner datasource.Runner, matchers []string, timeRange *processor.TimeRange) *PrometheusSeriesRequest {
	return &PrometheusSeriesRequest{
		Runner:    runner,
		Matchers:  matchers,
		TimeRange: timeRange,
	}
}

// PostgreSQLQueryRequest represents a request to execute a SQL query against a PostgreSQL data source.
type PostgreSQLQueryRequest struct {
	// Runner contains the necessary information to execute the query against the data source.
	Runner datasource.Runner `json:"runner"`

	// Query is the SQL query to be executed.
	Query string `json:"query"`

	// TimeRange specifies the time range for the query, if applicable.
	TimeRange *processor.TimeRange `json:"timeRange,omitempty"`
}

// NewPostgreSQLQueryRequest creates a new PostgreSQLQueryRequest from the given Runner, query, and time range.
func NewPostgreSQLQueryRequest(runner datasource.Runner, query string, timeRange *processor.TimeRange) *PostgreSQLQueryRequest {
	return &PostgreSQLQueryRequest{
		Runner:    runner,
		Query:     query,
		TimeRange: timeRange,
	}
}

// MySQLQueryRequest represents a request to execute a SQL query against a MySQL data source.
type MySQLQueryRequest struct {
	// Runner contains the necessary information to execute the query against the data source.
	Runner datasource.Runner `json:"runner"`

	// Query is the SQL query to be executed.
	Query string `json:"query"`

	// TimeRange specifies the time range for the query, if applicable.
	TimeRange *processor.TimeRange `json:"timeRange,omitempty"`
}

// NewMySQLQueryRequest creates a new MySQLQueryRequest from the given Runner, query, and time range.
func NewMySQLQueryRequest(runner datasource.Runner, query string, timeRange *processor.TimeRange) *MySQLQueryRequest {
	return &MySQLQueryRequest{
		Runner:    runner,
		Query:     query,
		TimeRange: timeRange,
	}
}

// SQLQueryLabelsRequest represents a request to fetch label columns and example values from a SQL query.
type SQLQueryLabelsRequest struct {
	// Runner contains the necessary information to connect to the data source.
	Runner datasource.Runner `json:"runner"`

	// Query is the SQL query to be executed.
	Query string `json:"query"`

	// TimeRange specifies the time range for the query, if applicable.
	TimeRange *processor.TimeRange `json:"timeRange,omitempty"`
}

// NewSQLQueryLabelsRequest creates a new SQLQueryLabelsRequest from the given Runner, query, and time range.
func NewSQLQueryLabelsRequest(runner datasource.Runner, query string, timeRange *processor.TimeRange) *SQLQueryLabelsRequest {
	return &SQLQueryLabelsRequest{
		Runner:    runner,
		Query:     query,
		TimeRange: timeRange,
	}
}

// SQLMetadataRequest represents a request to fetch metadata from a SQL data source.
type SQLMetadataRequest struct {
	// Runner contains the necessary information to connect to the data source.
	Runner datasource.Runner `json:"runner"`
}

// NewSQLMetadataRequest creates a new SQLMetadataRequest from the given Runner.
func NewSQLMetadataRequest(runner datasource.Runner) *SQLMetadataRequest {
	return &SQLMetadataRequest{
		Runner: runner,
	}
}

// Response represents the response from the connector service.
type Response struct {
	// Status indicates the connection status after processing the request.
	Status processor.ConnectionStatus `json:"status"`

	// Result contains any additional data returned by the connector, such as query
	// results or error details.
	Result json.RawMessage `json:"result,omitempty"`
}

// NewResponse creates a new Response with the given connection status and result data.
func NewResponse(status processor.ConnectionStatus, result any) (*Response, error) {
	var rawResult json.RawMessage

	if result != nil {
		encodedResult, err := json.Marshal(result)
		if err != nil {
			return nil, err
		}

		rawResult = encodedResult
	}

	return &Response{
		Status: status,
		Result: rawResult,
	}, nil
}
