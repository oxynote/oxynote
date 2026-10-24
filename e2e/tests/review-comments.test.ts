import {
	expect,
	test,
	type APIRequestContext,
	type Locator,
	type Page,
} from "@playwright/test"
import {
	commentedText,
	commentOnBlock,
	commentOnSelection,
	openTextComment,
	paneCommentPopover,
} from "../helpers/comments"
import { joinAsSecondUser } from "../helpers/collaboration"
import {
	contentEditor,
	contentPane,
	createDocument,
	diffEditor,
	diffPane,
	documentPersisted,
	editorText,
	openBlockMenu,
	openSlashMenu,
	placeCaret,
	selectText,
	waitForEditor,
} from "../helpers/editor"
import { t } from "../helpers/i18n"
import { visit } from "../helpers/page"
import {
	hideChanges,
	makeReviewable,
	showChanges,
	switchToBranch,
} from "../helpers/review"
import { signUpWithWorkspace } from "../helpers/workspace"

test.describe("comments on draft changes", () => {
	// every test builds a page, makes it reviewable, edits the draft and
	// waits for the edits to be stored before the first assertion. slow()
	// triples the budget for the whole flow.
	test.beforeEach(() => {
		test.slow()
	})

	test("files a comment on an added paragraph", async ({ page, request }) => {
		await draftWithParagraphChanges(page, request)

		const id = await commentOnBlock(
			page,
			diffPane(page),
			addedParagraph(page),
			"Is this paragraph needed?",
		)

		await expect(
			paneCommentPopover(diffPane(page)).getByText("Is this paragraph needed?"),
		).toBeVisible()
		await expect(
			diffEditor(page).locator(`p[data-node-comment-id="${id}"]`),
		).toHaveText("Added paragraph")
	})

	test("shows the added paragraph's comment on the draft once changes are hidden", async ({
		page,
		request,
	}) => {
		await draftWithParagraphChanges(page, request)
		const id = await commentOnBlock(
			page,
			diffPane(page),
			addedParagraph(page),
			"Reads well",
		)

		await hideChanges(page)

		await expect(
			contentEditor(page).locator(`p[data-node-comment-id="${id}"]`),
		).toHaveText("Added paragraph")

		await showChanges(page)

		await expect(
			diffEditor(page).locator(`p[data-node-comment-id="${id}"]`),
		).toHaveText("Added paragraph")
	})

	test("keeps the added paragraph's comment across a reload", async ({
		page,
		request,
	}) => {
		const url = await draftWithParagraphChanges(page, request)
		const id = await commentOnBlock(
			page,
			diffPane(page),
			addedParagraph(page),
			"Still here later",
		)
		await documentPersisted(page)

		await reopenDraftChanges(page, url)

		await diffEditor(page).locator(`p[data-node-comment-id="${id}"]`).click()
		await expect(
			paneCommentPopover(diffPane(page)).getByText("Still here later"),
		).toBeVisible()
	})

	test("files a comment on a removed paragraph", async ({ page, request }) => {
		await draftWithParagraphChanges(page, request)

		const id = await commentOnBlock(
			page,
			diffPane(page),
			removedParagraph(page),
			"Why was this dropped?",
		)

		await expect(
			paneCommentPopover(diffPane(page)).getByText("Why was this dropped?"),
		).toBeVisible()
		await expect(
			diffEditor(page).locator(`p[data-node-comment-id="${id}"]`),
		).toHaveText("Doomed paragraph")
	})

	test("keeps the removed paragraph's comment when changes are hidden and shown again", async ({
		page,
		request,
	}) => {
		await draftWithParagraphChanges(page, request)
		const id = await commentOnBlock(
			page,
			diffPane(page),
			removedParagraph(page),
			"Bring it back",
		)

		await hideChanges(page)
		await showChanges(page)

		await diffEditor(page).locator(`p[data-node-comment-id="${id}"]`).click()
		await expect(
			paneCommentPopover(diffPane(page)).getByText("Bring it back"),
		).toBeVisible()
	})

	test("keeps the removed paragraph's comment across a reload", async ({
		page,
		request,
	}) => {
		const url = await draftWithParagraphChanges(page, request)
		const id = await commentOnBlock(
			page,
			diffPane(page),
			removedParagraph(page),
			"Gone but discussed",
		)

		await reopenDraftChanges(page, url)

		await diffEditor(page).locator(`p[data-node-comment-id="${id}"]`).click()
		await expect(
			paneCommentPopover(diffPane(page)).getByText("Gone but discussed"),
		).toBeVisible()
	})

	test("files a comment on added text", async ({ page, request }) => {
		await draftWithParagraphChanges(page, request)
		await selectText(page, modifiedParagraph(page), "delta")

		const id = await commentOnSelection(page, diffPane(page), "Prefer gamma")

		await expect(
			paneCommentPopover(diffPane(page)).getByText("Prefer gamma"),
		).toBeVisible()
		await expect.poll(() => commentedText(diffPane(page), id)).toBe("delta")
	})

	test("shows the added text's comment on the draft once changes are hidden", async ({
		page,
		request,
	}) => {
		await draftWithParagraphChanges(page, request)
		await selectText(page, modifiedParagraph(page), "delta")
		const id = await commentOnSelection(page, diffPane(page), "Typo?")

		await hideChanges(page)

		await expect.poll(() => commentedText(contentPane(page), id)).toBe("delta")

		await showChanges(page)

		await expect.poll(() => commentedText(diffPane(page), id)).toBe("delta")
	})

	test("keeps the added text's comment across a reload", async ({
		page,
		request,
	}) => {
		const url = await draftWithParagraphChanges(page, request)
		await selectText(page, modifiedParagraph(page), "delta")
		const id = await commentOnSelection(page, diffPane(page), "Keep delta")
		await documentPersisted(page)

		await reopenDraftChanges(page, url)

		await expect.poll(() => commentedText(diffPane(page), id)).toBe("delta")
		await openTextComment(page, diffPane(page), id, "delta")
		await expect(
			paneCommentPopover(diffPane(page)).getByText("Keep delta"),
		).toBeVisible()
	})

	test("files a comment on removed text", async ({ page, request }) => {
		await draftWithParagraphChanges(page, request)
		await selectText(page, modifiedParagraph(page), "beta")

		const id = await commentOnSelection(page, diffPane(page), "Beta was fine")

		await expect(
			paneCommentPopover(diffPane(page)).getByText("Beta was fine"),
		).toBeVisible()
		await expect.poll(() => commentedText(diffPane(page), id)).toBe("beta")
	})

	test("keeps the removed text's comment when changes are hidden and shown again", async ({
		page,
		request,
	}) => {
		await draftWithParagraphChanges(page, request)
		await selectText(page, modifiedParagraph(page), "beta")
		const id = await commentOnSelection(page, diffPane(page), "Restore beta")

		await hideChanges(page)
		await showChanges(page)

		await expect.poll(() => commentedText(diffPane(page), id)).toBe("beta")
	})

	test("keeps the removed text's comment across a reload", async ({
		page,
		request,
	}) => {
		const url = await draftWithParagraphChanges(page, request)
		await selectText(page, modifiedParagraph(page), "beta")
		const id = await commentOnSelection(page, diffPane(page), "Beta again")

		await reopenDraftChanges(page, url)

		await expect.poll(() => commentedText(diffPane(page), id)).toBe("beta")
		await openTextComment(page, diffPane(page), id, "beta")
		await expect(
			paneCommentPopover(diffPane(page)).getByText("Beta again"),
		).toBeVisible()
	})

	test("files a comment spanning kept, removed and added text", async ({
		page,
		request,
	}) => {
		await draftWithParagraphChanges(page, request)
		await selectText(page, modifiedParagraph(page), "alpha betadelta gamma")

		const id = await commentOnSelection(page, diffPane(page), "Whole line")

		await expect
			.poll(() => commentedText(diffPane(page), id))
			.toBe("alpha betadelta gamma")
	})

	test("keeps the kept and added part of a spanning comment on the draft", async ({
		page,
		request,
	}) => {
		await draftWithParagraphChanges(page, request)
		await selectText(page, modifiedParagraph(page), "alpha betadelta gamma")
		const id = await commentOnSelection(page, diffPane(page), "Whole line")

		await hideChanges(page)

		await expect
			.poll(() => commentedText(contentPane(page), id))
			.toBe("alpha delta gamma")

		// the removed part is drawn back in once the changes are shown again
		await showChanges(page)

		await expect
			.poll(() => commentedText(diffPane(page), id))
			.toBe("alpha betadelta gamma")
	})

	test("keeps a spanning comment across a reload", async ({
		page,
		request,
	}) => {
		const url = await draftWithParagraphChanges(page, request)
		await selectText(page, modifiedParagraph(page), "alpha betadelta gamma")
		const id = await commentOnSelection(page, diffPane(page), "Whole line")
		await documentPersisted(page)

		await reopenDraftChanges(page, url)

		await expect
			.poll(() => commentedText(diffPane(page), id))
			.toBe("alpha betadelta gamma")
	})

	test("files a comment on text inside a removed paragraph", async ({
		page,
		request,
	}) => {
		await draftWithParagraphChanges(page, request)
		await selectText(page, removedParagraph(page), "Doomed")

		const id = await commentOnSelection(page, diffPane(page), "Harsh word")

		await expect(
			paneCommentPopover(diffPane(page)).getByText("Harsh word"),
		).toBeVisible()
		await expect.poll(() => commentedText(diffPane(page), id)).toBe("Doomed")
	})

	test("keeps a comment on text inside a removed paragraph across a reload", async ({
		page,
		request,
	}) => {
		const url = await draftWithParagraphChanges(page, request)
		await selectText(page, removedParagraph(page), "Doomed")
		const id = await commentOnSelection(page, diffPane(page), "Harsh word")

		await reopenDraftChanges(page, url)

		await expect.poll(() => commentedText(diffPane(page), id)).toBe("Doomed")
		await openTextComment(page, diffPane(page), id, "Doomed")
		await expect(
			paneCommentPopover(diffPane(page)).getByText("Harsh word"),
		).toBeVisible()
	})

	test("files a comment on a parameter added to a split block", async ({
		page,
		request,
	}) => {
		await draftWithSplitChanges(page, request)

		const id = await commentOnBlock(
			page,
			diffPane(page),
			addedParameter(page),
			"Document the default",
		)

		await expect(
			paneCommentPopover(diffPane(page)).getByText("Document the default"),
		).toBeVisible()
		await expect(commentedParameter(diffEditor(page), id)).toContainText(
			"limit",
		)
	})

	test("shows the added parameter's comment on the draft once changes are hidden", async ({
		page,
		request,
	}) => {
		await draftWithSplitChanges(page, request)
		const id = await commentOnBlock(
			page,
			diffPane(page),
			addedParameter(page),
			"Reads well",
		)

		await hideChanges(page)

		await expect(commentedParameter(contentEditor(page), id)).toContainText(
			"limit",
		)

		await showChanges(page)

		await expect(commentedParameter(diffEditor(page), id)).toContainText(
			"limit",
		)
	})

	test("keeps the added parameter's comment across a reload", async ({
		page,
		request,
	}) => {
		const url = await draftWithSplitChanges(page, request)
		const id = await commentOnBlock(
			page,
			diffPane(page),
			addedParameter(page),
			"Still here later",
		)
		await documentPersisted(page)

		await reopenDraftChanges(page, url)

		await commentedParameter(diffEditor(page), id).click()
		await expect(
			paneCommentPopover(diffPane(page)).getByText("Still here later"),
		).toBeVisible()
	})

	test("files a comment on a parameter removed from a split block", async ({
		page,
		request,
	}) => {
		await draftWithSplitChanges(page, request)

		const id = await commentOnBlock(
			page,
			diffPane(page),
			removedParameter(page),
			"Callers still send this",
		)

		await expect(
			paneCommentPopover(diffPane(page)).getByText("Callers still send this"),
		).toBeVisible()
		await expect(commentedParameter(diffEditor(page), id)).toContainText(
			"Identifier",
		)
	})

	test("keeps the removed parameter's comment across a reload", async ({
		page,
		request,
	}) => {
		const url = await draftWithSplitChanges(page, request)
		const id = await commentOnBlock(
			page,
			diffPane(page),
			removedParameter(page),
			"Gone but discussed",
		)

		await reopenDraftChanges(page, url)

		await commentedParameter(diffEditor(page), id).click()
		await expect(
			paneCommentPopover(diffPane(page)).getByText("Gone but discussed"),
		).toBeVisible()
	})

	test("files a comment on a code block added to a split block", async ({
		page,
		request,
	}) => {
		await draftWithSplitChanges(page, request)

		const id = await commentOnBlock(
			page,
			diffPane(page),
			addedCodeBlock(page),
			"Add a usage example",
		)

		await expect(
			paneCommentPopover(diffPane(page)).getByText("Add a usage example"),
		).toBeVisible()
		await expect(commentedCodeBlock(diffEditor(page), id)).toContainText(
			"Gamma",
		)
	})

	test("shows the added code block's comment on the draft once changes are hidden", async ({
		page,
		request,
	}) => {
		await draftWithSplitChanges(page, request)
		const id = await commentOnBlock(
			page,
			diffPane(page),
			addedCodeBlock(page),
			"Looks right",
		)

		await hideChanges(page)

		await expect(commentedCodeBlock(contentEditor(page), id)).toContainText(
			"Gamma",
		)

		await showChanges(page)

		await expect(commentedCodeBlock(diffEditor(page), id)).toContainText(
			"Gamma",
		)
	})

	test("keeps the added code block's comment across a reload", async ({
		page,
		request,
	}) => {
		const url = await draftWithSplitChanges(page, request)
		const id = await commentOnBlock(
			page,
			diffPane(page),
			addedCodeBlock(page),
			"Still here later",
		)
		await documentPersisted(page)

		await reopenDraftChanges(page, url)

		await commentedCodeBlock(diffEditor(page), id).click()
		await expect(
			paneCommentPopover(diffPane(page)).getByText("Still here later"),
		).toBeVisible()
	})

	test("files a comment on a code block removed from a split block", async ({
		page,
		request,
	}) => {
		await draftWithSplitChanges(page, request)

		const id = await commentOnBlock(
			page,
			diffPane(page),
			removedCodeBlock(page),
			"This one was useful",
		)

		await expect(
			paneCommentPopover(diffPane(page)).getByText("This one was useful"),
		).toBeVisible()
		await expect(commentedCodeBlock(diffEditor(page), id)).toContainText("Beta")
	})

	test("keeps the removed code block's comment across a reload", async ({
		page,
		request,
	}) => {
		const url = await draftWithSplitChanges(page, request)
		const id = await commentOnBlock(
			page,
			diffPane(page),
			removedCodeBlock(page),
			"Gone but discussed",
		)

		await reopenDraftChanges(page, url)

		await commentedCodeBlock(diffEditor(page), id).click()
		await expect(
			paneCommentPopover(diffPane(page)).getByText("Gone but discussed"),
		).toBeVisible()
	})

	test("files a comment on the changed description of a split block", async ({
		page,
		request,
	}) => {
		await draftWithSplitChanges(page, request)
		await selectText(page, modifiedParagraph(page), "thingsstuff")

		const id = await commentOnSelection(page, diffPane(page), "Pick one word")

		await expect
			.poll(() => commentedText(diffPane(page), id))
			.toBe("thingsstuff")

		await hideChanges(page)

		await expect.poll(() => commentedText(contentPane(page), id)).toBe("stuff")

		await showChanges(page)

		await expect
			.poll(() => commentedText(diffPane(page), id))
			.toBe("thingsstuff")
	})

	test("shows a comment on added text to a teammate viewing the changes", async ({
		page,
		request,
		browser,
	}) => {
		await draftWithParagraphChanges(page, request)
		const other = await joinAsSecondUser(browser, page, request)
		await switchToBranch(other.page, "draft")
		await showChanges(other.page)
		await selectText(page, modifiedParagraph(page), "delta")

		const id = await commentOnSelection(page, diffPane(page), "Seen by both")

		await expect
			.poll(() => commentedText(diffPane(other.page), id))
			.toBe("delta")
		await openTextComment(other.page, diffPane(other.page), id, "delta")
		await expect(
			paneCommentPopover(diffPane(other.page)).getByText("Seen by both"),
		).toBeVisible()

		await other.context.close()
	})

	test("shows a comment on a removed paragraph to a teammate viewing the changes", async ({
		page,
		request,
		browser,
	}) => {
		await draftWithParagraphChanges(page, request)
		const other = await joinAsSecondUser(browser, page, request)
		await switchToBranch(other.page, "draft")
		await showChanges(other.page)

		const id = await commentOnBlock(
			page,
			diffPane(page),
			removedParagraph(page),
			"Missed by the draft",
		)

		const commented = diffEditor(other.page).locator(
			`p[data-node-comment-id="${id}"]`,
		)
		await expect(commented).toHaveText("Doomed paragraph")
		await commented.click()
		await expect(
			paneCommentPopover(diffPane(other.page)).getByText("Missed by the draft"),
		).toBeVisible()

		await other.context.close()
	})

	test("shows a teammate's comment on removed text to the owner", async ({
		page,
		request,
		browser,
	}) => {
		await draftWithParagraphChanges(page, request)
		const other = await joinAsSecondUser(browser, page, request)
		await switchToBranch(other.page, "draft")
		await showChanges(other.page)
		await selectText(other.page, modifiedParagraph(other.page), "beta")

		const id = await commentOnSelection(
			other.page,
			diffPane(other.page),
			"Keep beta please",
		)

		await expect.poll(() => commentedText(diffPane(page), id)).toBe("beta")
		await openTextComment(page, diffPane(page), id, "beta")
		await expect(
			paneCommentPopover(diffPane(page)).getByText("Keep beta please"),
		).toBeVisible()

		await other.context.close()
	})
})

// the paragraphs a draft changes, as the diff shows them. The merged
// document marks each block with the change it carries.
function addedParagraph(page: Page): Locator {
	return diffEditor(page).locator('p[data-diff-status="added"]')
}

function removedParagraph(page: Page): Locator {
	return diffEditor(page).locator('p[data-diff-status="removed"]')
}

function modifiedParagraph(page: Page): Locator {
	return diffEditor(page).locator('p[data-diff-status="modified"]')
}

// draftWithParagraphChanges builds a page of three paragraphs, makes it
// reviewable and gives the draft one change of each kind: a paragraph
// added after the first, "beta" replaced by "delta" in the second and the
// third removed. It leaves the draft open with the changes shown and
// returns the page's url.
async function draftWithParagraphChanges(
	page: Page,
	request: APIRequestContext,
): Promise<string> {
	await signUpWithWorkspace(page, request)
	await createDocument(page)
	const url = page.url()
	await contentEditor(page).click()
	await page.keyboard.type("First paragraph")
	await page.keyboard.press("Enter")
	await page.keyboard.type("alpha beta gamma")
	await page.keyboard.press("Enter")
	await page.keyboard.type("Doomed paragraph")
	await documentPersisted(page)
	await makeReviewable(page)
	await switchToBranch(page, "draft")
	await expect
		.poll(() => editorText(contentEditor(page)))
		.toContain("Doomed paragraph")

	await placeCaret(draftText(page, "First paragraph"), "end")
	await page.keyboard.press("Enter")
	await page.keyboard.type("Added paragraph")
	await expect
		.poll(() => editorText(contentEditor(page)))
		.toContain("Added paragraph")

	await selectText(page, draftText(page, "alpha beta gamma"), "beta")
	await page.keyboard.type("delta")
	await expect
		.poll(() => editorText(contentEditor(page)))
		.toContain("alpha delta gamma")

	// the text goes first and the empty paragraph joins the one before it,
	// which removes the block without touching its neighbour
	await selectText(
		page,
		draftText(page, "Doomed paragraph"),
		"Doomed paragraph",
	)
	await page.keyboard.press("Backspace")
	await page.keyboard.press("Backspace")
	await expect
		.poll(() => editorText(contentEditor(page)))
		.not.toContain("Doomed")

	await documentPersisted(page)
	await showChanges(page)
	await expect(addedParagraph(page)).toHaveText("Added paragraph")
	await expect(removedParagraph(page)).toHaveText("Doomed paragraph")
	await expect(modifiedParagraph(page)).toHaveText("alpha betadelta gamma")

	return url
}

// reopenDraftChanges reloads the page, which lands on the main branch,
// and opens the draft with its changes shown again.
async function reopenDraftChanges(page: Page, url: string): Promise<void> {
	await visit(page, url)
	await waitForEditor(page)
	await switchToBranch(page, "draft")
	await showChanges(page)
}

// the parts of the split documentation block the draft changes, as the
// diff shows them. A parameter and a code block are found by the text
// they hold, since the change is marked on the block or on its children.
function addedParameter(page: Page): Locator {
	return diffEditor(page).locator(
		'[data-type="split-documentation-parameter-list-item"]',
		{ hasText: "Page size" },
	)
}

function removedParameter(page: Page): Locator {
	return diffEditor(page).locator(
		'[data-type="split-documentation-parameter-list-item"]',
		{ hasText: "Identifier" },
	)
}

function addedCodeBlock(page: Page): Locator {
	return diffEditor(page).locator('[data-type="titled-code-block"]', {
		hasText: "Gamma",
	})
}

function removedCodeBlock(page: Page): Locator {
	return diffEditor(page).locator('[data-type="titled-code-block"]', {
		hasText: "Beta",
	})
}

function commentedParameter(editor: Locator, id: string): Locator {
	return editor.locator(
		`[data-type="split-documentation-parameter-list-item"][data-node-comment-id="${id}"]`,
	)
}

function commentedCodeBlock(editor: Locator, id: string): Locator {
	return editor.locator(
		`[data-type="titled-code-block"][data-node-comment-id="${id}"]`,
	)
}

// draftWithSplitChanges builds a page holding one split documentation
// block — a description, two parameters and two code blocks — makes it
// reviewable and changes every part on the draft: a parameter and a code
// block are added, a parameter and a code block removed, and a word of
// the description replaced. It leaves the draft open with the changes
// shown and returns the page's url.
async function draftWithSplitChanges(
	page: Page,
	request: APIRequestContext,
): Promise<string> {
	await signUpWithWorkspace(page, request)
	await createDocument(page)
	const url = page.url()
	await contentEditor(page).click()
	const menu = await openSlashMenu(page)
	await page.keyboard.type("Split")
	await expect(menu.getByRole("button")).toHaveCount(1)
	await page.keyboard.press("Enter")

	// the block opens with the caret in its heading, and its description
	// is the line below
	await page.keyboard.type("Topic")
	await page.keyboard.press("ArrowDown")
	await page.keyboard.type("Describe things")
	await bottomAction(
		page,
		t("editor.split-documentation.left-side-bottom-action-button"),
	).click()
	await page.keyboard.type("Options")
	await page.keyboard.press("ArrowDown")
	await typeParameter(page, "id", "string", "Identifier")
	await page.keyboard.press("Enter")
	await typeParameter(page, "name", "string", "Display name")
	await addRightSideBlock(
		page,
		t("editor.split-documentation.right-side-bottom-action-buttons.code"),
	)
	await page.keyboard.type("Beta")
	await expect.poll(() => editorText(contentEditor(page))).toContain("Beta")
	await documentPersisted(page)
	await makeReviewable(page)
	await switchToBranch(page, "draft")
	await expect
		.poll(() => editorText(contentEditor(page)))
		.toContain("Display name")

	// Enter at the end of a parameter's description starts a new parameter
	await placeCaret(draftText(page, "Display name"), "end")
	await page.keyboard.press("Enter")
	await typeParameter(page, "limit", "int", "Page size")
	await expect
		.poll(() => editorText(contentEditor(page)))
		.toContain("Page size")

	// Backspace at the start of a parameter's name removes the parameter
	await placeCaret(draftText(page, "id"), "start")
	await page.keyboard.press("Backspace")
	await expect
		.poll(() => editorText(contentEditor(page)))
		.not.toContain("Identifier")

	await selectText(page, draftText(page, "Describe things"), "things")
	await page.keyboard.type("stuff")
	await expect
		.poll(() => editorText(contentEditor(page)))
		.toContain("Describe stuff")

	await addRightSideBlock(
		page,
		t("editor.split-documentation.right-side-bottom-action-buttons.code"),
	)
	await page.keyboard.type("Gamma")
	await expect.poll(() => editorText(contentEditor(page))).toContain("Gamma")

	await openBlockMenu(page, contentPane(page), draftText(page, "Beta"))
	await page
		.getByRole("menuitem", {
			name: t("editor.drag-handle.options.delete-block"),
		})
		.click()
	await expect.poll(() => editorText(contentEditor(page))).not.toContain("Beta")

	await documentPersisted(page)
	await showChanges(page)
	await expect(
		diffEditor(page).locator('[data-diff-status="added"]', {
			hasText: "Page size",
		}),
	).not.toHaveCount(0)
	await expect(
		diffEditor(page).locator('[data-diff-status="removed"]', {
			hasText: "Identifier",
		}),
	).not.toHaveCount(0)
	await expect(
		diffEditor(page).locator('[data-diff-status="added"]', {
			hasText: "Gamma",
		}),
	).not.toHaveCount(0)
	await expect(
		diffEditor(page).locator('[data-diff-status="removed"]', {
			hasText: "Beta",
		}),
	).not.toHaveCount(0)
	await expect(modifiedParagraph(page)).toHaveText("Describe thingsstuff")

	return url
}

// the split block's bottom actions: one adds a parameter list on the
// left, the other opens the menu of blocks to add on the right
function bottomAction(page: Page, name: string): Locator {
	return contentPane(page).getByRole("button", { name: name })
}

// the menu is teleported out of the content pane, so its entry is looked
// up on the whole page
async function addRightSideBlock(page: Page, name: string): Promise<void> {
	await bottomAction(
		page,
		t("editor.split-documentation.bottom-action-trigger-label"),
	).click()
	await page.getByRole("menuitem", { name: name }).click()
}

// typeParameter fills the parameter the caret has just been placed in:
// its name, then its type to the right, then its description below
async function typeParameter(
	page: Page,
	name: string,
	type: string,
	description: string,
): Promise<void> {
	await page.keyboard.type(name)
	await page.keyboard.press("ArrowRight")
	await page.keyboard.type(type)
	await page.keyboard.press("ArrowDown")
	await page.keyboard.type(description)
}

function draftText(page: Page, text: string): Locator {
	return contentEditor(page).getByText(text, { exact: true })
}
