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

	assert.Equal(t, _typeObject, got[_keyType])

	// every setting of every type is published, so a client can fill any
	// of them without the prose, and every one names a type that owns it.
	props, ok := got["properties"].(map[string]any)
	require.True(t, ok)

	for _, key := range []string{_keySchedule, _keyRepository, _keyBranch, _keyPaths, _keyURL, _keyImage} {
		assert.Contains(t, props, key)

		_, owned := hookSettingOwner(key)
		assert.True(t, owned, key)
	}
}

func Test_decodeHookSettings(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Type   hook.Type
		Raw    string
		Result string
		Err    error
	}{
		"Unknown type": {Type: "bogus", Raw: `{}`, Err: fmt.Errorf("type %q: %w", "bogus", hook.ErrInvalidType)},
		"Malformed settings": {
			Type: hook.TypeScheduledReminder,
			Raw:  `{`,
			Err:  assert.AnError,
		},
		// the editor labels a linear reminder with a custom duration as
		// a date, which is what a tool call sets.
		"Scheduled reminder": {
			Type:   hook.TypeScheduledReminder,
			Raw:    `{"schedule":"2030-01-01T00:00:00Z"}`,
			Result: `{"scale":"linear","duration":"custom","schedule":"2030-01-01T00:00:00Z"}`,
		},
		"Scheduled reminder without a schedule": {
			Type: hook.TypeScheduledReminder,
			Raw:  `{}`,
			Err:  errRequired(_keySchedule),
		},
		"Scheduled reminder without settings": {
			Type: hook.TypeScheduledReminder,
			Err:  errRequired(_keySchedule),
		},
		"GitHub tracking": {
			Type:   hook.TypeGithubTracking,
			Raw:    `{"repository":"o/r","branch":"main","paths":["a","b"]}`,
			Result: `{"repository":"o/r","branch":"main","paths":["a","b"]}`,
		},
		"GitHub tracking without a repository": {
			Type: hook.TypeGithubTracking,
			Raw:  `{"branch":"main","paths":["a"]}`,
			Err:  errRequired(_keyRepository),
		},
		"GitHub tracking without a branch": {
			Type: hook.TypeGithubTracking,
			Raw:  `{"repository":"o/r","paths":["a"]}`,
			Err:  errRequired(_keyBranch),
		},
		"GitHub tracking without paths": {
			Type: hook.TypeGithubTracking,
			Raw:  `{"repository":"o/r","branch":"main","paths":[]}`,
			Err:  errRequired(_keyPaths),
		},
		"URL watcher": {
			Type:   hook.TypeURLWatcher,
			Raw:    `{"url":"https://example.com"}`,
			Result: `{"url":"https://example.com"}`,
		},
		"URL watcher without a url": {
			Type: hook.TypeURLWatcher,
			Raw:  `{}`,
			Err:  errRequired(_keyURL),
		},
		"Container image watcher": {
			Type:   hook.TypeContainerImageWatcher,
			Raw:    `{"image":"nginx:1"}`,
			Result: `{"image":"nginx:1"}`,
		},
		"Container image watcher without an image": {
			Type: hook.TypeContainerImageWatcher,
			Raw:  `{}`,
			Err:  errRequired(_keyImage),
		},
		// another type's field is refused naming the type it belongs to.
		"Scheduled reminder with a url": {
			Type: hook.TypeScheduledReminder,
			Raw:  `{"schedule":"2030-01-01T00:00:00Z","url":"https://example.com"}`,
			Err:  fmt.Errorf("%s applies to %s only", _keyURL, hook.TypeURLWatcher),
		},
		"URL watcher with github paths": {
			Type: hook.TypeURLWatcher,
			Raw:  `{"url":"https://example.com","paths":["a"]}`,
			Err:  fmt.Errorf("%s applies to %s only", _keyPaths, hook.TypeGithubTracking),
		},
		"Container image watcher with a schedule": {
			Type: hook.TypeContainerImageWatcher,
			Raw:  `{"image":"nginx:1","schedule":"2030-01-01T00:00:00Z"}`,
			Err:  fmt.Errorf("%s applies to %s only", _keySchedule, hook.TypeScheduledReminder),
		},
		"Setting of no type": {
			Type: hook.TypeURLWatcher,
			Raw:  `{"url":"https://example.com","colour":"red"}`,
			Err:  fmt.Errorf("%s is not a hook setting", "colour"),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			got, err := decodeHookSettings(c.Type, json.RawMessage(c.Raw))
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				assert.Nil(t, got)
				return
			}

			assert.JSONEq(t, c.Result, string(got))
		})
	}
}

func Test_decodeSettings(t *testing.T) {
	t.Parallel()

	var uw processor.URLWatcher

	// an absent payload decodes as an empty one.
	require.NoError(t, decodeSettings(nil, &uw))
	assert.Empty(t, uw.URL)

	require.NoError(t, decodeSettings(json.RawMessage(`{"url":"https://example.com"}`), &uw))
	assert.Equal(t, "https://example.com", uw.URL)

	err := decodeSettings(json.RawMessage(`{"image":"nginx:1"}`), &uw)
	assert.Equal(t, fmt.Errorf("%s applies to %s only", _keyImage, hook.TypeContainerImageWatcher), err)

	err = decodeSettings(json.RawMessage(`{"colour":"red"}`), &uw)
	assert.Equal(t, fmt.Errorf("%s is not a hook setting", "colour"), err)

	// a value of the wrong shape is reported with its path.
	err = decodeSettings(json.RawMessage(`{"url":1}`), &uw)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid settings")

	require.Error(t, decodeSettings(json.RawMessage(`{`), &uw))
}

func Test_hookSettingOwner(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Key    string
		Result hook.Type
		Owned  bool
	}{
		"Schedule":   {Key: _keySchedule, Result: hook.TypeScheduledReminder, Owned: true},
		"Repository": {Key: _keyRepository, Result: hook.TypeGithubTracking, Owned: true},
		"Branch":     {Key: _keyBranch, Result: hook.TypeGithubTracking, Owned: true},
		"Paths":      {Key: _keyPaths, Result: hook.TypeGithubTracking, Owned: true},
		"URL":        {Key: _keyURL, Result: hook.TypeURLWatcher, Owned: true},
		"Image":      {Key: _keyImage, Result: hook.TypeContainerImageWatcher, Owned: true},
		"Unknown":    {Key: "colour"},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			got, owned := hookSettingOwner(c.Key)
			assert.Equal(t, c.Owned, owned)
			assert.Equal(t, c.Result, got)
		})
	}
}

func Test_listHooksArgs_Validate(t *testing.T) {
	t.Parallel()

	assertValidate(t,
		listHooksArgs{docTarget{DocumentID: _testDocID, BranchID: _stubMainBranchID}},
		map[string]Args{
			_keyDocumentID: listHooksArgs{docTarget{BranchID: _stubMainBranchID}},
			_keyBranchID:   listHooksArgs{docTarget{DocumentID: _testDocID}},
		},
	)
}

func Test_listHooks_Info(t *testing.T) {
	t.Parallel()

	info := listHooks{}.Info()

	assert.Equal(t, NameListHooks, info.Name)
	assert.NotEmpty(t, info.Description)
	assert.Equal(t, []string{_keyDocumentID, _keyBranchID}, info.Required)
	assert.Contains(t, info.Properties, _keyBranchID)
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

	scheduled := json.RawMessage(`{"schedule":"` + _stubSchedule + `"}`)

	ok := createHookArgs{
		DocumentID: _testDocID, BranchID: _stubMainBranchID,
		Type:     hook.TypeScheduledReminder,
		Settings: scheduled,
	}

	assertValidate(t, ok, map[string]Args{
		_keyDocumentID: createHookArgs{
			docTarget: docTarget{BranchID: _stubMainBranchID},
			Type:      hook.TypeScheduledReminder,
			Settings:  scheduled,
		},
		_keyBranchID: createHookArgs{
			docTarget: docTarget{DocumentID: _testDocID},
			Type:      hook.TypeScheduledReminder,
			Settings:  scheduled,
		},
		_keyType: createHookArgs{
			docTarget: ok.docTarget,
			Settings:  scheduled,
		},
		_keySchedule: createHookArgs{
			docTarget: ok.docTarget,
			Type:      hook.TypeScheduledReminder,
		},
	})

	// an unknown type is refused before its settings are looked at.
	err := createHookArgs{docTarget: ok.docTarget, Type: "bogus"}.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), `type "bogus"`)

	// another type's field is refused naming the type it belongs to.
	err = createHookArgs{docTarget: ok.docTarget, Type: hook.TypeURLWatcher, Settings: json.RawMessage(`{"url":"https://example.com","image":"x"}`)}.Validate()
	require.Error(t, err)
	assert.Equal(t, fmt.Errorf("%s applies to %s only", _keyImage, hook.TypeContainerImageWatcher), err)
}

func Test_createHook_Info(t *testing.T) {
	t.Parallel()

	info := createHook{}.Info()

	assert.Equal(t, NameCreateHook, info.Name)
	assert.NotEmpty(t, info.Description)
	assert.Equal(t, []string{_keyDocumentID, _keyBranchID, _keyType, _keySettings}, info.Required)

	for _, key := range []string{_keyDocumentID, _keyBranchID, _keyType, _keyBlockUID, _keySettings} {
		assert.Contains(t, info.Properties, key)
	}

	assert.Equal(t, hookSettingsSchema(), info.Properties[_keySettings])

	// the type is an enum of the four hook types, so a client can refuse
	// a bad one itself.
	assert.Equal(t, hookTypes(), info.Properties[_keyType].(map[string]any)[_keyEnum])

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

	scheduled := `,"type":"scheduled-reminder","settings":{"schedule":"` + _stubSchedule + `"}`

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

	scheduled := `,"type":"scheduled-reminder","settings":{"schedule":"` + _stubSchedule + `"}`

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

	scheduled := `"type":"scheduled-reminder","settings":{"schedule":"` + _stubSchedule + `"}`

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
		"Type is required":    {DB: stubHookDB(), Args: `{` + targetArgs(_stubBranchID) + `}`, Err: assert.AnError},
		"Schedule is required": {
			DB:   stubHookDB(),
			Args: `{` + targetArgs(_stubBranchID) + `,"type":"scheduled-reminder","settings":{}}`,
			Err:  assert.AnError,
		},
		"Another type's field is refused": {
			DB:   stubHookDB(),
			Args: `{` + targetArgs(_stubBranchID) + `,"type":"scheduled-reminder","settings":{"schedule":"` + _stubSchedule + `","url":"https://example.com"}}`,
			Err:  fmt.Errorf("%s: %w", NameCreateHook, fmt.Errorf("%s applies to %s only", _keyURL, hook.TypeURLWatcher)),
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
			Args: `{` + targetArgs(_stubBranchID) + `,"type":"github-tracking","settings":{"repository":"o/r","branch":"main","paths":["a"]}}`,
			Err:  fmt.Errorf("create_hook: %w", fmt.Errorf("%s: %w", hook.TypeGithubTracking, github.ErrNotConfigured)),
		},
		"URL watcher without changedetection": {
			DB:   stubHookDB(),
			Args: `{` + targetArgs(_stubBranchID) + `,"type":"url-watcher","settings":{"url":"https://example.com"}}`,
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

	settings := json.RawMessage(`{}`)

	// the settings are checked against the hook's type later, so any
	// payload passes here.
	assertValidate(t,
		updateHookArgs{DocumentID: _testDocID, HookID: _testHookID, Settings: settings},
		map[string]Args{
			_keyDocumentID: updateHookArgs{hookRefArgs: hookRefArgs{HookID: _testHookID}, Settings: settings},
			_keyHookID:     updateHookArgs{hookRefArgs: hookRefArgs{DocumentID: _testDocID}, Settings: settings},
			_keySettings:   updateHookArgs{hookRefArgs: hookRefArgs{DocumentID: _testDocID, HookID: _testHookID}},
		},
	)
}

func Test_updateHook_Info(t *testing.T) {
	t.Parallel()

	info := updateHook{}.Info()

	assert.Equal(t, NameUpdateHook, info.Name)
	assert.NotEmpty(t, info.Description)
	assert.Equal(t, []string{_keyDocumentID, _keyHookID, _keySettings}, info.Required)

	for _, key := range []string{_keyDocumentID, _keyHookID, _keySettings} {
		assert.Contains(t, info.Properties, key)
	}

	assert.Equal(t, hookSettingsSchema(), info.Properties[_keySettings])
	assert.NotContains(t, info.Properties, _keyType)
}

func Test_updateHook_Traits(t *testing.T) {
	t.Parallel()

	assert.Equal(t, Traits{Write: true}, updateHook{}.Traits())
}

func Test_updateHook_Title(t *testing.T) {
	t.Parallel()

	got, err := updateHook{}.Title(testInput(testDeps(stubHookDB(), nil, nil), NameUpdateHook, `{`+hookArgs(_testHookID)+`,"settings":{}}`))
	require.NoError(t, err)
	assert.Equal(t, "Updating a hook on Runbook", got)

	_, err = updateHook{}.Title(testInput(testDeps(failingDocumentDB(), nil, nil), NameUpdateHook, requiredArgs(t, NameUpdateHook)))
	require.Error(t, err)

	_, err = updateHook{}.Title(testInput(testDeps(stubHookDB(), nil, nil), NameUpdateHook, `{`))
	require.Error(t, err)
}

func Test_updateHook_Summary(t *testing.T) {
	t.Parallel()

	got, err := updateHook{}.Summary(testInput(testDeps(stubHookDB(), nil, nil), NameUpdateHook, `{`+hookArgs(_testHookID)+`,"settings":{"schedule":"`+_stubSchedule+`"}}`))
	require.NoError(t, err)
	assert.Equal(t, ActionSummary{
		Tool:         NameUpdateHook,
		DocumentID:   _testDocID,
		DocumentName: "Runbook",
		Summary:      "Update the Scheduled Reminder hook on Runbook on branch draft",
	}, got)

	// settings of another type are refused here, so the user is not
	// asked to approve a call that cannot run.
	_, err = updateHook{}.Summary(testInput(testDeps(stubHookDB(), nil, nil), NameUpdateHook, `{`+hookArgs(_testHookID)+`,"settings":{"url":"https://example.com"}}`))
	require.Error(t, err)
	assert.Equal(t, fmt.Errorf("%s: %w", NameUpdateHook, fmt.Errorf("%s applies to %s only", _keyURL, hook.TypeURLWatcher)), err)

	_, err = updateHook{}.Summary(testInput(testDeps(stubHookDB(), nil, nil), NameUpdateHook, `{`+hookArgs(_unknownHookID)+`,"settings":{"schedule":"`+_stubSchedule+`"}}`))
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
			Args: `{` + hookArgs(_unknownHookID) + `,"settings":{"schedule":"` + _stubSchedule + `"}}`,
			Err:  fmt.Errorf("update_hook: %w", fmt.Errorf("hook %s on document %s: %w", _unknownHookID, _testDocID, errUnknownHook)),
		},
		"Settings are required": {
			DB:   stubHookDB(),
			Args: `{` + hookArgs(_testHookID) + `}`,
			Err:  fmt.Errorf("%s: %w", NameUpdateHook, errRequired(_keySettings)),
		},
		"Schedule is required for a scheduled reminder": {
			DB:   stubHookDB(),
			Args: `{` + hookArgs(_testHookID) + `,"settings":{}}`,
			Err:  fmt.Errorf("update_hook: %w", errRequired(_keySchedule)),
		},
		"Another type's field is refused": {
			DB:   stubHookDB(),
			Args: `{` + hookArgs(_testHookID) + `,"settings":{"schedule":"` + _stubSchedule + `","image":"nginx:1"}}`,
			Err:  fmt.Errorf("update_hook: %w", fmt.Errorf("%s applies to %s only", _keyImage, hook.TypeContainerImageWatcher)),
		},
		"Error returned by db.UpdateDocumentHook": {
			DB:   failing,
			Args: `{` + hookArgs(_testHookID) + `,"settings":{"schedule":"` + later.Format(time.RFC3339) + `"}}`,
			Err:  assert.AnError,
		},
		"Updated": {
			DB:     stubHookDB(),
			Args:   `{` + hookArgs(_testHookID) + `,"settings":{"schedule":"` + later.Format(time.RFC3339) + `"}}`,
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
			_keyDocumentID: hookRefArgs{HookID: _testHookID},
			_keyHookID:     hookRefArgs{DocumentID: _testDocID},
		},
	)
}

func Test_hookRefProps(t *testing.T) {
	t.Parallel()

	got := hookRefProps()

	assert.Contains(t, got, _keyDocumentID)
	assert.Contains(t, got, _keyHookID)
}

func Test_resetHook_Info(t *testing.T) {
	t.Parallel()

	info := resetHook{}.Info()

	assert.Equal(t, NameResetHook, info.Name)
	assert.NotEmpty(t, info.Description)
	assert.Equal(t, []string{_keyDocumentID, _keyHookID}, info.Required)
	assert.Contains(t, info.Properties, _keyHookID)
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
	assert.Equal(t, []string{_keyDocumentID, _keyHookID}, info.Required)
	assert.Contains(t, info.Properties, _keyHookID)
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

func Test_hookTypes(t *testing.T) {
	t.Parallel()

	got := hookTypes()
	require.Len(t, got, 4)

	// every published type is one the domain accepts.
	for _, v := range got {
		require.NoError(t, hook.Type(v).Validate())
	}
}
