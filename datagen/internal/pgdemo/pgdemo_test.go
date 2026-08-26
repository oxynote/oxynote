package pgdemo

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/orlangure/gnomock"
	pgDocker "github.com/orlangure/gnomock/preset/postgres"
	"github.com/oxynote/oxynote/datagen/internal/demodata"
	"github.com/oxynote/oxynote/datagen/internal/mockmetrics"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

const (
	// _testDB is the database name used in the throwaway container.
	_testDB = "testdb"

	// _pgUser is the privileged container user.
	_pgUser = "pgtest"

	// _pgPass is the privileged container password.
	_pgPass = "pgpass"
)

// _schema mirrors docker/demo/postgres/init-demo-db.sh, which is what the
// demo data source is really created with.
var _schema = []string{
	`CREATE TABLE deployments (
		id BIGSERIAL PRIMARY KEY,
		time TIMESTAMPTZ NOT NULL,
		service TEXT NOT NULL,
		environment TEXT NOT NULL,
		duration_seconds DOUBLE PRECISION NOT NULL,
		success BOOLEAN NOT NULL,
		rollback BOOLEAN NOT NULL DEFAULT FALSE
	)`,
	`CREATE TABLE incidents (
		id BIGSERIAL PRIMARY KEY,
		time TIMESTAMPTZ NOT NULL,
		severity TEXT NOT NULL,
		service TEXT NOT NULL,
		time_to_detect_minutes DOUBLE PRECISION NOT NULL,
		time_to_resolve_minutes DOUBLE PRECISION NOT NULL
	)`,
	`CREATE TABLE build_metrics (
		id BIGSERIAL PRIMARY KEY,
		time TIMESTAMPTZ NOT NULL,
		repository TEXT NOT NULL,
		branch TEXT NOT NULL,
		duration_seconds DOUBLE PRECISION NOT NULL,
		test_count INTEGER NOT NULL,
		tests_failed INTEGER NOT NULL,
		coverage_pct DOUBLE PRECISION NOT NULL
	)`,
}

// _pgAddr is the address of the throwaway PostgreSQL container.
var _pgAddr string

func TestMain(m *testing.M) {
	container, err := gnomock.Start(pgDocker.Preset(
		pgDocker.WithUser(_pgUser, _pgPass),
		pgDocker.WithDatabase(_testDB),
	))
	if err != nil {
		panic("cannot set up postgres: " + err.Error())
	}

	defer func() {
		if err = gnomock.Stop(container); err != nil {
			panic("cannot clean up postgres: " + err.Error())
		}
	}()

	_pgAddr = container.Host + ":" + strconv.Itoa(container.DefaultPort())

	goleak.VerifyTestMain(m, goleak.IgnoreCurrent())
}

func Test_NewGenerator(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.DiscardHandler)

	g := NewGenerator("postgres://test", log)
	require.NotNil(t, g)

	assert.Equal(t, log, g.log)
	assert.Equal(t, "postgres://test", g.dsn)
	assert.NotNil(t, g.r)
	assert.Equal(t, demodata.TickInterval, g.interval)
	assert.Equal(t, _backfillDays, g.backfillDays)
}

func Test_Generator_Run(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		// DSN overrides the throwaway database's DSN, for the cases that
		// must fail before any query runs.
		DSN string

		// DropTable names a table to remove before the run, so the backfill
		// check fails against a real connection.
		DropTable string

		// Err is the expected outcome of the run.
		Err error
	}{
		"Error returned by pgx.Connect": {
			DSN: "postgres://nobody@127.0.0.1:1/nothing",
			Err: assert.AnError,
		},
		"Error returned by needsBackfill": {
			DropTable: "deployments",
			Err:       assert.AnError,
		},
		"Error returned by backfill": {
			DropTable: "incidents",
			Err:       assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			g := prepGenerator(t)
			if c.DSN != "" {
				g.dsn = c.DSN
			}

			if c.DropTable != "" {
				exec(t, g.dsn, "DROP TABLE "+c.DropTable)
			}

			ctx := t.Context()

			testutil.AssertEqualError(t, c.Err, g.Run(ctx))
		})
	}

	// an empty database is backfilled before the tick loop starts.
	t.Run("Successful run backfills an empty database", func(t *testing.T) {
		t.Parallel()

		log, wr, content := bufferLog()

		g := prepGenerator(t)
		g.log = log

		// a wide interval keeps the backfill to a handful of ticks; the
		// periodic loop then never fires before the context is cancelled.
		g.interval = 6 * time.Hour

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		stopCh := make(chan struct{})

		go func() {
			defer close(stopCh)

			assert.NoError(t, g.Run(ctx))
		}()

		// cancelling mid-backfill would abort it, so wait for the run to
		// announce that it finished.
		require.Eventually(t, func() bool {
			require.NoError(t, wr.Flush())

			return strings.Contains(content(), "backfill complete")
		}, 60*time.Second, 50*time.Millisecond)

		assert.Positive(t, count(t, g.dsn, "deployments"))

		cancel()

		<-stopCh
	})

	// a populated database skips the backfill and goes straight to
	// appending a tick per interval.
	t.Run("Successful run appends to a populated database", func(t *testing.T) {
		t.Parallel()

		log, wr, content := bufferLog()

		g := prepGenerator(t)
		g.log = log

		require.NoError(t, g.insertTick(context.Background(), connect(t, g.dsn), _tick))

		seeded := count(t, g.dsn, "deployments")
		require.Positive(t, seeded)

		g.interval = 10 * time.Millisecond

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		stopCh := make(chan struct{})

		go func() {
			defer close(stopCh)

			assert.NoError(t, g.Run(ctx))
		}()

		require.Eventually(t, func() bool {
			return count(t, g.dsn, "deployments") > seeded
		}, 60*time.Second, 50*time.Millisecond)

		cancel()

		<-stopCh

		require.NoError(t, wr.Flush())
		assert.NotContains(t, content(), "backfilling historical data")
	})

	// a failure inside the tick loop is logged and the loop carries on.
	t.Run("Tick insert failure is logged", func(t *testing.T) {
		t.Parallel()

		lg, wr, content := bufferLog()

		g := prepGenerator(t)
		g.log = lg

		require.NoError(t, g.insertTick(context.Background(), connect(t, g.dsn), _tick))

		seeded := count(t, g.dsn, "deployments")
		require.Positive(t, seeded)

		g.interval = 10 * time.Millisecond

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		stopCh := make(chan struct{})

		go func() {
			defer close(stopCh)

			assert.NoError(t, g.Run(ctx))
		}()

		// the table can only be pulled out from under the loop once the
		// loop is running, which its first successful insert proves.
		require.Eventually(t, func() bool {
			return count(t, g.dsn, "deployments") > seeded
		}, 60*time.Second, 50*time.Millisecond)

		exec(t, g.dsn, "DROP TABLE deployments")

		require.Eventually(t, func() bool {
			require.NoError(t, wr.Flush())

			return strings.Contains(content(), "tick insert failed")
		}, 60*time.Second, 50*time.Millisecond)

		cancel()

		<-stopCh
	})
}

func Test_Generator_needsBackfill(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		// Seed indicates whether a row is inserted before the check.
		Seed bool

		// DropTable removes the table so the query itself fails.
		DropTable bool

		Result bool
		Err    error
	}{
		"Empty database still needs a backfill": {},
		"Populated database does not": {
			Seed:   true,
			Result: true,
		},
		"Error returned by the query": {
			DropTable: true,
			Err:       assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			g := prepGenerator(t)

			if c.Seed {
				require.NoError(t, g.insertTick(context.Background(), connect(t, g.dsn), _tick))
			}

			if c.DropTable {
				exec(t, g.dsn, "DROP TABLE deployments")
			}

			res, err := g.needsBackfill(context.Background(), connect(t, g.dsn))
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_Generator_backfill(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		// DropTable removes a table so an insert inside the loop fails.
		DropTable string

		Err error
	}{
		"Successful backfill": {},
		"Error returned by insertTick": {
			DropTable: "deployments",
			Err:       assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			g := prepGenerator(t)

			if c.DropTable != "" {
				exec(t, g.dsn, "DROP TABLE "+c.DropTable)
			}

			err := g.backfill(context.Background(), connect(t, g.dsn))
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			// one day of ticks covers a stretch of demo history, so every
			// table has to have been written to.
			assert.Positive(t, count(t, g.dsn, "deployments"))
			assert.Positive(t, count(t, g.dsn, "incidents"))
			assert.Positive(t, count(t, g.dsn, "build_metrics"))
		})
	}
}

func Test_Generator_insertTick(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		// DropTable removes a table so the batch fails on exec.
		DropTable string

		// EmptyTick indicates the generator should be steered at an instant
		// that produces nothing at all.
		EmptyTick bool

		Err error
	}{
		"Successful insert": {},
		"A tick with no rows sends no batch": {
			EmptyTick: true,
		},
		"Error returned by the batch": {
			DropTable: "deployments",
			Err:       assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			g := prepGenerator(t)

			if c.DropTable != "" {
				exec(t, g.dsn, "DROP TABLE "+c.DropTable)
			}

			at := _tick

			// the early return needs a tick that generates nothing, which
			// only a specific seed at a quiet instant produces.
			if c.EmptyTick {
				seed, quiet := emptyDraw()
				g.r = mockmetrics.NewRand(seed)
				at = quiet
			}

			err := g.insertTick(context.Background(), connect(t, g.dsn), at)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			total := count(t, g.dsn, "deployments") +
				count(t, g.dsn, "incidents") +
				count(t, g.dsn, "build_metrics")

			if c.EmptyTick {
				assert.Zero(t, total)
				return
			}

			assert.Positive(t, total)
		})
	}
}

// _tick is a weekday office-hours instant, busy enough that every table gets
// rows.
var _tick = time.Date(2024, time.January, 9, 11, 0, 0, 0, time.UTC)

// prepGenerator gives a test its own randomly named database in the shared
// container, already carrying the demo schema.
func prepGenerator(t *testing.T) *Generator {
	t.Helper()

	name := "demo_" + strconv.FormatInt(time.Now().UnixNano(), 36) + strconv.Itoa(len(t.Name()))

	admin := connect(t, dsn(_testDB))
	_, err := admin.Exec(context.Background(), "CREATE DATABASE "+name)
	require.NoError(t, err)

	conn := connect(t, dsn(name))
	for _, q := range _schema {
		_, err = conn.Exec(context.Background(), q)
		require.NoError(t, err)
	}

	g := NewGenerator(dsn(name), slog.New(slog.DiscardHandler))

	// a single day keeps the backfill loop to a few hundred ticks instead
	// of the month the generator runs with in production.
	g.backfillDays = 1

	// a pinned seed makes the first tick at _tick fill every table, so no
	// test depends on a sparse table happening to draw a row.
	g.r = mockmetrics.NewRand(_fullDrawSeed)

	return g
}

// dsn builds a connection string for one database in the container.
func dsn(db string) string {
	return fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable", _pgUser, _pgPass, _pgAddr, db)
}

// connect opens a connection that is closed when the test ends.
func connect(t *testing.T, dsn string) *pgx.Conn {
	t.Helper()

	conn, err := pgx.Connect(context.Background(), dsn)
	require.NoError(t, err)

	t.Cleanup(func() {
		assert.NoError(t, conn.Close(context.Background()))
	})

	return conn
}

// exec runs a single statement against the given database.
func exec(t *testing.T, dsn, query string) {
	t.Helper()

	_, err := connect(t, dsn).Exec(context.Background(), query)
	require.NoError(t, err)
}

// count returns the number of rows in a table over a connection of its own,
// so the polling callers cannot pile up connections.
func count(t *testing.T, dsn, table string) int {
	t.Helper()

	conn, err := pgx.Connect(context.Background(), dsn)
	require.NoError(t, err)

	defer func() {
		assert.NoError(t, conn.Close(context.Background()))
	}()

	var n int

	require.NoError(t, conn.QueryRow(context.Background(), "SELECT COUNT(*) FROM "+table).Scan(&n))

	return n
}

// emptyDraw finds a seed and a quiet instant that together generate nothing
// at all, so the empty-tick early return can be reached deterministically.
func emptyDraw() (int64, time.Time) {
	at := time.Date(2024, time.January, 7, 3, 0, 0, 0, time.UTC)

	for seed := int64(1); ; seed++ {
		tick := demodata.GenerateTick(mockmetrics.NewRand(seed), at)
		if len(tick.Deployments)+len(tick.Incidents)+len(tick.BuildMetrics) == 0 {
			return seed, at
		}
	}
}

// bufferLog creates a logger writing into a buffer whose contents the caller
// can read back once flushed.
func bufferLog() (*slog.Logger, *testutil.Writer, func() string) {
	wr, buf := testutil.NewBuffer()

	return slog.New(slog.NewJSONHandler(wr, nil)), wr, buf.String
}

// _fullDrawSeed is a seed whose first draw at _tick fills every table, so a
// dropped table reliably breaks the batch.
var _fullDrawSeed = fullDraw()

// fullDraw searches for that seed.
func fullDraw() int64 {
	for seed := int64(1); ; seed++ {
		tick := demodata.GenerateTick(mockmetrics.NewRand(seed), _tick)
		if len(tick.Deployments) > 0 && len(tick.Incidents) > 0 && len(tick.BuildMetrics) > 0 {
			return seed
		}
	}
}
