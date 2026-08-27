// which stack the run is driving. Two suites share these tests: the dev
// stack (docker-compose.dev.yaml) and the all-in-one image an operator
// installs (docker-compose.prod.yaml). Each publishes its own ports so
// both can be up at once, and each playwright config names its stack here
// before playwright loads anything else — the workers are spawned from
// this process, so they inherit the choice.
export const STACK = process.env.E2E_STACK === "prod" ? "prod" : "dev"

// the ports the stack under test publishes. Changing one means changing
// the matching docker-compose file.
const PORTS =
	STACK === "prod"
		? { frontDoor: "19080", mailpit: "19025" }
		: { frontDoor: "18080", mailpit: "18025" }

export const BASE_URL = `http://localhost:${PORTS.frontDoor}`
export const MAILPIT_URL = `http://localhost:${PORTS.mailpit}`
export const REALTIME_WS_URL = `ws://localhost:${PORTS.frontDoor}/auth-realtime/hocuspocus`
// the mcp surface's canonical url, which is also the OAuth resource
// (RFC 8707) a token is bound to.
export const MCP_URL = `${BASE_URL}/core/api/mcp`
// where the authorization server sends the browser back with the code.
// It is mailpit's origin: an address in the stack that is not the app
// under test, answers, and leaves the query string alone — the only
// things a redirect target has to do here. A loopback http address also
// needs the client to register as native; a web client would have to
// offer https.
export const MCP_REDIRECT_URL = `http://127.0.0.1:${PORTS.mailpit}/`
