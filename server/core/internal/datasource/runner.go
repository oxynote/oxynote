package datasource

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"github.com/oxynote/oxynote/server/core/internal/datasource/demo"
	"github.com/oxynote/oxynote/server/core/internal/datasource/processor"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/rs/xid"
)

// Manager hands out the runner for a data source.
//
// It is what the per-data-source runners share: the store their
// observations are recorded in, and the log those recordings complain
// to. A runner is built per call and holds only the data source it
// reads, so anything longer-lived than one call belongs here.
type Manager struct {
	log   *slog.Logger
	store StatusStore
	demo  *demo.Client
}

// NewManager creates a fresh instance of Manager.
func NewManager(log *slog.Logger, store StatusStore) *Manager {
	return &Manager{
		log:   log.With("component", "datasource"),
		store: store,
		demo:  demo.NewClient(),
	}
}

// Runner returns the runner that operates the given data source.
func (m *Manager) Runner(ds DataSource) Runner {
	return &runner{ds: ds, log: m.log, store: m.store, demo: m.demo}
}

// runner operates one data source.
type runner struct {
	// ds is the data source being operated, which is everything a
	// connection needs plus the identity its status is recorded under.
	ds DataSource

	// log records a status that could not be stored.
	log *slog.Logger

	// store is where an observed status is recorded.
	store StatusStore

	// prepared reports whether client holds the processor for this data
	// source's type yet.
	prepared bool

	// client is the processor this runner's type resolves to.
	client connectionTester

	// demo answers the data source this process synthesizes, shared with
	// every other runner the manager hands out.
	demo *demo.Client
}

// Type reports what the data source speaks.
func (r *runner) Type() Type {
	return r.ds.Type
}

// TestConnection tests the connection to the data source and reports
// what it found.
//
// It does not record the answer. Callers ask this about a data source
// that is being created or changed — one whose stored row is not the
// thing being described yet — while the accessors below ask it about a
// data source in use, which is the observation worth keeping.
func (r *runner) TestConnection(ctx context.Context) (processor.ConnectionStatus, error) {
	if !r.ds.Credentials.IsValid() {
		return processor.ConnectionStatusInvalidSigningSecret, nil
	}

	if err := r.ensurePrepared(); err != nil {
		return "", err
	}

	return r.client.TestConnection(ctx)
}

// Prometheus returns the data source's Prometheus client.
func (r *runner) Prometheus(ctx context.Context) (Prometheus, error) {
	return connect[Prometheus](ctx, r, TypePrometheus)
}

// PostgreSQL returns the data source's PostgreSQL client.
func (r *runner) PostgreSQL(ctx context.Context) (PostgreSQL, error) {
	return connect[PostgreSQL](ctx, r, TypePostgreSQL)
}

// MySQL returns the data source's MySQL client.
func (r *runner) MySQL(ctx context.Context) (MySQL, error) {
	return connect[MySQL](ctx, r, TypeMariaDB, TypeMySQL)
}

// SQL returns the data source's dialect-agnostic SQL client.
func (r *runner) SQL(ctx context.Context) (SQL, error) {
	return connect[SQL](ctx, r, TypePostgreSQL, TypeMariaDB, TypeMySQL)
}

// connect resolves the client of the wanted type, tests the connection
// and records what it found.
//
// A data source that cannot serve the client asked for is an error
// rather than a status the caller has to inspect: there is nothing to
// return but the reason, and folding the two together is what keeps a
// caller from reading an empty result as an empty data source.
func connect[C any](ctx context.Context, r *runner, allowed ...Type) (C, error) {
	var zero C

	if !slices.Contains(allowed, r.ds.Type) {
		return zero, errutil.New(
			http.StatusBadRequest,
			"data_source.type_not_supported",
			"Data source %q is a %s data source, which does not serve this operation.",
			r.ds.Name, r.ds.Type,
		)
	}

	// credentials that cannot be decrypted are not a connection worth
	// attempting: there is nothing left to authenticate with, and the data
	// source would answer for the empty login rather than for the reason.
	if !r.ds.Credentials.IsValid() {
		r.recordStatus(ctx, processor.ConnectionStatusInvalidSigningSecret)

		return zero, processor.ConnectionStatusInvalidSigningSecret.Error()
	}

	if err := r.ensurePrepared(); err != nil {
		return zero, err
	}

	cs, err := r.client.TestConnection(ctx)
	if err != nil {
		return zero, err
	}

	r.recordStatus(ctx, cs)

	if serr := cs.Error(); serr != nil {
		return zero, serr
	}

	client, ok := r.client.(C)
	if !ok {
		// NOCOV: the type check above admits only the processors that
		// satisfy C, so this cannot be reached.
		return zero, errutil.ErrNotFound
	}

	return client, nil
}

// recordStatus keeps the stored status honest about what the connection
// last reported.
//
// A status that has not changed is not written, and a write that fails
// is logged rather than returned: the caller asked to read the data
// source, and what its row says about connectivity is not the answer
// they were waiting for.
func (r *runner) recordStatus(ctx context.Context, cs processor.ConnectionStatus) {
	if cs == r.ds.Status || r.store == nil {
		return
	}

	r.ds.Status = cs

	if err := r.store.UpdateDataSourceStatus(ctx, r.ds.ID, r.ds.OrganizationID, cs); err != nil {
		r.log.Error(
			"cannot record data source status",
			slog.String("data_source_id", r.ds.ID.String()),
			slog.String("status", string(cs)),
			slog.String("error", err.Error()),
		)
	}
}

// ensurePrepared builds the processor this data source's type calls for.
func (r *runner) ensurePrepared() error {
	if r.prepared {
		return nil
	}

	switch r.ds.Type {
	case TypePrometheus:
		client, err := r.prometheusClient()
		if err != nil {
			return err
		}

		r.client = client
	case TypePostgreSQL:
		r.client = processor.NewPostgreSQL(newStateInput(r.ds.URL, r.ds.Credentials))
	case TypeMariaDB, TypeMySQL:
		r.client = processor.NewMySQL(newStateInput(r.ds.URL, r.ds.Credentials))
	default:
		return errors.New("invalid data source type")
	}

	r.prepared = true

	return nil
}

// prometheusClient resolves which Prometheus a data source speaks to. The
// demo scheme names a source this process synthesizes rather than one it
// dials, and only the one demo source exists — a URL that merely looks
// like it is a mistake worth reporting, not a second demo.
func (r *runner) prometheusClient() (connectionTester, error) {
	if r.ds.URL == demo.URL {
		return r.demo, nil
	}

	if strings.HasPrefix(r.ds.URL, demo.Scheme) {
		return nil, demo.ErrUnknownSource
	}

	return processor.NewPrometheus(newStateInput(r.ds.URL, r.ds.Credentials)), nil
}

// Runner operates one data source: it hands out the typed client for
// the operation being performed, or the reason it cannot.
//
//go:generate ../../scripts/codegen/mock Runner
type Runner interface {
	// Type should report what the data source speaks, for the caller
	// that has to pick a dialect.
	Type() Type

	// TestConnection should test the connection and report what it
	// found, without recording it.
	TestConnection(ctx context.Context) (processor.ConnectionStatus, error)

	// Prometheus should return the data source's Prometheus client.
	Prometheus(ctx context.Context) (Prometheus, error)

	// PostgreSQL should return the data source's PostgreSQL client.
	PostgreSQL(ctx context.Context) (PostgreSQL, error)

	// MySQL should return the data source's MySQL client.
	MySQL(ctx context.Context) (MySQL, error)

	// SQL should return the data source's dialect-agnostic SQL client.
	SQL(ctx context.Context) (SQL, error)
}

// StatusStore records what a data source's connection last reported.
//
//go:generate ../../scripts/codegen/mock -t both StatusStore status_store
type StatusStore interface {
	// UpdateDataSourceStatus should store the status observed for the
	// data source the id names.
	UpdateDataSourceStatus(ctx context.Context, id xid.ID, organizationID string, status processor.ConnectionStatus) error
}

// Prometheus represents a Prometheus data source processor.
//
//go:generate ../../scripts/codegen/mock Prometheus
type Prometheus interface {
	connectionTester

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
//
//go:generate ../../scripts/codegen/mock SQL
type SQL interface {
	connectionTester

	// Metadata retrieves all tables and their columns from the data source.
	Metadata(ctx context.Context) (*processor.SQLMetadataResult, error)

	// QueryLabels executes a SQL query with LIMIT 1 and returns string column names with example values.
	QueryLabels(ctx context.Context, q string, tr processor.TimeRange) (map[string]string, error)
}

// PostgreSQL represents a PostgreSQL data source processor.
//
//go:generate ../../scripts/codegen/mock PostgreSQL
type PostgreSQL interface {
	SQL

	// Query executes a SQL query against the data source.
	Query(ctx context.Context, q string, tr processor.TimeRange) (*processor.PostgreSQLQueryResult, error)
}

// MySQL represents a MySQL data source processor.
//
//go:generate ../../scripts/codegen/mock MySQL
type MySQL interface {
	SQL

	// Query executes a SQL query against the data source.
	Query(ctx context.Context, q string, tr processor.TimeRange) (*processor.MySQLQueryResult, error)
}

// connectionTester is what every processor can do regardless of what it
// speaks.
type connectionTester interface {
	// TestConnection tests the data source connection.
	TestConnection(ctx context.Context) (processor.ConnectionStatus, error)
}
