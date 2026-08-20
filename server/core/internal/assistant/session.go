package assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/jellydator/xync"
	"github.com/oxynote/oxynote/server/core/internal/assistant/middleware"
	"github.com/oxynote/oxynote/server/core/internal/assistant/offload"
	"github.com/oxynote/oxynote/server/core/internal/assistant/protocol"
	"github.com/oxynote/oxynote/server/core/internal/assistant/tools"
	"github.com/oxynote/oxynote/server/core/pkg/logutil"
	"github.com/tidwall/gjson"
)

const (
	// _agentName identifies the assistant in agent events and logs.
	_agentName = "rubber-duck"

	// _agentDescription describes the assistant to the framework. It is
	// only surfaced when an agent is used as a tool by another agent,
	// which Oxynote does not do, but the framework asks for it.
	_agentDescription = "Reads and edits documents in the user's organisation."
)

// _maxAgentTurns caps the model/tool cycles in a single turn so a
// runaway model cannot burn budget indefinitely.
const _maxAgentTurns = 50

// newAgent builds the chat agent for one session. Tools are bound to
// the session's organisation and user, so an agent can never reach
// another organisation's documents.
func (m *Manager) newAgent(
	ctx context.Context,
	toolSet *tools.Set,
	obs *middleware.Observer,
) (adk.Agent, error) {
	backend := offload.New(m.blobs)
	ts := append(toolSet.Tools(), backend.ReadTool())

	compaction, err := middleware.NewCompaction(ctx, m.summary, backend, toolSet.WriteNames())
	if err != nil {
		return nil, err
	}

	// the observer runs outermost so it sees the conversation as the
	// compaction middlewares left it.
	handlers := append([]adk.ChatModelAgentMiddleware{obs}, compaction...)

	agent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:          _agentName,
		Description:   _agentDescription,
		Model:         m.model,
		MaxIterations: _maxAgentTurns,
		GenModelInput: genModelInput,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: ts,

				// tools run one at a time, in the order the model asked
				// for them. Reads are order-independent, but an edit may
				// reference a block an earlier edit in the same batch
				// created, and two edits to one document must not race.
				ExecuteSequentially: true,
			},
		},
		Handlers: handlers,
	})
	if err != nil {
		return nil, fmt.Errorf("building assistant agent: %w", err)
	}

	return agent, nil
}

// session holds per-connection state for an AI assistant session.
type session struct {
	man    *Manager
	orgID  string
	userID string
	writer protocol.SessionWriter
	tools  *tools.Set
	runner *adk.Runner

	// key addresses this conversation's history and checkpoint.
	key string

	supv *xync.Supervisor

	mu         sync.Mutex
	processing bool

	// activeDocumentID is the document the user is currently viewing,
	// surfaced into the system prompt so the model can resolve "this
	// document" without asking.
	activeDocumentID string

	// messages is the conversation as the last completed turn left it,
	// after the context middlewares compacted it.
	messages []*schema.Message
}

// newSession creates a chat session bound to the given organisation,
// user, and message writer, restoring whatever the last conversation
// left behind.
func (m *Manager) newSession(
	ctx context.Context,
	orgID, userID string,
	writer protocol.SessionWriter,
) (*session, error) {
	key := createSessionKey(orgID, userID)

	toolSet := tools.New(tools.NewInput(m.log, m.db, m.search, m.applier, m.tree, orgID, userID))

	s := &session{
		man:    m,
		orgID:  orgID,
		userID: userID,
		writer: writer,
		tools:  toolSet,
		key:    key,
		supv:   xync.NewSupervisor(),
	}

	obs := middleware.NewObserver(toolSet, writer, s.rememberMessages, m.metrics.observeToolCall)

	agent, err := m.newAgent(ctx, toolSet, obs)
	if err != nil {
		return nil, err
	}

	s.runner = adk.NewRunner(ctx, adk.RunnerConfig{
		Agent:           agent,
		EnableStreaming: true,
		CheckPointStore: &checkpointStore{store: m.blobs},
	})

	s.messages = m.loadMessages(ctx, key)
	s.sendHistory(ctx)
	s.resendPendingConfirmation(ctx)

	return s, nil
}

// Close stops and closes all processes for the session.
func (s *session) Close() error {
	s.supv.CloseAndWait()

	return nil
}

// SetActiveDocument records which document the user is viewing.
func (s *session) SetActiveDocument(documentID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.activeDocumentID = documentID
}

// rememberMessages records the conversation as a completed run left it,
// so the next turn starts from the compacted history rather than the
// raw one.
func (s *session) rememberMessages(msgs []*schema.Message) {
	s.mu.Lock()
	s.messages = msgs
	s.mu.Unlock()
}

// resendPendingConfirmation re-delivers an unanswered confirmation, so
// a user who reloaded the page or lost their connection can still
// answer the writes their assistant is waiting on.
//
// It belongs to the session rather than a turn: the turn that asked the
// question ended when the run parked, and the one that will answer it
// does not exist until the user replies.
func (s *session) resendPendingConfirmation(ctx context.Context) {
	pending := s.man.loadPending(ctx, s.key)
	if pending == nil {
		return
	}

	s.writer.WriteJSON(ctx, protocol.NewConfirmRequest(pending.TurnID, pending.Actions))
}

// sendHistory sends the restored conversation to the client so it can
// rebuild the chat pane.
func (s *session) sendHistory(ctx context.Context) {
	entries := buildHistory(s.messages)
	if len(entries) == 0 {
		return
	}

	s.writer.WriteJSON(ctx, protocol.NewHistoryMessage(entries))
}

// Process handles incoming messages from the client, dispatching by
// message type.
func (s *session) Process(ctx context.Context, msg []byte) {
	tp := gjson.GetBytes(msg, "type").String()

	switch protocol.ClientMessageType(tp) {
	case protocol.ClientTypeMessage:
		s.handleMessage(ctx, gjson.GetBytes(msg, "content").String())

	case protocol.ClientTypeReset:
		s.handleReset(ctx)

	case protocol.ClientTypeConfirmResponse:
		var resp protocol.ConfirmResponse

		if err := json.Unmarshal(msg, &resp); err != nil {
			s.writer.WriteJSON(ctx, protocol.NewErrorMessage("invalid confirm response"))

			return
		}

		s.handleConfirmResponse(ctx, resp.TurnID, resp.Approved, resp.All)

	case protocol.ClientTypeSetActiveDocument:
		s.SetActiveDocument(gjson.GetBytes(msg, "documentId").String())

	default:
		s.writer.WriteJSON(ctx, protocol.NewErrorMessage("unknown message type"))
	}
}

// handleMessage starts a turn for a new user message.
func (s *session) handleMessage(ctx context.Context, content string) {
	// an empty message would be rejected by the model and leave the
	// conversation carrying a message every later turn re-sends.
	if strings.TrimSpace(content) == "" {
		s.writer.WriteJSON(ctx, protocol.NewErrorMessage("message content is required"))

		return
	}

	if !s.beginTurn(ctx) {
		return
	}

	// a new message abandons an unanswered confirmation: this turn
	// finishes under the same checkpoint id the parked turn would
	// resume from, so treat the confirmation as declined and clear
	// both records rather than leaving a pending entry whose
	// checkpoint is about to be deleted.
	if s.man.loadPending(ctx, s.key) != nil {
		s.man.clearPending(ctx, s.key)
		s.man.clearCheckpoint(ctx, s.key)
	}

	tn := s.newTurn()

	s.goTurn(ctx, func(turnCtx context.Context) *pendingConfirm {
		s.mu.Lock()
		msgs := append(cloneMessages(s.messages), schema.UserMessage(content))
		s.messages = msgs
		s.mu.Unlock()

		iter := s.runner.Run(turnCtx, msgs, s.runOptions()...)

		return tn.run(turnCtx, iter)
	})
}

// handleReset clears the conversation, provided a turn is not running.
func (s *session) handleReset(ctx context.Context) {
	s.mu.Lock()
	if s.processing {
		s.mu.Unlock()
		s.writer.WriteJSON(ctx, protocol.NewErrorMessage("cannot reset while processing"))

		return
	}

	s.messages = nil
	s.mu.Unlock()

	s.man.clearPending(ctx, s.key)
	s.man.clearCheckpoint(ctx, s.key)

	if err := s.man.history.Delete(ctx, s.key); err != nil {
		s.man.log.Error("failed to delete conversation",
			slog.String("error", err.Error()),
			slog.String("key", s.key),
		)
	}
}

// handleConfirmResponse resumes a turn parked on a confirmation. A
// mismatched turn id is dropped: only the outstanding confirmation can
// be answered, and a stale reply from a previous turn must not fire.
func (s *session) handleConfirmResponse(ctx context.Context, turnID string, approved, all bool) {
	pending := s.man.loadPending(ctx, s.key)
	if pending == nil || pending.TurnID != turnID {
		return
	}

	if !s.beginTurn(ctx) {
		return
	}

	s.man.clearPending(ctx, s.key)

	s.man.log.Info("assistant confirm answered",
		slog.String("turn_id", turnID),
		slog.Bool("approved", approved),
		slog.Bool("all", all),
	)

	// the answer resumes the parked run, so the turn keeps the id the
	// user was asked under.
	tn := &turn{sess: s, id: turnID}

	s.goTurn(ctx, func(turnCtx context.Context) *pendingConfirm {
		targets := make(map[string]any, len(pending.InterruptIDs))
		for _, id := range pending.InterruptIDs {
			targets[id] = tools.Decision{Approved: approved}
		}

		opts := s.runOptions()
		if approved && all {
			opts = append(opts, adk.WithSessionValues(map[string]any{
				tools.SessionKeyAutoApprove: true,
			}))
		}

		iter, err := s.runner.ResumeWithParams(
			turnCtx,
			s.key,
			&adk.ResumeParams{Targets: targets},
			opts...,
		)
		if err != nil {
			tn.fail(turnCtx, "failed to resume the assistant turn", err)

			return nil
		}

		return tn.run(turnCtx, iter)
	})
}

// beginTurn claims the session for a turn, reporting to the client when
// one is already running.
func (s *session) beginTurn(ctx context.Context) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.processing {
		s.writer.WriteJSON(ctx, protocol.NewErrorMessage("already processing a message"))

		return false
	}

	s.processing = true

	return true
}

// goTurn runs one turn on the supervisor, guaranteeing the session is
// released and the client hears about it even if the turn panics.
//
// A turn that parks on a confirmation returns it rather than sending it
// itself, so the session is already free by the time the client is asked.
// Otherwise a fast answer would arrive while the finished turn still held
// the session and be rejected as "already processing".
func (s *session) goTurn(ctx context.Context, fn func(context.Context) *pendingConfirm) {
	s.supv.Go(func(sctx context.Context) {
		defer logutil.Recover(s.man.log, nil)

		var pending *pendingConfirm

		defer func() {
			s.mu.Lock()
			s.processing = false
			s.mu.Unlock()

			if pending != nil {
				// the pending record must survive the connection: its
				// checkpoint is already in Redis, and losing the record
				// would make the parked turn unreachable on reconnect.
				s.man.savePending(context.WithoutCancel(ctx), s.key, *pending)
				s.writer.WriteJSON(ctx, protocol.NewConfirmRequest(pending.TurnID, pending.Actions))
			}

			// a panic skips the turn's own done message, leaving the
			// client waiting for a reply that never arrives.
			if rec := recover(); rec != nil {
				s.writer.WriteJSON(ctx, protocol.NewErrorMessage("the assistant failed to complete the turn"))
				s.writer.WriteJSON(ctx, protocol.NewDoneMessage())

				panic(rec)
			}
		}()

		// Close cancels the supervisor context, so merge it into the
		// connection context: otherwise a closing session cannot
		// interrupt a turn parked on a model stream.
		turnCtx, cancel := context.WithCancel(ctx)
		defer cancel()

		stop := context.AfterFunc(sctx, cancel)
		defer stop()

		pending = fn(turnCtx)
	})
}

// runOptions are the per-run options shared by fresh and resumed turns.
// The active document is snapshotted here so a mid-turn navigation
// cannot shift the system prompt under an in-flight turn.
func (s *session) runOptions() []adk.AgentRunOption {
	s.mu.Lock()
	activeDocumentID := s.activeDocumentID
	s.mu.Unlock()

	return []adk.AgentRunOption{
		adk.WithCheckPointID(s.key),
		adk.WithSessionValues(map[string]any{
			_sessionKeyActiveDocument: activeDocumentID,
		}),
	}
}

// cloneMessages copies the conversation so an in-flight turn cannot be
// mutated by the session underneath it.
func cloneMessages(msgs []*schema.Message) []*schema.Message {
	return append(make([]*schema.Message, 0, len(msgs)+1), msgs...)
}
