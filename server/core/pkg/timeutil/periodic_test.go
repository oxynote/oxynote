package timeutil

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_NewPeriodicExec(t *testing.T) {
	t.Parallel()

	pe := NewPeriodicExec(time.Minute, time.Second, func(context.Context) {}, func(any) {}, true)
	require.NotNil(t, pe)
	assert.Equal(t, time.Minute, pe.interval)
	assert.Equal(t, time.Second, pe.offset)
	assert.NotNil(t, pe.fn)
	assert.NotNil(t, pe.recovery)
	assert.True(t, pe.immediate)
	assert.Equal(t, 1, cap(pe.trigger))
}

func Test_PeriodicExec_Trigger(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Pending bool
	}{
		"Trigger is queued when none is pending": {},
		"Repeated triggers never block and collapse into one pending run": {
			Pending: true,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			pe := NewPeriodicExec(time.Hour, 0, func(context.Context) {}, nil, false)

			if c.Pending {
				pe.Trigger()
			}

			pe.Trigger()
			assert.Len(t, pe.trigger, 1)
		})
	}
}

func Test_PeriodicExec_Start(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Immediate bool
		Interval  time.Duration
		Trigger   bool
		Runs      int
	}{
		"Cancelled before the first interval elapses": {
			Interval: time.Hour,
		},
		"Immediate run happens before any waiting": {
			Immediate: true,
			Interval:  time.Hour,
			Runs:      1,
		},
		"Runs repeat on the interval": {
			Interval: time.Millisecond,
			Runs:     3,
		},
		"Trigger runs the function without waiting for the interval": {
			Interval: time.Hour,
			Trigger:  true,
			Runs:     1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			var runs int

			stopCh := make(chan struct{})

			pe := NewPeriodicExec(c.Interval, 0, func(context.Context) {
				runs++

				if runs >= c.Runs {
					cancel()
				}
			}, nil, c.Immediate)

			go func() {
				defer close(stopCh)

				pe.Start(ctx)
			}()

			if c.Trigger {
				pe.Trigger()
			}

			if c.Runs == 0 {
				cancel()
			}

			<-stopCh

			assert.Equal(t, c.Runs, runs)
		})
	}
}

func Test_PeriodicExec_exec(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Panic     bool
		Recovery  bool
		Panics    bool
		Recovered []any
	}{
		"Panic is handed to the recovery function instead of unwinding": {
			Panic:     true,
			Recovery:  true,
			Recovered: []any{"boom"},
		},
		"Panic keeps unwinding without a recovery function": {
			Panic:  true,
			Panics: true,
		},
		"Run that does not panic leaves nothing behind": {
			Recovery: true,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var (
				runs      int
				recovered []any
				recovery  func(v any)
			)

			if c.Recovery {
				recovery = func(v any) {
					recovered = append(recovered, v)
				}
			}

			pe := NewPeriodicExec(time.Hour, 0, func(context.Context) {
				runs++

				if c.Panic {
					panic("boom")
				}
			}, recovery, false)

			if c.Panics {
				assert.Panics(t, func() { pe.exec(context.Background()) })
			} else {
				assert.NotPanics(t, func() { pe.exec(context.Background()) })
			}

			assert.Equal(t, 1, runs)
			assert.Equal(t, c.Recovered, recovered)
		})
	}
}
