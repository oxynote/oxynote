import { expect, test } from "@playwright/test"
import { newCredentials, submitSignupForm } from "../helpers/auth"
import { BASE_URL } from "../helpers/config"
import { t } from "../helpers/i18n"
import { fetchVerificationLink } from "../helpers/mailpit"

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
})
