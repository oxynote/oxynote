import { expect, type Page } from "@playwright/test"
import { BASE_URL } from "./config"

// coreURL builds an address for core's API through the front door. Caddy
// strips the /core prefix before proxying, so the path is written here
// exactly as core serves it.
export function coreURL(path: string): string {
	return `${BASE_URL}/core${path}`
}

// documentId reads the identifier out of the URL of an open document.
// Every document page ends in "<name-slug>-<id>", and the id is the
// trailing xid the routes elsewhere assert the shape of.
export function documentId(page: Page): string {
	const id = /-([a-z0-9]{20})$/.exec(page.url())?.[1]

	if (!id) {
		throw new Error(`no document id in the url ${page.url()}`)
	}

	return id
}

// defaultBranchId asks core, as the page's own user, which branch of the
// document a reader lands on without picking one.
export async function defaultBranchId(page: Page, id: string): Promise<string> {
	const response = await page.request.get(
		coreURL(`/api/documents/${id}/branches`),
	)
	expect(response.ok()).toBe(true)

	const branches = (await response.json()) as {
		branchId: string
		default: boolean
	}[]
	const branch = branches.find((b) => b.default)

	if (!branch) {
		throw new Error(`document ${id} has no default branch`)
	}

	return branch.branchId
}

// sessionCookie serialises a browser context's cookies into a request
// header, so a caller outside the browser can present the same session.
// Reading them while the session is live is the only way to still hold
// the cookie after the product has invalidated it.
export async function sessionCookie(page: Page): Promise<string> {
	const cookies = await page.context().cookies()

	return cookies.map((c) => `${c.name}=${c.value}`).join("; ")
}
