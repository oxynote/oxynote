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

	// the closed inbox stays mounted beside the screen, so only its place
	// tells that it opened
	await expect(
		page.getByRole("button", {
			name: t("notification.actions.close-notification-box"),
		}),
	).toBeInViewport()
}
