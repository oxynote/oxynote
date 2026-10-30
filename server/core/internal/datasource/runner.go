package datasource

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/jellydator/xync"
	"github.com/oxynote/oxynote/server/core/internal/datasource/demo"
	"github.com/oxynote/oxynote/server/core/internal/datasource/processor"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/memkit"
	"github.com/oxynote/oxynote/server/core/pkg/metricutil"
	"github.com/oxynote/oxynote/server/core/pkg/redkit"
	"github.com/oxynote/oxynote/server/core/pkg/syncutil"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
	"github.com/rs/xid"
	"golang.org/x/sync/singleflight"
)

const (
	// _statusTTL is how long a passing connection test vouches for a data
	// source before a call tests it again.
	_statusTTL = 10 * time.Second

	// _failureTTL is how long a failed connection test answers for a data
	// source before a call tests it again. Each test of a misconfigured
	// data source is a failed login, and some servers block an address
	// after a few of them.
	_failureTTL = 30 * time.Second

	// _idleAfter is how long a runner may go unasked for before its
	// connections are closed.
	_idleAfter = 5 * time.Minute

	// _sweepInterval is how often the manager looks for idle runners.
	_sweepInterval = time.Minute
)

// _errDataSourceChanged is returned when a data source is edited or
// removed while a call is reading it.
var _errDataSourceChanged = errutil.New(
	http.StatusConflict,
	"data_source.changed",
	"The data source changed while it was being read. Try again.",
)

// Manager hands out the runner for a data source.
//
// It keeps one runner per data source, so the connections a runner opens
// serve every call that reads the same data source. A runner goes, and its
// connections close, when the data source changes, when a connection test
// fails, or when nobody has asked for it in _idleAfter. What the runners
// share lives here too: the store their observations are recorded in, the
// log those recordings complain to, the demo client, and the cache their
// queries are answered from.
type Manager struct {
	log     *slog.Logger
	store   StatusStore
	cache   queryCache
	demo    *demo.Client
	metrics *metrics

	// tests lets one call at a time test a data source's connection. The
	// calls waiting behind it reuse what it found.
	tests *syncutil.KeyedMutex[xid.ID]

	// flights lets identical queries arriving together share one run.
	flights singleflight.Group

	// mu guards runners and the status and usedAt of each runner. It is
	// never held across I/O.
	mu      sync.Mutex
	runners map[xid.ID]*runner

	supv *xync.Supervisor
}

// NewManager creates a fresh instance of Manager. Query answers are kept in
// valkey when pool is set, so every core process shares them, and in this
// process otherwise.
func NewManager(
	log *slog.Logger,
	store StatusStore,
	pool *redis.Pool,
	fc metricutil.Factory,
) *Manager {
	var cache queryCache = memkit.NewBytesStore(_cacheTTL, _cacheMaxBytes)

	if pool != nil {
		cache = redkit.NewBytesStore(pool, _cacheTTL)
	}

	return &Manager{
		log:     log.With("component", "datasource"),
		store:   store,
		cache:   cache,
		demo:    demo.NewClient(),
		metrics: newMetrics(fc),
		tests:   syncutil.NewKeyedMutex[xid.ID](),
		runners: make(map[xid.ID]*runner),
		supv:    xync.NewSupervisor(),
	}
}

// Start runs the upkeep of the query cache and closes the connections of
// idle runners until ctx is cancelled. It blocks, so the caller owns the
// goroutine it runs on.
func (m *Manager) Start(ctx context.Context) {
	supv := xync.NewSupervisor(xync.WithSupervisorBaseContext(ctx))
	defer supv.Close()

	supv.Go(m.cache.Start)
	supv.Go(timeutil.NewPeriodicExec(_sweepInterval, 0, m.closeIdle, nil, false).Start)

	supv.Wait()
}

// Close closes the connections of every runner, waiting for the queries
// still running on them.
func (m *Manager) Close() error {
	m.mu.Lock()

	for id, r := range m.runners {
		delete(m.runners, id)
		m.release(r)
	}

	m.mu.Unlock()

	m.supv.CloseAndWait()

	return nil
}

// Runner returns the runner that operates the given data source: the one
// the manager keeps while the data source is unchanged, or a new one.
//
// A new runner is kept only once a query connects through it (see adopt),
// so a connection test of a data source being created or edited never
// replaces the runner its readers use.
func (m *Manager) Runner(ds DataSource) Runner {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := timeutil.Now()

	r, ok := m.runners[ds.ID]
	if ok && r.ds.UpdatedAt.Equal(ds.UpdatedAt) {
		// the caller read the stored status just now, so a status observed
		// later has to be compared with this one.
		r.status = ds.Status
		r.usedAt = now

		return r
	}

	r = &runner{
		m:      m,
		ds:     ds,
		status: ds.Status,
		usedAt: now,
	}
	r.client, r.clientErr = m.newClient(ds)

	return r
}

// adopt keeps r as the runner of its data source, closing the connections
// of the one kept for an earlier version of it. When another call already
// kept a runner for the same version, that one is returned instead; r has
// not connected yet, so dropping it leaves nothing open. A runner the
// manager already let go is not kept again.
func (m *Manager) adopt(r *runner) *runner {
	m.mu.Lock()
	defer m.mu.Unlock()

	kept, ok := m.runners[r.ds.ID]

	switch {
	case r.released, ok && kept == r:
		return r
	case ok && kept.ds.UpdatedAt.Equal(r.ds.UpdatedAt):
		kept.status = r.status
		kept.usedAt = timeutil.Now()

		return kept
	case ok:
		m.release(kept)
	}

	m.runners[r.ds.ID] = r

	return r
}

// Evict closes the connections of the data source the id names. The next
// call builds a runner from the data source as it is then.
func (m *Manager) Evict(id xid.ID) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if r, ok := m.runners[id]; ok {
		delete(m.runners, id)
		m.release(r)
	}
}

// renew replaces r, whose connection test failed, with a runner of the same
// data source that remembers the failure, and closes the connections of r.
// A session keeps the settings it started with, so a test on them would
// keep failing after the data source is fixed. A runner the manager already
// let go is left as it is.
func (m *Manager) renew(r *runner) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.runners[r.ds.ID] != r {
		return
	}

	nr := &runner{
		m:        m,
		ds:       r.ds,
		status:   r.status,
		usedAt:   r.usedAt,
		testedAt: r.testedAt,
		tested:   r.tested,
	}
	nr.client, nr.clientErr = m.newClient(r.ds)

	m.runners[r.ds.ID] = nr
	m.release(r)
}

// keeps reports whether r is the runner the manager keeps for its data
// source.
func (m *Manager) keeps(r *runner) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.runners[r.ds.ID] == r
}

// closeIdle closes the connections of the runners nobody asked for in
// _idleAfter.
func (m *Manager) closeIdle(context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := timeutil.Now()

	for id, r := range m.runners {
		if now.Sub(r.usedAt) >= _idleAfter {
			delete(m.runners, id)
			m.release(r)
		}
	}
}

// release closes the connections of a runner the caller took out of the
// map while holding m.mu. Closing waits for the queries still running on
// them, so it happens off the caller's path.
func (m *Manager) release(r *runner) {
	r.released = true

	if r.client == nil {
		return
	}

	m.supv.Go(func(context.Context) {
		if err := r.client.Close(); err != nil {
			m.log.Error(
				"cannot close data source connections",
				slog.String("data_source_id", r.ds.ID.String()),
				slog.String("error", err.Error()),
			)
		}
	})
}

// newClient builds the processor the data source's type calls for.
func (m *Manager) newClient(ds DataSource) (ownedClient, error) {
	switch ds.Type {
	case TypePrometheus:
		return m.prometheusClient(ds)
	case TypePostgreSQL:
		return processor.NewPostgreSQL(newStateInput(ds.URL, ds.Credentials)), nil
	case TypeSupabase:
		return processor.NewSupabase(newStateInput(ds.URL, ds.Credentials)), nil
	case TypeMariaDB, TypeMySQL:
		return processor.NewMySQL(newStateInput(ds.URL, ds.Credentials)), nil
	case TypeLoki:
		return m.lokiClient(ds)
	default:
		return nil, errors.New("invalid data source type")
	}
}

// prometheusClient resolves which Prometheus a data source speaks to. The
// demo scheme names a source this process synthesizes rather than one it
// dials, and only the one demo source exists — a URL that merely looks
// like it is a mistake worth reporting, not a second demo.
func (m *Manager) prometheusClient(ds DataSource) (ownedClient, error) {
	if ds.URL == demo.URL {
		return m.demo, nil
	}

	if strings.HasPrefix(ds.URL, demo.Scheme) {
		return nil, demo.ErrUnknownSource
	}

	return processor.NewPrometheus(newStateInput(ds.URL, ds.Credentials)), nil
}

// lokiClient resolves which Loki a data source speaks to. The demo URL
// is the same one the demo Prometheus answers at, served as Loki for this
// type.
func (m *Manager) lokiClient(ds DataSource) (ownedClient, error) {
	if ds.URL == demo.URL {
		return m.demo, nil
	}

	if strings.HasPrefix(ds.URL, demo.Scheme) {
		return nil, demo.ErrUnknownSource
	}

	return processor.NewLoki(newStateInput(ds.URL, ds.Credentials)), nil
}

// runner operates one data source. The manager keeps it, so every call
// that reads the data source shares it and the connections its client
// holds.
type runner struct {
	// m is the manager that keeps the runner.
	m *Manager

	// ds is the data source the runner was built from.
	ds DataSource

	// client is the processor the data source's type resolves to, or nil
	// when clientErr says why there is none.
	client    ownedClient
	clientErr error

	// status is the connection status stored for the data source, as far
	// as this process knows. Guarded by m.mu.
	status processor.ConnectionStatus

	// usedAt is when a call last asked the manager for the runner.
	// Guarded by m.mu.
	usedAt time.Time

	// released indicates whether the manager let the runner go and closed
	// its connections. Guarded by m.mu.
	released bool

	// testedAt is when the connection was last tested, or zero, and tested
	// is what that test found. Guarded by the manager's test lock for the
	// data source.
	testedAt time.Time
	tested   processor.ConnectionStatus
}

// Type reports what the data source speaks.
func (r *runner) Type() Type {
	return r.ds.Type
}

// TestConnection tests the connection to the data source and reports
// what it found.
//
// It does not record the answer, and it tests through a client of its own,
// closed afterwards, rather than the runner's. Callers ask this about a data
// source that is being created or changed, one whose stored row is not the
// thing being described yet, or about one a person asked to check, which a
// test remembered from a moment ago would not answer. The accessors below
// ask it about a data source in use, which is the observation worth keeping.
func (r *runner) TestConnection(ctx context.Context) (processor.ConnectionStatus, error) {
	if !r.ds.Credentials.IsValid() {
		return processor.ConnectionStatusInvalidEncryptionKey, nil
	}

	client, err := r.m.newClient(r.ds)
	if err != nil {
		return "", err
	}
	defer client.Close() //nolint:errcheck // error provides no meaningful info

	return client.TestConnection(ctx)
}

// Prometheus returns the data source's Prometheus client, whose range
// queries are answered out of the query cache.
func (r *runner) Prometheus(ctx context.Context) (Prometheus, error) {
	client, err := connect[Prometheus](ctx, r, TypePrometheus)
	if err != nil {
		return nil, err
	}

	return cachedPrometheus{Prometheus: client, m: r.m, ds: r.ds}, nil
}

// PostgreSQL returns the data source's PostgreSQL client, whose queries are
// answered out of the query cache.
func (r *runner) PostgreSQL(ctx context.Context) (PostgreSQL, error) {
	client, err := connect[PostgreSQL](ctx, r, TypePostgreSQL, TypeSupabase)
	if err != nil {
		return nil, err
	}

	return cachedPostgreSQL{PostgreSQL: client, m: r.m, ds: r.ds}, nil
}

// MySQL returns the data source's MySQL client, whose queries are answered
// out of the query cache.
func (r *runner) MySQL(ctx context.Context) (MySQL, error) {
	client, err := connect[MySQL](ctx, r, TypeMariaDB, TypeMySQL)
	if err != nil {
		return nil, err
	}

	return cachedMySQL{MySQL: client, m: r.m, ds: r.ds}, nil
}

// SQL returns the data source's dialect-agnostic SQL client.
func (r *runner) SQL(ctx context.Context) (SQL, error) {
	return connect[SQL](ctx, r, TypePostgreSQL, TypeSupabase, TypeMariaDB, TypeMySQL)
}

// Logs returns the data source's log client, whose reads are answered out
// of the query cache.
func (r *runner) Logs(ctx context.Context) (Logs, error) {
	client, err := connect[Logs](ctx, r, TypeLoki)
	if err != nil {
		return nil, err
	}

	return cachedLogs{client: client, m: r.m, ds: r.ds}, nil
}

// connect resolves the client of the wanted type, testing the connection
// unless a test passed a moment ago.
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
		r.recordStatus(ctx, processor.ConnectionStatusInvalidEncryptionKey)

		return zero, processor.ConnectionStatusInvalidEncryptionKey.Error()
	}

	if r.clientErr != nil {
		return zero, r.clientErr
	}

	r = r.m.adopt(r)

	if err := r.ensureTested(ctx); err != nil {
		return zero, err
	}

	client, ok := r.client.(C)
	if !ok {
		// NOCOV: the type check above admits only the processors that
		// satisfy C, so this cannot be reached.
		return zero, errutil.ErrNotFound
	}

	return client, nil
}

// ensureTested tests the connection unless it passed a test within
// _statusTTL or failed one within _failureTTL, and records what it found.
// A failing test renews the runner. The new one remembers the failure, so
// it answers the calls that follow without a new login, and its next test
// opens new connections.
//
// One call at a time tests a data source, and the calls waiting behind it
// reuse what it found, a failure included. A document opening many charts
// at once would otherwise test the same connection once per chart.
func (r *runner) ensureTested(ctx context.Context) error {
	unlock, err := r.m.tests.Lock(ctx, r.ds.ID)
	if err != nil {
		return err
	}
	defer unlock()

	ttl := _statusTTL
	if r.tested.Error() != nil {
		ttl = _failureTTL
	}

	fresh := timeutil.Now().Sub(r.testedAt) < ttl

	// a recent failure answers every call, the ones holding a runner the
	// manager let go since included.
	if fresh {
		if serr := r.tested.Error(); serr != nil {
			return serr
		}
	}

	// a runner the manager let go has its connections closed, so it can
	// neither hand them out nor test them.
	if !r.m.keeps(r) {
		return _errDataSourceChanged
	}

	if fresh {
		return nil
	}

	cs, err := r.client.TestConnection(ctx)
	if err != nil {
		return err
	}

	// a test the caller gave up on says nothing about the data source, and
	// neither does one whose connections were closed under it.
	if cerr := ctx.Err(); cerr != nil {
		return cerr
	}

	if !r.m.keeps(r) {
		return _errDataSourceChanged
	}

	r.recordStatus(ctx, cs)
	r.testedAt = timeutil.Now()
	r.tested = cs

	if serr := cs.Error(); serr != nil {
		r.m.renew(r)

		return serr
	}

	return nil
}

// recordStatus keeps the stored status honest about what the connection
// last reported.
//
// A status that has not changed is not written, and a write that fails
// is logged rather than returned: the caller asked to read the data
// source, and what its row says about connectivity is not the answer
// they were waiting for.
func (r *runner) recordStatus(ctx context.Context, cs processor.ConnectionStatus) {
	r.m.mu.Lock()
	unchanged := cs == r.status
	r.status = cs
	r.m.mu.Unlock()

	if unchanged || r.m.store == nil {
		return
	}

	if err := r.m.store.UpdateDataSourceStatus(ctx, r.ds.ID, r.ds.OrganizationID, cs); err != nil {
		r.m.log.Error(
			"cannot record data source status",
			slog.String("data_source_id", r.ds.ID.String()),
			slog.String("status", string(cs)),
			slog.String("error", err.Error()),
		)
	}
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

	// Logs should return the data source's log client.
	Logs(ctx context.Context) (Logs, error)
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
//go:generate ../../scripts/codegen/mock SQL sql
type SQL interface {
	connectionTester

	// Metadata retrieves all tables and their columns from the data source.
	Metadata(ctx context.Context) (*processor.SQLMetadataResult, error)

	// QueryLabels executes a SQL query with LIMIT 1 and returns string column names with example values.
	QueryLabels(ctx context.Context, q string, tr processor.TimeRange) (map[string]string, error)
}

// PostgreSQL represents a PostgreSQL data source processor.
//
//go:generate ../../scripts/codegen/mock PostgreSQL postgresql
type PostgreSQL interface {
	SQL

	// Query executes a SQL query against the data source.
	Query(ctx context.Context, q string, tr processor.TimeRange) (*processor.PostgreSQLQueryResult, error)
}

// MySQL represents a MySQL data source processor.
//
//go:generate ../../scripts/codegen/mock MySQL mysql
type MySQL interface {
	SQL

	// Query executes a SQL query against the data source.
	Query(ctx context.Context, q string, tr processor.TimeRange) (*processor.MySQLQueryResult, error)
}

// Logs represents a log data source processor.
//
//go:generate ../../scripts/codegen/mock Logs logs
type Logs interface {
	connectionTester

	// Logs should read one page of the lines q matches in the window.
	Logs(ctx context.Context, q string, tr processor.TimeRange, page processor.LogsPage) (*processor.LogsResult, error)

	// LogsVolume should count the lines q matches per time bucket, split
	// by level.
	LogsVolume(ctx context.Context, q string, tr processor.TimeRange) (*processor.QueryResult, error)

	// LogFields should list the field names the lines carry in the
	// window, narrowed to the lines q matches when q is given.
	LogFields(ctx context.Context, q string, tr processor.TimeRange) (*processor.LogFieldsResult, error)

	// LogFieldValues should list the values one field takes in the
	// window, narrowed to the lines q matches when q is given.
	LogFieldValues(ctx context.Context, field, q string, tr processor.TimeRange) (*processor.LogFieldValuesResult, error)
}

// connectionTester is what every processor can do regardless of what it
// speaks.
type connectionTester interface {
	// TestConnection tests the data source connection.
	TestConnection(ctx context.Context) (processor.ConnectionStatus, error)
}

// ownedClient is the processor a runner owns. Close stays off the
// interfaces the accessors hand out: their callers share the connections
// it closes.
type ownedClient interface {
	connectionTester

	// Close closes the connections the processor holds.
	io.Closer
}
