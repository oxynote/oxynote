import { expect, test, type Page } from "@playwright/test"
import { inviteTeamMember } from "../helpers/collaboration"
import { waitForEditor } from "../helpers/editor"
import { t } from "../helpers/i18n"
import {
	confirmKeycloakLogout,
	newIdpUser,
	type IdpUser,
} from "../helpers/keycloak"
import {
	activeMemberRole,
	keycloakLoginButton,
	logInThroughKeycloak,
	OIDC_SINGLE_WORKSPACE_URL,
	signUpThroughKeycloak,
} from "../helpers/oidc"
import { hydrated, visit } from "../helpers/page"

// the instance starts with no workspace and no admin, and the first user
// to sign in creates both. Every test here shares that one workspace, so
// they run in this order, and a retry would meet an admin who already
// exists, so there is none.
test.describe("a single workspace that only keycloak signs in to", () => {
	test.describe.configure({ mode: "serial", retries: 0 })

	let admin: IdpUser

	test("offers keycloak and no password on the login page", async ({
		page,
	}) => {
		await visit(page, `${OIDC_SINGLE_WORKSPACE_URL}/login`)

		await expect(keycloakLoginButton(page)).toBeVisible()
		await expect(
			page.getByRole("button", {
				name: t("onboarding.login.login-email-password"),
			}),
		).toHaveCount(0)
		await expect(page.locator('input[type="password"]')).toHaveCount(0)
	})

	test("makes the first user to sign in the workspace's admin", async ({
		page,
		request,
	}) => {
		admin = await newIdpUser(request, { emailVerified: true })
		await visit(page, `${OIDC_SINGLE_WORKSPACE_URL}/login`)

		await logInThroughKeycloak(page, admin)

		await expect(page).toHaveURL(/-[a-z0-9]{20}$/, { timeout: 30_000 })
		expect(
			await activeMemberRole(page.request, OIDC_SINGLE_WORKSPACE_URL),
		).toBe("admin")
	})

	test("refuses the next user without an invitation", async ({
		page,
		request,
	}) => {
		const stranger = await newIdpUser(request, { emailVerified: true })
		await visit(page, `${OIDC_SINGLE_WORKSPACE_URL}/login`)

		await logInThroughKeycloak(page, stranger)

		await expect(page).toHaveURL(
			(url) =>
				url.origin === OIDC_SINGLE_WORKSPACE_URL &&
				url.pathname === "/auth-error" &&
				url.searchParams.get("error") === "SIGN_UP_DISABLED",
			{ timeout: 15_000 },
		)
		await expect(
			page.getByText(t("oauth.error.sign-up-disabled")),
		).toBeVisible()

		// keycloak still has the refused user signed in, and only ending
		// that session lets someone else try.
		await hydrated(page)
		await page
			.getByRole("button", { name: t("oauth.error.other-account") })
			.click()
		await confirmKeycloakLogout(page)
		await expect(page).toHaveURL(`${OIDC_SINGLE_WORKSPACE_URL}/login`, {
			timeout: 15_000,
		})
	})

	test("lets an invited user join through keycloak with the invited role", async ({
		browser,
		request,
	}) => {
		// an invitation and two OIDC round trips.
		test.slow()

		const adminContext = await browser.newContext()
		const adminPage = await adminContext.newPage()
		await signInAsAdmin(adminPage, admin)
		const invitee = await newIdpUser(request, { emailVerified: true })
		const inviteLink = await inviteTeamMember(
			adminPage,
			request,
			invitee.email,
			"viewer",
		)

		const inviteeContext = await browser.newContext()
		const inviteePage = await inviteeContext.newPage()
		await visit(inviteePage, inviteLink)
		await inviteePage
			.getByRole("button", {
				name: t("onboarding.accept-invite.signup-button"),
			})
			.click()
		await expect(inviteePage).toHaveURL(/\/signup\?next=/)
		await signUpThroughKeycloak(inviteePage, invitee)
		await expect(inviteePage).toHaveURL(/\/accept-invite\?/, {
			timeout: 15_000,
		})
		await hydrated(inviteePage)
		await inviteePage
			.getByRole("button", {
				name: t("onboarding.accept-invite.accept-button"),
			})
			.click()

		await expect(inviteePage).toHaveURL(/-[a-z0-9]{20}$/, { timeout: 30_000 })
		expect(
			await activeMemberRole(inviteeContext.request, OIDC_SINGLE_WORKSPACE_URL),
		).toBe("viewer")
		await inviteeContext.close()
		await adminContext.close()
	})
})

async function signInAsAdmin(page: Page, admin: IdpUser): Promise<void> {
	await visit(page, `${OIDC_SINGLE_WORKSPACE_URL}/login`)
	await logInThroughKeycloak(page, admin)
	await expect(page).toHaveURL(/-[a-z0-9]{20}$/, { timeout: 30_000 })
	await waitForEditor(page)
}
