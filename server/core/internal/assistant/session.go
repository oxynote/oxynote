package assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
	"github.com/jellydator/xync"
	"github.com/oxynote/oxynote/server/core/internal/assistant/protocol"
	"github.com/oxynote/oxynote/server/core/internal/assistant/tools"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
	"github.com/rs/xid"
	"github.com/tidwall/gjson"
)

const (
	// _maxAgentTurns caps the agent loop so a runaway model can't
	// burn budget indefinitely.
	_maxAgentTurns = 50

	// _maxMessages caps the conversation history retained in
	// Redis. The trim is applied at the start of each user message.
	_maxMessages = 100

	// _model is the Anthropic model used for chat turns.
	_model = anthropic.ModelClaudeOpus4_6

	// _maxTokens caps the model's response length per turn.
	_maxTokens = 64000

	// _temperature keeps chat turns focused: low enough for
	// deterministic tool use, high enough for natural prose.
	_temperature = 0.3

	// _maxErrorArgsLogBytes caps tool args in error-level logs.
	_maxErrorArgsLogBytes = 512

	// _maxDebugArgsLogBytes caps tool args in debug-level logs.
	_maxDebugArgsLogBytes = 256

	// _maxToolResultBytes caps any single tool_result. Larger
	// results are truncated to a short error so the conversation
	// doesn't balloon.
	_maxToolResultBytes = 80000

	// _confirmTimeout bounds how long the agent waits for a
	// confirm_response from the client. Treated as a decline on
	// expiry — the writes are skipped and the model receives
	// declined results.
	_confirmTimeout = 10 * time.Minute

	// _errorKey is the JSON field carrying an error in tool results and
	// the status label recorded for failed tool calls.
	_errorKey = "error"
)

// Session holds per-connection state for an AI assistant session.
type Session struct {
	man    *Manager
	orgID  string
	userID string
	writer protocol.SessionWriter
	tools  *tools.Manager

	// activeDocumentID is the document the user is currently
	// viewing, surfaced into the system prompt so the model can
	// resolve "this document" without asking.
	activeDocumentID string

	supv     *xync.Supervisor
	messages []anthropic.MessageParam

	mu         sync.Mutex
	processing bool

	// confirmMu guards the per-turn confirmation channel. Only one
	// confirm can be in flight at a time because the agent loop
	// blocks until it receives the response (or the timeout fires).
	confirmMu sync.Mutex
	confirmCh chan bool
	confirmID string

	// autoApproveTurn skips the confirm prompt for the remainder
	// of the current turn — set when the client responds to a
	// confirm with All=true. Reset at the start of every new user
	// message so a fresh turn always re-prompts on the first
	// write batch. Guarded by mu.
	autoApproveTurn bool
}

// Close stops and closes all processes for the session.
func (s *Session) Close() error {
	s.supv.CloseAndWait()
	return nil
}

// SetActiveDocument records which document the user is viewing.
// Called by the chat handler from the WebSocket upgrade path or
// from a "document changed" client message.
func (s *Session) SetActiveDocument(documentID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.activeDocumentID = documentID
}

// sendHistory sends the current message history to the client so it
// can restore previous conversation state.
func (s *Session) sendHistory(ctx context.Context) {
	entries := buildHistory(s.messages)
	if len(entries) == 0 {
		return
	}

	s.writer.WriteJSON(ctx, protocol.NewHistoryMessage(entries))
}

// Process handles incoming messages from the client. It dispatches
// by message type to the appropriate handler.
func (s *Session) Process(ctx context.Context, msg []byte) {
	tp := gjson.GetBytes(msg, "type").String()

	switch protocol.ClientMessageType(tp) {
	case protocol.ClientTypeMessage:
		s.mu.Lock()
		if s.processing {
			s.mu.Unlock()
			s.writer.WriteJSON(ctx, protocol.NewErrorMessage("already processing a message"))

			return
		}

		s.processing = true
		s.mu.Unlock()

		s.supv.Go(func(_ context.Context) {
			s.handleUserMessage(
				ctx,
				gjson.GetBytes(msg, "content").String(),
			)

			s.mu.Lock()
			s.processing = false
			s.mu.Unlock()
		})

	case protocol.ClientTypeReset:
		s.mu.Lock()
		if s.processing {
			s.mu.Unlock()
			s.writer.WriteJSON(ctx, protocol.NewErrorMessage("cannot reset while processing"))

			return
		}

		s.messages = nil
		s.mu.Unlock()

		s.man.deleteMessages(ctx, s.orgID, s.userID)

	case protocol.ClientTypeConfirmResponse:
		s.deliverConfirmResponse(
			gjson.GetBytes(msg, "turn_id").String(),
			gjson.GetBytes(msg, "approved").Bool(),
			gjson.GetBytes(msg, "all").Bool(),
		)

	case protocol.ClientTypeSetActiveDocument:
		s.SetActiveDocument(gjson.GetBytes(msg, "document_id").String())

	default:
		s.writer.WriteJSON(ctx, protocol.NewErrorMessage("unknown message type"))
	}
}

// handleUserMessage runs the agent loop for a single user message.
func (s *Session) handleUserMessage(ctx context.Context, content string) {
	s.trimMessages()

	checkpoint := len(s.messages)

	s.messages = append(s.messages, anthropic.NewUserMessage(
		anthropic.NewTextBlock(content),
	))

	// Snapshot the active document so a mid-turn navigation by the
	// user doesn't shift the system prompt under the in-flight turn.
	// Also reset auto-approve so a fresh turn always re-prompts.
	s.mu.Lock()
	activeDocumentID := s.activeDocumentID
	s.autoApproveTurn = false
	s.mu.Unlock()

	for turn := range _maxAgentTurns {
		stopReason, err := s.callAnthropic(ctx, activeDocumentID)
		if err != nil {
			s.man.log.Error(
				"anthropic call failed",
				slog.String("error", err.Error()),
				slog.Int("turn", turn),
			)

			// Roll back to the checkpoint so the next turn
			// doesn't inherit a partial tool dialogue.
			s.messages = s.messages[:checkpoint]

			s.writer.WriteJSON(ctx, protocol.NewErrorMessage("failed to execute AI request"))
			s.writer.WriteJSON(ctx, protocol.NewDoneMessage())

			return
		}

		if stopReason != anthropic.StopReasonToolUse {
			s.messages = stripToolMessages(s.messages)
			s.man.saveMessages(ctx, s.orgID, s.userID, s.messages)
			s.writer.WriteJSON(ctx, protocol.NewDoneMessage())

			return
		}
	}

	s.messages = stripToolMessages(s.messages)
	s.man.saveMessages(ctx, s.orgID, s.userID, s.messages)
	s.writer.WriteJSON(ctx, protocol.NewErrorMessage("maximum agent turns reached"))
	s.writer.WriteJSON(ctx, protocol.NewDoneMessage())
}

// callAnthropic streams one Anthropic call, relays text deltas to
// the client, collects any tool_use blocks, runs reads
// concurrently, gates writes behind a confirm prompt, and feeds the
// results back into the conversation. Returns the stop reason so
// the outer loop knows whether another turn is needed.
func (s *Session) callAnthropic(ctx context.Context, activeDocumentID string) (anthropic.StopReason, error) {
	stream := s.man.client.Messages.NewStreaming(ctx, anthropic.MessageNewParams{
		Model:     _model,
		MaxTokens: _maxTokens,
		Temperature: param.Opt[float64]{
			Value: _temperature,
		},
		System: []anthropic.TextBlockParam{
			{
				Text: buildSystemPrompt(activeDocumentID),
				CacheControl: anthropic.CacheControlEphemeralParam{
					TTL: anthropic.CacheControlEphemeralTTLTTL1h,
				},
			},
		},
		Messages: s.messages,
		Tools:    tools.AnthropicTools(),
	})
	defer stream.Close() //nolint:errcheck // error provides no meaningful info

	var (
		assistantBlocks []anthropic.ContentBlockParamUnion
		stopReason      anthropic.StopReason

		// Track text accumulation for conversation history.
		textAccum strings.Builder
		inText    bool

		// textStreamed is true once any text_delta has been sent
		// out — used to decide whether a closing text_end is owed
		// to the client at the end of the call.
		textStreamed bool

		// Active tool_use being assembled from streaming deltas.
		activeTool struct {
			id   string
			name string
			json strings.Builder
		}
	)

	for stream.Next() {
		evt := stream.Current()

		switch evt.Type {
		case "content_block_start":
			switch evt.ContentBlock.Type {
			case "text":
				inText = true

				textAccum.Reset()
			case "tool_use":
				activeTool.id = evt.ContentBlock.ID
				activeTool.name = evt.ContentBlock.Name
				activeTool.json.Reset()
			}

		case "content_block_delta":
			switch evt.Delta.Type {
			case "text_delta":
				s.writer.WriteJSON(ctx, protocol.NewTextDeltaMessage(evt.Delta.Text))

				textStreamed = true

				if inText {
					textAccum.WriteString(evt.Delta.Text)
				}
			case "input_json_delta":
				activeTool.json.WriteString(evt.Delta.PartialJSON)
			}

		case "content_block_stop":
			if inText {
				assistantBlocks = append(assistantBlocks,
					anthropic.NewTextBlock(textAccum.String()),
				)

				inText = false
			}

			if activeTool.id != "" {
				inputJSON := json.RawMessage(activeTool.json.String())
				if len(inputJSON) == 0 {
					inputJSON = json.RawMessage("{}")
				}

				assistantBlocks = append(assistantBlocks, anthropic.ContentBlockParamUnion{
					OfToolUse: &anthropic.ToolUseBlockParam{
						ID:    activeTool.id,
						Name:  activeTool.name,
						Input: inputJSON,
					},
				})

				activeTool.id = ""
				activeTool.name = ""
				activeTool.json.Reset()
			}

		case "message_delta":
			stopReason = evt.Delta.StopReason
			s.man.metrics.recordTokenUsage(
				evt.Usage.InputTokens,
				evt.Usage.OutputTokens,
				evt.Usage.CacheCreationInputTokens,
				evt.Usage.CacheReadInputTokens,
			)
		}
	}

	if err := stream.Err(); err != nil {
		return "", err
	}

	// Signal end of this call's text run so the client can move
	// intermediate narration into a transient status pill and only
	// commit final answers into the persistent chat.
	if textStreamed {
		kind := protocol.TextEndKindMessage
		if stopReason == anthropic.StopReasonToolUse {
			kind = protocol.TextEndKindStatus
		}

		s.writer.WriteJSON(ctx, protocol.NewTextEndMessage(kind))
	}

	if stopReason != anthropic.StopReasonToolUse {
		s.messages = append(s.messages, anthropic.MessageParam{
			Role:    anthropic.MessageParamRoleAssistant,
			Content: assistantBlocks,
		})

		return stopReason, nil
	}

	// Persist the assistant turn (text + tool_use blocks) before
	// running the tools so a tool failure doesn't lose what the
	// model said.
	s.messages = append(s.messages, anthropic.MessageParam{
		Role:    anthropic.MessageParamRoleAssistant,
		Content: assistantBlocks,
	})

	toolResults := s.dispatchTools(ctx, assistantBlocks)

	s.messages = append(s.messages, anthropic.MessageParam{
		Role:    anthropic.MessageParamRoleUser,
		Content: toolResults,
	})

	// Prune older read_block / read_document_summary results from
	// history when a newer call for the same (tool, doc, block)
	// supersedes them — keeps the conversation cheap.
	s.pruneStaleReads()

	return stopReason, nil
}

// dispatchTools splits the streamed tool_use blocks into reads and
// writes, runs reads concurrently, gates writes behind a confirm
// prompt, and returns the tool_result blocks in input order so the
// next Anthropic call sees a consistent dialogue.
func (s *Session) dispatchTools(
	ctx context.Context,
	assistantBlocks []anthropic.ContentBlockParamUnion,
) []anthropic.ContentBlockParamUnion {
	type call struct {
		// idx is the position of the result block in the returned
		// slice; preserved so the order matches input.
		idx   int
		id    string
		name  string
		args  json.RawMessage
		write bool
	}

	var (
		calls        []call
		writeIndexes []int
	)

	for _, block := range assistantBlocks {
		if block.OfToolUse == nil {
			continue
		}

		args, _ := block.OfToolUse.Input.(json.RawMessage)
		if args == nil {
			args = json.RawMessage("{}")
		}

		c := call{
			idx:   len(calls),
			id:    block.OfToolUse.ID,
			name:  block.OfToolUse.Name,
			args:  args,
			write: tools.IsWrite(tools.Name(block.OfToolUse.Name)),
		}

		if c.write {
			writeIndexes = append(writeIndexes, c.idx)
		}

		calls = append(calls, c)
	}

	results := make([]anthropic.ContentBlockParamUnion, len(calls))

	approved := true

	if len(writeIndexes) > 0 {
		s.mu.Lock()
		autoApprove := s.autoApproveTurn
		s.mu.Unlock()

		if !autoApprove {
			actions := make([]protocol.ConfirmAction, 0, len(writeIndexes))
			for _, i := range writeIndexes {
				summary := s.tools.Summarize(ctx, tools.Name(calls[i].name), calls[i].args)
				actions = append(actions, protocol.ConfirmAction{
					Tool:         calls[i].name,
					DocumentID:   summary.DocumentID,
					DocumentName: summary.DocumentName,
					Summary:      summary.Summary,
				})
			}

			approved = s.requestConfirmation(ctx, actions)
		}
	}

	var wg sync.WaitGroup

	for i := range calls {
		c := calls[i]

		if c.write && !approved {
			results[c.idx] = makeToolResult(c.id, mustJSON(map[string]any{
				"approved": false,
				"reason":   "user declined",
			}))

			continue
		}

		wg.Go(func() {
			results[c.idx] = s.runTool(ctx, c.id, tools.Name(c.name), c.args)
		})
	}

	wg.Wait()

	return results
}

// runTool executes one tool and packages its outcome as a
// tool_result block. The result envelope is JSON: the success path
// is the tool's own JSON output; the failure path is
// {"error": "..."}.
func (s *Session) runTool(
	ctx context.Context,
	id string,
	name tools.Name,
	args json.RawMessage,
) anthropic.ContentBlockParamUnion {
	if !tools.IsValid(name) {
		s.man.log.Warn("assistant called unknown tool",
			slog.String("tool", string(name)),
		)

		return makeToolResult(id, mustJSON(map[string]any{_errorKey: "unknown tool"}))
	}

	// Surface a short pill to the client describing what we're
	// about to do. Skipped for tools without a friendly label
	// (list_documents, get_document, set_document_icon).
	if label := s.tools.ToolStatusLabel(ctx, name, args); label != "" {
		s.writer.WriteJSON(ctx, protocol.NewToolStatusMessage(string(name), label))
	}

	start := timeutil.Now()
	result, err := s.tools.Execute(ctx, name, args)
	durationMs := time.Since(start).Milliseconds()

	status := "success"

	if err != nil {
		status = _errorKey
		result = mustJSON(map[string]any{_errorKey: err.Error()})

		s.man.log.Warn("assistant tool error",
			slog.String("tool", string(name)),
			slog.String("error", err.Error()),
			slog.String("args", truncateForLog(args, _maxErrorArgsLogBytes)),
			slog.Int64("duration_ms", durationMs),
		)
	}

	if len(result) > _maxToolResultBytes {
		result = mustJSON(map[string]any{
			_errorKey: "result too large; use a more specific query to narrow it down",
		})

		status = _errorKey

		s.man.log.Warn("assistant tool result truncated (too large)",
			slog.String("tool", string(name)),
			slog.Int("size_bytes", len(result)),
		)
	}

	if err == nil {
		s.man.log.Debug("assistant tool ok",
			slog.String("tool", string(name)),
			slog.String("args", truncateForLog(args, _maxDebugArgsLogBytes)),
			slog.Int64("duration_ms", durationMs),
		)
	}

	s.man.metrics.recordToolCall(name, status)
	s.man.metrics.recordToolDuration(name, time.Since(start).Seconds())

	return makeToolResult(id, result)
}

// truncateForLog returns a short, single-line preview of raw for
// inclusion in log fields. Strings longer than maxLen are tail-elided
// with an ellipsis so a long tool argument doesn't fill the log.
func truncateForLog(raw json.RawMessage, maxLen int) string {
	if len(raw) <= maxLen {
		return string(raw)
	}

	return string(raw[:maxLen]) + "…"
}

// requestConfirmation sends a confirm_request and blocks until the
// client returns a confirm_response (or the timeout fires). Returns
// true when the user approved the batch, false otherwise.
func (s *Session) requestConfirmation(ctx context.Context, actions []protocol.ConfirmAction) bool {
	turnID := xid.New().String()
	ch := make(chan bool, 1)

	s.confirmMu.Lock()
	s.confirmCh = ch
	s.confirmID = turnID
	s.confirmMu.Unlock()

	defer func() {
		s.confirmMu.Lock()
		s.confirmCh = nil
		s.confirmID = ""
		s.confirmMu.Unlock()
	}()

	toolNames := make([]string, 0, len(actions))
	for _, a := range actions {
		toolNames = append(toolNames, a.Tool)
	}

	s.man.log.Info("assistant confirm requested",
		slog.String("turn_id", turnID),
		slog.Int("action_count", len(actions)),
		slog.Any("tools", toolNames),
	)

	s.writer.WriteJSON(ctx, protocol.NewConfirmRequest(turnID, actions))

	select {
	case approved := <-ch:
		s.man.log.Info("assistant confirm answered",
			slog.String("turn_id", turnID),
			slog.Bool("approved", approved),
		)

		return approved
	case <-time.After(_confirmTimeout):
		s.man.log.Warn("assistant confirm timeout",
			slog.String("turn_id", turnID),
			slog.Duration("timeout", _confirmTimeout),
		)

		return false
	case <-ctx.Done():
		s.man.log.Info("assistant confirm cancelled by context",
			slog.String("turn_id", turnID),
		)

		return false
	}
}

// deliverConfirmResponse routes a client confirm_response into the
// pending confirmation channel. A mismatched turn id is dropped
// silently — only the in-flight confirm reads the channel, and
// stale answers from prior turns shouldn't fire. When all is set
// alongside approved, the rest of the current turn skips the
// prompt entirely.
func (s *Session) deliverConfirmResponse(turnID string, approved, all bool) {
	s.confirmMu.Lock()
	defer s.confirmMu.Unlock()

	if s.confirmCh == nil || s.confirmID != turnID {
		return
	}

	if approved && all {
		s.mu.Lock()
		s.autoApproveTurn = true
		s.mu.Unlock()
	}

	// Non-blocking send: the channel is buffered (cap 1); a second
	// answer for the same turn is dropped.
	select {
	case s.confirmCh <- approved:
	default:
	}
}

// pruneStaleReads collapses older read tool results when a newer
// call for the same (tool, document, block) supersedes them. This
// keeps the conversation history small even when the model
// re-reads the same document across turns.
func (s *Session) pruneStaleReads() {
	type readKey struct {
		tool, doc, block string
	}

	// Walk backwards and record the latest tool_use for each
	// read-tool key. Earlier matches (smaller positions) are the
	// ones we'll prune.
	lastSeen := map[readKey]int{}

	for i, msg := range s.messages {
		if msg.Role != anthropic.MessageParamRoleAssistant {
			continue
		}

		for _, block := range msg.Content {
			if block.OfToolUse == nil {
				continue
			}

			name := tools.Name(block.OfToolUse.Name)
			if name != tools.NameReadBlock &&
				name != tools.NameReadDocumentSummary {
				continue
			}

			args, _ := block.OfToolUse.Input.(json.RawMessage)

			var probe struct {
				DocumentID string `json:"document_id"`
				BlockUID   string `json:"block_uid"`
			}
			if err := json.Unmarshal(args, &probe); err != nil {
				// Malformed args from a prior turn shouldn't block
				// pruning the rest of the history. Log and skip this
				// entry; probe stays at zero values, so its readKey
				// will just collide with any other malformed read.
				s.man.log.Warn("pruneStaleReads: probe unmarshal failed",
					slog.String("tool", string(name)),
					slog.String("tool_use_id", block.OfToolUse.ID),
					slog.String("error", err.Error()),
				)

				continue
			}

			k := readKey{tool: string(name), doc: probe.DocumentID, block: probe.BlockUID}
			if last, ok := lastSeen[k]; ok && last != i {
				s.prunePriorReadResult(last, block.OfToolUse.ID)
			}

			lastSeen[k] = i
		}
	}
}

// prunePriorReadResult replaces an older read result (the user
// message following the assistant turn at assistantIdx) with a
// compact placeholder, keeping the conversation honest about what
// was once read without paying tokens to keep the full content.
func (s *Session) prunePriorReadResult(assistantIdx int, toolUseID string) {
	resultIdx := assistantIdx + 1
	if resultIdx >= len(s.messages)-1 {
		return
	}

	resultMsg := &s.messages[resultIdx]
	for j, rb := range resultMsg.Content {
		if rb.OfToolResult == nil || rb.OfToolResult.ToolUseID != toolUseID {
			continue
		}

		resultMsg.Content[j].OfToolResult.Content = []anthropic.ToolResultBlockParamContentUnion{
			{
				OfText: &anthropic.TextBlockParam{
					Text: `{"pruned": "A newer read for the same target exists later in the conversation. Call the read tool again if you need current content."}`,
				},
			},
		}

		return
	}
}

// trimMessages keeps the conversation history under _maxMessages.
// We trim from the front in pairs to avoid stranding a user message
// without its assistant follow-up (or vice versa).
func (s *Session) trimMessages() {
	if len(s.messages) <= _maxMessages {
		return
	}

	excess := len(s.messages) - _maxMessages
	if excess%2 != 0 {
		excess++
	}

	s.messages = s.messages[excess:]
}

// makeToolResult wraps a json payload as a ToolResultBlockParam.
func makeToolResult(toolUseID string, result json.RawMessage) anthropic.ContentBlockParamUnion {
	return anthropic.ContentBlockParamUnion{
		OfToolResult: &anthropic.ToolResultBlockParam{
			ToolUseID: toolUseID,
			Content: []anthropic.ToolResultBlockParamContentUnion{
				{
					OfText: &anthropic.TextBlockParam{
						Text: string(result),
					},
				},
			},
		},
	}
}

// mustJSON marshals v as JSON; on failure (which only happens for
// malformed input) it returns a hand-rolled error envelope so the
// session never panics.
func mustJSON(v any) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage(fmt.Sprintf(`{"error": %q}`, err.Error()))
	}

	return data
}
