package processor

import (
	"context"
	"net/http"
	"time"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
)

// ErrMissingSchedule is returned when a scheduled reminder has no schedule.
var ErrMissingSchedule = errutil.New(http.StatusBadRequest, "document_hook.missing_schedule", "schedule is required")

// _scheduleGracePeriod is the minimum lead time a schedule must have on
// reset; anything closer triggers immediately.
const _scheduleGracePeriod = time.Second * 5

// ScheduledReminder specifies a processor that triggers at a scheduled
// time.
type ScheduledReminder struct {
	PlainDeleter
	PlainSummarizer

	// Duration is a duration value (not parsed by backend).
	// This is used only by the frontend for display purposes.
	Duration null.String `json:"duration"`

	// Schedule is the concrete time when the reminder triggers.
	Schedule time.Time `json:"schedule"`
}

// Validate checks the reminder's settings.
func (sr *ScheduledReminder) Validate() error {
	if sr.Schedule.IsZero() {
		return ErrMissingSchedule
	}

	return nil
}

// Process triggers the reminder once its schedule has passed.
func (sr *ScheduledReminder) Process(_ context.Context, _ Input) (Result, error) {
	return active(!timeutil.Now().Before(sr.Schedule), struct{}{})
}

// Reset restarts the reminder. A schedule inside the grace period
// triggers it right away.
func (sr *ScheduledReminder) Reset(_ context.Context, _ Input) (Result, error) {
	return active(timeutil.Now().Add(_scheduleGracePeriod).After(sr.Schedule), struct{}{})
}
