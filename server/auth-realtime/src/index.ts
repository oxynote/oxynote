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

	// Resources are optional because startup can fail at any point.
	const database = createDatabase(env.databaseDSN)
	let redis: ReturnType<typeof createClient> | undefined
	let hocuspocus:
		ReturnType<typeof createHocuspocus>["instance"] | undefined
	let server: ReturnType<typeof serve> | undefined

	let shutdownPromise: Promise<void> | undefined
	let exitCode = 0

	// Close whichever resources were successfully initialized.
	async function cleanup() {
		const currentServer = server

		if (currentServer?.listening) {
			await new Promise<void>((resolve) => {
				currentServer.close(() => {
					resolve()
				})
			})
		}

		const currentHocuspocus = hocuspocus

		if (currentHocuspocus) {
			await bestEffort(async () => {
				if (currentHocuspocus.getDocumentsCount() > 0) {
					await new Promise<void>((resolve) => {
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

						currentHocuspocus.closeConnections()
						currentHocuspocus.flushPendingStores()
					})
				}
			})

			await bestEffort(() =>
				currentHocuspocus.hooks("onDestroy", {
					instance: currentHocuspocus,
				}),
			)
		}

		const currentRedis = redis

		if (currentRedis) {
			await bestEffort(() => currentRedis.close())
		}

		await bestEffort(() => database.close())
	}

	// The same shutdown path handles signals and HTTP server failures.
	function shutdown(code: number): Promise<void> {
		if (code !== 0) {
			exitCode = code
		}

		process.exitCode = exitCode

		if (shutdownPromise) {
			return shutdownPromise
		}

		const deadline = setTimeout(() => {
			log.error("shutdown deadline exceeded")
			process.exit(exitCode || 1)
		}, 15_000)

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
		const { store, dialect } = database
		const core = createCoreClient(env.coreUrl)

		if (env.valkeyDsn) {
			let connecting = true

			redis = createClient({
				url: env.valkeyDsn,
				RESP: 2,
				socket: {
					reconnectStrategy: (retries) => {
						if (connecting) return false
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
						if (!ws.raw) return

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
						)
							return
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

		server.on("error", (error: unknown) => {
			log.error(`HTTP server error: ${formatError(error)}`)
			void shutdown(1)
		})

		process.once("SIGINT", () => void shutdown(0))
		process.once("SIGTERM", () => void shutdown(0))
	} catch (error: unknown) {
		log.error(formatError(error))
		await shutdown(1)
	}
}

main().catch((error: unknown) => {
	log.error(formatError(error))
	process.exitCode = 1
})
