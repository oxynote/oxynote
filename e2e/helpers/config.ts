// the ports the e2e stack publishes; they differ from the dev stack's so
// both can run at once. Changing one means changing docker-compose.yaml.
export const BASE_URL = "http://localhost:18080"
export const MAILPIT_URL = "http://localhost:18025"
export const REALTIME_WS_URL = `ws://localhost:18080/auth-realtime/hocuspocus`
// the mcp surface's canonical url, which is also the OAuth resource
// (RFC 8707) a token is bound to.
export const MCP_URL = `${BASE_URL}/core/api/mcp`
// where the authorization server sends the browser back with the code.
// It is mailpit's origin: an address in the stack that is not the app
// under test, answers, and leaves the query string alone — the only
// things a redirect target has to do here. A loopback http address also
// needs the client to register as native; a web client would have to
// offer https.
export const MCP_REDIRECT_URL = "http://127.0.0.1:18025/"
