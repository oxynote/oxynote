import { beforeEach, describe, it, vi } from "vitest"
import useAuthSession from "./useAuthSession"

const { authClientStub, createAuthClientMock, allStubFns } = vi.hoisted(() => {
	// getSession resolves an empty session by default because the websocket
	// plugin refreshes the session query while the test app bootstraps with
	// this module mock in place
	const authClientStub = {
		getSession: vi.fn(() => Promise.resolve({ data: null, error: null })),
		listAccounts: vi.fn(() => Promise.resolve({ data: null, error: null })),
		signOut: vi.fn(),
		requestPasswordReset: vi.fn(),
		resetPassword: vi.fn(),
		changePassword: vi.fn(),
		updateUser: vi.fn(),
		changeEmail: vi.fn(),
		deleteUser: vi.fn(),
		ensureElectronRedirect: vi.fn(),
		signIn: { social: vi.fn(), email: vi.fn() },
		signUp: { email: vi.fn() },
		organization: {
			getFullOrganization: vi.fn(() =>
				Promise.resolve({ data: null, error: null }),
			),
			setActive: vi.fn(),
			checkSlug: vi.fn(),
			create: vi.fn(),
			acceptInvitation: vi.fn(),
			update: vi.fn(),
			inviteMember: vi.fn(),
			cancelInvitation: vi.fn(),
			removeMember: vi.fn(),
		},
	}

	const allStubFns = [
		authClientStub.getSession,
		authClientStub.listAccounts,
		authClientStub.signOut,
		authClientStub.requestPasswordReset,
		authClientStub.resetPassword,
		authClientStub.changePassword,
		authClientStub.updateUser,
		authClientStub.changeEmail,
		authClientStub.deleteUser,
		authClientStub.ensureElectronRedirect,
		authClientStub.signIn.social,
		authClientStub.signIn.email,
		authClientStub.signUp.email,
		authClientStub.organization.getFullOrganization,
		authClientStub.organization.setActive,
		authClientStub.organization.checkSlug,
		authClientStub.organization.create,
		authClientStub.organization.acceptInvitation,
		authClientStub.organization.update,
		authClientStub.organization.inviteMember,
		authClientStub.organization.cancelInvitation,
		authClientStub.organization.removeMember,
	]

	return {
		authClientStub,
		allStubFns,
		createAuthClientMock: vi.fn(() => authClientStub),
	}
})

vi.mock("better-auth/vue", () => {
	return {
		createAuthClient: createAuthClientMock,
	}
})

// the composable's queries inject() their option defaults, which vue only
// allows inside a component or an app context — runWithContext provides
// the latter, keeping the tests free of inject() warnings. The assertion
// is for eslint's ts program, which resolves the call through
// runWithContext as error typed; vue-tsc infers it fine.
function makeAuthSession() {
	return useNuxtApp().runWithContext(() => useAuthSession()) as ReturnType<
		typeof useAuthSession
	>
}

// the delegating methods differ only in which auth client method they wrap
// — a pure data table. The desktop (__DESKTOP_BUILD__) halves of these
// methods are compile-time dead in the web bundle tests run in.
const DELEGATION_CASES: {
	name: string
	invoke: (auth: ReturnType<typeof useAuthSession>) => unknown
	mock: () => ReturnType<typeof vi.fn>
	expectedArgs: unknown[]
}[] = [
	{
		name: "signInSocial",
		invoke: (auth) => auth.signInSocial({ provider: "github" }),
		mock: () => authClientStub.signIn.social,
		expectedArgs: [{ provider: "github" }],
	},
	{
		name: "signInEmailPassword",
		invoke: (auth) =>
			auth.signInEmailPassword({ email: "a@b.co", password: "pw" }),
		mock: () => authClientStub.signIn.email,
		expectedArgs: [{ email: "a@b.co", password: "pw" }],
	},
	{
		name: "signUpEmailPassword",
		invoke: (auth) =>
			auth.signUpEmailPassword({ email: "a@b.co", password: "pw", name: "A" }),
		mock: () => authClientStub.signUp.email,
		expectedArgs: [{ email: "a@b.co", password: "pw", name: "A" }],
	},
	{
		name: "requestPasswordReset",
		invoke: (auth) => auth.requestPasswordReset({ email: "a@b.co" }),
		mock: () => authClientStub.requestPasswordReset,
		expectedArgs: [{ email: "a@b.co" }],
	},
	{
		name: "resetPassword",
		invoke: (auth) => auth.resetPassword({ newPassword: "pw", token: "t" }),
		mock: () => authClientStub.resetPassword,
		expectedArgs: [{ newPassword: "pw", token: "t" }],
	},
	{
		name: "changePassword",
		invoke: (auth) =>
			auth.changePassword({ currentPassword: "old", newPassword: "new" }),
		mock: () => authClientStub.changePassword,
		expectedArgs: [{ currentPassword: "old", newPassword: "new" }],
	},
	{
		name: "setupSignInRedirect",
		invoke: (auth) => auth.setupSignInRedirect(),
		mock: () => authClientStub.ensureElectronRedirect,
		expectedArgs: [],
	},
	{
		name: "updateUser",
		invoke: (auth) => auth.updateUser({ name: "New Name" }),
		mock: () => authClientStub.updateUser,
		expectedArgs: [{ name: "New Name" }],
	},
	{
		name: "changeEmail",
		invoke: (auth) => auth.changeEmail({ newEmail: "new@b.co" }),
		mock: () => authClientStub.changeEmail,
		expectedArgs: [{ newEmail: "new@b.co" }],
	},
	{
		name: "deleteUser",
		invoke: (auth) => auth.deleteUser({ password: "pw" }),
		mock: () => authClientStub.deleteUser,
		expectedArgs: [{ password: "pw" }],
	},
	{
		name: "checkOrganizationSlug",
		invoke: (auth) => auth.checkOrganizationSlug({ slug: "my-org" }),
		mock: () => authClientStub.organization.checkSlug,
		expectedArgs: [{ slug: "my-org" }],
	},
	{
		name: "createOrganization",
		invoke: (auth) => auth.createOrganization({ name: "Org", slug: "org" }),
		mock: () => authClientStub.organization.create,
		expectedArgs: [{ name: "Org", slug: "org" }],
	},
	{
		name: "setActiveOrganization",
		invoke: (auth) => auth.setActiveOrganization({ organizationId: "o1" }),
		mock: () => authClientStub.organization.setActive,
		expectedArgs: [{ organizationId: "o1" }],
	},
	{
		name: "acceptOrganizationInvitation",
		invoke: (auth) => auth.acceptOrganizationInvitation({ invitationId: "i1" }),
		mock: () => authClientStub.organization.acceptInvitation,
		expectedArgs: [{ invitationId: "i1" }],
	},
	{
		name: "updateOrganization",
		invoke: (auth) =>
			auth.updateOrganization({ organizationId: "o1", data: { name: "N" } }),
		mock: () => authClientStub.organization.update,
		expectedArgs: [{ organizationId: "o1", data: { name: "N" } }],
	},
	{
		name: "inviteOrganizationMember",
		invoke: (auth) =>
			auth.inviteOrganizationMember({ email: "a@b.co", role: "member" }),
		mock: () => authClientStub.organization.inviteMember,
		expectedArgs: [{ email: "a@b.co", role: "member" }],
	},
	{
		name: "cancelOrganizationInvitation",
		invoke: (auth) => auth.cancelOrganizationInvitation({ invitationId: "i1" }),
		mock: () => authClientStub.organization.cancelInvitation,
		expectedArgs: [{ invitationId: "i1" }],
	},
	{
		name: "removeOrganizationMember",
		invoke: (auth) =>
			auth.removeOrganizationMember({ memberIdOrEmail: "a@b.co" }),
		mock: () => authClientStub.organization.removeMember,
		expectedArgs: [{ memberIdOrEmail: "a@b.co" }],
	},
]

// the tests assert call accounting on shared module-level mocks and share
// the app-wide query cache, so they cannot interleave
describe("useAuthSession", { concurrent: false }, () => {
	// restoreMocks does not touch hand-made vi.fn() singletons in vitest 4
	// — reset the module-level mocks explicitly
	beforeEach(() => {
		allStubFns.forEach((fn) => {
			fn.mockReset()
		})
	})

	it("fetches the session through the auth client", async ({ expect }) => {
		const sessionData = { data: { session: { id: "s1" } }, error: null }
		authClientStub.getSession.mockResolvedValue(sessionData as never)
		const auth = makeAuthSession()

		const result = await auth.fetchAuthSession.refetch()

		expect(authClientStub.getSession).toHaveBeenCalledTimes(1)
		expect(result.data).toEqual(sessionData)
	})

	it("fetches the organization through the auth client", async ({ expect }) => {
		const orgData = { data: { id: "o1", slug: "my-org" }, error: null }
		authClientStub.organization.getFullOrganization.mockResolvedValue(
			orgData as never,
		)
		const auth = makeAuthSession()

		const result = await auth.fetchOrganization.refetch()

		expect(
			authClientStub.organization.getFullOrganization,
		).toHaveBeenCalledTimes(1)
		expect(result.data).toEqual(orgData)
	})

	describe("hasPassword", () => {
		it("reports true when a credential account exists", async ({ expect }) => {
			authClientStub.getSession.mockResolvedValue({
				data: { session: { id: "s1" } },
				error: null,
			} as never)
			authClientStub.listAccounts.mockResolvedValue({
				data: [{ providerId: "github" }, { providerId: "credential" }],
				error: null,
			} as never)
			const auth = makeAuthSession()

			await auth.fetchAuthSession.refetch()
			await auth.fetchAccounts.refetch()

			expect(auth.hasPassword.value).toBe(true)
		})

		it("reports false without a credential account", async ({ expect }) => {
			authClientStub.getSession.mockResolvedValue({
				data: { session: { id: "s1" } },
				error: null,
			} as never)
			authClientStub.listAccounts.mockResolvedValue({
				data: [{ providerId: "github" }],
				error: null,
			} as never)
			const auth = makeAuthSession()

			await auth.fetchAuthSession.refetch()
			await auth.fetchAccounts.refetch()

			expect(auth.hasPassword.value).toBe(false)
		})
	})

	describe("updateSessionOnInviteAccept", () => {
		it("returns false when activating the organization fails", async ({
			expect,
		}) => {
			authClientStub.organization.setActive.mockResolvedValue({
				data: null,
				error: { message: "denied" },
			})
			const auth = makeAuthSession()

			const result = await auth.updateSessionOnInviteAccept("o1")

			expect(result).toBe(false)
			expect(authClientStub.getSession).toHaveBeenCalledTimes(0)
			expect(
				authClientStub.organization.getFullOrganization,
			).toHaveBeenCalledTimes(0)
		})

		it("returns true when the new organization becomes active", async ({
			expect,
		}) => {
			authClientStub.organization.setActive.mockResolvedValue({
				data: {},
				error: null,
			})
			authClientStub.organization.getFullOrganization.mockResolvedValue({
				data: { id: "o1" },
				error: null,
			} as never)
			const auth = makeAuthSession()

			const result = await auth.updateSessionOnInviteAccept("o1")

			expect(result).toBe(true)
			expect(authClientStub.organization.setActive).toHaveBeenCalledWith({
				organizationId: "o1",
			})
			expect(authClientStub.getSession).toHaveBeenCalledTimes(1)
			expect(
				authClientStub.organization.getFullOrganization,
			).toHaveBeenCalledTimes(1)
		})

		it("returns false when a different organization stays active", async ({
			expect,
		}) => {
			authClientStub.organization.setActive.mockResolvedValue({
				data: {},
				error: null,
			})
			authClientStub.organization.getFullOrganization.mockResolvedValue({
				data: { id: "other" },
				error: null,
			} as never)
			const auth = makeAuthSession()

			const result = await auth.updateSessionOnInviteAccept("o1")

			expect(result).toBe(false)
		})
	})

	describe("safeSignOut", () => {
		it("returns the auth error without touching the cache", async ({
			expect,
		}) => {
			const failure = { data: null, error: { message: "nope" } }
			authClientStub.signOut.mockResolvedValue(failure)
			const auth = makeAuthSession()

			const result = await auth.safeSignOut()

			expect(result).toBe(failure)
			expect(authClientStub.signOut).toHaveBeenCalledTimes(1)
		})

		it("signs out and clears the cached session", async ({ expect }) => {
			authClientStub.getSession.mockResolvedValue({
				data: { session: { id: "s1" } },
				error: null,
			} as never)
			const success = { data: {}, error: null }
			authClientStub.signOut.mockResolvedValue(success)
			const auth = makeAuthSession()
			await auth.fetchAuthSession.refetch()

			const result = await auth.safeSignOut()

			expect(result).toBe(success)
			// the cache entry is removed, so a fresh consumer starts without
			// a session
			const fresh = makeAuthSession()
			expect(fresh.fetchAuthSession.data.value).toBeUndefined()
		})
	})

	it.for(DELEGATION_CASES)(
		"$name delegates to the auth client",
		async ({ invoke, mock, expectedArgs }, { expect }) => {
			const clientMethod = mock()
			const response = { data: { ok: true }, error: null }
			clientMethod.mockResolvedValue(response)
			const auth = makeAuthSession()

			const result = await invoke(auth)

			expect(clientMethod).toHaveBeenCalledExactlyOnceWith(...expectedArgs)
			expect(result).toBe(response)
		},
	)
})
