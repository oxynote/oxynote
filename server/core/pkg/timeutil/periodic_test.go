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

	fn := func(context.Context) {}
	recovery := func(any) {}

	pe := NewPeriodicExec(time.Minute, time.Second, fn, recovery, true)
	require.NotNil(t, pe)
	assert.Equal(t, time.Minute, pe.interval)
	assert.Equal(t, time.Second, pe.offset)
	assert.NotNil(t, pe.fn)
	assert.NotNil(t, pe.recovery)
	assert.True(t, pe.immediate)
}

func Test_PeriodicExec_Start(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Immediate bool
		Interval  time.Duration
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
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			var runs int

			stopCh := make(chan struct{})

			go func() {
				defer close(stopCh)

				NewPeriodicExec(c.Interval, 0, func(context.Context) {
					runs++

					if runs >= c.Runs {
						cancel()
					}
				}, nil, c.Immediate).Start(ctx)
			}()

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

	// a panic is handed to the recovery function instead of unwinding.
	var recovered any

	NewPeriodicExec(time.Hour, 0, func(context.Context) {
		panic("boom")
	}, func(v any) {
		recovered = v
	}, false).exec(context.Background())

	assert.Equal(t, "boom", recovered)

	// without a recovery function the panic keeps unwinding.
	assert.Panics(t, func() {
		NewPeriodicExec(time.Hour, 0, func(context.Context) {
			panic("boom")
		}, nil, false).exec(context.Background())
	})

	// a run that does not panic leaves nothing behind.
	var called bool

	NewPeriodicExec(time.Hour, 0, func(context.Context) {
		called = true
	}, func(any) {
		t.Fatal("recovery must not be called")
	}, false).exec(context.Background())

	assert.True(t, called)
}
