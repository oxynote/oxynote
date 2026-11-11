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
// needs, which is what makes the rest of the service testable.

let log = createLogger("INFO", destination(1))

function formatError(error: unknown): string {
	if (error instanceof AggregateError) {
		return [
			error.message,
			...error.errors.map((cause: unknown) =>
				formatError(cause),
			),
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

	let database: ReturnType<typeof createDatabase> | undefined
	let redis: ReturnType<typeof createClient> | undefined
	let hocuspocus:
		ReturnType<typeof createHocuspocus>["instance"] | undefined
	let server: ReturnType<typeof serve> | undefined
	let webSocketServer:
		ReturnType<typeof createNodeWebSocket>["wss"] | undefined

	let shutdownPromise: Promise<void> | undefined
	let exitCode = 0

	const shutdownDeadlineMs = 15_000

	// Clean up resources that were successfully initialized.
	async function cleanup() {
		// Stop accepting new HTTP connections. Do not wait yet because
		// upgraded WebSocket connections need to be closed separately.
		const currentServer = server

		const httpClosed = new Promise<void>((resolve) => {
			if (!currentServer?.listening) {
				resolve()
				return
			}

			currentServer.close(() => {
				resolve()
			})
		})

		const currentHocuspocus = hocuspocus

		if (currentHocuspocus) {
			// Ask Hocuspocus to close its document connections.
			currentHocuspocus.closeConnections()
		}

		// Hocuspocus closeConnections() does not itself terminate the
		// underlying WebSocket transport connections.
		const currentWebSocketServer = webSocketServer

		if (currentWebSocketServer) {
			for (const client of currentWebSocketServer.clients) {
				client.close(1001, "Server shutting down")
			}
		}

		if (currentHocuspocus) {
			// Register the unload listener before flushing or closing
			// connections so the final document cannot be missed.
			const documentsClosed = new Promise<void>((resolve) => {
				if (
					currentHocuspocus.getDocumentsCount() ===
					0
				) {
					resolve()
					return
				}

				currentHocuspocus.configuration.extensions.push(
					{
						afterUnloadDocument({
							instance,
						}) {
							if (
								instance.getDocumentsCount() ===
								0
							) {
								resolve()
							}

							return Promise.resolve()
						},
					},
				)
			})

			// Flush pending document persistence operations.
			currentHocuspocus.flushPendingStores()

			await documentsClosed

			await bestEffort(() =>
				currentHocuspocus.hooks("onDestroy", {
					instance: currentHocuspocus,
				}),
			)
		}

		// Wait for ordinary HTTP connections to close.
		await httpClosed

		// Close dependencies only after document persistence has completed.
		const currentRedis = redis

		if (currentRedis) {
			await bestEffort(() => currentRedis.close())
		}

		const currentDatabase = database

		if (currentDatabase) {
			await bestEffort(() => currentDatabase.close())
		}
	}

	// All termination paths share this function.
	// A fatal error takes precedence over a normal signal-based shutdown.
	function shutdown(code: number): Promise<void> {
		if (code !== 0) {
			exitCode = code
		}

		process.exitCode = exitCode

		if (shutdownPromise) {
			return shutdownPromise
		}

		// Emergency deadline in case document draining or connection
		// closure never completes.
		const deadline = setTimeout(() => {
			log.error("shutdown deadline exceeded")

			// Force-close remaining WebSocket transports.
			for (const client of webSocketServer?.clients ?? []) {
				client.terminate()
			}

			process.exit(exitCode || 1)
		}, shutdownDeadlineMs)

		shutdownPromise = (async () => {
			try {
				await cleanup()
			} catch (error: unknown) {
				log.error(
					`Shutdown error: ${formatError(error)}`,
				)
				exitCode = 1
			} finally {
				clearTimeout(deadline)
				process.exitCode = exitCode
			}
		})()

		return shutdownPromise
	}

	try {
		database = createDatabase(env.databaseDSN)

		const { store, dialect } = database
		const core = createCoreClient(env.coreUrl)

		// Valkey is optional: without it Better Auth keeps no secondary
		// storage, so nothing is dialed here.
		//
		// RESP2: node-redis 6 defaults to RESP3, whose reply shapes Better
		// Auth's secondary storage does not read correctly.
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

						return Math.min(
							2 ** retries * 50,
							3000,
						)
					},
				},
			})

			redis.on("error", (error) => {
				if (connecting) return

				log.error(
					`Valkey client error: ${formatError(error)}`,
				)
			})

			try {
				await redis.connect()
			} finally {
				connecting = false
			}
		}

		const auth = createAuth({
			env,
			store,
			dialect,
			redis,
			core,
			log,
		})

		const authContext = await auth.$context

		// Before listening, so no signup can race the organization's creation.
		await bootstrapSingleOrganization({
			env,
			store,
			log,
			accounts: authContext.internalAdapter,
			hashPassword: (password) =>
				authContext.password.hash(password),
			createOrganization: (organization) =>
				auth.api.createOrganization({
					body: organization,
				}),
			random: Math.random,
		})

		const hocuspocusResult = createHocuspocus({
			auth: {
				getSession: (input) =>
					auth.api.getSession(input),
			},
			core,
			log,
		})

		const currentHocuspocus = hocuspocusResult.instance
		hocuspocus = currentHocuspocus

		const { flushDocument, resetConnections } = hocuspocusResult

		const app = new Hono()

		const nodeWs = createNodeWebSocket({ app })
		webSocketServer = nodeWs.wss

		app.route(
			"/api",
			createRoutes({
				env,
				auth,
				store,
				hocuspocus: currentHocuspocus,
				flushDocument,
				resetConnections,
				findDefaultAdmin: createDefaultAdminLookup({
					store,
					verifyPassword: (data) =>
						authContext.password.verify(
							data,
						),
				}),
				core,
			}),
		)

		// The RFC 9728 protected-resource metadata for the MCP surface is
		// served by the MCP plugin's request hook.
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
				let clientConnection:
					| ReturnType<
							typeof currentHocuspocus.handleConnection
					  >
					| undefined

				return {
					onOpen(_evt, ws) {
						if (!ws.raw) {
							return
						}

						ws.raw.binaryType =
							"arraybuffer"
						clientConnection =
							currentHocuspocus.handleConnection(
								ws.raw,
								c.req.raw,
							)
					},
					onMessage(evt) {
						if (
							!(
								evt.data instanceof
								ArrayBuffer
							)
						) {
							return
						}

						clientConnection?.handleMessage(
							new Uint8Array(
								evt.data,
							),
						)
					},
					onClose() {
						clientConnection?.handleClose()
					},
				}
			}),
		)

		server = serve(
			{
				fetch: app.fetch,
				hostname: env.listenHost,
				port: env.listenPort,
			},
			(info) => {
				void currentHocuspocus.hooks("onListen", {
					instance: currentHocuspocus,
					configuration:
						currentHocuspocus.configuration,
					port: info.port,
				})

				log.info("listening")
			},
		)

		nodeWs.injectWebSocket(server)

		// Handle asynchronous HTTP server errors, including EADDRINUSE.
		server.on("error", (error: unknown) => {
			log.error(`HTTP server error: ${formatError(error)}`)
			void shutdown(1)
		})

		process.once("SIGINT", () => {
			void shutdown(0)
		})

		process.once("SIGTERM", () => {
			void shutdown(0)
		})
	} catch (error: unknown) {
		log.error(formatError(error))
		await shutdown(1)
	}
}

main().catch((error: unknown) => {
	log.error(`Fatal lifecycle error: ${formatError(error)}`)
	process.exitCode = 1
})
