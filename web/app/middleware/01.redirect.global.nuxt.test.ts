import { mockNuxtImport } from "@nuxt/test-utils/runtime"
import { beforeAll, beforeEach, describe, it, vi } from "vitest"
import type { RouteLocationNormalized } from "vue-router"
import redirectMiddleware from "./01.redirect.global"

const { navigateToMock, useAuthSessionMock, useDocumentAPIMock } = vi.hoisted(
	() => {
		return {
			navigateToMock: vi.fn(),
			// the default implementation keeps the app bootstrap alive: the
			// websocket plugin refreshes the session query during nuxt init,
			// before any test arranges its own return value
			useAuthSessionMock: vi.fn((): any => ({
				fetchAuthSession: {
					refresh: () => Promise.resolve({ data: undefined }),
				},
				fetchOrganization: {
					refresh: () => Promise.resolve({ data: undefined }),
				},
			})),
			useDocumentAPIMock: vi.fn((): any => ({})),
		}
	},
)

mockNuxtImport("navigateTo", () => navigateToMock)
mockNuxtImport("useAuthSession", () => useAuthSessionMock)
mockNuxtImport("useDocumentAPI", () => useDocumentAPIMock)

const DOC_ID = "aaaaaaaaaaaaaaaaaaaa" // xid length

interface Session {
	activeOrganizationId: string | null
}

function route(overrides: {
	name?: string
	path?: string
	skipAuth?: boolean
	query?: Record<string, string>
}): RouteLocationNormalized {
	const path = overrides.path ?? "/Some-Org/some-doc"

	return {
		name: overrides.name ?? "organizationSlug-documentSlug",
		path,
		fullPath: path,
		meta: overrides.skipAuth ? { skipAuth: true } : {},
		query: overrides.query ?? {},
	} as unknown as RouteLocationNormalized
}

function arrange(opts: {
	session?: Session
	orgSlug?: string
	docTree?: { id: string; documentName: string }[]
}) {
	const fetchAuthSession = {
		refresh: vi
			.fn()
			.mockResolvedValue(
				opts.session
					? { data: { data: { session: opts.session } } }
					: { data: undefined },
			),
	}
	const fetchOrganization = {
		refresh: vi
			.fn()
			.mockResolvedValue(
				opts.orgSlug
					? { data: { data: { slug: opts.orgSlug } } }
					: { data: undefined },
			),
	}
	const fetchDocumentTree = {
		refresh: vi.fn().mockResolvedValue({ data: opts.docTree }),
	}

	useAuthSessionMock.mockReturnValue({ fetchAuthSession, fetchOrganization })
	useDocumentAPIMock.mockReturnValue({ fetchDocumentTree })
	navigateToMock.mockReturnValue("navigated")

	return { fetchAuthSession, fetchOrganization, fetchDocumentTree }
}

// the tests arrange shared module-level mocks (mockNuxtImport singletons),
// so they cannot interleave
describe("01.redirect.global", { concurrent: false }, () => {
	// the nuxt test app's initial navigation runs this same global
	// middleware asynchronously; wait for it to settle so its calls don't
	// leak into the first test's mock accounting
	beforeAll(async () => {
		await useRouter().isReady()
	})

	// restoreMocks does not touch hand-made vi.fn() singletons in vitest 4
	// — reset the module-level mocks explicitly
	beforeEach(() => {
		navigateToMock.mockReset()
		useAuthSessionMock.mockReset()
		useDocumentAPIMock.mockReset()
	})

	it.for([{ page: "desktop-auth" }, { page: "verify-email" }])(
		"lets the $page page pass without touching the session",
		async ({ page }, { expect }) => {
			const { fetchAuthSession } = arrange({})

			const result = await redirectMiddleware(route({ name: page }), route({}))

			expect(result).toBeUndefined()
			expect(fetchAuthSession.refresh).toHaveBeenCalledTimes(0)
			expect(navigateToMock).toHaveBeenCalledTimes(0)
		},
	)

	describe("accept-invite", () => {
		it("redirects to the root when the user already has an organization", async ({
			expect,
		}) => {
			arrange({ session: { activeOrganizationId: "org1" } })

			const result = await redirectMiddleware(
				route({ name: "accept-invite", path: "/accept-invite" }),
				route({}),
			)

			expect(result).toBe("navigated")
			expect(navigateToMock).toHaveBeenCalledExactlyOnceWith("/", {
				replace: true,
			})
		})

		it("proceeds when the user has no organization yet", async ({ expect }) => {
			arrange({ session: { activeOrganizationId: null } })

			const result = await redirectMiddleware(
				route({ name: "accept-invite", path: "/accept-invite" }),
				route({}),
			)

			expect(result).toBeUndefined()
			expect(navigateToMock).toHaveBeenCalledTimes(0)
		})

		it("proceeds for a signed-out visitor", async ({ expect }) => {
			arrange({})

			const result = await redirectMiddleware(
				route({ name: "accept-invite", path: "/accept-invite" }),
				route({}),
			)

			expect(result).toBeUndefined()
			expect(navigateToMock).toHaveBeenCalledTimes(0)
		})
	})

	describe("onboarding", () => {
		it("sends a session without an organization to onboarding", async ({
			expect,
		}) => {
			arrange({ session: { activeOrganizationId: null } })

			const result = await redirectMiddleware(route({}), route({}))

			expect(result).toBe("navigated")
			expect(navigateToMock).toHaveBeenCalledExactlyOnceWith(
				{ name: "onboarding" },
				{ replace: true },
			)
		})

		it("keeps an onboarded user away from onboarding", async ({ expect }) => {
			arrange({ session: { activeOrganizationId: "org1" } })

			const result = await redirectMiddleware(
				route({ name: "onboarding", path: "/onboarding" }),
				route({}),
			)

			expect(result).toBe("navigated")
			expect(navigateToMock).toHaveBeenCalledExactlyOnceWith("/", {
				replace: true,
			})
		})

		it("lets a user without an organization stay on onboarding", async ({
			expect,
		}) => {
			arrange({ session: { activeOrganizationId: null } })

			const result = await redirectMiddleware(
				route({ name: "onboarding", path: "/onboarding" }),
				route({}),
			)

			expect(result).toBeUndefined()
			expect(navigateToMock).toHaveBeenCalledTimes(0)
		})
	})

	describe("auth gate", () => {
		it("redirects a signed-out visitor to login with the target path", async ({
			expect,
		}) => {
			arrange({})

			const result = await redirectMiddleware(
				route({ path: "/Some-Org/some-doc" }),
				route({}),
			)

			expect(result).toBe("navigated")
			expect(navigateToMock).toHaveBeenCalledExactlyOnceWith(
				{
					name: "login",
					query: { next: encodeURIComponent("/Some-Org/some-doc") },
				},
				{ replace: true },
			)
		})

		it("omits the next param when a signed-out visitor hits the root", async ({
			expect,
		}) => {
			arrange({})

			const result = await redirectMiddleware(route({ path: "/" }), route({}))

			expect(result).toBe("navigated")
			expect(navigateToMock).toHaveBeenCalledExactlyOnceWith(
				{ name: "login", query: { next: undefined } },
				{ replace: true },
			)
		})

		it("lets a signed-out visitor view auth pages", async ({ expect }) => {
			arrange({})

			const result = await redirectMiddleware(
				route({ name: "login", path: "/login", skipAuth: true }),
				route({}),
			)

			expect(result).toBeUndefined()
			expect(navigateToMock).toHaveBeenCalledTimes(0)
		})

		it("redirects a signed-in user away from auth pages to the next target", async ({
			expect,
		}) => {
			arrange({ session: { activeOrganizationId: "org1" } })

			const result = await redirectMiddleware(
				route({
					name: "login",
					path: "/login",
					skipAuth: true,
					query: { next: encodeURIComponent("/Some-Org/some-doc") },
				}),
				route({}),
			)

			expect(result).toBe("navigated")
			expect(navigateToMock).toHaveBeenCalledExactlyOnceWith(
				"/Some-Org/some-doc",
				{ replace: true },
			)
		})

		it("redirects a signed-in user away from auth pages to the root", async ({
			expect,
		}) => {
			arrange({ session: { activeOrganizationId: "org1" } })

			const result = await redirectMiddleware(
				route({ name: "login", path: "/login", skipAuth: true }),
				route({}),
			)

			expect(result).toBe("navigated")
			expect(navigateToMock).toHaveBeenCalledExactlyOnceWith("/", {
				replace: true,
			})
		})
	})

	describe("root path", () => {
		it("redirects to login when the organization cannot be resolved", async ({
			expect,
		}) => {
			const { fetchOrganization } = arrange({
				session: { activeOrganizationId: "org1" },
			})

			const result = await redirectMiddleware(route({ path: "/" }), route({}))

			expect(result).toBe("navigated")
			expect(fetchOrganization.refresh).toHaveBeenCalledTimes(1)
			expect(navigateToMock).toHaveBeenCalledExactlyOnceWith(
				{ name: "login", query: { next: encodeURIComponent("/") } },
				{ replace: true },
			)
		})

		it("redirects to the organization root when there are no documents", async ({
			expect,
		}) => {
			const { fetchDocumentTree } = arrange({
				session: { activeOrganizationId: "org1" },
				orgSlug: "My Org",
				docTree: [],
			})

			const result = await redirectMiddleware(route({ path: "/" }), route({}))

			expect(result).toBe("navigated")
			expect(fetchDocumentTree.refresh).toHaveBeenCalledTimes(1)
			expect(navigateToMock).toHaveBeenCalledExactlyOnceWith("/My-Org", {
				replace: true,
			})
		})

		it("redirects to the first document", async ({ expect }) => {
			arrange({
				session: { activeOrganizationId: "org1" },
				orgSlug: "My Org",
				docTree: [
					{ id: DOC_ID, documentName: "My Doc" },
					{ id: "bbbbbbbbbbbbbbbbbbbb", documentName: "Other Doc" },
				],
			})

			const result = await redirectMiddleware(route({ path: "/" }), route({}))

			expect(result).toBe("navigated")
			expect(navigateToMock).toHaveBeenCalledExactlyOnceWith(
				`/My-Org/My-Doc-${DOC_ID}`,
				{ replace: true },
			)
		})

		it("redirects a signed-out visitor on a skip-auth root page to login", async ({
			expect,
		}) => {
			arrange({})

			const result = await redirectMiddleware(
				route({ path: "/", skipAuth: true }),
				route({}),
			)

			expect(result).toBe("navigated")
			expect(navigateToMock).toHaveBeenCalledExactlyOnceWith(
				{ name: "login", query: { next: undefined } },
				{ replace: true },
			)
		})
	})

	it("does nothing for an authenticated user on a document page", async ({
		expect,
	}) => {
		const { fetchAuthSession, fetchOrganization } = arrange({
			session: { activeOrganizationId: "org1" },
		})

		const result = await redirectMiddleware(route({}), route({}))

		expect(result).toBeUndefined()
		expect(fetchAuthSession.refresh).toHaveBeenCalledTimes(1)
		expect(fetchOrganization.refresh).toHaveBeenCalledTimes(0)
		expect(useDocumentAPIMock).toHaveBeenCalledTimes(0)
		expect(navigateToMock).toHaveBeenCalledTimes(0)
	})
})
