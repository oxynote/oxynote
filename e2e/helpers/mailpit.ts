import { expect, type APIRequestContext } from "@playwright/test"
import { MAILPIT_URL } from "./config"

// delivery is asynchronous on both sides — core hands the message to its
// supervisor and mailpit accepts it over SMTP — so the inbox is polled.
const DELIVERY_TIMEOUT = 30_000

const VERIFICATION_LINK = /href="([^"]*\/api\/auth\/verify-email[^"]*)"/
const INVITE_LINK = /href="([^"]*\/accept-invite[^"]*)"/
const PASSWORD_RESET_LINK = /href="([^"]*\/api\/auth\/reset-password[^"]*)"/
// the account-exists notice links to the bare login page, which is what
// separates it from the verification email in the same inbox — that
// one's link only carries "/login" inside its callback parameter.
const ACCOUNT_EXISTS_LINK = /href="([^"]*\/login)"/

// fetchVerificationLink waits for the account-activation email and hands
// back the link it carries.
export function fetchVerificationLink(
	request: APIRequestContext,
	address: string,
): Promise<string> {
	return fetchLink(request, address, VERIFICATION_LINK, "activation")
}

// fetchInviteLink waits for a workspace invitation email and hands back
// its accept link.
export function fetchInviteLink(
	request: APIRequestContext,
	address: string,
): Promise<string> {
	return fetchLink(request, address, INVITE_LINK, "invitation")
}

// fetchPasswordResetLink waits for the password reset email and hands
// back the link it carries.
export function fetchPasswordResetLink(
	request: APIRequestContext,
	address: string,
): Promise<string> {
	return fetchLink(request, address, PASSWORD_RESET_LINK, "password reset")
}

// fetchAccountExistsLink waits for the notice a repeated signup sends to
// the address's real owner and hands back its login link.
export function fetchAccountExistsLink(
	request: APIRequestContext,
	address: string,
): Promise<string> {
	return fetchLink(request, address, ACCOUNT_EXISTS_LINK, "account exists")
}

// the inbox is searched for the message carrying the wanted link rather
// than the newest one being taken: an address can receive more than one
// message and nothing guarantees the order they land in. Every test
// mints fresh addresses, so a search only ever sees its own mail.
async function fetchLink(
	request: APIRequestContext,
	address: string,
	pattern: RegExp,
	kind: string,
): Promise<string> {
	const found = { link: "" }

	await expect
		.poll(
			async () => {
				found.link = await findLink(request, address, pattern)

				return found.link
			},
			{
				message: `no ${kind} email was delivered to ${address}`,
				timeout: DELIVERY_TIMEOUT,
			},
		)
		.not.toBe("")

	return found.link
}

async function findLink(
	request: APIRequestContext,
	address: string,
	pattern: RegExp,
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
		const match = pattern.exec(HTML)

		if (match?.[1]) {
			// the template escapes the query separator, and following the
			// link as written would hand the server a parameter called
			// "amp;callbackURL" — leaving it with no redirect target, so
			// it answers with raw JSON instead of returning to the app.
			return match[1].replaceAll("&amp;", "&")
		}
	}

	return ""
}
