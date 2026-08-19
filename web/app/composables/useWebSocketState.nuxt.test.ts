import { mockNuxtImport } from "@nuxt/test-utils/runtime"
import { beforeEach, describe, it, vi } from "vitest"
import useWebSocketState from "./useWebSocketState"

interface CapturedWsOptions {
	immediate?: boolean
	autoClose?: boolean
	autoReconnect?: {
		retries: () => boolean
		delay: number
		onFailed: () => void
	}
	onConnected?: (ws: unknown) => void
	onDisconnected?: () => void
	onMessage?: (ws: unknown, msg: unknown) => void
}

const { useWebSocketMock, useWebSocketStateStoreMock } = vi.hoisted(() => {
	// the default implementations keep the app bootstrap alive: the
	// websocket plugin builds this composable during nuxt init
	return {
		useWebSocketMock: vi.fn((_url: unknown, _opts: CapturedWsOptions): any => ({
			open: () => undefined,
			close: () => undefined,
		})),
		// init exists because the websocket plugin calls it during nuxt init
		useWebSocketStateStoreMock: vi.fn((): any => ({
			state: null,
			init: () => undefined,
		})),
	}
})

mockNuxtImport("useWebSocket", () => useWebSocketMock)
mockNuxtImport("useWebSocketStateStore", () => useWebSocketStateStoreMock)

function arrange(opts?: {
	authenticated?: boolean
	storeState?: object | null
}) {
	const open = vi.fn()
	const close = vi.fn()
	useWebSocketMock.mockReturnValue({ open, close })
	useWebSocketStateStoreMock.mockReturnValue({
		state: opts?.storeState ?? null,
	})

	const wsFailFn = vi.fn()
	const control = useWebSocketState({
		wsFailFn,
		authenticated: opts?.authenticated ?? true,
	})

	const captured = useWebSocketMock.mock.calls[0]
	if (!control || !captured) {
		throw new Error("the composable did not create a websocket")
	}

	return { control, open, close, wsFailFn, url: captured[0], ws: captured[1] }
}

function makeStoreState() {
	return {
		ws: null as unknown,
		resubscribeAll: vi.fn(),
		processMessage: vi.fn(),
	}
}

// the tests arrange shared module-level mocks (mockNuxtImport singletons),
// so they cannot interleave
describe("useWebSocketState", { concurrent: false }, () => {
	// restoreMocks does not touch hand-made vi.fn() singletons in vitest 4
	// — reset the module-level mocks explicitly
	beforeEach(() => {
		useWebSocketMock.mockReset()
		useWebSocketStateStoreMock.mockReset()
	})

	it("connects to the core websocket endpoint without auto-connecting", ({
		expect,
	}) => {
		const { url, ws } = arrange()

		// the core ws base is empty in the test runtime config
		expect(url).toBe("/api/ws")
		expect(ws.immediate).toBe(false)
		expect(ws.autoClose).toBe(false)
	})

	it("opens and closes the connection on demand", ({ expect }) => {
		const { control, open, close } = arrange()

		control.openConn()
		control.closeConn()

		expect(open).toHaveBeenCalledTimes(1)
		expect(close).toHaveBeenCalledTimes(1)
	})

	describe("autoReconnect", () => {
		it("retries up to the limit while authenticated", ({ expect }) => {
			const { ws } = arrange({ authenticated: true })

			const results = Array.from({ length: 31 }, () =>
				ws.autoReconnect?.retries(),
			)

			expect(results.slice(0, 30)).toEqual(
				Array.from({ length: 30 }).fill(true),
			)
			expect(results[30]).toBe(false)
		})

		it("does not retry when unauthenticated", ({ expect }) => {
			const { ws } = arrange({ authenticated: false })

			expect(ws.autoReconnect?.retries()).toBe(false)
		})

		it("resets the retry budget on a successful connection", ({ expect }) => {
			const { ws } = arrange({ authenticated: true })

			for (let i = 0; i < 30; i++) {
				ws.autoReconnect?.retries()
			}
			expect(ws.autoReconnect?.retries()).toBe(false)

			ws.onConnected?.({})

			expect(ws.autoReconnect?.retries()).toBe(true)
		})

		it("reports the failure only while authenticated", ({ expect }) => {
			const { ws, wsFailFn } = arrange({ authenticated: true })

			ws.autoReconnect?.onFailed()

			expect(wsFailFn).toHaveBeenCalledTimes(1)
		})

		it("swallows the failure when unauthenticated", ({ expect }) => {
			const { ws, wsFailFn } = arrange({ authenticated: false })

			ws.autoReconnect?.onFailed()

			expect(wsFailFn).toHaveBeenCalledTimes(0)
		})
	})

	it("attaches the socket to the state store on connect", ({ expect }) => {
		const storeState = makeStoreState()
		const { ws } = arrange({ storeState })
		const socket = {}

		ws.onConnected?.(socket)

		expect(storeState.ws).toBe(socket)
		expect(storeState.resubscribeAll).toHaveBeenCalledTimes(1)
	})

	it("detaches the socket from the state store on disconnect", ({ expect }) => {
		const storeState = makeStoreState()
		storeState.ws = {}
		const { ws } = arrange({ storeState })

		ws.onDisconnected?.()

		expect(storeState.ws).toBeNull()
	})

	it("forwards messages to the state store", ({ expect }) => {
		const storeState = makeStoreState()
		const { ws } = arrange({ storeState })
		const message = { data: "{}" }

		ws.onMessage?.({}, message)

		expect(storeState.processMessage).toHaveBeenCalledExactlyOnceWith(message)
	})

	it("tolerates a missing state store on socket events", ({ expect }) => {
		const { ws } = arrange({ storeState: null })

		expect(() => {
			ws.onConnected?.({})
			ws.onDisconnected?.()
			ws.onMessage?.({}, { data: "{}" })
		}).not.toThrow()
	})
})
