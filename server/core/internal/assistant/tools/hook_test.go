package tools

import (
	"context"
	"encoding/json"
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

// hookDeps builds session wiring over the given DB with a hook notifier
// attached, so a test can assert the broadcast a hook write ends with.
func hookDeps(db *DBMock) (*Deps, *HookNotifierMock) {
	d := testDeps(db, nil, nil)
	hooks := &HookNotifierMock{}
	d.hooks = hooks

	return d, hooks
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

// decodeHookRow reads a hook tool's result back into its row.
func decodeHookRow(t *testing.T, res string) hookRow {
	t.Helper()

	var row hookRow

	require.NoError(t, json.Unmarshal([]byte(res), &row))

	return row
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

		assert.Equal(t, string(tp), v["title"])
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
			Raw:    `{"type":"github-tracking","repository":"o/r","branch":"main","paths":["a","b"]}`,
			Type:   hook.TypeGithubTracking,
			Result: `{"repository":"o/r","branch":"main","paths":["a","b"]}`,
		},
		"GitHub tracking without a repository": {
			Raw: `{"type":"github-tracking","branch":"main","paths":["a"]}`,
			Err: errRequired("repository"),
		},
		"GitHub tracking without a branch": {
			Raw: `{"type":"github-tracking","repository":"o/r","paths":["a"]}`,
			Err: errRequired("branch"),
		},
		"GitHub tracking without paths": {
			Raw: `{"type":"github-tracking","repository":"o/r","branch":"main","paths":[]}`,
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

	var uw processor.URLWatcher

	// an absent payload decodes as an empty one.
	require.NoError(t, decodeSettings(hook.TypeURLWatcher, nil, &uw))
	assert.Empty(t, uw.URL)

	require.NoError(t, decodeSettings(hook.TypeURLWatcher, json.RawMessage(`{"url":"https://example.com"}`), &uw))
	assert.Equal(t, "https://example.com", uw.URL)

	err := decodeSettings(hook.TypeURLWatcher, json.RawMessage(`{"colour":"red"}`), &uw)
	assert.Equal(t, fmt.Errorf("%s is not a %s setting", "colour", hook.TypeURLWatcher), err)

	// a value of the wrong shape is reported with its path.
	err = decodeSettings(hook.TypeURLWatcher, json.RawMessage(`{"url":1}`), &uw)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid settings")

	require.Error(t, decodeSettings(hook.TypeURLWatcher, json.RawMessage(`{`), &uw))
}

func Test_listHooksArgs_Validate(t *testing.T) {
	t.Parallel()

	assertValidate(t,
		listHooksArgs{docTarget{DocumentID: _testDocID, BranchID: _stubMainBranchID}},
		map[string]Args{
			"document_id": listHooksArgs{docTarget{BranchID: _stubMainBranchID}},
			"branch_id":   listHooksArgs{docTarget{DocumentID: _testDocID}},
		},
	)
}

func Test_listHooks_Info(t *testing.T) {
	t.Parallel()

	info := listHooks{}.Info()

	assert.Equal(t, NameListHooks, info.Name)
	assert.NotEmpty(t, info.Description)
	assert.Equal(t, []string{"document_id", "branch_id"}, info.Required)
	assert.Contains(t, info.Properties, "branch_id")
}

func Test_listHooks_Traits(t *testing.T) {
	t.Parallel()

	assert.Equal(t, Traits{}, listHooks{}.Traits())
}

func Test_listHooks_Title(t *testing.T) {
	t.Parallel()

	got, err := listHooks{}.Title(testInput(testDeps(nil, nil, nil), NameListHooks, `{}`))
	require.NoError(t, err)
	assert.Empty(t, got)
}

func Test_listHooks_Execute(t *testing.T) {
	t.Parallel()

	failing := stubHookDB()
	failing.FetchDocumentHooksByBranchIDFunc = func(context.Context, xid.ID, string) ([]hook.Hook, error) {
		return nil, assert.AnError
	}

	cc := map[string]struct {
		DB     *DBMock
		Args   string
		Result string
		Err    error
	}{
		"Malformed arguments":   {DB: stubHookDB(), Args: `{`, Err: assert.AnError},
		"Branch id is required": {DB: stubHookDB(), Args: `{"document_id":"` + _testDocID.String() + `"}`, Err: assert.AnError},
		"Unknown branch":        {DB: stubHookDB(), Args: `{` + targetArgs(_unknownBranchID) + `}`, Err: assert.AnError},
		"Error returned by db.FetchDocumentHooksByBranchID": {
			DB:   failing,
			Args: `{` + targetArgs(_stubBranchID) + `}`,
			Err:  assert.AnError,
		},
		"No hooks": {
			DB:     stubHookDB(),
			Args:   `{` + targetArgs(_stubMainBranchID) + `}`,
			Result: `{"hooks":[]}`,
		},
		"Hooks with their settings and state": {
			DB:   stubHookDB(),
			Args: `{` + targetArgs(_stubBranchID) + `}`,
			Result: `{"hooks":[{"id":"` + _testHookID.String() + `","type":"scheduled-reminder","block_uid":null,` +
				`"settings":{"scale":"linear","duration":"custom","schedule":"2030-01-01T00:00:00Z"},` +
				`"score":"100","state":{"startedAt":"2026-01-01T00:00:00Z"},` +
				`"created_at":"2026-01-01T00:00:00Z","updated_at":null}]}`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := listHooks{}.Execute(testInput(testDeps(c.DB, nil, nil), NameListHooks, c.Args))
			testutil.AssertEqualError(t, c.Err, err)

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
		State:     json.RawMessage(hk.State),
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

	assertValidate(t, ok, map[string]Args{
		"document_id": createHookArgs{
			docTarget: docTarget{BranchID: _stubMainBranchID},
			Settings:  scheduled,
		},
		"branch_id": createHookArgs{
			docTarget: docTarget{DocumentID: _testDocID},
			Settings:  scheduled,
		},
		"settings.type": createHookArgs{
			docTarget: ok.docTarget,
			Settings:  json.RawMessage(`{"schedule":"` + _stubSchedule + `"}`),
		},
		"schedule": createHookArgs{
			docTarget: ok.docTarget,
			Settings:  json.RawMessage(`{"type":"scheduled-reminder"}`),
		},
	})

	// an unknown type is refused before its settings are looked at.
	err := createHookArgs{docTarget: ok.docTarget, Settings: json.RawMessage(`{"type":"bogus"}`)}.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), `type "bogus"`)

	// another type's field is refused like any other unknown one.
	err = createHookArgs{docTarget: ok.docTarget, Settings: json.RawMessage(`{"type":"url-watcher","url":"https://example.com","image":"x"}`)}.Validate()
	require.Error(t, err)
	assert.Equal(t, fmt.Errorf("%s is not a %s setting", "image", hook.TypeURLWatcher), err)
}

func Test_createHook_Info(t *testing.T) {
	t.Parallel()

	info := createHook{}.Info()

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

func Test_createHook_Traits(t *testing.T) {
	t.Parallel()

	assert.Equal(t, Traits{Write: true}, createHook{}.Traits())
}

func Test_createHook_Title(t *testing.T) {
	t.Parallel()

	scheduled := `,"settings":{"type":"scheduled-reminder","schedule":"` + _stubSchedule + `"}`

	got, err := createHook{}.Title(testInput(testDeps(stubHookDB(), nil, nil), NameCreateHook, `{`+targetArgs(_stubMainBranchID)+scheduled+`}`))
	require.NoError(t, err)
	assert.Equal(t, "Adding a hook to Runbook", got)

	got, err = createHook{}.Title(testInput(testDeps(stubHookDB(), nil, nil), NameCreateHook, `{`+targetArgs(_stubBranchID)+scheduled+`}`))
	require.NoError(t, err)
	assert.Equal(t, "Adding a hook to Runbook on branch draft", got)

	_, err = createHook{}.Title(testInput(testDeps(failingDocumentDB(), nil, nil), NameCreateHook, requiredArgs(t, NameCreateHook)))
	require.Error(t, err)

	_, err = createHook{}.Title(testInput(testDeps(stubHookDB(), nil, nil), NameCreateHook, `{`))
	require.Error(t, err)
}

func Test_createHook_Summary(t *testing.T) {
	t.Parallel()

	scheduled := `,"settings":{"type":"scheduled-reminder","schedule":"` + _stubSchedule + `"}`

	got, err := createHook{}.Summary(testInput(testDeps(stubHookDB(), nil, nil), NameCreateHook, `{`+targetArgs(_stubMainBranchID)+scheduled+`}`))
	require.NoError(t, err)
	assert.Equal(t, ActionSummary{
		Tool:         NameCreateHook,
		DocumentID:   _testDocID,
		DocumentName: "Runbook",
		Summary:      "Add a Scheduled Reminder hook on Runbook",
	}, got)

	// an anchored hook names its block, so the user approves the
	// placement as well as the hook.
	got, err = createHook{}.Summary(testInput(testDeps(stubHookDB(), nil, nil), NameCreateHook, `{`+targetArgs(_stubBranchID)+scheduled+`,"block_uid":"b"}`))
	require.NoError(t, err)
	assert.Equal(t, "Add a Scheduled Reminder hook on Runbook on branch draft, block b", got.Summary)

	_, err = createHook{}.Summary(testInput(testDeps(failingDocumentDB(), nil, nil), NameCreateHook, requiredArgs(t, NameCreateHook)))
	require.Error(t, err)

	_, err = createHook{}.Summary(testInput(testDeps(stubHookDB(), nil, nil), NameCreateHook, `{`))
	require.Error(t, err)
}

func Test_createHook_Execute(t *testing.T) {
	t.Parallel()

	scheduled := `"settings":{"type":"scheduled-reminder","schedule":"` + _stubSchedule + `"}`

	failing := stubHookDB()
	failing.InsertDocumentHookFunc = func(context.Context, hook.Hook) error {
		return assert.AnError
	}

	cc := map[string]struct {
		DB       *DBMock
		Args     string
		BlockUID null.String
		Notify   int
		Err      error
	}{
		"Malformed arguments": {DB: stubHookDB(), Args: `{`, Err: assert.AnError},
		"Type is required":    {DB: stubHookDB(), Args: `{` + targetArgs(_stubBranchID) + `,"settings":{}}`, Err: assert.AnError},
		"Schedule is required": {
			DB:   stubHookDB(),
			Args: `{` + targetArgs(_stubBranchID) + `,"settings":{"type":"scheduled-reminder"}}`,
			Err:  assert.AnError,
		},
		"Another type's field is refused": {
			DB:   stubHookDB(),
			Args: `{` + targetArgs(_stubBranchID) + `,"settings":{"type":"scheduled-reminder","schedule":"` + _stubSchedule + `","url":"https://example.com"}}`,
			Err:  fmt.Errorf("%s: %w", NameCreateHook, fmt.Errorf("%s is not a %s setting", "url", hook.TypeScheduledReminder)),
		},
		"Unknown branch": {
			DB:   stubHookDB(),
			Args: `{` + targetArgs(_unknownBranchID) + `,` + scheduled + `}`,
			Err:  assert.AnError,
		},
		"Unknown block": {
			DB:   stubHookDB(),
			Args: `{` + targetArgs(_stubBranchID) + `,` + scheduled + `,"block_uid":"nope"}`,
			Err:  fmt.Errorf("create_hook: %w", fmt.Errorf("block %s: %w", "nope", errUnknownBlock)),
		},
		"GitHub tracking without the app": {
			DB:   stubHookDB(),
			Args: `{` + targetArgs(_stubBranchID) + `,"settings":{"type":"github-tracking","repository":"o/r","branch":"main","paths":["a"]}}`,
			Err:  fmt.Errorf("create_hook: %w", fmt.Errorf("%s: %w", hook.TypeGithubTracking, github.ErrNotConfigured)),
		},
		"URL watcher without changedetection": {
			DB:   stubHookDB(),
			Args: `{` + targetArgs(_stubBranchID) + `,"settings":{"type":"url-watcher","url":"https://example.com"}}`,
			Err:  fmt.Errorf("create_hook: %w", fmt.Errorf("%s: %w", hook.TypeURLWatcher, webchange.ErrNotConfigured)),
		},
		"Error returned by db.InsertDocumentHook": {
			DB:   failing,
			Args: `{` + targetArgs(_stubBranchID) + `,` + scheduled + `}`,
			Err:  assert.AnError,
		},
		"Created on the document": {
			DB:     stubHookDB(),
			Args:   `{` + targetArgs(_stubBranchID) + `,` + scheduled + `}`,
			Notify: 1,
		},
		"Created on a block": {
			DB:       stubHookDB(),
			Args:     `{` + targetArgs(_stubBranchID) + `,` + scheduled + `,"block_uid":"` + _stubContentUID + `"}`,
			BlockUID: null.StringFrom(_stubContentUID),
			Notify:   1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			d, hooks := hookDeps(c.DB)
			inp := testInput(d, NameCreateHook, c.Args)

			res, err := createHook{}.Execute(inp)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, hooks.NotifyHooksChangeCalls(), c.Notify)

			if err != nil {
				return
			}

			ff := c.DB.InsertDocumentHookCalls()
			require.Len(t, ff, 1)
			assert.Equal(t, hook.TypeScheduledReminder, ff[0].Hk.Type)
			assert.Equal(t, null.ValueFrom(_testDocID), ff[0].Hk.DocumentID)
			assert.Equal(t, null.ValueFrom(_stubBranchID), ff[0].Hk.BranchID)
			assert.Equal(t, null.StringFrom("org"), ff[0].Hk.OrganizationID)
			assert.Equal(t, c.BlockUID, ff[0].Hk.BlockID)

			row := decodeHookRow(t, res)
			assert.Equal(t, ff[0].Hk.ID, row.ID)
			assert.Equal(t, hook.TypeScheduledReminder, row.Type)
			assert.Equal(t, c.BlockUID, row.BlockUID)
			assert.JSONEq(t, `{"scale":"linear","duration":"custom","schedule":"`+_stubSchedule+`"}`, string(row.Settings))
			assert.Equal(t, "100", row.Score.String())
			assert.Contains(t, string(row.State), "startedAt")

			bf := hooks.NotifyHooksChangeCalls()
			require.Len(t, bf, 1)
			assert.Equal(t, "org", bf[0].OrganizationID)
			assert.Equal(t, null.ValueFrom(_testDocID), bf[0].DocumentID)
			assert.Equal(t, null.ValueFrom(_stubBranchID), bf[0].BranchID)

			assert.Equal(t, []Touched{{DocumentID: _testDocID, BranchID: _stubBranchID}}, inp.touched)
		})
	}
}

func Test_updateHookArgs_Validate(t *testing.T) {
	t.Parallel()

	settings := json.RawMessage(`{"type":"url-watcher","url":"https://example.com"}`)

	// the settings are checked against the hook's own type later, so any
	// type's complete settings pass here.
	assertValidate(t,
		updateHookArgs{DocumentID: _testDocID, HookID: _testHookID, Settings: settings},
		map[string]Args{
			"document_id":   updateHookArgs{hookRefArgs: hookRefArgs{HookID: _testHookID}, Settings: settings},
			"hook_id":       updateHookArgs{hookRefArgs: hookRefArgs{DocumentID: _testDocID}, Settings: settings},
			"settings.type": updateHookArgs{hookRefArgs: hookRefArgs{DocumentID: _testDocID, HookID: _testHookID}},
			"url":           updateHookArgs{hookRefArgs: hookRefArgs{DocumentID: _testDocID, HookID: _testHookID}, Settings: json.RawMessage(`{"type":"url-watcher"}`)},
		},
	)
}

func Test_updateHook_Info(t *testing.T) {
	t.Parallel()

	info := updateHook{}.Info()

	assert.Equal(t, NameUpdateHook, info.Name)
	assert.NotEmpty(t, info.Description)
	assert.Equal(t, []string{"document_id", "hook_id", "settings"}, info.Required)

	for _, key := range []string{"document_id", "hook_id", "settings"} {
		assert.Contains(t, info.Properties, key)
	}

	assert.Equal(t, hookSettingsSchema(), info.Properties["settings"])
	assert.NotContains(t, info.Properties, "type")

	// a hook keeps its type, and the description says how to change it.
	assert.Contains(t, info.Description, "delete the hook and create another")
}

func Test_updateHook_Traits(t *testing.T) {
	t.Parallel()

	assert.Equal(t, Traits{Write: true}, updateHook{}.Traits())
}

func Test_updateHook_Title(t *testing.T) {
	t.Parallel()

	got, err := updateHook{}.Title(testInput(testDeps(stubHookDB(), nil, nil), NameUpdateHook, `{`+hookArgs(_testHookID)+`,"settings":{"type":"scheduled-reminder","schedule":"`+_stubSchedule+`"}}`))
	require.NoError(t, err)
	assert.Equal(t, "Updating a hook on Runbook", got)

	_, err = updateHook{}.Title(testInput(testDeps(failingDocumentDB(), nil, nil), NameUpdateHook, requiredArgs(t, NameUpdateHook)))
	require.Error(t, err)

	_, err = updateHook{}.Title(testInput(testDeps(stubHookDB(), nil, nil), NameUpdateHook, `{`))
	require.Error(t, err)
}

func Test_updateHook_Summary(t *testing.T) {
	t.Parallel()

	got, err := updateHook{}.Summary(testInput(testDeps(stubHookDB(), nil, nil), NameUpdateHook, `{`+hookArgs(_testHookID)+`,"settings":{"type":"scheduled-reminder","schedule":"`+_stubSchedule+`"}}`))
	require.NoError(t, err)
	assert.Equal(t, ActionSummary{
		Tool:         NameUpdateHook,
		DocumentID:   _testDocID,
		DocumentName: "Runbook",
		Summary:      "Update the Scheduled Reminder hook on Runbook on branch draft",
	}, got)

	// settings of another type are refused here, so the user is not
	// asked to approve a call that cannot run.
	_, err = updateHook{}.Summary(testInput(testDeps(stubHookDB(), nil, nil), NameUpdateHook, `{`+hookArgs(_testHookID)+`,"settings":{"type":"url-watcher","url":"https://example.com"}}`))
	require.Error(t, err)
	assert.Equal(t, fmt.Errorf("%s: %w", NameUpdateHook, errHookTypeMismatch(hook.TypeScheduledReminder, hook.TypeURLWatcher)), err)

	_, err = updateHook{}.Summary(testInput(testDeps(stubHookDB(), nil, nil), NameUpdateHook, `{`+hookArgs(_unknownHookID)+`,"settings":{"type":"scheduled-reminder","schedule":"`+_stubSchedule+`"}}`))
	require.Error(t, err)

	_, err = updateHook{}.Summary(testInput(testDeps(stubHookDB(), nil, nil), NameUpdateHook, `{`))
	require.Error(t, err)
}

func Test_updateHook_Execute(t *testing.T) {
	t.Parallel()

	failing := stubHookDB()
	failing.UpdateDocumentHookFunc = func(context.Context, hook.Hook) error {
		return assert.AnError
	}

	later := time.Date(2031, 6, 1, 12, 0, 0, 0, time.UTC)

	cc := map[string]struct {
		DB     *DBMock
		Args   string
		Notify int
		Err    error
	}{
		"Malformed arguments": {DB: stubHookDB(), Args: `{`, Err: assert.AnError},
		"Hook id is required": {DB: stubHookDB(), Args: `{"document_id":"` + _testDocID.String() + `"}`, Err: assert.AnError},
		"Unknown hook": {
			DB:   stubHookDB(),
			Args: `{` + hookArgs(_unknownHookID) + `,"settings":{"type":"scheduled-reminder","schedule":"` + _stubSchedule + `"}}`,
			Err:  fmt.Errorf("update_hook: %w", fmt.Errorf("hook %s on document %s: %w", _unknownHookID, _testDocID, errUnknownHook)),
		},
		"Settings are required": {
			DB:   stubHookDB(),
			Args: `{` + hookArgs(_testHookID) + `}`,
			Err:  fmt.Errorf("%s: %w", NameUpdateHook, errRequired("settings"+"."+"type")),
		},
		"Schedule is required for a scheduled reminder": {
			DB:   stubHookDB(),
			Args: `{` + hookArgs(_testHookID) + `,"settings":{"type":"scheduled-reminder"}}`,
			Err:  fmt.Errorf("%s: %w", NameUpdateHook, errRequired("schedule")),
		},
		"Another type's field is refused": {
			DB:   stubHookDB(),
			Args: `{` + hookArgs(_testHookID) + `,"settings":{"type":"scheduled-reminder","schedule":"` + _stubSchedule + `","image":"nginx:1"}}`,
			Err:  fmt.Errorf("%s: %w", NameUpdateHook, fmt.Errorf("%s is not a %s setting", "image", hook.TypeScheduledReminder)),
		},
		// a hook keeps its type for life, so complete settings of another
		// type are refused once the hook is known.
		"Another type's settings are refused": {
			DB:   stubHookDB(),
			Args: `{` + hookArgs(_testHookID) + `,"settings":{"type":"url-watcher","url":"https://example.com"}}`,
			Err:  fmt.Errorf("update_hook: %w", errHookTypeMismatch(hook.TypeScheduledReminder, hook.TypeURLWatcher)),
		},
		"Error returned by db.UpdateDocumentHook": {
			DB:   failing,
			Args: `{` + hookArgs(_testHookID) + `,"settings":{"type":"scheduled-reminder","schedule":"` + later.Format(time.RFC3339) + `"}}`,
			Err:  assert.AnError,
		},
		"Updated": {
			DB:     stubHookDB(),
			Args:   `{` + hookArgs(_testHookID) + `,"settings":{"type":"scheduled-reminder","schedule":"` + later.Format(time.RFC3339) + `"}}`,
			Notify: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			d, hooks := hookDeps(c.DB)
			inp := testInput(d, NameUpdateHook, c.Args)

			res, err := updateHook{}.Execute(inp)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, hooks.NotifyHooksChangeCalls(), c.Notify)

			if err != nil {
				return
			}

			// the settings are replaced and the state reset: a fresh
			// start rather than the old one carried over.
			ff := c.DB.UpdateDocumentHookCalls()
			require.Len(t, ff, 1)
			assert.Equal(t, _testHookID, ff[0].Hk.ID)
			assert.JSONEq(t, `{"scale":"linear","duration":"custom","schedule":"2031-06-01T12:00:00Z"}`, string(ff[0].Hk.Settings))
			assert.NotContains(t, string(ff[0].Hk.State), "2026-01-01")
			assert.True(t, ff[0].Hk.UpdatedAt.Valid)

			row := decodeHookRow(t, res)
			assert.Equal(t, _testHookID, row.ID)
			assert.JSONEq(t, string(ff[0].Hk.Settings), string(row.Settings))
			assert.Equal(t, "100", row.Score.String())

			assert.Equal(t, []Touched{{DocumentID: _testDocID, BranchID: _stubBranchID}}, inp.touched)
		})
	}
}

func Test_hookRefArgs_Validate(t *testing.T) {
	t.Parallel()

	assertValidate(t,
		hookRefArgs{DocumentID: _testDocID, HookID: _testHookID},
		map[string]Args{
			"document_id": hookRefArgs{HookID: _testHookID},
			"hook_id":     hookRefArgs{DocumentID: _testDocID},
		},
	)
}

func Test_hookRefProps(t *testing.T) {
	t.Parallel()

	got := hookRefProps()

	assert.Contains(t, got, "document_id")
	assert.Contains(t, got, "hook_id")
}

func Test_resetHook_Info(t *testing.T) {
	t.Parallel()

	info := resetHook{}.Info()

	assert.Equal(t, NameResetHook, info.Name)
	assert.NotEmpty(t, info.Description)
	assert.Equal(t, []string{"document_id", "hook_id"}, info.Required)
	assert.Contains(t, info.Properties, "hook_id")
}

func Test_resetHook_Traits(t *testing.T) {
	t.Parallel()

	assert.Equal(t, Traits{Write: true}, resetHook{}.Traits())
}

func Test_resetHook_Title(t *testing.T) {
	t.Parallel()

	got, err := resetHook{}.Title(testInput(testDeps(stubHookDB(), nil, nil), NameResetHook, `{`+hookArgs(_testHookID)+`}`))
	require.NoError(t, err)
	assert.Equal(t, "Resetting a hook on Runbook", got)

	_, err = resetHook{}.Title(testInput(testDeps(failingDocumentDB(), nil, nil), NameResetHook, requiredArgs(t, NameResetHook)))
	require.Error(t, err)

	_, err = resetHook{}.Title(testInput(testDeps(stubHookDB(), nil, nil), NameResetHook, `{`))
	require.Error(t, err)
}

func Test_resetHook_Summary(t *testing.T) {
	t.Parallel()

	got, err := resetHook{}.Summary(testInput(testDeps(stubHookDB(), nil, nil), NameResetHook, `{`+hookArgs(_testHookID)+`}`))
	require.NoError(t, err)
	assert.Equal(t, ActionSummary{
		Tool:         NameResetHook,
		DocumentID:   _testDocID,
		DocumentName: "Runbook",
		Summary:      "Reset the Scheduled Reminder hook on Runbook on branch draft",
	}, got)

	_, err = resetHook{}.Summary(testInput(testDeps(stubHookDB(), nil, nil), NameResetHook, `{`+hookArgs(_unknownHookID)+`}`))
	require.Error(t, err)

	_, err = resetHook{}.Summary(testInput(testDeps(stubHookDB(), nil, nil), NameResetHook, `{`))
	require.Error(t, err)
}

func Test_resetHook_Execute(t *testing.T) {
	t.Parallel()

	failing := stubHookDB()
	failing.UpdateDocumentHookFunc = func(context.Context, hook.Hook) error {
		return assert.AnError
	}

	cc := map[string]struct {
		DB     *DBMock
		Args   string
		Notify int
		Err    error
	}{
		"Malformed arguments": {DB: stubHookDB(), Args: `{`, Err: assert.AnError},
		"Hook id is required": {DB: stubHookDB(), Args: `{"document_id":"` + _testDocID.String() + `"}`, Err: assert.AnError},
		"Unknown hook": {
			DB:   stubHookDB(),
			Args: `{` + hookArgs(_unknownHookID) + `}`,
			Err:  fmt.Errorf("reset_hook: %w", fmt.Errorf("hook %s on document %s: %w", _unknownHookID, _testDocID, errUnknownHook)),
		},
		"Error returned by db.UpdateDocumentHook": {
			DB:   failing,
			Args: `{` + hookArgs(_testHookID) + `}`,
			Err:  assert.AnError,
		},
		"Reset": {
			DB:     stubHookDB(),
			Args:   `{` + hookArgs(_testHookID) + `}`,
			Notify: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			d, hooks := hookDeps(c.DB)
			inp := testInput(d, NameResetHook, c.Args)

			res, err := resetHook{}.Execute(inp)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, hooks.NotifyHooksChangeCalls(), c.Notify)

			if err != nil {
				return
			}

			// the settings stay as they were; only the state starts over.
			ff := c.DB.UpdateDocumentHookCalls()
			require.Len(t, ff, 1)
			assert.Equal(t, _testHookID, ff[0].Hk.ID)
			assert.JSONEq(t, string(stubHook().Settings), string(ff[0].Hk.Settings))
			assert.NotContains(t, string(ff[0].Hk.State), "2026-01-01")

			row := decodeHookRow(t, res)
			assert.Equal(t, _testHookID, row.ID)
			assert.Equal(t, "100", row.Score.String())

			assert.Equal(t, []Touched{{DocumentID: _testDocID, BranchID: _stubBranchID}}, inp.touched)
		})
	}
}

func Test_deleteHook_Info(t *testing.T) {
	t.Parallel()

	info := deleteHook{}.Info()

	assert.Equal(t, NameDeleteHook, info.Name)
	assert.NotEmpty(t, info.Description)
	assert.Equal(t, []string{"document_id", "hook_id"}, info.Required)
	assert.Contains(t, info.Properties, "hook_id")
}

func Test_deleteHook_Traits(t *testing.T) {
	t.Parallel()

	assert.Equal(t, Traits{Write: true, Destructive: true}, deleteHook{}.Traits())
}

func Test_deleteHook_Title(t *testing.T) {
	t.Parallel()

	got, err := deleteHook{}.Title(testInput(testDeps(stubHookDB(), nil, nil), NameDeleteHook, `{`+hookArgs(_testHookID)+`}`))
	require.NoError(t, err)
	assert.Equal(t, "Deleting a hook on Runbook", got)

	_, err = deleteHook{}.Title(testInput(testDeps(failingDocumentDB(), nil, nil), NameDeleteHook, requiredArgs(t, NameDeleteHook)))
	require.Error(t, err)

	_, err = deleteHook{}.Title(testInput(testDeps(stubHookDB(), nil, nil), NameDeleteHook, `{`))
	require.Error(t, err)
}

func Test_deleteHook_Summary(t *testing.T) {
	t.Parallel()

	got, err := deleteHook{}.Summary(testInput(testDeps(stubHookDB(), nil, nil), NameDeleteHook, `{`+hookArgs(_testHookID)+`}`))
	require.NoError(t, err)
	assert.Equal(t, ActionSummary{
		Tool:         NameDeleteHook,
		DocumentID:   _testDocID,
		DocumentName: "Runbook",
		Summary:      "Delete the Scheduled Reminder hook on Runbook on branch draft",
	}, got)

	_, err = deleteHook{}.Summary(testInput(testDeps(stubHookDB(), nil, nil), NameDeleteHook, `{`+hookArgs(_unknownHookID)+`}`))
	require.Error(t, err)

	_, err = deleteHook{}.Summary(testInput(testDeps(stubHookDB(), nil, nil), NameDeleteHook, `{`))
	require.Error(t, err)
}

func Test_deleteHook_Execute(t *testing.T) {
	t.Parallel()

	failing := stubHookDB()
	failing.DeleteDocumentHookFunc = func(context.Context, xid.ID) error {
		return assert.AnError
	}

	cc := map[string]struct {
		DB     *DBMock
		Args   string
		Notify int
		Err    error
	}{
		"Malformed arguments": {DB: stubHookDB(), Args: `{`, Err: assert.AnError},
		"Hook id is required": {DB: stubHookDB(), Args: `{"document_id":"` + _testDocID.String() + `"}`, Err: assert.AnError},
		"Unknown hook": {
			DB:   stubHookDB(),
			Args: `{` + hookArgs(_unknownHookID) + `}`,
			Err:  fmt.Errorf("delete_hook: %w", fmt.Errorf("hook %s on document %s: %w", _unknownHookID, _testDocID, errUnknownHook)),
		},
		"Error returned by db.DeleteDocumentHook": {
			DB:   failing,
			Args: `{` + hookArgs(_testHookID) + `}`,
			Err:  assert.AnError,
		},
		"Deleted": {
			DB:     stubHookDB(),
			Args:   `{` + hookArgs(_testHookID) + `}`,
			Notify: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			d, hooks := hookDeps(c.DB)
			inp := testInput(d, NameDeleteHook, c.Args)

			res, err := deleteHook{}.Execute(inp)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, hooks.NotifyHooksChangeCalls(), c.Notify)

			if err != nil {
				return
			}

			ff := c.DB.DeleteDocumentHookCalls()
			require.Len(t, ff, 1)
			assert.Equal(t, _testHookID, ff[0].ID)
			assert.JSONEq(t, `{"hook_id":"`+_testHookID.String()+`","deleted":true}`, res)

			// the branch stays, and the editor showing it has to redraw.
			assert.Equal(t, []Touched{{DocumentID: _testDocID, BranchID: _stubBranchID}}, inp.touched)
		})
	}
}

func Test_describeHook(t *testing.T) {
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
		DB     *DBMock
		HookID xid.ID
		Branch string
		Err    error
	}{
		"Unknown hook": {
			DB:     stubHookDB(),
			HookID: _unknownHookID,
			Err:    fmt.Errorf("%s: fetch hook: %w", NameResetHook, fmt.Errorf("hook %s on document %s: %w", _unknownHookID, _testDocID, errUnknownHook)),
		},
		"Error returned by the branch fetch": {
			DB:     failingDoc,
			HookID: _testHookID,
			Err:    assert.AnError,
		},
		"Hook on a branch": {
			DB:     stubHookDB(),
			HookID: _testHookID,
			Branch: _stubBranchName,
		},
		// a hook whose branch is gone is named on the default branch,
		// which the sweep will remove it from regardless.
		"Hook whose branch is gone": {
			DB:     branchless,
			HookID: _testHookID,
			Branch: document.DefaultBranch,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			doc, hk, err := describeHook(testInput(testDeps(c.DB, nil, nil), NameResetHook, `{}`), NameResetHook, hookRefArgs{DocumentID: _testDocID, HookID: c.HookID})
			testutil.AssertEqualError(t, c.Err, err)

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

	assert.Equal(t, "Website Changes hook on Runbook on branch draft", hookLabel(hook.TypeURLWatcher, null.String{}, doc))
	assert.Equal(t, "Website Changes hook on Runbook on branch draft, block b", hookLabel(hook.TypeURLWatcher, null.StringFrom("b"), doc))
}
