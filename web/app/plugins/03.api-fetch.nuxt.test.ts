import { mockNuxtImport } from "@nuxt/test-utils/runtime"
import { beforeEach, describe, it, vi } from "vitest"
import plugin, {
	redirectToDocId,
	redirectToFirst,
	redirectToLogin,
	redirectToOrgRoot,
} from "./03.api-fetch"

const { navigateToMock, useRouteMock } = vi.hoisted(() => {
	return {
		navigateToMock: vi.fn(),
		useRouteMock: vi.fn(() => ({ fullPath: "/" })),
	}
})

mockNuxtImport("navigateTo", () => navigateToMock)
mockNuxtImport("useRoute", () => useRouteMock)

// restoreMocks does not touch hand-made vi.fn() singletons in vitest 4 —
// every describe resets the module-level mocks explicitly
function resetSharedMocks() {
	navigateToMock.mockReset()
	useRouteMock.mockReset()
}

const DOC_ID_A = "aaaaaaaaaaaaaaaaaaaa" // xid length
const DOC_ID_B = "bbbbbbbbbbbbbbbbbbbb"

interface CreatedClientOptions {
	baseURL?: string
	credentials?: RequestCredentials
	onRequest?: (ctx: { options: { headers?: unknown } }) => void
	onResponseError?: (ctx: { response: Response }) => Promise<void>
}

function arrangeFetch() {
	const created: CreatedClientOptions[] = []
	const clients: { id: number }[] = []

	vi.stubGlobal("$fetch", {
		create: vi.fn((options: CreatedClientOptions) => {
			created.push(options)

			const client = { id: clients.length }
			clients.push(client)

			return client
		}),
	})

	return { created, clients }
}

function nuxtAppStub() {
	const runWithContext = vi.fn(async (fn: () => Promise<unknown>) => fn())

	return {
		runWithContext,
		nuxtApp: { runWithContext } as unknown as {
			runWithContext: <T>(fn: () => Promise<T>) => Promise<T>
		},
	}
}

function orgFetcherStub(slug?: string) {
	const refresh = vi
		.fn()
		.mockResolvedValue(
			slug ? { data: { data: { slug } } } : { data: undefined },
		)

	return {
		refresh,
		fetcher: { refresh } as unknown as ReturnType<
			typeof useAuthSession
		>["fetchOrganization"],
	}
}

function docTreeFetcherStub(docs?: { id: string; documentName: string }[]) {
	const refresh = vi.fn().mockResolvedValue({ data: docs })

	return {
		refresh,
		fetcher: { refresh } as unknown as ReturnType<
			typeof useDocumentAPI
		>["fetchDocumentTree"],
	}
}

// the tests assert call accounting on shared module-level mocks
// (mockNuxtImport singletons), so they cannot interleave
describe("03.api-fetch", { concurrent: false }, () => {
	beforeEach(resetSharedMocks)

	it("provides one client per backend api", ({ expect }) => {
		const { created, clients } = arrangeFetch()

		// the plugin type is a union with the void/promise setup variants;
		// this plugin synchronously returns its provide block
		const result = plugin(useNuxtApp()) as unknown as {
			provide: { coreAPIClient: unknown; authRealtimeAPIClient: unknown }
		}

		expect(result.provide.coreAPIClient).toBe(clients[0])
		expect(result.provide.authRealtimeAPIClient).toBe(clients[1])
		// the test runtime config only overrides the auth-realtime base; the
		// core base keeps its empty default
		expect(created[0]?.baseURL).toBe("")
		expect(created[1]?.baseURL).toBe("http://test.local/auth-realtime")
		expect(created[0]?.credentials).toBe("include")
		expect(created[1]?.credentials).toBe("include")
	})

	it.for([
		{ name: "the core api client", index: 0 },
		{ name: "the auth-realtime api client", index: 1 },
	])(
		"$name leaves request headers untouched in the browser",
		({ index }, { expect }) => {
			const { created } = arrangeFetch()
			void plugin(useNuxtApp())

			const options = { headers: { "x-custom": "1" } }
			created[index]?.onRequest?.({ options })

			expect(options.headers).toEqual({ "x-custom": "1" })
		},
	)

	it.for([
		{ name: "the core api client", index: 0 },
		{ name: "the auth-realtime api client", index: 1 },
	])(
		"$name redirects to login on a 401 response",
		async ({ index }, { expect }) => {
			useRouteMock.mockReturnValue({ fullPath: "/docs/current" })
			const { created } = arrangeFetch()
			void plugin(useNuxtApp())

			await created[index]?.onResponseError?.({
				response: new Response(null, { status: 401 }),
			})

			expect(navigateToMock).toHaveBeenCalledExactlyOnceWith(
				{
					name: "login",
					query: { next: encodeURIComponent("/docs/current") },
				},
				{ replace: false },
			)
		},
	)

	it("ignores non-401 error responses", async ({ expect }) => {
		const { created } = arrangeFetch()
		void plugin(useNuxtApp())

		await created[0]?.onResponseError?.({
			response: new Response(null, { status: 500 }),
		})

		expect(navigateToMock).toHaveBeenCalledTimes(0)
	})
})

describe("redirectToLogin", { concurrent: false }, () => {
	beforeEach(resetSharedMocks)

	it("navigates to login with the encoded current path", async ({ expect }) => {
		navigateToMock.mockReturnValue("navigated")
		const { runWithContext, nuxtApp } = nuxtAppStub()

		const result = await redirectToLogin("/docs/a b", true, nuxtApp)

		expect(result).toBe("navigated")
		expect(runWithContext).toHaveBeenCalledTimes(1)
		expect(navigateToMock).toHaveBeenCalledExactlyOnceWith(
			{ name: "login", query: { next: encodeURIComponent("/docs/a b") } },
			{ replace: true },
		)
	})

	it("omits the next param when there is no current path", async ({
		expect,
	}) => {
		const { nuxtApp } = nuxtAppStub()

		await redirectToLogin(undefined, false, nuxtApp)

		expect(navigateToMock).toHaveBeenCalledExactlyOnceWith(
			{ name: "login", query: { next: undefined } },
			{ replace: false },
		)
	})
})

describe("redirectToDocId", { concurrent: false }, () => {
	beforeEach(resetSharedMocks)

	it("navigates to the organization slug and the document id", async ({
		expect,
	}) => {
		navigateToMock.mockReturnValue("navigated")
		const org = orgFetcherStub("My Org")

		const result = await redirectToDocId(DOC_ID_A, org.fetcher)

		expect(result).toBe("navigated")
		expect(org.refresh).toHaveBeenCalledTimes(1)
		expect(navigateToMock).toHaveBeenCalledExactlyOnceWith(
			`/My-Org/${DOC_ID_A}`,
			{ replace: true },
		)
	})

	it("falls back to the root when the organization is missing", async ({
		expect,
	}) => {
		const org = orgFetcherStub()

		await redirectToDocId(DOC_ID_A, org.fetcher)

		expect(navigateToMock).toHaveBeenCalledExactlyOnceWith("/", {
			replace: true,
		})
	})
})

describe("redirectToOrgRoot", { concurrent: false }, () => {
	beforeEach(resetSharedMocks)

	it("navigates to the organization root", async ({ expect }) => {
		navigateToMock.mockReturnValue("navigated")
		const org = orgFetcherStub("My Org")

		const result = await redirectToOrgRoot(org.fetcher)

		expect(result).toBe("navigated")
		expect(org.refresh).toHaveBeenCalledTimes(1)
		expect(navigateToMock).toHaveBeenCalledExactlyOnceWith("/My-Org", {
			replace: true,
		})
	})

	it("falls back to the bare root when the organization is missing", async ({
		expect,
	}) => {
		const org = orgFetcherStub()

		await redirectToOrgRoot(org.fetcher)

		expect(navigateToMock).toHaveBeenCalledExactlyOnceWith("/", {
			replace: true,
		})
	})
})

describe("redirectToFirst", { concurrent: false }, () => {
	beforeEach(resetSharedMocks)

	it("resolves the organization when no name is given", async ({ expect }) => {
		navigateToMock.mockReturnValue("navigated")
		const org = orgFetcherStub("My Org")
		const tree = docTreeFetcherStub([{ id: DOC_ID_A, documentName: "My Doc" }])

		const result = await redirectToFirst(null, org.fetcher, tree.fetcher, null)

		expect(result).toBe("navigated")
		expect(org.refresh).toHaveBeenCalledTimes(1)
		expect(tree.refresh).toHaveBeenCalledTimes(1)
		expect(navigateToMock).toHaveBeenCalledExactlyOnceWith(
			`/My-Org/My-Doc-${DOC_ID_A}`,
			{ replace: true },
		)
	})

	it("skips the organization fetch when a name is given", async ({
		expect,
	}) => {
		const org = orgFetcherStub("My Org")
		const tree = docTreeFetcherStub([{ id: DOC_ID_A, documentName: "My Doc" }])

		await redirectToFirst("Other Org", org.fetcher, tree.fetcher, null)

		expect(org.refresh).toHaveBeenCalledTimes(0)
		expect(navigateToMock).toHaveBeenCalledExactlyOnceWith(
			`/Other-Org/My-Doc-${DOC_ID_A}`,
			{ replace: true },
		)
	})

	it("falls back to the root without touching the tree when the organization is missing", async ({
		expect,
	}) => {
		const org = orgFetcherStub()
		const tree = docTreeFetcherStub([{ id: DOC_ID_A, documentName: "My Doc" }])

		await redirectToFirst(null, org.fetcher, tree.fetcher, null)

		expect(tree.refresh).toHaveBeenCalledTimes(0)
		expect(navigateToMock).toHaveBeenCalledExactlyOnceWith("/", {
			replace: true,
		})
	})

	it("navigates to the organization root when the tree is empty", async ({
		expect,
	}) => {
		const org = orgFetcherStub("My Org")
		const tree = docTreeFetcherStub([])

		await redirectToFirst(null, org.fetcher, tree.fetcher, null)

		expect(navigateToMock).toHaveBeenCalledExactlyOnceWith("/My-Org", {
			replace: true,
		})
	})

	it("skips the document that is pending deletion", async ({ expect }) => {
		const org = orgFetcherStub("My Org")
		const tree = docTreeFetcherStub([
			{ id: DOC_ID_A, documentName: "My Doc" },
			{ id: DOC_ID_B, documentName: "Other Doc" },
		])

		await redirectToFirst(null, org.fetcher, tree.fetcher, DOC_ID_A)

		expect(navigateToMock).toHaveBeenCalledExactlyOnceWith(
			`/My-Org/Other-Doc-${DOC_ID_B}`,
			{ replace: true },
		)
	})

	it("navigates to the organization root when every document is pending deletion", async ({
		expect,
	}) => {
		const org = orgFetcherStub("My Org")
		const tree = docTreeFetcherStub([{ id: DOC_ID_A, documentName: "My Doc" }])

		await redirectToFirst(null, org.fetcher, tree.fetcher, DOC_ID_A)

		expect(navigateToMock).toHaveBeenCalledExactlyOnceWith("/My-Org", {
			replace: true,
		})
	})
})
