import { mockNuxtImport } from "@nuxt/test-utils/runtime"
import { beforeEach, describe, it, vi } from "vitest"
import { nextTick, reactive, toValue } from "vue"
import plugin from "./websocket.client"

interface WsControlArgs {
	wsFailFn: () => void
	authenticated: unknown
}

const {
	useAuthSessionMock,
	useWebSocketStateMock,
	useWebSocketStateStoreMock,
} = vi.hoisted(() => {
	// the default implementations keep the app bootstrap alive: the real
	// plugin instance created during nuxt init consumes these before any
	// test arranges its own values
	return {
		useAuthSessionMock: vi.fn((): any => ({
			fetchAuthSession: {
				refresh: () => Promise.resolve({ data: undefined }),
			},
			fetchOrganization: {
				refresh: () => Promise.resolve({ data: undefined }),
			},
		})),
		useWebSocketStateMock: vi.fn((_opts: WsControlArgs): any => ({
			openConn: () => undefined,
			closeConn: () => undefined,
		})),
		useWebSocketStateStoreMock: vi.fn((): any => ({ init: () => undefined })),
	}
})

mockNuxtImport("useAuthSession", () => useAuthSessionMock)
mockNuxtImport("useWebSocketState", () => useWebSocketStateMock)
mockNuxtImport("useWebSocketStateStore", () => useWebSocketStateStoreMock)

function arrange(sessionValue: object | null) {
	const session = reactive({ data: { data: { session: sessionValue } } })
	const refresh = vi.fn().mockResolvedValue(session)
	useAuthSessionMock.mockReturnValue({ fetchAuthSession: { refresh } })

	const wsControl = { openConn: vi.fn(), closeConn: vi.fn() }
	useWebSocketStateMock.mockReturnValue(wsControl)

	const init = vi.fn()
	useWebSocketStateStoreMock.mockReturnValue({ init })

	return { session, refresh, wsControl, init }
}

// the tests arrange shared module-level mocks (mockNuxtImport singletons),
// so they cannot interleave
describe("websocket.client", { concurrent: false }, () => {
	// restoreMocks does not touch hand-made vi.fn() singletons in vitest 4
	// — reset the module-level mocks explicitly
	beforeEach(() => {
		useAuthSessionMock.mockReset()
		useWebSocketStateMock.mockReset()
		useWebSocketStateStoreMock.mockReset()
	})

	it("provides the websocket control and initializes the state store", async ({
		expect,
	}) => {
		const { refresh, wsControl, init } = arrange(null)

		// the plugin type is a union with the void setup variant; this
		// object-form setup resolves to its provide block
		const result = (await plugin(useNuxtApp())) as unknown as {
			provide: { wsControl: unknown }
		}

		expect(result.provide.wsControl).toBe(wsControl)
		expect(init).toHaveBeenCalledTimes(1)
		expect(refresh).toHaveBeenCalledTimes(1)
		expect(wsControl.openConn).toHaveBeenCalledTimes(0)
		expect(wsControl.closeConn).toHaveBeenCalledTimes(0)
	})

	it("opens the connection when the session becomes authenticated", async ({
		expect,
	}) => {
		const { session, wsControl } = arrange(null)
		await plugin(useNuxtApp())

		session.data.data.session = { id: "s1" }
		await nextTick()

		expect(wsControl.openConn).toHaveBeenCalledTimes(1)
		expect(wsControl.closeConn).toHaveBeenCalledTimes(0)
	})

	it("opens the connection immediately when already authenticated", async ({
		expect,
	}) => {
		const { wsControl } = arrange({ id: "s1" })

		await plugin(useNuxtApp())

		expect(wsControl.openConn).toHaveBeenCalledTimes(1)
		expect(wsControl.closeConn).toHaveBeenCalledTimes(0)
	})

	it("closes the connection when the session is lost", async ({ expect }) => {
		const { session, wsControl } = arrange({ id: "s1" })
		await plugin(useNuxtApp())

		session.data.data.session = null
		await nextTick()

		expect(wsControl.closeConn).toHaveBeenCalledTimes(1)
		expect(wsControl.openConn).toHaveBeenCalledTimes(1)
	})

	it("closes the connection when the window unloads", async ({ expect }) => {
		const { wsControl } = arrange({ id: "s1" })
		await plugin(useNuxtApp())

		window.dispatchEvent(new Event("beforeunload"))

		expect(wsControl.closeConn).toHaveBeenCalledTimes(1)
	})

	it("reports websocket failures to the console", async ({ expect }) => {
		arrange(null)
		const logSpy = vi.spyOn(console, "log").mockImplementation(() => undefined)

		await plugin(useNuxtApp())

		const opts = useWebSocketStateMock.mock.calls[0]?.[0]
		expect(toValue(opts?.authenticated)).toBe(false)

		opts?.wsFailFn()

		expect(logSpy).toHaveBeenCalledExactlyOnceWith(
			"websocket connection failed",
		)
	})
})
