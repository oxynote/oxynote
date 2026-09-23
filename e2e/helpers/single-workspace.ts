import {
	expect,
	type APIRequestContext,
	type APIResponse,
	type Browser,
	type BrowserContext,
	type Page,
} from "@playwright/test"
import { fillSignupForm, submitLoginForm, type Credentials } from "./auth"
import { t } from "./i18n"
import { fetchVerificationLink } from "./mailpit"
import { visit } from "./page"

// the prod stack's third instance of the image, started with the default
// workspace limits (oxynote-single-workspace in docker-compose.prod.yaml).
// Changing the port means changing that service.
export const SINGLE_WORKSPACE_URL = "http://localhost:19082"

// the admin and workspace the instance creates at its first boot. The
// operator README documents these values.
export const DEFAULT_ADMIN: Credentials = {
	email: "admin@example.com",
	password: "oxynote-admin-1234",
}
export const DEFAULT_WORKSPACE_NAME = "Oxynote"

// authPost calls one of better-auth's endpoints on the instance as the
// context's user. better-auth refuses a request without a trusted origin,
// which a browser would send on its own.
export function authPost(
	request: APIRequestContext,
	path: string,
	data: Record<string, unknown> = {},
): Promise<APIResponse> {
	return request.post(`${SINGLE_WORKSPACE_URL}/auth-realtime/api/auth${path}`, {
		data,
		headers: { Origin: SINGLE_WORKSPACE_URL },
	})
}

// signedInContext opens a browser context already signed in with the given
// credentials. It signs in over the API, since the login page itself is
// what other tests drive.
export async function signedInContext(
	browser: Browser,
	credentials: Credentials,
): Promise<BrowserContext> {
	const context = await browser.newContext()
	const response = await authPost(context.request, "/sign-in/email", {
		email: credentials.email,
		password: credentials.password,
	})
	expect(response.ok()).toBe(true)

	return context
}

// joinThroughInvitation takes an invitee from the emailed invitation to a
// member of the workspace. It signs up from the invitation, which is the
// only way to reach the signup form here, then verifies, logs in and
// accepts.
export async function joinThroughInvitation(
	page: Page,
	request: APIRequestContext,
	inviteLink: string,
	credentials: Credentials,
): Promise<void> {
	await visit(page, inviteLink)
	await page
		.getByRole("button", { name: t("onboarding.accept-invite.signup-button") })
		.click()
	await expect(page).toHaveURL(/\/signup\?next=/)
	await fillSignupForm(page, credentials)
	await expect(page).toHaveURL(/\/verify-email\?/, { timeout: 15_000 })

	await page.goto(await fetchVerificationLink(request, credentials.email))
	await expect(page).toHaveURL(`${SINGLE_WORKSPACE_URL}/login?verified=true`)

	await submitLoginForm(page, credentials, SINGLE_WORKSPACE_URL)
	// a fresh account has no workspace yet, so the login redirect lands on
	// onboarding. The invitation is what gets them out of it.
	await expect(page).toHaveURL(/\/welcome$/, { timeout: 15_000 })

	await visit(page, inviteLink)
	await page
		.getByRole("button", { name: t("onboarding.accept-invite.accept-button") })
		.click()
	await expect(page).toHaveURL(/-[a-z0-9]{20}$/, { timeout: 30_000 })
}
