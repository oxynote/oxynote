import { expect, test } from "@playwright/test"
import { OIDC_URL } from "../helpers/config"
import { t } from "../helpers/i18n"
import { newIdpUser } from "../helpers/keycloak"
import { fetchVerificationLink } from "../helpers/mailpit"
import {
	keycloakLoginButton,
	linkedProviders,
	logInThroughKeycloak,
	signUpAndVerifyByEmail,
} from "../helpers/oidc"
import { hydrated, visit } from "../helpers/page"

// every test here makes its own keycloak user and its own accounts, so
// they run in parallel on the one OIDC instance.
test.describe("signing in through keycloak", () => {
	test("creates the account of a new user and opens onboarding", async ({
		page,
		request,
	}) => {
		const user = await newIdpUser(request, { emailVerified: true })
		await visit(page, `${OIDC_URL}/login`)

		await logInThroughKeycloak(page, user)

		await expect(page).toHaveURL(`${OIDC_URL}/welcome`, { timeout: 15_000 })
		expect(await linkedProviders(page.request, OIDC_URL)).toEqual(["oidc"])
	})

	test("sends a new user with an unverified email a verification link first", async ({
		page,
		request,
	}) => {
		const user = await newIdpUser(request, { emailVerified: false })
		await visit(page, `${OIDC_URL}/login`)

		await logInThroughKeycloak(page, user)

		await expect(page).toHaveURL(
			(url) =>
				url.origin === OIDC_URL &&
				url.pathname === "/auth-error" &&
				url.searchParams.get("error") === "email_not_verified",
			{ timeout: 15_000 },
		)
		await expect(
			page.getByText(t("oauth.error.email-not-verified")),
		).toBeVisible()
		expect(await fetchVerificationLink(request, user.email)).toContain(
			"/verify-email",
		)
	})

	test("leaves the keycloak session alone on logout, so the next sign-in goes straight through", async ({
		page,
		request,
	}) => {
		const user = await newIdpUser(request, { emailVerified: true })
		await visit(page, `${OIDC_URL}/login`)
		await logInThroughKeycloak(page, user)
		await expect(page).toHaveURL(`${OIDC_URL}/welcome`, { timeout: 15_000 })
		await hydrated(page)

		await page
			.getByRole("button", {
				name: t("onboarding.welcome.login-info.logout"),
			})
			.click()

		await expect(page).toHaveURL(`${OIDC_URL}/login`, { timeout: 15_000 })
		await hydrated(page)
		await keycloakLoginButton(page).click()
		await expect(page).toHaveURL(`${OIDC_URL}/welcome`, { timeout: 15_000 })
	})

	test("signs a verified email into the account that already has it", async ({
		browser,
		page,
		request,
	}) => {
		// a signup, a verification and an OIDC round trip.
		test.slow()

		const credentials = await signUpAndVerifyByEmail(page, request)
		const user = await newIdpUser(request, {
			emailVerified: true,
			email: credentials.email,
		})
		const context = await browser.newContext()
		const idpPage = await context.newPage()
		await visit(idpPage, `${OIDC_URL}/login`)

		await logInThroughKeycloak(idpPage, user)

		// the account has no workspace yet, so it lands on onboarding.
		await expect(idpPage).toHaveURL(`${OIDC_URL}/welcome`, {
			timeout: 15_000,
		})
		expect(await linkedProviders(context.request, OIDC_URL)).toEqual([
			"credential",
			"oidc",
		])
		await context.close()
	})

	test("refuses an unverified email that belongs to an existing account", async ({
		browser,
		page,
		request,
	}) => {
		test.slow()

		const credentials = await signUpAndVerifyByEmail(page, request)
		const user = await newIdpUser(request, {
			emailVerified: false,
			email: credentials.email,
		})
		const context = await browser.newContext()
		const idpPage = await context.newPage()
		await visit(idpPage, `${OIDC_URL}/login`)

		await logInThroughKeycloak(idpPage, user)

		await expect(idpPage).toHaveURL(
			(url) =>
				url.origin === OIDC_URL &&
				url.pathname === "/auth-error" &&
				url.searchParams.get("error") === "account_not_linked",
			{ timeout: 15_000 },
		)
		await expect(
			idpPage.getByText(t("oauth.error.account-not-linked")),
		).toBeVisible()
		await context.close()
	})
})
