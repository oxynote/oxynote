import { expect, type Page } from "@playwright/test"
import { newCredentials, submitSignupForm, type Credentials } from "./auth"
import { waitForEditor } from "./editor"
import { newWorkspace, submitWorkspaceForm, type Workspace } from "./workspace"

// the prod stack's second instance of the image, started with no email
// sender (oxynote-without-email in docker-compose.prod.yaml). Changing the
// port means changing that service.
export const WITHOUT_EMAIL_URL = "http://localhost:19081"

// signUpWithWorkspaceWithoutEmail is signUpWithWorkspace for an instance
// with no email sender, by default the one above. Signup signs the new
// account straight in, so there is no link to follow and no login step
// before the workspace is created.
export async function signUpWithWorkspaceWithoutEmail(
	page: Page,
	origin = WITHOUT_EMAIL_URL,
): Promise<{ credentials: Credentials; workspace: Workspace }> {
	const credentials = newCredentials()
	await submitSignupForm(page, credentials, origin)
	await expect(page).toHaveURL(`${origin}/welcome`, {
		timeout: 15_000,
	})

	const workspace = newWorkspace()
	await submitWorkspaceForm(page, workspace)

	await expect(page).toHaveURL(/-[a-z0-9]{20}$/, { timeout: 30_000 })
	await waitForEditor(page)

	return { credentials, workspace }
}
