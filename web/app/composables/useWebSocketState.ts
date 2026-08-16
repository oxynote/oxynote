const MAX_WS_RETRIES = 30

export default function (opts: {
	wsFailFn: () => void
	authenticated: MaybeRefOrGetter<boolean>
}) {
	if (!import.meta.client) {
		return undefined
	}

	const store = useWebSocketStateStore()
	const config = useRuntimeConfig()

	let wsConnRetry = 0
	const wsControl = useWebSocket(`${config.public.coreAPIBaseWsURL}/api/ws`, {
		immediate: false,
		autoClose: false,
		autoReconnect: {
			retries: (): boolean => {
				if (wsConnRetry >= MAX_WS_RETRIES || !toValue(opts.authenticated)) {
					return false
				}

				wsConnRetry++

				return true
			},
			delay: 2500,
			onFailed: () => {
				if (!toValue(opts.authenticated)) {
					return
				}

				opts.wsFailFn()
			},
		},
		onConnected: (ws: WebSocket) => {
			wsConnRetry = 0

			if (store.state) {
				store.state.ws = ws
				store.state.resubscribeAll()
			}
		},
		onDisconnected: () => {
			if (store.state) {
				store.state.ws = null
			}
		},
		onMessage: (_: WebSocket, msg: MessageEvent<string>) => {
			if (store.state) {
				store.state.processMessage(msg)
			}
		},
	})

	function openConn() {
		wsControl.open()
	}

	function closeConn() {
		wsControl.close()
	}

	return {
		openConn,
		closeConn,
	}
}
