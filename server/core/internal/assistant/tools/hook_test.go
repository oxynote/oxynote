package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/apps/github"
	"github.com/oxynote/oxynote/server/core/internal/apps/webchange"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/document/hook"
	"github.com/oxynote/oxynote/server/core/internal/document/hook/processor"
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

// hookArgs renders the document_id and hook_id arguments every hook
// write takes.
func hookArgs(hookID xid.ID) string {
	return `"document_id":"` + _testDocID.String() + `","hook_id":"` + hookID.String() + `"`
}

// _stubScheduleTime is _stubSchedule as the settings carry it.
var _stubScheduleTime = time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)

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
		// the editor labels a linear reminder with a custom duration as
		// a date, which is what a tool call sets.
		"Scheduled reminder": {
			Raw:    `{"type":"scheduled-reminder","schedule":"2030-01-01T00:00:00Z"}`,
			Type:   hook.TypeScheduledReminder,
			Result: `{"scale":"linear","duration":"custom","schedule":"2030-01-01T00:00:00Z"}`,
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

func Test_listHooks_Info(t *testing.T) {
	t.Parallel()

	info := listHooks{}.Info()

	assert.Equal(t, Traits{Group: GroupHooks}, info.Traits)

	assert.Equal(t, NameListHooks, info.Name)
	assert.NotEmpty(t, info.Description)
	assert.Equal(t, []string{"document_id", "branch_id"}, info.Required)
	assert.Contains(t, info.Properties, "branch_id")
}

func Test_listHooks_Execute(t *testing.T) {
	t.Parallel()

	failing := stubHookDB()
	failing.FetchDocumentHooksByBranchIDFunc = func(context.Context, xid.ID, string) ([]hook.Hook, error) {
		return nil, assert.AnError
	}

	cc := map[string]struct {
		DB            *DBMock
		Args          string
		BranchFetches int
		BranchLists   int
		HookLists     int
		Result        string
		Err           error
	}{
		"Malformed arguments":   {DB: stubHookDB(), Args: `{`, Err: assert.AnError},
		"Branch id is required": {DB: stubHookDB(), Args: `{"document_id":"` + _testDocID.String() + `"}`, Err: assert.AnError},
		"Unknown branch":        {DB: stubHookDB(), Args: `{` + targetArgs(_unknownBranchID) + `}`, BranchFetches: 1, BranchLists: 1, Err: assert.AnError},
		"Error returned by db.FetchDocumentHooksByBranchID": {
			DB:            failing,
			Args:          `{` + targetArgs(_stubBranchID) + `}`,
			BranchFetches: 1,
			HookLists:     1,
			Err:           assert.AnError,
		},
		"No hooks": {
			DB:            stubHookDB(),
			Args:          `{` + targetArgs(_stubMainBranchID) + `}`,
			BranchFetches: 1,
			HookLists:     1,
			Result:        `{"hooks":[]}`,
		},
		"Hooks with their settings and state": {
			DB:            stubHookDB(),
			Args:          `{` + targetArgs(_stubBranchID) + `}`,
			BranchFetches: 1,
			HookLists:     1,
			Result: `{"hooks":[{"id":"` + _testHookID.String() + `","type":"scheduled-reminder","block_uid":null,` +
				`"settings":{"scale":"linear","duration":"custom","schedule":"2030-01-01T00:00:00Z"},` +
				`"score":"100","state":{"startedAt":"2026-01-01T00:00:00Z"},"status":"active",` +
				`"created_at":"2026-01-01T00:00:00Z","updated_at":null}]}`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := listHooks{}.Execute(testDeps(c.DB, nil, nil).newInput(context.Background(), NameListHooks, json.RawMessage(c.Args)))
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, c.DB.FetchDocumentByBranchIDCalls(), c.BranchFetches)
			assert.Len(t, c.DB.FetchDocumentBranchesCalls(), c.BranchLists)
			assert.Len(t, c.DB.FetchDocumentHooksByBranchIDCalls(), c.HookLists)

			if err != nil {
				return
			}

			assert.JSONEq(t, c.Result, res)
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
		Score:     hk.Score,
		State:     json.RawMessage(hk.State.V),
		Status:    processor.StatusActive,
		CreatedAt: hk.CreatedAt,
		UpdatedAt: null.TimeFrom(_stubScheduleTime),
	}, newHookRow(hk))
}

func Test_createHookArgs_Validate(t *testing.T) {
	t.Parallel()

	scheduled := json.RawMessage(`{"type":"scheduled-reminder","schedule":"` + _stubSchedule + `"}`)

	ok := createHookArgs{
		DocumentID: _testDocID, BranchID: _stubMainBranchID,
		Settings: scheduled,
	}

	cc := map[string]struct {
		Args createHookArgs
		Err  error
	}{
		"Document id is required": {
			Args: createHookArgs{
				docTarget: docTarget{BranchID: _stubMainBranchID},
				Settings:  scheduled,
			},
			Err: errRequired("document_id"),
		},
		"Branch id is required": {
			Args: createHookArgs{
				docTarget: docTarget{DocumentID: _testDocID},
				Settings:  scheduled,
			},
			Err: errRequired("branch_id"),
		},
		"Settings type is required": {
			Args: createHookArgs{
				docTarget: ok.docTarget,
				Settings:  json.RawMessage(`{"schedule":"` + _stubSchedule + `"}`),
			},
			Err: errRequired("settings.type"),
		},
		"Schedule is required": {
			Args: createHookArgs{
				docTarget: ok.docTarget,
				Settings:  json.RawMessage(`{"type":"scheduled-reminder"}`),
			},
			Err: errRequired("schedule"),
		},
		// an unknown type is refused before its settings are looked at.
		"Unknown type": {
			Args: createHookArgs{docTarget: ok.docTarget, Settings: json.RawMessage(`{"type":"bogus"}`)},
			Err:  fmt.Errorf("type %q: %w", "bogus", hook.ErrInvalidType),
		},
		// another type's field is refused like any other unknown one.
		"Field of another type is refused": {
			Args: createHookArgs{docTarget: ok.docTarget, Settings: json.RawMessage(`{"type":"url-watcher","url":"https://example.com","image":"x"}`)},
			Err:  fmt.Errorf("%s is not a %s setting", "image", hook.TypeURLWatcher),
		},
		"Valid arguments": {Args: ok},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.Args.Validate())
		})
	}
}

func Test_createHook_Info(t *testing.T) {
	t.Parallel()

	info := createHook{}.Info()

	assert.Equal(t, Traits{Write: true, Group: GroupHooks}, info.Traits)

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

func Test_createHook_Summary(t *testing.T) {
	t.Parallel()

	scheduled := `,"settings":{"type":"scheduled-reminder","schedule":"` + _stubSchedule + `"}`

	cc := map[string]struct {
		DB            *DBMock
		Args          string
		BranchFetches int
		Result        string
		Err           error
	}{
		"Hook on the document": {
			DB:            stubHookDB(),
			Args:          `{` + targetArgs(_stubMainBranchID) + scheduled + `}`,
			BranchFetches: 1,
			Result:        "Add a Scheduled Reminder hook on Runbook",
		},
		// an anchored hook names its block, so the user approves the
		// placement as well as the hook.
		"Hook on a block": {
			DB:            stubHookDB(),
			Args:          `{` + targetArgs(_stubBranchID) + scheduled + `,"block_uid":"b"}`,
			BranchFetches: 1,
			Result:        "Add a Scheduled Reminder hook on Runbook on branch draft, block b",
		},
		"Error returned by db.FetchDocumentByBranchID": {
			DB:            failingDocumentDB(),
			Args:          requiredArgs(t, NameCreateHook),
			BranchFetches: 1,
			Err:           assert.AnError,
		},
		"Malformed arguments": {DB: stubHookDB(), Args: `{`, Err: assert.AnError},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			got, err := createHook{}.Summary(testDeps(c.DB, nil, nil).newInput(context.Background(), NameCreateHook, json.RawMessage(c.Args)))
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, c.DB.FetchDocumentByBranchIDCalls(), c.BranchFetches)
			assert.Empty(t, c.DB.FetchDocumentBranchesCalls())

			if err != nil {
				return
			}

			assert.Equal(t, ActionSummary{
				Tool:         NameCreateHook,
				DocumentID:   _testDocID,
				DocumentName: "Runbook",
				Summary:      c.Result,
			}, got)
		})
	}
}

func Test_createHook_Execute(t *testing.T) {
	t.Parallel()

	scheduled := `"settings":{"type":"scheduled-reminder","schedule":"` + _stubSchedule + `"}`

	failing := stubHookManager()
	failing.CreateHookFunc = func(context.Context, hook.CreateInput, xid.ID, string, string) (*hook.Hook, error) {
		return nil, assert.AnError
	}

	blockless := stubHookManager()
	blockless.CreateHookFunc = func(context.Context, hook.CreateInput, xid.ID, string, string) (*hook.Hook, error) {
		return nil, hook.ErrBlockNotFound
	}

	cc := map[string]struct {
		DB            *DBMock
		Man           *HookManagerMock
		Args          string
		BranchFetches int
		BranchLists   int
		Creates       int
		BlockUID      null.String
		Err           error
	}{
		"Malformed arguments": {DB: stubHookDB(), Args: `{`, Err: assert.AnError},
		"Type is required":    {DB: stubHookDB(), Args: `{` + targetArgs(_stubBranchID) + `,"settings":{}}`, Err: assert.AnError},
		"Schedule is required": {
			DB:   stubHookDB(),
			Args: `{` + targetArgs(_stubBranchID) + `,"settings":{"type":"scheduled-reminder"}}`,
			Err:  assert.AnError,
		},
		"Field of another type is refused": {
			DB:   stubHookDB(),
			Args: `{` + targetArgs(_stubBranchID) + `,"settings":{"type":"scheduled-reminder","schedule":"` + _stubSchedule + `","url":"https://example.com"}}`,
			Err:  fmt.Errorf("%s is not a %s setting", "url", hook.TypeScheduledReminder),
		},
		"Unknown branch": {
			DB:            stubHookDB(),
			Args:          `{` + targetArgs(_unknownBranchID) + `,` + scheduled + `}`,
			BranchFetches: 1,
			BranchLists:   1,
			Err:           assert.AnError,
		},
		"Unknown block": {
			DB:            stubHookDB(),
			Man:           blockless,
			Args:          `{` + targetArgs(_stubBranchID) + `,` + scheduled + `,"block_uid":"nope"}`,
			BranchFetches: 1,
			Creates:       1,
			Err:           fmt.Errorf("block %s: %w", "nope", errUnknownBlock),
		},
		"GitHub tracking without the app": {
			DB:            stubHookDB(),
			Args:          `{` + targetArgs(_stubBranchID) + `,"settings":{"type":"github-tracking","repository":"r","branch":"main","paths":["a"]}}`,
			BranchFetches: 1,
			Err:           fmt.Errorf("%s: %w", hook.TypeGithubTracking, github.ErrNotConfigured),
		},
		"URL watcher without changedetection": {
			DB:            stubHookDB(),
			Args:          `{` + targetArgs(_stubBranchID) + `,"settings":{"type":"url-watcher","url":"https://example.com"}}`,
			BranchFetches: 1,
			Err:           fmt.Errorf("%s: %w", hook.TypeURLWatcher, webchange.ErrNotConfigured),
		},
		"Error returned by hookMan.CreateHook": {
			DB:            stubHookDB(),
			Man:           failing,
			Args:          `{` + targetArgs(_stubBranchID) + `,` + scheduled + `}`,
			BranchFetches: 1,
			Creates:       1,
			Err:           assert.AnError,
		},
		"Created on the document": {
			DB:            stubHookDB(),
			Args:          `{` + targetArgs(_stubBranchID) + `,` + scheduled + `}`,
			BranchFetches: 1,
			Creates:       1,
		},
		"Created on a block": {
			DB:            stubHookDB(),
			Args:          `{` + targetArgs(_stubBranchID) + `,` + scheduled + `,"block_uid":"` + _stubContentUID + `"}`,
			BranchFetches: 1,
			Creates:       1,
			BlockUID:      null.StringFrom(_stubContentUID),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			d := hookDeps(c.DB, c.Man)
			inp := d.newInput(context.Background(), NameCreateHook, json.RawMessage(c.Args))

			res, err := createHook{}.Execute(inp)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, c.DB.FetchDocumentByBranchIDCalls(), c.BranchFetches)
			assert.Len(t, c.DB.FetchDocumentBranchesCalls(), c.BranchLists)
			assert.Len(t, d.hookMan.(*HookManagerMock).CreateHookCalls(), c.Creates)

			if err != nil {
				return
			}

			ff := d.hookMan.(*HookManagerMock).CreateHookCalls()
			require.Len(t, ff, 1)
			assert.Equal(t, hook.TypeScheduledReminder, ff[0].Ci.Type)
			assert.Equal(t, _testDocID, ff[0].DocumentID)
			assert.Equal(t, _stubBranchID, ff[0].Ci.BranchID)
			assert.Equal(t, "org", ff[0].OrganizationID)
			assert.Equal(t, c.BlockUID, ff[0].Ci.BlockID)
			assert.Equal(t, inp.userID, ff[0].UpdatedBy)

			var row hookRow
			require.NoError(t, json.Unmarshal([]byte(res), &row))
			assert.False(t, row.ID.IsNil())
			assert.Equal(t, hook.TypeScheduledReminder, row.Type)
			assert.Equal(t, c.BlockUID, row.BlockUID)
			assert.JSONEq(t, `{"scale":"linear","duration":"custom","schedule":"`+_stubSchedule+`"}`, string(row.Settings))
			assert.Equal(t, "100", row.Score.String())
			assert.Contains(t, string(row.State), "startedAt")
		})
	}
}

func Test_updateHookArgs_Validate(t *testing.T) {
	t.Parallel()

	settings := json.RawMessage(`{"type":"url-watcher","url":"https://example.com"}`)
	ref := hookRefArgs{DocumentID: _testDocID, HookID: _testHookID}

	// the settings are checked against the hook's own type later, so any
	// type's complete settings pass here, and none at all is a reset.
	cc := map[string]struct {
		Args updateHookArgs
		Err  error
	}{
		"Document id is required": {
			Args: updateHookArgs{hookRefArgs: hookRefArgs{HookID: _testHookID}, Settings: settings},
			Err:  errRequired("document_id"),
		},
		"Hook id is required": {
			Args: updateHookArgs{hookRefArgs: hookRefArgs{DocumentID: _testDocID}, Settings: settings},
			Err:  errRequired("hook_id"),
		},
		"URL is required": {
			Args: updateHookArgs{hookRefArgs: ref, Settings: json.RawMessage(`{"type":"url-watcher"}`)},
			Err:  errRequired("url"),
		},
		"Complete settings of any type": {Args: updateHookArgs{hookRefArgs: ref, Settings: settings}},
		"Absent settings":               {Args: updateHookArgs{hookRefArgs: ref}},
		"Null settings":                 {Args: updateHookArgs{hookRefArgs: ref, Settings: json.RawMessage(` null `)}},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.Args.Validate())
		})
	}
}

func Test_updateHookArgs_settingsFor(t *testing.T) {
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
			Result:   `{"scale":"linear","duration":"custom","schedule":"` + _stubSchedule + `"}`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			got, err := updateHookArgs{Settings: json.RawMessage(c.Settings)}.settingsFor(stubHook())
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.JSONEq(t, c.Result, string(got))
		})
	}
}

func Test_updateHookArgs_replacesSettings(t *testing.T) {
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

			assert.Equal(t, c.Result, updateHookArgs{Settings: c.Settings}.replacesSettings())
		})
	}
}

func Test_updateHook_Info(t *testing.T) {
	t.Parallel()

	info := updateHook{}.Info()

	assert.Equal(t, Traits{Write: true, Overwrites: true, Group: GroupHooks}, info.Traits)

	assert.Equal(t, NameUpdateHook, info.Name)
	assert.NotEmpty(t, info.Description)
	assert.Equal(t, []string{"document_id", "hook_id"}, info.Required)

	for _, key := range []string{"document_id", "hook_id", "settings"} {
		assert.Contains(t, info.Properties, key)
	}

	// a hook keeps its type, and the description says how to change it.
	assert.Contains(t, info.Description, "delete the hook and create another")
}

func Test_updateHook_Summary(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Args          string
		HookFetches   int
		BranchFetches int
		Result        string
		Err           error
	}{
		"New settings": {
			Args:          `{` + hookArgs(_testHookID) + `,"settings":{"type":"scheduled-reminder","schedule":"` + _stubSchedule + `"}}`,
			HookFetches:   1,
			BranchFetches: 1,
			Result:        "Update the Scheduled Reminder hook on Runbook on branch draft",
		},
		"A reset": {
			Args:          `{` + hookArgs(_testHookID) + `}`,
			HookFetches:   1,
			BranchFetches: 1,
			Result:        "Reset the Scheduled Reminder hook on Runbook on branch draft",
		},
		// settings of another type are refused here, so the user is not
		// asked to approve a call that cannot run.
		"Settings of another type": {
			Args:          `{` + hookArgs(_testHookID) + `,"settings":{"type":"url-watcher","url":"https://example.com"}}`,
			HookFetches:   1,
			BranchFetches: 1,
			Err:           errors.New("the hook is a scheduled-reminder, not a url-watcher; to change the type, delete it and create another"),
		},
		"Unknown hook": {
			Args:        `{` + hookArgs(_unknownHookID) + `}`,
			HookFetches: 1,
			Err:         assert.AnError,
		},
		"Malformed arguments": {Args: `{`, Err: assert.AnError},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := stubHookDB()

			got, err := updateHook{}.Summary(testDeps(db, nil, nil).newInput(context.Background(), NameUpdateHook, json.RawMessage(c.Args)))
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, db.FetchDocumentHookCalls(), c.HookFetches)
			assert.Len(t, db.FetchDocumentByBranchIDCalls(), c.BranchFetches)
			assert.Empty(t, db.FetchDocumentCalls())
			assert.Empty(t, db.FetchDocumentBranchesCalls())

			if err != nil {
				return
			}

			assert.Equal(t, ActionSummary{
				Tool:         NameUpdateHook,
				DocumentID:   _testDocID,
				DocumentName: "Runbook",
				Summary:      c.Result,
			}, got)
		})
	}
}

func Test_updateHook_Execute(t *testing.T) {
	t.Parallel()

	failingUpdate := stubHookManager()
	failingUpdate.UpdateHookFunc = func(context.Context, xid.ID, xid.ID, string, hook.UpdateInput, string) (*hook.Hook, error) {
		return nil, assert.AnError
	}

	failingReset := stubHookManager()
	failingReset.ResetHookFunc = func(context.Context, xid.ID, xid.ID, string) (*hook.Hook, error) {
		return nil, assert.AnError
	}

	later := time.Date(2031, 6, 1, 12, 0, 0, 0, time.UTC)

	cc := map[string]struct {
		DB          *DBMock
		Man         *HookManagerMock
		Args        string
		HookFetches int
		Updates     int
		Resets      int
		Settings    string
		Err         error
	}{
		"Malformed arguments": {DB: stubHookDB(), Args: `{`, Err: assert.AnError},
		"Hook id is required": {DB: stubHookDB(), Args: `{"document_id":"` + _testDocID.String() + `"}`, Err: assert.AnError},
		"Unknown hook": {
			DB:          stubHookDB(),
			Args:        `{` + hookArgs(_unknownHookID) + `,"settings":{"type":"scheduled-reminder","schedule":"` + _stubSchedule + `"}}`,
			HookFetches: 1,
			Err:         fmt.Errorf("hook %s on document %s: %w", _unknownHookID, _testDocID, errUnknownHook),
		},
		"Settings without a type": {
			DB:   stubHookDB(),
			Args: `{` + hookArgs(_testHookID) + `,"settings":{}}`,
			Err:  errRequired("settings" + "." + "type"),
		},
		"Schedule is required for a scheduled reminder": {
			DB:   stubHookDB(),
			Args: `{` + hookArgs(_testHookID) + `,"settings":{"type":"scheduled-reminder"}}`,
			Err:  errRequired("schedule"),
		},
		"Field of another type is refused": {
			DB:   stubHookDB(),
			Args: `{` + hookArgs(_testHookID) + `,"settings":{"type":"scheduled-reminder","schedule":"` + _stubSchedule + `","image":"nginx:1"}}`,
			Err:  fmt.Errorf("%s is not a %s setting", "image", hook.TypeScheduledReminder),
		},
		// a hook keeps its type for life, so complete settings of another
		// type are refused once the hook is known.
		"Settings of another type are refused": {
			DB:          stubHookDB(),
			Args:        `{` + hookArgs(_testHookID) + `,"settings":{"type":"url-watcher","url":"https://example.com"}}`,
			HookFetches: 1,
			Err:         errors.New("the hook is a scheduled-reminder, not a url-watcher; to change the type, delete it and create another"),
		},
		"Error returned by hookMan.UpdateHook": {
			DB:          stubHookDB(),
			Man:         failingUpdate,
			Args:        `{` + hookArgs(_testHookID) + `,"settings":{"type":"scheduled-reminder","schedule":"` + later.Format(time.RFC3339) + `"}}`,
			HookFetches: 1,
			Updates:     1,
			Err:         assert.AnError,
		},
		"Error returned by hookMan.ResetHook": {
			DB:          stubHookDB(),
			Man:         failingReset,
			Args:        `{` + hookArgs(_testHookID) + `}`,
			HookFetches: 1,
			Resets:      1,
			Err:         assert.AnError,
		},
		"Updated": {
			DB:          stubHookDB(),
			Args:        `{` + hookArgs(_testHookID) + `,"settings":{"type":"scheduled-reminder","schedule":"` + later.Format(time.RFC3339) + `"}}`,
			HookFetches: 1,
			Updates:     1,
			Settings:    `{"scale":"linear","duration":"custom","schedule":"2031-06-01T12:00:00Z"}`,
		},
		"Reset keeping the settings": {
			DB:          stubHookDB(),
			Args:        `{` + hookArgs(_testHookID) + `}`,
			HookFetches: 1,
			Resets:      1,
			Settings:    string(stubHook().Settings),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			d := hookDeps(c.DB, c.Man)
			inp := d.newInput(context.Background(), NameUpdateHook, json.RawMessage(c.Args))

			res, err := updateHook{}.Execute(inp)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, c.DB.FetchDocumentHookCalls(), c.HookFetches)

			man := d.hookMan.(*HookManagerMock)
			assert.Len(t, man.UpdateHookCalls(), c.Updates)
			assert.Len(t, man.ResetHookCalls(), c.Resets)

			if err != nil {
				return
			}

			var row hookRow
			require.NoError(t, json.Unmarshal([]byte(res), &row))
			assert.Equal(t, _testHookID, row.ID)
			assert.JSONEq(t, c.Settings, string(row.Settings))
			assert.Equal(t, null.TimeFrom(_stubScheduleTime), row.UpdatedAt)
		})
	}
}

func Test_hookRefArgs_Validate(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Args hookRefArgs
		Err  error
	}{
		"Document id is required": {Args: hookRefArgs{HookID: _testHookID}, Err: errRequired("document_id")},
		"Hook id is required":     {Args: hookRefArgs{DocumentID: _testDocID}, Err: errRequired("hook_id")},
		"Valid arguments":         {Args: hookRefArgs{DocumentID: _testDocID, HookID: _testHookID}},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.Args.Validate())
		})
	}
}

func Test_deleteHook_Info(t *testing.T) {
	t.Parallel()

	info := deleteHook{}.Info()

	assert.Equal(t, Traits{Write: true, Destructive: true, Group: GroupHooks}, info.Traits)

	assert.Equal(t, NameDeleteHook, info.Name)
	assert.NotEmpty(t, info.Description)
	assert.Equal(t, []string{"document_id", "hook_id"}, info.Required)
	assert.Contains(t, info.Properties, "hook_id")
}

func Test_deleteHook_Summary(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Args          string
		HookFetches   int
		BranchFetches int
		Result        string
		Err           error
	}{
		"Hook on a branch": {
			Args:          `{` + hookArgs(_testHookID) + `}`,
			HookFetches:   1,
			BranchFetches: 1,
			Result:        "Delete the Scheduled Reminder hook on Runbook on branch draft",
		},
		"Unknown hook": {
			Args:        `{` + hookArgs(_unknownHookID) + `}`,
			HookFetches: 1,
			Err:         assert.AnError,
		},
		"Malformed arguments": {Args: `{`, Err: assert.AnError},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := stubHookDB()

			got, err := deleteHook{}.Summary(testDeps(db, nil, nil).newInput(context.Background(), NameDeleteHook, json.RawMessage(c.Args)))
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, db.FetchDocumentHookCalls(), c.HookFetches)
			assert.Len(t, db.FetchDocumentByBranchIDCalls(), c.BranchFetches)
			assert.Empty(t, db.FetchDocumentCalls())
			assert.Empty(t, db.FetchDocumentBranchesCalls())

			if err != nil {
				return
			}

			assert.Equal(t, ActionSummary{
				Tool:         NameDeleteHook,
				DocumentID:   _testDocID,
				DocumentName: "Runbook",
				Summary:      c.Result,
			}, got)
		})
	}
}

func Test_deleteHook_Execute(t *testing.T) {
	t.Parallel()

	failing := stubHookManager()
	failing.DeleteHookFunc = func(context.Context, xid.ID, xid.ID, string, string) error {
		return assert.AnError
	}

	cc := map[string]struct {
		DB          *DBMock
		Man         *HookManagerMock
		Args        string
		HookFetches int
		Deletes     int
		Err         error
	}{
		"Malformed arguments": {DB: stubHookDB(), Args: `{`, Err: assert.AnError},
		"Hook id is required": {DB: stubHookDB(), Args: `{"document_id":"` + _testDocID.String() + `"}`, Err: assert.AnError},
		"Unknown hook": {
			DB:          stubHookDB(),
			Args:        `{` + hookArgs(_unknownHookID) + `}`,
			HookFetches: 1,
			Err:         fmt.Errorf("hook %s on document %s: %w", _unknownHookID, _testDocID, errUnknownHook),
		},
		"Error returned by hookMan.DeleteHook": {
			DB:          stubHookDB(),
			Man:         failing,
			Args:        `{` + hookArgs(_testHookID) + `}`,
			HookFetches: 1,
			Deletes:     1,
			Err:         assert.AnError,
		},
		"Deleted": {
			DB:          stubHookDB(),
			Args:        `{` + hookArgs(_testHookID) + `}`,
			HookFetches: 1,
			Deletes:     1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			d := hookDeps(c.DB, c.Man)
			inp := d.newInput(context.Background(), NameDeleteHook, json.RawMessage(c.Args))

			res, err := deleteHook{}.Execute(inp)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, c.DB.FetchDocumentHookCalls(), c.HookFetches)
			assert.Len(t, d.hookMan.(*HookManagerMock).DeleteHookCalls(), c.Deletes)

			if err != nil {
				return
			}

			ff := d.hookMan.(*HookManagerMock).DeleteHookCalls()
			require.Len(t, ff, 1)
			assert.Equal(t, _testHookID, ff[0].ID)
			assert.JSONEq(t, `{"hook_id":"`+_testHookID.String()+`","deleted":true}`, res)

			// the branch stays, and the editor showing it has to redraw.
		})
	}
}

func Test_hookRefArgs_fetchHookAndBranch(t *testing.T) {
	t.Parallel()

	branchless := stubHookDB()
	branchless.FetchDocumentHookFunc = func(context.Context, xid.ID, string) (*hook.Hook, error) {
		hk := stubHook()
		hk.BranchID = null.Value[xid.ID]{}

		return hk, nil
	}

	failingDoc := stubHookDB()
	failingDoc.FetchDocumentByBranchIDFunc = func(context.Context, xid.ID, string) (*document.Document, error) {
		return nil, assert.AnError
	}

	cc := map[string]struct {
		DB              *DBMock
		HookID          xid.ID
		BranchFetches   int
		DocumentFetches int
		Branch          string
		Err             error
	}{
		"Unknown hook": {
			DB:     stubHookDB(),
			HookID: _unknownHookID,
			Err:    fmt.Errorf("fetch hook: %w", fmt.Errorf("hook %s on document %s: %w", _unknownHookID, _testDocID, errUnknownHook)),
		},
		"Error returned by db.FetchDocumentByBranchID": {
			DB:            failingDoc,
			HookID:        _testHookID,
			BranchFetches: 1,
			Err:           assert.AnError,
		},
		"Hook on a branch": {
			DB:            stubHookDB(),
			HookID:        _testHookID,
			BranchFetches: 1,
			Branch:        _stubBranchName,
		},
		// a hook whose branch is gone is named on the default branch,
		// which the sweep will remove it from regardless.
		"Hook whose branch is gone": {
			DB:              branchless,
			HookID:          _testHookID,
			DocumentFetches: 1,
			Branch:          document.DefaultBranch,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			doc, hk, err := hookRefArgs{DocumentID: _testDocID, HookID: c.HookID}.fetchHookAndBranch(testDeps(c.DB, nil, nil).newInput(context.Background(), NameUpdateHook, json.RawMessage(`{}`)))
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, c.DB.FetchDocumentHookCalls(), 1)
			assert.Len(t, c.DB.FetchDocumentByBranchIDCalls(), c.BranchFetches)
			assert.Len(t, c.DB.FetchDocumentCalls(), c.DocumentFetches)
			assert.Empty(t, c.DB.FetchDocumentBranchesCalls())

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
