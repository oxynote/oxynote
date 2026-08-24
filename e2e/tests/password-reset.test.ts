import { expect, test } from "@playwright/test"
import {
	resetPassword,
	signUpAndVerify,
	submitLoginForm,
} from "../helpers/auth"
import { t } from "../helpers/i18n"

test.describe("password reset", () => {
	test("lets the user in with the new password", async ({ page, request }) => {
		const credentials = await signUpAndVerify(page, request)
		const newPassword = "e2e-Rebooted-Passw0rd!"

		await resetPassword(page, request, credentials.email, newPassword)

		await expect(
			page.getByText(t("onboarding.login.password-reset-success")),
		).toBeVisible()

		await submitLoginForm(page, {
			email: credentials.email,
			password: newPassword,
		})
		await expect(page).toHaveURL(/\/welcome$/, { timeout: 15_000 })
	})

	test("rejects the old password", async ({ page, request }) => {
		const credentials = await signUpAndVerify(page, request)

		await resetPassword(
			page,
			request,
			credentials.email,
			"e2e-Rebooted-Passw0rd!",
		)

		await submitLoginForm(page, credentials)

		await expect(
			page.getByText(t("onboarding.login.errors.invalid-credentials")),
		).toBeVisible()
		await expect(page).toHaveURL(/\/login$/)
	})
})
