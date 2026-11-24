import { Readable, Writable } from "node:stream"
import { describe, it } from "vitest"
import { run } from "./run.js"

const env = { PATH: process.env.PATH ?? "" }

describe("run", () => {
	it("returns what the command printed", async ({ expect }) => {
		await expect(
			run("/bin/sh", ["-c", "echo hi"], { env }),
		).resolves.toBe("hi\n")
	})

	it("feeds the input and streams the output", async ({ expect }) => {
		const chunks: Buffer[] = []
		const output = new Writable({
			write(chunk: Buffer, _encoding, done) {
				chunks.push(chunk)
				done()
			},
		})

		await run("/bin/cat", [], {
			env,
			input: Readable.from(["a", "b"]),
			output,
		})

		expect(Buffer.concat(chunks).toString()).toBe("ab")
	})

	it("throws the command's own error on a failure", async ({
		expect,
	}) => {
		await expect(
			run("/bin/sh", ["-c", "echo broken >&2; exit 3"], {
				env,
			}),
		).rejects.toThrow("sh exited with code 3: broken")
	})

	// the command exits before it reads its input, which breaks the pipe
	it("prefers the exit error over a broken input pipe", async ({
		expect,
	}) => {
		const input = Readable.from(
			(function* () {
				for (let i = 0; i < 1000; i++) {
					yield Buffer.alloc(65536)
				}
			})(),
		)

		await expect(
			run("/bin/sh", ["-c", "exit 4"], { env, input }),
		).rejects.toThrow("sh exited with code 4")
	})

	it("throws when the command cannot start", async ({ expect }) => {
		await expect(run("/nonexistent", [], { env })).rejects.toThrow(
			"ENOENT",
		)
	})

	it("throws a failed output stream", async ({ expect }) => {
		const output = new Writable({
			write(_chunk, _encoding, done) {
				done(new Error("disk full"))
			},
		})

		await expect(
			run("/bin/sh", ["-c", "echo hi"], { env, output }),
		).rejects.toThrow("disk full")
	})
})
