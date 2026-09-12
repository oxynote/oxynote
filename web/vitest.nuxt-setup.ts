import { registerEndpoint } from "@nuxt/test-utils/runtime"

// the nuxt project points @nuxt/icon at no provider, so every icon that is
// not already in the client bundle fails its load and warns once per
// render. The load is asynchronous and the warning often lands after the
// test has finished, and vitest 5 turns a console message caught while the
// worker is tearing down into an unhandled EnvironmentTeardownError. The
// warning is dropped here, before vitest's console interception forwards
// it, rather than in the main process's onConsoleLog, which runs too late
// to stop the forwarding.
const warn = console.warn

console.warn = (...args: unknown[]) => {
	if (
		typeof args[0] === "string" &&
		args[0].startsWith("[Icon] failed to load icon")
	) {
		return
	}

	warn(...args)
}

// mounting anything inside the nuxt runtime boots a router, and the global
// middleware in app/middleware/01.redirect.global.ts asks better-auth for
// the current session before every navigation. Unstubbed that is a real
// request to the (nonexistent) test host, costing ~5s per mount before it
// gives up — so the nuxt project stubs a signed-out session for every
// suite. Suites that need a session register the same url again: the
// endpoint registry resolves the most recently registered handler first.
//
// It takes both spellings of the url, for two different reasons.
// @nuxt/test-utils only diverts a request away from the real network when
// the string the client asked for is in its registry, and better-auth asks
// for the absolute url its client was built with — so the absolute entry is
// what keeps the request off the network. The diverted request then reaches
// the test-time h3 app with the origin stripped, so the handler that
// actually answers it is the one filed under the bare path. With only the
// absolute entry the call 404s; with only the path entry it goes to the
// network and stalls.
registerEndpoint("http://test.local/auth-realtime/api/auth/get-session", () => {
	return null
})
registerEndpoint("/auth-realtime/api/auth/get-session", () => {
	return null
})
