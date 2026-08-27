import { BASE_URL, MAILPIT_URL } from "./helpers/config"
import { buildStack, startStack } from "./helpers/stack"

const READY_TIMEOUT = 180_000
const POLL_INTERVAL = 2_000
const TICK_INTERVAL = 1_000

// build the stack from this checkout, bring it up, and wait until every
// service the tests touch answers through the front door. `up --wait` only
// covers the containers that declare a healthcheck, and core cannot
// declare one — its image is distroless, with no shell or http client to
// probe itself with.
//
// The build runs here rather than in the caller so that every way of
// starting the suite behaves the same: the make targets, the pnpm scripts
// and the play button in the Playwright VS Code extension all test the code
// that is in the tree right now, not whatever image was last left behind.
// Which stack gets built and started is the playwright config's choice, and
// this hook is shared by both.
export default async function globalSetup(): Promise<void> {
	// make prints its own labelled step per image, so it is not wrapped
	// in a progress line of this file's own.
	await buildStack()

	await progress("starting the stack", startStack)

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
