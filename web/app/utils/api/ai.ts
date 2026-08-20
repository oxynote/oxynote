// wire contract for the assistant chat socket. It mirrors
// server/core/internal/assistant/protocol/protocol.go; the two must be
// changed together.

export enum ChatMessageRole {
	User = "user",
	Assistant = "assistant",
	Error = "error",
}

export enum ServerMessageType {
	TextDelta = "text_delta",
	TextEnd = "text_end",
	ToolStatus = "tool_status",
	ConfirmRequest = "confirm_request",
	Done = "done",
	Error = "error",
	History = "history",
}

// TextEndKind decides what happens to the text run that just ended.
// Narration the model produced on its way to a tool call is shown while
// it works and then discarded; a final reply is kept in the chat.
export enum TextEndKind {
	Status = "status",
	Message = "message",
}

export enum ClientMessageType {
	Message = "message",
	Reset = "reset",
	ConfirmResponse = "confirm_response",
	SetActiveDocument = "set_active_document",
}

export interface HistoryEntry {
	role: ChatMessageRole
	content: string
}

// ConfirmAction is one write the assistant is asking permission for.
export interface ConfirmAction {
	tool: string
	documentId?: string
	documentName?: string
	summary: string
}

// ConfirmRequest asks the user to approve every write the assistant
// proposed in one turn.
export interface ConfirmRequest {
	turnId: string
	actions: ConfirmAction[]
}

export interface ServerMessage {
	type: ServerMessageType
	content?: string
	message?: string
	kind?: TextEndKind
	tool?: string
	label?: string
	turnId?: string
	actions?: ConfirmAction[]
	messages?: HistoryEntry[]
}

export interface ChatMessage {
	role: ChatMessageRole
	text: string
}
