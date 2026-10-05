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

// diffEditor is the merged read-only editor a draft shows while "show
// changes" is on. It is the one editor that hides its caret, which is
// also what tells it apart from the comment composer mounted in the same
// pane.
export function diffEditor(page: Page): Locator {
	return page.locator(".diff-editor .ProseMirror.caret-transparent")
}

// the panes wrapping each editor together with its block handle, bubble
// menu and comment popover. Anything the editor owns but renders outside
// its ProseMirror element is found through the pane.
export function contentPane(page: Page): Locator {
	return page.locator(".content-editor")
}

export function diffPane(page: Page): Locator {
	return page.locator(".diff-editor")
}

// placeCaret clicks just inside the left or right edge of a block, which
// maps to the first or last position of its line. The block is a
// full-width element, so its edge sits well past its text, and the click
// is retried until focus sticks: a late re-render of a freshly switched
// branch can swallow the first one. Home and End are not used because on
// macOS chromium binds them to the document, not the line.
export async function placeCaret(
	block: Locator,
	edge: "start" | "end",
): Promise<void> {
	await expect(async () => {
		const box = await block.boundingBox()
		expect(box).not.toBeNull()

		await block.click({
			position: {
				x: edge === "start" ? 1 : (box?.width ?? 2) - 1,
				y: (box?.height ?? 2) / 2,
			},
		})
		await expect(
			block.locator("xpath=ancestor::*[contains(@class, 'ProseMirror')]"),
		).toBeFocused()
	}).toPass()
	await selectionSettled(block)
}

// selectText selects one run of a block's text with the keyboard, walking
// to it from the block's start. A mouse drag would depend on where each
// glyph is drawn.
export async function selectText(
	page: Page,
	block: Locator,
	text: string,
): Promise<void> {
	const content = await editorText(block)
	const start = content.indexOf(text)
	if (start === -1) {
		throw new Error(`"${text}" is not in the block reading "${content}"`)
	}

	// ProseMirror puts the caret back where a click left it for 50ms after
	// the click, so arrow keys sent inside that window move nothing. The
	// walk is repeated until the editor holds the text as its selection.
	await expect(async () => {
		await placeCaret(block, "start")
		await pressTimes(page, "ArrowRight", start)
		await pressTimes(page, "Shift+ArrowRight", text.length)
		await selectionSettled(block)
		expect(await selectedText(block)).toBe(text)
	}).toPass()
}

// selectedText is the text the block's editor holds as its selection.
function selectedText(block: Locator): Promise<string> {
	return block.evaluate((el) => {
		const editor = el.closest<Element & { editor?: EditorHandle }>(
			".ProseMirror",
		)?.editor
		if (!editor) {
			return ""
		}

		const { from, to } = editor.state.selection

		return editor.state.doc.textBetween(from, to)
	})
}

// the slice of tiptap's editor, reachable from its element, that
// selectionSettled and selectedText read
interface EditorHandle {
	state: {
		selection: { from: number; to: number }
		doc: { textBetween(from: number, to: number): string }
	}
	view: { posAtDOM(node: Node, offset: number): number }
}

// selectionSettled waits until the editor's own selection matches the
// browser's. A click or an arrow key moves the browser's caret at once,
// but the editor only learns of it from the selectionchange event, which
// queues behind the input that caused it. A key the editor handles
// itself, such as Enter or Backspace, sent before then acts on the old
// selection.
export async function selectionSettled(block: Locator): Promise<void> {
	await expect
		.poll(() =>
			block.evaluate((el) => {
				const root = el.closest<Element & { editor?: EditorHandle }>(
					".ProseMirror",
				)
				const editor = root?.editor
				const selection = document.getSelection()
				if (!editor || !selection?.anchorNode || !selection.focusNode) {
					return false
				}

				try {
					const anchor = editor.view.posAtDOM(
						selection.anchorNode,
						selection.anchorOffset,
					)
					const focus = editor.view.posAtDOM(
						selection.focusNode,
						selection.focusOffset,
					)
					const { from, to } = editor.state.selection

					return (
						Math.min(anchor, focus) === from && Math.max(anchor, focus) === to
					)
				} catch {
					return false
				}
			}),
		)
		.toBe(true)
}

async function pressTimes(
	page: Page,
	key: string,
	times: number,
): Promise<void> {
	for (let pressed = 0; pressed < times; pressed += 1) {
		await page.keyboard.press(key)
	}
}

// the heights, as fractions of a block, at which openBlockMenu hovers it
const HOVER_HEIGHTS = [0.5, 0.25, 0.75, 0.1, 0.9]

// openBlockMenu opens the block handle's menu for a block. The handle
// slides in beside whichever block the pointer is over, so the block is
// hovered first, at more than one height if need be: the handle stays
// away while the pointer is level with an element that opts out of it,
// such as a parameter separator in the other column of a split block.
// On an editable branch the handle carries two triggers, hooks and block
// actions, told apart by the icon each one shows.
export async function openBlockMenu(
	page: Page,
	pane: Locator,
	block: Locator,
): Promise<Locator> {
	const trigger = pane.locator(
		'.z-drag-handle [data-slot="dropdown-menu-trigger"]',
		{ has: page.locator('[class*="dots-line"]') },
	)

	let attempt = 0
	await expect(async () => {
		const box = await block.boundingBox()
		expect(box).not.toBeNull()

		const height = HOVER_HEIGHTS[attempt % HOVER_HEIGHTS.length] ?? 0.5
		attempt += 1
		await block.hover({
			position: { x: (box?.width ?? 2) / 2, y: (box?.height ?? 2) * height },
		})
		await expect(trigger).toBeVisible({ timeout: 1_000 })
	}).toPass()
	await trigger.click()

	const menu = page.getByRole("menu")
	await expect(menu).toBeVisible()

	return menu
}

// readModeToggle is the navbar button switching the page between read and
// edit mode. Its accessible name is the mode the page is in.
export function readModeToggle(page: Page): Locator {
	return page.getByRole("button", {
		name: new RegExp(
			`^(${t("editor.navbar.toggle-edit-mode")}|${t("editor.navbar.toggle-read-mode")})$`,
		),
	})
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
