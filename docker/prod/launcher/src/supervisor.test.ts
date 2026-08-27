import { EventEmitter } from "node:events"
import type { ChildProcess } from "node:child_process"
import { PassThrough } from "node:stream"
import { describe, it, vi } from "vitest"
import { createSupervisor, type ChildSpec } from "./supervisor.js"

interface StubChild {
	process: ChildProcess
	emitter: EventEmitter
	stdout: PassThrough
	stderr: PassThrough
	kill: ReturnType<typeof vi.fn>
	exit(code: number | null, signal?: string): void
}

// a ChildProcess stand-in exposing exactly what the supervisor touches:
// the two output streams, kill, and the exit event.
function stubChild(options: { exitOnTerm?: boolean } = {}): StubChild {
	const emitter = new EventEmitter()
	const stdout = new PassThrough()
	const stderr = new PassThrough()

	const exit = (code: number | null, signal?: string) => {
		emitter.emit("exit", code, signal ?? null)
	}

	const kill = vi.fn((signal: string) => {
		if (signal === "SIGKILL") {
			exit(null, "SIGKILL")

			return true
		}

		if (options.exitOnTerm !== false) {
			exit(0)
		}

		return true
	})

	const child = {
		stdout,
		stderr,
		kill,
		once: emitter.once.bind(emitter),
	} as unknown as ChildProcess

	return { process: child, emitter, stdout, stderr, kill, exit }
}

function spec(name: string, overrides: Partial<ChildSpec> = {}): ChildSpec {
	return {
		name,
		command: `/bin/${name}`,
		args: ["--run"],
		env: { KEY: name },
		readyUrl: `http://127.0.0.1:1/${name}`,
		readyTimeoutMs: 1_000,
		stopGraceMs: 100,
		...overrides,
	}
}

function harness(childOptions: { exitOnTerm?: boolean } = {}) {
	const children: StubChild[] = []

	const spawn = vi.fn(() => {
		const child = stubChild(childOptions)

		children.push(child)

		return child.process
	})
	const probe = vi
		.fn<(url: string) => Promise<boolean>>()
		.mockResolvedValue(true)
	const log = vi.fn<(line: string) => void>()
	const onUnexpectedExit = vi.fn<(name: string, code: number) => void>()

	const supervisor = createSupervisor({
		spawn,
		probe,
		sleep: () => Promise.resolve(),
		log,
		onUnexpectedExit,
	})

	return { children, spawn, probe, log, onUnexpectedExit, supervisor }
}

describe("createSupervisor", () => {
	describe("start", () => {
		it("spawns the child with its own command and environment", async ({
			expect,
		}) => {
			const h = harness()

			await h.supervisor.start(spec("core"))

			expect(h.spawn).toHaveBeenCalledWith(
				"/bin/core",
				["--run"],
				{ KEY: "core" },
			)
		})

		it("waits for the readiness probe before resolving", async ({
			expect,
		}) => {
			const h = harness()
			h.probe
				.mockResolvedValueOnce(false)
				.mockResolvedValueOnce(false)
				.mockResolvedValue(true)

			await h.supervisor.start(
				spec("core", { readyTimeoutMs: 5_000 }),
			)

			expect(h.probe).toHaveBeenCalledTimes(3)
			expect(h.probe).toHaveBeenCalledWith(
				"http://127.0.0.1:1/core",
			)
			expect(h.log).toHaveBeenCalledWith(
				"[launcher] core is ready",
			)
		})

		it("gives up when the child never becomes ready", async ({
			expect,
		}) => {
			const h = harness()
			h.probe.mockResolvedValue(false)

			await expect(
				h.supervisor.start(
					spec("core", { readyTimeoutMs: 1_000 }),
				),
			).rejects.toThrow(
				"core did not become ready within 1000ms",
			)
			expect(h.onUnexpectedExit).toHaveBeenCalledTimes(0)
		})

		it("fails fast when the child exits before becoming ready", async ({
			expect,
		}) => {
			const h = harness()
			h.probe.mockImplementation(() => {
				h.children[0]?.exit(3)

				return Promise.resolve(false)
			})

			await expect(
				h.supervisor.start(spec("core")),
			).rejects.toThrow("core exited before becoming ready")
			expect(h.onUnexpectedExit).toHaveBeenCalledWith(
				"core",
				3,
			)
		})

		it("tags the child's output with its name", async ({
			expect,
		}) => {
			const h = harness()

			await h.supervisor.start(spec("core"))

			h.children[0]?.stdout.emit("data", "hello\n")
			h.children[0]?.stderr.emit("data", "oops\n")

			expect(h.log).toHaveBeenCalledWith("[core] hello")
			expect(h.log).toHaveBeenCalledWith("[core] oops")
		})

		it("reports an exit after readiness as unexpected", async ({
			expect,
		}) => {
			const h = harness()

			await h.supervisor.start(spec("core"))

			h.children[0]?.exit(2)
			await Promise.resolve()

			expect(h.onUnexpectedExit).toHaveBeenCalledWith(
				"core",
				2,
			)
		})

		it("normalizes a signal death into a failure code", async ({
			expect,
		}) => {
			const h = harness()

			await h.supervisor.start(spec("core"))

			h.children[0]?.exit(null, "SIGSEGV")
			await Promise.resolve()

			expect(h.onUnexpectedExit).toHaveBeenCalledWith(
				"core",
				1,
			)
		})
	})

	describe("stopAll", () => {
		it("terminates the children in reverse start order", async ({
			expect,
		}) => {
			const h = harness()

			await h.supervisor.start(spec("core"))
			await h.supervisor.start(spec("caddy"))

			await h.supervisor.stopAll()

			const stops = h.log.mock.calls
				.map(([line]) => line)
				.filter((line) => line.includes("stopping"))

			expect(stops).toEqual([
				"[launcher] stopping caddy",
				"[launcher] stopping core",
			])
			expect(h.children[0]?.kill).toHaveBeenCalledWith(
				"SIGTERM",
			)
			expect(h.children[1]?.kill).toHaveBeenCalledWith(
				"SIGTERM",
			)
		})

		it("does not report shutdown exits as unexpected", async ({
			expect,
		}) => {
			const h = harness()

			await h.supervisor.start(spec("core"))
			await h.supervisor.stopAll()
			await Promise.resolve()

			expect(h.onUnexpectedExit).toHaveBeenCalledTimes(0)
		})

		it("kills a child that outlives its stop grace", async ({
			expect,
		}) => {
			const h = harness({ exitOnTerm: false })

			await h.supervisor.start(spec("core"))
			await h.supervisor.stopAll()

			expect(h.children[0]?.kill).toHaveBeenCalledWith(
				"SIGTERM",
			)
			expect(h.children[0]?.kill).toHaveBeenCalledWith(
				"SIGKILL",
			)
		})

		it("skips a child that already exited", async ({ expect }) => {
			const h = harness()

			await h.supervisor.start(spec("core"))

			h.children[0]?.exit(1)
			await Promise.resolve()

			await h.supervisor.stopAll()

			expect(h.children[0]?.kill).toHaveBeenCalledTimes(0)
		})
	})
})
