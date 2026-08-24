import { expect, test } from "@playwright/test"
import { joinAsSecondUser } from "../helpers/collaboration"
import {
	addComment,
	commentComposer,
	commentPopover,
} from "../helpers/comments"
import {
	contentEditor,
	createDocument,
	documentPersisted,
	waitForEditor,
} from "../helpers/editor"
import { t } from "../helpers/i18n"
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
