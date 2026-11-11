
import { serve } from "@hono/node-server"
import { Hono } from "hono"
import { createNodeWebSocket } from "@hono/node-ws"
import { createClient } from "redis"
import { destination } from "pino"
import { loadEnv } from "./env.js"
import { createDatabase } from "./db.js"
import { createCoreClient } from "./core.js"
import { createAuth } from "./auth.js"
import {
	bootstrapSingleOrganization,
	createDefaultAdminLookup,
} from "./bootstrap.js"
import { createHocuspocus } from "./hocuspocus.js"
import { createLogger } from "./logging.js"
import { createRoutes } from "./routes.js"
import { bestEffort } from "./reporting.js"

// The composition root: the only module that reads the environment, opens
// connections, or listens. Everything below it is a factory taking what it
// needs, which is what makes the rest of the service testable without a
// database, a redis, or a port.

let log = createLogger("INFO", destination(1))

function formatError(error: unknown): string {
	if (error instanceof AggregateError) {
		const causes = error.errors.map((cause: unknown) =>
			formatError(cause),
		)

		return [
			error.message,
			...causes,
		]
			.filter(Boolean)
			.join("; ")
	}

	if (error instanceof Error) {
		return error.message || error.name
	}

	return String(error)
}

async function main() {
	const env = loadEnv(process.env)
	log = createLogger(env.logLevel, destination(1))

	const database = createDatabase(env.databaseDSN)
	const { store, dialect } = database
	const core = createCoreClient(env.coreUrl)

	// Valkey is optional: without it better-auth keeps no secondary storage,
	// so nothing is dialed here either.
	//
	// RESP2: node-redis 6 defaults to RESP3, whose reply shapes better-auth's
	// secondary storage does not read correctly.
	// The type is spelled out because the shutdown handler reads it from a
	// closure, where an inferred evolving any does not survive.
	let redis: ReturnType<typeof createClient> | undefined

	if (env.valkeyDsn) {
		let connecting = true

		redis = createClient({
			url: env.valkeyDsn,
			RESP: 2,
			socket: {
				reconnectStrategy: (retries) => {
					if (connecting) {
						return false
					}

					return Math.min(2 ** retries * 50, 3000)
				},
			},
		})

		redis.on("error", (err) => {
			if (connecting) return

			const message =
				err instanceof Error && err.message
					? err.message
					: String(err)

			log.error(
				`Valkey client error: ${message || "unknown error"}`,
			)
		})

		try {
			await redis.connect()
		} finally {
			connecting = false
		}
	}

	const auth = createAuth({ env, store, dialect, redis, core, log })
	const authContext = await auth.$context

	// Before listening, so no signup can race the organization's creation.
	await bootstrapSingleOrganization({
		env,
		store,
		log,
		accounts: authContext.internalAdapter,
		hashPassword: (password) => authContext.password.hash(password),
		createOrganization: (organization) =>
			auth.api.createOrganization({ body: organization }),
		random: Math.random,
	})

	const {
		instance: hocuspocus,
		flushDocument,
		resetConnections,
	} = createHocuspocus({
		auth: { getSession: (input) => auth.api.getSession(input) },
		core,
		log,
	})

	const app = new Hono()

	// Kept as one object rather than destructured: both are methods bound to
	// the adapter's own state.
	const nodeWs = createNodeWebSocket({ app })

	app.route(
		"/api",
		createRoutes({
			env,
			auth,
			store,
			hocuspocus,
			flushDocument,
			resetConnections,
			findDefaultAdmin: createDefaultAdminLookup({
				store,
				verifyPassword: (data) =>
					authContext.password.verify(data),
			}),
			core,
		}),
	)

	// The RFC 9728 protected-resource metadata for the MCP surface is served
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
			// Hocuspocus does not attach its own socket listeners here, so the
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

			log.info("listening")
		},
	)

	nodeWs.injectWebSocket(server)

	// Handle asynchronous HTTP server errors, such as EADDRINUSE.
	server.on("error", (error: unknown) => {
		log.error(`HTTP server error: ${formatError(error)}`)
		process.exitCode = 1
	})

	// How long a graceful shutdown may take before the process force-exits.
	const shutdownDeadlineMs = 15_000

	// Graceful shutdown: stop accepting HTTP, then drain hocuspocus before
	// exiting — its onStoreDocument persists are debounced for up to ten
	// seconds, so exiting on the signal alone would drop that window of edits.
	async function shutdown() {
		const deadline = setTimeout(() => {
			log.error("shutdown deadline exceeded")
			log.flush(() => process.exit(1))
		}, shutdownDeadlineMs)
		deadline.unref()

		const httpClosed = new Promise<void>((resolve) => {
			server.close(() => {
				resolve()
			})
		})

		// The drain hocuspocus's own standalone server performs in destroy():
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

		// Redis is absent on a deployment running without Valkey.
		await bestEffort(() => redis?.close())
		await bestEffort(() => database.close())

		process.exit(0)
	}

	process.once("SIGINT", () => void shutdown())
	process.once("SIGTERM", () => void shutdown())
}

// Handle rejected startup operations in one place.
main().catch((error: unknown) => {
	log.error(formatError(error))
	process.exitCode = 1
})
