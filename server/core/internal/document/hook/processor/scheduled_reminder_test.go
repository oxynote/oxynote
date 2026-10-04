package processor

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// reminderState marshals a scheduled-reminder state started at the given time.
func reminderState(t *testing.T, startedAt time.Time) State {
	t.Helper()

	raw, err := json.Marshal(ScheduledReminderState{StartedAt: startedAt})
	require.NoError(t, err)

	return State(raw)
}

func Test_ScheduledReminder_Process(t *testing.T) {
	t.Parallel()

	now := time.Now()

	cc := map[string]struct {
		Reminder ScheduledReminder
		State    State
		Score    decimal.Decimal
		Err      error
	}{
		"Halfway through scores about half": {
			Reminder: ScheduledReminder{
				Scale:    ScaleTypeLinear,
				Schedule: now.Add(time.Hour),
			},
			State: reminderState(t, now.Add(-time.Hour)),
			Score: decimal.NewFromInt(50),
		},
		"Elapsed schedule scores zero": {
			Reminder: ScheduledReminder{
				Scale:    ScaleTypeLinear,
				Schedule: now.Add(-time.Minute),
			},
			State: reminderState(t, now.Add(-time.Hour)),
			Score: decimal.Zero,
		},
		"Fresh schedule scores full": {
			Reminder: ScheduledReminder{
				Scale:    ScaleTypeLinear,
				Schedule: now.Add(1000 * time.Hour),
			},
			State: reminderState(t, now),
			Score: decimal.NewFromInt(100),
		},
		"Malformed state fails": {
			Reminder: ScheduledReminder{
				Scale: ScaleTypeLinear,
			},
			State: State(`{not json`),
			Err:   assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := c.Reminder.Process(context.Background(), stubInput{state: c.State})
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, StatusActive, res.Status)
			assert.True(
				t,
				res.Score.Sub(c.Score).Abs().LessThanOrEqual(decimal.NewFromInt(1)),
				"score %s should be within 1 of %s", res.Score, c.Score,
			)
			assert.JSONEq(t, string(c.State), string(res.State))
		})
	}
}

func Test_ScheduledReminder_Reset(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Schedule time.Time
		Score    decimal.Decimal
	}{
		"Future schedule starts at full score": {
			Schedule: time.Now().Add(time.Hour),
			Score:    decimal.NewFromInt(100),
		},
		"Schedule inside the grace period starts at zero": {
			Schedule: time.Now().Add(time.Second),
			Score:    decimal.Zero,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			sr := ScheduledReminder{
				Scale:    ScaleTypeLinear,
				Schedule: c.Schedule,
			}

			res, err := sr.Reset(context.Background(), stubInput{})
			require.NoError(t, err)

			assert.Equal(t, StatusActive, res.Status)
			assert.True(t, res.Score.Equal(c.Score), "score %s", res.Score)

			var srs ScheduledReminderState

			require.NoError(t, json.Unmarshal(res.State, &srs))
			assert.False(t, srs.StartedAt.IsZero())
		})
	}
}

func Test_ScheduledReminder_Validate(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Reminder ScheduledReminder
		Err      error
	}{
		"Linear scale with a schedule is valid": {
			Reminder: ScheduledReminder{
				Scale:    ScaleTypeLinear,
				Schedule: time.Now().Add(time.Hour),
			},
		},
		"Unknown scale is rejected": {
			Reminder: ScheduledReminder{
				Scale:    ScaleType("exponential"),
				Schedule: time.Now().Add(time.Hour),
			},
			Err: ErrInvalidScaleType,
		},
		"Missing schedule is rejected": {
			Reminder: ScheduledReminder{
				Scale: ScaleTypeLinear,
			},
			Err: ErrMissingSchedule,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.Reminder.Validate())
		})
	}
}
