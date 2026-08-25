// Package retryutil provides retry helpers for operations that are expected
// to succeed once another party catches up.
package retryutil

import (
	"context"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
)

var (
	// _untilFoundInterval is the interval between not-found retries.
	_untilFoundInterval = time.Second

	// _untilFoundMaxRetries is the maximum number of not-found retries.
	_untilFoundMaxRetries uint64 = 5
)

// Retry retries fn, backing off with the given strategy, until it
// succeeds, the context ends, or maxRetries retries are spent; the last
// error is returned once the retries run out.
func Retry(ctx context.Context, strategy backoff.BackOff, maxRetries uint64, fn func() error) error {
	return backoff.Retry(
		fn,
		backoff.WithMaxRetries(
			backoff.WithContext(strategy, ctx),
			maxRetries,
		),
	)
}

// UntilFound retries fn while it reports that the target does not exist yet,
// which gives a row another party is still committing a bounded chance to
// land. Any other failure ends the retry immediately and is returned as it
// is; the last not-found error is returned once the retries run out.
func UntilFound(ctx context.Context, fn func(ctx context.Context) error) error {
	return backoff.Retry(
		func() error {
			err := fn(ctx)
			if err != nil && !errutil.IsNotFound(err) {
				return &backoff.PermanentError{Err: err}
			}

			return err
		},
		backoff.WithMaxRetries(
			backoff.WithContext(
				backoff.NewConstantBackOff(_untilFoundInterval),
				ctx,
			),
			_untilFoundMaxRetries,
		),
	)
}
