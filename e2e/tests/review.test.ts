import { expect, test } from "@playwright/test"
import { joinAsSecondUser } from "../helpers/collaboration"
import {
	contentEditor,
	createDocument,
	documentPersisted,
	editorText,
	titleEditor,
	waitForEditor,
} from "../helpers/editor"
import { t } from "../helpers/i18n"
import { openInbox } from "../helpers/inbox"
import { visit } from "../helpers/page"
import {
	branchLabel,
	branchSwitcher,
	inviteReviewer,
	makeReviewable,
	mergeDraftIntoMain,
	openReviewerPopover,
	reviewActions,
	switchToBranch,
} from "../helpers/review"
import { signUpWithWorkspace } from "../helpers/workspace"

test.describe("review workflow", () => {
	// every test here layers branch creation, protection flips and the
	// storage debounce on top of the usual signup-and-workspace setup —
	// and the teammate tests run a second signup and an invitation on
	// top of that. slow() triples the budget for the whole flow.
	test.beforeEach(() => {
		test.slow()
	})

	test("offers a main and a draft branch once the page is made reviewable", async ({
		page,
		request,
	}) => {
		await signUpWithWorkspace(page, request)
		await createDocument(page)

		await makeReviewable(page)

		await branchSwitcher(page).click()
		await expect(
			page.getByRole("menuitem", { name: branchLabel("main") }),
		).toBeVisible()
		await expect(
			page.getByRole("menuitem", { name: branchLabel("draft") }),
		).toBeVisible()
	})

	test("locks the main branch against direct edits", async ({
		page,
		request,
	}) => {
		await signUpWithWorkspace(page, request)
		await createDocument(page)

		await makeReviewable(page)

		await expect(contentEditor(page)).toHaveAttribute(
			"contenteditable",
			"false",
		)
		await expect(titleEditor(page)).toHaveAttribute("contenteditable", "false")
	})

	test("keeps the draft editable while the main branch is locked", async ({
		page,
		request,
	}) => {
		await signUpWithWorkspace(page, request)
		await createDocument(page)
		await makeReviewable(page)

		await switchToBranch(page, "draft")

		await expect(contentEditor(page)).toHaveAttribute("contenteditable", "true")

		await contentEditor(page).click()
		await page.keyboard.type("Written on the draft")

		await expect
			.poll(() => editorText(contentEditor(page)))
			.toBe("Written on the draft")
	})

	test("keeps draft edits out of the main branch until merged", async ({
		page,
		request,
	}) => {
		await signUpWithWorkspace(page, request)
		await createDocument(page)
		await contentEditor(page).click()
		await page.keyboard.type("Base text")
		await documentPersisted(page)
		await makeReviewable(page)

		await switchToBranch(page, "draft")
		await expect.poll(() => editorText(contentEditor(page))).toBe("Base text")
		await contentEditor(page).click()
		await page.keyboard.press("End")
		await page.keyboard.type(" plus a draft change")

		await switchToBranch(page, "main")

		await expect.poll(() => editorText(contentEditor(page))).toBe("Base text")
	})

	test("marks text added on the draft in the diff", async ({
		page,
		request,
	}) => {
		await signUpWithWorkspace(page, request)
		await createDocument(page)
		await contentEditor(page).click()
		await page.keyboard.type("Shared intro")
		await documentPersisted(page)
		await makeReviewable(page)
		await switchToBranch(page, "draft")
		await expect
			.poll(() => editorText(contentEditor(page)))
			.toBe("Shared intro")
		await contentEditor(page).click()
		await page.keyboard.press("End")
		await page.keyboard.type(" and-fresh-words")

		await page.locator("#show-diff").click()

		await expect(page.locator(".diff-editor .diff-text-added")).toContainText(
			"and-fresh-words",
		)
	})

	test("marks text removed from the draft in the diff", async ({
		page,
		request,
	}) => {
		await signUpWithWorkspace(page, request)
		await createDocument(page)
		await contentEditor(page).click()
		await page.keyboard.type("alpha omega")
		await documentPersisted(page)
		await makeReviewable(page)
		await switchToBranch(page, "draft")
		await expect.poll(() => editorText(contentEditor(page))).toBe("alpha omega")

		// deleting inside the same paragraph keeps the block identity, so
		// the diff renders it as modified rather than as a removed block
		// plus an added one. The caret is placed by clicking just past the
		// last character — the paragraph is a full-width block, so a click
		// near its right edge maps past the end of the line — and the
		// click is retried until focus sticks: a late re-render of the
		// freshly switched branch can swallow the first one.
		const paragraph = contentEditor(page).getByText("alpha omega")

		await expect(async () => {
			const box = await paragraph.boundingBox()
			expect(box).not.toBeNull()

			await paragraph.click({
				position: { x: (box?.width ?? 2) - 1, y: (box?.height ?? 2) / 2 },
			})
			await expect(contentEditor(page)).toBeFocused()
		}).toPass()

		// driven by the observed text rather than a fixed count of key
		// presses: right after a branch switch the editor can swallow the
		// first keystrokes while its collaboration plumbing settles, so
		// each round deletes one character and checks what is left
		await expect(async () => {
			await page.keyboard.press("Backspace")
			expect(await editorText(contentEditor(page))).toBe("alpha")
		}).toPass()

		await page.locator("#show-diff").click()

		await expect(page.locator(".diff-editor .diff-text-removed")).toContainText(
			"omega",
		)
	})

	test("shows the draft's title change in the diff", async ({
		page,
		request,
	}) => {
		await signUpWithWorkspace(page, request)
		await createDocument(page)
		await documentPersisted(page)
		await makeReviewable(page)
		await switchToBranch(page, "draft")

		await titleEditor(page).click()
		await page.keyboard.press("ControlOrMeta+A")
		await page.keyboard.type("Retitled Draft")

		await page.locator("#show-diff").click()

		// the word diff marks each changed word in a span of its own, so
		// the assertions pick one word per side
		await expect(
			page.locator(".diff-title .diff-text-added", { hasText: "Retitled" }),
		).toBeVisible()
		await expect(
			page.locator(".diff-title .diff-text-removed", { hasText: "Page" }),
		).toBeVisible()
	})

	test("merges the draft into the main branch", async ({ page, request }) => {
		await signUpWithWorkspace(page, request)
		await createDocument(page)
		await contentEditor(page).click()
		await page.keyboard.type("Base text")
		await documentPersisted(page)
		await makeReviewable(page)
		await switchToBranch(page, "draft")
		await expect.poll(() => editorText(contentEditor(page))).toBe("Base text")
		await contentEditor(page).click()
		await page.keyboard.press("End")
		await page.keyboard.type(" with the draft merged in")
		await documentPersisted(page)

		await mergeDraftIntoMain(page)

		await expect
			.poll(() => editorText(contentEditor(page)))
			.toBe("Base text with the draft merged in")
	})

	test("keeps the merged content after a reload", async ({ page, request }) => {
		await signUpWithWorkspace(page, request)
		await createDocument(page)
		const url = page.url()
		await contentEditor(page).click()
		await page.keyboard.type("Base text")
		await documentPersisted(page)
		await makeReviewable(page)
		await switchToBranch(page, "draft")
		await expect.poll(() => editorText(contentEditor(page))).toBe("Base text")
		await contentEditor(page).click()
		await page.keyboard.press("End")
		await page.keyboard.type(" survives the merge")
		await documentPersisted(page)
		await mergeDraftIntoMain(page)

		await visit(page, url)

		await waitForEditor(page)
		await expect
			.poll(() => editorText(contentEditor(page)))
			.toBe("Base text survives the merge")
	})

	test("records an approval and lists the approver", async ({
		page,
		request,
	}) => {
		const { credentials } = await signUpWithWorkspace(page, request)
		await createDocument(page)
		await makeReviewable(page)
		await switchToBranch(page, "draft")

		await reviewActions(page)
			.getByRole("button", {
				name: t("editor.name-editor.review-workflow.approve.title"),
				exact: true,
			})
			.click()

		await expect(
			page.getByText(t("editor.name-editor.review-workflow.approve.success")),
		).toBeVisible()

		const popover = await openReviewerPopover(page)
		await expect(
			popover.getByText(
				t("editor.name-editor.reviewer-popover.approved-by-label"),
			),
		).toBeVisible()
		await expect(
			popover.getByText(credentials.email.split("@")[0] ?? ""),
		).toBeVisible()
	})

	test("revokes an approval", async ({ page, request }) => {
		await signUpWithWorkspace(page, request)
		await createDocument(page)
		await makeReviewable(page)
		await switchToBranch(page, "draft")
		await reviewActions(page)
			.getByRole("button", {
				name: t("editor.name-editor.review-workflow.approve.title"),
				exact: true,
			})
			.click()
		await expect(
			page.getByText(t("editor.name-editor.review-workflow.approve.success")),
		).toBeVisible()

		await reviewActions(page)
			.getByRole("button", {
				name: t("editor.name-editor.review-workflow.unapprove.title"),
				exact: true,
			})
			.click()

		await expect(
			page.getByText(t("editor.name-editor.review-workflow.unapprove.success")),
		).toBeVisible()
		await expect(
			reviewActions(page).getByRole("button", {
				name: t("editor.name-editor.review-workflow.approve.title"),
				exact: true,
			}),
		).toBeVisible()
	})

	test("returns the page to a single editable branch when disabled", async ({
		page,
		request,
	}) => {
		await signUpWithWorkspace(page, request)
		await createDocument(page)
		await makeReviewable(page)

		await page
			.getByRole("button", { name: t("editor.navbar.open-document-options") })
			.click()
		await page
			.getByRole("menuitem", {
				name: t("editor.navbar.document-options.review-workflow.disable-title"),
			})
			.click()

		await expect(branchSwitcher(page)).toHaveCount(0)
		await expect(contentEditor(page)).toHaveAttribute("contenteditable", "true")
	})

	test("requests a review from a teammate", async ({
		page,
		request,
		browser,
	}) => {
		await signUpWithWorkspace(page, request)
		await createDocument(page)
		const other = await joinAsSecondUser(browser, page, request)
		// the owner's member list predates the teammate, so a reload picks
		// them up before the reviewer invite looks for them
		await visit(page, page.url())
		await waitForEditor(page)
		await makeReviewable(page)
		await switchToBranch(page, "draft")

		await inviteReviewer(page, other.credentials.email)

		const popover = page.locator('[data-slot="popover-content"]')
		await expect(
			popover.getByText(t("editor.name-editor.reviewer-popover.invited-label")),
		).toBeVisible()
		await expect(
			popover.getByText(other.credentials.email.split("@")[0] ?? ""),
		).toBeVisible()

		await other.context.close()
	})

	test("delivers the review request to the teammate's inbox", async ({
		page,
		request,
		browser,
	}) => {
		await signUpWithWorkspace(page, request)
		await createDocument(page)
		const other = await joinAsSecondUser(browser, page, request)
		// the owner's member list predates the teammate, so a reload picks
		// them up before the reviewer invite looks for them
		await visit(page, page.url())
		await waitForEditor(page)
		await makeReviewable(page)
		await switchToBranch(page, "draft")
		await inviteReviewer(page, other.credentials.email)

		await openInbox(other.page)

		await expect(
			other.page.getByText(
				t("notification.messages.document-review-request-description"),
			),
		).toBeVisible()

		await other.context.close()
	})

	test("lets an invited teammate approve the draft", async ({
		page,
		request,
		browser,
	}) => {
		await signUpWithWorkspace(page, request)
		await createDocument(page)
		const other = await joinAsSecondUser(browser, page, request)
		// the owner's member list predates the teammate, so a reload picks
		// them up before the reviewer invite looks for them
		await visit(page, page.url())
		await waitForEditor(page)
		await makeReviewable(page)
		await switchToBranch(page, "draft")
		await inviteReviewer(page, other.credentials.email)

		// the teammate's page predates the draft, so it reloads to pick
		// the new branch up before switching to it
		await visit(other.page, other.page.url())
		await waitForEditor(other.page)
		await switchToBranch(other.page, "draft")
		await reviewActions(other.page)
			.getByRole("button", {
				name: t("editor.name-editor.review-workflow.approve.title"),
				exact: true,
			})
			.click()
		await expect(
			other.page.getByText(
				t("editor.name-editor.review-workflow.approve.success"),
			),
		).toBeVisible()

		// the owner's reviewer list refreshes over the websocket, so the
		// approval arrives without any action on the owner's side
		const popover = page.locator('[data-slot="popover-content"]')
		await expect(
			popover.getByText(
				t("editor.name-editor.reviewer-popover.approved-by-label"),
			),
		).toBeVisible()
		await expect(
			popover.getByText(other.credentials.email.split("@")[0] ?? ""),
		).toBeVisible()

		await other.context.close()
	})

	test("withdraws a review invitation", async ({ page, request, browser }) => {
		await signUpWithWorkspace(page, request)
		await createDocument(page)
		const other = await joinAsSecondUser(browser, page, request)
		// the owner's member list predates the teammate, so a reload picks
		// them up before the reviewer invite looks for them
		await visit(page, page.url())
		await waitForEditor(page)
		await makeReviewable(page)
		await switchToBranch(page, "draft")
		await inviteReviewer(page, other.credentials.email)

		const popover = page.locator('[data-slot="popover-content"]')
		await popover
			.getByRole("button", {
				name: t(
					"editor.name-editor.reviewer-popover.invite-remove-screen-reader-hint",
				),
			})
			.click()

		await expect(
			page.getByText(
				t("editor.name-editor.reviewer-popover.invite-remove-success"),
			),
		).toBeVisible()
		await expect(
			popover.getByText(other.credentials.email.split("@")[0] ?? ""),
		).toHaveCount(0)

		await other.context.close()
	})
})
