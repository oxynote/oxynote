import { expect, type Locator, type Page } from "@playwright/test"
import { t } from "./i18n"
import { visit } from "./page"

// the two tiptap editors on a document page. They are separate editor
// instances, not one editor with a heading: the title has its own
// schema (a single paragraph) and its own yjs field.
export function titleEditor(page: Page): Locator {
	return page.locator(".group\\/name-editor .ProseMirror")
}

export function contentEditor(page: Page): Locator {
	return page.locator(".content-editor .ProseMirror")
}

// editorText reads an editor's text without its decoration widgets. A
// collaborator's caret is rendered as a widget inside the paragraph it
// sits in, label and all, so a plain text read of a shared document
// comes back with the other user's name spliced into the content.
export function editorText(editor: Locator): Promise<string> {
	return editor.evaluate((el) => {
		const clone = el.cloneNode(true) as HTMLElement

		clone
			.querySelectorAll(".ProseMirror-widget, .pm-gap-wrapper")
			.forEach((widget) => {
				widget.remove()
			})

		return clone.textContent
	})
}

// remoteCarets are the other users' cursors as rendered in this editor.
// The label carries the collaborator's display name. Scoped to the
// decoration widget because the caret-transparent utility also appears
// on unrelated editor chrome (the metric block's empty state).
export function remoteCarets(editor: Locator): Locator {
	return editor.locator(".ProseMirror-widget .caret-transparent")
}

// waitForEditor gates on the editor being mounted. Everything on a
// document page sits behind the hocuspocus sync: until the websocket
// reports the document loaded, the editor, the sidebar body and the
// header are all absent — a blank page, not an error. A cold load is
// the whole chain — session, organization, tree, socket, sync, then a
// fade-in — and with several workers loading documents at once it can
// outrun the default assertion timeout while being entirely healthy.
export async function waitForEditor(page: Page): Promise<void> {
	await expect(contentEditor(page)).toBeVisible({ timeout: 15_000 })
}

// workspaceSection is the sidebar group holding the document tree. The
// sidebar has several groups, and a document listed under a tag is the
// same row markup as the one in the tree, so anything looked up by row
// has to say which group it means.
function workspaceSection(page: Page): Locator {
	return page.locator('[data-sidebar="group"]', {
		has: page.locator('[data-sidebar="group-label"]', {
			hasText: t("sidebar.sections.main-workspace.heading"),
		}),
	})
}

// sidebarDocument is the document's row in the workspace tree. Scoped
// to the sidebar link because the breadcrumb in the header also exposes
// the name as a link, and to the workspace group because the tags
// section lists the same documents again.
export function sidebarDocument(page: Page, name: string): Locator {
	return workspaceSection(page).locator('a[data-sidebar="menu-button"]', {
		hasText: name,
	})
}

// sidebarDocumentRow is the whole tree row — the link plus the collapse
// and actions buttons that appear on hover. The inner locator is written
// out rather than reusing sidebarDocument because `has` resolves its
// argument against the row, where the enclosing group is out of reach.
export function sidebarDocumentRow(page: Page, name: string): Locator {
	return workspaceSection(page).locator('[data-sidebar="menu-item"]', {
		has: page.locator('a[data-sidebar="menu-button"]', { hasText: name }),
	})
}

// openDocumentActions opens the "…" menu on a document's sidebar row.
// The button is summoned by hovering the row and every row has one, so
// it is found through the row rather than by name alone.
export async function openDocumentActions(
	page: Page,
	name: string,
): Promise<void> {
	const row = sidebarDocumentRow(page, name)

	await row.first().hover()
	await row
		.first()
		.getByRole("button", {
			name: t("sidebar.item-dropdown-menu-trigger-button.screen-reader-hint"),
		})
		.click()
}

// createDocument adds a page at the workspace root and opens it. The
// new row appears at once as an optimistic insert with no href; the real
// one, with a navigable href, replaces it when the server answers. The
// href is read back and navigated to directly rather than clicked: the
// tree refetch that brings the real row can re-render the sidebar under
// a click that has already been dispatched.
export async function createDocument(page: Page): Promise<void> {
	await workspaceSection(page).locator('[data-sidebar="group-action"]').click()

	const row = sidebarDocument(page, t("editor.new-document-name"))
	await expect(row).toHaveAttribute("href", /New-Page-[a-z0-9]{20}$/)

	const href = await row.getAttribute("href")
	await visit(page, href ?? "")

	await expect(page).toHaveURL(/New-Page-[a-z0-9]{20}$/)
	await waitForEditor(page)
}

// openSlashMenu types the trigger and waits for the menu, which is
// appended to the body rather than rendered inside the editor.
export async function openSlashMenu(page: Page): Promise<Locator> {
	await page.keyboard.type("/")

	const menu = page.locator('body > div[data-state="open"]').first()
	await expect(menu).toBeVisible()

	return menu
}

// documentPersisted waits for the yjs document to reach the server.
// Hocuspocus stores on a 2 s debounce after the last change; nothing
// on the client reports when that happened, so this is the one place
// the suite waits on the clock, and it waits for the longest the
// debounce can take rather than the usual case.
export async function documentPersisted(page: Page): Promise<void> {
	// eslint-disable-next-line playwright/no-wait-for-timeout -- the store is debounced server-side with no client-visible signal
	await page.waitForTimeout(2_500)
}
