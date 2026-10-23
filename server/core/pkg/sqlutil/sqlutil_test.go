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
	cc := map[string]struct {
		SortKey httpserver.SortKey
		Result  string
	}{
		"Descending order": {
			SortKey: httpserver.SortKey{Key: "test", Asc: false},
			Result:  "test DESC",
		},
		"Ascending order": {
			SortKey: httpserver.SortKey{Key: "test", Asc: true},
			Result:  "test ASC",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, SortString(c.SortKey))
		})
	}
}

func Test_WrapTx(t *testing.T) {
	cc := map[string]struct {
		Expect  func(sqlmock.Sqlmock)
		Agent   func(*testing.T, *sqlx.DB) any
		FnErr   error
		FnPanic bool
		FnCalls int
		Panic   any
		Err     error
	}{
		"Invalid agent type": {
			Agent: func(*testing.T, *sqlx.DB) any {
				return struct{}{}
			},
			Panic: "invalid agent type",
		},
		"Error returned by DB.BeginTxx": {
			Expect: func(m sqlmock.Sqlmock) {
				m.ExpectBegin().WillReturnError(assert.AnError)
			},
			Agent: func(_ *testing.T, db *sqlx.DB) any { return db },
			Err:   assert.AnError,
		},
		"Error returned by fn": {
			Expect: func(m sqlmock.Sqlmock) {
				m.ExpectBegin()
				m.ExpectRollback()
			},
			Agent:   func(_ *testing.T, db *sqlx.DB) any { return db },
			FnErr:   assert.AnError,
			FnCalls: 1,
			Err:     assert.AnError,
		},
		"Panic in fn": {
			Expect: func(m sqlmock.Sqlmock) {
				m.ExpectBegin()
				m.ExpectRollback()
			},
			Agent:   func(_ *testing.T, db *sqlx.DB) any { return db },
			FnPanic: true,
			FnCalls: 1,
			Panic:   assert.AnError,
		},
		"Error returned by Tx.Commit": {
			Expect: func(m sqlmock.Sqlmock) {
				m.ExpectBegin()
				m.ExpectCommit().WillReturnError(assert.AnError)
			},
			Agent:   func(_ *testing.T, db *sqlx.DB) any { return db },
			FnCalls: 1,
			Err:     assert.AnError,
		},
		"Successful execution with DB": {
			Expect: func(m sqlmock.Sqlmock) {
				m.ExpectBegin()
				m.ExpectCommit()
			},
			Agent:   func(_ *testing.T, db *sqlx.DB) any { return db },
			FnCalls: 1,
		},
		"Successful execution with Tx": {
			Expect: func(m sqlmock.Sqlmock) {
				m.ExpectBegin()
				m.ExpectRollback()
			},
			Agent: func(t *testing.T, db *sqlx.DB) any {
				tx, err := db.BeginTxx(context.Background(), nil)
				require.NoError(t, err)

				return tx
			},
			FnCalls: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db, mock, err := sqlmock.New()
			require.NoError(t, err)

			defer db.Close() //nolint:errcheck // error provides no meaningful info

			if c.Expect != nil {
				c.Expect(mock)
			}

			agent := c.Agent(t, sqlx.NewDb(db, ""))

			var calls int

			fn := func(_ *sqlx.Tx) error {
				calls++

				if c.FnPanic {
					panic(assert.AnError)
				}

				return c.FnErr
			}

			if c.Panic != nil {
				assert.PanicsWithValue(t, c.Panic, func() {
					err = WrapTx(context.Background(), agent, fn)
				})
			} else {
				err = WrapTx(context.Background(), agent, fn)
			}

			assert.Equal(t, c.Err, err)
			assert.Equal(t, c.FnCalls, calls)

			if tx, ok := agent.(*sqlx.Tx); ok {
				assert.NoError(t, tx.Rollback())
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
