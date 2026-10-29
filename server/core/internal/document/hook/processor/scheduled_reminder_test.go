package processor

import (
	"context"
	"testing"
	"time"

	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_ScheduledReminder_Process(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Schedule  time.Time
		State     State
		Triggered bool
	}{
		"Future schedule is not triggered": {
			Schedule: time.Now().Add(time.Hour),
			State:    State(`{}`),
		},
		"Passed schedule is triggered": {
			Schedule:  time.Now().Add(-time.Minute),
			State:     State(`{}`),
			Triggered: true,
		},
		"Legacy state is replaced": {
			Schedule: time.Now().Add(time.Hour),
			State:    State(`{"startedAt":"2026-01-01T00:00:00Z"}`),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			sr := ScheduledReminder{Schedule: c.Schedule}

			res, err := sr.Process(context.Background(), stubInput{state: c.State})
			require.NoError(t, err)

			assert.Equal(t, StatusActive, res.Status)
			assert.Equal(t, c.Triggered, res.Triggered)
			assert.JSONEq(t, `{}`, string(res.State))
		})
	}
}

func Test_ScheduledReminder_Reset(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Schedule  time.Time
		Triggered bool
	}{
		"Future schedule starts untriggered": {
			Schedule: time.Now().Add(time.Hour),
		},
		"Schedule inside the grace period starts triggered": {
			Schedule:  time.Now().Add(time.Second),
			Triggered: true,
		},
		"Passed schedule starts triggered": {
			Schedule:  time.Now().Add(-time.Hour),
			Triggered: true,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			sr := ScheduledReminder{Schedule: c.Schedule}

			res, err := sr.Reset(context.Background(), stubInput{})
			require.NoError(t, err)

			assert.Equal(t, StatusActive, res.Status)
			assert.Equal(t, c.Triggered, res.Triggered)
			assert.JSONEq(t, `{}`, string(res.State))
		})
	}
}

func Test_ScheduledReminder_Validate(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Reminder ScheduledReminder
		Err      error
	}{
		"Reminder with a schedule is valid": {
			Reminder: ScheduledReminder{
				Schedule: time.Now().Add(time.Hour),
			},
		},
		"Missing schedule is rejected": {
			Reminder: ScheduledReminder{},
			Err:      ErrMissingSchedule,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.Reminder.Validate())
		})
	}
}
