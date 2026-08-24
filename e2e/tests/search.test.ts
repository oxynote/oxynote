import { expect, test } from "@playwright/test"
import {
	contentEditor,
	createDocument,
	documentPersisted,
	sidebarDocument,
	titleEditor,
	waitForEditor,
} from "../helpers/editor"
import { t } from "../helpers/i18n"
import { openSearch, searchFor } from "../helpers/search"
import { signUpWithWorkspace } from "../helpers/workspace"

test.describe("search", () => {
	// on top of the usual setup, a find waits out the storage debounce
	// and the ten-second search-indexing pass before the first hit can
	// appear. slow() triples the budget for that chain.
	test.beforeEach(() => {
		test.slow()
	})

	test("finds a page by its name", async ({ page, request }) => {
		await signUpWithWorkspace(page, request)
		await createDocument(page)
		await titleEditor(page).click()
		await page.keyboard.press("ControlOrMeta+A")
		await page.keyboard.type("Quarterly Roadmap")
		await expect(page).toHaveURL(/Quarterly-Roadmap-[a-z0-9]{20}$/)
		await documentPersisted(page)

		const dialog = await openSearch(page)
		const result = dialog.locator("a", { hasText: "Quarterly Roadmap" })
		await searchFor(dialog, "Quarterly", result)
		await result.click()

		await expect(page).toHaveURL(/Quarterly-Roadmap-[a-z0-9]{20}$/)
		await waitForEditor(page)
	})

	test("finds a page by its content", async ({ page, request }) => {
		await signUpWithWorkspace(page, request)
		await createDocument(page)
		await contentEditor(page).click()
		await page.keyboard.type("The xylophone brigade rehearses daily")
		await documentPersisted(page)
		// searching starts from another page, so that landing on the
		// found one is observable in the URL
		await sidebarDocument(page, "Welcome to Oxynote!").click()
		await expect(page).toHaveURL(/Welcome-to-Oxynote-[a-z0-9]{20}$/)

		const dialog = await openSearch(page)
		const result = dialog.locator("a", { hasText: "xylophone" })
		await searchFor(dialog, "xylophone", result)
		await result.click()

		await expect(page).toHaveURL(/New-Page-[a-z0-9]{20}/)
	})

	test("reports when nothing matches", async ({ page, request }) => {
		await signUpWithWorkspace(page, request)

		const dialog = await openSearch(page)
		await dialog
			.getByPlaceholder(t("sidebar.search.input-placeholder"))
			.fill("zz-no-such-page-anywhere")

		await expect(
			dialog.getByText(
				t("sidebar.search.no-results", {
					query: "zz-no-such-page-anywhere",
				}),
			),
		).toBeVisible()
	})
})
