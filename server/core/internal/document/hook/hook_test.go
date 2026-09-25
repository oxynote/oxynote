package hook

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/apps/webchange"
	"github.com/oxynote/oxynote/server/core/internal/document/hook/processor"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
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
			BranchID: branchID,
			BlockID:  null.StringFrom("block-1"),
			Settings: reminderSettings(t, time.Now().Add(time.Hour)),
		}, documentID, "org-1", nil)
		require.NoError(t, err)

		assert.False(t, h.ID.IsZero())
		assert.Equal(t, TypeScheduledReminder, h.Type)
		assert.Equal(t, null.ValueFrom(documentID), h.DocumentID)
		assert.Equal(t, null.ValueFrom(branchID), h.BranchID)
		assert.Equal(t, null.StringFrom("org-1"), h.OrganizationID)
		assert.Equal(t, null.StringFrom("block-1"), h.BlockID)
		assert.False(t, h.CreatedAt.IsZero())

		// the reset scored the fresh future schedule at full.
		assert.True(t, h.Score.Equal(decimal.NewFromInt(100)))
		assert.True(t, h.State.Valid)
		assert.Equal(t, processor.StatusActive, h.Status)
	})

	t.Run("Malformed settings fail", func(t *testing.T) {
		t.Parallel()

		_, err := NewHook(context.Background(), CreateInput{
			Type:     TypeScheduledReminder,
			Settings: processor.Settings(`{not json`),
		}, documentID, "org-1", nil)
		assert.Equal(t, ErrInvalidSettings, err)
	})

	t.Run("Invalid settings fail", func(t *testing.T) {
		t.Parallel()

		_, err := NewHook(context.Background(), CreateInput{
			Type:     TypeScheduledReminder,
			Settings: processor.Settings(`{"scale":"bogus","schedule":"2030-01-01T00:00:00Z"}`),
		}, documentID, "org-1", nil)
		assert.Equal(t, processor.ErrInvalidScaleType, err)
	})

	t.Run("Unknown type fails", func(t *testing.T) {
		t.Parallel()

		_, err := NewHook(context.Background(), CreateInput{
			Type:     Type("bogus"),
			Settings: processor.Settings(`{}`),
		}, documentID, "org-1", nil)
		assert.Equal(t, ErrInvalidType, err)
	})

	t.Run("A hook that cannot check its target is refused", func(t *testing.T) {
		t.Parallel()

		_, err := NewHook(context.Background(), CreateInput{
			Type:     TypeURLWatcher,
			Settings: processor.Settings(`{"url":"https://example.com"}`),
		}, documentID, "org-1", NewInput("org-1", nil, webchange.NewClient("", "")))
		require.Error(t, err)
		assert.Equal(t, http.StatusUnprocessableEntity, errutil.StatusCode(err, false))
		assert.EqualError(t, err, "the hook cannot check its target: unconfigured")
	})
}

func Test_Hook_ApplyUpdate(t *testing.T) {
	t.Parallel()

	h, err := NewHook(context.Background(), CreateInput{
		Type:     TypeScheduledReminder,
		Settings: reminderSettings(t, time.Now().Add(time.Hour)),
	}, xid.New(), "org-1", nil)
	require.NoError(t, err)

	// an already-elapsed schedule resets the score straight to zero.
	newSettings := reminderSettings(t, time.Now().Add(time.Second))

	require.NoError(t, h.ApplyUpdate(context.Background(), UpdateInput{Settings: newSettings}, nil))

	assert.Equal(t, newSettings, h.Settings)
	assert.True(t, h.UpdatedAt.Valid)
	assert.True(t, h.Score.Equal(decimal.Zero))

	assert.Equal(t, ErrInvalidSettings, h.ApplyUpdate(context.Background(), UpdateInput{
		Settings: processor.Settings(`{not json`),
	}, nil))
}

func Test_Hook_Process(t *testing.T) {
	t.Parallel()

	t.Run("Elapsed schedule scores zero", func(t *testing.T) {
		t.Parallel()

		h, err := NewHook(context.Background(), CreateInput{
			Type:     TypeScheduledReminder,
			Settings: reminderSettings(t, time.Now().Add(-time.Hour)),
		}, xid.New(), "org-1", nil)
		require.NoError(t, err)

		// backdate the started-at state so the schedule has elapsed.
		state, merr := json.Marshal(processor.ScheduledReminderState{
			StartedAt: time.Now().Add(-2 * time.Hour),
		})
		require.NoError(t, merr)

		h.State = null.ValueFrom(processor.State(state))

		require.NoError(t, h.Process(context.Background(), nil))
		assert.True(t, h.Score.Equal(decimal.Zero))
	})

	t.Run("A hook never set up is reset", func(t *testing.T) {
		t.Parallel()

		h := &Hook{
			Type:     TypeScheduledReminder,
			Settings: reminderSettings(t, time.Now().Add(time.Hour)),
			Status:   processor.StatusActive,
		}

		require.NoError(t, h.Process(context.Background(), nil))
		assert.True(t, h.State.Valid)
		assert.True(t, h.Score.Equal(decimal.NewFromInt(100)))
	})

	t.Run("A run that cannot check keeps score and state", func(t *testing.T) {
		t.Parallel()

		h := &Hook{
			Type:     TypeURLWatcher,
			Settings: processor.Settings(`{"url":"https://example.com"}`),
			State:    null.ValueFrom(processor.State(`{"watcherId":"w1"}`)),
			Score:    decimal.NewFromInt(40),
			Status:   processor.StatusActive,
		}

		require.NoError(t, h.Process(context.Background(), NewInput("org-1", nil, webchange.NewClient("", ""))))
		assert.Equal(t, processor.StatusUnconfigured, h.Status)
		assert.True(t, h.Score.Equal(decimal.NewFromInt(40)))
		assert.Equal(t, processor.State(`{"watcherId":"w1"}`), h.State.V)
	})

	t.Run("Malformed settings fail", func(t *testing.T) {
		t.Parallel()

		h := &Hook{
			Type:     TypeScheduledReminder,
			Settings: processor.Settings(`{not json`),
			State:    null.ValueFrom(processor.State(`{}`)),
		}

		require.Error(t, h.Process(context.Background(), nil))
	})
}

func Test_Hook_Delete(t *testing.T) {
	t.Parallel()

	h, err := NewHook(context.Background(), CreateInput{
		Type:     TypeScheduledReminder,
		Settings: reminderSettings(t, time.Now().Add(time.Hour)),
	}, xid.New(), "org-1", nil)
	require.NoError(t, err)

	// scheduled reminders have no external resources; delete is a no-op.
	assert.NoError(t, h.Delete(context.Background(), nil))

	// a url watcher never set up has no watcher yet, so its teardown
	// reaches nothing outside.
	unset := Hook{
		Type:     TypeURLWatcher,
		Settings: processor.Settings(`{"url":"https://example.com"}`),
	}
	assert.NoError(t, unset.Delete(context.Background(), nil))
}

func Test_Hook_ChangedFrom(t *testing.T) {
	t.Parallel()

	base := Hook{
		Status: processor.StatusActive,
		Score:  decimal.NewFromInt(100),
		State:  null.ValueFrom(processor.State(`{}`)),
	}

	cc := map[string]struct {
		Change  func(*Hook)
		Changed bool
	}{
		"Same hook": {
			Change: func(*Hook) {},
		},
		"Only the state content moved": {
			Change: func(h *Hook) {
				h.State = null.ValueFrom(processor.State(`{"a":1}`))
			},
		},
		"Status": {
			Change:  func(h *Hook) { h.Status = processor.StatusUnconfigured },
			Changed: true,
		},
		"Score": {
			Change:  func(h *Hook) { h.Score = decimal.Zero },
			Changed: true,
		},
		"Set up": {
			Change:  func(h *Hook) { h.State = null.Value[processor.State]{} },
			Changed: true,
		},
		"Soft deletion": {
			Change:  func(h *Hook) { h.SoftDeletedAt = null.TimeFrom(time.Now()) },
			Changed: true,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			h := base
			c.Change(&h)

			assert.Equal(t, c.Changed, h.ChangedFrom(base))
		})
	}
}

func Test_Hook_NewCopy(t *testing.T) {
	t.Parallel()

	src := Hook{
		ID:             xid.New(),
		Type:           TypeURLWatcher,
		DocumentID:     null.ValueFrom(xid.New()),
		OrganizationID: null.StringFrom("org-1"),
		BranchID:       null.ValueFrom(xid.New()),
		BlockID:        null.StringFrom("b1"),
		Settings:       processor.Settings(`{"url":"https://example.com"}`),
		State:          null.ValueFrom(processor.State(`{"watcherId":"w1"}`)),
		Status:         processor.StatusUnreachableURL,
		Score:          decimal.NewFromInt(10),
	}

	documentID, branchID := xid.New(), xid.New()

	cp := src.NewCopy(documentID, branchID, null.StringFrom("b2"))

	assert.NotEqual(t, src.ID, cp.ID)
	assert.Equal(t, src.Type, cp.Type)
	assert.Equal(t, null.ValueFrom(documentID), cp.DocumentID)
	assert.Equal(t, src.OrganizationID, cp.OrganizationID)
	assert.Equal(t, null.ValueFrom(branchID), cp.BranchID)
	assert.Equal(t, null.StringFrom("b2"), cp.BlockID)
	assert.Equal(t, src.Settings, cp.Settings)
	assert.Equal(t, src.Status, cp.Status)
	assert.Equal(t, "10", cp.Score.String())
	assert.False(t, cp.CreatedAt.IsZero())

	// the source's state names its own watcher, which the copy must never
	// reach.
	assert.False(t, cp.State.Valid)
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

func Test_Type_Validate(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Type Type
		Err  error
	}{
		"Scheduled reminder":      {Type: TypeScheduledReminder},
		"Github tracking":         {Type: TypeGithubTracking},
		"URL watcher":             {Type: TypeURLWatcher},
		"Container image watcher": {Type: TypeContainerImageWatcher},
		"Unknown type":            {Type: Type("bogus"), Err: ErrInvalidType},
		"Empty type":              {Err: ErrInvalidType},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			err := c.Type.Validate()
			testutil.AssertEqualError(t, c.Err, err)

			if err == nil {
				return
			}

			assert.Equal(t, http.StatusBadRequest, errutil.StatusCode(err, false))
		})
	}
}
