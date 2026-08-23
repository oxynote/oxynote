import { expect, test } from "@playwright/test"
import {
	newCredentials,
	signUpAndVerify,
	submitLoginForm,
} from "../helpers/auth"
import { t } from "../helpers/i18n"

test.describe("login", () => {
	test("takes a verified user to workspace creation", async ({
		page,
		request,
	}) => {
		const credentials = await signUpAndVerify(page, request)

		await submitLoginForm(page, credentials)

		// a logged in user without an organization is routed to onboarding,
		// so landing there is what proves the session was established.
		await expect(page).toHaveURL(/\/welcome$/)
		await expect(page.getByText(t("onboarding.welcome.title"))).toBeVisible()
		await expect(page.getByText(credentials.email)).toBeVisible()
	})

	test("rejects an unknown email and password", async ({ page }) => {
		await submitLoginForm(page, newCredentials())

		await expect(
			page.getByText(t("onboarding.login.errors.invalid-credentials")),
		).toBeVisible()
		await expect(page).toHaveURL(/\/login$/)
	})
})
