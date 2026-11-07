import { expect, test } from "@playwright/test"
import { joinAsSecondUser } from "../helpers/collaboration"
import { addComment, commentPopover } from "../helpers/comments"
import {
	contentEditor,
	createDocument,
	documentPersisted,
	editorText,
	placeCaret,
	waitForEditor,
} from "../helpers/editor"
import { t } from "../helpers/i18n"
import { visit } from "../helpers/page"
import {
	inviteReviewer,
	makeReviewable,
	reviewActions,
	switchToBranch,
} from "../helpers/review"
import { openSettings } from "../helpers/settings"
import { signUpWithWorkspace } from "../helpers/workspace"

test.describe("member roles", () => {
	// every test runs two signups, two email verifications, an invitation
	// and its acceptance before the first assertion.
	test.beforeEach(() => {
		test.slow()
	})

	test("shows a viewer the document and its comments but keeps the viewer's typing out", async ({
		page,
		request,
		browser,
	}) => {
		await signUpWithWorkspace(page, request)
		await createDocument(page)
		const url = page.url()
		await contentEditor(page).click()
		await page.keyboard.type("Written by the editor")
		await addComment(page, "Read this first")
		const viewer = await joinAsSecondUser(browser, page, request, "viewer")

		await expect
			.poll(() => editorText(contentEditor(viewer.page)))
			.toBe("Written by the editor")
		await contentEditor(viewer.page).locator(".comment-mark").click()
		await expect(
			commentPopover(viewer.page).getByText("Read this first"),
		).toBeVisible()

		await placeCaret(
			contentEditor(viewer.page)
				.getByRole("paragraph")
				.filter({ hasText: "Written by the editor" }),
			"end",
		)
		await viewer.page.keyboard.type(" and the viewer")
		await documentPersisted(viewer.page)

		// the server dropped the viewer's change, so a fresh load on either
		// side shows only the editor's text.
		await visit(page, url)
		await waitForEditor(page)
		expect(await editorText(contentEditor(page))).toBe("Written by the editor")

		await visit(viewer.page, url)
		await waitForEditor(viewer.page)
		expect(await editorText(contentEditor(viewer.page))).toBe(
			"Written by the editor",
		)

		await viewer.context.close()
	})

	test("lets a viewer approve a draft", async ({ page, request, browser }) => {
		await signUpWithWorkspace(page, request)
		await createDocument(page)
		const viewer = await joinAsSecondUser(browser, page, request, "viewer")
		// the owner's member list predates the viewer, so a reload picks
		// them up before the reviewer invite looks for them.
		await visit(page, page.url())
		await waitForEditor(page)
		await makeReviewable(page)
		await switchToBranch(page, "draft")
		await inviteReviewer(page, viewer.credentials.email)

		// the viewer's page predates the draft, so it reloads to pick the
		// new branch up before switching to it.
		await visit(viewer.page, viewer.page.url())
		await waitForEditor(viewer.page)
		await switchToBranch(viewer.page, "draft")
		await reviewActions(viewer.page)
			.getByRole("button", {
				name: t("editor.name-editor.review-workflow.approve.title"),
				exact: true,
			})
			.click()

		await expect(
			viewer.page.getByText(
				t("editor.name-editor.review-workflow.approve.success"),
			),
		).toBeVisible()

		await viewer.context.close()
	})

	test("turns a demoted editor read-only without a reload", async ({
		page,
		request,
		browser,
	}) => {
		const { workspace } = await signUpWithWorkspace(page, request)
		await createDocument(page)
		const url = page.url()
		const editor = await joinAsSecondUser(browser, page, request)
		await contentEditor(editor.page).click()
		await editor.page.keyboard.type("From the editor")
		await expect
			.poll(() => editorText(contentEditor(page)))
			.toBe("From the editor")

		// the owner's member list predates the editor joining, so a reload
		// picks them up as a member rather than a pending invitation.
		await visit(page, url)
		await waitForEditor(page)
		const settings = await openSettings(page, workspace.name)
		const row = settings.locator("tr", { hasText: editor.credentials.email })
		await row
			.getByRole("button", {
				name: t("settings.workspace.members-option-button-screen-reader-hint"),
			})
			.click()
		await page
			.getByRole("menuitem", {
				name: t("settings.workspace.member-options.role.title"),
			})
			.click()
		await page
			.getByRole("menuitemradio", { name: t("general.member-roles.viewer") })
			.click()
		await expect(row).toContainText(t("general.member-roles.viewer"))
		await expect(async () => {
			await page.keyboard.press("Escape")
			await expect(page.getByRole("dialog")).toHaveCount(0, {
				timeout: 1_000,
			})
		}).toPass()

		// the owner's next change reaching the demoted user proves their
		// connection came back, now read-only.
		await placeCaret(
			contentEditor(page)
				.getByRole("paragraph")
				.filter({ hasText: "From the editor" }),
			"end",
		)
		await page.keyboard.type(" and the owner")
		await expect
			.poll(() => editorText(contentEditor(editor.page)))
			.toBe("From the editor and the owner")

		await placeCaret(
			contentEditor(editor.page)
				.getByRole("paragraph")
				.filter({ hasText: "From the editor and the owner" }),
			"end",
		)
		await editor.page.keyboard.type(" after demotion")
		await documentPersisted(editor.page)

		await visit(page, url)
		await waitForEditor(page)
		expect(await editorText(contentEditor(page))).toBe(
			"From the editor and the owner",
		)

		await editor.context.close()
	})
})
