package sqlutil

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	sq "github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"
	"github.com/oxynote/oxynote/server/core/pkg/httpserver"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func Test_Select(t *testing.T) {
	cc := map[string]struct {
		FilterFn func(sq.SelectBuilder, string, string) (sq.SelectBuilder, error)
		SortFn   func(sq.SelectBuilder, httpserver.SortKey) (sq.SelectBuilder, error)
		Query    httpserver.Query
		Result   sq.SelectBuilder
		Err      error
	}{
		"Invalid HTTP query": {
			FilterFn: func(b sq.SelectBuilder, k, v string) (sq.SelectBuilder, error) {
				return b.Where(sq.Eq{k: v}), nil
			},
			SortFn: func(b sq.SelectBuilder, k httpserver.SortKey) (sq.SelectBuilder, error) {
				return b.OrderBy(SortString(k)), nil
			},
			Query: httpserver.Query{
				Limit: 0,
				Page:  2,
				Filters: map[string]string{
					"test-1": "123",
				},
				SortKeys: []httpserver.SortKey{
					{
						Key: "sort-test",
						Asc: true,
					},
					{
						Key: "sort1-test",
					},
				},
			},
			Err: assert.AnError,
		},
		"Error returned by filter func when filters map is not empty": {
			FilterFn: func(_ sq.SelectBuilder, _, _ string) (sq.SelectBuilder, error) {
				return sq.SelectBuilder{}, assert.AnError
			},
			SortFn: func(b sq.SelectBuilder, k httpserver.SortKey) (sq.SelectBuilder, error) {
				return b.OrderBy(SortString(k)), nil
			},
			Query: httpserver.Query{
				Limit: 10,
				Page:  2,
				Filters: map[string]string{
					"test-1": "123",
				},
				SortKeys: []httpserver.SortKey{
					{
						Key: "sort-test",
						Asc: true,
					},
					{
						Key: "sort1-test",
					},
				},
			},
			Err: assert.AnError,
		},
		"Error returned by sort key func when sort keys slice is not empty": {
			FilterFn: func(b sq.SelectBuilder, k, v string) (sq.SelectBuilder, error) {
				return b.Where(sq.Eq{k: v}), nil
			},
			SortFn: func(_ sq.SelectBuilder, _ httpserver.SortKey) (sq.SelectBuilder, error) {
				return sq.SelectBuilder{}, assert.AnError
			},
			Query: httpserver.Query{
				Limit: 10,
				Page:  2,
				Filters: map[string]string{
					"test-1": "123",
				},
				SortKeys: []httpserver.SortKey{
					{
						Key: "sort-test",
						Asc: true,
					},
					{
						Key: "sort1-test",
					},
				},
			},
			Err: assert.AnError,
		},
		"Error returned by filter func when filters map is empty": {
			FilterFn: func(_ sq.SelectBuilder, _, _ string) (sq.SelectBuilder, error) {
				return sq.SelectBuilder{}, assert.AnError
			},
			SortFn: func(b sq.SelectBuilder, k httpserver.SortKey) (sq.SelectBuilder, error) {
				return b.OrderBy(SortString(k)), nil
			},
			Query: httpserver.Query{
				Limit: 10,
				Page:  2,
				SortKeys: []httpserver.SortKey{
					{
						Key: "sort-test",
						Asc: true,
					},
					{
						Key: "sort1-test",
					},
				},
			},
			Err: assert.AnError,
		},
		"Error returned by sort func when sort keys slice is empty": {
			FilterFn: func(b sq.SelectBuilder, k, v string) (sq.SelectBuilder, error) {
				return b.Where(sq.Eq{k: v}), nil
			},
			SortFn: func(_ sq.SelectBuilder, _ httpserver.SortKey) (sq.SelectBuilder, error) {
				return sq.SelectBuilder{}, assert.AnError
			},
			Query: httpserver.Query{
				Limit: 10,
				Page:  2,
				Filters: map[string]string{
					"test-1": "123",
				},
			},
			Err: assert.AnError,
		},
		"Successful execution with no filters and sort keys": {
			FilterFn: func(b sq.SelectBuilder, _, _ string) (sq.SelectBuilder, error) {
				return b.Where(sq.Eq{"hello": "123"}), nil
			},
			SortFn: func(b sq.SelectBuilder, _ httpserver.SortKey) (sq.SelectBuilder, error) {
				return b.OrderBy("test DESC"), nil
			},
			Query: httpserver.Query{
				Limit: 10,
				Page:  2,
			},
			Result: sq.Select().
				Column(`CAST(CEIL(COUNT(*) OVER() / (? * 1.0)) AS INTEGER) AS page_count`, uint64(10)).
				Limit(10).Offset(10).Where(sq.Eq{
				"hello": "123",
			}).OrderBy("test DESC").PlaceholderFormat(sq.Dollar),
		},
		"Successful execution": {
			FilterFn: func(b sq.SelectBuilder, k, v string) (sq.SelectBuilder, error) {
				return b.Where(sq.Eq{k: v}), nil
			},
			SortFn: func(b sq.SelectBuilder, k httpserver.SortKey) (sq.SelectBuilder, error) {
				return b.OrderBy(SortString(k)), nil
			},
			Query: httpserver.Query{
				Limit: 10,
				Page:  2,
				Filters: map[string]string{
					"test-1": "123",
				},
				SortKeys: []httpserver.SortKey{
					{
						Key: "sort-test",
						Asc: true,
					},
					{
						Key: "sort1-test",
					},
				},
			},
			Result: sq.Select().
				Column(`CAST(CEIL(COUNT(*) OVER() / (? * 1.0)) AS INTEGER) AS page_count`, uint64(10)).
				Limit(10).Offset(10).Where(sq.Eq{
				"test_1": "123",
			}).OrderBy("sort_test ASC", "sort1_test DESC").PlaceholderFormat(sq.Dollar),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := Select(c.Query, c.FilterFn, c.SortFn, PageCount)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			expSQL, expArgs := c.Result.MustSql()
			resSQL, resArgs := res.MustSql()

			assert.Equal(t, expSQL, resSQL)
			assert.Equal(t, expArgs, resArgs)
		})
	}
}

func Test_SortString(t *testing.T) {
	assert.Equal(t, "test DESC", SortString(httpserver.SortKey{Key: "test", Asc: false}))
	assert.Equal(t, "test ASC", SortString(httpserver.SortKey{Key: "test", Asc: true}))
}

func Test_WrapTx(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)

	defer db.Close() //nolint:errcheck // error provides no meaning info

	var called bool

	dbx := sqlx.NewDb(db, "")
	fn1 := func(_ *sqlx.Tx) error { //nolint:unparam // we must return an error here to fulfill WrapTx parameter requirements
		called = true
		return nil
	}

	fn2 := func(_ *sqlx.Tx) error {
		called = true
		return assert.AnError
	}

	fn3 := func(_ *sqlx.Tx) error {
		called = true

		panic(assert.AnError)
	}

	// error - invalid agent
	assert.PanicsWithValue(t, "invalid agent type", func() {
		require.NoError(t, WrapTx(context.Background(), struct{}{}, fn1))
	})

	// error in begin
	mock.ExpectBegin().WillReturnError(assert.AnError)
	assert.Equal(t, assert.AnError, WrapTx(context.Background(), dbx, fn1))
	assert.NoError(t, mock.ExpectationsWereMet())
	assert.False(t, called)

	// error in fn
	mock.ExpectBegin()
	mock.ExpectRollback()
	assert.Equal(t, assert.AnError, WrapTx(context.Background(), dbx, fn2))
	assert.NoError(t, mock.ExpectationsWereMet())
	assert.True(t, called)

	// panic in fn
	called = false

	mock.ExpectBegin()
	mock.ExpectRollback()
	assert.Equal(t, assert.AnError, func() (err error) {
		defer func() {
			if rerr := recover(); rerr != nil {
				err = rerr.(error)
			}
		}()

		return WrapTx(context.Background(), dbx, fn3)
	}())
	assert.NoError(t, mock.ExpectationsWereMet())
	assert.True(t, called)

	// error in commit
	called = false

	mock.ExpectBegin()
	mock.ExpectCommit().WillReturnError(assert.AnError)
	assert.Equal(t, assert.AnError, WrapTx(context.Background(), dbx, fn1))
	assert.NoError(t, mock.ExpectationsWereMet())
	assert.True(t, called)

	// success - db
	called = false

	mock.ExpectBegin()
	mock.ExpectCommit()
	assert.NoError(t, WrapTx(context.Background(), dbx, fn1))
	assert.NoError(t, mock.ExpectationsWereMet())
	assert.True(t, called)

	// success - tx
	called = false

	mock.ExpectBegin()
	mock.ExpectRollback()

	tx, err := dbx.BeginTxx(context.Background(), nil)
	require.NoError(t, err)

	assert.NoError(t, WrapTx(context.Background(), tx, fn1))
	assert.NoError(t, tx.Rollback())
	assert.NoError(t, mock.ExpectationsWereMet())
	assert.True(t, called)
}
