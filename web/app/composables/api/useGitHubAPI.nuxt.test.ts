import { flushPromises } from "@vue/test-utils"
import { afterEach, beforeEach, describe, it } from "vitest"
import {
	clearQueryCache,
	disposeMockEndpoints,
	mockEndpoint,
	runInApp,
	seedQueryData,
} from "./test-helpers"
import useGitHubAPI from "./useGitHubAPI"

const CONNECTED_KEY = ["github", "connected"] as const

function makeGitHubAPI() {
	return runInApp(() => useGitHubAPI())
}

// creating the composable eagerly loads its queries once; refresh() joins
// that in-flight load (or reuses its fresh result) instead of forcing a
// second request, which keeps the call accounting deterministic
describe("useGitHubAPI", { concurrent: false }, () => {
	// the tests share the app-wide query cache and the test-time endpoint
	// registry, so they cannot interleave
	beforeEach(clearQueryCache)

	afterEach(disposeMockEndpoints)

	describe("fetchGitHubConnectionStatus", () => {
		it("fetches the connection status", async ({ expect }) => {
			const statusCalls = mockEndpoint("GET", "/api/github", () => ({
				connected: true,
				configured: true,
			}))
			const api = makeGitHubAPI()

			const result = await api.fetchGitHubConnectionStatus.refresh()

			expect(result.data).toEqual({ connected: true, configured: true })
			expect(statusCalls).toHaveLength(1)
		})

		// a component only ever reads this query; the explicit refresh()
		// below is reserved for the repository queries, which are
		// unreachable while GitHub is disabled
		it("makes no request on a deployment without the GitHub App", async ({
			expect,
		}) => {
			seedQueryData(["capabilities"], { github: false })
			const statusCalls = mockEndpoint("GET", "/api/github", () => ({
				connected: true,
				configured: true,
			}))
			const api = makeGitHubAPI()

			await flushPromises()

			expect(statusCalls).toHaveLength(0)
			expect(api.fetchGitHubConnectionStatus.data.value).toBeUndefined()
		})
	})

	describe("gitHubConfigured", () => {
		it("reports configured while the capabilities are unknown", ({
			expect,
		}) => {
			const api = makeGitHubAPI()

			expect(api.gitHubConfigured.value).toBe(true)
		})

		it("reports unconfigured once the capabilities say so", ({ expect }) => {
			seedQueryData(["capabilities"], { github: false })
			const api = makeGitHubAPI()

			expect(api.gitHubConfigured.value).toBe(false)
		})
	})

	describe("fetchGitHubInstallURL", () => {
		it("fetches the install url", async ({ expect }) => {
			const installCalls = mockEndpoint("GET", "/api/github/install", () => ({
				url: "https://github.test/i",
			}))
			const api = makeGitHubAPI()

			const result = await api.fetchGitHubInstallURL()

			expect(result).toEqual({ url: "https://github.test/i" })
			expect(installCalls).toHaveLength(1)
		})
	})

	describe("connectGitHub", () => {
		it("connects with the callback query parameters", async ({ expect }) => {
			const connectCalls = mockEndpoint(
				"GET",
				"/api/github/connect",
				() => ({}),
			)
			const api = makeGitHubAPI()

			await api.connectGitHub.mutateAsync(new URLSearchParams({ code: "c1" }))

			expect(connectCalls).toHaveLength(1)
			expect(connectCalls[0]?.query).toEqual({ code: "c1" })
		})

		it("connects without query parameters", async ({ expect }) => {
			const connectCalls = mockEndpoint(
				"GET",
				"/api/github/connect",
				() => ({}),
			)
			const api = makeGitHubAPI()

			await api.connectGitHub.mutateAsync(new URLSearchParams())

			expect(connectCalls).toHaveLength(1)
			expect(connectCalls[0]?.query).toEqual({})
		})
	})

	describe("disconnectGitHub", () => {
		it("disconnects and refreshes the connection status", async ({
			expect,
		}) => {
			const statusCalls = mockEndpoint("GET", "/api/github", () => ({
				connected: true,
				configured: true,
			}))
			const deleteCalls = mockEndpoint("DELETE", "/api/github", () => ({}))
			const api = makeGitHubAPI()
			await api.fetchGitHubConnectionStatus.refresh()

			await api.disconnectGitHub.mutateAsync()

			expect(deleteCalls).toHaveLength(1)
			// the success invalidation refetches the active status query
			expect(statusCalls).toHaveLength(2)
			expect(api.fetchGitHubConnectionStatus.data.value).toEqual({
				connected: true,
				configured: true,
			})
		})

		it("rolls back the optimistic disconnect when the request fails", async ({
			expect,
		}) => {
			const statusCalls = mockEndpoint("GET", "/api/github", () => ({
				connected: true,
				configured: true,
			}))
			mockEndpoint("DELETE", "/api/github", () => {
				throw createError({ statusCode: 500 })
			})
			const api = makeGitHubAPI()
			await api.fetchGitHubConnectionStatus.refresh()

			await expect(api.disconnectGitHub.mutateAsync()).rejects.toThrow()

			expect(statusCalls).toHaveLength(1)
			expect(api.fetchGitHubConnectionStatus.data.value).toEqual({
				connected: true,
				configured: true,
			})
		})

		it("skips the rollback when the cache changed after the optimistic disconnect", async ({
			expect,
		}) => {
			let rejectDelete: (err: unknown) => void = () => undefined
			let deleteReached: () => void = () => undefined
			const deleteReachedSignal = new Promise<void>((resolve) => {
				deleteReached = resolve
			})

			mockEndpoint("GET", "/api/github", () => ({
				connected: true,
				configured: true,
			}))
			mockEndpoint("DELETE", "/api/github", () => {
				deleteReached()

				return new Promise((_resolve, reject) => {
					rejectDelete = reject
				})
			})
			const api = makeGitHubAPI()
			await api.fetchGitHubConnectionStatus.refresh()

			const pending = api.disconnectGitHub.mutateAsync()
			await deleteReachedSignal

			// the optimistic update landed; divergent data written afterwards
			// must survive the failure
			expect(api.fetchGitHubConnectionStatus.data.value).toEqual({
				connected: false,
				configured: true,
			})
			seedQueryData(CONNECTED_KEY, {
				connected: true,
				configured: false,
			})
			rejectDelete(createError({ statusCode: 500 }))

			await expect(pending).rejects.toThrow()
			expect(api.fetchGitHubConnectionStatus.data.value).toEqual({
				connected: true,
				configured: false,
			})
		})
	})

	describe("fetchGitHubRepositories", () => {
		it("returns no repositories when github is not connected", async ({
			expect,
		}) => {
			const statusCalls = mockEndpoint("GET", "/api/github", () => ({
				connected: false,
				configured: true,
			}))
			const repoCalls = mockEndpoint(
				"GET",
				"/api/github/repositories",
				() => [],
			)
			const api = makeGitHubAPI()

			const result = await api.fetchGitHubRepositories.refresh()

			expect(result.data).toEqual([])
			expect(repoCalls).toHaveLength(0)
			expect(statusCalls).toHaveLength(1)
		})

		it("fetches the repositories when github is connected", async ({
			expect,
		}) => {
			const statusCalls = mockEndpoint("GET", "/api/github", () => ({
				connected: true,
				configured: true,
			}))
			const repoCalls = mockEndpoint("GET", "/api/github/repositories", () => [
				{ name: "acme" },
			])
			const api = makeGitHubAPI()

			const result = await api.fetchGitHubRepositories.refresh()

			expect(result.data).toEqual([{ name: "acme" }])
			expect(repoCalls).toHaveLength(1)
			expect(statusCalls).toHaveLength(1)
		})
	})

	describe("useFetchGitHubBranchesByRepositoryName", () => {
		it("returns no branches when github is not connected", async ({
			expect,
		}) => {
			const statusCalls = mockEndpoint("GET", "/api/github", () => ({
				connected: false,
				configured: true,
			}))
			const branchCalls = mockEndpoint(
				"GET",
				"/api/github/repositories/acme/branches",
				() => [],
			)
			const api = makeGitHubAPI()
			const branches = runInApp(() =>
				api.useFetchGitHubBranchesByRepositoryName("acme"),
			)

			const result = await branches.refresh()

			expect(result.data).toEqual([])
			expect(branchCalls).toHaveLength(0)
			expect(statusCalls).toHaveLength(1)
		})

		it("returns no branches without a repository name", async ({ expect }) => {
			const statusCalls = mockEndpoint("GET", "/api/github", () => ({
				connected: true,
				configured: true,
			}))
			const api = makeGitHubAPI()
			const branches = runInApp(() =>
				api.useFetchGitHubBranchesByRepositoryName(null),
			)

			const result = await branches.refresh()

			expect(result.data).toEqual([])
			expect(statusCalls).toHaveLength(1)
		})

		it("fetches the branches of the repository", async ({ expect }) => {
			const statusCalls = mockEndpoint("GET", "/api/github", () => ({
				connected: true,
				configured: true,
			}))
			const branchCalls = mockEndpoint(
				"GET",
				"/api/github/repositories/acme/branches",
				() => ["main", "dev"],
			)
			const api = makeGitHubAPI()
			const branches = runInApp(() =>
				api.useFetchGitHubBranchesByRepositoryName("acme"),
			)

			const result = await branches.refresh()

			expect(result.data).toEqual(["main", "dev"])
			expect(branchCalls).toHaveLength(1)
			expect(statusCalls).toHaveLength(1)
		})
	})

	describe("useFetchGitHubFileTreeByRepositoryNameAndBranch", () => {
		it("returns no tree items when github is not connected", async ({
			expect,
		}) => {
			const statusCalls = mockEndpoint("GET", "/api/github", () => ({
				connected: false,
				configured: true,
			}))
			const treeCalls = mockEndpoint(
				"GET",
				"/api/github/repositories/acme/tree",
				() => [],
			)
			const api = makeGitHubAPI()
			const tree = runInApp(() =>
				api.useFetchGitHubFileTreeByRepositoryNameAndBranch("acme", "main"),
			)

			const result = await tree.refresh()

			expect(result.data).toEqual([])
			expect(treeCalls).toHaveLength(0)
			expect(statusCalls).toHaveLength(1)
		})

		it("returns no tree items without a repository name", async ({
			expect,
		}) => {
			const statusCalls = mockEndpoint("GET", "/api/github", () => ({
				connected: true,
				configured: true,
			}))
			const api = makeGitHubAPI()
			const tree = runInApp(() =>
				api.useFetchGitHubFileTreeByRepositoryNameAndBranch(null, "main"),
			)

			const result = await tree.refresh()

			expect(result.data).toEqual([])
			expect(statusCalls).toHaveLength(1)
		})

		it("fetches the file tree of the branch", async ({ expect }) => {
			const statusCalls = mockEndpoint("GET", "/api/github", () => ({
				connected: true,
				configured: true,
			}))
			const treeCalls = mockEndpoint(
				"GET",
				"/api/github/repositories/acme/tree",
				() => [{ path: "README.md" }],
			)
			const api = makeGitHubAPI()
			const tree = runInApp(() =>
				api.useFetchGitHubFileTreeByRepositoryNameAndBranch("acme", "main"),
			)

			const result = await tree.refresh()

			expect(result.data).toEqual([{ path: "README.md" }])
			expect(treeCalls).toHaveLength(1)
			expect(treeCalls[0]?.query).toEqual({ branch: "main" })
			expect(statusCalls).toHaveLength(1)
		})

		it("fetches the default branch tree when no branch is given", async ({
			expect,
		}) => {
			const statusCalls = mockEndpoint("GET", "/api/github", () => ({
				connected: true,
				configured: true,
			}))
			const treeCalls = mockEndpoint(
				"GET",
				"/api/github/repositories/acme/tree",
				() => [],
			)
			const api = makeGitHubAPI()
			const tree = runInApp(() =>
				api.useFetchGitHubFileTreeByRepositoryNameAndBranch("acme", null),
			)

			await tree.refresh()

			expect(treeCalls).toHaveLength(1)
			expect(treeCalls[0]?.query).toEqual({})
			expect(statusCalls).toHaveLength(1)
		})
	})
})
