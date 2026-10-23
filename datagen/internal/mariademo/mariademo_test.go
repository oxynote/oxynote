package mariademo

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log"
	"log/slog"
	"strconv"
	"strings"
	"testing"
	"time"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"github.com/orlangure/gnomock"
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

	// _rootUser is the privileged container user.
	_rootUser = "root"

	// _rootPass is the privileged container password.
	_rootPass = "rootpass"
)

// _schema mirrors docker/demo/mariadb/init-demo-db.sql, which is what the
// demo data source is really created with.
var _schema = []string{
	`CREATE TABLE deployments (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		time DATETIME NOT NULL,
		service VARCHAR(255) NOT NULL,
		environment VARCHAR(255) NOT NULL,
		duration_seconds DOUBLE NOT NULL,
		success BOOLEAN NOT NULL,
		rollback BOOLEAN NOT NULL DEFAULT FALSE
	)`,
	`CREATE TABLE incidents (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		time DATETIME NOT NULL,
		severity VARCHAR(255) NOT NULL,
		service VARCHAR(255) NOT NULL,
		time_to_detect_minutes DOUBLE NOT NULL,
		time_to_resolve_minutes DOUBLE NOT NULL
	)`,
	`CREATE TABLE build_metrics (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		time DATETIME NOT NULL,
		repository VARCHAR(255) NOT NULL,
		branch VARCHAR(255) NOT NULL,
		duration_seconds DOUBLE NOT NULL,
		test_count INT NOT NULL,
		tests_failed INT NOT NULL,
		coverage_pct DOUBLE NOT NULL
	)`,
}

// _addr is the address of the throwaway MariaDB container.
var _addr string

// _tick is a weekday office-hours instant, busy enough that every table gets
// rows.
var _tick = time.Date(2024, time.January, 9, 11, 0, 0, 0, time.UTC)

// _fullDrawSeed is a seed whose first draw at _tick fills every table, so a
// test can make any one of the three inserts fail.
var _fullDrawSeed = func() int64 {
	for seed := int64(1); ; seed++ {
		tick := demodata.GenerateTick(mockmetrics.NewRand(seed), _tick)
		if len(tick.Deployments) > 0 && len(tick.Incidents) > 0 && len(tick.BuildMetrics) > 0 {
			return seed
		}
	}
}()

func TestMain(m *testing.M) {
	// silence the driver's connection noise while the container boots.
	if err := mysqlDriver.SetLogger(log.New(io.Discard, "", 0)); err != nil {
		panic("cannot set mysql logger: " + err.Error())
	}

	container, err := gnomock.StartCustom(
		"docker.io/library/mariadb:11-noble",
		gnomock.DefaultTCP(3306),
		gnomock.WithEnv("MARIADB_ROOT_PASSWORD="+_rootPass),
		gnomock.WithEnv("MARIADB_DATABASE="+_testDB),
		gnomock.WithHealthCheck(func(ctx context.Context, c *gnomock.Container) error {
			db, err := sql.Open("mysql", fmt.Sprintf(
				"%s:%s@tcp(%s)/%s",
				_rootUser,
				_rootPass,
				c.Address(gnomock.DefaultPort),
				_testDB,
			))
			if err != nil {
				return err
			}

			defer db.Close() //nolint:errcheck // error provides no meaningful info

			return db.PingContext(ctx)
		}),
	)
	if err != nil {
		panic("cannot set up mariadb: " + err.Error())
	}

	defer func() {
		if err = gnomock.Stop(container); err != nil {
			panic("cannot clean up mariadb: " + err.Error())
		}
	}()

	_addr = container.Host + ":" + strconv.Itoa(container.DefaultPort())

	goleak.VerifyTestMain(m, goleak.IgnoreCurrent())
}

func Test_NewGenerator(t *testing.T) {
	t.Parallel()

	lg := slog.New(slog.DiscardHandler)

	g := NewGenerator("user:pass@tcp(host)/db", lg)
	require.NotNil(t, g)

	assert.Equal(t, lg, g.log)
	assert.Equal(t, "user:pass@tcp(host)/db", g.dsn)
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

		// DropTable names a table to remove before the run, so a query
		// fails against a real connection.
		DropTable string

		// Seed indicates whether one tick is inserted before the run, so
		// the database counts as populated.
		Seed bool

		// Interval overrides the tick interval when set.
		Interval time.Duration

		// Await blocks while the run is in progress until the case has
		// seen what it checks. The run is then cancelled. Without it the
		// run has to return by itself.
		Await func(t *testing.T, g *Generator, seeded int, wr *testutil.Writer, content func() string)

		Err error

		// Check is called once the run has returned.
		Check func(t *testing.T, wr *testutil.Writer, content func() string)
	}{
		"Error returned by sql.Open": {
			DSN: "not a dsn at all",
			Err: assert.AnError,
		},
		"Error returned by DB.PingContext": {
			DSN: fmt.Sprintf("%s:%s@tcp(127.0.0.1:1)/%s", _rootUser, _rootPass, _testDB),
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

		// an empty database is backfilled before the tick loop starts.
		"Successful run backfills an empty database": {
			// a wide interval keeps the backfill to a handful of ticks; the
			// periodic loop then never fires before the context is
			// cancelled.
			Interval: 6 * time.Hour,
			Await: func(t *testing.T, g *Generator, _ int, wr *testutil.Writer, content func() string) {
				// cancelling mid-backfill would abort it, so wait for the run
				// to announce that it finished.
				require.Eventually(t, func() bool {
					require.NoError(t, wr.Flush())

					return strings.Contains(content(), "backfill complete")
				}, 60*time.Second, 50*time.Millisecond)

				assert.Positive(t, count(t, g.dsn, "deployments"))
			},
		},

		// a populated database skips the backfill and goes straight to
		// appending a tick per interval.
		"Successful run appends to a populated database": {
			Seed:     true,
			Interval: 10 * time.Millisecond,
			Await: func(t *testing.T, g *Generator, seeded int, _ *testutil.Writer, _ func() string) {
				require.Eventually(t, func() bool {
					return count(t, g.dsn, "deployments") > seeded
				}, 60*time.Second, 50*time.Millisecond)
			},
			Check: func(t *testing.T, wr *testutil.Writer, content func() string) {
				require.NoError(t, wr.Flush())
				assert.NotContains(t, content(), "backfilling historical data")
			},
		},

		// a failure inside the tick loop is logged and the loop carries on.
		"Tick insert failure is logged": {
			Seed:     true,
			Interval: 10 * time.Millisecond,
			Await: func(t *testing.T, g *Generator, seeded int, wr *testutil.Writer, content func() string) {
				// the table can only be pulled out from under the loop once
				// the loop is running, which its first successful insert
				// proves.
				require.Eventually(t, func() bool {
					return count(t, g.dsn, "deployments") > seeded
				}, 60*time.Second, 50*time.Millisecond)

				exec(t, g.dsn, "DROP TABLE deployments")

				require.Eventually(t, func() bool {
					require.NoError(t, wr.Flush())

					return strings.Contains(content(), "tick insert failed")
				}, 60*time.Second, 50*time.Millisecond)
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			wr, buf := testutil.NewBuffer()

			g := prepGenerator(t)
			g.log = slog.New(slog.NewJSONHandler(wr, nil))

			if c.DSN != "" {
				g.dsn = c.DSN
			}

			if c.DropTable != "" {
				exec(t, g.dsn, "DROP TABLE "+c.DropTable)
			}

			var seeded int

			if c.Seed {
				require.NoError(t, g.insertTick(context.Background(), open(t, g.dsn), _tick))

				seeded = count(t, g.dsn, "deployments")
				require.Positive(t, seeded)
			}

			if c.Interval != 0 {
				g.interval = c.Interval
			}

			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()

			errCh := make(chan error, 1)

			go func() {
				errCh <- g.Run(ctx)
			}()

			// a successful run only returns once cancelled.
			if c.Await != nil {
				c.Await(t, g, seeded, wr, buf.String)
				cancel()
			}

			err := <-errCh
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			if c.Check != nil {
				c.Check(t, wr, buf.String)
			}
		})
	}
}

func Test_Generator_needsBackfill(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		// Seed indicates whether rows are inserted before the check.
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
		"Error returned by Row.Scan": {
			DropTable: true,
			Err:       assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			g := prepGenerator(t)
			db := open(t, g.dsn)

			if c.Seed {
				require.NoError(t, g.insertTick(context.Background(), db, _tick))
			}

			if c.DropTable {
				exec(t, g.dsn, "DROP TABLE deployments")
			}

			res, err := g.needsBackfill(context.Background(), db)
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

			err := g.backfill(context.Background(), open(t, g.dsn))
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
		// DropTable removes a table so one of the inserts fails.
		DropTable string

		// CloseDB indicates the pool should be closed first, so the
		// transaction cannot even begin.
		CloseDB bool

		Err error
	}{
		"Successful insert": {},
		"Error returned by Tx.ExecContext for deployments": {
			DropTable: "deployments",
			Err:       assert.AnError,
		},
		"Error returned by Tx.ExecContext for incidents": {
			DropTable: "incidents",
			Err:       assert.AnError,
		},
		"Error returned by Tx.ExecContext for build metrics": {
			DropTable: "build_metrics",
			Err:       assert.AnError,
		},
		"Error returned by DB.BeginTx": {
			CloseDB: true,
			Err:     assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			g := prepGenerator(t)
			db := open(t, g.dsn)

			if c.DropTable != "" {
				exec(t, g.dsn, "DROP TABLE "+c.DropTable)
			}

			if c.CloseDB {
				require.NoError(t, db.Close())
			}

			err := g.insertTick(context.Background(), db, _tick)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Positive(t, count(t, g.dsn, "deployments"))
			assert.Positive(t, count(t, g.dsn, "incidents"))
			assert.Positive(t, count(t, g.dsn, "build_metrics"))
		})
	}
}

// prepGenerator gives a test its own randomly named database in the shared
// container, already carrying the demo schema.
func prepGenerator(t *testing.T) *Generator {
	t.Helper()

	name := "demo_" + strconv.FormatInt(time.Now().UnixNano(), 36) + strconv.Itoa(len(t.Name()))

	exec(t, fmt.Sprintf("%s:%s@tcp(%s)/%s?parseTime=true", _rootUser, _rootPass, _addr, _testDB), "CREATE DATABASE "+name)

	dsn := fmt.Sprintf("%s:%s@tcp(%s)/%s?parseTime=true", _rootUser, _rootPass, _addr, name)

	for _, q := range _schema {
		exec(t, dsn, q)
	}

	g := NewGenerator(dsn, slog.New(slog.DiscardHandler))

	// a single day keeps the backfill loop to a few hundred ticks instead
	// of the month the generator runs with in production.
	g.backfillDays = 1

	// a pinned seed makes the first tick at _tick fill every table, so no
	// test depends on a sparse table happening to draw a row.
	g.r = mockmetrics.NewRand(_fullDrawSeed)

	return g
}

// open opens a pool that is closed when the test ends.
func open(t *testing.T, dsn string) *sql.DB {
	t.Helper()

	db, err := sql.Open("mysql", dsn)
	require.NoError(t, err)

	t.Cleanup(func() {
		// a case that closed the pool itself leaves nothing to do here.
		db.Close() //nolint:errcheck,gosec // the pool may already be closed
	})

	require.NoError(t, db.PingContext(context.Background()))

	return db
}

// exec runs a single statement against the given database.
func exec(t *testing.T, dsn, query string) {
	t.Helper()

	db, err := sql.Open("mysql", dsn)
	require.NoError(t, err)

	defer db.Close() //nolint:errcheck // error provides no meaningful info

	_, err = db.ExecContext(context.Background(), query)
	require.NoError(t, err)
}

// count returns the number of rows in a table over a pool of its own, so the
// polling callers cannot pile up connections.
func count(t *testing.T, dsn, table string) int {
	t.Helper()

	db, err := sql.Open("mysql", dsn)
	require.NoError(t, err)

	defer db.Close() //nolint:errcheck // error provides no meaningful info

	var n int

	require.NoError(t, db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM "+table).Scan(&n))

	return n
}
