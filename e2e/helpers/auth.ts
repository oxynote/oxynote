import { randomUUID } from "node:crypto"
import { expect, type APIRequestContext, type Page } from "@playwright/test"
import { BASE_URL } from "./config"
import { fetchVerificationLink } from "./mailpit"
import { t } from "./i18n"
import { visit } from "./page"

export interface Credentials {
	email: string
	password: string
}

// newCredentials mints an account nobody else uses, which is what keeps
// the tests independent of each other on a shared stack. The password
// satisfies the signup rules: at least 16 characters, one digit and one
// symbol.
export function newCredentials(): Credentials {
	return {
		email: `e2e-${randomUUID()}@oxynote.test`,
		password: "e2e-Passw0rd!-secure",
	}
}

// submitSignupForm walks the email-password branch of the signup page and
// leaves the browser wherever the app sends it.
export async function submitSignupForm(
	page: Page,
	credentials: Credentials,
): Promise<void> {
	await visit(page, "/signup")

	await page
		.getByRole("button", {
			name: t("onboarding.signup.signup-email-password"),
		})
		.click()

	await page
		.getByPlaceholder(
			t("onboarding.signup.email-password-form.email-placeholder"),
		)
		.fill(credentials.email)
	await page
		.getByPlaceholder(
			t("onboarding.signup.email-password-form.password-placeholder"),
		)
		.fill(credentials.password)

	await page
		.getByRole("button", {
			name: t("onboarding.signup.email-password-form.continue"),
		})
		.click()
}

// submitLoginForm walks the email-password branch of the login page and
// leaves the browser wherever the app sends it.
export async function submitLoginForm(
	page: Page,
	credentials: Credentials,
): Promise<void> {
	await visit(page, "/login")

	await page
		.getByRole("button", { name: t("onboarding.login.login-email-password") })
		.click()

	await page
		.getByPlaceholder(
			t("onboarding.login.email-password-form.email-placeholder"),
		)
		.fill(credentials.email)
	await page
		.getByPlaceholder(
			t("onboarding.login.email-password-form.password-placeholder"),
		)
		.fill(credentials.password)

	await page
		.getByRole("button", {
			name: t("onboarding.login.email-password-form.continue"),
		})
		.click()
}

// signUpAndVerify creates an account that can log in. Verification is not
// optional: the server refuses to sign in an unverified user, so this is
// the setup any test needing an existing account starts from.
export async function signUpAndVerify(
	page: Page,
	request: APIRequestContext,
): Promise<Credentials> {
	const credentials = newCredentials()

	await submitSignupForm(page, credentials)
	await expect(page).toHaveURL(/\/verify-email\?/)

	await page.goto(await fetchVerificationLink(request, credentials.email))

	// asserted here rather than left to the caller: a rejected token still
	// lands on the login page, and the sign-in that follows would then fail
	// somewhere far away from the reason it failed.
	await expect(page).toHaveURL(`${BASE_URL}/login?verified=true`)

	return credentials
}
