import { expect, test } from "@playwright/test"
import { joinAsSecondUser } from "../helpers/collaboration"
import { sidebarDocument } from "../helpers/editor"
import { signUpWithWorkspace } from "../helpers/workspace"

test.describe("invitation", () => {
	test("takes an invited teammate into the shared workspace", async ({
		page,
		request,
		browser,
	}) => {
		// two signups, a verification each, an invitation and its
		// acceptance run before the first assertion
		test.slow()

		const { workspace } = await signUpWithWorkspace(page, request)

		const other = await joinAsSecondUser(browser, page, request)

		await expect(other.page).toHaveURL(new RegExp(`/${workspace.slug}/`))
		await expect(
			sidebarDocument(other.page, "Welcome to Oxynote!"),
		).toBeVisible()

		await other.context.close()
	})
})
