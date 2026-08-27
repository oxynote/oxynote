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

// the composition root: the only module that reads the environment, opens
// connections, or listens. Everything below it is a factory taking what it
// needs, which is what makes the rest of the service testable without a
// database, a redis, or a port.
const env = loadEnv(process.env)

const { store, dialect } = createDatabase(env.databaseDSN)
const core = createCoreClient(env.coreUrl)

// valkey is optional: without it better-auth keeps no secondary storage,
// so nothing is dialed here either.
//
// RESP2: node-redis 6 defaults to RESP3, whose reply shapes better-auth's
// secondary storage does not read correctly
let redis

if (env.valkeyUrl) {
	redis = createClient({ url: env.valkeyUrl, RESP: 2 })
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
		port: 8081,
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

// graceful shutdown
process.on("SIGINT", () => {
	server.close()
	process.exit(0)
})

process.on("SIGTERM", () => {
	server.close((err) => {
		if (err) {
			console.error(err)
			process.exit(1)
		}

		process.exit(0)
	})
})
