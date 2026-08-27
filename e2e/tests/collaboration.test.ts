import { expect, test } from "@playwright/test"
import { joinAsSecondUser } from "../helpers/collaboration"
import {
	contentEditor,
	createDocument,
	documentPersisted,
	editorText,
	remoteCarets,
	sidebarDocument,
	titleEditor,
	waitForEditor,
} from "../helpers/editor"
import { visit } from "../helpers/page"
import { signUpWithWorkspace } from "../helpers/workspace"

test.describe("collaboration", () => {
	// the heaviest setup in the suite: every test runs two signups, two
	// email verifications, an invitation and its acceptance before the
	// first assertion. slow() triples the budget for exactly these tests.
	test.beforeEach(() => {
		test.slow()
	})

	test("shows text typed by one user to the other", async ({
		page,
		request,
		browser,
	}) => {
		await signUpWithWorkspace(page, request)
		await createDocument(page)
		const other = await joinAsSecondUser(browser, page, request)

		await contentEditor(page).click()
		await page.keyboard.type("Typed by the owner")

		await expect
			.poll(() => editorText(contentEditor(other.page)))
			.toBe("Typed by the owner")

		await other.context.close()
	})

	test("merges edits made by both users", async ({
		page,
		request,
		browser,
	}) => {
		await signUpWithWorkspace(page, request)
		await createDocument(page)
		const other = await joinAsSecondUser(browser, page, request)

		await contentEditor(page).click()
		await page.keyboard.type("one")
		await expect.poll(() => editorText(contentEditor(other.page))).toBe("one")

		await contentEditor(other.page).click()
		await other.page.keyboard.press("End")
		await other.page.keyboard.type(" two")

		// both editors converge on the same text, whoever typed last
		await expect.poll(() => editorText(contentEditor(page))).toBe("one two")
		await expect
			.poll(() => editorText(contentEditor(other.page)))
			.toBe("one two")

		await other.context.close()
	})

	test("shows the other user's caret with their name", async ({
		page,
		request,
		browser,
	}) => {
		const { credentials } = await signUpWithWorkspace(page, request)
		await createDocument(page)
		const other = await joinAsSecondUser(browser, page, request)

		await contentEditor(page).click()
		await page.keyboard.type("hello")

		// each side sees the other user's caret under the other user's
		// name — display names default to the email's local part
		const ownCaret = remoteCarets(contentEditor(other.page))
		await expect(ownCaret).toHaveCount(1)
		await expect(ownCaret).toHaveText(credentials.email.split("@")[0] ?? "")

		await contentEditor(other.page).click()
		const otherCaret = remoteCarets(contentEditor(page))
		await expect(otherCaret).toHaveCount(1)
		await expect(otherCaret).toHaveText(
			other.credentials.email.split("@")[0] ?? "",
		)

		await other.context.close()
	})

	test("removes the caret when the other user leaves", async ({
		page,
		request,
		browser,
	}) => {
		await signUpWithWorkspace(page, request)
		await createDocument(page)
		const other = await joinAsSecondUser(browser, page, request)

		await contentEditor(other.page).click()
		await other.page.keyboard.type("present")
		await expect(remoteCarets(contentEditor(page))).toHaveCount(1)

		// leaving is a navigation away from the document: it unmounts the
		// editor, which tears the sync connection down and broadcasts the
		// awareness removal straight away. Closing the page instead is a
		// hard kill that skips the unload path entirely, so nothing is
		// broadcast and the caret survives until yjs times the stale
		// state out 30s later. The app also tears down on pagehide, for
		// the real tab close this cannot reproduce.
		await visit(other.page, "/")
		await other.context.close()

		await expect(remoteCarets(contentEditor(page))).toHaveCount(0, {
			timeout: 15_000,
		})
	})

	test("renames the page in the other user's sidebar", async ({
		page,
		request,
		browser,
	}) => {
		await signUpWithWorkspace(page, request)
		await createDocument(page)
		const other = await joinAsSecondUser(browser, page, request)

		await titleEditor(page).click()
		await page.keyboard.press("ControlOrMeta+A")
		await page.keyboard.type("Shared Title")

		await expect
			.poll(() => editorText(titleEditor(other.page)))
			.toBe("Shared Title")
		await expect(sidebarDocument(other.page, "Shared Title")).toBeVisible()

		await other.context.close()
	})

	test("keeps what both users wrote after they close", async ({
		page,
		request,
		browser,
	}) => {
		await signUpWithWorkspace(page, request)
		await createDocument(page)
		const url = page.url()
		const other = await joinAsSecondUser(browser, page, request)

		await contentEditor(page).click()
		await page.keyboard.type("first")
		await expect.poll(() => editorText(contentEditor(other.page))).toBe("first")
		await contentEditor(other.page).click()
		await other.page.keyboard.press("End")
		await other.page.keyboard.type(" second")
		await expect
			.poll(() => editorText(contentEditor(page)))
			.toBe("first second")

		await other.context.close()
		await documentPersisted(page)
		await visit(page, url)

		await waitForEditor(page)
		await expect
			.poll(() => editorText(contentEditor(page)))
			.toBe("first second")
	})
})
