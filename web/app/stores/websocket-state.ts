import { defineStore } from "pinia"
import WsState from "~/utils/websocket"

export const useWebSocketStateStore = defineStore("websocket-state", () => {
	const state = ref<WsState | null>(null)

	function init() {
		if (import.meta.client && !state.value) {
			state.value = new WsState()
		}
	}

	return {
		init,
		state,
	}
})
