import { expect, type Locator, type Page } from "@playwright/test"
import { t } from "./i18n"

// openSearch opens the search dialog from the sidebar and returns the
// dialog. The search row is not a button — it is a plain sidebar row
// div — so it is reached through its sidebar slot.
export async function openSearch(page: Page): Promise<Locator> {
	await page
		.locator('[data-sidebar="menu-button"]', {
			hasText: t("sidebar.sections.top.search-button"),
		})
		.click()

	const dialog = page.getByRole("dialog")
	await expect(
		dialog.getByPlaceholder(t("sidebar.search.input-placeholder")),
	).toBeVisible()

	return dialog
}

// searchFor types the query into the open search dialog and waits for
// the expected result row. The results only refresh when the debounced
// query text changes, and a quick clear-and-retype of the same text is
// swallowed by the debounce as no change at all — so a search that
// raced the background indexing of freshly stored content would stay
// empty forever. Each retry therefore alternates between the query and
// the query minus its last character: both prefix-match the same
// results, and every attempt is a real change that triggers a fetch.
export async function searchFor(
	dialog: Locator,
	query: string,
	expected: Locator,
): Promise<void> {
	const input = dialog.getByPlaceholder(t("sidebar.search.input-placeholder"))
	let flip = false

	await expect(async () => {
		flip = !flip

		await input.fill(flip ? query : query.slice(0, -1))
		await expect(expected).toBeVisible({ timeout: 2_000 })
	}).toPass()
}
