package datasource

import (
	"context"

	"github.com/oxynote/oxynote/server/core/internal/datasource/processor"
)

// Executor runs data source operations in-process.
type Executor struct{}

// NewExecutor creates a fresh instance of Executor.
func NewExecutor() *Executor {
	return &Executor{}
}

// TestConnection tests the connection to a data source.
func (e Executor) TestConnection(ctx context.Context, ds DataSource) (processor.ConnectionStatus, error) {
	runner := NewRunner(ds)

	return runner.TestConnection(ctx)
}

// PrometheusQuery executes a Prometheus query against a data source.
func (e Executor) PrometheusQuery(ctx context.Context, ds DataSource, query string, tr processor.TimeRange) (processor.ConnectionStatus, *processor.PrometheusQueryResult, error) {
	runner := NewRunner(ds)

	prom, cs, err := runner.Prometheus(ctx)
	if err != nil {
		return "", nil, err
	}

	if cs != processor.ConnectionStatusSuccess {
		return cs, nil, nil
	}

	result, err := prom.QueryRange(ctx, query, tr)
	if err != nil {
		return "", nil, err
	}

	return cs, result, nil
}

// PrometheusMetadata retrieves metadata from a Prometheus data source.
func (e Executor) PrometheusMetadata(ctx context.Context, ds DataSource) (processor.ConnectionStatus, *processor.PrometheusMetadataResult, error) {
	runner := NewRunner(ds)

	prom, cs, err := runner.Prometheus(ctx)
	if err != nil {
		return "", nil, err
	}

	if cs != processor.ConnectionStatusSuccess {
		return cs, nil, nil
	}

	result, err := prom.Metadata(ctx)
	if err != nil {
		return "", nil, err
	}

	return cs, result, nil
}

// PrometheusLabelNames retrieves label names from a Prometheus data source.
func (e Executor) PrometheusLabelNames(ctx context.Context, ds DataSource, matchers []string, tr processor.TimeRange) (processor.ConnectionStatus, *processor.PrometheusLabelNamesResult, error) {
	runner := NewRunner(ds)

	prom, cs, err := runner.Prometheus(ctx)
	if err != nil {
		return "", nil, err
	}

	if cs != processor.ConnectionStatusSuccess {
		return cs, nil, nil
	}

	result, err := prom.LabelNames(ctx, matchers, tr)
	if err != nil {
		return "", nil, err
	}

	return cs, result, nil
}

// PrometheusLabelValues retrieves label values for a specific label from a Prometheus data source.
func (e Executor) PrometheusLabelValues(ctx context.Context, ds DataSource, label string, matchers []string, tr processor.TimeRange) (processor.ConnectionStatus, *processor.PrometheusLabelValuesResult, error) {
	runner := NewRunner(ds)

	prom, cs, err := runner.Prometheus(ctx)
	if err != nil {
		return "", nil, err
	}

	if cs != processor.ConnectionStatusSuccess {
		return cs, nil, nil
	}

	result, err := prom.LabelValues(ctx, label, matchers, tr)
	if err != nil {
		return "", nil, err
	}

	return cs, result, nil
}

// PrometheusSeries retrieves series matching the given selectors from a Prometheus data source.
func (e Executor) PrometheusSeries(ctx context.Context, ds DataSource, matchers []string, tr processor.TimeRange) (processor.ConnectionStatus, *processor.PrometheusSeriesResult, error) {
	runner := NewRunner(ds)

	prom, cs, err := runner.Prometheus(ctx)
	if err != nil {
		return "", nil, err
	}

	if cs != processor.ConnectionStatusSuccess {
		return cs, nil, nil
	}

	result, err := prom.Series(ctx, matchers, tr)
	if err != nil {
		return "", nil, err
	}

	return cs, result, nil
}

// MySQLQuery executes a SQL query against a MySQL data source.
func (e Executor) MySQLQuery(ctx context.Context, ds DataSource, query string, tr processor.TimeRange) (processor.ConnectionStatus, *processor.MySQLQueryResult, error) {
	runner := NewRunner(ds)

	mdb, cs, err := runner.MySQL(ctx)
	if err != nil {
		return "", nil, err
	}

	if cs != processor.ConnectionStatusSuccess {
		return cs, nil, nil
	}

	result, err := mdb.Query(ctx, query, tr)
	if err != nil {
		return "", nil, err
	}

	return cs, result, nil
}

// PostgreSQLQuery executes a SQL query against a PostgreSQL data source.
func (e Executor) PostgreSQLQuery(ctx context.Context, ds DataSource, query string, tr processor.TimeRange) (processor.ConnectionStatus, *processor.PostgreSQLQueryResult, error) {
	runner := NewRunner(ds)

	pg, cs, err := runner.PostgreSQL(ctx)
	if err != nil {
		return "", nil, err
	}

	if cs != processor.ConnectionStatusSuccess {
		return cs, nil, nil
	}

	result, err := pg.Query(ctx, query, tr)
	if err != nil {
		return "", nil, err
	}

	return cs, result, nil
}

// SQLQueryLabels executes a query with LIMIT 1 and returns string column names with example values.
func (e Executor) SQLQueryLabels(ctx context.Context, ds DataSource, query string, tr processor.TimeRange) (processor.ConnectionStatus, map[string]string, error) {
	runner := NewRunner(ds)

	sqlDS, cs, err := runner.SQL(ctx)
	if err != nil {
		return "", nil, err
	}

	if cs != processor.ConnectionStatusSuccess {
		return cs, nil, nil
	}

	result, err := sqlDS.QueryLabels(ctx, query, tr)
	if err != nil {
		return "", nil, err
	}

	return cs, result, nil
}

// SQLMetadata retrieves all tables and their columns from a SQL data source.
func (e Executor) SQLMetadata(ctx context.Context, ds DataSource) (processor.ConnectionStatus, *processor.SQLMetadataResult, error) {
	runner := NewRunner(ds)

	sqlDS, cs, err := runner.SQL(ctx)
	if err != nil {
		return "", nil, err
	}

	if cs != processor.ConnectionStatusSuccess {
		return cs, nil, nil
	}

	result, err := sqlDS.Metadata(ctx)
	if err != nil {
		return "", nil, err
	}

	return cs, result, nil
}
