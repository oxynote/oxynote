import { execFile, spawn, spawnSync } from "node:child_process"
import {
	chmodSync,
	existsSync,
	mkdirSync,
	readFileSync,
	renameSync,
	rmSync,
} from "node:fs"
import net from "node:net"
import os from "node:os"
import { join } from "node:path"
import * as Sentry from "@sentry/node"
import { destination } from "pino"
import { backup, removePartialBackups } from "./backup.js"
import {
	bakedImageVersion,
	bakedLauncherSentryDsn,
	bakedMigrationVersion,
	bakedSentryDsns,
} from "./baked.js"
import { parseCommand, type Command } from "./command.js"
import {
	binDir,
	createCluster,
	databaseReadiness,
	databaseServer,
	hbaFile,
	prepareDatabase,
} from "./database.js"
import { loadConfig, type Config } from "./env.js"
import { checkLoopbackExposure } from "./loopback.js"
import { childRecord, createLogger } from "./logging.js"
import {
	authRealtimePort,
	authRealtimeUrl,
	buildChildEnvs,
	caddyPort,
	corePort,
	coreUrl,
	dataDir,
	databasePasswordFile,
	mcpAvailable,
	postgresPort,
	postgresSocketDir,
	secretsDir,
	webPort,
	webUrl,
} from "./mapping.js"
import { client, embeddedConnection, type Connection } from "./psql.js"
import { restore } from "./restore.js"
import { ensureSecrets } from "./secrets.js"
import { createCrashReporter } from "./sentry.js"
import { finishRestore, stagingDir } from "./staging.js"
import {
	createSupervisor,
	type ChildSpec,
	type Supervisor,
} from "./supervisor.js"

// the composition root: the only module that reads the environment or
// installs signal handlers.

// built before anything else runs, because an invalid environment is one of
// the failures worth reporting and loadConfig throws on it — there is no
// parsed Config to read the switch from yet, so the raw variable is read
// here instead.
const crashReporter = createCrashReporter(
	Sentry,
	bakedLauncherSentryDsn,
	bakedImageVersion,
	process.env.OXYNOTE_CRASH_REPORTING_DISABLED === "true",
)

// how long the ordered shutdown may take before the launcher force-exits;
// it stays under the example compose's stop_grace_period so docker never
// has to SIGKILL the whole container.
const shutdownDeadlineMs = 55_000

// one open stdout for the launcher's records and the children's, so they
// reach the stream in write order rather than through two buffers.
const out = destination(1)
const log = createLogger("launcher", out)

// backup writes its archive to stdout, so the commands log to stderr.
const commandLog = createLogger("launcher", destination(2))

// the environment every child and every first-boot step starts from.
const inheritedEnv = { PATH: process.env.PATH ?? "", HOME: "/oxynote" }

// the temporary server a restore loads an embedded dump into. It is not the
// live socket, so a restore never meets a running instance there.
const restoreSocketDir = "/tmp/postgresql-restore"

function logChildLine(name: string, line: string): void {
	out.write(`${childRecord(name, line)}\n`)
}

function sleep(ms: number): Promise<void> {
	return new Promise((resolve) => {
		setTimeout(resolve, ms)
	})
}

async function probe(url: string): Promise<boolean> {
	try {
		const res = await fetch(url, {
			signal: AbortSignal.timeout(2_000),
		})

		await res.body?.cancel()

		return res.status === 200
	} catch {
		return false
	}
}

// the container's own externally reachable IPv4 addresses.
function externalAddresses(): string[] {
	const addresses: string[] = []

	for (const infos of Object.values(os.networkInterfaces())) {
		for (const info of infos ?? []) {
			if (info.family === "IPv4" && !info.internal) {
				addresses.push(info.address)
			}
		}
	}

	return addresses
}

function connects(host: string, port: number): Promise<boolean> {
	return new Promise((resolve) => {
		const socket = net.connect({ host, port, timeout: 1_000 })

		socket.once("connect", () => {
			socket.destroy()
			resolve(true)
		})
		socket.once("error", () => {
			resolve(false)
		})
		socket.once("timeout", () => {
			socket.destroy()
			resolve(false)
		})
	})
}

// runs one of the database's first-boot steps to completion. It blocks,
// which costs nothing: no child is running yet.
function runToCompletion(command: string, args: string[], input: string): void {
	const result = spawnSync(command, args, {
		env: inheritedEnv,
		input,
		encoding: "utf8",
	})

	if (result.error) {
		throw result.error
	}

	if (result.status !== 0) {
		throw new Error(
			`${command} exited with code ${String(result.status)}: ${`${result.stdout}${result.stderr}`.trim()}`,
		)
	}
}

// answers whether a command exits 0, for a readiness check that is a
// program rather than a URL.
function succeeds(
	command: string,
	args: string[],
	env: Record<string, string>,
): Promise<boolean> {
	return new Promise((resolve) => {
		execFile(command, args, { env, timeout: 2_000 }, (err) => {
			resolve(err === null)
		})
	})
}

let shuttingDown = false

async function shutdown(supervisor: Supervisor, code: number): Promise<void> {
	if (shuttingDown) {
		return
	}

	shuttingDown = true

	const deadline = setTimeout(() => {
		log.error("shutdown deadline exceeded")
		process.exit(1)
	}, shutdownDeadlineMs)

	deadline.unref()

	await supervisor.stopAll()
	process.exit(code)
}

function logEnabledFeatures(config: Config): void {
	const state = (enabled: boolean) => (enabled ? "on" : "off")

	log.info(
		`database ${config.databaseDsn === undefined ? "embedded" : "external"}, ` +
			`email ${state(config.smtp !== undefined)}, ` +
			`github app ${state(config.githubApp !== undefined)}, ` +
			`slack app ${state(config.slackApp !== undefined)}, ` +
			`ai assistant ${state((config.aiAssistant.PROVIDER ?? "") !== "")}, ` +
			`change detection ${state(config.changeDetection !== undefined)}`,
	)
}

function externalDatabase(config: Config): Connection | undefined {
	return config.databaseDsn === undefined
		? undefined
		: { args: ["--dbname", config.databaseDsn], env: inheritedEnv }
}

// how the client tools reach the instance's database. The embedded one takes
// the app role's password from the volume.
function databaseConnection(config: Config): Connection {
	return (
		externalDatabase(config) ??
		embeddedConnection(
			postgresSocketDir,
			readFileSync(
				join(dataDir, secretsDir, databasePasswordFile),
				"utf8",
			).trim(),
			inheritedEnv,
		)
	)
}

async function runCommand(
	command: Exclude<Command, { name: "serve" }>,
): Promise<void> {
	const config = loadConfig(process.env)

	if (command.name === "backup") {
		await backup({
			dataDir,
			binDir,
			connection: databaseConnection(config),
			external: config.databaseDsn !== undefined,
			remoteObjects: config.objectStorage !== undefined,
			secretsFromEnv: Object.entries({
				OXYNOTE_AUTH_SECRET: config.authSecret,
				OXYNOTE_DATA_SOURCE_ENCRYPTION_KEYS:
					config.dataSourceEncryptionKeys,
			})
				.filter(([, value]) => value !== undefined)
				.map(([name]) => name),
			imageVersion: bakedImageVersion,
			out: process.stdout,
			outIsTerminal: process.stdout.isTTY,
			log: commandLog,
		})

		return
	}

	await restore({
		dataDir,
		binDir,
		externalDatabase: externalDatabase(config),
		env: inheritedEnv,
		log: commandLog,
		socketDir: restoreSocketDir,
		hbaFile,
		remoteObjects: config.objectStorage !== undefined,
		image: {
			version: bakedImageVersion,
			migrationVersion: bakedMigrationVersion,
		},
		replace: command.replace,
		input: process.stdin,
		inputIsTerminal: process.stdin.isTTY,
		instanceRunning: () => probe(`${coreUrl}/api/x/version`),
		// tini is PID 1. It passes the signal to the launcher,
		// which stops in order and exits, and the runtime starts
		// it again.
		restartInstance: () => {
			process.kill(1, "SIGTERM")
		},
		createCluster: (dir, password) => {
			createCluster({ run: runToCompletion }, dir, password)
		},
	})
}

async function main(): Promise<void> {
	const config = loadConfig(process.env)

	// before the secrets are read, because a committed restore replaces
	// them. A failed load drops the restore, and the old instance serves.
	// Any other failure stops the boot, because a restore still staged
	// would overwrite on a later boot what the old instance writes now.
	const external = externalDatabase(config)

	await finishRestore(
		dataDir,
		external && client(binDir, external),
		log,
	).catch((err: unknown) => {
		if (existsSync(stagingDir(dataDir))) {
			throw err
		}

		log.error(
			`the restore failed, so the old instance stays: ${message(err)}`,
		)
	})
	removePartialBackups(dataDir)

	const { secrets, report } = ensureSecrets(join(dataDir, secretsDir), {
		authSecret: config.authSecret,
		dataSourceEncryptionKeys: config.dataSourceEncryptionKeys,
	})

	for (const name of report.generated) {
		log.info(`generated secret ${name}`)
	}

	if (report.fromVolume.length > 0) {
		log.info(
			`using existing secrets: ${report.fromVolume.join(", ")}`,
		)
	}

	if (report.fromEnv.length > 0) {
		log.info(
			`secrets provided via environment: ${report.fromEnv.join(", ")}`,
		)
	}

	const envs = buildChildEnvs(
		config,
		secrets,
		bakedSentryDsns,
		bakedImageVersion,
		inheritedEnv,
	)
	if (!mcpAvailable(config.publicOrigin)) {
		log.warn(
			`MCP is off: ${config.publicOrigin} is plain http on a non-loopback host. Serve Oxynote over https to use MCP.`,
		)
	}

	const embeddedDatabase = config.databaseDsn === undefined

	if (embeddedDatabase) {
		const created = prepareDatabase(
			{
				exists: existsSync,
				remove: (path) => {
					rmSync(path, {
						recursive: true,
						force: true,
					})
				},
				rename: renameSync,
				makeDir: (path) => {
					mkdirSync(path, {
						recursive: true,
						mode: 0o700,
					})
				},
				chmod: chmodSync,
				run: runToCompletion,
			},
			secrets.databasePassword,
		)

		if (created) {
			log.info("created the embedded database")
		}
	}

	const supervisor = createSupervisor({
		spawn: (command, args, env) =>
			spawn(command, args, {
				env,
				stdio: ["ignore", "pipe", "pipe"],
			}),
		sleep,
		log,
		logChildLine,
		onUnexpectedExit(name, code) {
			const message = `${name} exited unexpectedly with code ${code}`

			log.error(message)
			// the child is gone, so whatever it would have reported
			// for itself is lost — an OOM kill leaves no other
			// trace at all. Reporting is awaited before the
			// shutdown it precedes, which then exits the process.
			void crashReporter
				.report(new Error(message))
				.then(() =>
					shutdown(
						supervisor,
						code === 0 ? 1 : code,
					),
				)
		},
	})

	process.once("SIGTERM", () => void shutdown(supervisor, 0))
	process.once("SIGINT", () => void shutdown(supervisor, 0))

	const specs: ChildSpec[] = [
		// first to start and so last to stop, once every client is gone.
		...(embeddedDatabase
			? [
					{
						name: "postgres",
						command: databaseServer.command,
						args: databaseServer.args,
						env: inheritedEnv,
						ready: () =>
							succeeds(
								databaseReadiness.command,
								databaseReadiness.args,
								inheritedEnv,
							),
						readyTimeoutMs: 60_000,
						// the shutdown checkpoint writes out every
						// buffer still in memory.
						stopGraceMs: 20_000,
					},
				]
			: []),
		{
			name: "core",
			command: "/oxynote/core/server",
			args: [],
			env: envs.core,
			ready: () => probe(`${coreUrl}/api/x/version`),
			// the first boot runs the migrations and creates the
			// storage bucket before listening.
			readyTimeoutMs: 180_000,
			stopGraceMs: 10_000,
		},
		{
			name: "auth-realtime",
			command: process.execPath,
			// one esbuild bundle with sentry folded in; a separate
			// --import preload would initialize a second sentry copy
			// the bundled app never sees.
			args: ["/oxynote/auth-realtime/index.mjs"],
			env: envs.authRealtime,
			ready: () =>
				probe(`${authRealtimeUrl}/api/auth-config`),
			readyTimeoutMs: 60_000,
			// the shutdown flush may persist every open document.
			stopGraceMs: 20_000,
		},
		{
			name: "web",
			command: process.execPath,
			args: ["/oxynote/web/server/index.mjs"],
			env: envs.web,
			ready: () => probe(`${webUrl}/login`),
			readyTimeoutMs: 60_000,
			stopGraceMs: 10_000,
			// nitro announces its loopback bind with a bare
			// console.log and offers no way to silence it.
			mute: /^Listening on /,
		},
		{
			name: "caddy",
			command: "/usr/local/bin/caddy",
			args: [
				"run",
				"--config",
				"/oxynote/prod/Caddyfile",
				"--adapter",
				"caddyfile",
			],
			env: envs.caddy,
			ready: () =>
				probe(
					`http://127.0.0.1:${caddyPort}/auth-realtime/api/auth-config`,
				),
			readyTimeoutMs: 30_000,
			stopGraceMs: 10_000,
		},
	]

	try {
		for (const spec of specs) {
			await supervisor.start(spec)
		}

		// the loopback gate: the internal, unauthenticated surfaces
		// must be unreachable from the network. A regression that makes
		// a service bind all interfaces fails the boot instead of
		// serving.
		const exposed = await checkLoopbackExposure(
			{ addresses: externalAddresses, connects },
			[corePort, authRealtimePort, webPort, postgresPort],
		)

		if (exposed.length > 0) {
			throw new Error(
				`internal service ports are reachable from the network (${exposed.join(", ")}) — refusing to serve`,
			)
		}
	} catch (err) {
		log.error(message(err))
		await crashReporter.report(err)
		await supervisor.stopAll()
		process.exit(1)
	}

	log.info(`oxynote is up at ${config.publicOrigin}`)
	logEnabledFeatures(config)
}

function message(err: unknown): string {
	return err instanceof Error ? err.message : String(err)
}

let command: Command

try {
	command = parseCommand(process.argv.slice(2))
} catch (err) {
	process.stderr.write(`${message(err)}\n`)
	process.exit(2)
}

if (command.name === "serve") {
	main().catch((err: unknown) => {
		log.error(message(err))
		void crashReporter.report(err).then(() => {
			process.exit(1)
		})
	})
} else {
	runCommand(command).catch((err: unknown) => {
		commandLog.error(message(err))
		process.exit(1)
	})
}
