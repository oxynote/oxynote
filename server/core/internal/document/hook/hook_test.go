package hook

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/document/hook/processor"
	"github.com/rs/xid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// reminderSettings builds scheduled-reminder settings due at the given time.
func reminderSettings(t *testing.T, schedule time.Time) processor.Settings {
	t.Helper()

	raw, err := json.Marshal(map[string]any{
		"scale":    "linear",
		"schedule": schedule,
	})
	require.NoError(t, err)

	return processor.Settings(raw)
}

func Test_Type_HumanizedString(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Type     Type
		Expected string
	}{
		"Scheduled reminder":      {Type: TypeScheduledReminder, Expected: "Scheduled Reminder"},
		"GitHub tracking":         {Type: TypeGithubTracking, Expected: "GitHub Tracking"},
		"URL watcher":             {Type: TypeURLWatcher, Expected: "Website Changes"},
		"Container image watcher": {Type: TypeContainerImageWatcher, Expected: "Container Image Updates"},
		"Unknown type":            {Type: Type("bogus"), Expected: "Unknown"},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Expected, c.Type.HumanizedString())
		})
	}
}

func Test_NewHook(t *testing.T) {
	t.Parallel()

	documentID, branchID := xid.New(), xid.New()

	t.Run("Valid settings create a reset hook", func(t *testing.T) {
		t.Parallel()

		h, err := NewHook(context.Background(), CreateInput{
			Type:     TypeScheduledReminder,
			BlockID:  null.StringFrom("block-1"),
			Settings: reminderSettings(t, time.Now().Add(time.Hour)),
		}, documentID, branchID, "org-1", nil)
		require.NoError(t, err)

		assert.False(t, h.ID.IsZero())
		assert.Equal(t, TypeScheduledReminder, h.Type)
		assert.Equal(t, documentID, h.DocumentID)
		assert.Equal(t, null.ValueFrom(branchID), h.BranchID)
		assert.Equal(t, "org-1", h.OrganizationID)
		assert.Equal(t, null.StringFrom("block-1"), h.BlockID)
		assert.False(t, h.CreatedAt.IsZero())

		// the reset scored the fresh future schedule at full.
		assert.True(t, h.Score.Equal(decimal.NewFromInt(100)))
		assert.NotEmpty(t, h.State)
	})

	t.Run("Malformed settings fail", func(t *testing.T) {
		t.Parallel()

		_, err := NewHook(context.Background(), CreateInput{
			Type:     TypeScheduledReminder,
			Settings: processor.Settings(`{not json`),
		}, documentID, branchID, "org-1", nil)
		require.Error(t, err)
	})

	t.Run("Unknown type fails", func(t *testing.T) {
		t.Parallel()

		_, err := NewHook(context.Background(), CreateInput{
			Type:     Type("bogus"),
			Settings: processor.Settings(`{}`),
		}, documentID, branchID, "org-1", nil)
		assert.EqualError(t, err, "invalid processor type")
	})
}

func Test_Hook_ApplyUpdate(t *testing.T) {
	t.Parallel()

	h, err := NewHook(context.Background(), CreateInput{
		Type:     TypeScheduledReminder,
		Settings: reminderSettings(t, time.Now().Add(time.Hour)),
	}, xid.New(), xid.New(), "org-1", nil)
	require.NoError(t, err)

	// an already-elapsed schedule resets the score straight to zero.
	newSettings := reminderSettings(t, time.Now().Add(time.Second))

	require.NoError(t, h.ApplyUpdate(context.Background(), UpdateInput{Settings: newSettings}, nil))

	assert.Equal(t, newSettings, h.Settings)
	assert.True(t, h.UpdatedAt.Valid)
	assert.True(t, h.Score.Equal(decimal.Zero))
}

func Test_Hook_Process(t *testing.T) {
	t.Parallel()

	t.Run("Elapsed schedule scores zero", func(t *testing.T) {
		t.Parallel()

		h, err := NewHook(context.Background(), CreateInput{
			Type:     TypeScheduledReminder,
			Settings: reminderSettings(t, time.Now().Add(-time.Hour)),
		}, xid.New(), xid.New(), "org-1", nil)
		require.NoError(t, err)

		// backdate the started-at state so the schedule has elapsed.
		state, merr := json.Marshal(processor.ScheduledReminderState{
			StartedAt: time.Now().Add(-2 * time.Hour),
		})
		require.NoError(t, merr)

		h.State = processor.State(state)

		require.NoError(t, h.Process(context.Background(), nil))
		assert.True(t, h.Score.Equal(decimal.Zero))
	})

	t.Run("Malformed settings fail", func(t *testing.T) {
		t.Parallel()

		h := &Hook{
			Type:     TypeScheduledReminder,
			Settings: processor.Settings(`{not json`),
		}

		require.Error(t, h.Process(context.Background(), nil))
	})
}

func Test_Hook_Delete(t *testing.T) {
	t.Parallel()

	h, err := NewHook(context.Background(), CreateInput{
		Type:     TypeScheduledReminder,
		Settings: reminderSettings(t, time.Now().Add(time.Hour)),
	}, xid.New(), xid.New(), "org-1", nil)
	require.NoError(t, err)

	// scheduled reminders have no external resources; delete is a no-op.
	assert.NoError(t, h.Delete(context.Background(), nil))
}

func Test_Hook_ensurePrepared(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Type      Type
		Settings  processor.Settings
		ExpectErr bool
	}{
		"Scheduled reminder settings prepare": {
			Type:     TypeScheduledReminder,
			Settings: processor.Settings(`{"scale": "linear"}`),
		},
		"Github tracking settings prepare": {
			Type:     TypeGithubTracking,
			Settings: processor.Settings(`{"repository": "repo"}`),
		},
		"URL watcher settings prepare": {
			Type:     TypeURLWatcher,
			Settings: processor.Settings(`{"url": "https://example.com"}`),
		},
		"Container image watcher settings prepare": {
			Type:     TypeContainerImageWatcher,
			Settings: processor.Settings(`{"image": "nginx:latest"}`),
		},
		"Malformed settings fail": {
			Type:      TypeScheduledReminder,
			Settings:  processor.Settings(`{not json`),
			ExpectErr: true,
		},
		"Unknown type fails": {
			Type:      Type("bogus"),
			Settings:  processor.Settings(`{}`),
			ExpectErr: true,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			h := &Hook{Type: c.Type, Settings: c.Settings}

			err := h.ensurePrepared()

			if c.ExpectErr {
				require.Error(t, err)
				assert.False(t, h.prepared)

				return
			}

			require.NoError(t, err)
			assert.True(t, h.prepared)
			assert.NotNil(t, h.runner)

			// preparation is memoized.
			runner := h.runner
			require.NoError(t, h.ensurePrepared())
			assert.Same(t, runner, h.runner)
		})
	}
}
