import { expect, test } from "@playwright/test"
import {
	newCredentials,
	signUpAndVerify,
	submitLoginForm,
	submitSignupForm,
} from "../helpers/auth"
import { waitForEditor } from "../helpers/editor"
import { t } from "../helpers/i18n"
import { visit } from "../helpers/page"
import { signUpWithWorkspace } from "../helpers/workspace"

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

	test("rejects the wrong password for an existing account", async ({
		page,
		request,
	}) => {
		const credentials = await signUpAndVerify(page, request)

		await submitLoginForm(page, {
			email: credentials.email,
			password: "e2e-Wr0ng!-password-x",
		})

		await expect(
			page.getByText(t("onboarding.login.errors.invalid-credentials")),
		).toBeVisible()
		await expect(page).toHaveURL(/\/login$/)
	})

	test("sends an unverified user back to email verification", async ({
		page,
	}) => {
		const credentials = newCredentials()
		await submitSignupForm(page, credentials)
		await expect(page).toHaveURL(/\/verify-email\?/, { timeout: 15_000 })

		await submitLoginForm(page, credentials)

		// the server refuses the sign-in and re-sends the verification
		// link, so the check-your-inbox page is accurate again
		await expect(page).toHaveURL(/\/verify-email\?/, { timeout: 15_000 })
		await expect(
			page.getByText(
				t("onboarding.verify-email.sent-title", {
					email: credentials.email,
				}),
			),
		).toBeVisible()
	})

	test("keeps the user signed in across a reload", async ({
		page,
		request,
	}) => {
		await signUpWithWorkspace(page, request)

		await visit(page, "/")

		// the root path routes a signed-in user to their first document,
		// which only works while the session still holds
		await expect(page).toHaveURL(/Welcome-to-Oxynote-[a-z0-9]{20}$/, {
			timeout: 15_000,
		})
		await waitForEditor(page)
	})

	test("asks a signed-out visitor of a page to log in", async ({
		page,
		request,
		browser,
	}) => {
		await signUpWithWorkspace(page, request)

		const context = await browser.newContext()
		const visitor = await context.newPage()
		await visitor.goto(page.url())

		await expect(visitor).toHaveURL(/\/login/, { timeout: 15_000 })
		await expect(visitor.getByText(t("onboarding.login.title"))).toBeVisible()

		await context.close()
	})
})
