import { spawn } from "node:child_process"
import { basename } from "node:path"
import type { Readable, Writable } from "node:stream"
import { pipeline } from "node:stream/promises"

export interface RunOptions {
	env: Record<string, string>
	// fed to the command's standard input, which is closed at once
	// otherwise.
	input?: Readable
	// receives the command's standard output. Without it, run collects
	// the output and returns it.
	output?: Writable
}

// run executes a command to completion and throws when it exits non-zero,
// with the command's own error output in the message.
export async function run(
	command: string,
	args: string[],
	options: RunOptions,
): Promise<string> {
	const child = spawn(command, args, {
		env: options.env,
		stdio: "pipe",
	})

	let stdout = ""
	let stderr = ""

	child.stderr.setEncoding("utf8")
	child.stderr.on("data", (chunk: string) => {
		stderr += chunk
	})

	const streams: Promise<void>[] = []

	if (options.input) {
		streams.push(pipeline(options.input, child.stdin))
	} else {
		child.stdin.end()
	}

	if (options.output) {
		streams.push(pipeline(child.stdout, options.output))
	} else {
		child.stdout.setEncoding("utf8")
		child.stdout.on("data", (chunk: string) => {
			stdout += chunk
		})
	}

	// a command that fails early closes its input, and the broken pipe
	// that follows says less than the command's own error. So the streams
	// are settled at once, and checked only after the exit code.
	const settled = Promise.allSettled(streams)

	const code = await new Promise<number | null>((resolve, reject) => {
		child.once("error", reject)
		child.once("close", resolve)
	})

	const results = await settled

	if (code !== 0) {
		throw new Error(
			`${basename(command)} exited with code ${String(code)}: ${stderr.trim()}`,
		)
	}

	for (const result of results) {
		if (result.status === "rejected") {
			throw result.reason
		}
	}

	return stdout
}
