import { readFileSync, writeFileSync } from "node:fs"
import { expect, test } from "@playwright/test"
import { submitLoginForm, type Credentials } from "../helpers/auth"
import { BASE_URL } from "../helpers/config"
import {
	contentEditor,
	createDocument,
	documentPersisted,
	editorText,
	waitForEditor,
} from "../helpers/editor"
import { visit } from "../helpers/page"
import { signUpWithWorkspaceWithoutEmail } from "../helpers/without-email"

// the second test depends on the first. e2e/helm/run.sh runs the first on
// the previous release, upgrades the release, then runs the second. The
// first hands over the account and the document in E2E_HELM_STATE.
const statePath = process.env.E2E_HELM_STATE ?? ""
const text = "Written before the upgrade"

interface State {
	credentials: Credentials
	url: string
}

test.describe("a Helm upgrade", () => {
	test("writes a document before the upgrade", async ({ page }) => {
		const { credentials } = await signUpWithWorkspaceWithoutEmail(
			page,
			BASE_URL,
		)
		await createDocument(page)

		await contentEditor(page).click()
		await page.keyboard.type(text)
		await expect.poll(() => editorText(contentEditor(page))).toBe(text)
		await documentPersisted(page)

		const state: State = { credentials, url: page.url() }
		writeFileSync(statePath, JSON.stringify(state))
	})

	test("finds the document after the upgrade", async ({ page }) => {
		const state = JSON.parse(readFileSync(statePath, "utf8")) as State

		await submitLoginForm(page, state.credentials)
		await expect(page).not.toHaveURL(/\/login/, { timeout: 15_000 })

		await visit(page, state.url)
		await waitForEditor(page)

		await expect.poll(() => editorText(contentEditor(page))).toBe(text)
	})
})
