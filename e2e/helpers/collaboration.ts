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

// MemberRole is a role a workspace member can hold.
export type MemberRole = "admin" | "editor" | "viewer"

// inviteTeamMember sends a workspace invitation from settings and hands
// back the accept link the invitee is emailed. The admin's page is left
// with every dialog closed, on whatever it was showing before. The
// invitation carries the given role, editor when none is given.
export async function inviteTeamMember(
	page: Page,
	request: APIRequestContext,
	email: string,
	role: MemberRole = "editor",
): Promise<string> {
	// the sidebar's next-steps entry is only offered while the admin is
	// alone in the workspace, which every fresh workspace's admin is
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
		.fill(email)

	// the dialog preselects editor.
	if (role !== "editor") {
		await page
			.getByRole("combobox", {
				name: t("settings.action-modals.workspace-invitation.role-label"),
			})
			.click()
		await page
			.getByRole("option", { name: t(`general.member-roles.${role}`) })
			.click()
	}

	await page
		.getByRole("button", {
			name: t("settings.action-modals.workspace-invitation.submit-button"),
		})
		.click()

	// the delivered email is the proof the invitation went through
	const inviteLink = await fetchInviteLink(request, email)

	// one Escape closes whichever dialog is on top, and how many are open
	// depends on whether the invite modal has finished closing itself yet
	// — under load it has not, so a single press leaves settings behind
	// and every assertion after this one runs against a covered page.
	// Press until nothing is left rather than assuming one is enough.
	await expect(async () => {
		await page.keyboard.press("Escape")
		await expect(page.getByRole("dialog")).toHaveCount(0, {
			timeout: 1_000,
		})
	}).toPass()

	return inviteLink
}

// joinAsSecondUser brings a second, distinct user into the first user's
// workspace and opens the same document in a fresh browser context: the
// admin sends an invitation from settings, and the invitee signs up,
// verifies, logs in and accepts it — the whole journey a real teammate
// takes, nothing granted behind the product's back.
//
// The admin's page ends up back on the document it was on, dialogs
// closed, so a test can continue driving it immediately. The second user
// joins with the given role, editor when none is given.
export async function joinAsSecondUser(
	browser: Browser,
	page: Page,
	request: APIRequestContext,
	role: MemberRole = "editor",
): Promise<{ context: BrowserContext; page: Page; credentials: Credentials }> {
	const credentials = newCredentials()
	const documentUrl = page.url()

	const inviteLink = await inviteTeamMember(
		page,
		request,
		credentials.email,
		role,
	)

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
