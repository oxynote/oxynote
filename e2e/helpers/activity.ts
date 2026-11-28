import { expect, type Page } from "@playwright/test"
import { coreURL } from "./api"

// ActivityEntry is the part of a log entry the tests assert on.
interface ActivityEntry {
	type: string
	actor: { kind: string; userId: string | null; clientId: string | null }
	subject?: unknown
}

// ActivityPage is one page of a branch's activity log, newest first.
interface ActivityPage {
	entries: ActivityEntry[]
	next: string | null
}

// activityLog reads a page of a branch's activity log as the page's own
// user. The log has no screen yet, so its API is the product's surface.
export async function activityLog(
	page: Page,
	documentId: string,
	branchId: string,
	query = "",
): Promise<ActivityPage> {
	const response = await page.request.get(
		coreURL(
			`/api/documents/${documentId}/branches/${branchId}/activity${query}`,
		),
	)
	expect(response.ok(), await response.text()).toBe(true)

	return (await response.json()) as ActivityPage
}

// activityTypes reads the types of a branch's newest entries.
export async function activityTypes(
	page: Page,
	documentId: string,
	branchId: string,
	query = "",
): Promise<string[]> {
	const log = await activityLog(page, documentId, branchId, query)

	return log.entries.map((e) => e.type)
}

// draftBranchId asks core, as the page's own user, for the draft branch
// a reviewable document carries.
export async function draftBranchId(page: Page, id: string): Promise<string> {
	const response = await page.request.get(
		coreURL(`/api/documents/${id}/branches`),
	)
	expect(response.ok()).toBe(true)

	const branches = (await response.json()) as {
		branchId: string
		branchName: string
	}[]
	const branch = branches.find((b) => b.branchName === "draft")

	if (!branch) {
		throw new Error(`document ${id} has no draft branch`)
	}

	return branch.branchId
}
