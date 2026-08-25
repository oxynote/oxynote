import { expect, test } from "@playwright/test"
import { coreURL, documentId, sessionCookie } from "../helpers/api"
import {
	newCredentials,
	signUpAndVerify,
	submitLoginForm,
} from "../helpers/auth"
import { inviteTeamMember } from "../helpers/collaboration"
import { waitForEditor } from "../helpers/editor"
import { t } from "../helpers/i18n"
import {
	authorizeMCPClient,
	connectMCPClient,
	documentResourceURI,
} from "../helpers/mcp"
import { visit } from "../helpers/page"
import { documentAccepted } from "../helpers/realtime"
import { openSettings } from "../helpers/settings"
import {
	signUpWithSeparateWorkspace,
	signUpWithWorkspace,
} from "../helpers/workspace"

test.describe("access", () => {
	test("keeps another workspace's document out of the api", async ({
		page,
		request,
		browser,
	}) => {
		// two independent tenants, each paying a signup, a verification, a
		// login and a workspace creation before the first assertion
		test.slow()

		await signUpWithWorkspace(page, request)

		const other = await signUpWithSeparateWorkspace(browser, request)
		const foreign = documentId(other.page)

		// page.request carries the first user's own session cookies, so
		// this is their browser asking core for a document belonging to a
		// workspace they have never been a member of
		const branches = await page.request.get(
			coreURL(`/api/documents/${foreign}/branches`),
		)
		// 200 is what these answer today, not what they should. Both are
		// list queries whose organization filter simply matches nothing,
		// so a denial arrives as an empty success — a document another
		// workspace owns belongs behind a 404, as deleting one already is.
		expect(branches.status()).toBe(200)
		expect(await branches.json()).toEqual([])

		const maintainers = await page.request.get(
			coreURL(`/api/documents/${foreign}/maintainers`),
		)
		expect(maintainers.status()).toBe(200)
		expect(await maintainers.json()).toEqual([])

		await other.context.close()
	})

	test("leaves another workspace's document intact after a delete attempt", async ({
		page,
		request,
		browser,
	}) => {
		test.slow()

		await signUpWithWorkspace(page, request)

		const other = await signUpWithSeparateWorkspace(browser, request)
		const foreign = documentId(other.page)
		const foreignUrl = other.page.url()

		const deletion = await page.request.delete(
			coreURL(`/api/documents/${foreign}`),
		)
		expect(deletion.status()).toBe(404)

		// the owner reloads: a deletion that had gone through would leave
		// them redirected away from a document that no longer exists
		await visit(other.page, foreignUrl)
		await expect(other.page).toHaveURL(foreignUrl)
		await waitForEditor(other.page)

		await other.context.close()
	})

	test("refuses a revoked invitation", async ({ page, request, browser }) => {
		// a signup and verification on each side, plus an invitation sent
		// and withdrawn between them
		test.slow()

		const { workspace } = await signUpWithWorkspace(page, request)
		const invitee = newCredentials()
		const inviteLink = await inviteTeamMember(page, request, invitee.email)

		// withdraw it before it is ever used. A pending invitation is
		// listed among the members, so it is revoked the same way a member
		// is removed
		const settings = await openSettings(page, workspace.name)
		const row = settings.locator("tr", { hasText: invitee.email })
		await expect(row).toContainText(t("settings.workspace.invited-label"))
		await row
			.getByRole("button", {
				name: t("settings.workspace.members-option-button-screen-reader-hint"),
			})
			.click()
		await page
			.getByRole("menuitem", {
				name: t("settings.workspace.member-options.delete.title"),
			})
			.click()
		await page
			.getByRole("button", {
				name: t(
					"settings.action-modals.workspace-member-removal.submit-button",
				),
			})
			.click()
		await expect(row).toHaveCount(0)

		// the invitee still holds the emailed link, and it is the server
		// that has to refuse it
		const context = await browser.newContext()
		const joiner = await context.newPage()
		await signUpAndVerify(joiner, request, invitee)
		await submitLoginForm(joiner, invitee)
		await expect(joiner).toHaveURL(/\/welcome$/, { timeout: 15_000 })

		await visit(joiner, inviteLink)
		await joiner
			.getByRole("button", {
				name: t("onboarding.accept-invite.accept-button"),
			})
			.click()

		await expect(
			joiner.getByText(t("onboarding.accept-invite.errors.accept-failed")),
		).toBeVisible()
		await expect(joiner).toHaveURL(/\/accept-invite\?/)

		await context.close()
	})

	test("rejects a session that has been signed out", async ({
		page,
		request,
	}) => {
		const { workspace } = await signUpWithWorkspace(page, request)

		// captured while the session is still live. Signing out clears the
		// browser's copy, and the point of the case is that the server
		// refuses the cookie even when a caller still holds one
		const cookie = await sessionCookie(page)

		const before = await request.get(coreURL("/api/capabilities"), {
			headers: { cookie },
		})
		expect(before.status()).toBe(200)

		await page.getByRole("button", { name: workspace.name }).click()
		await page
			.getByRole("menuitem", { name: t("sidebar.header.log-out") })
			.click()
		await expect(page).toHaveURL(/\/login$/, { timeout: 15_000 })

		const after = await request.get(coreURL("/api/capabilities"), {
			headers: { cookie },
		})
		expect(after.status()).toBe(401)
	})

	test("keeps another workspace's document off the realtime socket", async ({
		page,
		request,
		browser,
	}) => {
		test.slow()

		await signUpWithWorkspace(page, request)
		const own = documentId(page)

		const other = await signUpWithSeparateWorkspace(browser, request)
		const foreign = documentId(other.page)

		// the control. If the probe cannot get into a document the user
		// does own, the rejection below would prove nothing about
		// authorization and everything about a broken probe
		expect(await documentAccepted(page, `${own}-default`)).toBe(true)

		// the same browser, the same session, asking for a document in a
		// workspace it has never been a member of. A document id is all
		// this takes, and every workspace shows its own in the address
		// bar of every page
		expect(await documentAccepted(page, `${foreign}-default`)).toBe(false)

		await other.context.close()
	})

	test("lists only the granting workspace's documents over mcp", async ({
		page,
		request,
		browser,
	}) => {
		// two tenants, plus a registration, a consent and a token
		// exchange on top of the usual setup
		test.slow()

		await signUpWithWorkspace(page, request)
		const own = documentId(page)

		const other = await signUpWithSeparateWorkspace(browser, request)
		const foreign = documentId(other.page)

		const client = await connectMCPClient(
			await authorizeMCPClient(page, request),
		)
		const { resources } = await client.listResources()
		const uris = resources.map((resource) => resource.uri)

		// the token was granted by a user of the first workspace, so the
		// organization it is bound to is the only one it can see
		expect(uris).toContain(documentResourceURI(own))
		expect(uris).not.toContain(documentResourceURI(foreign))

		await client.close()
		await other.context.close()
	})

	test("refuses to read another workspace's document over mcp", async ({
		page,
		request,
		browser,
	}) => {
		test.slow()

		await signUpWithWorkspace(page, request)
		const own = documentId(page)

		const other = await signUpWithSeparateWorkspace(browser, request)
		const foreign = documentId(other.page)

		const client = await connectMCPClient(
			await authorizeMCPClient(page, request),
		)

		// the control: the same call on the token's own document has to
		// come back, or the refusal below would say nothing about
		// scoping and everything about a read that never works
		const readable = await client.readResource({
			uri: documentResourceURI(own),
		})
		expect(readable.contents).not.toHaveLength(0)

		// named directly rather than picked off the listing: a scoped
		// listing is no protection if the read behind it answers for
		// any id it is handed
		await expect(
			client.readResource({ uri: documentResourceURI(foreign) }),
		).rejects.toThrow()

		await client.close()
		await other.context.close()
	})

	const BEARERS: { name: string; headers: Record<string, string> }[] = [
		{ name: "no authorization header", headers: {} },
		{
			name: "a malformed authorization header",
			headers: { authorization: "Bearer" },
		},
		{
			name: "an unissued bearer token",
			headers: { authorization: "Bearer not-a-real-access-token" },
		},
	]

	for (const bearer of BEARERS) {
		test(`refuses an mcp call with ${bearer.name}`, async ({ request }) => {
			const response = await request.post(coreURL("/api/mcp"), {
				headers: {
					...bearer.headers,
					accept: "application/json, text/event-stream",
					"content-type": "application/json",
				},
				data: {
					jsonrpc: "2.0",
					id: 1,
					method: "tools/list",
				},
			})

			expect(response.status()).toBe(401)
		})
	}
})
