export default defineNuxtPlugin({
	parallel: true,
	setup: async (nuxtApp) => {
		const { fetchAuthSession } = useAuthSession()
		const sessionReq = await fetchAuthSession.refresh()
		const sessionActive = computed(() => !!sessionReq.data?.data?.session)

		const { init } = useWebSocketStateStore()
		init()

		const wsControl = useWebSocketState({
			wsFailFn: () => {
				// TODO
				console.log("websocket connection failed")
			},
			authenticated: sessionActive,
		})

		// open a connections only when the user is authenticated
		void nuxtApp.runWithContext(() => {
			let beforeUnloadHandler: (() => void) | null = null

			watchImmediate(sessionActive, (isAuth, wasAuth) => {
				if (isAuth && !wasAuth) {
					wsControl?.openConn()

					if (typeof window !== "undefined") {
						beforeUnloadHandler = () => {
							wsControl?.closeConn()
						}
						window.addEventListener("beforeunload", beforeUnloadHandler)

						if (import.meta.hot) {
							const handler = beforeUnloadHandler

							import.meta.hot.dispose(() => {
								window.removeEventListener("beforeunload", handler)
							})
						}
					}
				} else if (!isAuth && wasAuth) {
					wsControl?.closeConn()

					if (beforeUnloadHandler) {
						window.removeEventListener("beforeunload", beforeUnloadHandler)
						beforeUnloadHandler = null
					}
				}
			})
		})

		return { provide: { wsControl } }
	},
})
