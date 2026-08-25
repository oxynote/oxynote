import { randomUUID } from "node:crypto"
import {
	expect,
	type APIRequestContext,
	type Browser,
	type BrowserContext,
	type Page,
} from "@playwright/test"
import { type Credentials, signUpAndVerify, submitLoginForm } from "./auth"
import { waitForEditor } from "./editor"
import { t } from "./i18n"

export interface Workspace {
	name: string
	slug: string
}

// newWorkspace mints a workspace nobody else uses. The slug is what the
// server checks for uniqueness, so it carries the random part; the name
// only has to pass validation, which allows no spaces.
export function newWorkspace(): Workspace {
	return {
		name: "Acme-Corp",
		slug: `ws-${randomUUID().slice(0, 8)}`,
	}
}

// submitWorkspaceForm fills the onboarding form and leaves the browser
// wherever the app sends it. The URL field cannot be reached by label —
// the prefix wrapper breaks the label association — so it is addressed
// by its form name.
export async function submitWorkspaceForm(
	page: Page,
	workspace: Workspace,
): Promise<void> {
	await page
		.getByLabel(t("onboarding.welcome.form.workspace-name.label"))
		.fill(workspace.name)
	await page.locator('input[name="workspaceSlug"]').fill(workspace.slug)
	await page
		.getByRole("button", { name: t("onboarding.welcome.create-workspace") })
		.click()
}

// signUpWithWorkspace is the starting point for every test that needs a
// signed-in user with somewhere to work: a verified account, logged in,
// with a workspace whose welcome page is open in the editor.
//
// Creating the workspace is the slow step. After the organization is
// created, core seeds its welcome document and the page polls for it,
// so the landing URL can take a while to appear.
export async function signUpWithWorkspace(
	page: Page,
	request: APIRequestContext,
): Promise<{ credentials: Credentials; workspace: Workspace }> {
	const credentials = await signUpAndVerify(page, request)
	await submitLoginForm(page, credentials)
	// the login redirect refetches the session and organization before it
	// moves, another cross-service round trip that stretches under load
	await expect(page).toHaveURL(/\/welcome$/, { timeout: 15_000 })

	const workspace = newWorkspace()
	await submitWorkspaceForm(page, workspace)

	await expect(page).toHaveURL(/-[a-z0-9]{20}$/, { timeout: 30_000 })
	await waitForEditor(page)

	return { credentials, workspace }
}

// signUpWithSeparateWorkspace builds a second tenant: another verified
// user with a workspace of their own, in a browser context of their own.
// Nothing links it to the caller's workspace — no invitation, no shared
// member — which is what makes it the far side of a cross-tenant test.
export async function signUpWithSeparateWorkspace(
	browser: Browser,
	request: APIRequestContext,
): Promise<{
	context: BrowserContext
	page: Page
	credentials: Credentials
	workspace: Workspace
}> {
	const context = await browser.newContext()
	const page = await context.newPage()
	const { credentials, workspace } = await signUpWithWorkspace(page, request)

	return { context, page, credentials, workspace }
}
