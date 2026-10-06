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

	cc := map[string]struct {
		Inp   CreateInput
		Input *Input
		Err   error
	}{
		"Malformed settings": {
			Inp: CreateInput{
				Type:     TypeScheduledReminder,
				Settings: processor.Settings(`{not json`),
			},
			Err: ErrInvalidSettings,
		},
		"Invalid settings": {
			Inp: CreateInput{
				Type:     TypeScheduledReminder,
				Settings: processor.Settings(`{"scale":"bogus","schedule":"2030-01-01T00:00:00Z"}`),
			},
			Err: processor.ErrInvalidScaleType,
		},
		"Unknown type": {
			Inp: CreateInput{
				Type:     Type("bogus"),
				Settings: processor.Settings(`{}`),
			},
			Err: ErrInvalidType,
		},
		"Hook that cannot check its target is refused": {
			Inp: CreateInput{
				Type:     TypeURLWatcher,
				Settings: processor.Settings(`{"url":"https://example.com"}`),
			},
			Input: NewInput("org-1", nil, webchange.NewClient("", "")),
			Err:   errutil.New(http.StatusUnprocessableEntity, "document_hook.unconfigured", "the integration the hook needs is not configured"),
		},
		"Successful creation": {
			Inp: CreateInput{
				Type:     TypeScheduledReminder,
				BlockID:  null.StringFrom("block-1"),
				Settings: reminderSettings(t, time.Now().Add(time.Hour)),
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			h, err := NewHook(context.Background(), c.Inp, documentID, branchID, "org-1", c.Input)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.False(t, h.ID.IsZero())
			assert.Equal(t, h.ID, h.CrossBranchID)
			assert.Equal(t, c.Inp.Type, h.Type)
			assert.Equal(t, null.ValueFrom(documentID), h.DocumentID)
			assert.Equal(t, null.ValueFrom(branchID), h.BranchID)
			assert.Equal(t, null.StringFrom("org-1"), h.OrganizationID)
			assert.Equal(t, c.Inp.BlockID, h.BlockID)
			assert.False(t, h.CreatedAt.IsZero())

			// the reset scored the fresh future schedule at full.
			assert.True(t, h.Score.Equal(decimal.NewFromInt(100)))
			assert.True(t, h.State.Valid)
			assert.Equal(t, processor.StatusActive, h.Status)
		})
	}
}

func Test_Hook_NewCopy(t *testing.T) {
	t.Parallel()

	src := Hook{
		ID:             xid.New(),
		CrossBranchID:  xid.New(),
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

	cc := map[string]struct {
		DocumentID       xid.ID
		KeepsCrossBranch bool
	}{
		"Copy within the document keeps the cross-branch ID": {
			DocumentID:       src.DocumentID.V,
			KeepsCrossBranch: true,
		},
		"Copy into another document is a hook of its own": {
			DocumentID: xid.New(),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			branchID := xid.New()

			cp := src.NewCopy(c.DocumentID, branchID, null.StringFrom("b2"))

			assert.NotEqual(t, src.ID, cp.ID)
			assert.Equal(t, src.Type, cp.Type)
			assert.Equal(t, null.ValueFrom(c.DocumentID), cp.DocumentID)
			assert.Equal(t, src.OrganizationID, cp.OrganizationID)
			assert.Equal(t, null.ValueFrom(branchID), cp.BranchID)
			assert.Equal(t, null.StringFrom("b2"), cp.BlockID)
			assert.Equal(t, src.Settings, cp.Settings)
			assert.Equal(t, processor.StatusInitializing, cp.Status)
			assert.Equal(t, "10", cp.Score.String())
			assert.False(t, cp.CreatedAt.IsZero())

			// the source's state names its own watcher, which the copy must
			// never reach.
			assert.False(t, cp.State.Valid)

			if c.KeepsCrossBranch {
				assert.Equal(t, src.CrossBranchID, cp.CrossBranchID)

				return
			}

			assert.Equal(t, cp.ID, cp.CrossBranchID)
		})
	}
}

func Test_Hook_ApplyUpdate(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Settings processor.Settings
		Err      error
	}{
		"Malformed settings": {
			Settings: processor.Settings(`{not json`),
			Err:      ErrInvalidSettings,
		},
		// an already elapsed schedule resets the score straight to zero.
		"Successful update": {
			Settings: reminderSettings(t, time.Now().Add(time.Second)),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			h, err := NewHook(context.Background(), CreateInput{
				Type:     TypeScheduledReminder,
				Settings: reminderSettings(t, time.Now().Add(time.Hour)),
			}, xid.New(), xid.New(), "org-1", nil)
			require.NoError(t, err)

			err = h.ApplyUpdate(context.Background(), UpdateInput{Settings: c.Settings}, nil)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Settings, h.Settings)
			assert.True(t, h.UpdatedAt.Valid)
			assert.True(t, h.Score.Equal(decimal.Zero))
		})
	}
}

func Test_Hook_Process(t *testing.T) {
	t.Parallel()

	// the state started two hours ago, so the schedule has elapsed.
	elapsed, err := json.Marshal(processor.ScheduledReminderState{
		StartedAt: time.Now().Add(-2 * time.Hour),
	})
	require.NoError(t, err)

	cc := map[string]struct {
		Hook  Hook
		Input *Input
		Err   error
		Check func(*testing.T, Hook)
	}{
		"Malformed settings": {
			Hook: Hook{
				Type:     TypeScheduledReminder,
				Settings: processor.Settings(`{not json`),
				State:    null.ValueFrom(processor.State(`{}`)),
			},
			Err: assert.AnError,
		},
		"Elapsed schedule scores zero": {
			Hook: Hook{
				Type:     TypeScheduledReminder,
				Settings: reminderSettings(t, time.Now().Add(-time.Hour)),
				State:    null.ValueFrom(processor.State(elapsed)),
				Score:    decimal.NewFromInt(100),
				Status:   processor.StatusActive,
			},
			Check: func(t *testing.T, h Hook) {
				assert.Equal(t, processor.StatusActive, h.Status)
				assert.True(t, h.Score.Equal(decimal.Zero))
			},
		},
		"Hook never set up is reset": {
			Hook: Hook{
				Type:     TypeScheduledReminder,
				Settings: reminderSettings(t, time.Now().Add(time.Hour)),
				Status:   processor.StatusActive,
			},
			Check: func(t *testing.T, h Hook) {
				assert.True(t, h.State.Valid)
				assert.True(t, h.Score.Equal(decimal.NewFromInt(100)))
			},
		},
		"Run that cannot check keeps score and state": {
			Hook: Hook{
				Type:     TypeURLWatcher,
				Settings: processor.Settings(`{"url":"https://example.com"}`),
				State:    null.ValueFrom(processor.State(`{"watcherId":"w1"}`)),
				Score:    decimal.NewFromInt(40),
				Status:   processor.StatusActive,
			},
			Input: NewInput("org-1", nil, webchange.NewClient("", "")),
			Check: func(t *testing.T, h Hook) {
				assert.Equal(t, processor.StatusUnconfigured, h.Status)
				assert.True(t, h.Score.Equal(decimal.NewFromInt(40)))
				assert.Equal(t, processor.State(`{"watcherId":"w1"}`), h.State.V)
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			h := c.Hook

			err := h.Process(context.Background(), c.Input)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			c.Check(t, h)
		})
	}
}

func Test_Hook_Delete(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Hook Hook
		Err  error
	}{
		"Malformed settings": {
			Hook: Hook{
				Type:     TypeScheduledReminder,
				Settings: processor.Settings(`{not json`),
				State:    null.ValueFrom(processor.State(`{}`)),
			},
			Err: assert.AnError,
		},
		// a url watcher never set up has no watcher yet, so its teardown
		// reaches nothing outside.
		"Hook never set up holds nothing": {
			Hook: Hook{
				Type:     TypeURLWatcher,
				Settings: processor.Settings(`{"url":"https://example.com"}`),
			},
		},
		// scheduled reminders have no external resources.
		"Hook without external resources": {
			Hook: Hook{
				Type:     TypeScheduledReminder,
				Settings: reminderSettings(t, time.Now().Add(time.Hour)),
				State:    null.ValueFrom(processor.State(`{}`)),
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.Hook.Delete(context.Background(), nil))
		})
	}
}

func Test_Hook_Summary(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Hook   Hook
		Result any
		Err    error
	}{
		"Malformed settings": {
			Hook: Hook{
				Type:     TypeGithubTracking,
				Settings: processor.Settings(`{not json`),
			},
			Err: assert.AnError,
		},
		"Malformed state": {
			Hook: Hook{
				Type:     TypeGithubTracking,
				Settings: processor.Settings(`{}`),
				State:    null.ValueFrom(processor.State(`{`)),
			},
			Err: assert.AnError,
		},
		"Hook type without a summary": {
			Hook: Hook{
				Type:     TypeURLWatcher,
				Settings: processor.Settings(`{"url":"https://example.com"}`),
				State:    null.ValueFrom(processor.State(`{}`)),
			},
		},
		"Successful summary": {
			Hook: Hook{
				Type:     TypeGithubTracking,
				Settings: processor.Settings(`{}`),
				State:    null.ValueFrom(processor.State(`{"changedPaths":2}`)),
			},
			Result: processor.GithubTrackingSummary{ChangedPaths: 2},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := c.Hook.Summary()
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)
		})
	}
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

func Test_Hook_ensurePrepared(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Type     Type
		Settings processor.Settings
		Err      error
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
			Type:     TypeScheduledReminder,
			Settings: processor.Settings(`{not json`),
			Err:      assert.AnError,
		},
		"Unknown type fails": {
			Type:     Type("bogus"),
			Settings: processor.Settings(`{}`),
			Err:      assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			h := &Hook{Type: c.Type, Settings: c.Settings}

			err := h.ensurePrepared()
			testutil.AssertEqualError(t, c.Err, err)
			assert.Equal(t, err == nil, h.prepared)

			if err != nil {
				return
			}

			assert.NotNil(t, h.runner)
			assert.NotNil(t, h.runner)

			// preparation is memoized.
			runner := h.runner
			require.NoError(t, h.ensurePrepared())
			assert.Same(t, runner, h.runner)
		})
	}
}

func Test_newStatusError(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Status processor.Status
		Err    error
	}{
		"Unconfigured": {
			Status: processor.StatusUnconfigured,
			Err:    errutil.New(http.StatusUnprocessableEntity, "document_hook.unconfigured", "the integration the hook needs is not configured"),
		},
		"Missing installation": {
			Status: processor.StatusMissingInstallation,
			Err:    errutil.New(http.StatusUnprocessableEntity, "document_hook.missing_installation", "the organization has no github app installation"),
		},
		"Missing repository": {
			Status: processor.StatusMissingRepository,
			Err:    errutil.New(http.StatusUnprocessableEntity, "document_hook.missing_repository", "the github repository was not found"),
		},
		"Missing branch": {
			Status: processor.StatusMissingBranch,
			Err:    errutil.New(http.StatusUnprocessableEntity, "document_hook.missing_branch", "the github branch was not found"),
		},
		"Tree truncated": {
			Status: processor.StatusTreeTruncated,
			Err:    errutil.New(http.StatusUnprocessableEntity, "document_hook.tree_truncated", "the github repository is too large to compare"),
		},
		"Unreachable url": {
			Status: processor.StatusUnreachableURL,
			Err:    errutil.New(http.StatusUnprocessableEntity, "document_hook.unreachable_url", "the url cannot be reached"),
		},
		"Unauthorized": {
			Status: processor.StatusUnauthorized,
			Err:    errutil.New(http.StatusUnprocessableEntity, "document_hook.unauthorized", "the container registry refused access"),
		},
		"Image not found": {
			Status: processor.StatusImageNotFound,
			Err:    errutil.New(http.StatusUnprocessableEntity, "document_hook.image_not_found", "the container image was not found"),
		},
		"Unknown status": {
			Status: processor.Status("bogus"),
			Err:    errutil.New(http.StatusUnprocessableEntity, "document_hook.cannot_check", "the hook cannot check its target"),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, newStatusError(c.Status))
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
