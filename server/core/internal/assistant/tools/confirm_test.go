package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// gateCase is one Test_confirming_InvokableRun scenario.
type gateCase struct {
	// Name selects the gated tool under test.
	Name Name

	// Args is the JSON argument payload handed to the tool.
	Args string

	// AutoApprove runs the tool under a session carrying an
	// approve-all answer.
	AutoApprove bool

	// Decision answers the pending confirmation. Nil leaves it
	// unanswered.
	Decision *Decision

	// ResumeOther resumes the run without targeting this
	// confirmation.
	ResumeOther bool

	// Interrupted expects the final outcome to be a pending
	// confirmation.
	Interrupted bool

	// Rejected, when set, expects the gate to refuse the call outright
	// instead of parking it, because the arguments cannot be read, and
	// is the text the refusal must carry.
	Rejected string

	// RespJSON is the final tool result when the run completes.
	RespJSON string

	// FetchDocumentCalls is the number of default branch reads.
	FetchDocumentCalls int

	// FetchDocumentByBranchIDCalls is the number of branch reads.
	FetchDocumentByBranchIDCalls int

	// FetchDocumentTreeCalls is the number of document tree reads.
	FetchDocumentTreeCalls int

	// ApplyCalls is the number of edits that must have reached the
	// applier.
	ApplyCalls int
}

func Test_confirming_InvokableRun(t *testing.T) {
	t.Parallel()

	docID := _testDocID
	writeArgs := `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"a","content":"<p id=\"a\">hi</p>"}`
	deleteArgs := `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"a"}`

	// every write tool must ask before it acts. Resuming an approved
	// turn reruns the tool from the top, so a tool that applied its
	// edit before interrupting would apply it a second time.
	cc := map[string]gateCase{
		"Insert blocks asks first": {
			Name:                         NameInsertBlocks,
			Args:                         `{` + targetArgs(_stubMainBranchID) + `,"reference_block_uid":"b","position":"after","content":"<p>x</p>"}`,
			Interrupted:                  true,
			FetchDocumentByBranchIDCalls: 1,
		},
		"Delete block asks first": {
			Name:                         NameDeleteBlock,
			Args:                         deleteArgs,
			Interrupted:                  true,
			FetchDocumentByBranchIDCalls: 1,
		},
		"Delete document asks first": {
			Name:                   NameDeleteDocument,
			Args:                   `{"document_id":"` + docID.String() + `"}`,
			Interrupted:            true,
			FetchDocumentCalls:     1,
			FetchDocumentTreeCalls: 1,
		},
		"Update document asks first": {
			Name:               NameUpdateDocument,
			Args:               `{"document_id":"` + docID.String() + `","name":"n"}`,
			Interrupted:        true,
			FetchDocumentCalls: 1,
		},
		"Create document asks first": {Name: NameCreateDocument, Args: `{"name":"n"}`, Interrupted: true},
		"Replace blocks asks first": {
			Name:                         NameReplaceBlocks,
			Args:                         writeArgs,
			Interrupted:                  true,
			FetchDocumentByBranchIDCalls: 1,
		},
		"Approve-all covers a later non-destructive write": {
			Name:                         NameReplaceBlocks,
			Args:                         writeArgs,
			AutoApprove:                  true,
			RespJSON:                     `{"content":"<p id=\"a\">hi</p>\n"}`,
			FetchDocumentByBranchIDCalls: 1,
			ApplyCalls:                   1,
		},
		"Approve-all never covers a destructive write": {
			Name:                         NameDeleteBlock,
			Args:                         deleteArgs,
			AutoApprove:                  true,
			Interrupted:                  true,
			FetchDocumentByBranchIDCalls: 1,
		},
		"Approved confirmation reruns the tool": {
			Name:                         NameReplaceBlocks,
			Args:                         writeArgs,
			Decision:                     &Decision{Approved: true},
			RespJSON:                     `{"content":"<p id=\"a\">hi</p>\n"}`,
			FetchDocumentByBranchIDCalls: 2,
			ApplyCalls:                   1,
		},
		"Declined confirmation reports the refusal": {
			Name:                         NameReplaceBlocks,
			Args:                         writeArgs,
			Decision:                     &Decision{Approved: false},
			RespJSON:                     `{"approved":false,"reason":"user declined"}`,
			FetchDocumentByBranchIDCalls: 1,
		},
		"Resume of another interrupt point re-interrupts": {
			Name:                         NameReplaceBlocks,
			Args:                         writeArgs,
			ResumeOther:                  true,
			Interrupted:                  true,
			FetchDocumentByBranchIDCalls: 2,
		},
		"Unreadable arguments are refused and never confirmed": {
			Name:     NameReplaceBlocks,
			Args:     `{`,
			Rejected: "replace_blocks: invalid input",
		},
		"Missing arguments are refused and never confirmed": {
			Name:     NameReplaceBlocks,
			Args:     `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"b"}`,
			Rejected: "replace_blocks: content is required",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := stubContentDB(nil)
			applier := stubApplier()

			// the tool as the registry stored it, wrapper and all, so the
			// test exercises what the agent actually runs.
			ct, ok := New(testDeps(db, applier, nil)).tools[c.Name]
			require.True(t, ok, "%s is not in the registry", c.Name)

			var (
				res string
				err error
			)

			switch {
			case c.Rejected != "":
				// the same payload would be rejected by the tool on
				// resume, so the run must not spend a confirmation on
				// it. The rejection is the call's result, not an
				// error: an error would end the turn, and the payload
				// is the model's to fix.
				res, err = ct.InvokableRun(context.Background(), c.Args)
				require.NoError(t, err, "bad arguments must not end the turn")
				assert.Contains(t, res, c.Rejected)
			case c.AutoApprove:
				// the approve-all answer travels as a session value of an
				// adk run.
				ag := &sessionAgent{tool: ct, args: c.Args}

				iter := adk.NewRunner(context.Background(), adk.RunnerConfig{Agent: ag}).Run(
					context.Background(),
					[]adk.Message{schema.UserMessage("go")},
					adk.WithSessionValues(map[string]any{SessionKeyAutoApprove: true}),
				)

				for {
					if _, ok := iter.Next(); !ok {
						break
					}
				}

				res, err = ag.res, ag.err
			default:
				// a one-node checkpointed graph is the smallest host that
				// can park the gate on its interrupt and resume it with the
				// case's answer.
				g := compose.NewGraph[string, string]()
				require.NoError(t, g.AddLambdaNode("gate", compose.InvokableLambda(
					func(ctx context.Context, _ string) (string, error) {
						return ct.InvokableRun(ctx, c.Args)
					},
				)))
				require.NoError(t, g.AddEdge(compose.START, "gate"))
				require.NoError(t, g.AddEdge("gate", compose.END))

				r, cerr := g.Compile(
					context.Background(),
					compose.WithCheckPointStore(&memStore{m: map[string][]byte{}}),
				)
				require.NoError(t, cerr)

				_, err = r.Invoke(context.Background(), "go", compose.WithCheckPointID("cp"))
				id := requireConfirmInterrupt(t, err, c.Name)

				// a case without an answer leaves the confirmation pending.
				switch {
				case c.ResumeOther:
					ctx := compose.ResumeWithData(context.Background(), "not-this-confirmation", Decision{Approved: true})
					res, err = r.Invoke(ctx, "go", compose.WithCheckPointID("cp"))
				case c.Decision != nil:
					ctx := compose.ResumeWithData(context.Background(), id, *c.Decision)
					res, err = r.Invoke(ctx, "go", compose.WithCheckPointID("cp"))
				}
			}

			if c.Interrupted {
				requireConfirmInterrupt(t, err, c.Name)
			}

			assert.Len(t, db.FetchDocumentCalls(), c.FetchDocumentCalls)
			assert.Len(t, db.FetchDocumentByBranchIDCalls(), c.FetchDocumentByBranchIDCalls)
			assert.Len(t, db.FetchDocumentTreeCalls(), c.FetchDocumentTreeCalls)
			assert.Len(t, applier.ApplyCalls(), c.ApplyCalls)

			// no case fails a branch lookup, names a data source or a
			// parent, or runs create_document or delete_document past
			// the gate.
			assert.Empty(t, db.FetchDocumentBranchesCalls())
			assert.Empty(t, db.FetchDataSourceCalls())
			assert.Empty(t, db.CheckDocumentExistsCalls())
			assert.Empty(t, db.CheckDocumentCycleCalls())
			assert.Empty(t, db.UpdateDocumentParentIDCalls())
			assert.Empty(t, db.BeginTxCalls())

			if c.Interrupted || c.Rejected != "" {
				return
			}

			require.NoError(t, err)
			assert.JSONEq(t, c.RespJSON, res)
		})
	}
}

// requireConfirmInterrupt asserts that err is an interrupt carrying the
// named tool's confirm summary and returns the interrupt id to resume
// with. It accepts both shapes an interrupt travels in: wrapped in a
// graph run's interrupt info, or raw from a direct tool call.
func requireConfirmInterrupt(t *testing.T, err error, name Name) string {
	t.Helper()

	require.Error(t, err, "a pending write must interrupt the run")

	var (
		id   string
		info any
	)

	if gi, ok := compose.ExtractInterruptInfo(err); ok {
		require.Len(t, gi.InterruptContexts, 1)

		id = gi.InterruptContexts[0].ID
		info = gi.InterruptContexts[0].Info
	} else {
		ri, ok := compose.IsInterruptRerunError(err)
		require.True(t, ok, "a pending write must interrupt the run")

		info = ri
	}

	summary, ok := info.(ActionSummary)
	require.True(t, ok, "the interrupt must carry a confirm summary, got %T", info)
	assert.Equal(t, name, summary.Tool)

	return id
}

// sessionAgent runs one gated tool call inside an adk run, which is
// the only host whose context carries session values.
type sessionAgent struct {
	// tool is the gated tool under test.
	tool tool.InvokableTool

	// args is the JSON argument payload handed to the tool.
	args string

	// res is the tool result captured from the run.
	res string

	// err is the tool error captured from the run.
	err error
}

// Name identifies the agent.
func (a *sessionAgent) Name(_ context.Context) string { return "session-agent" }

// Description describes the agent.
func (a *sessionAgent) Description(_ context.Context) string { return "runs one gated tool" }

// Run invokes the gated tool with the run's session-aware context and
// captures its outcome.
func (a *sessionAgent) Run(ctx context.Context, _ *adk.AgentInput, _ ...adk.AgentRunOption) *adk.AsyncIterator[*adk.AgentEvent] {
	iter, gen := adk.NewAsyncIteratorPair[*adk.AgentEvent]()

	a.res, a.err = a.tool.InvokableRun(ctx, a.args)

	gen.Close()

	return iter
}

// memStore is a throwaway in-memory checkpoint store, so the gate
// tests can park and resume a run without external infrastructure.
type memStore struct {
	// m maps checkpoint ids to their serialised payloads.
	m map[string][]byte
}

// Get returns the stored checkpoint, when one exists.
func (s *memStore) Get(_ context.Context, id string) ([]byte, bool, error) {
	data, ok := s.m[id]

	return data, ok, nil
}

// Set stores a checkpoint under the given id.
func (s *memStore) Set(_ context.Context, id string, data []byte) error {
	s.m[id] = data

	return nil
}

func Test_confirming_interrupt(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Args string

		// FetchDocumentByBranchIDCalls is how many times the summary
		// reads the branch.
		FetchDocumentByBranchIDCalls int

		// Interrupted expects the run to park on a pending confirmation.
		Interrupted bool

		// Rejected is the text the refusal carries when the arguments
		// cannot be described.
		Rejected string
	}{
		"Unreadable arguments are refused": {
			Args:     `{`,
			Rejected: "replace_blocks: invalid input",
		},
		"Block the document does not hold is refused": {
			Args:                         `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"zzz","content":"<p>x</p>"}`,
			FetchDocumentByBranchIDCalls: 1,
			Rejected:                     fmt.Sprintf("%s: block zzz: %v", NameReplaceBlocks, errUnknownBlock),
		},
		"Described write is parked": {
			Args:                         `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"a","content":"<p id=\"a\">hi</p>"}`,
			FetchDocumentByBranchIDCalls: 1,
			Interrupted:                  true,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := stubContentDB(nil)
			applier := stubApplier()
			gate := &confirming{einoTool: newEinoTool(replaceBlocks{}, testDeps(db, applier, nil))}

			res, err := gate.interrupt(context.Background(), json.RawMessage(c.Args))

			assert.Len(t, db.FetchDocumentByBranchIDCalls(), c.FetchDocumentByBranchIDCalls)
			assert.Empty(t, db.FetchDocumentBranchesCalls())
			assert.Empty(t, applier.ApplyCalls())

			if c.Interrupted {
				requireConfirmInterrupt(t, err, NameReplaceBlocks)

				return
			}

			// a refusal is the call's result, so the model can fix the
			// payload instead of the turn ending.
			require.NoError(t, err)
			assert.Contains(t, res, c.Rejected)
		})
	}
}

func Test_autoApproved(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		// Session holds the session values of the adk run the check runs
		// in. Nil runs it outside any run.
		Session map[string]any
		Result  bool
	}{
		// outside an agent run there is no session, so nothing is
		// auto-approved and the gate always prompts.
		"Outside an agent run": {
			Result: false,
		},
		"Every write approved for the turn": {
			Session: map[string]any{SessionKeyAutoApprove: true},
			Result:  true,
		},
		"Approval withdrawn": {
			Session: map[string]any{SessionKeyAutoApprove: false},
			Result:  false,
		},
		"Session value that is not a bool": {
			Session: map[string]any{SessionKeyAutoApprove: "true"},
			Result:  false,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			if c.Session == nil {
				assert.Equal(t, c.Result, autoApproved(context.Background()))

				return
			}

			// session values exist only inside an adk run, so the check
			// runs there as a tool call.
			ag := &sessionAgent{
				tool: utils.NewTool(&schema.ToolInfo{Name: "probe"}, func(ctx context.Context, _ struct{}) (bool, error) {
					return autoApproved(ctx), nil
				}),
				args: `{}`,
			}

			iter := adk.NewRunner(context.Background(), adk.RunnerConfig{Agent: ag}).Run(
				context.Background(),
				[]adk.Message{schema.UserMessage("go")},
				adk.WithSessionValues(c.Session),
			)

			for {
				if _, ok := iter.Next(); !ok {
					break
				}
			}

			require.NoError(t, ag.err)
			assert.Equal(t, strconv.FormatBool(c.Result), ag.res)
		})
	}
}
