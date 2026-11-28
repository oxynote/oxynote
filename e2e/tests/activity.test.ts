import { expect, test } from "@playwright/test"
import { activityLog, activityTypes, draftBranchId } from "../helpers/activity"
import { coreURL, defaultBranchId, documentId } from "../helpers/api"
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
import { authorizeMCPClient, connectMCPClient } from "../helpers/mcp"
import { visit } from "../helpers/page"
import {
	inviteReviewer,
	makeReviewable,
	mergeDraftIntoMain,
	reviewActions,
	switchToBranch,
} from "../helpers/review"
import { signUpWithWorkspace } from "../helpers/workspace"

// a persist lands two seconds after the last change, and an entry only
// exists once its persist is stored.
const PERSISTED = { timeout: 15_000 }

test.describe("activity log", () => {
	// signups, branch operations and the storage debounce add up well past
	// the default budget.
	test.beforeEach(() => {
		test.slow()
	})

	test("credits typed edits to the user and an MCP client's edit to the client", async ({
		page,
		request,
	}) => {
		await signUpWithWorkspace(page, request)
		await createDocument(page)
		const id = documentId(page)
		const branchId = await defaultBranchId(page, id)
		await contentEditor(page).click()
		await page.keyboard.type("Typed by hand")
		await documentPersisted(page)

		const client = await connectMCPClient(
			await authorizeMCPClient(page, request, "documents:read documents:write"),
		)
		const inserted = await client.callTool({
			name: "insert_blocks",
			arguments: {
				document_id: id,
				branch_id: branchId,
				position: "end",
				content: "<p>Written by an agent</p>",
			},
		})
		await client.close()
		expect(inserted.isError, JSON.stringify(inserted.content)).toBeFalsy()

		await expect
			.poll(async () => {
				const log = await activityLog(page, id, branchId, "?category=content")

				return log.entries.map((e) => e.actor.kind)
			}, PERSISTED)
			.toEqual(["mcp", "user"])

		const [agent, user] = (
			await activityLog(page, id, branchId, "?category=content")
		).entries
		expect(agent?.actor.clientId).toBeTruthy()
		expect(agent?.actor.userId).toBe(user?.actor.userId)
	})

	test("records a teammate's comment, a reply and the resolution, and no edit for the comment", async ({
		page,
		request,
		browser,
	}) => {
		await signUpWithWorkspace(page, request)
		await createDocument(page)
		const id = documentId(page)
		const branchId = await defaultBranchId(page, id)
		await contentEditor(page).click()
		await page.keyboard.type("Worth a look")
		await documentPersisted(page)
		const other = await joinAsSecondUser(browser, page, request)

		await addComment(other.page, "Opening thought")
		await documentPersisted(other.page)
		await contentEditor(page).locator(".comment-mark").click()
		await commentComposer(page).click()
		await page.keyboard.type("Agreed")
		await commentPopover(page)
			.getByRole("button", {
				name: t("editor.comment-thread.reply-button"),
				exact: true,
			})
			.click()
		await expect(commentPopover(page).getByText("Agreed")).toBeVisible()
		await commentPopover(page)
			.getByRole("button", {
				name: t("editor.comment-thread.resolve-button"),
				exact: true,
			})
			.click()

		await expect
			.poll(() => activityTypes(page, id, branchId, "?category=comment"))
			.toEqual(["comment.resolved", "comment.replied", "comment.created"])

		// the created entry carries the teammate's text as it was written
		const comments = await activityLog(page, id, branchId, "?category=comment")
		expect(JSON.stringify(comments.entries[2]?.subject)).toContain(
			"Opening thought",
		)

		// the comment mark the teammate wrote is the comment's change, so
		// the only edit is the owner's typing
		const edits = await activityLog(page, id, branchId, "?category=content")
		expect(edits.entries).toHaveLength(1)
		expect(edits.entries[0]?.actor.userId).not.toBe(
			comments.entries[2]?.actor.userId,
		)

		await other.context.close()
	})

	test("records a review and its merge on both branches", async ({
		page,
		request,
		browser,
	}) => {
		await signUpWithWorkspace(page, request)
		await createDocument(page)
		const id = documentId(page)
		const other = await joinAsSecondUser(browser, page, request)
		// the owner's member list predates the teammate, so a reload picks
		// them up before the reviewer invite looks for them
		await visit(page, page.url())
		await waitForEditor(page)
		await makeReviewable(page)
		const mainId = await defaultBranchId(page, id)
		const draftId = await draftBranchId(page, id)
		await switchToBranch(page, "draft")
		await contentEditor(page).click()
		await page.keyboard.type("Proposed change")
		await documentPersisted(page)
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
		await mergeDraftIntoMain(page)

		await expect(activityTypes(page, id, draftId)).resolves.toEqual([
			"branch.merged",
			"review.approved",
			"review.requested",
			"content.edited",
			"branch.forked",
		])
		// the merge's write-back is core's own, so main gains no edit
		await expect(activityTypes(page, id, mainId)).resolves.toEqual([
			"branch.merged",
			"branch.protected",
			"branch.forked",
			"document.created",
		])

		await other.context.close()
	})

	test("pages through a branch's log with the cursor", async ({
		page,
		request,
	}) => {
		await signUpWithWorkspace(page, request)
		await createDocument(page)
		const id = documentId(page)
		await makeReviewable(page)
		const branchId = await defaultBranchId(page, id)

		const first = await activityLog(page, id, branchId, "?limit=1")
		const second = await activityLog(
			page,
			id,
			branchId,
			`?limit=1&before=${first.next ?? ""}`,
		)
		const third = await activityLog(
			page,
			id,
			branchId,
			`?limit=1&before=${second.next ?? ""}`,
		)

		expect(first.entries.map((e) => e.type)).toEqual(["branch.protected"])
		expect(first.next).toBeTruthy()
		expect(second.entries.map((e) => e.type)).toEqual(["branch.forked"])
		expect(second.next).toBeTruthy()
		expect(third.entries.map((e) => e.type)).toEqual(["document.created"])
		expect(third.next).toBeNull()
	})

	test("refuses the log of a branch from another document", async ({
		page,
		request,
	}) => {
		// a new workspace opens on its welcome document, the other one here
		await signUpWithWorkspace(page, request)
		const welcome = documentId(page)
		const welcomeBranch = await defaultBranchId(page, welcome)
		await createDocument(page)
		const created = documentId(page)

		const response = await page.request.get(
			coreURL(`/api/documents/${created}/branches/${welcomeBranch}/activity`),
		)

		expect(response.status()).toBe(404)
	})
})
