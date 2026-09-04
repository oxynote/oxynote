import { randomUUID } from "node:crypto"
import { expect, test } from "@playwright/test"
import {
	contentEditor,
	createDocument,
	documentPersisted,
	sidebarDocument,
	titleEditor,
	waitForEditor,
} from "../helpers/editor"
import { t } from "../helpers/i18n"
import { documentId } from "../helpers/api"
import { authorizeMCPClient, connectMCPClient } from "../helpers/mcp"
import {
	branchLabel,
	branchSwitcher,
	makeReviewable,
	switchToBranch,
} from "../helpers/review"
import { visit } from "../helpers/page"
import { openSearch, searchFor } from "../helpers/search"
import { signUpWithWorkspace } from "../helpers/workspace"

test.describe("search", () => {
	// on top of the usual setup, a find waits out the storage debounce
	// and the ten-second search-indexing pass before the first hit can
	// appear. slow() triples the budget for that chain.
	test.beforeEach(() => {
		test.slow()
	})

	test("finds a page by its name", async ({ page, request }) => {
		await signUpWithWorkspace(page, request)
		await createDocument(page)
		await titleEditor(page).click()
		await page.keyboard.press("ControlOrMeta+A")
		await page.keyboard.type("Quarterly Roadmap")
		await expect(page).toHaveURL(/Quarterly-Roadmap-[a-z0-9]{20}$/)
		await documentPersisted(page)

		const dialog = await openSearch(page)
		const result = dialog.locator("a", { hasText: "Quarterly Roadmap" })
		await searchFor(dialog, "Quarterly", result)
		await result.click()

		await expect(page).toHaveURL(/Quarterly-Roadmap-[a-z0-9]{20}$/)
		await waitForEditor(page)
	})

	test("finds a page by its content", async ({ page, request }) => {
		await signUpWithWorkspace(page, request)
		await createDocument(page)
		await contentEditor(page).click()
		await page.keyboard.type("The xylophone brigade rehearses daily")
		await documentPersisted(page)
		// searching starts from another page, so that landing on the
		// found one is observable in the URL
		await sidebarDocument(page, "Welcome to Oxynote!").click()
		await expect(page).toHaveURL(/Welcome-to-Oxynote-[a-z0-9]{20}$/)

		const dialog = await openSearch(page)
		const result = dialog.locator("a", { hasText: "xylophone" })
		await searchFor(dialog, "xylophone", result)
		await result.click()

		await expect(page).toHaveURL(/New-Page-[a-z0-9]{20}/)
	})

	test("finds text written on a draft branch", async ({ page, request }) => {
		await signUpWithWorkspace(page, request)
		await createDocument(page)
		await documentPersisted(page)
		await makeReviewable(page)
		await switchToBranch(page, "draft")
		await contentEditor(page).click()
		await page.keyboard.type("The marmalade committee convenes")
		await documentPersisted(page)
		await sidebarDocument(page, "Welcome to Oxynote!").click()
		await expect(page).toHaveURL(/Welcome-to-Oxynote-[a-z0-9]{20}$/)

		const dialog = await openSearch(page)
		const result = dialog.locator("a", { hasText: "marmalade" })
		await searchFor(dialog, "marmalade", result)

		// the text exists on the draft alone, so that is the branch the
		// hit names and the one the link opens
		await expect(result.getByTestId("search-result-branch")).toHaveText("draft")
		await result.click()

		await expect(page).toHaveURL(/New-Page-[a-z0-9]{20}\?branch=[a-z0-9]{20}/)
		await expect(branchSwitcher(page)).toContainText(branchLabel("draft"), {
			timeout: 15_000,
		})
	})

	test("finds a metric block by its title", async ({ page, request }) => {
		// the welcome page is seeded with two metric blocks drawn at random
		// from a fixed pool, and one of those can carry this very title, so
		// the block under test is searched for by a marker of its own
		const marker = randomUUID().slice(0, 8)
		const title = `Pizza Fridays ${marker}`

		await signUpWithWorkspace(page, request)
		await createDocument(page)
		await documentPersisted(page)
		const id = documentId(page)

		// a metric block has no text to type; its title is configuration,
		// so the block goes in through the MCP surface the way an assistant
		// would write it
		const client = await connectMCPClient(
			await authorizeMCPClient(page, request, "documents:read documents:write"),
		)

		const listing = await client.callTool({
			name: "list_documents",
			arguments: {},
		})
		const listed = JSON.parse(
			(listing.content as { text: string }[])[0]?.text ?? "{}",
		) as { documents: { id: string; default_branch_id: string }[] }
		const branchId = listed.documents.find(
			(d) => d.id === id,
		)?.default_branch_id
		expect(branchId).toBeDefined()

		const inserted = await client.callTool({
			name: "insert_block",
			arguments: {
				document_id: id,
				branch_id: branchId,
				position: "end",
				// a metric lives in a grid; the root refuses a bare one
				block: {
					type: "metric_grid",
					items: [{ type: "metric", attrs: { title: title } }],
				},
			},
		})
		expect(inserted.isError, JSON.stringify(inserted.content)).toBeFalsy()
		await client.close()

		// the authorization walked the page off the app, and the search
		// starts from another page so that landing on the found one is
		// observable in the URL
		await visit(page, "/")
		await sidebarDocument(page, "Welcome to Oxynote!").click()
		await expect(page).toHaveURL(/Welcome-to-Oxynote-[a-z0-9]{20}$/)

		const dialog = await openSearch(page)
		const result = dialog.locator("a", { hasText: title })
		await searchFor(dialog, marker, result)
		await expect(result).toContainText("metricBlock")

		const href = await result.getAttribute("href")
		const uid = decodeURIComponent(href?.split("#")[1] ?? "")
		expect(uid).not.toBe("")

		await result.click()

		await expect(page).toHaveURL(/New-Page-[a-z0-9]{20}#/)
		await expect(page.locator(`[id="${uid}"]`)).toBeInViewport()
	})

	test("reports when nothing matches", async ({ page, request }) => {
		await signUpWithWorkspace(page, request)

		const dialog = await openSearch(page)
		await dialog
			.getByPlaceholder(t("sidebar.search.input-placeholder"))
			.fill("zz-no-such-page-anywhere")

		await expect(
			dialog.getByText(
				t("sidebar.search.no-results", {
					query: "zz-no-such-page-anywhere",
				}),
			),
		).toBeVisible()
	})
})
