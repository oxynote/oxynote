import { expect, test } from "@playwright/test"
import {
	fillLoginForm,
	newCredentials,
	signUpAndVerify,
	submitLoginForm,
	submitSignupForm,
} from "../helpers/auth"
import { contentEditor, createDocument, waitForEditor } from "../helpers/editor"
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

	test("takes a returning user back to their workspace", async ({
		page,
		request,
		browser,
	}) => {
		const { credentials, workspace } = await signUpWithWorkspace(page, request)

		const context = await browser.newContext()
		const returning = await context.newPage()
		await submitLoginForm(returning, credentials)

		// the login redirect ends on the workspace's first document, a cold
		// document load in a browser that has never opened it
		await expect(returning).toHaveURL(
			new RegExp(`/${workspace.slug}/.+-[a-z0-9]{20}$`),
			{ timeout: 30_000 },
		)

		await context.close()
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

	test("returns a signed-out visitor to the linked block after login", async ({
		page,
		request,
		browser,
	}) => {
		// a second login and a second cold document load come on top of the
		// usual signup-and-workspace setup
		test.slow()

		const { credentials } = await signUpWithWorkspace(page, request)
		await createDocument(page)
		// the linked block sits more than a screen down, so only a scroll to
		// it brings it into view
		await contentEditor(page).click()
		for (let line = 0; line < 40; line += 1) {
			await page.keyboard.press("Enter")
		}
		await page.keyboard.type("Linked block")
		const block = contentEditor(page).getByText("Linked block", { exact: true })
		await expect(block).toHaveAttribute("id", /.+/)
		const uid = (await block.getAttribute("id")) ?? ""
		const link = `${page.url()}#${uid}`

		const context = await browser.newContext()
		const visitor = await context.newPage()
		await visit(visitor, link)
		await expect(visitor).toHaveURL(/\/login/, { timeout: 15_000 })

		await fillLoginForm(visitor, credentials)

		await expect(visitor).toHaveURL(link, { timeout: 15_000 })
		await waitForEditor(visitor)
		await expect(visitor.locator(`[id="${uid}"]`)).toBeInViewport()

		await context.close()
	})
})
