import {
	expect,
	type APIRequestContext,
	type Locator,
	type Page,
} from "@playwright/test"
import {
	fillSignupForm,
	newCredentials,
	revealForm,
	type Credentials,
} from "./auth"
import { t } from "./i18n"
import { submitKeycloakLogin, type IdpUser } from "./keycloak"
import { fetchVerificationLink } from "./mailpit"
import { visit } from "./page"

// the prod stack's two instances that sign in through keycloak. Changing
// a port means changing that service.
export const OIDC_URL = "http://localhost:19083"
export const OIDC_SINGLE_WORKSPACE_URL = "http://localhost:19084"

// the button label both instances configure.
const DISPLAY_NAME = "Keycloak"

// keycloakLoginButton is the OIDC button on the login page the browser is
// on.
export function keycloakLoginButton(page: Page): Locator {
	return page.getByRole("button", {
		name: t("onboarding.login.login-oidc", { name: DISPLAY_NAME }),
	})
}

// logInThroughKeycloak presses the OIDC button on the login page the
// browser is on and signs in at keycloak. It leaves the browser wherever
// oxynote sends it back to.
export async function logInThroughKeycloak(
	page: Page,
	user: IdpUser,
): Promise<void> {
	await keycloakLoginButton(page).click()
	await submitKeycloakLogin(page, user)
}

// signUpThroughKeycloak is logInThroughKeycloak for the signup page.
export async function signUpThroughKeycloak(
	page: Page,
	user: IdpUser,
): Promise<void> {
	await page
		.getByRole("button", {
			name: t("onboarding.signup.signup-oidc", { name: DISPLAY_NAME }),
		})
		.click()
	await submitKeycloakLogin(page, user)
}

// signUpAndVerifyByEmail creates a password account that can log in on the
// OIDC instance. Its signup page offers keycloak too, so the email form has
// to be opened first.
export async function signUpAndVerifyByEmail(
	page: Page,
	request: APIRequestContext,
): Promise<Credentials> {
	const credentials = newCredentials()
	await visit(page, `${OIDC_URL}/signup`)
	await revealForm(
		page.getByRole("button", {
			name: t("onboarding.signup.signup-email-password"),
		}),
		page.getByPlaceholder(
			t("onboarding.signup.email-password-form.email-placeholder"),
		),
	)
	await fillSignupForm(page, credentials)
	await expect(page).toHaveURL(/\/verify-email\?/, { timeout: 15_000 })

	await page.goto(await fetchVerificationLink(request, credentials.email))
	await expect(page).toHaveURL(`${OIDC_URL}/login?verified=true`)

	return credentials
}

// linkedProviders lists the sign-in methods of the context's account, such
// as "credential" for a password and "oidc" for keycloak.
export async function linkedProviders(
	request: APIRequestContext,
	origin: string,
): Promise<string[]> {
	const response = await request.get(
		`${origin}/auth-realtime/api/auth/list-accounts`,
		{ headers: { Origin: origin } },
	)
	if (!response.ok()) {
		throw new Error(`list-accounts answered ${String(response.status())}`)
	}

	const accounts = (await response.json()) as { providerId: string }[]

	return accounts.map((account) => account.providerId).sort()
}

// activeMemberRole reads the context's role in its active workspace.
export async function activeMemberRole(
	request: APIRequestContext,
	origin: string,
): Promise<string> {
	const response = await request.get(
		`${origin}/auth-realtime/api/auth/organization/get-active-member`,
		{ headers: { Origin: origin } },
	)
	if (!response.ok()) {
		throw new Error(`get-active-member answered ${String(response.status())}`)
	}

	const { role } = (await response.json()) as { role: string }

	return role
}
