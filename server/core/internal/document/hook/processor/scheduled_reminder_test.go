package processor

import (
	"context"
	"encoding/json"
	"testing"
	"time"

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

	tests := map[string]struct {
		Reminder      ScheduledReminder
		State         State
		ExpectErr     bool
		ExpectedScore decimal.Decimal
	}{
		"Halfway through scores about half": {
			Reminder: ScheduledReminder{
				Scale:    ScaleTypeLinear,
				Schedule: now.Add(time.Hour),
			},
			State:         reminderState(t, now.Add(-time.Hour)),
			ExpectedScore: decimal.NewFromInt(50),
		},
		"Elapsed schedule scores zero": {
			Reminder: ScheduledReminder{
				Scale:    ScaleTypeLinear,
				Schedule: now.Add(-time.Minute),
			},
			State:         reminderState(t, now.Add(-time.Hour)),
			ExpectedScore: decimal.Zero,
		},
		"Fresh schedule scores full": {
			Reminder: ScheduledReminder{
				Scale:    ScaleTypeLinear,
				Schedule: now.Add(1000 * time.Hour),
			},
			State:         reminderState(t, now),
			ExpectedScore: decimal.NewFromInt(100),
		},
		"Invalid scale fails": {
			Reminder: ScheduledReminder{
				Scale:    ScaleType("exponential"),
				Schedule: now.Add(time.Hour),
			},
			State:     reminderState(t, now),
			ExpectErr: true,
		},
		"Malformed state fails": {
			Reminder: ScheduledReminder{
				Scale: ScaleTypeLinear,
			},
			State:     State(`{not json`),
			ExpectErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			score, state, err := tc.Reminder.Process(context.Background(), stubInput{state: tc.State})

			if tc.ExpectErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.True(
				t,
				score.Sub(tc.ExpectedScore).Abs().LessThanOrEqual(decimal.NewFromInt(1)),
				"score %s should be within 1 of %s", score, tc.ExpectedScore,
			)
			assert.JSONEq(t, string(tc.State), string(state))
		})
	}
}

func Test_ScheduledReminder_Reset(t *testing.T) {
	t.Parallel()

	t.Run("Future schedule starts at full score", func(t *testing.T) {
		t.Parallel()

		sr := ScheduledReminder{
			Scale:    ScaleTypeLinear,
			Schedule: time.Now().Add(time.Hour),
		}

		score, state, err := sr.Reset(context.Background(), stubInput{})
		require.NoError(t, err)

		assert.True(t, score.Equal(decimal.NewFromInt(100)))

		var srs ScheduledReminderState

		require.NoError(t, json.Unmarshal(state, &srs))
		assert.False(t, srs.StartedAt.IsZero())
	})

	t.Run("Schedule inside the grace period starts at zero", func(t *testing.T) {
		t.Parallel()

		sr := ScheduledReminder{
			Scale:    ScaleTypeLinear,
			Schedule: time.Now().Add(time.Second),
		}

		score, _, err := sr.Reset(context.Background(), stubInput{})
		require.NoError(t, err)

		assert.True(t, score.Equal(decimal.Zero))
	})
}
