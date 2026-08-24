import { expect, type Locator, type Page } from "@playwright/test"
import { contentEditor } from "./editor"
import { t } from "./i18n"

// commentPopover is the floating comment-thread panel. It is rendered
// inside the editor container rather than teleported, and it carries no
// dialog role.
export function commentPopover(page: Page): Locator {
	return page.locator(".content-editor div.z-popover")
}

// commentComposer is the tiptap instance inside the popover that new
// comment text is typed into. Saved comment bodies are read-only tiptap
// instances in the same popover, so the editable one is the composer.
export function commentComposer(page: Page): Locator {
	return commentPopover(page).locator('.ProseMirror[contenteditable="true"]')
}

// addComment selects the whole page body and files a comment on it
// through the bubble menu, leaving the thread popover open. Asserts the
// comment was saved: the highlight mark only trades its pending id for
// a real one once the server has answered.
export async function addComment(page: Page, text: string): Promise<void> {
	await contentEditor(page).click()
	await page.keyboard.press("ControlOrMeta+A")

	// the bubble menu mounts hidden and only gets data-visible once its
	// positioning has settled, so the attribute gates the click
	await page
		.locator(".content-editor [data-visible]")
		.getByRole("button", { name: t("editor.bubble-menu.comment-label") })
		.click()

	await commentComposer(page).click()
	await page.keyboard.type(text)
	await commentPopover(page)
		.getByRole("button", {
			name: t("editor.comment-thread.comment-button"),
			exact: true,
		})
		.click()

	await expect(
		contentEditor(page).locator(
			'[data-comment-id]:not([data-comment-id^="pending-"])',
		),
	).toBeVisible()
}
