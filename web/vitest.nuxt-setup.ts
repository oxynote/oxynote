import { registerEndpoint } from "@nuxt/test-utils/runtime"

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
