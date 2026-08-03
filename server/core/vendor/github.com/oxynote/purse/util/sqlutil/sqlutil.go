// Package sqlutil provides helper utilities for SQL operations.
package sqlutil

import (
	"context"
	"fmt"
	"strings"

	sq "github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"
	"github.com/oxynote/purse/http/httpserver"
)

// DB is an interface that should be implemented by all
// database managers.
type DB interface {
	// BeginTx should begin a transaction and set the active Tx
	// object to the dest parameter.
	// Example:
	//    type CustomTx interface {
	//        Tx // embed core transaction interface
	//        FetchUser() User
	//    }
	//
	//    var tx CustomTx
	//    db.BeginTx(context.Background(), &tx)
	//
	// NOTE #1: when go1.18 with the new type parameter feature is
	// released, the signature of this method, as well as all of its
	// implementations will need to be rewritten.
	// NOTE #2: since the only way to set a transaction object to the
	// destination parameter is by using reflection, this may not
	// be safe at compile-time, so destination interfaces need to be
	// thoroughly checked. However, this will not be a problem when
	// type parameters are introduced.
	BeginTx(ctx context.Context, dest any) error
}

// Tx is an interface that should be implemented by all
// transactions created by any database manager.
type Tx interface {
	// Commit should commit the active transaction.
	Commit() error

	// Rollback should rollback the active transaction.
	Rollback() error
}

// Select validates the provided query and uses it to start a SELECT SQL
// query.
// Filter and sort functions are called even when there are no filters
// or sort keys in the query. In these cases, the input is made of zero
// values (useful for default filter/sort settings).
// 'page_count' column is added and indicates the total count of pages
// that is available with the provided filters and limits.
// All dashes in query keys are transformed into underscores before
// filter/sort functions are called:
//
//	created-at -> created_at.
//	redeem-time-from -> redeem_time_from.
func Select(
	qr httpserver.Query,
	filter func(b sq.SelectBuilder, k, v string) (sq.SelectBuilder, error),
	sort func(b sq.SelectBuilder, k httpserver.SortKey) (sq.SelectBuilder, error),
	pageCount func(b sq.SelectBuilder, limit uint64) sq.SelectBuilder,
) (sq.SelectBuilder, error) {
	if err := qr.Validate(); err != nil {
		return sq.SelectBuilder{}, err
	}

	b := sq.Select().Limit(qr.Limit).Offset(qr.Limit * (qr.Page - 1))
	b = pageCount(b, qr.Limit)

	var err error

	for k, v := range qr.Filters {
		b, err = filter(b, strings.ReplaceAll(k, "-", "_"), v)
		if err != nil {
			return sq.SelectBuilder{}, err
		}
	}

	// make sure that filter func is called at least once
	if len(qr.Filters) == 0 {
		b, err = filter(b, "", "")
		if err != nil {
			return sq.SelectBuilder{}, err
		}
	}

	for _, v := range qr.SortKeys {
		v.Key = strings.ReplaceAll(v.Key, "-", "_")

		b, err = sort(b, v)
		if err != nil {
			return sq.SelectBuilder{}, err
		}
	}

	// make sure that sort func is called at least once
	if len(qr.SortKeys) == 0 {
		b, err = sort(b, httpserver.SortKey{})
		if err != nil {
			return sq.SelectBuilder{}, err
		}
	}

	return b, nil
}

// PageCount is the default way to calculate page count.
func PageCount(b sq.SelectBuilder, limit uint64) sq.SelectBuilder {
	return b.Column(`CAST(CEIL(COUNT(*) OVER() / (? * 1.0)) AS INTEGER) AS page_count`, limit).
		PlaceholderFormat(sq.Dollar)
}

// SortString transforms sort key type into a valid sorting clause string.
func SortString(k httpserver.SortKey) string {
	ord := "DESC"
	if k.Asc {
		ord = "ASC"
	}

	return fmt.Sprintf("%s %s", k.Key, ord)
}

// WrapTx executes the provided function in a transaction.
// NOTE: Agent should be a valid *sqlx.Tx or *sqlx.DB pointer.
func WrapTx(ctx context.Context, agent any, fn func(*sqlx.Tx) error) (err error) {
	var tx *sqlx.Tx

	switch tp := agent.(type) {
	case *sqlx.DB:
		tx, err = tp.BeginTxx(ctx, nil)
		if err != nil {
			return err
		}

		defer func() {
			if rerr := recover(); rerr != nil {
				tx.Rollback() //nolint:errcheck,gosec // error provides no meaningful info
				panic(rerr)   // re-throw panic after tx rollback
			} else if err != nil {
				tx.Rollback() //nolint:errcheck,gosec // error provides no meaningful info
				return
			}

			err = tx.Commit()
		}()
	case *sqlx.Tx:
		tx = tp
	default:
		panic("invalid agent type")
	}

	return fn(tx)
}
