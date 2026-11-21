import { expect, test, type APIRequestContext } from "@playwright/test"
import {
	failedSignIn,
	RATE_LIMITED_URL,
	TRUSTED_PROXIES_URL,
} from "../helpers/rate-limit"

// better-auth allows three sign-ins per address in ten seconds, and keeps
// the count in memory. Each test uses its own client addresses, so the
// tests never share a count. A retry inside the window would start from
// the count its first attempt left, so there is none.
//
// the client addresses come from the documentation ranges (RFC 5737),
// which fall outside every range the instance trusts.
const CLIENT_A = "203.0.113.4"
const CLIENT_B = "203.0.113.5"

async function failedSignIns(
	request: APIRequestContext,
	origin: string,
	forwardedFor: string[],
): Promise<number[]> {
	const statuses: number[] = []

	for (const value of forwardedFor) {
		statuses.push(await failedSignIn(request, origin, value))
	}

	return statuses
}

test.describe("rate limits behind a reverse proxy", () => {
	test.describe.configure({ retries: 0 })

	test("limits each client on its own when the proxy is trusted", async ({
		request,
	}) => {
		const statuses = await failedSignIns(request, TRUSTED_PROXIES_URL, [
			CLIENT_A,
			CLIENT_A,
			CLIENT_A,
			CLIENT_B,
		])

		expect(statuses).toEqual([401, 401, 401, 401])
	})

	test("still limits a single client when the proxy is trusted", async ({
		request,
	}) => {
		const client = "203.0.113.6"
		const statuses = await failedSignIns(request, TRUSTED_PROXIES_URL, [
			client,
			client,
			client,
			client,
		])

		expect(statuses).toEqual([401, 401, 401, 429])
	})

	test("ignores the addresses a client adds before the proxy's own", async ({
		request,
	}) => {
		// the client sends a new made-up address every time, and the proxy
		// appends the address it saw. Only the last one counts.
		const client = "203.0.113.7"
		const statuses = await failedSignIns(request, TRUSTED_PROXIES_URL, [
			`198.51.100.1, ${client}`,
			`198.51.100.2, ${client}`,
			`198.51.100.3, ${client}`,
			`198.51.100.4, ${client}`,
		])

		expect(statuses).toEqual([401, 401, 401, 429])
	})

	test("puts every client in one bucket when no proxy is trusted", async ({
		request,
	}) => {
		const statuses = await failedSignIns(request, RATE_LIMITED_URL, [
			CLIENT_A,
			CLIENT_A,
			CLIENT_A,
			CLIENT_B,
		])

		expect(statuses).toEqual([401, 401, 401, 429])
	})
})
