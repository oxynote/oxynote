import { expect, type Locator, type Page } from "@playwright/test"
import { t } from "./i18n"

// openSettings opens the settings dialog from the workspace menu in the
// sidebar header and returns the dialog.
export async function openSettings(
	page: Page,
	workspaceName: string,
): Promise<Locator> {
	await page.getByRole("button", { name: workspaceName }).click()
	await page
		.getByRole("menuitem", { name: t("sidebar.header.settings") })
		.click()

	const dialog = page.getByRole("dialog")
	await expect(
		dialog.getByRole("heading", { name: t("settings.workspace.title") }),
	).toBeVisible()

	return dialog
}
