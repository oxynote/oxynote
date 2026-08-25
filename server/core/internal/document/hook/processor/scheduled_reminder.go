package processor

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/mathutil"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
	"github.com/shopspring/decimal"
)

// ErrInvalidScaleType is returned when a scheduled reminder carries a scale
// the processor does not implement.
var ErrInvalidScaleType = errutil.New(http.StatusBadRequest, "document_hook.invalid_scale_type", "invalid scale type")

// ScaleType represents the type of scale used for scoring.
type ScaleType string

const (
	// ScaleTypeLinear represents a linear scale.
	ScaleTypeLinear ScaleType = "linear"
)

// _scheduleGracePeriod is the minimum lead time a schedule must have on
// reset; anything closer scores zero immediately.
const _scheduleGracePeriod = time.Second * 5

// ScheduledReminder specifies a processor that calculates the
// score based on a scheduled time.
type ScheduledReminder struct {
	PlainDeleter

	// Scale is the type of scale used for scoring.
	Scale ScaleType `json:"scale"`

	// Duration is a duration value (not parsed by backend).
	// This is used only by the frontend for display purposes.
	Duration null.String `json:"duration"`

	// Schedule is the concrete time when the score should reach 0.
	Schedule time.Time `json:"schedule"`
}

// Process calculates the scheduled reminder score.
func (sr *ScheduledReminder) Process(_ context.Context, inp Input) (decimal.Decimal, State, error) {
	var srs ScheduledReminderState

	if err := json.Unmarshal(inp.State(), &srs); err != nil {
		return decimal.Zero, nil, fmt.Errorf("unmarshaling scheduled reminder state: %w", err)
	}

	score := mathutil.Hundred

	switch sr.Scale {
	case ScaleTypeLinear:
		totalDuration := sr.Schedule.Sub(srs.StartedAt)
		elapsed := timeutil.Now().Sub(srs.StartedAt)

		if elapsed >= totalDuration {
			score = decimal.Zero
			break
		}

		elapsedPercent := mathutil.SafeDiv(
			decimal.NewFromInt(elapsed.Nanoseconds()),
			decimal.NewFromInt(totalDuration.Nanoseconds()),
		).Mul(mathutil.Hundred)
		score = score.Sub(elapsedPercent).Round(0)
	default:
		return decimal.Zero, nil, ErrInvalidScaleType
	}

	state, err := json.Marshal(srs)
	if err != nil {
		return decimal.Zero, nil, fmt.Errorf("marshaling scheduled reminder state: %w", err)
	}

	return score, state, nil
}

// Reset resets the state of the scheduled reminder processor.
func (sr *ScheduledReminder) Reset(_ context.Context, _ Input) (decimal.Decimal, State, error) {
	// validated here as well as in Process: an unknown scale would otherwise
	// be accepted at creation and then fail on every processing cycle.
	if sr.Scale != ScaleTypeLinear {
		return decimal.Zero, nil, ErrInvalidScaleType
	}

	srs := ScheduledReminderState{
		StartedAt: timeutil.Now(),
	}

	state, err := json.Marshal(srs)
	if err != nil {
		return decimal.Zero, nil, fmt.Errorf("marshaling scheduled reminder state: %w", err)
	}

	score := mathutil.Hundred

	if srs.StartedAt.Add(_scheduleGracePeriod).After(sr.Schedule) {
		score = decimal.Zero
	}

	return score, state, nil
}

// ScheduledReminderState represents the state of
// a scheduled reminder.
type ScheduledReminderState struct {
	// StartedAt is the time when the reminder was started/reset.
	StartedAt time.Time `json:"startedAt"`
}
