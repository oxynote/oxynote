import { serve } from "@hono/node-server"
import { Hono } from "hono"
import { createNodeWebSocket } from "@hono/node-ws"
import { createClient } from "redis"
import { loadEnv } from "./env.js"
import { createDatabase } from "./db.js"
import { createCoreClient } from "./core.js"
import { createAuth } from "./auth.js"
import { createHocuspocus } from "./hocuspocus.js"
import { createRoutes } from "./routes.js"
import { bestEffort } from "./reporting.js"

// the composition root: the only module that reads the environment, opens
// connections, or listens. Everything below it is a factory taking what it
// needs, which is what makes the rest of the service testable without a
// database, a redis, or a port.
const env = loadEnv(process.env)

const database = createDatabase(env.databaseDSN)
const { store, dialect } = database
const core = createCoreClient(env.coreUrl)

// valkey is optional: without it better-auth keeps no secondary storage,
// so nothing is dialed here either.
//
// RESP2: node-redis 6 defaults to RESP3, whose reply shapes better-auth's
// secondary storage does not read correctly
// the type is spelled out because the shutdown handler reads it from a
// closure, where an inferred evolving any does not survive.
let redis: ReturnType<typeof createClient> | undefined

if (env.valkeyDsn) {
	redis = createClient({ url: env.valkeyDsn, RESP: 2 })
	await redis.connect()
}

const auth = createAuth({ env, store, dialect, redis, core })

const hocuspocus = createHocuspocus({
	auth: { getSession: (input) => auth.api.getSession(input) },
	core,
})

const app = new Hono()
// kept as one object rather than destructured: both are methods bound to
// the adapter's own state.
const nodeWs = createNodeWebSocket({ app })

app.route("/api", createRoutes({ env, auth, store, hocuspocus, core }))

// the RFC 9728 protected-resource metadata for the MCP surface is served
// by the mcp plugin's request hook, which matches root-anchored
// well-known paths — the request must reach better-auth with its
// original path, unlike the basePath'd /api/auth endpoints.
app.on(
	["GET", "HEAD"],
	[
		"/.well-known/oauth-protected-resource",
		"/.well-known/oauth-protected-resource/*",
	],
	(c) => auth.handler(c.req.raw),
)
app.get(
	"/hocuspocus",
	nodeWs.upgradeWebSocket((c) => {
		// hocuspocus does not attach its own socket listeners here, so the
		// frames hono receives have to be fed in. Without that the client's
		// opening Sync message is dropped and the document never syncs.
		let clientConnection:
			| ReturnType<typeof hocuspocus.handleConnection>
			| undefined

		return {
			onOpen(_evt, ws) {
				if (!ws.raw) {
					return
				}

				ws.raw.binaryType = "arraybuffer"
				clientConnection = hocuspocus.handleConnection(
					ws.raw,
					c.req.raw,
				)
			},
			onMessage(evt) {
				if (!(evt.data instanceof ArrayBuffer)) {
					return
				}

				clientConnection?.handleMessage(
					new Uint8Array(evt.data),
				)
			},
			onClose() {
				clientConnection?.handleClose()
			},
		}
	}),
)

const server = serve(
	{
		fetch: app.fetch,
		hostname: env.listenHost,
		port: env.listenPort,
	},
	(info) => {
		void hocuspocus.hooks("onListen", {
			instance: hocuspocus,
			configuration: hocuspocus.configuration,
			port: info.port,
		})

		console.log(
			`Server is running on http://localhost:${info.port}`,
		)
	},
)
nodeWs.injectWebSocket(server)

// how long a graceful shutdown may take before the process force-exits.
const shutdownDeadlineMs = 15_000

// graceful shutdown: stop accepting HTTP, then drain hocuspocus before
// exiting — its onStoreDocument persists are debounced for up to ten
// seconds, so exiting on the signal alone would drop that window of edits.
async function shutdown() {
	const deadline = setTimeout(() => {
		console.error("shutdown deadline exceeded")
		process.exit(1)
	}, shutdownDeadlineMs)
	deadline.unref()

	const httpClosed = new Promise<void>((resolve) => {
		server.close(() => {
			resolve()
		})
	})

	// the drain hocuspocus's own standalone server performs in destroy():
	// a document unloads only after its pending store completed, so once
	// every document is gone the debounced persists have run. The
	// extension is registered before connections close so no unload can
	// slip past it.
	await new Promise<void>((resolve) => {
		if (hocuspocus.getDocumentsCount() === 0) {
			resolve()
			return
		}

		hocuspocus.configuration.extensions.push({
			afterUnloadDocument({ instance }) {
				if (instance.getDocumentsCount() === 0) {
					resolve()
				}

				return Promise.resolve()
			},
		})

		hocuspocus.closeConnections()
		hocuspocus.flushPendingStores()
	})

	await hocuspocus.hooks("onDestroy", { instance: hocuspocus })
	await httpClosed

	// redis is absent on a deployment running without valkey.
	await bestEffort(() => redis?.close())
	await bestEffort(() => database.close())

	process.exit(0)
}

process.once("SIGINT", () => void shutdown())
process.once("SIGTERM", () => void shutdown())
