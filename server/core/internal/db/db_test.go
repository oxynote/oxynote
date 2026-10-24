package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	sq "github.com/Masterminds/squirrel"
	"github.com/dchest/uniuri"
	"github.com/guregu/null/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jmoiron/sqlx"
	"github.com/lann/builder"
	"github.com/orlangure/gnomock"
	pgDocker "github.com/orlangure/gnomock/preset/postgres"
	"github.com/oxynote/oxynote/server/core/internal/tag"
	"github.com/oxynote/oxynote/server/core/internal/variable"
	"github.com/oxynote/oxynote/server/core/pkg/cryptoutil"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/ioutil"
	"github.com/oxynote/oxynote/server/core/pkg/metricutil"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/rs/xid"
	migr "github.com/rubenv/sql-migrate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

var (
	_pgDSN string

	// _pgContainerID is the Docker ID of the test Postgres container.
	_pgContainerID string
)

const (
	_pgUser string = "pgtest"
	_pgPass string = "pgpass"

	// _dataSourceKey is the base64 credentials encryption key every
	// temporary test database is configured with.
	_dataSourceKey = "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="

	// _retiredDataSourceKey is a base64 credentials encryption key no
	// temporary test database is configured with unless a test says so.
	_retiredDataSourceKey = "ZmVkY2JhOTg3NjU0MzIxMGZlZGNiYTk4NzY1NDMyMTA="
)

// _dataSourceKeys is the keyring every temporary test database is
// configured with.
var _dataSourceKeys = mustParseKeys(_dataSourceKey)

// mustParseKeys parses a keyring list or panics.
func mustParseKeys(list string) *cryptoutil.Keyring {
	keys, err := cryptoutil.ParseKeyring(list)
	if err != nil {
		panic("cannot parse test keyring: " + err.Error())
	}

	return keys
}

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

	_pgDSN = container.Host + ":" + strconv.Itoa(container.DefaultPort())
	_pgContainerID = container.DockerID()

	goleak.VerifyTestMain(m, goleak.IgnoreCurrent())
}

func Test_New(t *testing.T) {
	// error - db file not found
	opts := Options{
		DSN: filepath.Join(t.TempDir(), "test", "123"),
	}

	db, err := New(slog.New(slog.DiscardHandler), metricutil.NewFactory("test", nil), opts)
	assert.Error(t, err)
	assert.Nil(t, db)

	// success
	opts = Options{
		DSN: fmt.Sprintf("postgres://%s:%s@%s/postgres?sslmode=disable", _pgUser, _pgPass, _pgDSN),
	}

	db, err = New(slog.New(slog.DiscardHandler), metricutil.NewFactory("test", nil), opts)
	assert.NoError(t, err)
	require.NotNil(t, db)
	assert.NotZero(t, db.log)
	assert.NotNil(t, db.agent)
	assert.NotNil(t, db.closer)
	assert.NotZero(t, db.builder)

	assert.NoError(t, db.closer.Close())
}

func Test_DB_Close(t *testing.T) {
	db := &DB{
		closer: ioutil.CloserFunc(func() error {
			return assert.AnError
		}),
	}

	assert.Error(t, db.Close())

	db.closer = ioutil.CloserFunc(func() error {
		return nil
	})

	assert.NoError(t, db.Close())
}

func Test_DetectError(t *testing.T) {
	type tcase struct {
		Err    error
		Result error
	}

	cc := map[string]func(*testing.T) tcase{
		"Not found": func(*testing.T) tcase {
			return tcase{
				Err:    sql.ErrNoRows,
				Result: errutil.ErrNotFound,
			}
		},
		"Error pass through": func(*testing.T) tcase {
			return tcase{
				Err:    sql.ErrConnDone,
				Result: sql.ErrConnDone,
			}
		},
		"No error": func(*testing.T) tcase {
			return tcase{}
		},
		"Unmatched pgx constraint": func(*testing.T) tcase {
			return tcase{
				Err: &pgconn.PgError{
					Code: "123",
				},
				Result: &pgconn.PgError{
					Code: "123",
				},
			}
		},
		"Unmatched constraint": func(*testing.T) tcase {
			return tcase{
				Err:    errors.New("b"),
				Result: errors.New("b"),
			}
		},
		"Duplicate branch name": func(t *testing.T) tcase {
			db := prepTempDB(t)

			// renaming a branch onto a sibling's name trips the unique key
			// that DetectError has to recognise.
			branches := prepDocumentBranches(t, db, 2, nil)

			second := branches[1]
			second.BranchName = branches[0].BranchName

			err := db.UpdateDocumentBranchMetadata(context.Background(), *second)
			require.Error(t, err)

			return tcase{
				Err:    err,
				Result: errutil.New(http.StatusBadRequest, "document_branch.duplicate_name", "branch name is already in use"),
			}
		},
		"Duplicate file id": func(t *testing.T) tcase {
			db := prepTempDB(t)

			f := prepDocumentFiles(t, db, 1, nil)[0]

			err := db.InsertDocumentFile(context.Background(), f)
			require.Error(t, err)

			return tcase{
				Err:    err,
				Result: errutil.New(http.StatusConflict, "document_file.exists", "file id is already in use"),
			}
		},
		"Duplicate tag name": func(t *testing.T) tcase {
			db := prepTempDB(t)

			existing := prepTags(t, db, 1, nil)[0]

			dup := tag.NewTag(
				tag.CreateInput{TagName: existing.TagName, Color: "green"},
				existing.OrganizationID,
				prepUsers(t, db, 1)[0],
			)

			err := db.InsertTag(context.Background(), dup)
			require.Error(t, err)

			return tcase{
				Err:    err,
				Result: tag.ErrDuplicateTagName,
			}
		},
		"Duplicate variable group name": func(*testing.T) tcase {
			return tcase{
				Err:    &pgconn.PgError{Code: "23505", ConstraintName: "personal_variable_groups_fk_organization_id_group_name_key"},
				Result: variable.ErrDuplicateGroupName,
			}
		},
		"Duplicate variable key": func(*testing.T) tcase {
			return tcase{
				Err:    &pgconn.PgError{Code: "23505", ConstraintName: "personal_variables_fk_organization_id_variable_key_key"},
				Result: variable.ErrDuplicateKey,
			}
		},
		"Unmatched foreign key": func(*testing.T) tcase {
			return tcase{
				Err:    &pgconn.PgError{Code: "23503", ConstraintName: "tags_fk_organization_id_fkey"},
				Result: &pgconn.PgError{Code: "23503", ConstraintName: "tags_fk_organization_id_fkey"},
			}
		},
	}

	for cn, cfn := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			c := cfn(t)

			assert.Equal(t, c.Result, DetectError(c.Err))
		})
	}
}

func Test_DB_migrateUp(t *testing.T) {
	db := prepTempDB(t)

	organizationID := prepOrganizations(t, db, 1)[0]

	// rolling back to before the variables migration (008) and up again
	// runs its backfill over an organization that existed before it, and
	// the palette name migration (010) over tags stored as hex.
	fsys, err := fs.Sub(_migrations, "migrations")
	require.NoError(t, err)

	_, err = db.migrations.ExecVersion(
		db.sql.DB,
		"postgres",
		&migr.HttpFileSystemMigrationSource{FileSystem: http.FS(fsys)},
		migr.Down,
		7,
	)
	require.NoError(t, err)

	hexes := []string{"#f54a00", "#e7010c", "#155DFC", "rgb(1, 2, 3)"}

	prepTags(t, db, len(hexes), func(i int, tg *tag.Tag) {
		tg.OrganizationID = organizationID
		tg.Color = hexes[i]
	})

	require.NoError(t, db.migrateUp())

	tags, err := db.FetchTagTree(context.Background(), organizationID, "")
	require.NoError(t, err)

	colors := make([]string, 0, len(tags))

	for _, s := range tags {
		colors = append(colors, s.Color)
	}

	assert.Equal(t, []string{"orange", "red", "blue", "green"}, colors)

	tree, err := db.FetchVariableTree(context.Background(), organizationID, "")
	require.NoError(t, err)
	require.Len(t, tree, 1)
	require.Len(t, tree[0].Variables, 2)

	exp := variable.SeedBuiltins(organizationID)

	// option ids are random on both sides, so the list default is checked
	// against each side's own first option and then set aside.
	for _, g := range []variable.GroupTree{exp, tree[0]} {
		refresh := g.Variables[1]
		assert.Equal(t, null.StringFrom(refresh.Options[0].ID.String()), refresh.DefaultValue)

		g.Variables[1].DefaultValue = null.String{}
	}

	testutil.AssertFilterEqual(t, exp, tree[0], xid.ID{}, time.Time{})
}

func Test_DB_BeginTx(t *testing.T) {
	type test2 interface {
		Commit() error
	}

	cc := map[string]struct {
		CancelledContext bool
		Dest             any
		Panics           bool
		PanicValue       any
		Err              error
	}{
		"Destination is not a pointer": {
			Dest:       struct{}{},
			Panics:     true,
			PanicValue: "dest is not a pointer",
		},
		"Destination is nil": {
			Dest:       (*struct{})(nil),
			Panics:     true,
			PanicValue: "dest is not a pointer",
		},
		"Destination cannot hold a transaction": {
			Dest:   &struct{}{},
			Panics: true,
		},
		"Cancelled context": {
			CancelledContext: true,
			Dest:             new(test2),
			Err:              assert.AnError,
		},
		"Successful begin": {
			Dest: new(test2),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := prepTempDB(t)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			if c.CancelledContext {
				cancel()
			}

			if c.Panics {
				if c.PanicValue == nil {
					require.Panics(t, func() {
						require.NoError(t, db.BeginTx(ctx, c.Dest))
					})

					return
				}

				require.PanicsWithValue(t, c.PanicValue, func() {
					require.NoError(t, db.BeginTx(ctx, c.Dest))
				})

				return
			}

			err := db.BeginTx(ctx, c.Dest)
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			val := *c.Dest.(*test2)
			assert.IsType(t, &Tx{}, val)
			assert.NoError(t, val.Commit())
		})
	}
}

func Test_Tx_SQLTx(t *testing.T) {
	db := prepTempDB(t)

	var ntx *Tx

	require.NoError(t, db.BeginTx(context.Background(), &ntx))

	defer ntx.Rollback() //nolint:errcheck // error provides no meaningful info

	stx := ntx.SQLTx()
	require.NotNil(t, stx)
	assert.Same(t, ntx.sql.Tx, stx)
}

func Test_Tx_Commit(t *testing.T) {
	db := prepTempDB(t)

	type test interface {
		Commit() error
	}

	var ntx test

	require.NoError(t, db.BeginTx(context.Background(), &ntx))
	require.NoError(t, ntx.Commit())
	require.Error(t, ntx.Commit())
}

func Test_Tx_Rollback(t *testing.T) {
	db := prepTempDB(t)

	type test interface {
		Rollback() error
	}

	var ntx test

	require.NoError(t, db.BeginTx(context.Background(), &ntx))
	require.NoError(t, ntx.Rollback())
	require.Error(t, ntx.Rollback())
}

// prepMockDB creates an agent backed by sqlmock for reaching error
// branches that a real database cannot produce (e.g. failures in the
// middle of a multi-statement transaction).
func prepMockDB(t *testing.T) (*agent, sqlmock.Sqlmock) {
	t.Helper()

	mockDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)

	t.Cleanup(func() {
		mockDB.Close() //nolint:errcheck,gosec // error provides no meaningful info
		require.NoError(t, mock.ExpectationsWereMet())
	})

	return &agent{
		sql:     sqlx.NewDb(mockDB, "sqlmock"),
		builder: sq.StatementBuilderType(builder.EmptyBuilder).PlaceholderFormat(sq.Dollar),
	}, mock
}

func prepTempDB(t *testing.T) *DB {
	name := uniuri.NewLenChars(10, []byte("abcdefghijklmnopqrstuvwxyz"))

	opts := Options{
		DSN:                       fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable", _pgUser, _pgPass, _pgDSN, name),
		DataSourceCredentialsKeys: _dataSourceKeys,
	}

	tmpDB, err := sqlx.Connect(
		"pgx",
		fmt.Sprintf("postgres://%s:%s@%s/postgres?sslmode=disable", _pgUser, _pgPass, _pgDSN),
	)
	require.NoError(t, err)

	_, err = tmpDB.Exec("CREATE DATABASE " + name)
	require.NoError(t, err)
	require.NoError(t, tmpDB.Close())

	db, err := New(slog.New(slog.DiscardHandler), metricutil.NewFactory("test", nil), opts)
	require.NoError(t, err)

	t.Cleanup(func() {
		db.Close() //nolint:errcheck,gosec // error provides no meaningful info
	})

	return db
}
