import { spawn } from "node:child_process"
import { fileURLToPath } from "node:url"
import { BASE_URL, MAILPIT_URL } from "./helpers/config"

const READY_TIMEOUT = 180_000
const POLL_INTERVAL = 2_000
const TICK_INTERVAL = 1_000

const E2E_DIR = fileURLToPath(new URL(".", import.meta.url))

// bring the stack up and wait until every service the tests touch answers
// through the front door. `up --wait` only covers the containers that
// declare a healthcheck, and core cannot declare one — its image is
// distroless, with no shell or http client to probe itself with.
export default async function globalSetup(): Promise<void> {
	await progress("starting the stack", () => compose(["up", "-d", "--wait"]))

	await progress("waiting for services", () =>
		Promise.all([
			waitForService("web", `${BASE_URL}/login`),
			waitForService("core", `${BASE_URL}/core/api/github`),
			waitForService(
				"auth-realtime",
				`${BASE_URL}/auth-realtime/api/auth-config`,
			),
			waitForService("mailpit", `${MAILPIT_URL}/api/v1/info`),
		]),
	)
}

// progress runs a step under a ticking line, so a terminal waiting on
// docker is never mistaken for a hung one.
async function progress(
	label: string,
	step: () => Promise<unknown>,
): Promise<void> {
	process.stdout.write(`  ${label}`)

	const ticker = setInterval(() => process.stdout.write("."), TICK_INTERVAL)

	try {
		await step()
	} catch (err) {
		process.stdout.write(" failed\n\n")
		throw err
	} finally {
		clearInterval(ticker)
	}

	process.stdout.write(" ok\n")
}

// compose swallows docker's output and replays it only on failure: the
// container-by-container churn says nothing while it is working, and
// everything once it is not.
function compose(args: string[]): Promise<void> {
	const child = spawn("docker", ["compose", ...args], {
		cwd: E2E_DIR,
		stdio: ["ignore", "pipe", "pipe"],
	})

	// decoded as text rather than concatenated as buffers, so a multi-byte
	// character split across two chunks survives the join.
	child.stdout.setEncoding("utf8")
	child.stderr.setEncoding("utf8")

	let output = ""

	const collect = (chunk: string) => {
		output += chunk
	}

	child.stdout.on("data", collect)
	child.stderr.on("data", collect)

	return new Promise((resolve, reject) => {
		child.on("error", reject)
		child.on("close", (code) => {
			if (code === 0) {
				resolve()

				return
			}

			reject(new Error(`docker compose ${args.join(" ")} failed\n\n${output}`))
		})
	})
}

async function waitForService(name: string, url: string): Promise<void> {
	const deadline = Date.now() + READY_TIMEOUT

	for (;;) {
		try {
			const response = await fetch(url)

			// any answer from the service itself means it is serving —
			// core replies 401 to an unauthenticated probe. Caddy answers
			// 502 on its behalf while it is still starting up.
			if (response.status < 500) {
				return
			}
		} catch {
			// connection refused while the stack is still coming up
		}

		if (Date.now() > deadline) {
			throw new Error(`${name} never became ready at ${url}`)
		}

		await new Promise((resolve) => setTimeout(resolve, POLL_INTERVAL))
	}
}
