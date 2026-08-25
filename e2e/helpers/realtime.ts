import {
	HocuspocusProvider,
	HocuspocusProviderWebsocket,
} from "@hocuspocus/provider"
import type { Page } from "@playwright/test"
import WebSocket from "ws"
import { sessionCookie } from "./api"
import { REALTIME_WS_URL } from "./config"

// documentAccepted opens a hocuspocus connection for documentName as the
// page's own user and reports the verdict the realtime server answers
// with. It runs the app's own client, given the app's own arguments —
// the document name is the only thing a test chooses.
//
// The provider builds its socket from a url and nothing else, so the
// session rides in a subclass that adds the cookie header: a browser
// attaches it unasked, node does not.
export async function documentAccepted(
	page: Page,
	documentName: string,
): Promise<boolean> {
	const cookie = await sessionCookie(page)

	class SessionWebSocket extends WebSocket {
		constructor(url: string) {
			super(url, { headers: { cookie } })
		}
	}

	const socket = new HocuspocusProviderWebsocket({
		url: REALTIME_WS_URL,
		WebSocketPolyfill: SessionWebSocket,
	})

	return new Promise<boolean>((resolve) => {
		let provider: HocuspocusProvider | null = null

		const settle = (accepted: boolean): void => {
			provider?.destroy()
			socket.destroy()
			resolve(accepted)
		}

		provider = new HocuspocusProvider({
			name: documentName,
			token: "notoken",
			websocketProvider: socket,
			onAuthenticated: () => {
				settle(true)
			},
			onAuthenticationFailed: () => {
				settle(false)
			},
			// a refusal the server closes on rather than answers
			// would otherwise hang until the test budget expires
			onClose: () => {
				settle(false)
			},
		})

		// the provider only wires itself up to a socket it created
		// itself; one passed in has to be attached by hand, and without
		// this nothing ever connects
		provider.attach()
	})
}
