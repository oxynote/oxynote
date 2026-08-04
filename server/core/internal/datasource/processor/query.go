package processor

import (
	"math"
	"net/http"

	"github.com/oxynote/oxynote/server/core/pkg/errutil"
)

// ChartType represents the type of chart visualization.
type ChartType string

const (
	// ChartTypeLine represents a line chart visualization.
	ChartTypeLine ChartType = "line_chart"

	// ChartTypeBar represents a bar chart visualization.
	ChartTypeBar ChartType = "bar_chart"

	// ChartTypeGauge represents a gauge chart visualization.
	ChartTypeGauge ChartType = "gauge_chart"
)

// IsValid checks if the chart type is valid.
func (ct ChartType) IsValid() bool {
	switch ct {
	case ChartTypeLine, ChartTypeBar, ChartTypeGauge:
		return true
	default:
		return false
	}
}

// QueryStatus represents the status of a query result.
type QueryStatus string

const (
	// QueryStatusOK indicates a successful query with valid data.
	QueryStatusOK QueryStatus = "ok"

	// QueryStatusNoData indicates the query returned no data.
	QueryStatusNoData QueryStatus = "no-data"

	// QueryStatusTypeNotSelected indicates no chart type was specified.
	QueryStatusTypeNotSelected QueryStatus = "type-not-selected"

	// QueryStatusChartAndDataMismatch indicates the data cannot be rendered as the requested chart type.
	QueryStatusChartAndDataMismatch QueryStatus = "chart-and-data-mismatch"

	// QueryStatusInvalid indicates the query result contains only invalid numeric values.
	QueryStatusInvalid QueryStatus = "invalid"
)

// QueryResultSeries represents a single series in the query result.
type QueryResultSeries struct {
	// Labels contains the labels of the associated metrics.
	Labels map[string]string `json:"labels"`

	// Metrics contains [unix_timestamp_seconds, value] pairs.
	Metrics [][2]any `json:"metrics"`
}

// QueryResult represents the unified response format for data source queries.
type QueryResult struct {
	// Status indicates the result status.
	Status QueryStatus `json:"status"`

	// Data contains the series data.
	Data []QueryResultSeries `json:"data,omitempty"`
}

// isValidValue returns true if the float64 value is a valid numeric value (not NaN or Inf).
func isValidValue(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}

// NewInvalidQueryError creates a new error indicating an invalid query with the provided message.
func NewInvalidQueryError(msg string) error {
	return errutil.New(http.StatusBadRequest, "query.error", "%s", msg)
}
