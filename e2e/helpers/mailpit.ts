import { expect, type APIRequestContext } from "@playwright/test"
import { MAILPIT_URL } from "./config"

// delivery is asynchronous on both sides — core hands the message to its
// supervisor and mailpit accepts it over SMTP — so the inbox is polled.
const DELIVERY_TIMEOUT = 30_000

const VERIFICATION_LINK = /href="([^"]*\/api\/auth\/verify-email[^"]*)"/

// fetchVerificationLink waits for the account-activation email and hands
// back the link it carries.
//
// The inbox is searched for the message carrying an activation link
// rather than the newest one being taken: an address can receive more
// than one message and nothing guarantees the order they land in. Every
// test signs up a freshly generated address, so a search only ever sees
// that test's own mail.
export async function fetchVerificationLink(
	request: APIRequestContext,
	address: string,
): Promise<string> {
	const found = { link: "" }

	await expect
		.poll(
			async () => {
				found.link = await findVerificationLink(request, address)

				return found.link
			},
			{
				message: `no activation email was delivered to ${address}`,
				timeout: DELIVERY_TIMEOUT,
			},
		)
		.not.toBe("")

	return found.link
}

async function findVerificationLink(
	request: APIRequestContext,
	address: string,
): Promise<string> {
	const search = await request.get(`${MAILPIT_URL}/api/v1/search`, {
		params: { query: `to:${address}` },
	})
	expect(search.ok()).toBe(true)

	const { messages } = (await search.json()) as { messages: { ID: string }[] }

	for (const message of messages) {
		const response = await request.get(
			`${MAILPIT_URL}/api/v1/message/${message.ID}`,
		)
		expect(response.ok()).toBe(true)

		const { HTML } = (await response.json()) as { HTML: string }
		const match = VERIFICATION_LINK.exec(HTML)

		if (match?.[1]) {
			// an href holds HTML, where a literal "&" is written "&amp;".
			// A mail client decodes it on the way to the address bar;
			// pulling the attribute straight out of the source skips that
			// parser, so the entity has to be resolved here instead.
			return match[1].replaceAll("&amp;", "&")
		}
	}

	return ""
}
