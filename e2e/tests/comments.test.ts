import { expect, test } from "@playwright/test"
import { joinAsSecondUser } from "../helpers/collaboration"
import {
	addComment,
	commentComposer,
	commentOnBlock,
	commentPopover,
} from "../helpers/comments"
import {
	contentEditor,
	contentPane,
	createDocument,
	documentPersisted,
	waitForEditor,
} from "../helpers/editor"
import { t } from "../helpers/i18n"
import { openInbox } from "../helpers/inbox"
import { visit } from "../helpers/page"
import { signUpWithWorkspace } from "../helpers/workspace"

test.describe("comments", () => {
	test("adds a comment to selected text", async ({ page, request }) => {
		const { credentials } = await signUpWithWorkspace(page, request)
		await createDocument(page)
		await contentEditor(page).click()
		await page.keyboard.type("Needs a second pair of eyes")

		await addComment(page, "Please rephrase this")

		await expect(
			commentPopover(page).getByText("Please rephrase this"),
		).toBeVisible()
		await expect(
			commentPopover(page).getByText(credentials.email.split("@")[0] ?? ""),
		).toBeVisible()
		await expect(contentEditor(page).locator(".comment-mark")).toHaveText(
			"Needs a second pair of eyes",
		)
	})

	test("keeps a comment across a reload", async ({ page, request }) => {
		await signUpWithWorkspace(page, request)
		await createDocument(page)
		const url = page.url()
		await contentEditor(page).click()
		await page.keyboard.type("Commented text")
		await addComment(page, "Still here after a reload")
		await documentPersisted(page)

		await visit(page, url)

		await waitForEditor(page)
		await contentEditor(page).locator(".comment-mark").click()
		await expect(
			commentPopover(page).getByText("Still here after a reload"),
		).toBeVisible()
	})

	test("shows a comment to a collaborator", async ({
		page,
		request,
		browser,
	}) => {
		// two signups, a verification each, an invitation and its
		// acceptance run before the first assertion
		test.slow()

		const { credentials } = await signUpWithWorkspace(page, request)
		await createDocument(page)
		await contentEditor(page).click()
		await page.keyboard.type("Shared text for feedback")
		const other = await joinAsSecondUser(browser, page, request)

		await addComment(page, "Feedback for the team")

		await contentEditor(other.page).locator(".comment-mark").click()
		await expect(
			commentPopover(other.page).getByText("Feedback for the team"),
		).toBeVisible()
		await expect(
			commentPopover(other.page).getByText(
				credentials.email.split("@")[0] ?? "",
			),
		).toBeVisible()

		await other.context.close()
	})

	test("opens a teammate's comment from the inbox at the commented block", async ({
		page,
		request,
		browser,
	}) => {
		// two signups, a verification each, an invitation and its
		// acceptance run before the first assertion
		test.slow()

		await signUpWithWorkspace(page, request)
		await createDocument(page)
		const url = page.url()
		// the commented block sits more than a screen down, so only a scroll
		// to it brings it into view
		await contentEditor(page).click()
		for (let line = 0; line < 40; line += 1) {
			await page.keyboard.press("Enter")
		}
		await page.keyboard.type("Block under discussion")
		const other = await joinAsSecondUser(browser, page, request)
		// the owner's member list predates the teammate, so a reload picks
		// them up before the notification names them. It also leaves the
		// owner at the top of the page
		await visit(page, url)
		await waitForEditor(page)
		const block = contentEditor(other.page).getByText(
			"Block under discussion",
			{ exact: true },
		)
		const uid = (await block.getAttribute("id")) ?? ""
		await commentOnBlock(
			other.page,
			contentPane(other.page),
			block,
			"Worth a second look",
		)
		const linked = page.locator(`[id="${uid}"]`)
		await expect(linked).not.toBeInViewport()

		await openInbox(page)
		const notification = page.getByRole("link", {
			name: t("notification.messages.document-new-comment-description", {
				user: other.credentials.email.split("@")[0] ?? "",
			}),
		})
		await expect(notification).toContainText("Worth a second look")
		await notification.click()

		await expect(page).toHaveURL(
			new RegExp(`^${url}\\?branch=[a-z0-9]{20}#${uid}$`),
		)
		await expect(linked).toBeInViewport()

		await other.context.close()
	})

	test("tells the author in the inbox when a teammate resolves their comment", async ({
		page,
		request,
		browser,
	}) => {
		// two signups, a verification each, an invitation and its
		// acceptance run before the first assertion
		test.slow()

		await signUpWithWorkspace(page, request)
		await createDocument(page)
		await contentEditor(page).click()
		await page.keyboard.type("Soon to be settled")
		const other = await joinAsSecondUser(browser, page, request)
		// the owner's member list predates the teammate, so a reload picks
		// them up before the notification names them
		await visit(page, page.url())
		await waitForEditor(page)
		await addComment(page, "Handled already")

		await contentEditor(other.page).locator(".comment-mark").click()
		await commentPopover(other.page)
			.getByRole("button", {
				name: t("editor.comment-thread.resolve-button"),
				exact: true,
			})
			.click()

		await openInbox(page)
		await expect(
			page.getByRole("link", {
				name: t("notification.messages.document-comment-resolved-description", {
					user: other.credentials.email.split("@")[0] ?? "",
				}),
			}),
		).toContainText("Handled already")

		await other.context.close()
	})

	test("replies to a comment", async ({ page, request }) => {
		await signUpWithWorkspace(page, request)
		await createDocument(page)
		await contentEditor(page).click()
		await page.keyboard.type("Discussion starter")
		await addComment(page, "Opening take")

		await commentComposer(page).click()
		await page.keyboard.type("Seconded")
		await commentPopover(page)
			.getByRole("button", {
				name: t("editor.comment-thread.reply-button"),
				exact: true,
			})
			.click()

		await expect(commentPopover(page).getByText("Seconded")).toBeVisible()
	})

	test("removes the highlight when a comment is resolved", async ({
		page,
		request,
	}) => {
		await signUpWithWorkspace(page, request)
		await createDocument(page)
		await contentEditor(page).click()
		await page.keyboard.type("Soon to be settled")
		await addComment(page, "Handled already")

		await commentPopover(page)
			.getByRole("button", {
				name: t("editor.comment-thread.resolve-button"),
				exact: true,
			})
			.click()

		await expect(contentEditor(page).locator(".comment-mark")).toHaveCount(0)
	})
})
