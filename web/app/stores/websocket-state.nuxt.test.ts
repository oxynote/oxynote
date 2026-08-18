import { createPinia } from "pinia"
import { describe, it } from "vitest"
import WsState from "~/utils/websocket"
import { useWebSocketStateStore } from "./websocket-state"

describe("useWebSocketStateStore", () => {
	describe("init", () => {
		it("creates the websocket state on the client", ({ expect }) => {
			const store = useWebSocketStateStore(createPinia())

			store.init()

			expect(store.state).toBeInstanceOf(WsState)
		})

		it("keeps the existing state on repeated init", ({ expect }) => {
			const store = useWebSocketStateStore(createPinia())
			store.init()
			const first = store.state

			store.init()

			expect(store.state).toBe(first)
		})
	})
})
