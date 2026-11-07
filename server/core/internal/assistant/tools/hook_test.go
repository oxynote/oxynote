package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/document/hook"
	"github.com/oxynote/oxynote/server/core/internal/document/hook/processor"
	"github.com/oxynote/oxynote/server/core/internal/org"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// hookDeps builds session wiring over the given DB. A nil man keeps the
// stub hook manager testDeps wires.
func hookDeps(db *DBMock, man *HookManagerMock) *Deps {
	d := testDeps(db, nil, nil)

	if man != nil {
		d.hookMan = man
	}

	return d
}

// stubHookManager accepts every hook write. A write answers with
// stubHook changed as the call asks, and an update or reset stamps
// UpdatedAt so a test can tell the answer from the hook it handed in.
func stubHookManager() *HookManagerMock {
	return &HookManagerMock{
		CreateHookFunc: func(_ context.Context, ci hook.CreateInput, documentID xid.ID, organizationID, _ string) (*hook.Hook, error) {
			hk := stubHook()
			hk.Type = ci.Type
			hk.DocumentID = null.ValueFrom(documentID)
			hk.BranchID = null.ValueFrom(ci.BranchID)
			hk.OrganizationID = null.StringFrom(organizationID)
			hk.BlockID = ci.BlockID
			hk.Settings = ci.Settings

			return hk, nil
		},
		UpdateHookFunc: func(_ context.Context, id, _ xid.ID, _ string, ui hook.UpdateInput, _ string) (*hook.Hook, error) {
			hk := stubHook()
			hk.ID = id
			hk.Settings = ui.Settings
			hk.UpdatedAt = null.TimeFrom(_stubScheduleTime)

			return hk, nil
		},
		ResetHookFunc: func(_ context.Context, id, _ xid.ID, _ string) (*hook.Hook, error) {
			hk := stubHook()
			hk.ID = id
			hk.UpdatedAt = null.TimeFrom(_stubScheduleTime)

			return hk, nil
		},
	}
}

// stubHookDB answers every document and hook lookup: the branch content
// holds the stub paragraph, the draft branch carries the test hook, and
// every hook write is accepted.
func stubHookDB() *DBMock {
	db := stubContentDB(nil)
	db.FetchDocumentHooksByBranchIDFunc = func(_ context.Context, branchID xid.ID, _ string) ([]hook.Hook, error) {
		if branchID != _stubBranchID {
			return []hook.Hook{}, nil
		}

		return []hook.Hook{*stubHook()}, nil
	}

	return db
}

// stubHookInput answers document lookups as stubDocumentInput does and
// every hook lookup with the test hook, on the draft branch. It accepts
// every hook write.
func stubHookInput() *InputMock {
	inp := stubDocumentInput()
	inp.FetchHooksFunc = func(xid.ID, xid.ID) ([]hook.Hook, error) {
		return []hook.Hook{*stubHook()}, nil
	}
	inp.FetchHookFunc = func(xid.ID, xid.ID) (*hook.Hook, error) {
		return stubHook(), nil
	}
	inp.CreateHookFunc = func(xid.ID, xid.ID, string, hook.Type, processor.Settings) (*hook.Hook, error) {
		return stubHook(), nil
	}

	return inp
}

// _stubScheduleTime is _stubSchedule as the settings carry it.
var _stubScheduleTime = time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)

func Test_listHooks_Info(t *testing.T) {
	t.Parallel()

	info := listHooks{}.Info()

	assert.Equal(t, Traits{Group: GroupHooks, Role: org.RoleViewer}, info.Traits)

	assert.Equal(t, NameListHooks, info.Name)
	assert.NotEmpty(t, info.Description)
	assert.Equal(t, []string{"document_id", "branch_id"}, info.Required)
	assert.Contains(t, info.Properties, "branch_id")
}

func Test_listHooks_Exec(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Input  *InputMock
		Result any
		Err    error
	}{
		"Error returned by Input.FetchHooks": {
			Input: &InputMock{
				FetchHooksFunc: func(xid.ID, xid.ID) ([]hook.Hook, error) { return nil, assert.AnError },
			},
			Err: assert.AnError,
		},
		"Branch with a hook": {
			Input:  stubHookInput(),
			Result: listHooksResult{Hooks: []hookRow{newHookRow(stubHook())}},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := listHooks{DocumentID: _testDocID, BranchID: _stubBranchID}.Exec(c.Input)
			testutil.AssertEqualError(t, c.Err, err)

			ff := c.Input.FetchHooksCalls()
			require.Len(t, ff, 1)
			assert.Equal(t, _stubBranchID, ff[0].BranchID)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_newHookRow(t *testing.T) {
	t.Parallel()

	hk := stubHook()
	hk.BlockID = null.StringFrom("b")
	hk.UpdatedAt = null.TimeFrom(_stubScheduleTime)

	assert.Equal(t, hookRow{
		ID:        _testHookID,
		Type:      hook.TypeScheduledReminder,
		BlockUID:  null.StringFrom("b"),
		Settings:  json.RawMessage(hk.Settings),
		Triggered: hk.Triggered,
		State:     json.RawMessage(hk.State.V),
		Status:    processor.StatusActive,
		CreatedAt: hk.CreatedAt,
		UpdatedAt: null.TimeFrom(_stubScheduleTime),
	}, newHookRow(hk))
}

func Test_createHook_Info(t *testing.T) {
	t.Parallel()

	info := createHook{}.Info()

	assert.Equal(t, Traits{Write: true, Group: GroupHooks, Role: org.RoleEditor}, info.Traits)

	assert.Equal(t, NameCreateHook, info.Name)
	assert.NotEmpty(t, info.Description)
	assert.Equal(t, []string{"document_id", "branch_id", "settings"}, info.Required)

	for _, key := range []string{"document_id", "branch_id", "block_uid", "settings"} {
		assert.Contains(t, info.Properties, key)
	}

	// the type lives inside settings, where each variant pins it.
	assert.NotContains(t, info.Properties, "type")
	assert.Equal(t, hookSettingsSchema(), info.Properties["settings"])

	// the two types that need an integration say so, since a deployment
	// may lack it.
	assert.Contains(t, info.Description, "integrations")
}

func Test_createHook_Validate(t *testing.T) {
	t.Parallel()

	scheduled := json.RawMessage(`{"type":"scheduled-reminder","schedule":"` + _stubSchedule + `"}`)

	ok := createHook{
		DocumentID: _testDocID, BranchID: _stubMainBranchID,
		Settings: scheduled,
	}

	cc := map[string]struct {
		Tool createHook
		Err  error
	}{
		"Document id is required": {
			Tool: createHook{
				docTarget: docTarget{BranchID: _stubMainBranchID},
				Settings:  scheduled,
			},
			Err: errRequired("document_id"),
		},
		"Branch id is required": {
			Tool: createHook{
				docTarget: docTarget{DocumentID: _testDocID},
				Settings:  scheduled,
			},
			Err: errRequired("branch_id"),
		},
		"Settings type is required": {
			Tool: createHook{
				docTarget: ok.docTarget,
				Settings:  json.RawMessage(`{"schedule":"` + _stubSchedule + `"}`),
			},
			Err: errRequired("settings.type"),
		},
		"Schedule is required": {
			Tool: createHook{
				docTarget: ok.docTarget,
				Settings:  json.RawMessage(`{"type":"scheduled-reminder"}`),
			},
			Err: errRequired("schedule"),
		},
		// an unknown type is refused before its settings are looked at.
		"Unknown type": {
			Tool: createHook{docTarget: ok.docTarget, Settings: json.RawMessage(`{"type":"bogus"}`)},
			Err:  fmt.Errorf("type %q: %w", "bogus", hook.ErrInvalidType),
		},
		// another type's field is refused like any other unknown one.
		"Field of another type is refused": {
			Tool: createHook{docTarget: ok.docTarget, Settings: json.RawMessage(`{"type":"url-watcher","url":"https://example.com","image":"x"}`)},
			Err:  fmt.Errorf("%s is not a %s setting", "image", hook.TypeURLWatcher),
		},
		"Valid arguments": {Tool: ok},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.Tool.Validate())
		})
	}
}

func Test_createHook_Summary(t *testing.T) {
	t.Parallel()

	scheduled := json.RawMessage(`{"type":"scheduled-reminder","schedule":"` + _stubSchedule + `"}`)

	cc := map[string]struct {
		Input  *InputMock
		Tool   createHook
		Result string
		Err    error
	}{
		"Hook on the document": {
			Input:  stubHookInput(),
			Tool:   createHook{docTarget: docTarget{DocumentID: _testDocID, BranchID: _stubMainBranchID}, Settings: scheduled},
			Result: "Add a Scheduled Reminder hook on Runbook",
		},
		"Hook on a block": {
			Input:  stubHookInput(),
			Tool:   createHook{docTarget: docTarget{DocumentID: _testDocID, BranchID: _stubBranchID}, BlockUID: "b", Settings: scheduled},
			Result: "Add a Scheduled Reminder hook on Runbook on branch draft, block b",
		},
		"Settings without a type": {
			Input: stubHookInput(),
			Tool:  createHook{docTarget: docTarget{DocumentID: _testDocID, BranchID: _stubMainBranchID}, Settings: json.RawMessage(`{}`)},
			Err:   errRequired("settings.type"),
		},
		"Error returned by Input.FetchBranch": {
			Input: &InputMock{
				FetchBranchFunc: func(xid.ID, xid.ID) (*document.Document, error) { return nil, assert.AnError },
			},
			Tool: createHook{docTarget: docTarget{DocumentID: _testDocID, BranchID: _stubMainBranchID}, Settings: scheduled},
			Err:  assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			got, err := c.Tool.Summary(c.Input)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, c.Input.FetchBranchCalls(), 1)

			if err != nil {
				return
			}

			assert.Equal(t, ActionSummary{
				Tool:         NameCreateHook,
				DocumentID:   _testDocID,
				DocumentName: _stubDocumentName,
				Summary:      c.Result,
			}, got)
		})
	}
}

func Test_createHook_Exec(t *testing.T) {
	t.Parallel()

	scheduled := json.RawMessage(`{"type":"scheduled-reminder","schedule":"` + _stubSchedule + `"}`)
	target := docTarget{DocumentID: _testDocID, BranchID: _stubBranchID}

	cc := map[string]struct {
		Input   *InputMock
		Tool    createHook
		Creates int
		Err     error
	}{
		"Settings without a type": {
			Input: stubHookInput(),
			Tool:  createHook{docTarget: target, Settings: json.RawMessage(`{}`)},
			Err:   errRequired("settings.type"),
		},
		"Error returned by Input.CreateHook": {
			Input: &InputMock{
				CreateHookFunc: func(xid.ID, xid.ID, string, hook.Type, processor.Settings) (*hook.Hook, error) {
					return nil, assert.AnError
				},
			},
			Tool:    createHook{docTarget: target, Settings: scheduled},
			Creates: 1,
			Err:     assert.AnError,
		},
		"Created on a block": {
			Input:   stubHookInput(),
			Tool:    createHook{docTarget: target, BlockUID: _stubContentUID, Settings: scheduled},
			Creates: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := c.Tool.Exec(c.Input)
			testutil.AssertEqualError(t, c.Err, err)

			ff := c.Input.CreateHookCalls()
			require.Len(t, ff, c.Creates)

			if err != nil {
				return
			}

			// the settings are normalised by the hook type's own struct.
			assert.Equal(t, _testDocID, ff[0].DocumentID)
			assert.Equal(t, _stubBranchID, ff[0].BranchID)
			assert.Equal(t, _stubContentUID, ff[0].BlockUID)
			assert.Equal(t, hook.TypeScheduledReminder, ff[0].Tp)
			assert.JSONEq(t, `{"duration":"custom","schedule":"`+_stubSchedule+`"}`, string(ff[0].Settings))
			assert.Equal(t, newHookRow(stubHook()), res)
		})
	}
}

func Test_updateHook_Info(t *testing.T) {
	t.Parallel()

	info := updateHook{}.Info()

	assert.Equal(t, Traits{Write: true, Overwrites: true, Group: GroupHooks, Role: org.RoleEditor}, info.Traits)

	assert.Equal(t, NameUpdateHook, info.Name)
	assert.NotEmpty(t, info.Description)
	assert.Equal(t, []string{"document_id", "hook_id"}, info.Required)

	for _, key := range []string{"document_id", "hook_id", "settings"} {
		assert.Contains(t, info.Properties, key)
	}

	// a hook keeps its type, and the description says how to change it.
	assert.Contains(t, info.Description, "delete the hook and create another")
}

func Test_updateHook_Validate(t *testing.T) {
	t.Parallel()

	settings := json.RawMessage(`{"type":"url-watcher","url":"https://example.com"}`)
	ref := hookRef{DocumentID: _testDocID, HookID: _testHookID}

	// the settings are checked against the hook's own type later, so any
	// type's complete settings pass here, and none at all is a reset.
	cc := map[string]struct {
		Tool updateHook
		Err  error
	}{
		"Document id is required": {
			Tool: updateHook{hookRef: hookRef{HookID: _testHookID}, Settings: settings},
			Err:  errRequired("document_id"),
		},
		"Hook id is required": {
			Tool: updateHook{hookRef: hookRef{DocumentID: _testDocID}, Settings: settings},
			Err:  errRequired("hook_id"),
		},
		"URL is required": {
			Tool: updateHook{hookRef: ref, Settings: json.RawMessage(`{"type":"url-watcher"}`)},
			Err:  errRequired("url"),
		},
		"Complete settings of any type": {Tool: updateHook{hookRef: ref, Settings: settings}},
		"Absent settings":               {Tool: updateHook{hookRef: ref}},
		"Null settings":                 {Tool: updateHook{hookRef: ref, Settings: json.RawMessage(` null `)}},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.Tool.Validate())
		})
	}
}

func Test_updateHook_Summary(t *testing.T) {
	t.Parallel()

	ref := hookRef{DocumentID: _testDocID, HookID: _testHookID}

	cc := map[string]struct {
		Input  *InputMock
		Tool   updateHook
		Result string
		Err    error
	}{
		"New settings": {
			Input:  stubHookInput(),
			Tool:   updateHook{hookRef: ref, Settings: json.RawMessage(`{"type":"scheduled-reminder","schedule":"` + _stubSchedule + `"}`)},
			Result: "Update the Scheduled Reminder hook on Runbook on branch draft",
		},
		"A reset": {
			Input:  stubHookInput(),
			Tool:   updateHook{hookRef: ref},
			Result: "Reset the Scheduled Reminder hook on Runbook on branch draft",
		},
		// the user is not asked to approve a call that cannot run.
		"Settings of another type": {
			Input: stubHookInput(),
			Tool:  updateHook{hookRef: ref, Settings: json.RawMessage(`{"type":"url-watcher","url":"https://example.com"}`)},
			Err:   errors.New("the hook is a scheduled-reminder, not a url-watcher; to change the type, delete it and create another"),
		},
		"Error returned by Input.FetchHook": {
			Input: &InputMock{
				FetchHookFunc: func(xid.ID, xid.ID) (*hook.Hook, error) { return nil, assert.AnError },
			},
			Tool: updateHook{hookRef: ref},
			Err:  assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			got, err := c.Tool.Summary(c.Input)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, c.Input.FetchHookCalls(), 1)

			if err != nil {
				return
			}

			assert.Equal(t, ActionSummary{
				Tool:         NameUpdateHook,
				DocumentID:   _testDocID,
				DocumentName: _stubDocumentName,
				Summary:      c.Result,
			}, got)
		})
	}
}

func Test_updateHook_Exec(t *testing.T) {
	t.Parallel()

	ref := hookRef{DocumentID: _testDocID, HookID: _testHookID}
	later := json.RawMessage(`{"type":"scheduled-reminder","schedule":"2031-01-01T00:00:00Z"}`)

	withInput := func(edit func(inp *InputMock)) *InputMock {
		inp := stubHookInput()
		edit(inp)

		return inp
	}

	cc := map[string]struct {
		Input   *InputMock
		Tool    updateHook
		Updates int
		Resets  int
		Err     error
	}{
		"Error returned by Input.FetchHook": {
			Input: withInput(func(inp *InputMock) {
				inp.FetchHookFunc = func(xid.ID, xid.ID) (*hook.Hook, error) { return nil, assert.AnError }
			}),
			Tool: updateHook{hookRef: ref},
			Err:  assert.AnError,
		},
		// a hook keeps its type for life.
		"Settings of another type are refused": {
			Input: stubHookInput(),
			Tool:  updateHook{hookRef: ref, Settings: json.RawMessage(`{"type":"url-watcher","url":"https://example.com"}`)},
			Err:   errors.New("the hook is a scheduled-reminder, not a url-watcher; to change the type, delete it and create another"),
		},
		"Error returned by Input.UpdateHook": {
			Input: withInput(func(inp *InputMock) {
				inp.UpdateHookFunc = func(*hook.Hook, processor.Settings) error { return assert.AnError }
			}),
			Tool:    updateHook{hookRef: ref, Settings: later},
			Updates: 1,
			Err:     assert.AnError,
		},
		"Error returned by Input.ResetHook": {
			Input: withInput(func(inp *InputMock) {
				inp.ResetHookFunc = func(*hook.Hook) error { return assert.AnError }
			}),
			Tool:   updateHook{hookRef: ref},
			Resets: 1,
			Err:    assert.AnError,
		},
		"Updated": {
			Input:   stubHookInput(),
			Tool:    updateHook{hookRef: ref, Settings: later},
			Updates: 1,
		},
		"Reset keeping the settings": {
			Input:  stubHookInput(),
			Tool:   updateHook{hookRef: ref},
			Resets: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := c.Tool.Exec(c.Input)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, c.Input.FetchHookCalls(), 1)
			assert.Len(t, c.Input.UpdateHookCalls(), c.Updates)
			assert.Len(t, c.Input.ResetHookCalls(), c.Resets)

			if err != nil {
				return
			}

			if c.Updates == 1 {
				assert.JSONEq(t, `{"duration":"custom","schedule":"2031-01-01T00:00:00Z"}`, string(c.Input.UpdateHookCalls()[0].Settings))
			}

			assert.Equal(t, newHookRow(stubHook()), res)
		})
	}
}

func Test_updateHook_replacesSettings(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Settings json.RawMessage
		Result   bool
	}{
		"Absent settings": {},
		"Null settings":   {Settings: json.RawMessage("null")},
		"New settings":    {Settings: json.RawMessage(`{"type":"url-watcher"}`), Result: true},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, updateHook{Settings: c.Settings}.replacesSettings())
		})
	}
}

func Test_updateHook_settingsFor(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Settings string
		Result   string
		Err      error
	}{
		"Invalid settings": {
			Settings: `{}`,
			Err:      errRequired("settings.type"),
		},
		"Settings of another type": {
			Settings: `{"type":"url-watcher","url":"https://example.com"}`,
			Err:      errors.New("the hook is a scheduled-reminder, not a url-watcher; to change the type, delete it and create another"),
		},
		"Settings of the hook type": {
			Settings: `{"type":"scheduled-reminder","schedule":"` + _stubSchedule + `"}`,
			Result:   `{"duration":"custom","schedule":"` + _stubSchedule + `"}`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			got, err := updateHook{Settings: json.RawMessage(c.Settings)}.settingsFor(stubHook())
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.JSONEq(t, c.Result, string(got))
		})
	}
}

func Test_deleteHook_Info(t *testing.T) {
	t.Parallel()

	info := deleteHook{}.Info()

	assert.Equal(t, Traits{Write: true, Destructive: true, Group: GroupHooks, Role: org.RoleEditor}, info.Traits)

	assert.Equal(t, NameDeleteHook, info.Name)
	assert.NotEmpty(t, info.Description)
	assert.Equal(t, []string{"document_id", "hook_id"}, info.Required)
	assert.Contains(t, info.Properties, "hook_id")
}

func Test_deleteHook_Summary(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Input  *InputMock
		Result string
		Err    error
	}{
		"Error returned by Input.FetchHook": {
			Input: &InputMock{
				FetchHookFunc: func(xid.ID, xid.ID) (*hook.Hook, error) { return nil, assert.AnError },
			},
			Err: assert.AnError,
		},
		"Known hook": {
			Input:  stubHookInput(),
			Result: "Delete the Scheduled Reminder hook on Runbook on branch draft",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			got, err := deleteHook{DocumentID: _testDocID, HookID: _testHookID}.Summary(c.Input)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, c.Input.FetchHookCalls(), 1)

			if err != nil {
				return
			}

			assert.Equal(t, ActionSummary{
				Tool:         NameDeleteHook,
				DocumentID:   _testDocID,
				DocumentName: _stubDocumentName,
				Summary:      c.Result,
			}, got)
		})
	}
}

func Test_deleteHook_Exec(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Input   *InputMock
		Deletes int
		Result  any
		Err     error
	}{
		"Error returned by Input.FetchHook": {
			Input: &InputMock{
				FetchHookFunc: func(xid.ID, xid.ID) (*hook.Hook, error) { return nil, assert.AnError },
			},
			Err: assert.AnError,
		},
		"Error returned by Input.DeleteHook": {
			Input: func() *InputMock {
				inp := stubHookInput()
				inp.DeleteHookFunc = func(*hook.Hook) error { return assert.AnError }

				return inp
			}(),
			Deletes: 1,
			Err:     assert.AnError,
		},
		"Deleted": {
			Input:   stubHookInput(),
			Deletes: 1,
			Result:  deleteHookResult{HookID: _testHookID, Deleted: true},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := deleteHook{DocumentID: _testDocID, HookID: _testHookID}.Exec(c.Input)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, c.Input.FetchHookCalls(), 1)
			assert.Len(t, c.Input.DeleteHookCalls(), c.Deletes)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_hookRef_Validate(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Ref hookRef
		Err error
	}{
		"Document id is required": {Ref: hookRef{HookID: _testHookID}, Err: errRequired("document_id")},
		"Hook id is required":     {Ref: hookRef{DocumentID: _testDocID}, Err: errRequired("hook_id")},
		"Valid arguments":         {Ref: hookRef{DocumentID: _testDocID, HookID: _testHookID}},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.Ref.Validate())
		})
	}
}

func Test_hookRef_fetchHookAndBranch(t *testing.T) {
	t.Parallel()

	withInput := func(edit func(inp *InputMock)) *InputMock {
		inp := stubHookInput()
		edit(inp)

		return inp
	}

	cc := map[string]struct {
		Input           *InputMock
		BranchFetches   int
		DocumentFetches int
		Branch          string
		Err             error
	}{
		"Error returned by Input.FetchHook": {
			Input: withInput(func(inp *InputMock) {
				inp.FetchHookFunc = func(xid.ID, xid.ID) (*hook.Hook, error) { return nil, assert.AnError }
			}),
			Err: assert.AnError,
		},
		"Error returned by Input.FetchBranch": {
			Input: withInput(func(inp *InputMock) {
				inp.FetchBranchFunc = func(xid.ID, xid.ID) (*document.Document, error) { return nil, assert.AnError }
			}),
			BranchFetches: 1,
			Err:           assert.AnError,
		},
		"Hook on a branch": {
			Input:         stubHookInput(),
			BranchFetches: 1,
			Branch:        _stubBranchName,
		},
		// a hook whose branch is gone is named on the default branch,
		// which the sweep will remove it from regardless.
		"Hook whose branch is gone": {
			Input: withInput(func(inp *InputMock) {
				inp.FetchHookFunc = func(xid.ID, xid.ID) (*hook.Hook, error) {
					hk := stubHook()
					hk.BranchID = null.Value[xid.ID]{}

					return hk, nil
				}
			}),
			DocumentFetches: 1,
			Branch:          document.DefaultBranch,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			doc, hk, err := hookRef{DocumentID: _testDocID, HookID: _testHookID}.fetchHookAndBranch(c.Input)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, c.Input.FetchHookCalls(), 1)
			assert.Len(t, c.Input.FetchBranchCalls(), c.BranchFetches)
			assert.Len(t, c.Input.FetchDocumentCalls(), c.DocumentFetches)

			if err != nil {
				assert.Nil(t, doc)
				assert.Nil(t, hk)

				return
			}

			assert.Equal(t, c.Branch, doc.BranchName)
			assert.Equal(t, _testHookID, hk.ID)
		})
	}
}

func Test_hookLabel(t *testing.T) {
	t.Parallel()

	doc := stubBranchDocument(_testDocID, "org", _stubBranchID, _stubBranchName)

	cc := map[string]struct {
		BlockUID null.String
		Result   string
	}{
		"Hook on the document": {Result: "Website Changes hook on Runbook on branch draft"},
		"Hook on a block":      {BlockUID: null.StringFrom("b"), Result: "Website Changes hook on Runbook on branch draft, block b"},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, hookLabel(hook.TypeURLWatcher, c.BlockUID, doc))
		})
	}
}

func Test_hookSettingsSchema(t *testing.T) {
	t.Parallel()

	got := hookSettingsSchema()

	variants, ok := got["anyOf"].([]map[string]any)
	require.True(t, ok)

	// one variant per hook type, in the order the domain lists them,
	// each pinning its type as a constant so the settings cannot name
	// one type and take another's fields.
	want := []hook.Type{hook.TypeScheduledReminder, hook.TypeGithubTracking, hook.TypeURLWatcher, hook.TypeContainerImageWatcher}
	require.Len(t, variants, len(want))

	seen := map[string]bool{}

	for i, v := range variants {
		tp := want[i]
		require.NoError(t, tp.Validate())

		assert.Equal(t, false, v["additionalProperties"])

		props, pok := v["properties"].(map[string]any)
		require.True(t, pok)
		assert.Equal(t, map[string]any{"const": string(tp)}, props["type"])

		// every field of the variant is required, and belongs to the
		// type the variant names.
		required, rok := v["required"].([]string)
		require.True(t, rok)
		assert.Len(t, props, len(required))

		for _, key := range required {
			assert.Contains(t, props, key)

			if key == "type" {
				continue
			}

			seen[key] = true
		}
	}

	// every setting of every type is published, so a client can fill
	// any of them without the prose.
	for _, key := range []string{"schedule", "repository", "branch", "paths", "url", "image"} {
		assert.True(t, seen[key], key)
	}
}

func Test_decodeHookSettings(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Raw    string
		Type   hook.Type
		Result string
		Err    error
	}{
		"Type is required":   {Raw: `{"url":"https://example.com"}`, Err: errRequired("settings.type")},
		"Absent settings":    {Err: errRequired("settings.type")},
		"Unknown type":       {Raw: `{"type":"bogus"}`, Err: fmt.Errorf("type %q: %w", "bogus", hook.ErrInvalidType)},
		"Malformed settings": {Raw: `{`, Err: assert.AnError},
		"Malformed type":     {Raw: `{"type":1}`, Err: assert.AnError},
		// the editor labels a reminder with a custom duration as a date,
		// which is what a tool call sets.
		"Scheduled reminder": {
			Raw:    `{"type":"scheduled-reminder","schedule":"2030-01-01T00:00:00Z"}`,
			Type:   hook.TypeScheduledReminder,
			Result: `{"duration":"custom","schedule":"2030-01-01T00:00:00Z"}`,
		},
		"Scheduled reminder without a schedule": {
			Raw: `{"type":"scheduled-reminder"}`,
			Err: errRequired("schedule"),
		},
		"GitHub tracking": {
			Raw:    `{"type":"github-tracking","repository":"r","branch":"main","paths":["a","b"]}`,
			Type:   hook.TypeGithubTracking,
			Result: `{"repository":"r","branch":"main","paths":["a","b"]}`,
		},
		"GitHub tracking without a repository": {
			Raw: `{"type":"github-tracking","branch":"main","paths":["a"]}`,
			Err: errRequired("repository"),
		},
		"GitHub tracking without a branch": {
			Raw: `{"type":"github-tracking","repository":"r","paths":["a"]}`,
			Err: errRequired("branch"),
		},
		"GitHub tracking without paths": {
			Raw: `{"type":"github-tracking","repository":"r","branch":"main","paths":[]}`,
			Err: errRequired("paths"),
		},
		"URL watcher": {
			Raw:    `{"type":"url-watcher","url":"https://example.com"}`,
			Type:   hook.TypeURLWatcher,
			Result: `{"url":"https://example.com"}`,
		},
		"URL watcher without a url": {
			Raw: `{"type":"url-watcher"}`,
			Err: errRequired("url"),
		},
		"Container image watcher": {
			Raw:    `{"type":"container-image-watcher","image":"nginx:1"}`,
			Type:   hook.TypeContainerImageWatcher,
			Result: `{"image":"nginx:1"}`,
		},
		"Container image watcher without an image": {
			Raw: `{"type":"container-image-watcher"}`,
			Err: errRequired("image"),
		},
		// another type's field is refused like any other unknown one.
		"Scheduled reminder with a url": {
			Raw: `{"type":"scheduled-reminder","schedule":"2030-01-01T00:00:00Z","url":"https://example.com"}`,
			Err: fmt.Errorf("%s is not a %s setting", "url", hook.TypeScheduledReminder),
		},
		"Scheduled reminder with a scale": {
			Raw: `{"type":"scheduled-reminder","schedule":"2030-01-01T00:00:00Z","scale":"linear"}`,
			Err: fmt.Errorf("%s is not a %s setting", "scale", hook.TypeScheduledReminder),
		},
		"URL watcher with github paths": {
			Raw: `{"type":"url-watcher","url":"https://example.com","paths":["a"]}`,
			Err: fmt.Errorf("%s is not a %s setting", "paths", hook.TypeURLWatcher),
		},
		"Container image watcher with a schedule": {
			Raw: `{"type":"container-image-watcher","image":"nginx:1","schedule":"2030-01-01T00:00:00Z"}`,
			Err: fmt.Errorf("%s is not a %s setting", "schedule", hook.TypeContainerImageWatcher),
		},
		"Setting of no type": {
			Raw: `{"type":"url-watcher","url":"https://example.com","colour":"red"}`,
			Err: fmt.Errorf("%s is not a %s setting", "colour", hook.TypeURLWatcher),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			tp, got, err := decodeHookSettings(json.RawMessage(c.Raw))
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				assert.Empty(t, tp)
				assert.Nil(t, got)

				return
			}

			assert.Equal(t, c.Type, tp)
			assert.JSONEq(t, c.Result, string(got))
		})
	}
}

func Test_decodeSettings(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Raw    string
		Result processor.URLWatcher
		Err    error
	}{
		"Successful decode": {
			Raw:    `{"url":"https://example.com"}`,
			Result: processor.URLWatcher{URL: "https://example.com"},
		},
		"Type beside the settings": {
			Raw:    `{"type":"url-watcher","url":"https://example.com"}`,
			Result: processor.URLWatcher{URL: "https://example.com"},
		},
		"Unknown field": {
			Raw: `{"colour":"red"}`,
			Err: fmt.Errorf("%s is not a %s setting", "colour", hook.TypeURLWatcher),
		},
		"Value of the wrong shape": {
			Raw: `{"url":1}`,
			Err: assert.AnError,
		},
		"Malformed payload": {
			Raw: `{`,
			Err: assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			got, err := decodeSettings[processor.URLWatcher](hook.TypeURLWatcher, json.RawMessage(c.Raw))
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, got)
		})
	}
}
