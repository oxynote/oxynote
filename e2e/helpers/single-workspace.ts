import {
	expect,
	type APIRequestContext,
	type APIResponse,
	type Browser,
	type BrowserContext,
	type Page,
} from "@playwright/test"
import { fillLoginForm, fillSignupForm, type Credentials } from "./auth"
import { t } from "./i18n"
import { fetchVerificationLink } from "./mailpit"
import { hydrated, visit } from "./page"

// the prod stack's third instance of the image, started with the default
// workspace limits (oxynote-single-workspace in docker-compose.prod.yaml).
// Changing the port means changing that service.
export const SINGLE_WORKSPACE_URL = "http://localhost:19082"

// the admin the instance creates at its first boot. The operator README
// documents these values.
export const DEFAULT_ADMIN: Credentials = {
	email: "admin@example.com",
	password: "oxynote-admin-1234",
}

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

// defaultWorkspaceName reads the name of the instance's one workspace as
// the context's user. The instance picks the name at its first boot, so a
// test cannot know it up front.
export async function defaultWorkspaceName(
	request: APIRequestContext,
): Promise<string> {
	const response = await request.get(
		`${SINGLE_WORKSPACE_URL}/auth-realtime/api/auth/organization/list`,
		{ headers: { Origin: SINGLE_WORKSPACE_URL } },
	)
	expect(response.ok()).toBe(true)

	const [workspace] = (await response.json()) as { name: string }[]
	if (!workspace) {
		throw new Error("the instance has no workspace")
	}

	return workspace.name
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
	await expect(page).toHaveURL(
		(url) =>
			url.origin === SINGLE_WORKSPACE_URL &&
			url.pathname === "/login" &&
			url.searchParams.get("verified") === "true" &&
			decodeURIComponent(url.searchParams.get("next") ?? "").startsWith(
				"/accept-invite?",
			),
	)

	await hydrated(page)
	await fillLoginForm(page, credentials)
	// the verification link kept the invitation, so the login lands on it
	// rather than on onboarding
	await expect(page).toHaveURL(/\/accept-invite\?/, { timeout: 15_000 })

	await page
		.getByRole("button", { name: t("onboarding.accept-invite.accept-button") })
		.click()
	await expect(page).toHaveURL(/-[a-z0-9]{20}$/, { timeout: 30_000 })
}
