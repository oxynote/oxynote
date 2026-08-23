import {
	expect,
	type APIRequestContext,
	type Browser,
	type BrowserContext,
	type Page,
} from "@playwright/test"
import {
	newCredentials,
	signUpAndVerify,
	submitLoginForm,
	type Credentials,
} from "./auth"
import { waitForEditor } from "./editor"
import { t } from "./i18n"
import { fetchInviteLink } from "./mailpit"
import { visit } from "./page"

// joinAsSecondUser brings a second, distinct user into the first user's
// workspace and opens the same document in a fresh browser context: the
// owner sends an invitation from settings, and the invitee signs up,
// verifies, logs in and accepts it — the whole journey a real teammate
// takes, nothing granted behind the product's back.
//
// The owner's page ends up back on the document it was on, dialogs
// closed, so a test can continue driving it immediately.
export async function joinAsSecondUser(
	browser: Browser,
	page: Page,
	request: APIRequestContext,
): Promise<{ context: BrowserContext; page: Page; credentials: Credentials }> {
	const credentials = newCredentials()
	const documentUrl = page.url()

	// the sidebar's next-steps entry is only offered while the owner is
	// alone in the workspace, which every fresh workspace's owner is
	await page
		.getByText(t("sidebar.sections.next-steps.items.invite-team-members"))
		.click()
	await page
		.getByRole("button", { name: t("settings.workspace.invitation-button") })
		.click()
	await page
		.getByPlaceholder(
			t("settings.action-modals.workspace-invitation.email-placeholder"),
		)
		.fill(credentials.email)
	await page
		.getByRole("button", {
			name: t("settings.action-modals.workspace-invitation.submit-button"),
		})
		.click()

	// the delivered email is the proof the invitation went through; the
	// invite modal has closed itself by then, leaving only settings
	const inviteLink = await fetchInviteLink(request, credentials.email)

	await page.keyboard.press("Escape")
	await expect(page.getByRole("dialog")).toHaveCount(0)

	const context = await browser.newContext()
	const invitee = await context.newPage()
	await signUpAndVerify(invitee, request, credentials)
	await submitLoginForm(invitee, credentials)
	// a fresh account has no workspace yet, so the login redirect lands
	// on onboarding — the invitation is what gets them out of it
	await expect(invitee).toHaveURL(/\/welcome$/, { timeout: 15_000 })

	await visit(invitee, inviteLink)
	await invitee
		.getByRole("button", { name: t("onboarding.accept-invite.accept-button") })
		.click()
	await expect(invitee).toHaveURL(/-[a-z0-9]{20}$/, { timeout: 30_000 })

	await visit(invitee, documentUrl)
	await waitForEditor(invitee)

	return { context, page: invitee, credentials }
}
