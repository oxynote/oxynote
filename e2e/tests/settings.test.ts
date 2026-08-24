import { expect, test } from "@playwright/test"
import { newCredentials } from "../helpers/auth"
import { joinAsSecondUser } from "../helpers/collaboration"
import { waitForEditor } from "../helpers/editor"
import { t } from "../helpers/i18n"
import { visit } from "../helpers/page"
import { openSettings } from "../helpers/settings"
import { signUpWithWorkspace } from "../helpers/workspace"

test.describe("settings", () => {
	test("renames the workspace", async ({ page, request }) => {
		const { workspace } = await signUpWithWorkspace(page, request)

		const dialog = await openSettings(page, workspace.name)
		// the name field has no save button — it submits when it loses
		// focus
		const name = dialog.getByPlaceholder(
			t("settings.workspace.name-placeholder"),
			{ exact: true },
		)
		await name.fill("Retitled-Team")
		await name.blur()

		await expect(
			page.getByText(t("settings.workspace.success-messages.name.title")),
		).toBeVisible()

		await page.keyboard.press("Escape")
		await expect(page.getByRole("dialog")).toHaveCount(0)
		await expect(
			page.getByRole("button", { name: "Retitled-Team" }),
		).toBeVisible()
	})

	test("changes the username", async ({ page, request }) => {
		const { workspace } = await signUpWithWorkspace(page, request)
		const url = page.url()

		const dialog = await openSettings(page, workspace.name)
		const username = dialog.getByPlaceholder(
			t("settings.profile.username-placeholder"),
			{ exact: true },
		)
		await username.fill("Fresh-Nickname")
		await username.blur()

		await expect(
			page.getByText(t("settings.profile.success-messages.username.title")),
		).toBeVisible()

		// a reload proves the new name came back from the server rather
		// than lingering in the form
		await page.keyboard.press("Escape")
		await visit(page, url)
		await waitForEditor(page)
		const reopened = await openSettings(page, workspace.name)
		await expect(
			reopened.getByPlaceholder(t("settings.profile.username-placeholder"), {
				exact: true,
			}),
		).toHaveValue("Fresh-Nickname")
	})

	test("lists a joined teammate among the members", async ({
		page,
		request,
		browser,
	}) => {
		// two signups, a verification each, an invitation and its
		// acceptance run before the first assertion
		test.slow()

		const { credentials, workspace } = await signUpWithWorkspace(page, request)
		const other = await joinAsSecondUser(browser, page, request)

		const dialog = await openSettings(page, workspace.name)

		await expect(
			dialog.getByRole("row").filter({ hasText: credentials.email }),
		).toBeVisible()
		await expect(
			dialog.getByRole("row").filter({ hasText: other.credentials.email }),
		).toBeVisible()

		await other.context.close()
	})

	test("shows a pending invitation among the members", async ({
		page,
		request,
	}) => {
		const { workspace } = await signUpWithWorkspace(page, request)
		const invitee = newCredentials()

		await openSettings(page, workspace.name)
		await page
			.getByRole("button", {
				name: t("settings.workspace.invitation-button"),
				exact: true,
			})
			.click()
		await page
			.getByPlaceholder(
				t("settings.action-modals.workspace-invitation.email-placeholder"),
			)
			.fill(invitee.email)
		await page
			.getByRole("button", {
				name: t("settings.action-modals.workspace-invitation.submit-button"),
			})
			.click()

		await expect(
			page.getByText(
				t("settings.action-modals.workspace-invitation.success-message.title"),
			),
		).toBeVisible()

		const row = page.getByRole("row").filter({ hasText: invitee.email })
		await expect(row).toBeVisible()
		await expect(
			row.getByText(t("settings.workspace.invited-label")),
		).toBeVisible()
	})
})
