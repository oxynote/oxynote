package job

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dbos-inc/dbos-transact-golang/dbos"
	"github.com/dchest/uniuri"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/orlangure/gnomock"
	pgDocker "github.com/orlangure/gnomock/preset/postgres"
	"github.com/oxynote/oxynote/server/core/pkg/sqlutil"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

// _pgAddr is the host and port of the test Postgres container.
var _pgAddr string

// _testQueueOpts configures the queues of tests that need no particular
// options. The short interval keeps the tests fast.
var _testQueueOpts = QueueOptions{
	Concurrency:  1,
	PollInterval: 50 * time.Millisecond,
}

const (
	_pgUser string = "pgtest"
	_pgPass string = "pgpass"
)

func TestMain(m *testing.M) {
	container, err := gnomock.Start(pgDocker.Preset(
		pgDocker.WithUser(
			_pgUser,
			_pgPass,
		),
		pgDocker.WithVersion("18.6-alpine"),
	))
	if err != nil {
		panic("cannot set up: " + err.Error())
	}

	defer func() {
		err = gnomock.Stop(container)
		if err != nil {
			panic("cannot clean up: " + err.Error())
		}
	}()

	_pgAddr = container.Host + ":" + strconv.Itoa(container.DefaultPort())

	_retryBaseDelay = time.Millisecond
	_retryMaxDelay = 4 * time.Millisecond
	_cleanupBatch = 2

	goleak.VerifyTestMain(m, goleak.IgnoreCurrent())
}

func Test_NewManager(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		DSN string
		Err error
	}{
		"Empty DSN": {
			DSN: "",
			Err: errors.New("dsn is required"),
		},
		"Error returned by pgxpool.New": {
			DSN: "postgres://%zz",
			Err: assert.AnError,
		},
		"Error returned by dbos.NewContext": {
			DSN: fmt.Sprintf("postgres://%s:%s@%s/missing?sslmode=disable", _pgUser, _pgPass, _pgAddr),
			Err: assert.AnError,
		},
		"Successful creation": {
			DSN: prepDSN(t),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			m, err := NewManager(context.Background(), slog.New(slog.DiscardHandler), c.DSN)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				assert.Nil(t, m)

				return
			}

			require.NotNil(t, m)
			assert.NotNil(t, m.log)
			assert.NotNil(t, m.dbos)
			assert.NotNil(t, m.pe)
			assert.NoError(t, m.Close())
		})
	}
}

func Test_Manager_Launch(t *testing.T) {
	t.Parallel()

	m := prepManager(t, slog.New(slog.DiscardHandler))

	// success
	require.NoError(t, m.Launch())

	// error
	assert.Error(t, m.Launch())
}

func Test_Manager_Start(t *testing.T) {
	t.Parallel()

	m := prepManager(t, slog.New(slog.DiscardHandler))
	require.NoError(t, m.Launch())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	m.Start(ctx)
}

func Test_Manager_Close(t *testing.T) {
	t.Parallel()

	m, err := NewManager(context.Background(), slog.New(slog.DiscardHandler), prepDSN(t))
	require.NoError(t, err)
	require.NoError(t, m.Launch())

	assert.NoError(t, m.Close())
	assert.NoError(t, m.Close())
}

func Test_Manager_cleanup(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Closed bool
		Logged bool
	}{
		"Error returned by Manager.deleteFinished": {
			Closed: true,
			Logged: true,
		},
		"Successful cleanup": {},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			w, buf := testutil.NewBuffer()
			m := prepManager(t, slog.New(slog.NewTextHandler(w, nil)))

			// failing on a closed runtime is the only way to reach the log.
			if c.Closed {
				require.NoError(t, m.Close())
			}

			m.cleanup(context.Background())

			require.NoError(t, w.Flush())

			if !c.Logged {
				assert.NotContains(t, buf.String(), `msg="cannot delete finished jobs"`)

				return
			}

			assert.Contains(t, buf.String(), `msg="cannot delete finished jobs"`)
		})
	}
}

func Test_Manager_deleteFinished(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Jobs      int
		Retention time.Duration
		Closed    bool
		Remaining int
		Err       error
	}{
		"Error returned by dbos.ListWorkflows": {
			Closed: true,
			Err:    assert.AnError,
		},
		"Nothing to delete": {
			Remaining: 0,
		},
		"Jobs within retention are kept": {
			Jobs:      2,
			Retention: time.Hour,
			Remaining: 2,
		},
		"Successful deletion over several batches": {
			Jobs:      3,
			Remaining: 0,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			dsn := prepDSN(t)
			m := prepManager(t, slog.New(slog.DiscardHandler), dsn)

			q := prepQueue(t, m, "test", func(context.Context, int) error {
				return nil
			}, _testQueueOpts)
			require.NoError(t, m.Launch())

			for i := range c.Jobs {
				enqueueCommitted(t, dsn, q, i)
			}

			awaitQueue(t, m, "test", c.Jobs)

			if c.Closed {
				require.NoError(t, m.Close())
			}

			err := m.deleteFinished(context.Background(), time.Now().Add(-c.Retention))
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			wfs, err := dbos.ListWorkflows(m.dbos, dbos.WithFilterQueueName("test"))
			require.NoError(t, err)
			assert.Len(t, wfs, c.Remaining)
		})
	}
}

func Test_QueueOptions_validate(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Opts QueueOptions
		Err  error
	}{
		"Zero concurrency": {
			Opts: QueueOptions{PollInterval: time.Second},
			Err:  errors.New("concurrency must be positive"),
		},
		"Zero poll interval": {
			Opts: QueueOptions{Concurrency: 1},
			Err:  errors.New("poll interval must be positive"),
		},
		"Valid options": {
			Opts: QueueOptions{
				Concurrency:  1,
				PollInterval: time.Second,
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.Opts.validate())
		})
	}
}

func Test_Manager_Register(t *testing.T) {
	t.Parallel()

	dsn := prepDSN(t)
	m := prepManager(t, slog.New(slog.DiscardHandler), dsn)

	// error
	assert.Error(t, m.Register(NewQueue("test", func(context.Context, int) error { return nil }, QueueOptions{})))

	// success: queues of the same input type coexist
	handled := make(chan QueueName, 2)

	first := prepQueue(t, m, "first", func(context.Context, int) error {
		handled <- "first"

		return nil
	}, _testQueueOpts)

	second := prepQueue(t, m, "second", func(context.Context, int) error {
		handled <- "second"

		return nil
	}, _testQueueOpts)

	assert.Equal(t, m.dbos, first.dbos)
	assert.Contains(t, m.names, QueueName("first"))
	assert.Contains(t, m.names, QueueName("second"))

	require.NoError(t, m.Launch())

	enqueueCommitted(t, dsn, first, 1)
	enqueueCommitted(t, dsn, second, 2)

	assert.ElementsMatch(t, []QueueName{"first", "second"}, []QueueName{<-handled, <-handled})
}

func Test_NewQueue(t *testing.T) {
	t.Parallel()

	q := NewQueue("test", func(context.Context, int) error { return nil }, _testQueueOpts)

	require.NotNil(t, q)
	assert.Nil(t, q.log)
	assert.Nil(t, q.dbos)
	assert.Equal(t, QueueName("test"), q.name)
	assert.NotNil(t, q.handle)
	assert.Equal(t, _testQueueOpts, q.opts)
}

func Test_Queue_register(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Name      QueueName
		Opts      QueueOptions
		Duplicate bool
		Err       error
	}{
		"Invalid options": {
			Name: "test",
			Err:  errors.New("concurrency must be positive"),
		},
		"Duplicate name": {
			Name:      "test",
			Opts:      _testQueueOpts,
			Duplicate: true,
			Err:       errors.New("queue test is already registered"),
		},
		"Error returned by dbos.RegisterQueue": {
			Name: "_dbos_internal_queue",
			Opts: _testQueueOpts,
			Err:  assert.AnError,
		},
		"Successful registration": {
			Name: "test",
			Opts: QueueOptions{
				Concurrency:  2,
				PollInterval: 30 * time.Millisecond,
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			m := prepManager(t, slog.New(slog.DiscardHandler))

			if c.Duplicate {
				prepQueue(t, m, c.Name, func(context.Context, string) error {
					return nil
				}, _testQueueOpts)
			}

			q := NewQueue(c.Name, func(context.Context, int) error {
				return nil
			}, c.Opts)

			err := q.register(m)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				assert.Nil(t, q.dbos)

				return
			}

			assert.NotNil(t, q.log)
			assert.Equal(t, m.dbos, q.dbos)

			assert.Contains(t, m.names, c.Name)

			queue, err := dbos.RetrieveQueue(m.dbos, string(c.Name))
			require.NoError(t, err)
			require.NotNil(t, queue.GetWorkerConcurrency())
			assert.Equal(t, c.Opts.Concurrency, *queue.GetWorkerConcurrency())
			assert.Equal(t, c.Opts.PollInterval, queue.GetPollingInterval())
		})
	}
}

func Test_Queue_Enqueue(t *testing.T) {
	t.Parallel()

	begin := func(t *testing.T, sdb *sql.DB) sqlutil.Tx {
		t.Helper()

		tx, err := sdb.Begin()
		require.NoError(t, err)

		return stubTx{tx: tx}
	}

	cc := map[string]struct {
		Tx        func(t *testing.T, sdb *sql.DB) sqlutil.Tx
		Inp       []int
		Rollback  bool
		Handled   []int
		MaxActive int32
		Err       error
	}{
		"Transaction without an underlying one": {
			Tx: func(*testing.T, *sql.DB) sqlutil.Tx {
				return bareTx{}
			},
			Inp: []int{0},
			Err: errors.New("transaction job.bareTx has no underlying sql transaction"),
		},
		"Error returned by dbos.Enqueue": {
			Tx: func(t *testing.T, sdb *sql.DB) sqlutil.Tx {
				tx := begin(t, sdb)
				require.NoError(t, tx.Rollback())

				return tx
			},
			Inp: []int{1},
			Err: assert.AnError,
		},
		"Rolled back job never runs": {
			Tx:       begin,
			Inp:      []int{2},
			Rollback: true,
		},
		"Successful enqueue": {
			Tx:        begin,
			Inp:       []int{3, 4, 5},
			Handled:   []int{3, 4, 5},
			MaxActive: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			dsn := prepDSN(t)
			m := prepManager(t, slog.New(slog.DiscardHandler), dsn)

			var (
				active    atomic.Int32
				maxActive atomic.Int32
			)

			handled := make(chan int, 10)

			q := prepQueue(t, m, "test", func(_ context.Context, inp int) error {
				n := active.Add(1)
				defer active.Add(-1)

				if n > maxActive.Load() {
					maxActive.Store(n)
				}

				handled <- inp

				return nil
			}, _testQueueOpts)
			require.NoError(t, m.Launch())

			sdb := openDB(t, dsn)

			// a failed enqueue still reaches the queue checks below, which
			// prove it queued nothing.
			for _, inp := range c.Inp {
				tx := c.Tx(t, sdb)

				err := q.Enqueue(context.Background(), tx, inp)
				testutil.AssertEqualError(t, c.Err, err)

				if err != nil {
					continue
				}

				if c.Rollback {
					require.NoError(t, tx.Rollback())

					continue
				}

				require.NoError(t, tx.Commit())
			}

			got := make([]int, len(c.Handled))
			for i := range got {
				got[i] = <-handled
			}

			assert.ElementsMatch(t, c.Handled, got)

			awaitQueue(t, m, "test", len(c.Handled))

			assert.Empty(t, handled)
			assert.Equal(t, c.MaxActive, maxActive.Load())
		})
	}
}

func Test_Queue_run(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Panics  bool
		Fails   int32
		Restart bool
		Calls   int32
		Logs    map[int][]string
	}{
		// the first attempt panics, the next ten fail and the twelfth
		// succeeds, so the last failure is past the critical threshold.
		"Failed attempts are retried until one succeeds": {
			Panics: true,
			Fails:  11,
			Calls:  12,
			Logs: map[int][]string{
				0:  {`error="handler panicked: boom"`, "retry_in=1ms"},
				1:  {"retry_in=2ms"},
				9:  {`error="` + assert.AnError.Error() + `"`},
				10: {`error="internal error"`},
			},
		},
		// every attempt fails until the process stops, and a succeeding
		// handler takes the job over after the restart.
		"Retry pending at shutdown runs in the next process": {
			Fails:   math.MaxInt32,
			Restart: true,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			dsn := prepDSN(t)
			w, buf := testutil.NewBuffer()
			m := prepManager(t, slog.New(slog.NewTextHandler(w, nil)), dsn)

			var (
				calls      atomic.Int32
				failOnce   sync.Once
				handleOnce sync.Once
			)

			failed := make(chan struct{})
			handled := make(chan string, 1)

			q := prepQueue(t, m, "test", func(_ context.Context, inp string) error {
				n := calls.Add(1)

				switch {
				case c.Panics && n == 1:
					panic("boom")
				case n <= c.Fails:
					failOnce.Do(func() { close(failed) })

					return assert.AnError
				}

				handleOnce.Do(func() { handled <- inp })

				return nil
			}, _testQueueOpts)
			require.NoError(t, m.Launch())

			enqueueCommitted(t, dsn, q, "scope")

			if c.Restart {
				<-failed
				require.NoError(t, m.Close())

				m = prepManager(t, slog.New(slog.DiscardHandler), dsn)

				prepQueue(t, m, "test", func(_ context.Context, inp string) error {
					handleOnce.Do(func() { handled <- inp })

					return nil
				}, _testQueueOpts)
				require.NoError(t, m.Launch())

				assert.Equal(t, "scope", <-handled)

				return
			}

			assert.Equal(t, "scope", <-handled)

			awaitQueue(t, m, "test", int(c.Calls))
			assert.Equal(t, c.Calls, calls.Load())

			require.NoError(t, w.Flush())

			failures := map[int]string{}

			for line := range strings.Lines(buf.String()) {
				if !strings.Contains(line, `msg="cannot handle job"`) {
					continue
				}

				_, rest, ok := strings.Cut(line, "attempt=")
				require.True(t, ok)

				var attempt int

				_, err := fmt.Sscanf(rest, "%d", &attempt)
				require.NoError(t, err)

				failures[attempt] = line
			}

			require.Len(t, failures, int(c.Fails))

			for attempt, ss := range c.Logs {
				for _, s := range ss {
					assert.Contains(t, failures[attempt], s)
				}
			}
		})
	}
}

func Test_instance_ConfigName(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "test", instance("test").ConfigName())
}

func Test_retryDelay(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Attempt int
		Result  time.Duration
	}{
		"First attempt": {
			Attempt: 0,
			Result:  time.Millisecond,
		},
		"Second attempt": {
			Attempt: 1,
			Result:  2 * time.Millisecond,
		},
		"Capped attempt": {
			Attempt: 3,
			Result:  4 * time.Millisecond,
		},
		"Attempt beyond the shift limit": {
			Attempt: 1000,
			Result:  4 * time.Millisecond,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, retryDelay(c.Attempt))
		})
	}
}

// stubTx exposes a plain transaction as a Tx.
type stubTx struct {
	tx *sql.Tx
}

// SQLTx returns the wrapped transaction.
func (st stubTx) SQLTx() *sql.Tx {
	return st.tx
}

// Commit commits the wrapped transaction.
func (st stubTx) Commit() error {
	return st.tx.Commit()
}

// Rollback rolls the wrapped transaction back.
func (st stubTx) Rollback() error {
	return st.tx.Rollback()
}

// bareTx is a transaction with no underlying one.
type bareTx struct{}

// Commit does nothing.
func (bareTx) Commit() error {
	return nil
}

// Rollback does nothing.
func (bareTx) Rollback() error {
	return nil
}

// prepQueue creates a queue and registers it with the manager.
func prepQueue[T any](
	t *testing.T,
	m *Manager,
	name QueueName,
	handle func(ctx context.Context, inp T) error,
	opts QueueOptions,
) *Queue[T] {
	t.Helper()

	q := NewQueue(name, handle, opts)
	require.NoError(t, m.Register(q))

	return q
}

// prepDSN creates an empty database and returns its DSN.
func prepDSN(t *testing.T) string {
	t.Helper()

	admin := openDB(t, fmt.Sprintf("postgres://%s:%s@%s/postgres?sslmode=disable", _pgUser, _pgPass, _pgAddr))
	name := "job_" + strings.ToLower(uniuri.NewLen(12))

	_, err := admin.Exec("CREATE DATABASE " + name)
	require.NoError(t, err)

	return fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable", _pgUser, _pgPass, _pgAddr, name)
}

// prepManager creates a Manager on the given database, or on a new one,
// and closes it when the test ends.
func prepManager(t *testing.T, log *slog.Logger, dsn ...string) *Manager {
	t.Helper()

	if len(dsn) == 0 {
		dsn = append(dsn, prepDSN(t))
	}

	m, err := NewManager(context.Background(), log, dsn[0])
	require.NoError(t, err)

	t.Cleanup(func() {
		assert.NoError(t, m.Close())
	})

	return m
}

// openDB opens a connection pool closed when the test ends.
func openDB(t *testing.T, dsn string) *sql.DB {
	t.Helper()

	sdb, err := sql.Open("pgx", dsn)
	require.NoError(t, err)

	t.Cleanup(func() {
		assert.NoError(t, sdb.Close())
	})

	return sdb
}

// enqueueCommitted queues a job on a transaction and commits it.
func enqueueCommitted[T any](t *testing.T, dsn string, q *Queue[T], inp T) {
	t.Helper()

	tx, err := openDB(t, dsn).Begin()
	require.NoError(t, err)
	require.NoError(t, q.Enqueue(context.Background(), stubTx{tx: tx}, inp))
	require.NoError(t, tx.Commit())
}

// awaitQueue waits for every job of the queue to finish and checks that
// the queue holds the given number of jobs.
func awaitQueue(t *testing.T, m *Manager, name string, count int) {
	t.Helper()

	wfs, err := dbos.ListWorkflows(m.dbos, dbos.WithFilterQueueName(name))
	require.NoError(t, err)
	require.Len(t, wfs, count)

	for _, wf := range wfs {
		h, err := dbos.RetrieveWorkflow[struct{}](m.dbos, wf.ID)
		require.NoError(t, err)

		_, err = h.GetResult()
		require.NoError(t, err)
	}
}
