// Package job runs durable background jobs queued inside database
// transactions.
package job

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/dbos-inc/dbos-transact-golang/dbos"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oxynote/oxynote/server/core/pkg/logutil"
	"github.com/oxynote/oxynote/server/core/pkg/sqlutil"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
)

const (
	// _appName names core's executor in the DBOS tables.
	_appName = "oxynote-core"

	// _appVersion is the fixed application version every job is queued
	// with. DBOS defaults to a hash of the binary, and a worker only
	// dequeues jobs of its own version, so jobs queued before a deploy
	// would never run after it.
	_appVersion = "1"
)

var (
	// _cleanupBatch is how many finished jobs one cleanup query deletes.
	_cleanupBatch = 500

	// _criticalAttempt is the failed attempt from which a job's failures
	// are reported as critical. With the delay capped at _retryMaxDelay,
	// it is reached about half an hour after the first failure.
	_criticalAttempt = 10

	// _retryBaseDelay is the delay before a failed job's first retry.
	_retryBaseDelay = time.Second

	// _retryMaxDelay caps the delay between retries.
	_retryMaxDelay = 5 * time.Minute

	// _cleanupInterval is how often finished jobs are deleted.
	_cleanupInterval = time.Hour

	// _retention is how long a finished job is kept.
	_retention = 24 * time.Hour

	// _shutdownTimeout bounds how long Close waits for running jobs.
	_shutdownTimeout = 10 * time.Second
)

// All available QueueName constants. Every name is declared here, so no
// two queues share one.
const (
	// QueueIndexSearch holds the jobs that bring the search index in line
	// with the database.
	QueueIndexSearch QueueName = "index-search"
)

// QueueName names a job queue.
type QueueName string

// Manager owns the DBOS runtime that stores, schedules and runs jobs.
type Manager struct {
	log   *slog.Logger
	pool  *pgxpool.Pool
	dbos  dbos.Context
	names map[QueueName]struct{}
	pe    *timeutil.PeriodicExec
}

// NewManager creates a Manager that keeps its state in the dbos schema of
// the database the DSN points to.
func NewManager(ctx context.Context, log *slog.Logger, dsn string) (*Manager, error) {
	if dsn == "" {
		return nil, errors.New("dsn is required")
	}

	log = log.With("component", "job-manager")

	// a pool of our own keeps DBOS from connecting to the postgres
	// database to create the target one, which an external deployment
	// may not allow. DBOS never closes it, so Close does.
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("creating the connection pool: %w", err)
	}

	dc, err := dbos.NewContext(ctx, dbos.Config{
		AppName:            _appName,
		ApplicationVersion: _appVersion,
		SystemDBPool:       pool,
		Logger:             log,
	})
	if err != nil {
		pool.Close()

		return nil, fmt.Errorf("creating dbos context: %w", err)
	}

	m := &Manager{
		log:   log,
		pool:  pool,
		dbos:  dc,
		names: make(map[QueueName]struct{}),
	}

	m.pe = timeutil.NewPeriodicExec(
		_cleanupInterval,
		0,
		m.cleanup,
		logutil.RecoveryValue(m.log, logutil.NewRecoveryPlan("recovered from a panic while deleting finished jobs")),
		true,
	)

	return m, nil
}

// Launch starts running queued jobs, including those left pending by a
// previous process. Every queue must be registered before it.
func (m *Manager) Launch() error {
	return dbos.Launch(m.dbos)
}

// Start deletes finished jobs on every cleanup interval until the context
// ends.
func (m *Manager) Start(ctx context.Context) {
	m.log.Info("starting")
	defer m.log.Info("stopped")

	m.pe.Start(ctx)
}

// Close stops running jobs and releases the database connections. A job
// cut short is resumed by the next process.
func (m *Manager) Close() error {
	// running jobs use the pool until DBOS stops them, so it closes last.
	defer m.pool.Close()

	return dbos.Shutdown(m.dbos, _shutdownTimeout)
}

// cleanup deletes the jobs that finished longer than the retention ago.
func (m *Manager) cleanup(ctx context.Context) {
	if err := m.deleteFinished(ctx, timeutil.Now().Add(-_retention)); err != nil {
		m.log.With("error", err).
			Error("cannot delete finished jobs")
	}
}

// deleteFinished deletes, in batches, the jobs that finished before the
// given time.
func (m *Manager) deleteFinished(ctx context.Context, before time.Time) error {
	dc := dbos.From(m.dbos, ctx)

	for {
		wfs, err := dbos.ListWorkflows(
			dc,
			dbos.WithFilterStatus(dbos.WorkflowStatusSuccess, dbos.WorkflowStatusError),
			dbos.WithFilterCompletedBefore(before),
			dbos.WithFilterLimit(_cleanupBatch),
			dbos.WithFilterLoadInput(false),
			dbos.WithFilterLoadOutput(false),
		)
		if err != nil {
			return fmt.Errorf("listing finished jobs: %w", err)
		}

		if len(wfs) == 0 {
			return nil
		}

		ids := make([]string, len(wfs))
		for i, wf := range wfs {
			ids[i] = wf.ID
		}

		if err := dbos.DeleteWorkflows(dc, ids); err != nil {
			return fmt.Errorf("deleting finished jobs: %w", err)
		}

		if len(wfs) < _cleanupBatch {
			return nil
		}
	}
}

// Register adds the queue to the manager. Every queue must be registered
// before Launch.
func (m *Manager) Register(q Registrable) error {
	return q.register(m)
}

// Queue holds the jobs of one kind, each carrying an input of type T.
type Queue[T any] struct {
	log    *slog.Logger
	dbos   dbos.Context
	name   QueueName
	handle func(ctx context.Context, inp T) error
	opts   QueueOptions
}

// QueueOptions configures a queue.
type QueueOptions struct {
	// Concurrency specifies how many jobs of the queue run at once in this
	// process. It must be positive.
	Concurrency int

	// PollInterval specifies how often the queue is checked for jobs ready
	// to run. A queue takes at most Concurrency jobs per check, so it also
	// bounds throughput. It must be positive.
	PollInterval time.Duration
}

// validate checks that the options are usable.
func (opts QueueOptions) validate() error {
	if opts.Concurrency <= 0 {
		return errors.New("concurrency must be positive")
	}

	if opts.PollInterval <= 0 {
		return errors.New("poll interval must be positive")
	}

	return nil
}

// NewQueue creates the queue with the given name. Its jobs run through
// handle once a Manager registers it. A job whose handle fails is queued
// again with a growing delay until it succeeds, so handle must be safe to
// repeat.
func NewQueue[T any](name QueueName, handle func(ctx context.Context, inp T) error, opts QueueOptions) *Queue[T] {
	return &Queue[T]{
		name:   name,
		handle: handle,
		opts:   opts,
	}
}

// register creates the queue in the manager's runtime.
func (q *Queue[T]) register(m *Manager) error {
	if err := q.opts.validate(); err != nil {
		return err
	}

	if _, ok := m.names[q.name]; ok {
		return fmt.Errorf("queue %s is already registered", q.name)
	}

	if _, err := dbos.RegisterQueue(
		m.dbos,
		string(q.name),
		dbos.WithWorkerConcurrency(q.opts.Concurrency),
		dbos.WithQueueBasePollingInterval(q.opts.PollInterval),
		dbos.WithQueueOnConflict(dbos.QueueConflictAlwaysUpdate),
	); err != nil {
		return fmt.Errorf("registering queue %s: %w", q.name, err)
	}

	q.log = m.log.With("queue", q.name)
	q.dbos = m.dbos

	// DBOS keys a workflow by its function name, which is the same for
	// every queue's run method. Registering per instance keys it by the
	// queue name too, so the queues do not collide.
	dbos.RegisterWorkflow(
		m.dbos,
		q.run,
		dbos.WithWorkflowName(string(q.name)),
		dbos.WithInstance(instance(q.name)),
	)

	m.names[q.name] = struct{}{}

	return nil
}

// Enqueue queues a job on the transaction. The job runs only once the
// transaction commits. A failed enqueue aborts the transaction. The
// transaction must expose its underlying one through SQLTx, as the
// database package's does.
func (q *Queue[T]) Enqueue(ctx context.Context, tx sqlutil.Tx, inp T) error {
	stx, ok := tx.(sqlTx)
	if !ok {
		return fmt.Errorf("transaction %T has no underlying sql transaction", tx)
	}

	_, err := dbos.Enqueue[struct{}](
		dbos.From(q.dbos, ctx),
		string(q.name),
		string(q.name),
		envelope[T]{Input: inp},
		dbos.WithEnqueueConfigName(string(q.name)),
		dbos.WithEnqueueTransaction(stx.SQLTx()),
	)
	if err != nil {
		return fmt.Errorf("enqueuing job on %s: %w", q.name, err)
	}

	return nil
}

// run handles one job. A failure queues the job again with a delay
// instead of retrying in place, so a failing job never holds the queue.
func (q *Queue[T]) run(ctx dbos.Context, env envelope[T]) (struct{}, error) {
	_, err := dbos.RunAsStep(ctx, func(sctx context.Context) (_ struct{}, err error) {
		// a panic would end the job in DBOS's terminal error state, which
		// is never retried. As an error it is queued again like any other.
		defer func() {
			if rerr := recover(); rerr != nil {
				err = fmt.Errorf("handler panicked: %v", rerr)
			}
		}()

		return struct{}{}, q.handle(sctx, env.Input)
	}, dbos.WithStepName("handle"))
	if err == nil {
		return struct{}{}, nil
	}

	delay := retryDelay(env.Attempt)

	log := q.log.With("error", err)
	if env.Attempt >= _criticalAttempt {
		log = logutil.Critical(q.log, err)
	}

	log.With("attempt", env.Attempt).
		With("retry_in", delay.String()).
		Error("cannot handle job")

	_, err = dbos.Enqueue[struct{}](
		ctx,
		string(q.name),
		string(q.name),
		envelope[T]{
			Attempt: env.Attempt + 1,
			Input:   env.Input,
		},
		dbos.WithEnqueueConfigName(string(q.name)),
		dbos.WithEnqueueDelay(delay),
	)
	if err != nil {
		return struct{}{}, fmt.Errorf("requeuing job: %w", err)
	}

	return struct{}{}, nil
}

// instance identifies a queue's workflow to DBOS.
type instance QueueName

// ConfigName returns the queue name.
func (i instance) ConfigName() string {
	return string(i)
}

// envelope is the stored form of a job.
type envelope[T any] struct {
	// Attempt specifies how many times the job has failed before.
	Attempt int `json:"attempt"`

	// Input specifies the job's input.
	Input T `json:"input"`
}

// retryDelay returns the delay before the retry that follows the given
// failed attempt. It doubles per attempt up to _retryMaxDelay.
func retryDelay(attempt int) time.Duration {
	delay := _retryBaseDelay

	for range attempt {
		if delay >= _retryMaxDelay {
			break
		}

		delay *= 2
	}

	return min(delay, _retryMaxDelay)
}

// Registrable is a queue a Manager can register. Only this package's
// queues implement it.
type Registrable interface {
	// register should create the queue in the manager's runtime.
	register(m *Manager) error
}

// sqlTx is a transaction that exposes its underlying one.
type sqlTx interface {
	// SQLTx should return the underlying transaction.
	SQLTx() *sql.Tx
}
