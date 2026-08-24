import { expect, type Page } from "@playwright/test"
import { t } from "./i18n"

// openInbox opens the notification inbox from the sidebar. The inbox
// row is not a button — it is a plain sidebar row div — so it is
// reached through its sidebar slot.
export async function openInbox(page: Page): Promise<void> {
	await page
		.locator('[data-sidebar="menu-button"]', {
			hasText: t("sidebar.sections.top.inbox"),
		})
		.click()

	await expect(
		page.getByRole("button", { name: t("notification.read-all-button") }),
	).toBeVisible()
}
