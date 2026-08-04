package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/dchest/uniuri"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jmoiron/sqlx"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/ioutil"
	"github.com/oxynote/oxynote/server/core/pkg/metricutil"
	"github.com/orlangure/gnomock"
	pgDocker "github.com/orlangure/gnomock/preset/postgres"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

var _pgDSN string

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

func Test_SystemDB_Close(t *testing.T) {
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
	cc := map[string]struct {
		Err    error
		Result error
	}{
		"Not found": {
			Err:    sql.ErrNoRows,
			Result: errutil.ErrNotFound,
		},
		"Error pass through": {
			Err:    sql.ErrConnDone,
			Result: sql.ErrConnDone,
		},
		"No error": {
			Err:    nil,
			Result: nil,
		},
		"Unmatched pgx constraint": {
			Err: &pgconn.PgError{
				Code: "123",
			},
			Result: &pgconn.PgError{
				Code: "123",
			},
		},
		"Unmatched constraint": {
			Err:    errors.New("b"),
			Result: errors.New("b"),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			assert.Equal(t, c.Result, DetectError(c.Err))
		})
	}
}

func Test_SystemDB_BeginTx(t *testing.T) {
	db := prepTempDB(t)

	// provided destination isn't a pointer
	require.PanicsWithValue(
		t,
		"dest is not a pointer",
		func() {
			_ = db.BeginTx(context.Background(), struct{}{})
		},
	)

	// provided destination is nil
	require.PanicsWithValue(
		t,
		"dest is not a pointer",
		func() {
			_ = db.BeginTx(context.Background(), (*struct{})(nil))
		},
	)

	// cannot fulfill destination requirements
	require.Panics(
		t,
		func() {
			_ = db.BeginTx(context.Background(), &struct{}{})
		},
	)

	type test2 interface {
		Commit() error
	}

	var val test2

	// context error
	nctx, cancel := context.WithCancel(context.Background())
	cancel()

	require.Error(t, db.BeginTx(nctx, &val))

	// success
	require.NoError(t, db.BeginTx(context.Background(), &val))
	assert.IsType(t, &Tx{}, val)
	assert.NoError(t, val.Commit())
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

func prepTempDB(t *testing.T) *DB {
	name := uniuri.NewLenChars(10, []byte("abcdefghijklmnopqrstuvwxyz"))

	opts := Options{
		DSN: fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable", _pgUser, _pgPass, _pgDSN, name),
	}

	tmpDB, err := sqlx.Connect(
		"pgx",
		fmt.Sprintf("postgres://%s:%s@%s/postgres?sslmode=disable", _pgUser, _pgPass, _pgDSN),
	)
	require.NoError(t, err)

	_, err = tmpDB.Exec(fmt.Sprintf("CREATE DATABASE %s", name))
	require.NoError(t, err)
	require.NoError(t, tmpDB.Close())

	db, err := New(slog.New(slog.DiscardHandler), metricutil.NewFactory("test", nil), opts)
	require.NoError(t, err)

	t.Cleanup(func() {
		db.Close() //nolint:errcheck,gosec // error provides no meaningful info
	})

	return db
}
