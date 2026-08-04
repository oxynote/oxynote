// Package db provides functionality to persistently store data of
// the whole application system.
package db

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"math/rand"
	"net/http"
	"reflect"
	"strconv"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5/pgconn"
	pgx "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
	"github.com/lann/builder"
	"github.com/oxynote/purse/util/errutil"
	"github.com/oxynote/purse/util/ioutil"
	"github.com/oxynote/purse/util/metricutil"
	"github.com/oxynote/purse/util/sqlutil"
	"github.com/oxynote/purse/util/timeutil"
	"github.com/qustavo/sqlhooks/v2"
	migr "github.com/rubenv/sql-migrate"
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
	migrations migr.MigrationSet

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

	// DataSourceCredentialsSigningSecret is the secret used to sign
	// data source credentials.
	DataSourceCredentialsSigningSecret string
}

// New creates a fresh instance of database.
func New(
	log *slog.Logger,
	metrics metricutil.Factory,
	opts Options,
) (*DB, error) {
	// we use the unix nano timestamp as the driver name to ensure
	// that tests are able to register multiple drivers.
	driverName := strconv.Itoa(int(timeutil.Now().UnixNano()) + rand.Intn(1000)) //nolint:gosec // basic randomizer is enough for random driver name

	drv := pgx.GetDefaultDriver()

	sql.Register(driverName, sqlhooks.Wrap(
		drv,
		sqlutil.NewHooks(
			metrics,
			sqlutil.WithHookErrorHandler(DetectError),
			sqlutil.WithHookDatabaseName("oxynote"),
			sqlutil.WithHookDurationThreshold(time.Millisecond*100),
		)),
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

	db := &DB{
		log: log.With("component", "database"),
		sql: sqlDB,
		agent: agent{
			sql:     sqlDB,
			builder: sq.StatementBuilderType(builder.EmptyBuilder).PlaceholderFormat(sq.Dollar),
			opts:    opts,
		},
		metrics: metrics,
		migrations: migr.MigrationSet{
			TableName: "migrations",
		},
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
	fsys, err := fs.Sub(_migrations, "migrations")
	if err != nil {
		// NOCOV: since all arguments are constants, they
		// are always valid
		return err
	}

	_, err = db.migrations.Exec(
		db.sql.DB,
		"postgres",
		&migr.HttpFileSystemMigrationSource{
			FileSystem: http.FS(fsys),
		},
		migr.Up,
	)

	return err
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

	var perr *pgconn.PgError
	if !errors.As(err, &perr) {
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
		}
	}

	return err
}

// BeginTx begins a transaction and tries to apply its object to the one
// given in dest parameter.
func (db *DB) BeginTx(ctx context.Context, dest any) error {
	value := reflect.ValueOf(dest)

	if value.Kind() != reflect.Pointer || value.IsNil() {
		panic("dest is not a pointer")
	}

	tx, err := db.sql.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}

	defer func() {
		if rerr := recover(); rerr != nil {
			tx.Rollback() //nolint:gosec,errcheck // rollback errors provide no meaningful info
			panic(rerr)
		}
	}()

	value.Elem().Set(reflect.ValueOf(&Tx{
		agent: agent{
			sql:     tx,
			builder: db.builder,
			opts:    db.opts,
		},
		sql: tx,
	}))

	return nil
}

// Tx contains dependencies needed for direct communication
// with the database in a transaction mode.
type Tx struct {
	agent

	sql *sqlx.Tx
}

// Commit commits a transaction.
func (tx *Tx) Commit() error {
	return tx.sql.Commit()
}

// Rollback rollbacks a transaction.
func (tx *Tx) Rollback() error {
	return tx.sql.Rollback()
}
