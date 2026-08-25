package assistant

import (
	"context"
	"encoding/json"
	"log/slog"
	"maps"
	"strings"
	"sync"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
	"github.com/jellydator/xync"
	"github.com/oxynote/oxynote/server/core/internal/assistant/middleware"
	"github.com/oxynote/oxynote/server/core/internal/assistant/persist"
	"github.com/oxynote/oxynote/server/core/internal/assistant/protocol"
	"github.com/oxynote/oxynote/server/core/internal/assistant/tools"
	"github.com/oxynote/oxynote/server/core/pkg/logutil"
	"github.com/tidwall/gjson"
)

// session holds per-connection state for an AI assistant session.
type session struct {
	man    *Manager
	writer protocol.SessionWriter
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
	key := persist.SessionKey(orgID, userID)
	toolSet := m.ToolSet(orgID, userID)

	s := &session{
		man:    m,
		writer: writer,
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
		CheckPointStore: m.checkpoints,
	})

	s.messages = m.history.Load(ctx, key)
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
	pending := s.man.pendings.Load(ctx, s.key)
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

// buildHistory converts the stored conversation into the entries the
// client restores into its chat pane.
//
// Only what a person said or was told is shown. Tool calls and their
// results stay in the model's context, where they let it remember what
// it already read and changed, but they are machinery rather than
// conversation. An assistant message accompanying a tool call is
// intermediate narration ("Let me look at that document…") and is
// skipped for the same reason.
func buildHistory(messages []*schema.Message) []protocol.HistoryEntry {
	entries := make([]protocol.HistoryEntry, 0, len(messages))

	for _, msg := range messages {
		if msg == nil || msg.Content == "" {
			continue
		}

		switch msg.Role {
		case schema.User:
			entries = append(entries, protocol.HistoryEntry{
				Role:    string(schema.User),
				Content: msg.Content,
			})
		case schema.Assistant:
			if len(msg.ToolCalls) > 0 {
				continue
			}

			entries = append(entries, protocol.HistoryEntry{
				Role:    string(schema.Assistant),
				Content: msg.Content,
			})
		case schema.System, schema.Tool:
			// not part of the visible conversation.
		}
	}

	return entries
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
	if s.man.pendings.Load(ctx, s.key) != nil {
		s.man.pendings.Clear(ctx, s.key)
		s.man.checkpoints.Clear(ctx, s.key)
	}

	tn := s.newTurn()

	s.goTurn(ctx, func(turnCtx context.Context) *persist.PendingConfirm {
		s.mu.Lock()
		msgs := append(cloneMessages(s.messages), schema.UserMessage(content))
		s.messages = msgs
		s.mu.Unlock()

		iter := s.runner.Run(turnCtx, msgs, s.runOptions(nil)...)

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

	s.man.pendings.Clear(ctx, s.key)
	s.man.checkpoints.Clear(ctx, s.key)
	s.man.history.Clear(ctx, s.key)
}

// handleConfirmResponse resumes a turn parked on a confirmation. A
// mismatched turn id is dropped: only the outstanding confirmation can
// be answered, and a stale reply from a previous turn must not fire.
func (s *session) handleConfirmResponse(ctx context.Context, turnID string, approved, all bool) {
	pending := s.man.pendings.Load(ctx, s.key)
	if pending == nil || pending.TurnID != turnID {
		return
	}

	if !s.beginTurn(ctx) {
		return
	}

	s.man.pendings.Clear(ctx, s.key)

	s.man.log.Info("assistant confirm answered",
		slog.String("turn_id", turnID),
		slog.Bool("approved", approved),
		slog.Bool("all", all),
	)

	// the answer resumes the parked run, so the turn keeps the id the
	// user was asked under.
	tn := &turn{sess: s, id: turnID}

	s.goTurn(ctx, func(turnCtx context.Context) *persist.PendingConfirm {
		targets := make(map[string]any, len(pending.InterruptIDs))
		for _, id := range pending.InterruptIDs {
			targets[id] = tools.Decision{Approved: approved}
		}

		var extra map[string]any

		if approved && all {
			extra = map[string]any{
				tools.SessionKeyAutoApprove: true,
			}
		}

		iter, err := s.runner.ResumeWithParams(
			turnCtx,
			s.key,
			&adk.ResumeParams{Targets: targets},
			s.runOptions(extra)...,
		)
		if err != nil {
			// the pending record was the only route back to this
			// checkpoint and is already gone, so a resume that cannot
			// run leaves it unreachable. Reclaim it rather than let a
			// whole conversation sit in Redis until it expires.
			s.man.checkpoints.Clear(turnCtx, s.key)
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
func (s *session) goTurn(ctx context.Context, fn func(context.Context) *persist.PendingConfirm) {
	s.supv.Go(func(sctx context.Context) {
		defer logutil.Recover(s.man.log, nil)

		var pending *persist.PendingConfirm

		defer func() {
			// the pending record must survive the connection: its
			// checkpoint is already in Redis, and losing the record
			// would make the parked turn unreachable on reconnect.
			//
			// It is written before the session is released, because the
			// read loop is a separate goroutine: a message arriving on a
			// free session looks up the pending record to decide whether
			// to abandon a parked turn, and would find nothing while this
			// write was still in flight — then delete the checkpoint this
			// record points at.
			if pending != nil {
				s.man.pendings.Save(context.WithoutCancel(ctx), s.key, *pending)
			}

			s.mu.Lock()
			s.processing = false
			s.mu.Unlock()

			if pending != nil {
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

// runOptions are the per-run options shared by fresh and resumed turns,
// with extra merged into the shared session values. The snapshot
// exists so a mid-turn navigation cannot shift the system prompt under
// an in-flight turn.
func (s *session) runOptions(extra map[string]any) []adk.AgentRunOption {
	s.mu.Lock()
	activeDocumentID := s.activeDocumentID
	s.mu.Unlock()

	values := map[string]any{
		_sessionKeyActiveDocument: activeDocumentID,
	}

	maps.Copy(values, extra)

	return []adk.AgentRunOption{
		adk.WithCheckPointID(s.key),
		adk.WithSessionValues(values),
	}
}

// cloneMessages copies the conversation so an in-flight turn cannot be
// mutated by the session underneath it.
func cloneMessages(msgs []*schema.Message) []*schema.Message {
	return append(make([]*schema.Message, 0, len(msgs)+1), msgs...)
}
