// Package timeutil implements helper functions for time-related logic
// which extends the functionality of the standard time package.
package timeutil

import (
	"log/slog"
	"time"

	"github.com/oxynote/oxynote/server/core/pkg/logutil"
	"github.com/robfig/cron/v3"
)

// ReadableFormat is a time format that should be used when
// showing time information to the user.
const ReadableFormat = "2006-01-02 15:04 MST"

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
