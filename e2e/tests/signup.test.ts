import { expect, test } from "@playwright/test"
import {
	newCredentials,
	signUpAndVerify,
	submitSignupForm,
} from "../helpers/auth"
import { BASE_URL } from "../helpers/config"
import { t } from "../helpers/i18n"
import {
	fetchAccountExistsLink,
	fetchVerificationLink,
} from "../helpers/mailpit"

test.describe("signup", () => {
	test("sends a verification link that activates the new account", async ({
		page,
		request,
	}) => {
		const credentials = newCredentials()

		await submitSignupForm(page, credentials)

		await expect(
			page.getByText(t("onboarding.verify-email.sent-heading")),
		).toBeVisible()
		await expect(
			page.getByText(
				t("onboarding.verify-email.sent-title", {
					email: credentials.email,
				}),
			),
		).toBeVisible()

		await page.goto(await fetchVerificationLink(request, credentials.email))

		// better-auth appends an error parameter to the callback URL when a
		// token is rejected, so the bare success URL is what proves the
		// account was actually activated.
		await expect(page).toHaveURL(`${BASE_URL}/login?verified=true`)
		await expect(page.getByText(t("onboarding.login.title"))).toBeVisible()
	})

	test("answers a repeated signup as if the email were new", async ({
		page,
		request,
	}) => {
		const credentials = await signUpAndVerify(page, request)

		await submitSignupForm(page, credentials)

		// the browser must not be able to probe which emails have
		// accounts, so a duplicate gets the same check-your-inbox page a
		// fresh signup gets
		await expect(page).toHaveURL(/\/verify-email\?/, { timeout: 15_000 })
		await expect(
			page.getByText(
				t("onboarding.verify-email.sent-title", {
					email: credentials.email,
				}),
			),
		).toBeVisible()
	})

	test("warns the account owner about a repeated signup", async ({
		page,
		request,
	}) => {
		const credentials = await signUpAndVerify(page, request)

		await submitSignupForm(page, credentials)
		await expect(page).toHaveURL(/\/verify-email\?/, { timeout: 15_000 })

		// the delivered notice is the only signal the real owner gets —
		// the signup page itself reveals nothing
		const link = await fetchAccountExistsLink(request, credentials.email)
		expect(link).toBe(`${BASE_URL}/login`)
	})
})
