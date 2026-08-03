// Package timeutil implements helper functions for time-related logic
// which extends the functionality of the standard time package.
package timeutil

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jellydator/xync"
	"github.com/oxynote/purse/util/logutil"
	"github.com/robfig/cron/v3"
)

// ReadableFormat is a time format that should be used when
// showing time information to the user.
const ReadableFormat = "2006-01-02 15:04 MST"

// ErrInvalidDuration is returned when the duration is of invalid
// format.
var ErrInvalidDuration = errors.New("invalid duration")

// Duration is an extension of the standard time.Duration which adds
// text marshalling.
type Duration struct {
	time.Duration
}

// UnmarshalText parses duration from a string form input.
// A duration string is a possibly signed sequence of decimal numbers,
// each with optional fraction and a unit suffix, such as "300ms",
// "1.5h" or "2h45m".
// Valid time units are "ns", "us" (or "µs"), "ms", "s", "m", "h".
func (d *Duration) UnmarshalText(data []byte) error {
	if len(data) == 0 {
		d.Duration = 0
		return nil
	}

	nd, err := time.ParseDuration(string(data))
	if err != nil {
		return ErrInvalidDuration
	}

	d.Duration = nd

	return nil
}

// MarshalText converts duration to a string form output.
func (d Duration) MarshalText() ([]byte, error) {
	return []byte(d.String()), nil
}

// UnixMilli returns the time corresponding to the given Unix time,
// milliseconds since January 1, 1970 UTC.
// The location of the returned time instance is set to UTC.
func UnixMilli(t int64) time.Time {
	return time.UnixMilli(t).UTC()
}

// Unix returns the time corresponding to the given Unix time,
// seconds since January 1, 1970 UTC.
// The location of the returned time instance is set to UTC.
func Unix(t int64) time.Time {
	return time.Unix(t, 0).UTC()
}

// WithinDuration returns whether the provided time is in the provided
// duration (exclusively).
func WithinDuration(expected, actual time.Time, delta time.Duration) bool {
	dt := expected.Sub(actual)
	return dt > -delta && dt < delta
}

// Format formats the provided time into UTC RFC3339 format.
func Format(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

// Parse converts the provided RFC3339 string value into a time structure
// with time zone set to UTC.
func Parse(val string) (time.Time, error) {
	return time.ParseInLocation(time.RFC3339, val, time.UTC)
}

// Now returns the current time with the location set to UTC.
func Now() time.Time {
	return time.Now().UTC()
}

// NewCron creates a fresh instance of cron.
func NewCron(log *slog.Logger) *cron.Cron {
	return cron.New(
		cron.WithSeconds(),
		cron.WithChain(func(j cron.Job) cron.Job {
			return cron.FuncJob(func() {
				defer logutil.Recover(
					log,
					logutil.NewRecoveryPlan("cannot execute cron job"),
				)

				j.Run()
			})
		}),
	)
}

// PeriodicExec handles repeated execution of a function.
type PeriodicExec struct {
	repeatAfter time.Duration
	cooldown    time.Duration
	fn          func(context.Context)
	stopCh      chan struct{}
	triggerCh   chan struct{}
	supv        *xync.Supervisor
	fastStart   bool
}

// NewPeriodicExec creates a fresh instance of the PeriodicExec type.
// The cooldown parameter determines how long the periodic executer
// should wait after the Trigger method is called. This value cannot be
// greater than the 'repeatAfter' value and if it is, it is automatically
// set to the 'repeatAfter' value.
// The timer that uses the 'repeatAfter' value is (re)started
// after the active timer expires or the Trigger method is called but
// before the cooldown is activated.
// The recoveryFn function is used in case of a panic.
func NewPeriodicExec(
	repeatAfter, cooldown time.Duration,
	execFn func(context.Context),
	recoveryFn func(any),
	fastStart bool,
) *PeriodicExec {
	if cooldown > repeatAfter {
		cooldown = repeatAfter
	}

	return &PeriodicExec{
		repeatAfter: repeatAfter,
		cooldown:    cooldown,
		fn:          execFn,
		stopCh:      make(chan struct{}),
		triggerCh:   make(chan struct{}, 1), // we need to buffer 1 event during cooldown
		supv: xync.NewSupervisor(
			xync.WithSupervisorRecovery(recoveryFn),
		),
		fastStart: fastStart,
	}
}

// Start starts the repeated execution of the function.
// It blocks until Stop is called.
func (p *PeriodicExec) Start() {
	startAfter := p.repeatAfter

	if p.fastStart {
		startAfter = 0
	}

	timer := time.NewTimer(startAfter)
	stop := func() {
		if !timer.Stop() {
			select {
			case <-timer.C:
				// NOCOV: since the timer is created
				// and stopped inside this function, there
				// is no way to stop it from the outside to
				// trigger this case.
			default:
			}
		}
	}

	defer stop()

	for {
		var trg bool

		select {
		case <-p.stopCh:
			return
		case <-timer.C:
		case <-p.triggerCh:
			stop()

			trg = true
		}

		p.supv.Go(func(ctx context.Context) {
			p.fn(ctx)
		})
		timer.Reset(p.repeatAfter)

		if !trg {
			continue
		}

		cooldown := time.NewTimer(p.cooldown)

		select {
		case <-p.stopCh:
			if !cooldown.Stop() {
				select {
				case <-cooldown.C:
					// NOCOV: since the timer is created
					// and stopped inside this function,
					// there is no way to stop it from
					// the outside to trigger this case.
				default:
				}
			}

			return
		case <-cooldown.C:
		}
	}
}

// Stop stops the repeated execution of the function.
// It blocks until Start exits.
func (p *PeriodicExec) Stop() {
	p.stopCh <- struct{}{}
	p.supv.StopAndWait()
}

// Trigger forces the function to be executed even if the configured
// duration has not passed yet since the last execution.
func (p *PeriodicExec) Trigger() {
	select {
	case p.triggerCh <- struct{}{}:
	default:
	}
}

// Sleep is a convenience function which either sleeps for a given duration
// or returns early due to cancelation of a context. The boolean value
// specifies if the context was canceled.
func Sleep(ctx context.Context, dur time.Duration) bool {
	tc := time.NewTimer(dur)
	defer tc.Stop()

	select {
	case <-ctx.Done():
		return true
	case <-tc.C:
		return false
	}
}
