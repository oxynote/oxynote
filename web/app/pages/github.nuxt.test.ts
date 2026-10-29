import { mockNuxtImport, mountSuspended } from "@nuxt/test-utils/runtime"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import type { LocationQuery } from "vue-router"
import {
	clearQueryCache,
	disposeMockEndpoints,
	mockEndpoint,
	seedQueryData,
} from "~/composables/api/test-helpers"
import GitHub from "./github.vue"
import {
	seedCapabilities,
	settleMutations,
	t,
} from "../components/test-helpers"

// route.query is a readonly proxy, so the route itself is what a test
// arranges
const { useRouteMock } = vi.hoisted(() => ({ useRouteMock: vi.fn() }))

mockNuxtImport("useRoute", () => useRouteMock)

// the useRoute module mock and the query cache are app-wide singletons
// every mount in the file shares
describe("<github>", { concurrent: false }, () => {
	// restoreMocks does not touch hand-made vi.fn() singletons in vitest 4
	beforeEach(() => {
		clearQueryCache()
		useRouteMock.mockReset()
		seedCapabilities()
		// a fresh status entry keeps the query's own first load from
		// running, so the status calls count only the page's refetch
		seedQueryData(["github", "connected"], {
			connected: false,
			configured: true,
		})
	})

	afterEach(disposeMockEndpoints)

	it.for<{ name: string; input: LocationQuery }>([
		{
			name: "reports an error when the callback carries no installation id",
			input: { state: "s1" },
		},
		{
			name: "reports an error when the callback carries no state",
			input: { installation_id: "42" },
		},
	])("$name", async ({ input }, { expect }) => {
		const statusCalls = mockStatus()
		const connectCalls = mockConnect(() => ({}))

		const wrapper = await mountGitHub(input)

		expect.soft(wrapper.text()).toContain(t("apps.github-connect.error"))
		expect.soft(connectCalls).toHaveLength(0)
		expect.soft(statusCalls).toHaveLength(0)
	})

	it("connects the installation and refreshes the connection status", async ({
		expect,
	}) => {
		const statusCalls = mockStatus()
		const connectCalls = mockConnect(() => ({}))

		const wrapper = await mountGitHub({
			installation_id: "42",
			state: "s1",
			setup_action: "install",
		})

		expect.soft(wrapper.text()).toContain(t("apps.github-connect.success"))
		expect.soft(connectCalls).toHaveLength(1)
		expect.soft(connectCalls[0]?.query).toEqual({
			installation_id: "42",
			state: "s1",
			setup_action: "install",
		})
		expect.soft(statusCalls).toHaveLength(1)
	})

	it("forwards every value of a repeated query parameter and drops valueless ones", async ({
		expect,
	}) => {
		mockStatus()
		const connectCalls = mockConnect(() => ({}))

		await mountGitHub({
			installation_id: "42",
			state: "s1",
			scope: ["repo", null, "workflow"],
			flag: null,
		})

		expect(connectCalls).toHaveLength(1)
		expect(connectCalls[0]?.query).toEqual({
			installation_id: "42",
			state: "s1",
			scope: ["repo", "workflow"],
		})
	})

	it("reports an error when the connection is refused", async ({ expect }) => {
		const statusCalls = mockStatus()
		// ofetch retries a GET once on a 500, so the refusal is a 403
		const connectCalls = mockConnect(() => {
			throw createError({ statusCode: 403 })
		})

		const wrapper = await mountGitHub({ installation_id: "42", state: "s1" })

		expect.soft(wrapper.text()).toContain(t("apps.github-connect.error"))
		expect.soft(wrapper.text()).not.toContain(t("apps.github-connect.success"))
		expect.soft(connectCalls).toHaveLength(1)
		expect.soft(statusCalls).toHaveLength(0)
	})
})

function mockConnect(respond: () => unknown) {
	return mockEndpoint("GET", "/api/github/connect", respond)
}

function mockStatus() {
	return mockEndpoint("GET", "/api/github", () => ({
		connected: true,
		configured: true,
	}))
}

async function mountGitHub(query: LocationQuery) {
	useRouteMock.mockReturnValue({ query: query })

	const wrapper = await mountSuspended(GitHub)
	await settleMutations()

	return wrapper
}
