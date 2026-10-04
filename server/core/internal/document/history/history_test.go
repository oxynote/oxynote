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

func Test_Entry_Sum(t *testing.T) {
	t.Parallel()

	base := NewEntry(
		document.Document{
			ID:           xid.New(),
			BranchID:     xid.New(),
			DocumentName: "Runbook",
			Icon:         "📘",
			Content: document.RootBlock{
				Type: "doc",
				Content: []document.Block{
					{Type: document.BlockNodeParagraph, Text: "Hello"},
				},
			},
		},
		time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC),
		null.StringFrom("user-1"),
		Hooks{
			{
				Type:     hook.TypeURLWatcher,
				BlockID:  null.StringFrom("b1"),
				Settings: processor.Settings(`{"url":"https://example.com"}`),
			},
		},
		false,
	)

	baseSum, err := base.Sum()
	require.NoError(t, err)

	cc := map[string]struct {
		Edit func(e *Entry)
		Same bool
		Err  error
	}{
		"Other name": {
			Edit: func(e *Entry) { e.DocumentName = "Playbook" },
		},
		"Other icon": {
			Edit: func(e *Entry) { e.Icon = "📕" },
		},
		"Other content": {
			Edit: func(e *Entry) { e.Content.Content = []document.Block{{Type: document.BlockNodeParagraph, Text: "Bye"}} },
		},
		"Other hooks": {
			Edit: func(e *Entry) { e.Hooks = Hooks{} },
		},
		"Fields outside the snapshot": {
			Edit: func(e *Entry) {
				e.ID = xid.New()
				e.Checksum = "stale"
				e.LastUpdatedBy = null.String{}
				e.CreatedAt = e.CreatedAt.Add(time.Hour)
				e.UpdatedAt = e.UpdatedAt.Add(time.Hour)
				e.Boundary = true
			},
			Same: true,
		},
		"Spacing in hook settings": {
			Edit: func(e *Entry) {
				e.Hooks = Hooks{{
					Type:     hook.TypeURLWatcher,
					BlockID:  null.StringFrom("b1"),
					Settings: processor.Settings(`{ "url": "https://example.com" }`),
				}}
			},
			Same: true,
		},
		"Invalid hook settings": {
			Edit: func(e *Entry) {
				e.Hooks = Hooks{{Type: hook.TypeURLWatcher, Settings: processor.Settings(`{`)}}
			},
			Err: assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			entry := base
			c.Edit(&entry)

			sum, err := entry.Sum()
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Len(t, sum, 64)
			assert.Equal(t, c.Same, sum == baseSum)
		})
	}
}

func Test_NewHooks(t *testing.T) {
	t.Parallel()

	doc := document.Document{
		Content: document.RootBlock{
			Type: "doc",
			Content: []document.Block{
				{Type: document.BlockNodeParagraph, Attrs: document.Attributes{document.AttrUID: "b1"}},
			},
		},
	}

	cc := map[string]struct {
		Hooks  []hook.Hook
		Result Hooks
	}{
		"Nil hooks": {
			Result: Hooks{},
		},
		"Document level hook is kept": {
			Hooks: []hook.Hook{
				{
					ID:       xid.New(),
					Type:     hook.TypeScheduledReminder,
					Settings: processor.Settings(`{"cron":"0 9 * * 1"}`),
				},
			},
			Result: Hooks{
				{
					Type:     hook.TypeScheduledReminder,
					Settings: processor.Settings(`{"cron":"0 9 * * 1"}`),
				},
			},
		},
		"Hook whose block is present is kept": {
			Hooks: []hook.Hook{
				{
					ID:       xid.New(),
					Type:     hook.TypeURLWatcher,
					BlockID:  null.StringFrom("b1"),
					Settings: processor.Settings(`{"url":"https://example.com"}`),
					State:    null.ValueFrom(processor.State(`{"watcher":"w1"}`)),
				},
			},
			Result: Hooks{
				{
					Type:     hook.TypeURLWatcher,
					BlockID:  null.StringFrom("b1"),
					Settings: processor.Settings(`{"url":"https://example.com"}`),
				},
			},
		},
		"Soft deleted hook whose block is present is kept": {
			Hooks: []hook.Hook{
				{
					ID:            xid.New(),
					Type:          hook.TypeURLWatcher,
					BlockID:       null.StringFrom("b1"),
					Settings:      processor.Settings(`{"url":"https://example.com"}`),
					SoftDeletedAt: null.TimeFrom(time.Now()),
				},
			},
			Result: Hooks{
				{
					Type:     hook.TypeURLWatcher,
					BlockID:  null.StringFrom("b1"),
					Settings: processor.Settings(`{"url":"https://example.com"}`),
				},
			},
		},
		"Hook whose block is absent is dropped": {
			Hooks: []hook.Hook{
				{
					ID:       xid.New(),
					Type:     hook.TypeURLWatcher,
					BlockID:  null.StringFrom("removed"),
					Settings: processor.Settings(`{"url":"https://example.org"}`),
				},
			},
			Result: Hooks{},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, NewHooks(doc, c.Hooks))
		})
	}
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
