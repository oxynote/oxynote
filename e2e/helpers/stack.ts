import { spawn } from "node:child_process"
import { fileURLToPath } from "node:url"
import { STACK } from "./config"

const E2E_DIR = fileURLToPath(new URL("..", import.meta.url))
const ROOT_DIR = fileURLToPath(new URL("../..", import.meta.url))

// the two stacks this suite can drive, each with the make targets that
// build and drop it and the compose file that brings it up. The dev stack
// builds four images from this checkout; the prod one builds the
// all-in-one image and pulls the rest, so its compose file has no build
// section at all.
const STACKS = {
	dev: {
		composeFile: "docker-compose.dev.yaml",
		buildTarget: "e2e-dev-stack-build",
		stopTarget: "e2e-dev-stack-stop",
	},
	prod: {
		composeFile: "docker-compose.prod.yaml",
		buildTarget: "e2e-prod-stack-build",
		stopTarget: "e2e-prod-stack-stop",
	},
} as const

const stack = STACKS[STACK]

// the stack is driven through the root Makefile rather than through
// docker compose directly, so the build stays defined in one place: it is
// two steps, and only one of them is compose's. Core's image is produced
// by goreleaser, which compose cannot do. Running make also keeps
// E2E_DEV_COMPOSE_EXTRA and PROD_BUILD_EXTRA working — CI sets them to
// layer its buildx cache over the local build.
//
// Every make target here already runs its steps through
// scripts/run-quietly.sh, which ticks a line per step and replays the log
// only on failure. So they inherit this process's streams: capturing them
// would hide the per-step labels and replace two of them ("building go
// images", "building stack images") with one anonymous row of dots.
export function buildStack(): Promise<void> {
	return run("make", [stack.buildTarget], ROOT_DIR, "inherit")
}

export function stopStack(): Promise<void> {
	return run("make", [stack.stopTarget], ROOT_DIR, "inherit")
}

// compose is the exception: it is called directly, with nothing in front
// of it to quieten the container-by-container churn, so its output is
// held back and replayed only if the step fails.
export function startStack(): Promise<void> {
	return run(
		"docker",
		[...composeArgs(), "up", "-d", "--wait"],
		E2E_DIR,
		"capture",
	)
}

// composeArgs names the file every compose call has to be pointed at.
// Neither file carries compose's default name, so an unpointed call would
// find nothing — and if one of them ever did, it would silently drive the
// wrong project.
export function composeArgs(): string[] {
	return ["compose", "-f", stack.composeFile]
}

// composeCwd is the directory compose is invoked from — the compose file
// paths are relative to it.
export const composeCwd = E2E_DIR

// a step that did not finish. Playwright prints an error's stack, and a
// JS trace through this file says nothing about a docker build that broke
// or a run someone stopped on purpose — the message is the whole story,
// so it stands in for the trace.
class StepError extends Error {
	constructor(message: string) {
		super(message)
		this.name = "StepError"
		this.stack = message
	}
}

function run(
	command: string,
	args: string[],
	cwd: string,
	output: "inherit" | "capture",
): Promise<void> {
	const child = spawn(command, args, {
		cwd,
		stdio: output === "inherit" ? "inherit" : ["ignore", "pipe", "pipe"],
	})

	let captured = ""

	if (child.stdout && child.stderr) {
		// decoded as text rather than concatenated as buffers, so a
		// multi-byte character split across two chunks survives the join.
		child.stdout.setEncoding("utf8")
		child.stderr.setEncoding("utf8")

		const collect = (chunk: string) => {
			captured += chunk
		}

		child.stdout.on("data", collect)
		child.stderr.on("data", collect)
	}

	return new Promise((resolve, reject) => {
		child.on("error", reject)
		child.on("close", (code, signal) => {
			if (code === 0) {
				resolve()

				return
			}

			const step = `${command} ${args.join(" ")}`

			// ctrl-c goes to the whole foreground process group, so the
			// child is already gone by the time this runs — stopped on
			// purpose, not broken. It shows up either as the signal
			// itself or, where a shell script traps it, as the 128+n
			// status that script exited with.
			if (interrupted(code, signal)) {
				reject(new StepError(`${step} was interrupted`))

				return
			}

			// an inherited command has already printed whatever it had
			// to say, so only the captured form replays a log here.
			const reason = signal ? `on ${signal}` : `with code ${String(code)}`

			reject(
				new StepError(
					`${step} exited ${reason}${captured ? `\n\n${captured}` : ""}`,
				),
			)
		})
	})
}

function interrupted(code: number | null, signal: string | null): boolean {
	return (
		signal === "SIGINT" || signal === "SIGTERM" || code === 130 || code === 143
	)
}
