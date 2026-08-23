import { randomUUID } from "node:crypto"
import {
	expect,
	type APIRequestContext,
	type Locator,
	type Page,
} from "@playwright/test"
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

// revealForm clicks the method button until the form it reveals is on
// the page. The button is server-rendered and visible before vue has
// bound its handler; visit() waits for the app instance, but under load
// the handler can still land a beat later, and a click in that window
// does nothing at all — no request, no error, just the same page.
async function revealForm(button: Locator, field: Locator): Promise<void> {
	await expect(async () => {
		await button.click()
		await expect(field).toBeVisible({ timeout: 1_000 })
	}).toPass()
}

// submitSignupForm walks the email-password branch of the signup page and
// leaves the browser wherever the app sends it.
export async function submitSignupForm(
	page: Page,
	credentials: Credentials,
): Promise<void> {
	await visit(page, "/signup")

	const email = page.getByPlaceholder(
		t("onboarding.signup.email-password-form.email-placeholder"),
	)
	await revealForm(
		page.getByRole("button", {
			name: t("onboarding.signup.signup-email-password"),
		}),
		email,
	)

	await email.fill(credentials.email)
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

	const email = page.getByPlaceholder(
		t("onboarding.login.email-password-form.email-placeholder"),
	)
	await revealForm(
		page.getByRole("button", {
			name: t("onboarding.login.login-email-password"),
		}),
		email,
	)

	await email.fill(credentials.email)
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
	credentials: Credentials = newCredentials(),
): Promise<Credentials> {
	await submitSignupForm(page, credentials)
	// a signup is a round trip through auth-realtime and core, which
	// renders and queues two emails before answering; with several
	// workers signing up at once that can exceed the default wait.
	await expect(page).toHaveURL(/\/verify-email\?/, { timeout: 15_000 })

	await page.goto(await fetchVerificationLink(request, credentials.email))

	// asserted here rather than left to the caller: a rejected token still
	// lands on the login page, and the sign-in that follows would then fail
	// somewhere far away from the reason it failed.
	await expect(page).toHaveURL(`${BASE_URL}/login?verified=true`)

	return credentials
}
