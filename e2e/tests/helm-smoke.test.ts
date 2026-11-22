import { expect, test } from "@playwright/test"
import { BASE_URL } from "../helpers/config"
import {
	contentEditor,
	createDocument,
	editorText,
	waitForEditor,
} from "../helpers/editor"
import { visit } from "../helpers/page"
import { signUpWithWorkspaceWithoutEmail } from "../helpers/without-email"

// a realtime edit needs the Ingress controller or Gateway to pass the
// websocket on, which nothing else checks.
test.describe("the Helm chart behind a controller", () => {
	test("syncs an edit to a second tab", async ({ page, context }) => {
		await signUpWithWorkspaceWithoutEmail(page, BASE_URL)
		await createDocument(page)

		const other = await context.newPage()
		await visit(other, page.url())
		await waitForEditor(other)

		await contentEditor(page).click()
		await page.keyboard.type("Typed through the controller")

		await expect
			.poll(() => editorText(contentEditor(other)))
			.toBe("Typed through the controller")
	})
})
