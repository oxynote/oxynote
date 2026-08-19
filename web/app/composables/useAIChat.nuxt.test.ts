import { mockNuxtImport } from "@nuxt/test-utils/runtime"
import { beforeEach, describe, it, vi } from "vitest"
import { ref } from "vue"
import type { Ref } from "vue"
import {
	AIToolName,
	ChatMessageRole,
	ClientMessageType,
	ServerMessageType,
	type ServerMessage,
} from "~/utils/api/ai"
import { useAIChat } from "./useAIChat"

interface CapturedChatWsOptions {
	onMessage?: (ws: unknown, event: { data: string }) => void
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

function arrange(opts?: {
	status?: "OPEN" | "CLOSED"
	toolExecutor?: (tool: AIToolName, args: unknown) => Promise<object>
}) {
	const status: Ref<string> = ref(opts?.status ?? "OPEN")
	const send = vi.fn()
	const open = vi.fn()
	const close = vi.fn()
	useWebSocketMock.mockReturnValue({ status, send, open, close })
	useI18nMock.mockReturnValue({ t: (key: string) => key })

	const toolExecutor = vi.fn(
		opts?.toolExecutor ?? (() => Promise.resolve({ ok: true })),
	)
	const chat = useAIChat({ toolExecutor: toolExecutor })

	const captured = useWebSocketMock.mock.calls[0]
	if (!captured) {
		throw new Error("the composable did not create a websocket")
	}

	function receive(msg: Partial<ServerMessage> & { type: ServerMessageType }) {
		captured?.[1].onMessage?.({}, { data: JSON.stringify(msg) })
	}

	return {
		chat,
		status,
		send,
		open,
		close,
		toolExecutor,
		receive,
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

	it("connects to the ai chat endpoint on demand", ({ expect }) => {
		const { chat, open, url } = arrange()

		chat.connect()

		// the core ws base is empty in the test runtime config; the url is
		// passed as a computed
		expect((url as { value: string }).value).toBe("/api/ai/chat")
		expect(open).toHaveBeenCalledTimes(1)
	})

	it("disconnects and clears the conversation", ({ expect }) => {
		const { chat, close } = arrange()
		chat.sendMessage("hello")

		chat.disconnect()

		expect(close).toHaveBeenCalledTimes(1)
		expect(chat.messages.value).toEqual([])
		expect(chat.isStreaming.value).toBe(false)
		expect(chat.toolStatus.value).toBeNull()
	})

	it("sends a message and starts streaming", ({ expect }) => {
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

	it("starts a new assistant message on the first text delta", ({ expect }) => {
		const { chat, receive } = arrange()

		receive({ type: ServerMessageType.TextDelta, content: "Hel" })

		expect(chat.messages.value).toEqual([
			{ role: ChatMessageRole.Assistant, text: "Hel" },
		])
	})

	it("extends the last assistant message on later text deltas", ({
		expect,
	}) => {
		const { chat, receive } = arrange()
		receive({ type: ServerMessageType.TextDelta, content: "Hel" })

		receive({ type: ServerMessageType.TextDelta, content: "lo" })

		expect(chat.messages.value).toEqual([
			{ role: ChatMessageRole.Assistant, text: "Hello" },
		])
	})

	it("stops streaming when the server is done", ({ expect }) => {
		const { chat, receive } = arrange()
		chat.sendMessage("hello")

		receive({ type: ServerMessageType.Done })

		expect(chat.isStreaming.value).toBe(false)
		expect(chat.toolStatus.value).toBeNull()
	})

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

	it("executes a tool call and reports the result", async ({ expect }) => {
		let resolveTool!: (result: object) => void
		const { chat, send, toolExecutor, receive } = arrange({
			toolExecutor: () =>
				new Promise((resolve) => {
					resolveTool = resolve
				}),
		})

		receive({
			type: ServerMessageType.ToolCall,
			id: "t1",
			tool: AIToolName.ReadDocument,
		})

		expect(chat.toolStatus.value).toBe(
			"editor.ai-chat.tool-status.read-document",
		)

		resolveTool({ blocks: [] })
		// the composable fires handleMessage as a floating promise, so there
		// is no concrete signal for its completion to await
		await vi.waitFor(() => {
			expect(chat.toolStatus.value).toBeNull()
		})

		expect(toolExecutor).toHaveBeenCalledExactlyOnceWith(
			AIToolName.ReadDocument,
			undefined,
		)
		expect(send).toHaveBeenCalledExactlyOnceWith(
			JSON.stringify({
				type: ClientMessageType.ToolResult,
				id: "t1",
				result: { blocks: [] },
			}),
		)
	})

	it("reports a tool execution failure to the server", async ({ expect }) => {
		const { send, chat, receive } = arrange({
			toolExecutor: () => Promise.reject(new Error("tool blew up")),
		})

		receive({
			type: ServerMessageType.ToolCall,
			id: "t1",
			tool: AIToolName.InsertBlocks,
		})
		// the composable fires handleMessage as a floating promise, so there
		// is no concrete signal for its completion to await
		await vi.waitFor(() => {
			expect(chat.toolStatus.value).toBeNull()
		})

		expect(send).toHaveBeenCalledExactlyOnceWith(
			JSON.stringify({
				type: ClientMessageType.ToolResult,
				id: "t1",
				result: { error: "tool blew up" },
			}),
		)
	})

	it("ignores a tool call without an id or tool name", ({ expect }) => {
		const { toolExecutor, receive } = arrange()

		receive({ type: ServerMessageType.ToolCall, id: "t1" })
		receive({ type: ServerMessageType.ToolCall, tool: AIToolName.ReadDocument })

		expect(toolExecutor).toHaveBeenCalledTimes(0)
	})

	it("resets the chat and notifies the server", ({ expect }) => {
		const { chat, send, receive } = arrange()
		chat.sendMessage("hello")
		receive({ type: ServerMessageType.TextDelta, content: "hi" })
		send.mockClear()

		chat.resetChat()

		expect(chat.messages.value).toEqual([])
		expect(chat.isStreaming.value).toBe(false)
		expect(send).toHaveBeenCalledExactlyOnceWith(
			JSON.stringify({ type: ClientMessageType.Reset }),
		)
	})

	it("stops streaming when the connection drops", ({ expect }) => {
		const { chat, ws } = arrange()
		chat.sendMessage("hello")

		ws.onDisconnected?.()

		expect(chat.isStreaming.value).toBe(false)
		expect(chat.toolStatus.value).toBeNull()
	})
})
