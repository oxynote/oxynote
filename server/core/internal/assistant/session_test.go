package assistant

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"
	mock "github.com/oxynote/oxynote/server/core/internal/_mock"
	"github.com/oxynote/oxynote/server/core/internal/assistant/edit"
	"github.com/oxynote/oxynote/server/core/internal/assistant/persist"
	"github.com/oxynote/oxynote/server/core/internal/assistant/protocol"
	protocolMock "github.com/oxynote/oxynote/server/core/internal/assistant/protocol/_mock"
	"github.com/oxynote/oxynote/server/core/internal/assistant/tools"
	toolsMock "github.com/oxynote/oxynote/server/core/internal/assistant/tools/_mock"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// _turnTimeout bounds how long a test waits for a turn to finish.
var _turnTimeout = 10 * time.Second

// recorder captures everything a session writes to its client and
// signals when the turn reaches a terminal message.
type recorder struct {
	mu   sync.Mutex
	msgs []any

	// settled receives once per terminal message: a done, or a request
	// for the user to confirm a batch of writes. Both mean the session
	// has handed control back to the client. It is buffered rather than
	// closed so a test can await several turns in sequence.
	settled chan struct{}
}

// newRecorder builds a recorder wired into a session writer mock.
func newRecorder() (*recorder, *protocolMock.SessionWriter) {
	r := &recorder{settled: make(chan struct{}, 16)}

	return r, &protocolMock.SessionWriter{
		WriteJSONFunc: func(_ context.Context, msg any) {
			r.mu.Lock()
			r.msgs = append(r.msgs, msg)
			r.mu.Unlock()

			switch msg.(type) {
			case protocol.DoneMessage, protocol.ConfirmRequest:
				select {
				case r.settled <- struct{}{}:
				default:
				}
			}
		},
	}
}

// wait blocks until the turn settles.
func (r *recorder) wait(t *testing.T) {
	t.Helper()

	select {
	case <-r.settled:
	case <-time.After(_turnTimeout):
		require.FailNow(t, "the turn never settled", "captured: %v", r.types())
	}
}

// types lists the message types captured, in order.
func (r *recorder) types() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := make([]string, 0, len(r.msgs))

	for _, m := range r.msgs {
		switch v := m.(type) {
		case protocol.TextDeltaMessage:
			out = append(out, string(protocol.ServerTypeTextDelta))
		case protocol.TextEndMessage:
			out = append(out, string(protocol.ServerTypeTextEnd)+":"+string(v.Kind))
		case protocol.ToolStatusMessage:
			out = append(out, string(protocol.ServerTypeToolStatus)+":"+v.Tool)
		case protocol.ConfirmRequest:
			out = append(out, string(protocol.ServerTypeConfirmRequest))
		case protocol.DoneMessage:
			out = append(out, string(protocol.ServerTypeDone))
		case protocol.ErrorMessage:
			out = append(out, string(protocol.ServerTypeError))
		case protocol.HistoryMessage:
			out = append(out, string(protocol.ServerTypeHistory))
		}
	}

	return out
}

// deltaText joins every streamed text fragment.
func (r *recorder) deltaText() string {
	r.mu.Lock()
	defer r.mu.Unlock()

	var out strings.Builder

	for _, m := range r.msgs {
		if v, ok := m.(protocol.TextDeltaMessage); ok {
			out.WriteString(v.Content)
		}
	}

	return out.String()
}

// find returns the first captured message of the given type.
func find[T any](r *recorder) (T, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, m := range r.msgs {
		if v, ok := m.(T); ok {
			return v, true
		}
	}

	var zero T

	return zero, false
}

// toolCall builds an assistant message asking for one tool call.
func toolCall(id, name, args string) *schema.Message {
	return schema.AssistantMessage("", []schema.ToolCall{
		{
			ID:       id,
			Type:     "function",
			Function: schema.FunctionCall{Name: name, Arguments: args},
		},
	})
}

// stubDocumentDB answers every document lookup with the same document,
// so an edit tool can name what it is about to change, and every branch
// read with the two paragraphs the edits target.
func stubDocumentDB() *toolsMock.DB {
	content := document.RootBlock{
		Content: []document.Block{
			{Type: document.BlockNodeParagraph, Attrs: document.Attributes{document.AttrUID: "a"}},
			{Type: document.BlockNodeParagraph, Attrs: document.Attributes{document.AttrUID: "b"}},
		},
	}

	return &toolsMock.DB{
		FetchDocumentFunc: func(_ context.Context, id xid.ID, orgID, _ string) (*document.Document, error) {
			return &document.Document{
				BranchID:       _stubBranchID,
				DocumentName:   "Runbook",
				Default:        true,
				ID:             id,
				OrganizationID: orgID,
			}, nil
		},
		FetchDocumentByBranchIDFunc: func(_ context.Context, branchID xid.ID, orgID string) (*document.Document, error) {
			return &document.Document{
				BranchID:       branchID,
				DocumentName:   "Runbook",
				Default:        true,
				Content:        content,
				ID:             _stubDocID,
				OrganizationID: orgID,
			}, nil
		},
	}
}

// _stubDocID and _stubBranchID name the document and branch the stubs
// answer for; an edit has to name both.
var (
	_stubDocID    = xid.New()
	_stubBranchID = xid.New()
)

// stubEditApplier accepts every edit it is handed.
func stubEditApplier() *toolsMock.EditApplier {
	return &toolsMock.EditApplier{
		ApplyFunc: func(_ context.Context, _, _ xid.ID, _ []edit.Operation, _ bool) (edit.Result, error) {
			return edit.Result{Applied: 1, Errors: []edit.OpError{}}, nil
		},
	}
}

// prepSession builds a session over the given manager, wired to a
// recorder capturing everything the session writes back.
func prepSession(t *testing.T, m *Manager) (*session, *recorder) {
	t.Helper()

	rec, writer := newRecorder()

	s, err := m.newSession(context.Background(), "org", "user", writer)
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, s.Close())
	})

	return s, rec
}

func Test_Manager_newSession(t *testing.T) {
	t.Parallel()

	m, st := testManagerStores()
	st.history.GetFunc = func(_ context.Context, _ string) (*[]*schema.Message, error) {
		return &[]*schema.Message{schema.UserMessage("earlier question")}, nil
	}

	rec, writer := newRecorder()

	s, err := m.newSession(context.Background(), "org", "user", writer)
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, s.Close())
	})

	assert.Equal(t, persist.SessionKey("org", "user"), s.key)
	assert.Same(t, m, s.man)
	assert.Same(t, writer, s.writer)
	assert.NotNil(t, s.runner)
	assert.NotNil(t, s.supv)
	assert.Empty(t, s.activeDocumentID)
	require.Len(t, s.messages, 1)

	// a reconnecting client is handed the conversation it lost, so the
	// chat pane is not empty after a reload.
	msg, ok := find[protocol.HistoryMessage](rec)
	require.True(t, ok)
	require.Len(t, msg.Messages, 1)
	assert.Equal(t, "earlier question", msg.Messages[0].Content)
}

func Test_session_Close(t *testing.T) {
	t.Parallel()

	rec, writer := newRecorder()

	s, err := testManager().newSession(context.Background(), "org", "user", writer)
	require.NoError(t, err)

	require.NoError(t, s.Close())
	assert.Empty(t, rec.types())
}

func Test_session_SetActiveDocument(t *testing.T) {
	t.Parallel()

	s, _ := prepSession(t, testManager())

	// both halves are kept as reported; a branch that turns out not to
	// exist is the next call's to refuse.
	s.SetActiveDocument("doc-9", "branch-3")

	s.mu.Lock()
	defer s.mu.Unlock()

	assert.Equal(t, "doc-9", s.activeDocumentID)
	assert.Equal(t, "branch-3", s.activeBranchID)
}

func Test_session_rememberMessages(t *testing.T) {
	t.Parallel()

	s, _ := prepSession(t, testManager())

	msgs := []*schema.Message{schema.UserMessage("hi")}
	s.rememberMessages(msgs)

	s.mu.Lock()
	defer s.mu.Unlock()

	// the next turn starts from the conversation the middlewares
	// compacted, not the raw one this session began with.
	assert.Equal(t, msgs, s.messages)
}

func Test_session_sendHistory(t *testing.T) {
	t.Parallel()

	s, rec := prepSession(t, testManager())

	// nothing to restore sends nothing.
	s.sendHistory(context.Background())
	assert.Empty(t, rec.types())

	s.messages = []*schema.Message{schema.UserMessage("hi")}
	s.sendHistory(context.Background())

	msg, ok := find[protocol.HistoryMessage](rec)
	require.True(t, ok)
	require.Len(t, msg.Messages, 1)
	assert.Equal(t, "hi", msg.Messages[0].Content)
}

func Test_session_Process(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		JSON   string
		Expect func(t *testing.T, s *session, rec *recorder)
	}{
		"Unknown message type is reported": {
			JSON: `{"type":"nonsense"}`,
			Expect: func(t *testing.T, _ *session, rec *recorder) {
				msg, ok := find[protocol.ErrorMessage](rec)
				require.True(t, ok)
				assert.Equal(t, "unknown message type", msg.Message)
			},
		},
		"Active document is recorded": {
			JSON: `{"type":"set_active_document","documentId":"doc-1"}`,
			Expect: func(t *testing.T, s *session, _ *recorder) {
				s.mu.Lock()
				defer s.mu.Unlock()

				assert.Equal(t, "doc-1", s.activeDocumentID)
				assert.Empty(t, s.activeBranchID)
			},
		},
		"Active branch is recorded": {
			JSON: `{"type":"set_active_document","documentId":"doc-1","branchId":"branch-1"}`,
			Expect: func(t *testing.T, s *session, _ *recorder) {
				s.mu.Lock()
				defer s.mu.Unlock()

				assert.Equal(t, "doc-1", s.activeDocumentID)
				assert.Equal(t, "branch-1", s.activeBranchID)
			},
		},
		"Empty message content is refused": {
			JSON: `{"type":"message","content":""}`,
			Expect: func(t *testing.T, _ *session, rec *recorder) {
				_, ok := find[protocol.ErrorMessage](rec)
				assert.True(t, ok)
			},
		},
		"Stale confirm response is ignored": {
			JSON: `{"type":"confirm_response","turnId":"gone","approved":true}`,
			Expect: func(t *testing.T, _ *session, rec *recorder) {
				assert.Empty(t, rec.types())
			},
		},
		"Malformed confirm response is refused": {
			JSON: `{"type":"confirm_response","approved":"yes"}`,
			Expect: func(t *testing.T, _ *session, rec *recorder) {
				msg, ok := find[protocol.ErrorMessage](rec)
				require.True(t, ok)
				assert.Equal(t, "invalid confirm response", msg.Message)
			},
		},
		"Reset clears the conversation": {
			JSON: `{"type":"reset"}`,
			Expect: func(t *testing.T, s *session, _ *recorder) {
				s.mu.Lock()
				defer s.mu.Unlock()

				assert.Nil(t, s.messages)
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			s, rec := prepSession(t, testManager())

			s.Process(context.Background(), []byte(c.JSON))

			c.Expect(t, s, rec)
		})
	}
}

func Test_session_handleMessage(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Responses []*schema.Message
		Content   string
		Stored    []*schema.Message
		Pending   bool
		Settles   bool
		Expect    func(t *testing.T, rec *recorder, cm *mock.ChatModel, st *stores)
	}{
		"Turn builds on the stored conversation": {
			Content: "and what about this",
			Stored: []*schema.Message{
				schema.UserMessage("from another tab"),
				schema.AssistantMessage("noted", nil),
			},
			Settles: true,
			Expect: func(t *testing.T, _ *recorder, cm *mock.ChatModel, _ *stores) {
				// the turn starts from what the store holds, not from this
				// session's copy: another connection of the same user may
				// have finished turns since this one loaded it, and a turn
				// built on the stale copy would save a history discarding
				// theirs.
				require.NotEmpty(t, cm.StreamCalls())

				var all strings.Builder

				for _, msg := range cm.StreamCalls()[0].Input {
					all.WriteString(msg.Content)
					all.WriteString("\n")
				}

				assert.Contains(t, all.String(), "from another tab")
				assert.NotContains(t, all.String(), "stale local copy")
			},
		},
		"Outstanding confirmation is abandoned by a new message": {
			Responses: []*schema.Message{
				schema.AssistantMessage("fresh answer", nil),
			},
			Content: "actually, do something else",
			Pending: true,
			Settles: true,
			Expect: func(t *testing.T, rec *recorder, _ *mock.ChatModel, st *stores) {
				// the unanswered confirmation is treated as declined:
				// its record is cleared rather than left pointing at a
				// checkpoint the new turn is about to delete.
				require.Len(t, st.pendings.DeleteCalls(), 1)

				assert.Contains(t, rec.types(), string(protocol.ServerTypeDone))
			},
		},
		"Read turn answers without asking": {
			Responses: []*schema.Message{
				toolCall("1", string(tools.NameSearchDocuments), `{"query":"rate limit"}`),
				schema.AssistantMessage("there are no documents yet", nil),
			},
			Content: "what documents do we have",
			Settles: true,
			Expect: func(t *testing.T, rec *recorder, cm *mock.ChatModel, _ *stores) {
				// the user is told which tool is running before it runs,
				// sees the answer stream in, and the turn closes cleanly.
				assert.Contains(t, rec.types(),
					string(protocol.ServerTypeToolStatus)+":"+string(tools.NameSearchDocuments))
				assert.Contains(t, rec.types(), string(protocol.ServerTypeDone))
				assert.Equal(t, "there are no documents yet", rec.deltaText())

				// a read tool never asks permission.
				_, asked := find[protocol.ConfirmRequest](rec)
				assert.False(t, asked, "a read must not prompt for confirmation")

				assert.Len(t, cm.StreamCalls(), 2)
			},
		},
		"Empty content is refused": {
			Content: "   ",
			Expect: func(t *testing.T, rec *recorder, cm *mock.ChatModel, _ *stores) {
				msg, ok := find[protocol.ErrorMessage](rec)
				require.True(t, ok)
				assert.Equal(t, "message content is required", msg.Message)

				// an empty message never reaches the model, so the
				// conversation is not left carrying it.
				assert.Empty(t, cm.StreamCalls())
			},
		},
		"Write parks on a confirmation": {
			Responses: []*schema.Message{
				toolCall("1", string(tools.NameCreateDocument), `{"name":"Runbook"}`),
				schema.AssistantMessage("created", nil),
			},
			Content: "make a runbook",
			Settles: true,
			Expect: func(t *testing.T, rec *recorder, _ *mock.ChatModel, _ *stores) {
				req, ok := find[protocol.ConfirmRequest](rec)
				require.True(t, ok, "a write must prompt, got %v", rec.types())
				require.Len(t, req.Actions, 1)
				assert.Equal(t, string(tools.NameCreateDocument), req.Actions[0].Tool)
				assert.NotEmpty(t, req.TurnID)

				// the turn is parked, not finished: no done was sent.
				assert.NotContains(t, rec.types(), string(protocol.ServerTypeDone))
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			cm := stubChatModel(c.Responses...)

			m, st := testManagerStores()
			m.model = cm

			s, rec := prepSession(t, m)

			if c.Pending {
				m.pendings.Save(context.Background(), s.key, persist.PendingConfirm{
					TurnID:       "stale",
					InterruptIDs: []string{"i1"},
				})
			}

			if c.Stored != nil {
				st.history.GetFunc = func(_ context.Context, _ string) (*[]*schema.Message, error) {
					return &c.Stored, nil
				}

				s.mu.Lock()
				s.messages = []*schema.Message{schema.UserMessage("stale local copy")}
				s.mu.Unlock()
			}

			s.handleMessage(context.Background(), c.Content)

			if c.Settles {
				rec.wait(t)
			}

			c.Expect(t, rec, cm, st)
		})
	}
}

func Test_session_handleReset(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Processing bool
		DeleteErr  error
		Cleared    bool
	}{
		"Conversation is cleared": {
			Cleared: true,
		},
		"Error returned by history.Delete": {
			// a failed delete still clears the session's own copy, so
			// the user gets the fresh start they asked for.
			DeleteErr: assert.AnError,
			Cleared:   true,
		},
		"Reset is refused mid-turn": {
			Processing: true,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			m, st := testManagerStores()

			if c.DeleteErr != nil {
				st.history.DeleteFunc = func(_ context.Context, _ string) error {
					return c.DeleteErr
				}
			}

			s, rec := prepSession(t, m)

			s.mu.Lock()
			s.messages = []*schema.Message{schema.UserMessage("hi")}
			s.mu.Unlock()

			// a turn in flight holds the conversation's claim; it may be
			// running on any connection of the same user, so it is held
			// in the manager rather than on this session.
			if c.Processing {
				require.True(t, m.claimTurn(s.key))
			}

			s.handleReset(context.Background())

			s.mu.Lock()
			left := s.messages
			s.mu.Unlock()

			if !c.Cleared {
				assert.NotEmpty(t, left)
				assert.Empty(t, st.history.DeleteCalls())

				msg, ok := find[protocol.ErrorMessage](rec)
				require.True(t, ok)
				assert.Equal(t, "cannot reset while processing", msg.Message)

				return
			}

			assert.Nil(t, left)

			// the paused turn goes with the conversation: nothing can
			// resume writes belonging to a chat that no longer exists.
			assert.Len(t, st.history.DeleteCalls(), 1)
			assert.Len(t, st.pendings.DeleteCalls(), 1)
			assert.Len(t, st.blobs.DeleteCalls(), 1)

			// the reset's own claim is released, so the next turn can
			// start.
			assert.True(t, m.claimTurn(s.key))
		})
	}
}

func Test_session_handleConfirmResponse(t *testing.T) {
	t.Parallel()

	docID := _stubDocID.String()
	edit1 := `{"document_id":"` + docID + `","branch_id":"` + _stubBranchID.String() + `","block_uid":"a","text":"one"}`
	edit2 := `{"document_id":"` + docID + `","branch_id":"` + _stubBranchID.String() + `","block_uid":"b","text":"two"}`

	oneWrite := []*schema.Message{
		toolCall("1", string(tools.NameUpdateBlockText), edit1),
		schema.AssistantMessage("updated the intro", nil),
	}

	cc := map[string]struct {
		Responses           []*schema.Message
		StaleTurn           bool
		AbandonedUnderClaim bool
		CorruptCheckpoint   bool
		Approved            bool
		All                 bool
		Applied             int
	}{
		"Answer abandoned between the look and the claim is dropped": {
			// between the answer's first look at the pending record and
			// its claim of the conversation, a new message on another
			// connection of the same user can abandon the confirmation
			// and delete its checkpoint; the re-read under the claim has
			// to catch that and drop the answer instead of resuming into
			// a checkpoint that no longer exists.
			Responses:           oneWrite,
			AbandonedUnderClaim: true,
			Approved:            true,
		},
		"Approving resumes the parked write": {
			Responses: oneWrite,
			Approved:  true,
			Applied:   1,
		},
		"Unresumable checkpoint fails the turn": {
			// the pending record is the only route back to a checkpoint,
			// and it is already gone by the time the resume is attempted,
			// so a resume that cannot run has to reclaim the checkpoint
			// rather than leave a whole conversation in Redis until it
			// expires.
			Responses:         oneWrite,
			CorruptCheckpoint: true,
			Approved:          true,
		},
		"Declining finishes the turn without writing": {
			Responses: oneWrite,
		},
		"Approving all covers later writes in the same turn": {
			// two separate write batches: the first is confirmed, the
			// second must ride on the same answer.
			Responses: []*schema.Message{
				toolCall("1", string(tools.NameUpdateBlockText), edit1),
				toolCall("2", string(tools.NameUpdateBlockText), edit2),
				schema.AssistantMessage("both edits applied", nil),
			},
			Approved: true,
			All:      true,
			Applied:  2,
		},
		"Answer to another turn is ignored": {
			// only the outstanding confirmation can be answered, or a
			// stale reply would approve writes the user never saw.
			Responses: oneWrite,
			StaleTurn: true,
			Approved:  true,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			applier := stubEditApplier()
			cm := stubChatModel(c.Responses...)

			m, st := testManagerStores()
			m.model = cm
			m.db = stubDocumentDB()
			m.applier = applier

			s, rec := prepSession(t, m)
			s.SetActiveDocument(docID, _stubBranchID.String())

			s.handleMessage(context.Background(), "reword the intro")
			rec.wait(t)

			req, ok := find[protocol.ConfirmRequest](rec)
			require.True(t, ok, "the write did not prompt, got %v", rec.types())
			require.Empty(t, applier.ApplyCalls(), "nothing may be applied while the user decides")

			turnID := req.TurnID
			if c.StaleTurn {
				turnID = "some-other-turn"
			}

			if c.CorruptCheckpoint {
				require.NoError(t, m.checkpoints.Set(
					context.Background(),
					s.key,
					[]byte("not a checkpoint"),
				))
			}

			if c.AbandonedUnderClaim {
				// the first look still sees the record; the re-read under
				// the claim finds it gone, as it would after another
				// connection's message abandoned the confirmation.
				var loads atomic.Int32

				orig := st.pendings.GetFunc
				st.pendings.GetFunc = func(ctx context.Context, key string) (*persist.PendingConfirm, error) {
					if loads.Add(1) > 1 {
						return nil, errutil.ErrNotFound
					}

					return orig(ctx, key)
				}
			}

			s.handleConfirmResponse(context.Background(), turnID, c.Approved, c.All)

			if c.AbandonedUnderClaim {
				// nothing resumed and nothing was applied — and the claim
				// was handed back, so the abandoning turn's successor can
				// start.
				assert.Empty(t, applier.ApplyCalls())
				assert.True(t, m.claimTurn(s.key))

				return
			}

			if c.StaleTurn {
				assert.Empty(t, applier.ApplyCalls())

				stored := m.pendings.Load(context.Background(), s.key)
				require.NotNil(t, stored)
				assert.Equal(t, req.TurnID, stored.TurnID)

				return
			}

			rec.wait(t)

			assert.Len(t, applier.ApplyCalls(), c.Applied)
			assert.Contains(t, rec.types(), string(protocol.ServerTypeDone))

			// the answered confirmation is forgotten, so a reconnect
			// does not re-ask a question the user already settled.
			assert.Nil(t, m.pendings.Load(context.Background(), s.key))

			// and the checkpoint goes with it: nothing can reach a
			// paused turn once the question behind it is answered, so
			// leaving one behind only wastes the conversation's TTL.
			_, found, err := m.checkpoints.Get(context.Background(), s.key)
			require.NoError(t, err)
			assert.False(t, found)

			// every run stays anchored to the document the user is
			// viewing — the auto-approve flag of an "approve all"
			// answer must not displace it from the session values.
			for _, call := range cm.StreamCalls() {
				require.NotEmpty(t, call.Input)
				assert.Contains(t, call.Input[0].Content, docID)
			}
		})
	}
}

func Test_session_beginTurn(t *testing.T) {
	t.Parallel()

	m := testManager()
	s, rec := prepSession(t, m)

	require.True(t, s.beginTurn(context.Background()))

	// a second turn is refused while the first runs, so two messages
	// cannot interleave in one conversation.
	assert.False(t, s.beginTurn(context.Background()))

	msg, ok := find[protocol.ErrorMessage](rec)
	require.True(t, ok)
	assert.Equal(t, "already processing a message", msg.Message)

	// the claim covers every connection of the same (org, user): a
	// second session over the same conversation shares the one history
	// and checkpoint, so its turn is refused just the same.
	other, otherRec := prepSession(t, m)

	assert.False(t, other.beginTurn(context.Background()))

	msg, ok = find[protocol.ErrorMessage](otherRec)
	require.True(t, ok)
	assert.Equal(t, "already processing a message", msg.Message)

	// once released, either connection may begin the next turn.
	m.releaseTurn(s.key)
	assert.True(t, other.beginTurn(context.Background()))
}

func Test_session_goTurn(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Pending   *persist.PendingConfirm
		CancelCtx bool
		Panic     bool
	}{
		"Finished turn releases the session": {},
		"Parked turn asks the user once the session is free": {
			Pending: &persist.PendingConfirm{TurnID: "t1"},
		},
		"Parked turn is persisted past a dead connection": {
			Pending:   &persist.PendingConfirm{TurnID: "t1"},
			CancelCtx: true,
		},
		"Panicking turn still closes the turn out": {
			Panic: true,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			m, st := testManagerStores()
			s, rec := prepSession(t, m)

			// a parked turn's record has to be durable before anything
			// else can claim the conversation: a message arriving in that
			// window — on this connection or another of the same user —
			// would see no pending confirmation and orphan the checkpoint
			// the parked turn is waiting on.
			var claimedAtSave atomic.Bool

			st.pendings.SetFunc = func(context.Context, string, persist.PendingConfirm) error {
				m.turns.mu.Lock()
				_, held := m.turns.m[s.key]
				m.turns.mu.Unlock()

				claimedAtSave.Store(held)

				return nil
			}

			ctx := context.Background()

			if c.CancelCtx {
				cctx, cancel := context.WithCancel(ctx)
				cancel()

				ctx = cctx
			}

			require.True(t, s.beginTurn(context.Background()))

			s.goTurn(ctx, func(context.Context) *persist.PendingConfirm {
				if c.Panic {
					panic("turn exploded")
				}

				return c.Pending
			})

			if c.Panic {
				// the supervisor's recovery plan absorbs the panic; the
				// client must still hear that the turn ended.
				s.supv.CloseAndWait()

				assert.Contains(t, rec.types(), string(protocol.ServerTypeError))
				assert.Contains(t, rec.types(), string(protocol.ServerTypeDone))

				return
			}

			if c.Pending != nil {
				rec.wait(t)
			} else {
				s.supv.Wait()
			}

			// the claim is released before the question is asked, so an
			// immediate answer is not refused as a second turn.
			m.turns.mu.Lock()
			_, held := m.turns.m[s.key]
			m.turns.mu.Unlock()

			assert.False(t, held)

			if c.Pending == nil {
				assert.Empty(t, rec.types())

				return
			}

			req, found := find[protocol.ConfirmRequest](rec)
			require.True(t, found)
			assert.Equal(t, "t1", req.TurnID)

			// the record is written on a context that survives the
			// connection, so a turn parking as the socket dies still
			// leaves an answerable confirmation behind.
			ff := st.pendings.SetCalls()
			require.Len(t, ff, 1)
			assert.NoError(t, ff[0].Ctx.Err())

			assert.True(t, claimedAtSave.Load(),
				"the confirmation must be stored before the claim is released")
		})
	}
}

func Test_session_runOptions(t *testing.T) {
	t.Parallel()

	s, _ := prepSession(t, testManager())
	s.SetActiveDocument("doc-3", "")

	// the checkpoint id and the active document snapshot, so a mid-turn
	// navigation cannot shift the prompt under an in-flight turn.
	assert.Len(t, s.runOptions(nil), 2)
}

func Test_session_resendPendingConfirmation(t *testing.T) {
	t.Parallel()

	m := testManager()
	m.model = stubChatModel(
		toolCall("1", string(tools.NameCreateDocument), `{"name":"Runbook"}`),
		schema.AssistantMessage("created", nil),
	)

	s, rec := prepSession(t, m)

	s.handleMessage(context.Background(), "make a runbook")
	rec.wait(t)

	first, ok := find[protocol.ConfirmRequest](rec)
	require.True(t, ok, "the write did not prompt, got %v", rec.types())

	// a new connection for the same conversation, as though the user had
	// reloaded the page while the assistant waited on them.
	_, second := prepSession(t, m)

	again, ok := find[protocol.ConfirmRequest](second)
	require.True(t, ok, "the pending confirmation was not re-delivered")

	// the same question, so the answer the user gives still addresses
	// the writes the original turn parked on.
	assert.Equal(t, first.TurnID, again.TurnID)
	assert.Equal(t, first.Actions, again.Actions)
}
