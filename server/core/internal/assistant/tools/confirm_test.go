package tools

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/oxynote/oxynote/server/core/internal/assistant/edit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// gatedTool returns the named tool as the registry stored it, wrapper
// and all, so a test exercises what the agent actually runs.
func gatedTool(t *testing.T, s *Set, name Name) tool.InvokableTool {
	t.Helper()

	it, ok := s.tools[name]
	require.True(t, ok, "%s is not in the registry", name)

	return it
}

func Test_confirming_gatesEveryWrite(t *testing.T) {
	t.Parallel()

	s := New(testInput())

	for _, name := range allToolNames() {
		_, gated := s.tools[name].(*confirming)
		_, writes := unwrap(s.tools[name]).(Confirmer)

		// the gate and the write classification are the same fact, so
		// neither can be true without the other.
		assert.Equal(t, writes, gated, "%s", name)
	}
}

func Test_confirming_destructiveIgnoresAutoApprove(t *testing.T) {
	t.Parallel()

	s := New(testInput())

	destructive := map[Name]bool{NameDeleteDocument: true, NameDeleteBlock: true}

	for _, name := range allToolNames() {
		c, ok := s.tools[name].(*confirming)
		if !ok {
			continue
		}

		// approving a batch of text edits is not consent to delete.
		assert.Equal(t, destructive[name], c.destructive, "%s", name)
	}
}

func Test_confirming_InvokableRun(t *testing.T) {
	t.Parallel()

	// every write tool must ask before it acts. Resuming an approved
	// turn reruns the tool from the top, so a tool that applied its
	// edit before interrupting would apply it a second time.
	cc := map[string]struct {
		Name Name
		Args string
	}{
		"Insert block asks first":      {Name: NameInsertBlock, Args: `{"document_id":"d","block_uid":"b"}`},
		"Delete block asks first":      {Name: NameDeleteBlock, Args: `{"document_id":"d","block_uid":"b"}`},
		"Delete document asks first":   {Name: NameDeleteDocument, Args: `{"document_id":"d"}`},
		"Rename document asks first":   {Name: NameRenameDocument, Args: `{"document_id":"d","name":"n"}`},
		"Create document asks first":   {Name: NameCreateDocument, Args: `{"name":"n"}`},
		"Update block text asks first": {Name: NameUpdateBlockText, Args: `{"document_id":"d","block_uid":"b"}`},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			applier := &EditApplierMock{
				ApplyFunc: func(_ context.Context, _, _ string, _ []edit.Operation) (edit.Result, error) {
					require.FailNow(t, "the tool applied an edit before the user approved it")

					return edit.Result{}, nil
				},
			}

			inp := &Input{
				log:     slog.New(slog.DiscardHandler),
				db:      &DBMock{},
				applier: applier,
				orgID:   "org",
			}

			ct := gatedTool(t, New(inp), c.Name)

			res, err := ct.InvokableRun(context.Background(), c.Args)
			require.Error(t, err)
			assert.Empty(t, res)

			info, ok := compose.IsInterruptRerunError(err)
			require.True(t, ok, "a pending write must interrupt the run")

			summary, ok := info.(ConfirmActionSummary)
			require.True(t, ok, "the interrupt must carry a confirm summary, got %T", info)
			assert.Equal(t, string(c.Name), summary.Tool)

			assert.Empty(t, applier.ApplyCalls())
		})
	}
}

func Test_declinedResult(t *testing.T) {
	t.Parallel()

	res, err := declinedResult()
	require.NoError(t, err)

	var out struct {
		Approved bool   `json:"approved"`
		Reason   string `json:"reason"`
	}

	require.NoError(t, json.Unmarshal([]byte(res), &out))
	assert.False(t, out.Approved)
	assert.NotEmpty(t, out.Reason)
}

func Test_autoApproved(t *testing.T) {
	t.Parallel()

	// outside an agent run there is no session, so nothing is
	// auto-approved and the gate always prompts.
	assert.False(t, autoApproved(context.Background()))
}

func Test_Destructive(t *testing.T) {
	t.Parallel()

	s := New(testInput())

	// the marker is a method, so calling it is what proves the tool
	// carries it rather than merely satisfying the interface by name.
	d, ok := unwrap(s.tools[NameDeleteDocument]).(Destructive)
	require.True(t, ok)
	d.Destructive()

	d, ok = unwrap(s.tools[NameDeleteBlock]).(Destructive)
	require.True(t, ok)
	d.Destructive()

	// an ordinary write is not destructive, so approve-all covers it.
	_, ok = unwrap(s.tools[NameUpdateBlockText]).(Destructive)
	assert.False(t, ok)
}

func Test_SetAutoApprove(t *testing.T) {
	t.Parallel()

	// outside a run there is no session to record the answer in, so the
	// call is a no-op and the next write still asks.
	ctx := context.Background()

	SetAutoApprove(ctx)
	assert.False(t, autoApproved(ctx))
}
