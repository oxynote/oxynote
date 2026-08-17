package retryutil

import (
	"context"
	"testing"
	"time"

	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func Test_UntilFound(t *testing.T) {
	_untilFoundInterval = time.Millisecond

	cc := map[string]struct {
		Context context.Context
		Errs    []error
		Calls   int
		Err     error
	}{
		"Found on the first attempt": {
			Errs:  []error{nil},
			Calls: 1,
		},
		"Found once the row lands": {
			Errs:  []error{errutil.ErrNotFound, errutil.ErrNotFound, nil},
			Calls: 3,
		},
		"Error other than not found ends the retries": {
			Errs:  []error{assert.AnError},
			Calls: 1,
			Err:   assert.AnError,
		},
		"Never found": {
			Errs:  []error{errutil.ErrNotFound},
			Calls: int(_untilFoundMaxRetries) + 1,
			Err:   errutil.ErrNotFound,
		},
		"Context cancelled": {
			Context: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()

				return ctx
			}(),
			Errs:  []error{errutil.ErrNotFound},
			Calls: 1,
			Err:   context.Canceled,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			ctx := c.Context
			if ctx == nil {
				ctx = context.Background()
			}

			var calls int

			err := UntilFound(ctx, func(_ context.Context) error {
				calls++

				if calls > len(c.Errs) {
					return c.Errs[len(c.Errs)-1]
				}

				return c.Errs[calls-1]
			})

			testutil.AssertEqualError(t, c.Err, err)
			assert.Equal(t, c.Calls, calls)
		})
	}
}
