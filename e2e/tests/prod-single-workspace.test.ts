import { expect, test } from "@playwright/test"
import { newCredentials, revealForm, submitLoginForm } from "../helpers/auth"
import { inviteTeamMember } from "../helpers/collaboration"
import { waitForEditor } from "../helpers/editor"
import { t } from "../helpers/i18n"
import {
	deliveredTo,
	fetchAccountDeletionLink,
	fetchVerificationLink,
} from "../helpers/mailpit"
import { visit } from "../helpers/page"
import {
	authPost,
	DEFAULT_ADMIN,
	DEFAULT_WORKSPACE_NAME,
	joinThroughInvitation,
	signedInContext,
	SINGLE_WORKSPACE_URL,
} from "../helpers/single-workspace"

// the password the admin moves to partway through the file.
const NEW_ADMIN_PASSWORD = "e2e-Admin-Passw0rd!-new"

// the instance has one workspace and one admin, and every test here shares
// them. The tests run in this order, read-only ones first, and later ones
// change the admin. A retry would rerun the group against an admin that is
// already changed, so there is none.
test.describe("a single-workspace instance", () => {
	test.describe.configure({ mode: "serial", retries: 0 })

	test("shows the default credentials on the login page, and they sign the admin in", async ({
		page,
	}) => {
		await visit(page, `${SINGLE_WORKSPACE_URL}/login`)

		await revealForm(
			page.getByRole("button", {
				name: t("onboarding.login.login-email-password"),
			}),
			page.getByText(t("onboarding.login.default-admin.intro")),
		)
		await expect(page.getByRole("listitem")).toHaveText([
			DEFAULT_ADMIN.email,
			DEFAULT_ADMIN.password,
		])

		await submitLoginForm(page, DEFAULT_ADMIN, SINGLE_WORKSPACE_URL)

		await expect(page).toHaveURL(/-[a-z0-9]{20}$/, { timeout: 15_000 })
		await expect(
			page.getByRole("button", { name: DEFAULT_WORKSPACE_NAME }),
		).toBeVisible()
	})

	test("sends a visitor from the signup page to the login page", async ({
		page,
	}) => {
		await visit(page, `${SINGLE_WORKSPACE_URL}/signup`)

		await expect(page).toHaveURL(`${SINGLE_WORKSPACE_URL}/login`)
	})

	test("refuses a signup without an invitation", async ({ request }) => {
		const credentials = newCredentials()

		const response = await authPost(request, "/sign-up/email", {
			email: credentials.email,
			password: credentials.password,
			name: "stranger",
		})

		expect(response.status()).toBe(400)
		expect(await response.json()).toMatchObject({
			code: "SIGN_UP_DISABLED",
		})
	})

	test("keeps the last member from deleting their account", async ({
		browser,
		request,
	}) => {
		const admin = await signedInContext(browser, DEFAULT_ADMIN)

		const response = await authPost(admin.request, "/delete-user", {
			callbackURL: `${SINGLE_WORKSPACE_URL}/signup`,
		})

		expect(response.status()).toBe(403)
		expect(await response.json()).toMatchObject({
			code: "LAST_ORGANIZATION_MEMBER",
		})
		expect(await deliveredTo(request, DEFAULT_ADMIN.email)).toBe(0)
		await admin.close()
	})

	test("lets an invited address join, and lets that member delete their account", async ({
		browser,
		request,
	}) => {
		// an invitation, a signup, a verification, a login and an account
		// deletion, each a cross-service round trip.
		test.slow()

		const admin = await signedInContext(browser, DEFAULT_ADMIN)
		const adminPage = await admin.newPage()
		await visit(adminPage, `${SINGLE_WORKSPACE_URL}/`)
		await waitForEditor(adminPage)
		const invitee = newCredentials()
		const inviteLink = await inviteTeamMember(adminPage, request, invitee.email)

		const inviteeContext = await browser.newContext()
		const inviteePage = await inviteeContext.newPage()
		await joinThroughInvitation(inviteePage, request, inviteLink, invitee)

		// with the admin still there, the member is not the last one.
		const deletion = await authPost(inviteeContext.request, "/delete-user", {
			callbackURL: `${SINGLE_WORKSPACE_URL}/signup`,
		})
		expect(deletion.ok()).toBe(true)
		await inviteePage.goto(
			await fetchAccountDeletionLink(request, invitee.email),
		)

		const signIn = await authPost(request, "/sign-in/email", {
			email: invitee.email,
			password: invitee.password,
		})
		expect(signIn.status()).toBe(401)
		await inviteeContext.close()
		await admin.close()
	})

	test("hides the default credentials once the admin changes the password", async ({
		browser,
		page,
	}) => {
		const admin = await signedInContext(browser, DEFAULT_ADMIN)
		const change = await authPost(admin.request, "/change-password", {
			currentPassword: DEFAULT_ADMIN.password,
			newPassword: NEW_ADMIN_PASSWORD,
		})
		expect(change.ok()).toBe(true)
		await admin.close()

		await visit(page, `${SINGLE_WORKSPACE_URL}/login`)
		await revealForm(
			page.getByRole("button", {
				name: t("onboarding.login.login-email-password"),
			}),
			page.getByPlaceholder(
				t("onboarding.login.email-password-form.email-placeholder"),
			),
		)

		await expect(
			page.getByText(t("onboarding.login.default-admin.intro")),
		).toHaveCount(0)
	})

	test("sends the admin's new email one link, which completes the change", async ({
		browser,
		request,
	}) => {
		const admin = await signedInContext(browser, {
			email: DEFAULT_ADMIN.email,
			password: NEW_ADMIN_PASSWORD,
		})
		const newEmail = newCredentials().email

		const change = await authPost(admin.request, "/change-email", {
			newEmail,
			callbackURL: `${SINGLE_WORKSPACE_URL}/`,
		})
		expect(change.ok()).toBe(true)
		const adminPage = await admin.newPage()
		await adminPage.goto(await fetchVerificationLink(request, newEmail))

		const signIn = await authPost(request, "/sign-in/email", {
			email: newEmail,
			password: NEW_ADMIN_PASSWORD,
		})
		expect(signIn.ok()).toBe(true)
		expect(await deliveredTo(request, newEmail)).toBe(1)
		expect(await deliveredTo(request, DEFAULT_ADMIN.email)).toBe(0)
		await admin.close()
	})
})
