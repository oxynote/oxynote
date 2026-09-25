package history

import (
	"testing"
	"time"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/document/hook"
	"github.com/oxynote/oxynote/server/core/internal/document/hook/processor"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func Test_NewEntry(t *testing.T) {
	t.Parallel()

	doc := document.Document{
		ID:             xid.New(),
		OrganizationID: "org-1",
		BranchID:       xid.New(),
		BranchName:     document.DefaultBranch,
		DocumentName:   "Runbook",
		Icon:           "📘",
		Content: document.RootBlock{
			Type: "doc",
			Content: []document.Block{
				{Type: document.BlockNodeParagraph, Text: "Hello"},
			},
		},
	}
	at := time.Date(2026, 1, 1, 10, 44, 30, 0, time.UTC)
	hooks := Hooks{
		{
			Type:     hook.TypeURLWatcher,
			BlockID:  null.StringFrom("b1"),
			Settings: processor.Settings(`{"url":"https://example.com"}`),
		},
	}

	entry := NewEntry(doc, at, null.StringFrom("user-1"), hooks, true)

	assert.False(t, entry.ID.IsNil())
	assert.Equal(t, doc.ID, entry.DocumentID)
	assert.Equal(t, doc.BranchID, entry.BranchID)
	assert.Equal(t, doc.DocumentName, entry.DocumentName)
	assert.Equal(t, doc.Icon, entry.Icon)
	assert.Equal(t, doc.Content, entry.Content)
	assert.Equal(t, hooks, entry.Hooks)
	assert.Equal(t, null.StringFrom("user-1"), entry.LastUpdatedBy)
	assert.True(t, entry.Boundary)
	assert.Equal(t, at, entry.CreatedAt)
	assert.Equal(t, at, entry.UpdatedAt)

	// every call mints its own id.
	assert.NotEqual(t, entry.ID, NewEntry(doc, at, null.String{}, nil, false).ID)
}

func Test_NewHooks(t *testing.T) {
	t.Parallel()

	content := document.RootBlock{
		Type: "doc",
		Content: []document.Block{
			{Type: document.BlockNodeParagraph, Attrs: document.Attributes{document.AttrUID: "b1"}},
		},
	}

	assert.Equal(t, Hooks{}, NewHooks(content, nil))
	assert.Equal(t, Hooks{
		{
			Type:     hook.TypeURLWatcher,
			BlockID:  null.StringFrom("b1"),
			Settings: processor.Settings(`{"url":"https://example.com"}`),
		},
		{
			Type:     hook.TypeScheduledReminder,
			Settings: processor.Settings(`{"cron":"0 9 * * 1"}`),
		},
	}, NewHooks(content, []hook.Hook{
		{
			ID:       xid.New(),
			Type:     hook.TypeURLWatcher,
			BlockID:  null.StringFrom("b1"),
			Settings: processor.Settings(`{"url":"https://example.com"}`),
			State:    null.ValueFrom(processor.State(`{"watcher":"w1"}`)),
			// the block is back, so the sweep lifts the mark on its next
			// run.
			SoftDeletedAt: null.TimeFrom(time.Now()),
		},
		{
			ID:       xid.New(),
			Type:     hook.TypeURLWatcher,
			BlockID:  null.StringFrom("removed"),
			Settings: processor.Settings(`{"url":"https://example.org"}`),
		},
		{
			ID:       xid.New(),
			Type:     hook.TypeScheduledReminder,
			Settings: processor.Settings(`{"cron":"0 9 * * 1"}`),
		},
	}))
}

func Test_Hooks_Value(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Hooks Hooks
		JSON  string
	}{
		"Nil hooks": {
			JSON: `[]`,
		},
		"Hooks": {
			Hooks: Hooks{
				{
					Type:     hook.TypeURLWatcher,
					BlockID:  null.StringFrom("b1"),
					Settings: processor.Settings(`{"url":"https://example.com"}`),
				},
				{
					Type:     hook.TypeScheduledReminder,
					Settings: processor.Settings(`{"cron":"0 9 * * 1"}`),
				},
			},
			JSON: `[{"type":"url-watcher","blockId":"b1","settings":{"url":"https://example.com"}},` +
				`{"type":"scheduled-reminder","blockId":null,"settings":{"cron":"0 9 * * 1"}}]`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			v, err := c.Hooks.Value()
			require.NoError(t, err)

			data, ok := v.([]byte)
			require.True(t, ok)
			assert.JSONEq(t, c.JSON, string(data))
		})
	}
}

func Test_Hooks_Scan(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Src    any
		Result Hooks
		Err    error
	}{
		"Invalid type": {
			Src: 42,
			Err: assert.AnError,
		},
		"Invalid JSON": {
			Src: []byte(`{`),
			Err: assert.AnError,
		},
		"Bytes": {
			Src: []byte(`[{"type":"url-watcher","blockId":"b1","settings":{"url":"https://example.com"}}]`),
			Result: Hooks{
				{
					Type:     hook.TypeURLWatcher,
					BlockID:  null.StringFrom("b1"),
					Settings: processor.Settings(`{"url":"https://example.com"}`),
				},
			},
		},
		"String": {
			Src: `[{"type":"scheduled-reminder","blockId":null,"settings":{}}]`,
			Result: Hooks{
				{
					Type:     hook.TypeScheduledReminder,
					Settings: processor.Settings(`{}`),
				},
			},
		},
		"Empty list": {
			Src:    []byte(`[]`),
			Result: Hooks{},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var hh Hooks

			err := hh.Scan(c.Src)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, hh)
		})
	}
}
