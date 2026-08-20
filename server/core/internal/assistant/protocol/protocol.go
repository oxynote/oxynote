// Package protocol describes the WebSocket message envelope used
// between the AI chat client and the assistant session running on
// the server.
//
// Tools execute server-side: the protocol carries text deltas,
// completion signals, conversation history, errors, and a
// confirmation handshake the client uses to approve or decline
// batched write operations. The full set of tools and their inputs
// lives in package tools — this file only describes the wire
// envelope.
package protocol

import (
	"context"
)

const (
	// ServerTypeTextDelta carries a fragment of the streaming
	// assistant response.
	ServerTypeTextDelta ServerMessageType = "text_delta"

	// ServerTypeDone signals that the assistant finished its turn
	// (either reached a natural stop or hit the maximum agent
	// turn count).
	ServerTypeDone ServerMessageType = "done"

	// ServerTypeError carries a one-line error message intended
	// for surfacing in the chat UI.
	ServerTypeError ServerMessageType = "error"

	// ServerTypeHistory restores prior conversation entries when
	// the client first connects.
	ServerTypeHistory ServerMessageType = "history"

	// ServerTypeConfirmRequest asks the user to approve or decline
	// a batch of pending write operations before the assistant
	// applies them.
	ServerTypeConfirmRequest ServerMessageType = "confirm_request"

	// ServerTypeTextEnd signals that the assistant has finished
	// streaming one run of text. Kind tells the client whether the
	// text was intermediate narration (status — render as a
	// transient pill, don't persist in chat history) or the final
	// response (message — push into the chat as a real bubble).
	ServerTypeTextEnd ServerMessageType = "text_end"

	// ServerTypeToolStatus carries a short, user-facing label
	// describing a tool the assistant is about to run (e.g.
	// "Reading 'Cat Facts'"). Surfaced as a transient pill so the
	// user can see what the assistant is doing while a turn
	// resolves through multiple tool calls.
	ServerTypeToolStatus ServerMessageType = "tool_status"
)

// ServerMessageType identifies a message sent from the server to
// the client.
type ServerMessageType string

const (
	// TextEndKindStatus means the just-streamed text was the model
	// thinking out loud before invoking tools — show as a transient
	// status indicator and discard once superseded.
	TextEndKindStatus TextEndKind = "status"

	// TextEndKindMessage means the just-streamed text is the
	// assistant's final answer for this turn — keep it in the chat.
	TextEndKindMessage TextEndKind = "message"
)

// TextEndKind discriminates between intermediate narration and a
// final assistant response on a text_end message.
type TextEndKind string

const (
	// ClientTypeMessage is a new user prompt for the assistant.
	ClientTypeMessage ClientMessageType = "message"

	// ClientTypeReset asks the server to drop the conversation
	// history for the active session.
	ClientTypeReset ClientMessageType = "reset"

	// ClientTypeConfirmResponse carries the user's approval (or
	// decline) of a previously-sent ConfirmRequest.
	ClientTypeConfirmResponse ClientMessageType = "confirm_response"

	// ClientTypeSetActiveDocument tells the session which document
	// the user is currently viewing so the system prompt can resolve
	// "this document" without an explicit id. Sent on connect and
	// again whenever the user navigates between documents — the
	// connection itself persists across doc switches so an in-flight
	// turn isn't torn down.
	ClientTypeSetActiveDocument ClientMessageType = "set_active_document"
)

// ClientMessageType identifies a message sent from the client to
// the server.
type ClientMessageType string

// TextDeltaMessage carries one fragment of the streaming assistant
// response.
type TextDeltaMessage struct {
	// Type is always ServerTypeTextDelta.
	Type ServerMessageType `json:"type"`

	// Content is the text fragment.
	Content string `json:"content"`
}

// NewTextDeltaMessage builds a TextDeltaMessage.
func NewTextDeltaMessage(content string) TextDeltaMessage {
	return TextDeltaMessage{Type: ServerTypeTextDelta, Content: content}
}

// TextEndMessage marks the end of a text content block streamed
// via text_delta. Kind distinguishes intermediate narration from
// the assistant's final answer.
type TextEndMessage struct {
	// Type is always ServerTypeTextEnd.
	Type ServerMessageType `json:"type"`

	// Kind is one of TextEndKindStatus or TextEndKindMessage.
	Kind TextEndKind `json:"kind"`
}

// NewTextEndMessage builds a TextEndMessage.
func NewTextEndMessage(kind TextEndKind) TextEndMessage {
	return TextEndMessage{Type: ServerTypeTextEnd, Kind: kind}
}

// ToolStatusMessage carries a short label describing a tool the
// assistant is about to run. Tool is the raw tool name (same set
// as ConfirmAction.Tool) — the client maps it to a color category
// to keep the pill consistent with the confirm UI.
type ToolStatusMessage struct {
	// Type is always ServerTypeToolStatus.
	Type ServerMessageType `json:"type"`

	// Tool is the tool name (e.g. "create_document", "read_block").
	Tool string `json:"tool"`

	// Label is the user-facing description (e.g. "Reading 'Cat
	// Facts'", "Updating 'Auth spec'"). Starts with a verb the
	// client highlights in the pill.
	Label string `json:"label"`
}

// NewToolStatusMessage builds a ToolStatusMessage.
func NewToolStatusMessage(tool, label string) ToolStatusMessage {
	return ToolStatusMessage{Type: ServerTypeToolStatus, Tool: tool, Label: label}
}

// DoneMessage signals the end of the assistant's turn.
type DoneMessage struct {
	// Type is always ServerTypeDone.
	Type ServerMessageType `json:"type"`
}

// NewDoneMessage builds a DoneMessage.
func NewDoneMessage() DoneMessage {
	return DoneMessage{Type: ServerTypeDone}
}

// ErrorMessage surfaces a failure to the client UI.
type ErrorMessage struct {
	// Type is always ServerTypeError.
	Type ServerMessageType `json:"type"`

	// Message is the one-line error description.
	Message string `json:"message"`
}

// NewErrorMessage builds an ErrorMessage.
func NewErrorMessage(message string) ErrorMessage {
	return ErrorMessage{Type: ServerTypeError, Message: message}
}

// HistoryEntry is one prior assistant or user message recovered
// when the client reconnects.
type HistoryEntry struct {
	// Role is "user" or "assistant".
	Role string `json:"role"`

	// Content is the message text. Tool interactions are not part
	// of the restored history.
	Content string `json:"content"`
}

// HistoryMessage carries the conversation history sent to a freshly
// connected client.
type HistoryMessage struct {
	// Type is always ServerTypeHistory.
	Type ServerMessageType `json:"type"`

	// Messages is the ordered conversation history.
	Messages []HistoryEntry `json:"messages"`
}

// NewHistoryMessage builds a HistoryMessage from a list of entries.
func NewHistoryMessage(messages []HistoryEntry) HistoryMessage {
	return HistoryMessage{Type: ServerTypeHistory, Messages: messages}
}

// ConfirmRequest asks the user to approve or decline a batch of
// pending write actions the assistant has proposed in the current
// turn. The client must reply with a ConfirmResponse carrying the
// same TurnID before the assistant applies anything.
type ConfirmRequest struct {
	// Type is always ServerTypeConfirmRequest.
	Type ServerMessageType `json:"type"`

	// TurnID identifies the pending confirm. The client echoes it
	// back in ConfirmResponse so the session matches the answer
	// to the right batch.
	TurnID string `json:"turnId"`

	// Actions describes the proposed writes for display in the
	// confirm UI.
	Actions []ConfirmAction `json:"actions"`
}

// ConfirmAction summarises one write operation for the confirm UI.
type ConfirmAction struct {
	// Tool is the name of the tool the assistant intends to call
	// (e.g. "insert_block", "delete_document"). The UI uses this
	// to pick an affordance (e.g. emphasise deletes).
	Tool string `json:"tool"`

	// DocumentName is the human-friendly name of the document the
	// action targets, when known. Empty for ops that don't
	// reference an existing document (rare).
	DocumentName string `json:"documentName,omitempty"`

	// DocumentID is the target document's id, for any UI that
	// wants to render a deep link.
	DocumentID string `json:"documentId,omitempty"`

	// Summary is a one-sentence description of the proposed
	// change, e.g. "Insert callout after 'Rate limit' section".
	Summary string `json:"summary"`
}

// NewConfirmRequest builds a ConfirmRequest.
func NewConfirmRequest(turnID string, actions []ConfirmAction) ConfirmRequest {
	return ConfirmRequest{Type: ServerTypeConfirmRequest, TurnID: turnID, Actions: actions}
}

// ConfirmResponse carries the user's approval decision for a
// pending ConfirmRequest. The session unmarshals the client's
// confirm_response payload into it.
type ConfirmResponse struct {
	// TurnID echoes the ConfirmRequest's TurnID so the session
	// matches the answer to the right batch.
	TurnID string `json:"turnId"`

	// Approved is true when the user accepted the batch, false
	// when they declined.
	Approved bool `json:"approved"`

	// All, when set alongside Approved=true, also auto-approves
	// any further write batches in the current turn — the server
	// skips the prompt and proceeds directly. Resets at the next
	// user message.
	All bool `json:"all,omitempty"`
}

// SessionWriter is the interface the assistant session uses to
// stream messages back to the client. Implementations must be safe
// for concurrent use because text deltas and confirm requests can
// be produced from different goroutines.
//
//go:generate ../../../scripts/codegen/mock -t external SessionWriter session_writer
type SessionWriter interface {
	// WriteJSON should serialise msg as JSON and send it on the
	// underlying transport.
	WriteJSON(ctx context.Context, msg any)
}

// SessionConn is the bidirectional transport a chat session runs
// over. It stays transport-agnostic: implementations translate their
// clean-close signal into io.EOF.
//
//go:generate ../../../scripts/codegen/mock -t external SessionConn session_conn
type SessionConn interface {
	SessionWriter

	// Read should block until the next client message arrives,
	// returning io.EOF when the client ended the conversation
	// cleanly.
	Read(ctx context.Context) ([]byte, error)
}
