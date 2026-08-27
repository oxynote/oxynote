import { expect, type Locator, type Page } from "@playwright/test"
import { contentEditor, waitForEditor } from "./editor"
import { t } from "./i18n"

// the two branches a reviewable page carries. They are the branch names
// the product itself stores — core seeds "main" and the review workflow
// adds "draft".
type BranchName = "main" | "draft"

// branchLabel is the label the navbar shows for a branch. Every branch
// name a test asserts comes through here, so the message key — which
// still spells the concept as a document mode — is written once.
export function branchLabel(branch: BranchName): string {
	return t(`editor.navbar.document-modes.${branch}.title`)
}

// branchSwitcher is the navbar dropdown trigger naming the open branch
// of a reviewable page. Non-reviewable pages render the read/edit toggle
// in its place, so its presence is what proves reviewability.
export function branchSwitcher(page: Page): Locator {
	return page.getByRole("button", {
		name: new RegExp(`^(${branchLabel("main")}|${branchLabel("draft")})$`),
	})
}

// makeReviewable turns the open page into a reviewable one through the
// navbar options menu, which creates the draft branch and protects main.
// The page's current content must already be persisted if the draft is
// expected to start from it: the draft is seeded from the stored
// content, not the live document.
export async function makeReviewable(page: Page): Promise<void> {
	await page
		.getByRole("button", { name: t("editor.navbar.open-document-options") })
		.click()
	await page
		.getByRole("menuitem", {
			name: t("editor.navbar.document-options.review-workflow.enable-title"),
		})
		.click()

	// enabling the workflow forks the page server-side — the draft branch
	// is created, the stored content copied into it and main protected —
	// so the switcher only appears once that whole chain has come back.
	await expect(branchSwitcher(page)).toContainText(branchLabel("main"), {
		timeout: 15_000,
	})
}

// switchToBranch opens the given branch of a reviewable page through the
// navbar dropdown and waits for its editor to load. For the draft it
// also waits for the editor to become editable: the flip arrives with
// the branch data a beat after the switch, and keystrokes sent before it
// land in a read-only editor and are dropped.
export async function switchToBranch(
	page: Page,
	branch: BranchName,
): Promise<void> {
	await branchSwitcher(page).click()
	await page.getByRole("menuitem", { name: branchLabel(branch) }).click()

	// the switch fetches the branch and re-syncs the document over the
	// websocket before the switcher settles on the new name.
	await expect(branchSwitcher(page)).toContainText(branchLabel(branch), {
		timeout: 15_000,
	})
	await waitForEditor(page)

	if (branch === "draft") {
		await expect(contentEditor(page)).toHaveAttribute("contenteditable", "true")
	}
}

// reviewActions is the split button carrying the draft's approve and
// merge actions. It only renders while the draft branch is open.
export function reviewActions(page: Page): Locator {
	return page.locator('[data-slot="button-group"]')
}

// mergeDraftIntoMain merges the open draft into main and waits for the
// automatic switch back to the main branch. The merge action first has
// to be picked in the split button's dropdown half, which has no label
// of its own and is reached as the group's menu-opening button. The
// draft's latest changes must already be persisted: the merge reads the
// stored draft content.
export async function mergeDraftIntoMain(page: Page): Promise<void> {
	const title = t("editor.name-editor.review-workflow.merge.title")

	await reviewActions(page).locator('button[aria-haspopup="menu"]').click()
	await page.getByRole("menuitem", { name: title }).click()
	await reviewActions(page)
		.getByRole("button", { name: title, exact: true })
		.click()

	await expect(
		page.getByText(t("editor.name-editor.review-workflow.merge.success")),
	).toBeVisible({ timeout: 15_000 })
	// merging writes the draft back into main and drops the draft, so the
	// switcher settles only after that has been stored and reloaded.
	await expect(branchSwitcher(page)).toContainText(branchLabel("main"), {
		timeout: 15_000,
	})
	await waitForEditor(page)
}

// openReviewerPopover opens the reviewers popover from its icon stack
// next to the page title and returns the popover content, which is
// teleported out of the page's main tree.
export async function openReviewerPopover(page: Page): Promise<Locator> {
	await page
		.getByText(t("editor.name-editor.reviewers"), { exact: true })
		.click()

	const popover = page.locator('[data-slot="popover-content"]')
	await expect(popover).toBeVisible()

	return popover
}

// inviteReviewer requests a review from the given workspace member
// through the reviewers popover, and leaves the popover open. Asserts
// the request was accepted: the success toast only shows once the
// server has answered.
export async function inviteReviewer(page: Page, email: string): Promise<void> {
	const popover = await openReviewerPopover(page)

	await popover
		.getByRole("button", {
			name: t("editor.name-editor.reviewer-popover.invite-button"),
		})
		.click()
	await page.getByRole("menuitem", { name: email }).click()

	await expect(
		page.getByText(
			t("editor.name-editor.reviewer-popover.request-review-success.title"),
		),
	).toBeVisible()
}
