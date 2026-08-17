import { describe, it, vi, type Mock } from "vitest"
import WsState, { extractFromTopic } from "./websocket"

describe("extractFromTopic", () => {
	it.for([
		{
			name: "splits a topic into descriptor, operation, args and clean topic",
			input: "pub~change@documents.1",
			expected: ["pub", "change", ["documents.1"], "change@documents.1"],
		},
		{
			name: "returns empty parts without a descriptor separator",
			input: "change@documents.1",
			expected: ["", "", [], ""],
		},
		{
			name: "returns empty parts for multiple descriptor separators",
			input: "a~b~change@documents.1",
			expected: ["", "", [], ""],
		},
		{
			name: "returns empty parts without an operation separator",
			input: "pub~documents.1",
			expected: ["", "", [], ""],
		},
		{
			name: "returns empty parts for multiple operation separators",
			input: "pub~change@documents@extra",
			expected: ["", "", [], ""],
		},
		{
			name: "returns empty parts for an empty topic",
			input: "",
			expected: ["", "", [], ""],
		},
	])("$name", ({ input, expected }, { expect }) => {
		expect(extractFromTopic(input)).toEqual(expected)
	})
})

const TOPIC = "change@documents.1"

function fakeWs() {
	return { send: vi.fn() } as unknown as WebSocket & { send: Mock }
}

// decodes every message sent over the fake socket
function sentMessages(ws: { send: Mock }): { topic: string; id: number }[] {
	return ws.send.mock.calls.map(
		([raw]) => JSON.parse(raw as string) as { topic: string; id: number },
	)
}

function messageEvent(event: {
	topic?: string
	payload?: object
	success?: boolean
	id?: number
}) {
	return {
		data: JSON.stringify({
			topic: "",
			payload: {},
			success: false,
			id: 0,
			...event,
		}),
	} as MessageEvent<string>
}

// confirms the pending subscription of the given sent message id
function confirm(state: WsState, id: number) {
	state.processMessage(messageEvent({ success: true, id }))
}

// the constructor's auto-resubscribe interval keeps ticking on the
// process-global clock, so these tests use a far-away interval and the
// fake-timer test installs a global fake clock — both are shared state
// that cannot interleave with other tests
describe("WsState", { concurrent: false }, () => {
	function makeState() {
		const ws = fakeWs()
		const state = new WsState(1_000_000)
		state.ws = ws

		return { ws, state }
	}

	describe("subscribe", () => {
		it("sends a subscription message for a new topic", ({ expect }) => {
			const { ws, state } = makeState()

			state.subscribe(TOPIC, vi.fn())

			expect(sentMessages(ws)).toEqual([{ topic: `sub~${TOPIC}`, id: 1 }])
		})

		it("does not send anything without a socket", ({ expect }) => {
			const ws = fakeWs()
			const state = new WsState(1_000_000)

			state.subscribe(TOPIC, vi.fn())

			expect(ws.send).toHaveBeenCalledTimes(0)
		})

		it("delivers publish payloads to every subscriber once confirmed", ({
			expect,
		}) => {
			const { ws, state } = makeState()
			const first = vi.fn()
			const second = vi.fn()

			state.subscribe(TOPIC, first)
			state.subscribe(TOPIC, second)
			confirm(state, 1)
			state.processMessage(
				messageEvent({ topic: `pub~${TOPIC}`, payload: { x: 1 } }),
			)

			expect(first).toHaveBeenCalledTimes(1)
			expect(first).toHaveBeenCalledWith({ x: 1 })
			expect(second).toHaveBeenCalledTimes(1)
			expect(second).toHaveBeenCalledWith({ x: 1 })
			// every subscribe retries all still-unconfirmed topics, so the
			// second subscriber triggers another sub message
			expect(sentMessages(ws)).toEqual([
				{ topic: `sub~${TOPIC}`, id: 1 },
				{ topic: `sub~${TOPIC}`, id: 2 },
			])
		})

		it("ignores publishes for unconfirmed topics", ({ expect }) => {
			const { state } = makeState()
			const fn = vi.fn()

			state.subscribe(TOPIC, fn)
			state.processMessage(
				messageEvent({ topic: `pub~${TOPIC}`, payload: { x: 1 } }),
			)

			expect(fn).toHaveBeenCalledTimes(0)
		})

		it("ignores non-publish payloads", ({ expect }) => {
			const { state } = makeState()
			const fn = vi.fn()

			state.subscribe(TOPIC, fn)
			confirm(state, 1)
			state.processMessage(
				messageEvent({ topic: `sub~${TOPIC}`, payload: { x: 1 } }),
			)

			expect(fn).toHaveBeenCalledTimes(0)
		})
	})

	describe("unsubscribe", () => {
		it("sends an unsubscription message when the last subscriber leaves", ({
			expect,
		}) => {
			const { ws, state } = makeState()
			const fn = vi.fn()

			const unsubscribe = state.subscribe(TOPIC, fn)
			confirm(state, 1)
			unsubscribe()
			state.processMessage(
				messageEvent({ topic: `pub~${TOPIC}`, payload: { x: 1 } }),
			)

			expect(sentMessages(ws)).toEqual([
				{ topic: `sub~${TOPIC}`, id: 1 },
				{ topic: `unsub~${TOPIC}`, id: 2 },
			])
			expect(fn).toHaveBeenCalledTimes(0)
		})

		it("keeps the topic alive while other subscribers remain", ({ expect }) => {
			const { ws, state } = makeState()
			const leaving = vi.fn()
			const staying = vi.fn()

			const unsubscribe = state.subscribe(TOPIC, leaving)
			state.subscribe(TOPIC, staying)
			confirm(state, 1)
			unsubscribe()
			state.processMessage(
				messageEvent({ topic: `pub~${TOPIC}`, payload: { x: 1 } }),
			)

			expect(staying).toHaveBeenCalledTimes(1)
			expect(leaving).toHaveBeenCalledTimes(0)
			// two sub messages (one per subscribe on the unconfirmed topic),
			// but no unsub — the topic still has a subscriber
			expect(sentMessages(ws)).toEqual([
				{ topic: `sub~${TOPIC}`, id: 1 },
				{ topic: `sub~${TOPIC}`, id: 2 },
			])
		})

		it("ignores repeated unsubscriptions", ({ expect }) => {
			const { ws, state } = makeState()

			const unsubscribe = state.subscribe(TOPIC, vi.fn())
			unsubscribe()
			unsubscribe()

			expect(sentMessages(ws)).toEqual([
				{ topic: `sub~${TOPIC}`, id: 1 },
				{ topic: `unsub~${TOPIC}`, id: 2 },
			])
		})
	})

	describe("processMessage", () => {
		it("ignores confirmations for unknown message ids", ({ expect }) => {
			const { state } = makeState()
			const fn = vi.fn()

			state.subscribe(TOPIC, fn)
			confirm(state, 999)
			state.processMessage(messageEvent({ topic: `pub~${TOPIC}`, payload: {} }))

			expect(fn).toHaveBeenCalledTimes(0)
		})

		it("ignores confirmations for topics unsubscribed in the meantime", ({
			expect,
		}) => {
			const { state } = makeState()
			const fn = vi.fn()

			const unsubscribe = state.subscribe(TOPIC, fn)
			unsubscribe()
			confirm(state, 1)
			state.processMessage(messageEvent({ topic: `pub~${TOPIC}`, payload: {} }))

			expect(fn).toHaveBeenCalledTimes(0)
		})
	})

	describe("ws", () => {
		it("marks all topics unconfirmed when the socket drops", ({ expect }) => {
			const { state } = makeState()
			const fn = vi.fn()

			state.subscribe(TOPIC, fn)
			confirm(state, 1)
			state.ws = null
			state.processMessage(messageEvent({ topic: `pub~${TOPIC}`, payload: {} }))

			expect(fn).toHaveBeenCalledTimes(0)
		})
	})

	describe("resubscribeAll", () => {
		it("resends subscriptions for every topic", ({ expect }) => {
			const { ws, state } = makeState()

			state.subscribe(TOPIC, vi.fn())
			confirm(state, 1)
			state.resubscribeAll()

			expect(sentMessages(ws)).toEqual([
				{ topic: `sub~${TOPIC}`, id: 1 },
				{ topic: `sub~${TOPIC}`, id: 2 },
			])
		})
	})

	describe("auto-resubscription", () => {
		it("retries unconfirmed subscriptions on the configured interval", ({
			expect,
		}) => {
			vi.useFakeTimers()

			try {
				const ws = fakeWs()
				const state = new WsState(5000)
				state.ws = ws
				state.subscribe(TOPIC, vi.fn())

				vi.advanceTimersByTime(5000)

				expect(sentMessages(ws)).toEqual([
					{ topic: `sub~${TOPIC}`, id: 1 },
					{ topic: `sub~${TOPIC}`, id: 2 },
				])

				// confirmed topics are not retried on the next tick
				confirm(state, 2)
				vi.advanceTimersByTime(5000)
				expect(ws.send).toHaveBeenCalledTimes(2)
			} finally {
				vi.useRealTimers()
			}
		})
	})
})
