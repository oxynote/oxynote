import { expect, type Locator, type Page } from "@playwright/test"
import { t } from "./i18n"

// tagsSection is the sidebar group listing the organization's tags. Every
// lookup below is scoped to it: a document listed under a tag is the same
// row markup as the one in the workspace tree.
function tagsSection(page: Page): Locator {
	return page.locator('[data-sidebar="group"]', {
		has: page.locator('[data-sidebar="group-label"]', {
			hasText: t("sidebar.sections.tags.heading"),
		}),
	})
}

// sidebarTag is a tag's own label in the tags section. A tag has no page
// to link to, so unlike a document its button is a plain element.
export function sidebarTag(page: Page, name: string): Locator {
	return tagsSection(page).locator('div[data-sidebar="menu-button"]', {
		hasText: name,
	})
}

// tagRow is the whole row of a tag: the label, the buttons that appear on
// hover and the collapsible list of the documents carrying it. The
// documents nest inside the row, which is why the lookup goes by the tag's
// own label rather than by any text the row contains. The inner locator is
// written out rather than reusing sidebarTag because `has` resolves its
// argument against the row, where the enclosing section is out of reach.
function tagRow(page: Page, name: string): Locator {
	return tagsSection(page).locator('[data-sidebar="menu-item"]', {
		has: page.locator('div[data-sidebar="menu-button"]', { hasText: name }),
	})
}

// expandTag opens a tag's list of documents. A tag created during the
// session starts collapsed, and clicking its label toggles the list. The
// list's open state sits on the nearest collapsible above the label; the
// documents nested below it carry collapsibles of their own.
export async function expandTag(page: Page, name: string): Promise<void> {
	const collapsible = sidebarTag(page, name).locator(
		"xpath=ancestor::*[@data-state][1]",
	)

	if ((await collapsible.getAttribute("data-state")) === "open") {
		return
	}

	await sidebarTag(page, name).click()
	await expect(collapsible).toHaveAttribute("data-state", "open")
}

// taggedDocument is a document's link as listed under a tag.
export function taggedDocument(
	page: Page,
	tagName: string,
	documentName: string,
): Locator {
	return tagRow(page, tagName).locator('a[data-sidebar="menu-button"]', {
		hasText: documentName,
	})
}

// openTagActions opens the "…" menu on a tag's sidebar row. The button is
// summoned by hovering the row and every row has one, the documents
// nested under the tag included, so it is found next to the tag's own
// label rather than anywhere in the row.
export async function openTagActions(page: Page, name: string): Promise<void> {
	const label = sidebarTag(page, name)

	await label.hover()
	await label
		.locator("..")
		.getByRole("button", {
			name: t("sidebar.item-dropdown-menu-trigger-button.screen-reader-hint"),
		})
		.click()
}

// openTagVisibility opens the section heading's menu listing every tag,
// hidden ones included, which is where a hidden tag is brought back.
export async function openTagVisibility(page: Page): Promise<void> {
	await tagsSection(page).locator('[data-sidebar="group-action"]').click()
}

// tagPicker is the header's tag menu, which is teleported out of the
// header and reached by the search box it opens with.
function tagPicker(page: Page): Locator {
	return page.getByPlaceholder(t("editor.tags.search-placeholder"))
}

// createTagFromHeader creates a tag from the document header's picker and
// assigns it to the open branch in one go: typing a name nobody uses and
// pressing enter is the create row. The picker stays open across
// creations, so it is closed again afterwards.
export async function createTagFromHeader(
	page: Page,
	name: string,
): Promise<void> {
	await page.locator(`[title="${t("editor.tags.trigger-title")}"]`).click()
	await tagPicker(page).fill(name)
	await tagPicker(page).press("Enter")

	await expect(headerTagPill(page, name)).toBeVisible()
	await page.keyboard.press("Escape")
}

// headerTagPill is a tag's pill in the document header, which shows the
// tags of the branch on screen.
export function headerTagPill(page: Page, name: string): Locator {
	return page
		.locator(`[title="${t("editor.tags.trigger-title")}"]`)
		.getByText(name, { exact: true })
}
