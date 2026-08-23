import { expect, test } from "@playwright/test"
import { signUpAndVerify, submitLoginForm } from "../helpers/auth"
import { editorText, titleEditor, waitForEditor } from "../helpers/editor"
import { t } from "../helpers/i18n"
import { visit } from "../helpers/page"
import {
	newWorkspace,
	signUpWithWorkspace,
	submitWorkspaceForm,
} from "../helpers/workspace"

test.describe("onboarding", () => {
	test("creates a workspace and opens its welcome page", async ({
		page,
		request,
	}) => {
		const { workspace } = await signUpWithWorkspace(page, request)

		// the URL is built from the workspace slug and the seeded document's
		// name, so both ends of the creation are visible in it
		await expect(page).toHaveURL(
			new RegExp(`/${workspace.slug}/Welcome-to-Oxynote-[a-z0-9]{20}$`),
		)
		await expect
			.poll(() => editorText(titleEditor(page)))
			.toBe("Welcome to Oxynote!")
		await expect(
			page.getByRole("button", { name: workspace.name }),
		).toBeVisible()
	})

	test("rejects a workspace name with spaces", async ({ page, request }) => {
		const credentials = await signUpAndVerify(page, request)
		await submitLoginForm(page, credentials)
		await expect(page).toHaveURL(/\/welcome$/, { timeout: 15_000 })

		await submitWorkspaceForm(page, { ...newWorkspace(), name: "Acme Corp" })

		await expect(
			page.getByText(t("onboarding.welcome.errors.workspace-name-regex")),
		).toBeVisible()
		await expect(page).toHaveURL(/\/welcome$/)
	})

	test("rejects a workspace URL that is already taken", async ({
		page,
		request,
		browser,
	}) => {
		const { workspace } = await signUpWithWorkspace(page, request)

		const other = await browser.newContext()
		const otherPage = await other.newPage()
		const credentials = await signUpAndVerify(otherPage, request)
		await submitLoginForm(otherPage, credentials)
		await expect(otherPage).toHaveURL(/\/welcome$/, { timeout: 15_000 })

		await submitWorkspaceForm(otherPage, {
			...newWorkspace(),
			slug: workspace.slug,
		})

		await expect(
			otherPage.getByText(t("onboarding.welcome.errors.workspace-url-taken")),
		).toBeVisible()
		await expect(otherPage).toHaveURL(/\/welcome$/)

		await other.close()
	})

	test("sends a user who already has a workspace straight to it", async ({
		page,
		request,
	}) => {
		await signUpWithWorkspace(page, request)

		await visit(page, "/welcome")

		await expect(page).toHaveURL(/Welcome-to-Oxynote-[a-z0-9]{20}$/)
		await waitForEditor(page)
	})

	test("lets a user without a workspace log out", async ({ page, request }) => {
		const credentials = await signUpAndVerify(page, request)
		await submitLoginForm(page, credentials)
		await expect(page).toHaveURL(/\/welcome$/, { timeout: 15_000 })

		await page
			.getByRole("button", { name: t("onboarding.welcome.login-info.logout") })
			.click()

		await expect(page).toHaveURL(/\/login$/)
		await expect(page.getByText(t("onboarding.login.title"))).toBeVisible()
	})
})
