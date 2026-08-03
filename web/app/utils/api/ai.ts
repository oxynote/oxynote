export enum ChatMessageRole {
	User = "user",
	Assistant = "assistant",
	Error = "error",
}

export enum ServerMessageType {
	TextDelta = "text_delta",
	ToolCall = "tool_call",
	Done = "done",
	Error = "error",
	History = "history",
}

export interface HistoryEntry {
	role: ChatMessageRole
	content: string
}

export enum ClientMessageType {
	Message = "message",
	ToolResult = "tool_result",
	Reset = "reset",
}

export enum AIToolName {
	ReadDocument = "read_document",
	InsertBlocks = "insert_blocks",
	ReplaceBlockContent = "replace_block_content",
	DeleteBlocks = "delete_blocks",
	ReplaceBlockAttributes = "replace_block_attributes",
	ReadAvailableIcons = "read_available_icons",
	UpdateDocumentName = "update_document_name",
	UpdateDocumentIcon = "update_document_icon",
}

export enum InsertPosition {
	Before = "before",
	After = "after",
}

export interface InsertBlocksArgs {
	reference_uid: string
	position: InsertPosition
	blocks: object[]
}

export interface ReplaceBlockContentArgs {
	uid: string
	content: object[]
}

export interface ReplaceBlockAttributesArgs {
	uid: string
	attributes: Record<string, unknown>
}

export interface UpdateDocumentNameArgs {
	name: string
}

export interface UpdateDocumentIconArgs {
	icon: string
}

export interface DeleteBlocksArgs {
	uids: string[]
}

export type ExecuteToolArgs =
	| InsertBlocksArgs
	| ReplaceBlockContentArgs
	| ReplaceBlockAttributesArgs
	| UpdateDocumentNameArgs
	| UpdateDocumentIconArgs
	| DeleteBlocksArgs

export interface ChatMessage {
	role: ChatMessageRole
	text: string
}

export interface ServerMessage {
	type: ServerMessageType
	content?: string
	message?: string
	id?: string
	tool?: AIToolName
	args?: ExecuteToolArgs
	messages?: HistoryEntry[]
}
