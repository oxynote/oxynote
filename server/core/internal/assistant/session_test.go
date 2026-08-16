package assistant

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/jellydator/xync"
	"github.com/oxynote/oxynote/server/core/internal/assistant/edit"
	"github.com/oxynote/oxynote/server/core/internal/assistant/protocol"
	protocolMock "github.com/oxynote/oxynote/server/core/internal/assistant/protocol/_mock"
	"github.com/oxynote/oxynote/server/core/internal/assistant/tools"
	toolsMock "github.com/oxynote/oxynote/server/core/internal/assistant/tools/_mock"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/pkg/metricutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sseEvent renders one server-sent event frame.
func sseEvent(name, data string) string {
	return "event: " + name + "\ndata: " + data + "\n\n"
}

// sseTextTurn renders a streamed Anthropic response carrying a
// single text block that ends with the given stop reason.
func sseTextTurn(text string) string {
	return sseEvent("message_start",
		`{"type":"message_start","message":{"id":"m1","type":"message","role":"assistant","content":[],"model":"claude","usage":{"input_tokens":1,"output_tokens":1}}}`) +
		sseEvent("content_block_start",
			`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`) +
		sseEvent("content_block_delta",
			`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"`+text+`"}}`) +
		sseEvent("content_block_stop", `{"type":"content_block_stop","index":0}`) +
		sseEvent("message_delta",
			`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":3,"output_tokens":5,"cache_creation_input_tokens":2,"cache_read_input_tokens":4}}`) +
		sseEvent("message_stop", `{"type":"message_stop"}`)
}

// sseToolTurn renders a streamed response invoking one tool. An
// empty args string omits the input_json_delta event entirely so
// the empty-input fallback path is exercised.
func sseToolTurn(toolName, args string) string {
	body := sseEvent("message_start",
		`{"type":"message_start","message":{"id":"m1","type":"message","role":"assistant","content":[],"model":"claude","usage":{"input_tokens":1,"output_tokens":1}}}`) +
		sseEvent("content_block_start",
			`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"tu1","name":"`+toolName+`","input":{}}}`)

	if args != "" {
		body += sseEvent("content_block_delta",
			`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":`+strconv(args)+`}}`)
	}

	return body +
		sseEvent("content_block_stop", `{"type":"content_block_stop","index":0}`) +
		sseEvent("message_delta",
			`{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"input_tokens":1,"output_tokens":1,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}`) +
		sseEvent("message_stop", `{"type":"message_stop"}`)
}

// strconv JSON-quotes a string for embedding in an SSE data frame.
func strconv(s string) string {
	out, err := json.Marshal(s)
	if err != nil {
		return `""`
	}

	return string(out)
}

// anthClient starts a fake Anthropic endpoint whose n-th call (1
// based) answers with the body returned by respond. Retries are
// disabled so 5xx responses surface immediately.
func anthClient(t *testing.T, respond func(call int) (int, string)) *anthropic.Client {
	t.Helper()

	var calls atomic.Int64

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		status, body := respond(int(calls.Add(1)))

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(status)

		_, err := w.Write([]byte(body))
		assert.NoError(t, err)
	}))
	t.Cleanup(srv.Close)

	client := anthropic.NewClient(
		option.WithBaseURL(srv.URL),
		option.WithAPIKey("test"),
		option.WithMaxRetries(0),
	)

	return &client
}

// newTestSession wires a Session around the given collaborators,
// defaulting any nil dependency to a benign stub.
func newTestSession(
	client *anthropic.Client,
	writer protocol.SessionWriter,
	db tools.DB,
	store SessionStore,
	log *slog.Logger,
) *session {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}

	if store == nil {
		store = &SessionStoreMock{}
	}

	if db == nil {
		db = &toolsMock.DB{}
	}

	man := &Manager{
		log:     log,
		store:   store,
		client:  client,
		metrics: newMetrics(metricutil.NewFactory("test", nil)),
	}

	return &session{
		man:    man,
		orgID:  "org",
		userID: "user",
		writer: writer,
		supv:   xync.NewSupervisor(),
		tools: tools.NewManager(
			log,
			db,
			&toolsMock.Searcher{},
			&toolsMock.EditApplier{},
			&toolsMock.TreeNotifier{},
			"org",
			"user",
		),
	}
}

// writtenTypes extracts the protocol message type of every WriteJSON
// call for order assertions.
func writtenTypes(writer *protocolMock.SessionWriter) []string {
	var out []string

	for _, call := range writer.WriteJSONCalls() {
		switch msg := call.Msg.(type) {
		case protocol.TextDeltaMessage:
			out = append(out, string(msg.Type))
		case protocol.TextEndMessage:
			out = append(out, string(msg.Type)+":"+string(msg.Kind))
		case protocol.ToolStatusMessage:
			out = append(out, string(msg.Type))
		case protocol.DoneMessage:
			out = append(out, string(msg.Type))
		case protocol.ErrorMessage:
			out = append(out, string(msg.Type)+":"+msg.Message)
		case protocol.HistoryMessage:
			out = append(out, string(msg.Type))
		case protocol.ConfirmRequest:
			out = append(out, string(msg.Type))
		}
	}

	return out
}

func Test_Session_Close(t *testing.T) {
	t.Parallel()

	s := newTestSession(nil, &protocolMock.SessionWriter{}, nil, nil, nil)
	assert.NoError(t, s.Close())
}

func Test_Session_SetActiveDocument(t *testing.T) {
	t.Parallel()

	s := newTestSession(nil, &protocolMock.SessionWriter{}, nil, nil, nil)
	s.SetActiveDocument("doc-1")

	assert.Equal(t, "doc-1", s.activeDocumentID)
}

func Test_Session_sendHistory(t *testing.T) {
	t.Parallel()

	// empty history sends nothing
	writer := &protocolMock.SessionWriter{}
	s := newTestSession(nil, writer, nil, nil, nil)

	s.sendHistory(context.Background())
	assert.Empty(t, writer.WriteJSONCalls())

	// non-empty history replays as one history message
	s.messages = []anthropic.MessageParam{userMsg("hi")}

	s.sendHistory(context.Background())
	assert.Equal(t, []string{"history"}, writtenTypes(writer))
}

func Test_Session_Process(t *testing.T) {
	t.Run("Unknown message type", func(t *testing.T) {
		t.Parallel()

		writer := &protocolMock.SessionWriter{}
		s := newTestSession(nil, writer, nil, nil, nil)

		s.Process(context.Background(), []byte(`{"type":"wibble"}`))
		assert.Equal(t, []string{"error:unknown message type"}, writtenTypes(writer))
	})

	t.Run("Set active document", func(t *testing.T) {
		t.Parallel()

		s := newTestSession(nil, &protocolMock.SessionWriter{}, nil, nil, nil)

		s.Process(context.Background(), []byte(`{"type":"set_active_document","document_id":"doc-9"}`))
		assert.Equal(t, "doc-9", s.activeDocumentID)
	})

	t.Run("Confirm response routed to pending confirm", func(t *testing.T) {
		t.Parallel()

		s := newTestSession(nil, &protocolMock.SessionWriter{}, nil, nil, nil)

		ch := make(chan bool, 1)
		s.confirmCh = ch
		s.confirmID = "turn-1"

		s.Process(context.Background(), []byte(`{"type":"confirm_response","turn_id":"turn-1","approved":true}`))

		select {
		case approved := <-ch:
			assert.True(t, approved)
		default:
			t.Fatal("confirm response was not delivered")
		}
	})

	t.Run("Reset clears history", func(t *testing.T) {
		t.Parallel()

		store := &SessionStoreMock{}
		s := newTestSession(nil, &protocolMock.SessionWriter{}, nil, store, nil)
		s.messages = []anthropic.MessageParam{userMsg("hi")}

		s.Process(context.Background(), []byte(`{"type":"reset"}`))

		assert.Nil(t, s.messages)
		assert.Len(t, store.DeleteCalls(), 1)
	})

	t.Run("Reset while processing is rejected", func(t *testing.T) {
		t.Parallel()

		writer := &protocolMock.SessionWriter{}
		s := newTestSession(nil, writer, nil, nil, nil)
		s.processing = true

		s.Process(context.Background(), []byte(`{"type":"reset"}`))
		assert.Equal(t, []string{"error:cannot reset while processing"}, writtenTypes(writer))
	})

	t.Run("Message while processing is rejected", func(t *testing.T) {
		t.Parallel()

		writer := &protocolMock.SessionWriter{}
		s := newTestSession(nil, writer, nil, nil, nil)
		s.processing = true

		s.Process(context.Background(), []byte(`{"type":"message","content":"hi"}`))
		assert.Equal(t, []string{"error:already processing a message"}, writtenTypes(writer))
	})

	t.Run("Message runs a full turn", func(t *testing.T) {
		t.Parallel()

		client := anthClient(t, func(_ int) (int, string) {
			return http.StatusOK, sseTextTurn("hello")
		})

		writer := &protocolMock.SessionWriter{}
		store := &SessionStoreMock{}
		s := newTestSession(client, writer, nil, store, nil)

		s.Process(context.Background(), []byte(`{"type":"message","content":"hi"}`))

		// drain without cancelling: Close interrupts a running turn.
		s.supv.Wait()

		require.NoError(t, s.Close())

		assert.Equal(t, []string{"text_delta", "text_end:message", "done"}, writtenTypes(writer))
		assert.Len(t, store.SetCalls(), 1)
		assert.False(t, s.processing)
	})
}

func Test_Session_handleUserMessage(t *testing.T) {
	t.Run("Anthropic failure rolls back the turn", func(t *testing.T) {
		t.Parallel()

		client := anthClient(t, func(_ int) (int, string) {
			return http.StatusInternalServerError, `{"error":"boom"}`
		})

		writer := &protocolMock.SessionWriter{}
		s := newTestSession(client, writer, nil, nil, nil)

		s.handleUserMessage(context.Background(), "hi")

		assert.Empty(t, s.messages)
		assert.Equal(t, []string{"error:failed to execute AI request", "done"}, writtenTypes(writer))
	})

	t.Run("Successful turn saves stripped history", func(t *testing.T) {
		t.Parallel()

		client := anthClient(t, func(_ int) (int, string) {
			return http.StatusOK, sseTextTurn("answer")
		})

		writer := &protocolMock.SessionWriter{}
		store := &SessionStoreMock{}
		s := newTestSession(client, writer, nil, store, nil)

		s.handleUserMessage(context.Background(), "hi")

		require.Len(t, s.messages, 2)
		assert.Equal(t, anthropic.MessageParamRoleUser, s.messages[0].Role)
		assert.Equal(t, anthropic.MessageParamRoleAssistant, s.messages[1].Role)
		assert.Len(t, store.SetCalls(), 1)
		assert.Equal(t, []string{"text_delta", "text_end:message", "done"}, writtenTypes(writer))
	})

	t.Run("Maximum turns reached", func(t *testing.T) {
		// mutates the package-level turn cap; must not run parallel
		orig := _maxAgentTurns
		_maxAgentTurns = 2

		t.Cleanup(func() { _maxAgentTurns = orig })

		client := anthClient(t, func(_ int) (int, string) {
			return http.StatusOK, sseToolTurn("list_documents", "{}")
		})

		writer := &protocolMock.SessionWriter{}
		db := &toolsMock.DB{}
		s := newTestSession(client, writer, db, nil, nil)

		s.handleUserMessage(context.Background(), "hi")

		types := writtenTypes(writer)
		assert.Contains(t, types, "error:maximum agent turns reached")
		assert.Equal(t, "done", types[len(types)-1])
		assert.Len(t, db.FetchDocumentTreeCalls(), 2)
	})
}

func Test_Session_callAnthropic(t *testing.T) {
	t.Run("Text turn relays deltas and records the answer", func(t *testing.T) {
		t.Parallel()

		client := anthClient(t, func(_ int) (int, string) {
			return http.StatusOK, sseTextTurn("hello")
		})

		writer := &protocolMock.SessionWriter{}
		s := newTestSession(client, writer, nil, nil, nil)

		stop, err := s.callAnthropic(context.Background(), "")
		require.NoError(t, err)

		assert.Equal(t, anthropic.StopReasonEndTurn, stop)
		assert.Equal(t, []string{"text_delta", "text_end:message"}, writtenTypes(writer))

		require.Len(t, s.messages, 1)
		require.Len(t, s.messages[0].Content, 1)
		assert.Equal(t, "hello", s.messages[0].Content[0].OfText.Text)
	})

	t.Run("Narration before tools ends as a status pill", func(t *testing.T) {
		t.Parallel()

		// one turn carrying narration text and a tool_use block
		body := sseEvent("message_start",
			`{"type":"message_start","message":{"id":"m1","type":"message","role":"assistant","content":[],"model":"claude","usage":{"input_tokens":1,"output_tokens":1}}}`) +
			sseEvent("content_block_start",
				`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`) +
			sseEvent("content_block_delta",
				`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"let me look"}}`) +
			sseEvent("content_block_stop", `{"type":"content_block_stop","index":0}`) +
			sseEvent("content_block_start",
				`{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"tu1","name":"list_documents","input":{}}}`) +
			sseEvent("content_block_stop", `{"type":"content_block_stop","index":1}`) +
			sseEvent("message_delta",
				`{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"input_tokens":1,"output_tokens":1,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}`) +
			sseEvent("message_stop", `{"type":"message_stop"}`)

		client := anthClient(t, func(_ int) (int, string) {
			return http.StatusOK, body
		})

		writer := &protocolMock.SessionWriter{}
		s := newTestSession(client, writer, nil, nil, nil)

		stop, err := s.callAnthropic(context.Background(), "")
		require.NoError(t, err)

		assert.Equal(t, anthropic.StopReasonToolUse, stop)
		assert.Equal(t, []string{"text_delta", "text_end:status"}, writtenTypes(writer))
	})

	t.Run("Tool turn executes the tool and appends results", func(t *testing.T) {
		t.Parallel()

		client := anthClient(t, func(_ int) (int, string) {
			// empty args exercise the missing-input fallback
			return http.StatusOK, sseToolTurn("list_documents", "")
		})

		writer := &protocolMock.SessionWriter{}
		db := &toolsMock.DB{}
		s := newTestSession(client, writer, db, nil, nil)

		stop, err := s.callAnthropic(context.Background(), "")
		require.NoError(t, err)

		assert.Equal(t, anthropic.StopReasonToolUse, stop)
		assert.Len(t, db.FetchDocumentTreeCalls(), 1)

		// assistant tool_use turn plus the tool_result user turn
		require.Len(t, s.messages, 2)
		assert.Equal(t, anthropic.MessageParamRoleAssistant, s.messages[0].Role)
		assert.Equal(t, anthropic.MessageParamRoleUser, s.messages[1].Role)

		// pure tool turns stream no text, so no text_end is owed
		assert.Empty(t, writtenTypes(writer))
	})

	t.Run("Stream failure returns the error", func(t *testing.T) {
		t.Parallel()

		client := anthClient(t, func(_ int) (int, string) {
			return http.StatusInternalServerError, `{"error":"boom"}`
		})

		s := newTestSession(client, &protocolMock.SessionWriter{}, nil, nil, nil)

		_, err := s.callAnthropic(context.Background(), "")
		require.Error(t, err)
	})
}

func Test_Session_Close_interruptsPendingConfirmation(t *testing.T) {
	t.Parallel()

	client := anthClient(t, func(_ int) (int, string) {
		return http.StatusOK, sseToolTurn("delete_document", `{"document_id":"doc-1"}`)
	})

	s := newTestSession(client, &protocolMock.SessionWriter{}, nil, nil, nil)

	s.Process(context.Background(), []byte(`{"type":"message","content":"hi"}`))

	// no completion signal exists for "the turn has parked on the confirm
	// prompt", so poll for the pending channel the prompt installs.
	require.Eventually(t, func() bool {
		s.confirmMu.Lock()
		defer s.confirmMu.Unlock()

		return s.confirmCh != nil
	}, 5*time.Second, time.Millisecond)

	done := make(chan struct{})

	go func() {
		defer close(done)

		assert.NoError(t, s.Close())
	}()

	// Close cancels the turn instead of waiting out _confirmTimeout, which
	// is ten minutes.
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Close did not interrupt the pending confirmation")
	}
}

func Test_Session_dispatchTools_writesRunSequentially(t *testing.T) {
	t.Parallel()

	var s *session

	writer := &protocolMock.SessionWriter{
		WriteJSONFunc: func(_ context.Context, msg any) {
			if req, ok := msg.(protocol.ConfirmRequest); ok {
				s.deliverConfirmResponse(req.TurnID, true, false)
			}
		},
	}

	db := &toolsMock.DB{
		FetchDocumentFunc: func(_ context.Context, id xid.ID, _, _ string) (*document.Document, error) {
			return &document.Document{
				ID:     id,
				Branch: document.Branch{BranchID: xid.New()},
			}, nil
		},
	}

	var (
		active     atomic.Int32
		overlapped atomic.Bool
		orderMu    sync.Mutex
		order      []string
	)

	applier := &toolsMock.EditApplier{
		ApplyFunc: func(_ context.Context, documentID, _ string, _ []edit.Operation) (edit.Result, error) {
			if active.Add(1) > 1 {
				overlapped.Store(true)
			}

			// widen the window a concurrent second write would land in.
			// Sequential execution can never overlap, so this only ever
			// slows the test down.
			time.Sleep(20 * time.Millisecond)

			orderMu.Lock()

			order = append(order, documentID)
			orderMu.Unlock()

			active.Add(-1)

			return edit.Result{Applied: 1}, nil
		},
	}

	first, second := xid.New(), xid.New()

	s = newTestSession(nil, writer, db, nil, nil)
	s.tools = tools.NewManager(
		slog.New(slog.DiscardHandler),
		db,
		&toolsMock.Searcher{},
		applier,
		&toolsMock.TreeNotifier{},
		"org",
		"user",
	)

	results := s.dispatchTools(context.Background(), []anthropic.ContentBlockParamUnion{
		toolUseBlock("t1", "append_block",
			`{"document_id":"`+first.String()+`","block":{"type":"paragraph","text":"a"}}`),
		toolUseBlock("t2", "append_block",
			`{"document_id":"`+second.String()+`","block":{"type":"paragraph","text":"b"}}`),
	})

	require.Len(t, results, 2)
	assert.False(t, overlapped.Load(), "approved writes must not run concurrently")
	assert.Equal(t, []string{first.String(), second.String()}, order)
}

func Test_Session_handleUserMessage_dropsEmptyTextBlocks(t *testing.T) {
	t.Parallel()

	// a text block that never receives a delta, which is what the API emits
	// ahead of a tool_use and then rejects on the way back in.
	body := sseEvent("message_start",
		`{"type":"message_start","message":{"id":"m1","type":"message","role":"assistant","content":[],"model":"claude","usage":{"input_tokens":1,"output_tokens":1}}}`) +
		sseEvent("content_block_start",
			`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`) +
		sseEvent("content_block_stop", `{"type":"content_block_stop","index":0}`) +
		sseEvent("message_delta",
			`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":1,"output_tokens":1}}`) +
		sseEvent("message_stop", `{"type":"message_stop"}`)

	client := anthClient(t, func(int) (int, string) {
		return http.StatusOK, body
	})

	s := newTestSession(client, &protocolMock.SessionWriter{}, nil, nil, nil)

	s.Process(context.Background(), []byte(`{"type":"message","content":"hi"}`))
	s.supv.Wait()

	for _, m := range s.messages {
		for _, b := range m.Content {
			if b.OfText != nil {
				assert.NotEmpty(t, b.OfText.Text, "an empty text block would be rejected on the next turn")
			}
		}
	}
}

func Test_Session_dispatchTools_containsToolPanic(t *testing.T) {
	t.Parallel()

	writer := &protocolMock.SessionWriter{}

	db := &toolsMock.DB{
		FetchDocumentTreeFunc: func(context.Context, string) (document.Summaries, error) {
			panic("tool exploded")
		},
	}

	s := newTestSession(nil, writer, db, nil, nil)

	// a panicking tool degrades to an error result instead of taking the
	// process down or abandoning the rest of the batch.
	var results []anthropic.ContentBlockParamUnion

	require.NotPanics(t, func() {
		results = s.dispatchTools(context.Background(), []anthropic.ContentBlockParamUnion{
			toolUseBlock("t1", "list_documents", `{}`),
		})
	})

	require.Len(t, results, 1)
	require.NotNil(t, results[0].OfToolResult)
}

func Test_Session_Process_releasesTurnOnPanic(t *testing.T) {
	t.Parallel()

	client := anthClient(t, func(int) (int, string) {
		return http.StatusOK, sseTextTurn("hello")
	})

	// panic partway through the turn, while still letting the recovery path
	// deliver its error and done messages.
	writer := &protocolMock.SessionWriter{}
	writer.WriteJSONFunc = func(_ context.Context, msg any) {
		if _, ok := msg.(protocol.TextDeltaMessage); ok {
			panic("writer exploded")
		}
	}

	s := newTestSession(client, writer, nil, nil, nil)

	s.Process(context.Background(), []byte(`{"type":"message","content":"hi"}`))
	s.supv.Wait()

	// the turn is released and the client is told, rather than the session
	// rejecting every later message with "already processing".
	assert.False(t, s.processing)
	assert.Contains(t, writtenTypes(writer), "done")
}

func Test_Session_dispatchTools(t *testing.T) {
	t.Run("Reads execute without confirmation", func(t *testing.T) {
		t.Parallel()

		writer := &protocolMock.SessionWriter{}
		db := &toolsMock.DB{}
		s := newTestSession(nil, writer, db, nil, nil)

		results := s.dispatchTools(context.Background(), []anthropic.ContentBlockParamUnion{
			toolUseBlock("t1", "list_documents", `{}`),
			toolUseBlock("t2", "list_documents", `{}`),
		})

		require.Len(t, results, 2)
		assert.Len(t, db.FetchDocumentTreeCalls(), 2)
		assert.NotContains(t, writtenTypes(writer), "confirm_request")
	})

	t.Run("Declined writes are skipped", func(t *testing.T) {
		t.Parallel()

		var s *session

		writer := &protocolMock.SessionWriter{
			WriteJSONFunc: func(_ context.Context, msg any) {
				if req, ok := msg.(protocol.ConfirmRequest); ok {
					s.deliverConfirmResponse(req.TurnID, false, false)
				}
			},
		}

		db := &toolsMock.DB{}
		s = newTestSession(nil, writer, db, nil, nil)

		results := s.dispatchTools(context.Background(), []anthropic.ContentBlockParamUnion{
			toolUseBlock("t1", "delete_block", `{"document_id":"d","block_uid":"b"}`),
		})

		require.Len(t, results, 1)
		assert.Contains(t, results[0].OfToolResult.Content[0].OfText.Text, "user declined")
		assert.Empty(t, db.FetchDocumentCalls())
	})

	t.Run("Approved writes execute", func(t *testing.T) {
		t.Parallel()

		var s *session

		writer := &protocolMock.SessionWriter{
			WriteJSONFunc: func(_ context.Context, msg any) {
				if req, ok := msg.(protocol.ConfirmRequest); ok {
					s.deliverConfirmResponse(req.TurnID, true, false)
				}
			},
		}

		db := &toolsMock.DB{}
		s = newTestSession(nil, writer, db, nil, nil)

		results := s.dispatchTools(context.Background(), []anthropic.ContentBlockParamUnion{
			toolUseBlock("t1", "delete_block", `{"document_id":"not-an-xid","block_uid":"b"}`),
		})

		// the write ran (and failed inside the tool on the bad id),
		// proving approval unblocked execution
		require.Len(t, results, 1)
		assert.Contains(t, results[0].OfToolResult.Content[0].OfText.Text, "error")
	})

	t.Run("Auto-approve skips the prompt", func(t *testing.T) {
		t.Parallel()

		writer := &protocolMock.SessionWriter{}
		s := newTestSession(nil, writer, &toolsMock.DB{}, nil, nil)
		s.autoApproveTurn = true

		results := s.dispatchTools(context.Background(), []anthropic.ContentBlockParamUnion{
			toolUseBlock("t1", "delete_block", `{"document_id":"not-an-xid","block_uid":"b"}`),
		})

		require.Len(t, results, 1)
		assert.NotContains(t, writtenTypes(writer), "confirm_request")
	})

	t.Run("Non-tool blocks are ignored", func(t *testing.T) {
		t.Parallel()

		s := newTestSession(nil, &protocolMock.SessionWriter{}, nil, nil, nil)

		results := s.dispatchTools(context.Background(), []anthropic.ContentBlockParamUnion{
			anthropic.NewTextBlock("just narration"),
		})

		assert.Empty(t, results)
	})

	t.Run("Nil input defaults to empty args", func(t *testing.T) {
		t.Parallel()

		db := &toolsMock.DB{}
		s := newTestSession(nil, &protocolMock.SessionWriter{}, db, nil, nil)

		results := s.dispatchTools(context.Background(), []anthropic.ContentBlockParamUnion{
			{OfToolUse: &anthropic.ToolUseBlockParam{ID: "t1", Name: "list_documents"}},
		})

		require.Len(t, results, 1)
		assert.Len(t, db.FetchDocumentTreeCalls(), 1)
	})
}

func Test_Session_runTool(t *testing.T) {
	t.Run("Unknown tool", func(t *testing.T) {
		t.Parallel()

		s := newTestSession(nil, &protocolMock.SessionWriter{}, nil, nil, nil)

		result := s.runTool(context.Background(), "t1", "wibble", json.RawMessage(`{}`))
		assert.Contains(t, result.OfToolResult.Content[0].OfText.Text, "unknown tool")
	})

	t.Run("Tool failure becomes an error envelope", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer

		db := &toolsMock.DB{
			FetchDocumentTreeFunc: func(_ context.Context, _ string) (document.Summaries, error) {
				return nil, assert.AnError
			},
		}
		s := newTestSession(nil, &protocolMock.SessionWriter{}, db, nil, slog.New(slog.NewTextHandler(&buf, nil)))

		result := s.runTool(context.Background(), "t1", "list_documents", json.RawMessage(`{}`))
		assert.Contains(t, result.OfToolResult.Content[0].OfText.Text, "error")
		assert.Contains(t, buf.String(), "assistant tool error")
	})

	t.Run("Oversized result is truncated", func(t *testing.T) {
		t.Parallel()

		huge := strings.Repeat("x", _maxToolResultBytes)

		db := &toolsMock.DB{
			FetchDocumentTreeFunc: func(_ context.Context, _ string) (document.Summaries, error) {
				return document.Summaries{{DocumentName: huge}}, nil
			},
		}
		s := newTestSession(nil, &protocolMock.SessionWriter{}, db, nil, nil)

		result := s.runTool(context.Background(), "t1", "list_documents", json.RawMessage(`{}`))
		assert.Contains(t, result.OfToolResult.Content[0].OfText.Text, "result too large")
	})

	t.Run("Successful run emits a status pill", func(t *testing.T) {
		t.Parallel()

		writer := &protocolMock.SessionWriter{}
		s := newTestSession(nil, writer, nil, nil, nil)

		result := s.runTool(context.Background(), "t1", "search_documents", json.RawMessage(`{"query":"cats"}`))
		assert.JSONEq(t, `{"hits":[]}`, result.OfToolResult.Content[0].OfText.Text)
		assert.Equal(t, []string{"tool_status"}, writtenTypes(writer))
	})
}

func Test_Session_requestConfirmation(t *testing.T) {
	t.Run("Approved", func(t *testing.T) {
		t.Parallel()

		var s *session

		writer := &protocolMock.SessionWriter{
			WriteJSONFunc: func(_ context.Context, msg any) {
				if req, ok := msg.(protocol.ConfirmRequest); ok {
					s.deliverConfirmResponse(req.TurnID, true, false)
				}
			},
		}
		s = newTestSession(nil, writer, nil, nil, nil)

		assert.True(t, s.requestConfirmation(context.Background(), []protocol.ConfirmAction{{Tool: "delete_block"}}))
		assert.Nil(t, s.confirmCh)
		assert.Empty(t, s.confirmID)
	})

	t.Run("Timeout declines", func(t *testing.T) {
		// mutates the package-level timeout; must not run parallel
		orig := _confirmTimeout
		_confirmTimeout = 25 * time.Millisecond

		t.Cleanup(func() { _confirmTimeout = orig })

		s := newTestSession(nil, &protocolMock.SessionWriter{}, nil, nil, nil)

		assert.False(t, s.requestConfirmation(context.Background(), []protocol.ConfirmAction{{Tool: "delete_block"}}))
	})

	t.Run("Context cancellation declines", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())

		writer := &protocolMock.SessionWriter{
			WriteJSONFunc: func(_ context.Context, msg any) {
				if _, ok := msg.(protocol.ConfirmRequest); ok {
					cancel()
				}
			},
		}
		s := newTestSession(nil, writer, nil, nil, nil)

		assert.False(t, s.requestConfirmation(ctx, []protocol.ConfirmAction{{Tool: "delete_block"}}))
	})
}

func Test_Session_deliverConfirmResponse(t *testing.T) {
	t.Parallel()

	s := newTestSession(nil, &protocolMock.SessionWriter{}, nil, nil, nil)

	// no pending confirm is a no-op
	s.deliverConfirmResponse("turn-1", true, false)

	// mismatched turn id is dropped
	ch := make(chan bool, 1)
	s.confirmCh = ch
	s.confirmID = "turn-1"

	s.deliverConfirmResponse("turn-2", true, false)
	assert.Empty(t, ch)

	// matching id delivers; all=true arms auto-approve
	s.deliverConfirmResponse("turn-1", true, true)
	assert.True(t, <-ch)
	assert.True(t, s.autoApproveTurn)

	// a duplicate answer for the same turn is dropped without blocking
	s.deliverConfirmResponse("turn-1", false, false)
	s.deliverConfirmResponse("turn-1", false, false)
}

func Test_Session_pruneStaleReads(t *testing.T) {
	readTurn := func(id, args string) []anthropic.MessageParam {
		return []anthropic.MessageParam{
			assistantMsg(toolUseBlock(id, "read_block", args)),
			{
				Role:    anthropic.MessageParamRoleUser,
				Content: []anthropic.ContentBlockParamUnion{makeToolResult(id, json.RawMessage(`{"full":"content"}`))},
			},
		}
	}

	t.Run("Older duplicate read is pruned", func(t *testing.T) {
		t.Parallel()

		s := newTestSession(nil, &protocolMock.SessionWriter{}, nil, nil, nil)

		s.messages = append(readTurn("t1", `{"document_id":"d","block_uid":"b"}`),
			readTurn("t2", `{"document_id":"d","block_uid":"b"}`)...)

		s.pruneStaleReads()

		pruned := s.messages[1].Content[0].OfToolResult.Content[0].OfText.Text
		assert.Contains(t, pruned, "pruned")

		kept := s.messages[3].Content[0].OfToolResult.Content[0].OfText.Text
		assert.Contains(t, kept, "full")
	})

	t.Run("Different targets are kept", func(t *testing.T) {
		t.Parallel()

		s := newTestSession(nil, &protocolMock.SessionWriter{}, nil, nil, nil)

		s.messages = append(readTurn("t1", `{"document_id":"d","block_uid":"b1"}`),
			readTurn("t2", `{"document_id":"d","block_uid":"b2"}`)...)

		s.pruneStaleReads()

		assert.Contains(t, s.messages[1].Content[0].OfToolResult.Content[0].OfText.Text, "full")
		assert.Contains(t, s.messages[3].Content[0].OfToolResult.Content[0].OfText.Text, "full")
	})

	t.Run("Malformed args are logged and skipped", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer

		s := newTestSession(nil, &protocolMock.SessionWriter{}, nil, nil, slog.New(slog.NewTextHandler(&buf, nil)))
		s.messages = readTurn("t1", `{broken`)

		s.pruneStaleReads()
		assert.Contains(t, buf.String(), "probe unmarshal failed")
	})

	t.Run("Non-read tools are ignored", func(t *testing.T) {
		t.Parallel()

		s := newTestSession(nil, &protocolMock.SessionWriter{}, nil, nil, nil)
		s.messages = []anthropic.MessageParam{
			assistantMsg(anthropic.NewTextBlock("narration"), toolUseBlock("t1", "delete_block", `{"document_id":"d"}`)),
			assistantMsg(toolUseBlock("t2", "delete_block", `{"document_id":"d"}`)),
		}

		s.pruneStaleReads()
	})
}

func Test_Session_prunePriorReadResult(t *testing.T) {
	t.Parallel()

	s := newTestSession(nil, &protocolMock.SessionWriter{}, nil, nil, nil)
	s.messages = []anthropic.MessageParam{
		assistantMsg(toolUseBlock("t1", "read_block", `{}`)),
		{
			Role: anthropic.MessageParamRoleUser,
			Content: []anthropic.ContentBlockParamUnion{
				makeToolResult("other", json.RawMessage(`{"full":"other"}`)),
				makeToolResult("t1", json.RawMessage(`{"full":"content"}`)),
			},
		},
		assistantMsg(anthropic.NewTextBlock("later")),
	}

	// an index at the tail is guarded against
	s.prunePriorReadResult(len(s.messages)-1, "t1")
	assert.Contains(t, s.messages[1].Content[1].OfToolResult.Content[0].OfText.Text, "full")

	// only the matching tool result is replaced
	s.prunePriorReadResult(0, "t1")
	assert.Contains(t, s.messages[1].Content[0].OfToolResult.Content[0].OfText.Text, "other")
	assert.Contains(t, s.messages[1].Content[1].OfToolResult.Content[0].OfText.Text, "pruned")
}

func Test_Session_trimMessages(t *testing.T) {
	buildMessages := func(n int) []anthropic.MessageParam {
		out := make([]anthropic.MessageParam, 0, n)
		for range n {
			out = append(out, userMsg("x"))
		}

		return out
	}

	cc := map[string]struct {
		Count  int
		Result int
	}{
		"Under the limit":            {Count: 10, Result: 10},
		"At the limit":               {Count: _maxMessages, Result: _maxMessages},
		"Even excess trims in pairs": {Count: _maxMessages + 2, Result: _maxMessages},
		"Odd excess rounds up":       {Count: _maxMessages + 3, Result: _maxMessages - 1},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			s := newTestSession(nil, &protocolMock.SessionWriter{}, nil, nil, nil)
			s.messages = buildMessages(c.Count)

			s.trimMessages()
			assert.Len(t, s.messages, c.Result)
		})
	}
}

func Test_makeToolResult(t *testing.T) {
	t.Parallel()

	result := makeToolResult("t1", json.RawMessage(`{"ok":true}`))

	require.NotNil(t, result.OfToolResult)
	assert.Equal(t, "t1", result.OfToolResult.ToolUseID)
	require.Len(t, result.OfToolResult.Content, 1)
	assert.JSONEq(t, `{"ok":true}`, result.OfToolResult.Content[0].OfText.Text)
}

func Test_mustJSON(t *testing.T) {
	t.Parallel()

	// success
	assert.JSONEq(t, `{"ok":true}`, string(mustJSON(map[string]any{"ok": true})))

	// marshal failure degrades to an error envelope
	out := mustJSON(make(chan int))
	assert.Contains(t, string(out), "error")
}
