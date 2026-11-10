import { randomUUID } from "node:crypto"
import { expect, type APIRequestContext, type Page } from "@playwright/test"

// keycloak in the prod stack. Changing the port means changing that
// service.
const KEYCLOAK_URL = "http://localhost:19180"

export interface IdpUser {
	username: string
	password: string
	email: string
}

// newIdpUser creates a keycloak user that only the calling test knows, so
// a retry starts from a user oxynote has never seen. The email is new too,
// unless the test passes the address of an account it already made.
export async function newIdpUser(
	request: APIRequestContext,
	options: { emailVerified: boolean; email?: string },
): Promise<IdpUser> {
	const id = randomUUID()
	const user = {
		username: `idp-${id}`,
		password: `idp-${id}`,
		email: options.email ?? `idp-${id}@oxynote.test`,
	}

	const response = await request.post(
		`${KEYCLOAK_URL}/admin/realms/oxynote/users`,
		{
			headers: { Authorization: `Bearer ${await adminToken(request)}` },
			data: {
				username: user.username,
				email: user.email,
				emailVerified: options.emailVerified,
				enabled: true,
				// keycloak asks for both names on the first sign-in when
				// they are missing.
				firstName: "Idp",
				lastName: "User",
				credentials: [
					{ type: "password", value: user.password, temporary: false },
				],
			},
		},
	)
	expect(response.status()).toBe(201)

	return user
}

// submitKeycloakLogin fills keycloak's own login form, which the browser
// is on after pressing the OIDC button. Its text is keycloak's, so the
// fields are found by their ids.
export async function submitKeycloakLogin(
	page: Page,
	user: IdpUser,
): Promise<void> {
	await expect(page).toHaveURL((url) => url.origin === KEYCLOAK_URL, {
		timeout: 15_000,
	})
	await page.locator("#username").fill(user.username)
	await page.locator("#password").fill(user.password)
	await page.locator("#kc-login").click()
}

// confirmKeycloakLogout answers keycloak's "Do you want to log out?" page.
// Keycloak asks when the logout request carries no id token, which is the
// case for a user oxynote refused and so never signed in.
export async function confirmKeycloakLogout(page: Page): Promise<void> {
	await expect(page).toHaveURL((url) => url.origin === KEYCLOAK_URL, {
		timeout: 15_000,
	})
	await page.locator("#kc-logout").click()
}

async function adminToken(request: APIRequestContext): Promise<string> {
	const response = await request.post(
		`${KEYCLOAK_URL}/realms/master/protocol/openid-connect/token`,
		{
			form: {
				grant_type: "password",
				client_id: "admin-cli",
				username: "admin",
				password: "admin",
			},
		},
	)
	expect(response.ok()).toBe(true)

	const { access_token } = (await response.json()) as { access_token: string }

	return access_token
}
