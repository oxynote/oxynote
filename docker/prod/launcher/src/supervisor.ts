import type { ChildProcess } from "node:child_process"
import { createLinePrefixer } from "./logging.js"

export interface ChildSpec {
	name: string
	command: string
	args: string[]
	env: Record<string, string>
	// the URL polled until it answers 200; the child counts as started
	// only after that, which is what serializes core's migrations before
	// auth-realtime and keeps caddy's published port silent until the
	// whole stack answers.
	readyUrl: string
	readyTimeoutMs: number
	// how long a SIGTERM'd child may drain before SIGKILL.
	stopGraceMs: number
}

export interface SupervisorDeps {
	spawn(
		command: string,
		args: string[],
		env: Record<string, string>,
	): ChildProcess
	// answers whether a GET to the URL returned 200.
	probe(url: string): Promise<boolean>
	sleep(ms: number): Promise<void>
	log(line: string): void
	// called when a child exits outside an initiated shutdown.
	onUnexpectedExit(name: string, code: number): void
}

export interface Supervisor {
	start(spec: ChildSpec): Promise<void>
	stopAll(): Promise<void>
}

interface Child {
	spec: ChildSpec
	process: ChildProcess
	exited: Promise<number>
	running: boolean
}

const readyPollIntervalMs = 500

// createSupervisor owns the child processes: it starts them one at a time,
// gating each on readiness, tags their output, converts any unexpected exit
// into a callback — the policy is crash-the-container, restarts belong to
// the container runtime — and stops them in reverse order on shutdown.
export function createSupervisor(deps: SupervisorDeps): Supervisor {
	const children: Child[] = []
	let stopping = false

	function attachOutput(name: string, child: ChildProcess): void {
		for (const stream of [child.stdout, child.stderr]) {
			if (!stream) {
				continue
			}

			const prefixer = createLinePrefixer(
				`[${name}]`,
				(line) => {
					deps.log(line)
				},
			)

			stream.on("data", (chunk: Buffer) => {
				prefixer.data(chunk)
			})
			stream.on("close", () => {
				prefixer.end()
			})
		}
	}

	async function waitReady(child: Child): Promise<void> {
		const attempts = Math.max(
			1,
			Math.ceil(
				child.spec.readyTimeoutMs / readyPollIntervalMs,
			),
		)

		for (let i = 0; i < attempts; i++) {
			if (!child.running) {
				throw new Error(
					`${child.spec.name} exited before becoming ready`,
				)
			}

			if (await deps.probe(child.spec.readyUrl)) {
				return
			}

			await deps.sleep(readyPollIntervalMs)
		}

		throw new Error(
			`${child.spec.name} did not become ready within ${child.spec.readyTimeoutMs}ms`,
		)
	}

	async function start(spec: ChildSpec): Promise<void> {
		deps.log(`[launcher] starting ${spec.name}`)

		const process = deps.spawn(spec.command, spec.args, spec.env)

		attachOutput(spec.name, process)

		const exited = new Promise<number>((resolve) => {
			process.once("exit", (code, signal) => {
				resolve(code ?? (signal === null ? 0 : 1))
			})
		})

		const child: Child = { spec, process, exited, running: true }

		children.push(child)

		void exited.then((code) => {
			child.running = false

			if (!stopping) {
				deps.onUnexpectedExit(spec.name, code)
			}
		})

		await waitReady(child)

		deps.log(`[launcher] ${spec.name} is ready`)
	}

	async function stopAll(): Promise<void> {
		stopping = true

		for (const child of [...children].reverse()) {
			if (!child.running) {
				continue
			}

			deps.log(`[launcher] stopping ${child.spec.name}`)
			child.process.kill("SIGTERM")

			const graceful = await Promise.race([
				child.exited.then(() => true),
				deps
					.sleep(child.spec.stopGraceMs)
					.then(() => false),
			])

			if (graceful) {
				continue
			}

			deps.log(
				`[launcher] ${child.spec.name} exceeded its stop grace, killing`,
			)
			child.process.kill("SIGKILL")

			await child.exited
		}
	}

	return { start, stopAll }
}
