package datasource

import (
	"context"
	"errors"

	"github.com/oxynote/oxynote/server/core/internal/datasource/processor"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
)

// Runner represents the data source processor with necessary information for processing.
type Runner struct {
	// Type indicates the type of the data source (e.g., Prometheus).
	Type Type `json:"type"`

	// URL is the endpoint URL of the data source.
	URL string `json:"url"`

	// Credentials holds the authentication credentials for the data source.
	Credentials processor.Credentials `json:"-"`

	prepared bool
	runner   runner
}

// NewRunner creates a new Runner instance from the given DataSource.
func NewRunner(ds DataSource) Runner {
	return Runner{
		Type:        ds.Type,
		URL:         ds.URL,
		Credentials: ds.Credentials,
	}
}

// TestConnection tests the connection to the data source.
func (r *Runner) TestConnection(ctx context.Context) (processor.ConnectionStatus, error) {
	if err := r.ensurePrepared(); err != nil {
		return "", err
	}

	return r.runner.TestConnection(ctx)
}

// Prometheus returns a prometheus client or an error in case data source is not of type prometheus.
// The boolean return value indicates whether the data source status has changed.
func (r *Runner) Prometheus(ctx context.Context) (Prometheus, processor.ConnectionStatus, error) {
	if r.Type != TypePrometheus {
		return nil, "", errutil.ErrNotFound
	}

	if err := r.ensurePrepared(); err != nil {
		return nil, "", err
	}

	cs, err := r.runner.TestConnection(ctx)
	if err != nil {
		return nil, "", err
	}

	return r.runner.(*processor.Prometheus), cs, nil //nolint:forcetypeassert // the type is static
}

// PostgreSQL returns a postgresql client or an error in case data source is not of type postgresql.
// The boolean return value indicates whether the data source status has changed.
func (r *Runner) PostgreSQL(ctx context.Context) (PostgreSQL, processor.ConnectionStatus, error) {
	if r.Type != TypePostgreSQL {
		return nil, "", errutil.ErrNotFound
	}

	if err := r.ensurePrepared(); err != nil {
		return nil, "", err
	}

	cs, err := r.runner.TestConnection(ctx)
	if err != nil {
		return nil, "", err
	}

	return r.runner.(*processor.PostgreSQL), cs, nil //nolint:forcetypeassert // the type is static
}

// MySQL returns a mysql client or an error in case data source is not of type mariadb or mysql.
func (r *Runner) MySQL(ctx context.Context) (MySQL, processor.ConnectionStatus, error) {
	if r.Type != TypeMariaDB && r.Type != TypeMySQL {
		return nil, "", errutil.ErrNotFound
	}

	if err := r.ensurePrepared(); err != nil {
		return nil, "", err
	}

	cs, err := r.runner.TestConnection(ctx)
	if err != nil {
		return nil, "", err
	}

	return r.runner.(*processor.MySQL), cs, nil //nolint:forcetypeassert // the type is static
}

// SQL returns a SQL client or an error in case data source is not a SQL-based type.
func (r *Runner) SQL(ctx context.Context) (SQL, processor.ConnectionStatus, error) {
	switch r.Type {
	case TypePostgreSQL:
		return r.PostgreSQL(ctx)
	case TypeMariaDB, TypeMySQL:
		return r.MySQL(ctx)
	default:
		return nil, "", errutil.ErrNotFound
	}
}

// ensurePrepared prepares the hook for processing.
func (r *Runner) ensurePrepared() error {
	if r.prepared {
		return nil
	}

	switch r.Type {
	case TypePrometheus:
		r.runner = processor.NewPrometheus(newStateInput(r.URL, r.Credentials))
		r.prepared = true

		return nil
	case TypePostgreSQL:
		r.runner = processor.NewPostgreSQL(newStateInput(r.URL, r.Credentials))
		r.prepared = true

		return nil
	case TypeMariaDB, TypeMySQL:
		r.runner = processor.NewMySQL(newStateInput(r.URL, r.Credentials))
		r.prepared = true

		return nil
	default:
		return errors.New("invalid data source type")
	}
}

// Prometheus represents a Prometheus data source processor.
type Prometheus interface {
	runner

	// Metadata retrieves metadata about the data source.
	Metadata(ctx context.Context) (*processor.PrometheusMetadataResult, error)

	// QueryRange performs a query against the data source over a specified time range.
	QueryRange(ctx context.Context, q string, tr processor.TimeRange) (*processor.PrometheusQueryResult, error)

	// LabelNames retrieves label names from the data source based on matchers and time range.
	LabelNames(ctx context.Context, matchers []string, tr processor.TimeRange) (*processor.PrometheusLabelNamesResult, error)

	// LabelValues retrieves label values for a specific label from the data source based on matchers and time range.
	LabelValues(ctx context.Context, label string, matchers []string, tr processor.TimeRange) (*processor.PrometheusLabelValuesResult, error)

	// Series retrieves series matching the given selectors from the data source based on matchers and time range.
	Series(ctx context.Context, matchers []string, tr processor.TimeRange) (*processor.PrometheusSeriesResult, error)
}

// SQL represents a SQL-based data source processor.
type SQL interface {
	runner

	// Metadata retrieves all tables and their columns from the data source.
	Metadata(ctx context.Context) (*processor.SQLMetadataResult, error)

	// QueryLabels executes a SQL query with LIMIT 1 and returns string column names with example values.
	QueryLabels(ctx context.Context, q string, tr processor.TimeRange) (map[string]string, error)
}

// PostgreSQL represents a PostgreSQL data source processor.
type PostgreSQL interface {
	SQL

	// Query executes a SQL query against the data source.
	Query(ctx context.Context, q string, tr processor.TimeRange) (*processor.PostgreSQLQueryResult, error)

	// QueryLabels executes a SQL query with LIMIT 1 and returns string column names with example values.
	QueryLabels(ctx context.Context, q string, tr processor.TimeRange) (map[string]string, error)
}

// MySQL represents a MySQL data source processor.
type MySQL interface {
	SQL

	// Query executes a SQL query against the data source.
	Query(ctx context.Context, q string, tr processor.TimeRange) (*processor.MySQLQueryResult, error)

	// QueryLabels executes a SQL query with LIMIT 1 and returns string column names with example values.
	QueryLabels(ctx context.Context, q string, tr processor.TimeRange) (map[string]string, error)
}

// runner represents a data source runner.
type runner interface {
	// TestConnection tests the data source connection.
	TestConnection(ctx context.Context) (processor.ConnectionStatus, error)
}
