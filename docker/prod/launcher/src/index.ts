import { spawn } from "node:child_process"
import net from "node:net"
import os from "node:os"
import { join } from "node:path"
import * as Sentry from "@sentry/node"
import { destination } from "pino"
import { bakedLauncherSentryDsn, bakedSentryDsns } from "./baked.js"
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
	webPort,
	webUrl,
} from "./mapping.js"
import { ensureSecrets } from "./secrets.js"
import { createCrashReporter } from "./sentry.js"
import {
	createSupervisor,
	type ChildSpec,
	type Supervisor,
} from "./supervisor.js"

// the composition root: the only module that reads the environment, touches
// the filesystem, spawns processes, or installs signal handlers.

// built before anything else runs, because an invalid environment is one of
// the failures worth reporting and loadConfig throws on it — there is no
// parsed Config to read the switch from yet, so the raw variable is read
// here instead.
const crashReporter = createCrashReporter(
	Sentry,
	bakedLauncherSentryDsn,
	process.env.OXYNOTE_CRASH_REPORTING_DISABLED === "true",
)

// how long the ordered shutdown may take before the launcher force-exits;
// it stays under the example compose's stop_grace_period so docker never
// has to SIGKILL the whole container.
const shutdownDeadlineMs = 55_000

// one open stdout for the launcher's own logger and for the children's
// relayed lines, so every record reaches the stream in write order rather
// than through two buffers.
const out = destination(1)
const log = createLogger("launcher", out)

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
		`search ${state(config.meilisearch !== undefined)}, ` +
			`email ${state(config.smtp !== undefined)}, ` +
			`github app ${state(config.githubApp !== undefined)}, ` +
			`slack app ${state(config.slackApp !== undefined)}, ` +
			`ai assistant ${state((config.aiAssistant.PROVIDER ?? "") !== "")}, ` +
			`change detection ${state(config.changeDetection !== undefined)}`,
	)
}

async function main(): Promise<void> {
	const config = loadConfig(process.env)

	const { secrets, report } = ensureSecrets(join(dataDir, "secrets"), {
		authSecret: config.authSecret,
		dataSourceEncryptionKey: config.dataSourceEncryptionKey,
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

	const envs = buildChildEnvs(config, secrets, bakedSentryDsns, {
		PATH: process.env.PATH ?? "",
		HOME: "/oxynote",
	})

	const supervisor = createSupervisor({
		spawn: (command, args, env) =>
			spawn(command, args, {
				env,
				stdio: ["ignore", "pipe", "pipe"],
			}),
		probe,
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
		{
			name: "core",
			command: "/oxynote/core/server",
			args: [],
			env: envs.core,
			readyUrl: `${coreUrl}/api/x/version`,
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
			readyUrl: `${authRealtimeUrl}/api/auth-config`,
			readyTimeoutMs: 60_000,
			// the shutdown flush may persist every open document.
			stopGraceMs: 20_000,
		},
		{
			name: "web",
			command: process.execPath,
			args: ["/oxynote/web/server/index.mjs"],
			env: envs.web,
			readyUrl: `${webUrl}/login`,
			readyTimeoutMs: 60_000,
			stopGraceMs: 10_000,
			// nitro's node-server entry announces the address it
			// bound with a bare console.log and offers no way to
			// silence it. That address is this container's
			// loopback, not the one anyone reaches the app on, so
			// it is dropped here and the launcher's own "up at"
			// line — the public URL — is the one left standing.
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
			readyUrl: `http://127.0.0.1:${caddyPort}/auth-realtime/api/auth-config`,
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
			[corePort, authRealtimePort, webPort],
		)

		if (exposed.length > 0) {
			throw new Error(
				`internal service ports are reachable from the network (${exposed.join(", ")}) — refusing to serve`,
			)
		}
	} catch (err) {
		log.error(err instanceof Error ? err.message : String(err))
		await crashReporter.report(err)
		await supervisor.stopAll()
		process.exit(1)
	}

	log.info(`oxynote is up at ${config.publicOrigin}`)
	logEnabledFeatures(config)
}

main().catch((err: unknown) => {
	log.error(err instanceof Error ? err.message : String(err))
	void crashReporter.report(err).then(() => {
		process.exit(1)
	})
})
