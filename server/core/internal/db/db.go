// Package db provides functionality to persistently store data of
// the whole application system.
package db

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"math/rand"
	"net/http"
	"strconv"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5/pgconn"
	pgx "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
	"github.com/lann/builder"
	"github.com/oxynote/oxynote/server/core/internal/db/migrations"
	"github.com/oxynote/oxynote/server/core/internal/tag"
	"github.com/oxynote/oxynote/server/core/internal/variable"
	"github.com/oxynote/oxynote/server/core/pkg/cryptoutil"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/ioutil"
	"github.com/oxynote/oxynote/server/core/pkg/metricutil"
	"github.com/oxynote/oxynote/server/core/pkg/sqlutil"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/database"
	"github.com/qustavo/sqlhooks/v2"
)

// _migrations contains all database migrations.
//
//go:embed migrations
var _migrations embed.FS

const (
	// _maxIdleConnections is the maximum number of idle connections to the database.
	_maxIdleConnections = 10

	// _connMaxIdleTime is the maximum amount of time a connection may be idle.
	_connMaxIdleTime = 5 * time.Second

	// _maxOpenConnections is the maximum number of open connections to the database.
	_maxOpenConnections = 100

	// _connMaxLifetime is the maximum amount of time a connection may be reused.
	_connMaxLifetime = 2 * time.Minute

	// _driverNameRandBound bounds the random suffix appended to the
	// registered driver name.
	_driverNameRandBound = 1000

	// _slowQueryThreshold is the query duration above which the hooks
	// report a slow query.
	_slowQueryThreshold = time.Millisecond * 100
)

type agent struct {
	sql     sqlx.ExtContext
	builder sq.StatementBuilderType
	opts    Options
}

// DB contains dependencies needed for direct communication
// with the database.
type DB struct {
	agent

	log        *slog.Logger
	sql        *sqlx.DB
	metrics    metricutil.Factory
	migrations *goose.Provider

	closer io.Closer
}

// Options holds all database configuration options.
type Options struct {
	// DSN specifies the location of the database.
	DSN string

	// MaxNotifications specifies the maximum number of notifications
	// to keep per user.o
	// Zero means no limit.
	MaxNotifications uint64

	// MaxDocumentHistoryEntries specifies the maximum number of history
	// entries to keep per document branch.
	// Zero means no limit.
	MaxDocumentHistoryEntries uint64

	// MaxSlackMessages specifies the maximum number of Slack messages to
	// keep per organization. It bounds the listing as well, which can
	// never legitimately need more than retention holds.
	// Zero means no limit.
	MaxSlackMessages uint64

	// DocumentHistoryRetention specifies how long history entries are
	// kept before the file manager trims them.
	// Zero means no age limit.
	DocumentHistoryRetention time.Duration

	// DataSourceCredentialsKeys holds the keys data source credentials
	// are encrypted with. The first key encrypts; every key decrypts.
	// It must be set: New does not check it, and a nil keyring panics on
	// the first credential written or read.
	DataSourceCredentialsKeys *cryptoutil.Keyring
}

// New creates a fresh instance of database.
func New(
	log *slog.Logger,
	metrics metricutil.Factory,
	opts Options,
) (*DB, error) {
	// we use the unix nano timestamp as the driver name to ensure
	// that tests are able to register multiple drivers.
	driverName := strconv.Itoa(int(timeutil.Now().UnixNano()) + rand.Intn(_driverNameRandBound)) //nolint:gosec // basic randomizer is enough for random driver name

	drv := pgx.GetDefaultDriver()

	sql.Register(driverName, sqlhooks.Wrap(
		drv,
		sqlutil.NewHooks(
			metrics,
			sqlutil.WithHookErrorHandler(DetectError),
			sqlutil.WithHookDatabaseName("oxynote"),
			sqlutil.WithHookDurationThreshold(_slowQueryThreshold),
		),
	),
	)

	sqlDB, err := sqlx.Connect(driverName, opts.DSN)
	if err != nil {
		return nil, err
	}

	// NOTE: although 'unsafe' sounds scary, it only affects
	// column parsing: when some columns are not found in
	// the destination struct, it won't return an error (the
	// default behaviour is to return one). This is useful
	// when parsing rows that contain a 'page_count' column.
	sqlDB = sqlDB.Unsafe()

	sqlDB.SetMaxIdleConns(_maxIdleConnections)
	sqlDB.SetConnMaxIdleTime(_connMaxIdleTime)
	sqlDB.SetMaxOpenConns(_maxOpenConnections)
	sqlDB.SetConnMaxLifetime(_connMaxLifetime)

	closers := []io.Closer{sqlDB}

	fsys, err := fs.Sub(_migrations, "migrations")
	if err != nil {
		// NOCOV: since all arguments are constants, they
		// are always valid.
		return nil, ioutil.AppendCloseErr(
			ioutil.MultiCloser(true, closers...),
			err,
		)
	}

	// NOTE: the global registry is disabled so that a goose.AddMigration
	// call in any imported package cannot add a migration to this run.
	provider, err := goose.NewProvider(
		goose.DialectPostgres,
		sqlDB.DB,
		fsys,
		goose.WithGoMigrations(migrations.Go()...),
		goose.WithDisableGlobalRegistry(true),
	)
	if err != nil {
		// NOCOV: error cannot be produced because the
		// migrations are fixed at build time.
		return nil, ioutil.AppendCloseErr(
			ioutil.MultiCloser(true, closers...),
			err,
		)
	}

	db := &DB{
		log: log.With("component", "database"),
		sql: sqlDB,
		agent: agent{
			sql:     sqlDB,
			builder: sq.StatementBuilderType(builder.EmptyBuilder).PlaceholderFormat(sq.Dollar),
			opts:    opts,
		},
		metrics:    metrics,
		migrations: provider,
	}

	if err = db.migrateUp(); err != nil {
		// NOCOV: error cannot be produced because the
		// database cannot be manipulated (eg. closed) from
		// the outside yet.
		return nil, ioutil.AppendCloseErr(
			ioutil.MultiCloser(true, closers...),
			err,
		)
	}

	db.closer = ioutil.MultiCloser(true, closers...)

	return db, nil
}

// migrateUp prepares the database for usage.
func (db *DB) migrateUp() error {
	ctx := context.Background()

	if err := db.moveSQLMigrateHistory(ctx); err != nil {
		return fmt.Errorf("moving sql-migrate history: %w", err)
	}

	_, err := db.migrations.Up(ctx)

	return err
}

// moveSQLMigrateHistory copies the versions that rubenv/sql-migrate
// recorded in its migrations table into the goose version table, then
// drops the migrations table. It does nothing when that table does not
// exist.
func (db *DB) moveSQLMigrateHistory(ctx context.Context) error {
	tx, err := db.sql.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}

	defer tx.Rollback() //nolint:errcheck // error provides no meaningful info

	var hasHistory, hasVersionTable bool

	// NOTE: sql-migrate named its table without a schema, so the table sits
	// wherever the search_path pointed. Goose looks for its own table only
	// in the current schema. Each lookup here matches the tool it checks
	// for.
	err = tx.QueryRowContext(
		ctx,
		"SELECT to_regclass('migrations') IS NOT NULL, to_regclass(format('%I.%I', current_schema(), $1::text)) IS NOT NULL",
		goose.DefaultTablename,
	).Scan(&hasHistory, &hasVersionTable)
	if err != nil {
		// NOCOV: a test cannot make this statement fail
		// without failing BeginTx first.
		return fmt.Errorf("checking tables: %w", err)
	}

	if !hasHistory {
		return nil
	}

	store, err := database.NewStore(database.DialectPostgres, goose.DefaultTablename)
	if err != nil {
		// NOCOV: since all arguments are constants, they
		// are always valid.
		return err
	}

	// NOTE: goose inserts version 0 whenever it creates its table, and
	// its version lookups expect that row.
	if !hasVersionTable {
		if err = store.CreateVersionTable(ctx, tx); err != nil {
			// NOCOV: a test cannot make this statement fail
			// without failing BeginTx first.
			return fmt.Errorf("creating the goose version table: %w", err)
		}

		if err = store.Insert(ctx, tx, database.InsertRequest{Version: 0}); err != nil {
			// NOCOV: a test cannot make this statement fail
			// without failing BeginTx first.
			return fmt.Errorf("recording version 0: %w", err)
		}
	}

	recorded, err := store.ListMigrations(ctx, tx)
	if err != nil {
		// NOCOV: a test cannot make this statement fail
		// without failing BeginTx first.
		return fmt.Errorf("listing goose versions: %w", err)
	}

	known := make(map[int64]bool, len(recorded))

	for _, r := range recorded {
		known[r.Version] = true
	}

	var ids []string

	if err = tx.SelectContext(ctx, &ids, "SELECT id FROM migrations ORDER BY id"); err != nil {
		// NOCOV: a test cannot make this statement fail
		// without failing BeginTx first.
		return fmt.Errorf("listing sql-migrate history: %w", err)
	}

	for _, id := range ids {
		version, perr := goose.NumericComponent(id)
		if perr != nil {
			return fmt.Errorf("parsing sql-migrate id %q: %w", id, perr)
		}

		if known[version] {
			continue
		}

		if err = store.Insert(ctx, tx, database.InsertRequest{Version: version}); err != nil {
			// NOCOV: a test cannot make this statement fail
			// without failing BeginTx first.
			return fmt.Errorf("recording version %d: %w", version, err)
		}
	}

	if _, err = tx.ExecContext(ctx, "DROP TABLE migrations"); err != nil {
		// NOCOV: a test cannot make this statement fail
		// without failing BeginTx first.
		return fmt.Errorf("dropping the sql-migrate table: %w", err)
	}

	return tx.Commit()
}

// Close closes the database.
func (db *DB) Close() error {
	return db.closer.Close()
}

// DetectError converts internal, database-specific errors into user errors.
// PostgreSQL error codes: https://www.postgresql.org/docs/11/errcodes-appendix.html.
func DetectError(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return errutil.ErrNotFound
	}

	perr, ok := errors.AsType[*pgconn.PgError](err)
	if !ok {
		return err
	}

	if perr.Code == "23505" { // Unique violation.
		switch perr.ConstraintName {
		case "data_sources_fk_organization_id_name_key":
			return errutil.New(
				http.StatusBadRequest,
				"data_source.duplicate_name",
				"name is already in use",
			)
		case "document_branches_fk_document_id_branch_name_key":
			return errutil.New(
				http.StatusBadRequest,
				"document_branch.duplicate_name",
				"branch name is already in use",
			)
		case "tags_fk_organization_id_tag_name_key":
			return tag.ErrDuplicateTagName
		case "personal_variable_groups_fk_organization_id_group_name_key":
			return variable.ErrDuplicateGroupName
		case "personal_variables_fk_organization_id_variable_key_key":
			return variable.ErrDuplicateKey
		case "document_files_pkey":
			return errutil.New(
				http.StatusConflict,
				"document_file.exists",
				"file id is already in use",
			)
		}
	}

	return err
}

// BeginTx begins a transaction. It returns a *Tx, which callers get as
// their own transaction type through sqlutil.BeginTx.
func (db *DB) BeginTx(ctx context.Context) (sqlutil.Tx, error) {
	tx, err := db.sql.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}

	return &Tx{
		agent: agent{
			sql:     tx,
			builder: db.builder,
			opts:    db.opts,
		},
		sql: tx,
	}, nil
}

// Tx contains dependencies needed for direct communication
// with the database in a transaction mode.
type Tx struct {
	agent

	sql *sqlx.Tx
}

// SQLTx returns the underlying transaction.
func (tx *Tx) SQLTx() *sql.Tx {
	return tx.sql.Tx
}

// Commit commits a transaction.
func (tx *Tx) Commit() error {
	return tx.sql.Commit()
}

// Rollback rollbacks a transaction.
func (tx *Tx) Rollback() error {
	return tx.sql.Rollback()
}
