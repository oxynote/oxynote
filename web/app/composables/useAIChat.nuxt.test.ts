import { mockNuxtImport } from "@nuxt/test-utils/runtime"
import { beforeEach, describe, it, vi } from "vitest"
import { ref } from "vue"
import type { Ref } from "vue"
import {
	ChatMessageRole,
	ClientMessageType,
	ServerMessageType,
	TextEndKind,
	type ServerMessage,
} from "~/utils/api/ai"
import { useAIChat } from "./useAIChat"

interface CapturedChatWsOptions {
	onMessage?: (ws: unknown, event: { data: string }) => void
	onConnected?: () => void
	onDisconnected?: () => void
}

const { useWebSocketMock, useI18nMock } = vi.hoisted(() => {
	return {
		useWebSocketMock: vi.fn(
			(_url: unknown, _opts: CapturedChatWsOptions): any => ({
				status: { value: "CLOSED" },
				send: () => undefined,
				open: () => undefined,
				close: () => undefined,
			}),
		),
		// t returns the raw key so assertions stay locale-independent
		useI18nMock: vi.fn((): any => ({ t: (key: string) => key })),
	}
})

mockNuxtImport("useWebSocket", () => useWebSocketMock)
mockNuxtImport("useI18n", () => useI18nMock)

function arrange(opts?: { status?: "OPEN" | "CLOSED" }) {
	const status: Ref<string> = ref(opts?.status ?? "OPEN")
	const send = vi.fn()
	const open = vi.fn()
	const close = vi.fn()
	useWebSocketMock.mockReturnValue({ status, send, open, close })
	useI18nMock.mockReturnValue({ t: (key: string) => key })

	const chat = useAIChat()

	const captured = useWebSocketMock.mock.calls[0]
	if (!captured) {
		throw new Error("the composable did not create a websocket")
	}

	function receive(msg: Partial<ServerMessage> & { type: ServerMessageType }) {
		captured?.[1].onMessage?.({}, { data: JSON.stringify(msg) })
	}

	function reconnect() {
		captured?.[1].onConnected?.()
	}

	function sentPayloads(): Record<string, unknown>[] {
		return send.mock.calls.map(
			(c) => JSON.parse(c[0] as string) as Record<string, unknown>,
		)
	}

	return {
		chat,
		status,
		send,
		open,
		close,
		receive,
		reconnect,
		sentPayloads,
		url: captured[0],
		ws: captured[1],
	}
}

// the tests arrange shared module-level mocks (mockNuxtImport singletons),
// so they cannot interleave
describe("useAIChat", { concurrent: false }, () => {
	// restoreMocks does not touch hand-made vi.fn() singletons in vitest 4
	// — reset the module-level mocks explicitly
	beforeEach(() => {
		useWebSocketMock.mockReset()
		useI18nMock.mockReset()
	})

	describe("connect", () => {
		it("connects to the ai chat endpoint on demand", ({ expect }) => {
			const { chat, open, url } = arrange()

			chat.connect()

			// the core ws base is empty in the test runtime config; the url is
			// passed as a computed
			expect((url as { value: string }).value).toBe("/api/ai/chat")
			expect(open).toHaveBeenCalledTimes(1)
		})
	})

	describe("disconnect", () => {
		it("closes the socket and clears the conversation", ({ expect }) => {
			const { chat, close } = arrange()
			chat.sendMessage("hello")

			chat.disconnect()

			expect(close).toHaveBeenCalledTimes(1)
			expect(chat.messages.value).toEqual([])
			expect(chat.isStreaming.value).toBe(false)
			expect(chat.toolStatus.value).toBeNull()
			expect(chat.pendingConfirm.value).toBeNull()
		})
	})

	describe("sendMessage", () => {
		it("shows the message and starts the turn", ({ expect }) => {
			const { chat, send } = arrange()

			chat.sendMessage("hello")

			expect(chat.messages.value).toEqual([
				{ role: ChatMessageRole.User, text: "hello" },
			])
			expect(chat.isStreaming.value).toBe(true)
			expect(send).toHaveBeenCalledExactlyOnceWith(
				JSON.stringify({ type: ClientMessageType.Message, content: "hello" }),
			)
		})

		it("drops messages while disconnected", ({ expect }) => {
			const { chat, send } = arrange({ status: "CLOSED" })

			chat.sendMessage("hello")

			expect(chat.messages.value).toEqual([])
			expect(chat.isStreaming.value).toBe(false)
			expect(send).toHaveBeenCalledTimes(0)
		})
	})

	describe("setActiveDocument", () => {
		it("tells the server which document and branch the user has open", ({
			expect,
		}) => {
			const { chat, sentPayloads } = arrange()

			chat.setActiveDocument("doc-1", "branch-1")

			expect(sentPayloads()).toEqual([
				{
					type: ClientMessageType.SetActiveDocument,
					documentId: "doc-1",
					branchId: "branch-1",
				},
			])
		})

		it("sends empty ids when no document is open", ({ expect }) => {
			const { chat, sentPayloads } = arrange()

			chat.setActiveDocument(null)

			expect(sentPayloads()).toEqual([
				{
					type: ClientMessageType.SetActiveDocument,
					documentId: "",
					branchId: "",
				},
			])
		})

		it("replays the active document when the socket reconnects", ({
			expect,
		}) => {
			const { chat, reconnect, sentPayloads } = arrange()
			chat.setActiveDocument("doc-2")

			reconnect()

			// the server forgets the active document with the connection, so
			// a reconnect that did not replay it would leave the model unable
			// to resolve "this document"
			expect(sentPayloads()).toHaveLength(2)
			expect(sentPayloads().at(-1)).toEqual({
				type: ClientMessageType.SetActiveDocument,
				documentId: "doc-2",
				branchId: "",
			})
		})

		it("drops the update while disconnected", ({ expect }) => {
			const { chat, send } = arrange({ status: "CLOSED" })

			chat.setActiveDocument("doc-1")

			expect(send).toHaveBeenCalledTimes(0)
		})
	})

	describe("history", () => {
		it("replaces the conversation with the server history", ({ expect }) => {
			const { chat, receive } = arrange()
			chat.sendMessage("stale")

			receive({
				type: ServerMessageType.History,
				messages: [
					{ role: ChatMessageRole.User, content: "hi" },
					{ role: ChatMessageRole.Assistant, content: "hello" },
				],
			})

			expect(chat.messages.value).toEqual([
				{ role: ChatMessageRole.User, text: "hi" },
				{ role: ChatMessageRole.Assistant, text: "hello" },
			])
		})
	})

	describe("text streaming", () => {
		it("accumulates deltas without committing them to the chat", ({
			expect,
		}) => {
			const { chat, receive } = arrange()

			receive({ type: ServerMessageType.TextDelta, content: "Hel" })
			receive({ type: ServerMessageType.TextDelta, content: "lo" })

			expect(chat.streamingText.value).toBe("Hello")
			expect(chat.messages.value).toEqual([])
		})

		it("commits a finished reply into the chat", ({ expect }) => {
			const { chat, receive } = arrange()
			receive({ type: ServerMessageType.TextDelta, content: "the answer" })

			receive({ type: ServerMessageType.TextEnd, kind: TextEndKind.Message })

			expect(chat.messages.value).toEqual([
				{ role: ChatMessageRole.Assistant, text: "the answer" },
			])
			expect(chat.streamingText.value).toBe("")
		})

		it("discards narration that only preceded a tool call", ({ expect }) => {
			const { chat, receive } = arrange()
			receive({ type: ServerMessageType.TextDelta, content: "let me look" })

			receive({ type: ServerMessageType.TextEnd, kind: TextEndKind.Status })

			// narration is shown while the assistant works and then dropped,
			// so the chat keeps only the answers
			expect(chat.messages.value).toEqual([])
			expect(chat.streamingText.value).toBe("")
		})

		it("commits an empty run as nothing", ({ expect }) => {
			const { chat, receive } = arrange()

			receive({ type: ServerMessageType.TextEnd, kind: TextEndKind.Message })

			expect(chat.messages.value).toEqual([])
		})
	})

	describe("tool status", () => {
		it("shows the label the server sends", ({ expect }) => {
			const { chat, receive } = arrange()

			receive({
				type: ServerMessageType.ToolStatus,
				tool: "search_documents",
				label: "Searching documents...",
			})

			expect(chat.toolStatus.value).toBe("Searching documents...")
		})

		it("falls back to a generic label when the server sends none", ({
			expect,
		}) => {
			const { chat, receive } = arrange()

			receive({ type: ServerMessageType.ToolStatus, tool: "x" })

			expect(chat.toolStatus.value).toBe("editor.ai-chat.tool-status.working")
		})

		it("clears the status once the reply starts arriving", ({ expect }) => {
			const { chat, receive } = arrange()
			receive({
				type: ServerMessageType.ToolStatus,
				tool: "x",
				label: "working",
			})

			receive({ type: ServerMessageType.TextDelta, content: "found it" })

			expect(chat.toolStatus.value).toBeNull()
		})
	})

	describe("confirmations", () => {
		it("surfaces the batch of writes awaiting approval", ({ expect }) => {
			const { chat, receive } = arrange()

			receive({
				type: ServerMessageType.ConfirmRequest,
				turnId: "t1",
				actions: [
					{ tool: "insert_block", summary: "Insert a callout" },
					{ tool: "update_block_text", summary: "Reword the intro" },
				],
			})

			expect(chat.pendingConfirm.value?.turnId).toBe("t1")
			expect(chat.pendingConfirm.value?.actions).toHaveLength(2)
		})

		it("stops showing the turn as working while it waits", ({ expect }) => {
			const { chat, receive } = arrange()
			chat.sendMessage("do it")

			receive({
				type: ServerMessageType.ConfirmRequest,
				turnId: "t1",
				actions: [],
			})

			expect(chat.isStreaming.value).toBe(false)
			expect(chat.toolStatus.value).toBeNull()
		})

		it("ignores a confirmation with no turn id", ({ expect }) => {
			const { chat, receive } = arrange()

			receive({ type: ServerMessageType.ConfirmRequest, actions: [] })

			expect(chat.pendingConfirm.value).toBeNull()
		})

		it.for([
			{ name: "approving", approved: true, all: false },
			{ name: "declining", approved: false, all: false },
			{ name: "approving all", approved: true, all: true },
		])(
			"answers the confirmation when $name",
			({ approved, all }, { expect }) => {
				const { chat, receive, sentPayloads } = arrange()
				receive({
					type: ServerMessageType.ConfirmRequest,
					turnId: "t1",
					actions: [],
				})

				chat.answerConfirm(approved, all)

				expect(sentPayloads().at(-1)).toEqual({
					type: ClientMessageType.ConfirmResponse,
					turnId: "t1",
					approved,
					all,
				})
				expect(chat.pendingConfirm.value).toBeNull()
				expect(chat.isStreaming.value).toBe(true)
			},
		)

		it("ignores an answer when nothing is pending", ({ expect }) => {
			const { chat, send } = arrange()

			chat.answerConfirm(true)

			expect(send).toHaveBeenCalledTimes(0)
		})

		it("keeps the confirmation pending while disconnected", ({ expect }) => {
			const { chat, receive, send } = arrange({ status: "CLOSED" })
			receive({
				type: ServerMessageType.ConfirmRequest,
				turnId: "t1",
				actions: [],
			})

			chat.answerConfirm(true)

			expect(send).toHaveBeenCalledTimes(0)
			expect(chat.pendingConfirm.value?.turnId).toBe("t1")
		})
	})

	describe("done", () => {
		it("ends the turn and clears transient state", ({ expect }) => {
			const { chat, receive } = arrange()
			chat.sendMessage("hello")
			receive({
				type: ServerMessageType.ToolStatus,
				tool: "x",
				label: "working",
			})

			receive({ type: ServerMessageType.Done })

			expect(chat.isStreaming.value).toBe(false)
			expect(chat.toolStatus.value).toBeNull()
			expect(chat.streamingText.value).toBe("")
		})
	})

	describe("errors", () => {
		it("records a server error and stops streaming", ({ expect }) => {
			const { chat, receive } = arrange()
			chat.sendMessage("hello")

			receive({ type: ServerMessageType.Error, message: "model unavailable" })

			expect(chat.messages.value.at(-1)).toEqual({
				role: ChatMessageRole.Error,
				text: "model unavailable",
			})
			expect(chat.isStreaming.value).toBe(false)
		})

		it("falls back to the generic error text", ({ expect }) => {
			const { chat, receive } = arrange()

			receive({ type: ServerMessageType.Error })

			expect(chat.messages.value.at(-1)).toEqual({
				role: ChatMessageRole.Error,
				text: "editor.ai-chat.generic-error",
			})
		})

		it("ignores malformed server payloads", ({ expect }) => {
			const { chat, ws } = arrange()

			ws.onMessage?.({}, { data: "not json" })

			expect(chat.messages.value).toEqual([])
		})
	})

	describe("resetChat", () => {
		it("clears the conversation and tells the server", ({ expect }) => {
			const { chat, receive, sentPayloads } = arrange()
			chat.sendMessage("hello")
			receive({
				type: ServerMessageType.ConfirmRequest,
				turnId: "t1",
				actions: [],
			})

			chat.resetChat()

			expect(chat.messages.value).toEqual([])
			expect(chat.pendingConfirm.value).toBeNull()
			expect(chat.streamingText.value).toBe("")
			expect(sentPayloads().at(-1)).toEqual({ type: ClientMessageType.Reset })
		})

		it("clears locally even while disconnected", ({ expect }) => {
			const { chat, send } = arrange({ status: "CLOSED" })
			chat.messages.value.push({ role: ChatMessageRole.User, text: "hi" })

			chat.resetChat()

			expect(chat.messages.value).toEqual([])
			expect(send).toHaveBeenCalledTimes(0)
		})
	})

	describe("disconnection", () => {
		it("stops showing the turn as running when the socket drops", ({
			expect,
		}) => {
			const { chat, ws } = arrange()
			chat.sendMessage("hello")

			ws.onDisconnected?.()

			expect(chat.isStreaming.value).toBe(false)
			expect(chat.toolStatus.value).toBeNull()
			expect(chat.streamingText.value).toBe("")
		})
	})
})
