import { expect, test } from "@playwright/test"
import {
	contentEditor,
	createDocument,
	documentPersisted,
	editorText,
	openDocumentActions,
	openSlashMenu,
	sidebarDocument,
	titleEditor,
	waitForEditor,
} from "../helpers/editor"
import { t } from "../helpers/i18n"
import { visit } from "../helpers/page"
import { signUpWithWorkspace } from "../helpers/workspace"

test.describe("editor", () => {
	test("creates a page and opens it empty", async ({ page, request }) => {
		await signUpWithWorkspace(page, request)

		await createDocument(page)

		await expect
			.poll(() => editorText(titleEditor(page)))
			.toBe(t("editor.new-document-name"))
		await expect(
			contentEditor(page).locator("[data-placeholder]"),
		).toHaveAttribute(
			"data-placeholder",
			t("editor.placeholders.content.paragraph"),
		)
		await expect(page).toHaveTitle(
			t("general.document-page-title", {
				suffix: t("editor.new-document-name"),
			}),
		)
	})

	test("renames a page from its title", async ({ page, request }) => {
		await signUpWithWorkspace(page, request)
		await createDocument(page)

		await titleEditor(page).click()
		await page.keyboard.press("ControlOrMeta+A")
		await page.keyboard.type("Release Notes")

		// the name is pushed everywhere it is shown: the URL slug, the
		// sidebar row, the breadcrumb and the tab title
		await expect(page).toHaveURL(/Release-Notes-[a-z0-9]{20}$/)
		await expect(sidebarDocument(page, "Release Notes")).toBeVisible()
		await expect(
			page
				.getByLabel("breadcrumb")
				.getByRole("link", { name: "Release Notes" }),
		).toBeVisible()
		await expect(page).toHaveTitle(
			t("general.document-page-title", { suffix: "Release Notes" }),
		)
	})

	test("keeps content across a reload", async ({ page, request }) => {
		await signUpWithWorkspace(page, request)
		await createDocument(page)
		const url = page.url()

		await contentEditor(page).click()
		await page.keyboard.type("Persist me")
		await documentPersisted(page)

		await visit(page, url)

		await waitForEditor(page)
		await expect.poll(() => editorText(contentEditor(page))).toBe("Persist me")
	})

	test.describe("markdown shortcuts", () => {
		const CASES = [
			{ name: "a heading", typed: "# Title", selector: "h1", text: "Title" },
			{
				name: "a bulleted list",
				typed: "- item",
				selector: "ul > li",
				text: "item",
			},
			{
				name: "a numbered list",
				typed: "1. first",
				selector: "ol > li",
				text: "first",
			},
			{
				name: "bold text",
				typed: "**strong**",
				selector: "strong",
				text: "strong",
			},
			{
				name: "italic text",
				typed: "*slanted*",
				selector: "em",
				text: "slanted",
			},
		]

		for (const c of CASES) {
			test(`turns ${c.typed} into ${c.name}`, async ({ page, request }) => {
				await signUpWithWorkspace(page, request)
				await createDocument(page)

				await contentEditor(page).click()
				await page.keyboard.type(c.typed)

				await expect(contentEditor(page).locator(c.selector)).toHaveText(c.text)
			})
		}
	})

	test("inserts a code block from the slash menu", async ({
		page,
		request,
	}) => {
		await signUpWithWorkspace(page, request)
		await createDocument(page)

		await contentEditor(page).click()
		const menu = await openSlashMenu(page)
		await page.keyboard.type("Code")

		// typing narrows the menu down to the one matching command
		await expect(menu.getByRole("button")).toHaveCount(1)
		await expect(menu.getByRole("button")).toContainText("Code Block")

		await page.keyboard.press("Enter")
		await page.keyboard.type("const answer = 42")

		await expect(contentEditor(page).locator("pre code")).toContainText(
			"const answer = 42",
		)
	})

	test("deletes a page after confirming", async ({ page, request }) => {
		await signUpWithWorkspace(page, request)
		await createDocument(page)
		const name = t("editor.new-document-name")

		await openDocumentActions(page, name)
		await page
			.getByRole("menuitem", {
				name: t("sidebar.item-dropdown-menu-buttons.delete-page"),
			})
			.click()

		const dialog = page.getByRole("dialog")
		await expect(dialog).toContainText(
			t("editor.document-deletion-modal.title"),
		)
		await dialog
			.getByRole("button", {
				name: t("editor.document-deletion-modal.confirm-button"),
			})
			.click()

		await expect(sidebarDocument(page, name)).toHaveCount(0)
		await expect(
			page.getByText(
				t("editor.document-deletion-modal.deletion-success", { name }),
			),
		).toBeVisible()
		// the welcome page is the only one left, so that is where the
		// user lands
		await expect(page).toHaveURL(/Welcome-to-Oxynote-[a-z0-9]{20}$/)
	})

	test("nests a sub page under its parent", async ({ page, request }) => {
		await signUpWithWorkspace(page, request)
		await createDocument(page)
		const parent = t("editor.new-document-name")

		await openDocumentActions(page, parent)
		await page
			.getByRole("menuitem", {
				name: t("sidebar.item-dropdown-menu-buttons.add-sub-page"),
			})
			.click()

		const parentRow = page
			.locator('[data-sidebar="menu-item"]', {
				has: sidebarDocument(page, parent),
			})
			.first()
		await expect(parentRow).toHaveAttribute("data-item-children", "1")

		// the child list is collapsed until the parent is expanded
		await parentRow.hover()
		await parentRow
			.getByRole("button", {
				name: t("sidebar.item-collapse-trigger-button.open-screen-reader-hint"),
			})
			.click()

		const child = parentRow.locator(
			'[data-sidebar="menu-badge"] a[data-sidebar="menu-button"]',
		)
		await expect(child).toHaveText(parent)
		await expect(child).not.toHaveAttribute("data-optimistic-insert", "")
	})
})
