package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
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

	// ApplyCalls is the number of edits that must have reached the
	// applier.
	ApplyCalls int
}

func Test_confirming_InvokableRun(t *testing.T) {
	t.Parallel()

	docID := _testDocID
	writeArgs := `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"a","text":"hi"}`
	deleteArgs := `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"a"}`

	// every write tool must ask before it acts. Resuming an approved
	// turn reruns the tool from the top, so a tool that applied its
	// edit before interrupting would apply it a second time.
	cc := map[string]gateCase{
		"Insert block asks first": {
			Name:        NameInsertBlock,
			Args:        `{` + targetArgs(_stubMainBranchID) + `,"reference_block_uid":"b","position":"after","block":{"type":"paragraph"}}`,
			Interrupted: true,
		},
		"Delete block asks first":      {Name: NameDeleteBlock, Args: deleteArgs, Interrupted: true},
		"Delete document asks first":   {Name: NameDeleteDocument, Args: `{"document_id":"` + docID.String() + `"}`, Interrupted: true},
		"Update document asks first":   {Name: NameUpdateDocument, Args: `{"document_id":"` + docID.String() + `","name":"n"}`, Interrupted: true},
		"Create document asks first":   {Name: NameCreateDocument, Args: `{"name":"n"}`, Interrupted: true},
		"Update block text asks first": {Name: NameUpdateBlockText, Args: writeArgs, Interrupted: true},
		"Approve-all covers a later non-destructive write": {
			Name:        NameUpdateBlockText,
			Args:        writeArgs,
			AutoApprove: true,
			RespJSON:    `{"blocks":[{"uid":"a","kind":"paragraph","text":"hi","depth":0}]}`,
			ApplyCalls:  1,
		},
		"Approve-all never covers a destructive write": {
			Name:        NameDeleteBlock,
			Args:        deleteArgs,
			AutoApprove: true,
			Interrupted: true,
		},
		"Approved confirmation reruns the tool": {
			Name:       NameUpdateBlockText,
			Args:       writeArgs,
			Decision:   &Decision{Approved: true},
			RespJSON:   `{"blocks":[{"uid":"a","kind":"paragraph","text":"hi","depth":0}]}`,
			ApplyCalls: 1,
		},
		"Declined confirmation reports the refusal": {
			Name:     NameUpdateBlockText,
			Args:     writeArgs,
			Decision: &Decision{Approved: false},
			RespJSON: `{"approved":false,"reason":"user declined"}`,
		},
		"Resume of another interrupt point re-interrupts": {
			Name:        NameUpdateBlockText,
			Args:        writeArgs,
			ResumeOther: true,
			Interrupted: true,
		},
		"Unreadable arguments are refused, never confirmed": {
			Name:     NameUpdateBlockText,
			Args:     `{`,
			Rejected: "update_block_text: invalid input",
		},
		"Missing arguments are refused, never confirmed": {
			Name:     NameUpdateBlockText,
			Args:     `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"b"}`,
			Rejected: "update_block_text: text is required",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			applier := stubApplier()
			ct := gatedTool(t, New(testDeps(stubContentDB(nil), applier, nil)), c.Name)

			if c.Rejected != "" {
				// the same payload would be rejected by the tool on
				// resume, so the run must not spend a confirmation on
				// it. The rejection is the call's result, not an
				// error: an error would end the turn, and the payload
				// is the model's to fix.
				res, rerr := ct.InvokableRun(context.Background(), c.Args)
				require.NoError(t, rerr, "bad arguments must not end the turn")
				assert.Contains(t, res, c.Rejected)
				assert.Empty(t, applier.ApplyCalls())

				return
			}

			var (
				res string
				err error
			)

			if c.AutoApprove {
				res, err = runGateInSession(ct, c.Args)
			} else {
				res, err = runGateParked(t, ct, c)
			}

			if c.Interrupted {
				requireConfirmInterrupt(t, err, c.Name)
			}

			assert.Len(t, applier.ApplyCalls(), c.ApplyCalls)

			if c.Interrupted {
				return
			}

			require.NoError(t, err)
			assert.JSONEq(t, c.RespJSON, res)
		})
	}
}

// runGateInSession runs one gated tool call inside an adk run carrying
// an approve-all answer, which is the only host whose context has
// session values, and returns the captured outcome.
func runGateInSession(ct tool.InvokableTool, args string) (string, error) {
	ag := &sessionAgent{tool: ct, args: args}

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

	return ag.res, ag.err
}

// runGateParked hosts the gated tool in a checkpointed graph, asserts
// the run parks on the tool's confirmation, and replays the case's
// answer — or leaves the confirmation unanswered when the case gives
// none.
func runGateParked(t *testing.T, ct tool.InvokableTool, c gateCase) (string, error) {
	t.Helper()

	r := gateHost(t, ct, c.Args)

	_, err := r.Invoke(context.Background(), "go", compose.WithCheckPointID("cp"))
	id := requireConfirmInterrupt(t, err, c.Name)

	if c.Decision == nil && !c.ResumeOther {
		return "", err
	}

	ctx := context.Background()

	if c.ResumeOther {
		ctx = compose.ResumeWithData(ctx, "not-this-confirmation", Decision{Approved: true})
	} else {
		ctx = compose.ResumeWithData(ctx, id, *c.Decision)
	}

	return r.Invoke(ctx, "go", compose.WithCheckPointID("cp"))
}

// gateHost compiles a one-node graph around the gated tool, the
// smallest host that can park the gate on its interrupt and resume it
// with the user's answer.
func gateHost(t *testing.T, ct tool.InvokableTool, args string) compose.Runnable[string, string] {
	t.Helper()

	g := compose.NewGraph[string, string]()
	require.NoError(t, g.AddLambdaNode("gate", compose.InvokableLambda(
		func(ctx context.Context, _ string) (string, error) {
			return ct.InvokableRun(ctx, args)
		},
	)))
	require.NoError(t, g.AddEdge(compose.START, "gate"))
	require.NoError(t, g.AddEdge("gate", compose.END))

	r, err := g.Compile(
		context.Background(),
		compose.WithCheckPointStore(&memStore{m: map[string][]byte{}}),
	)
	require.NoError(t, err)

	return r
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
