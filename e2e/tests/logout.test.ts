import { expect, test } from "@playwright/test"
import { t } from "../helpers/i18n"
import { signUpWithWorkspace } from "../helpers/workspace"

test.describe("logout", () => {
	test("returns to the login page and locks the workspace", async ({
		page,
		request,
	}) => {
		const { workspace } = await signUpWithWorkspace(page, request)
		const url = page.url()

		await page.getByRole("button", { name: workspace.name }).click()
		await page
			.getByRole("menuitem", { name: t("sidebar.header.log-out") })
			.click()

		await expect(page).toHaveURL(/\/login$/, { timeout: 15_000 })

		// the session is gone server-side, so the old page URL no longer
		// opens
		await page.goto(url)
		await expect(page).toHaveURL(/\/login/, { timeout: 15_000 })
	})
})
