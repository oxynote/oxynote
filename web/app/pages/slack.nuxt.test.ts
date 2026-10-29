import { mockNuxtImport, mountSuspended } from "@nuxt/test-utils/runtime"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import type { LocationQuery } from "vue-router"
import {
	clearQueryCache,
	disposeMockEndpoints,
	mockEndpoint,
	seedQueryData,
} from "~/composables/api/test-helpers"
import Slack from "./slack.vue"
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
describe("<slack>", { concurrent: false }, () => {
	// restoreMocks does not touch hand-made vi.fn() singletons in vitest 4
	beforeEach(() => {
		clearQueryCache()
		useRouteMock.mockReset()
		seedCapabilities()
		// a fresh status entry keeps the query's own first load from
		// running, so the status calls count only the page's refetch
		seedQueryData(["slack", "connected"], {
			connected: false,
			configured: true,
		})
	})

	afterEach(disposeMockEndpoints)

	it("connects the workspace and refreshes the connection status", async ({
		expect,
	}) => {
		const endpoints = mockSlackEndpoints()

		const wrapper = await mountSlack({
			code: "c1",
			state: "s1",
			scope: ["chat:write", null, "commands"],
			flag: null,
		})

		expect.soft(wrapper.text()).toContain(t("apps.slack-connect.success"))
		expect.soft(endpoints.connect).toHaveLength(1)
		expect.soft(endpoints.connect[0]?.query).toEqual({
			code: "c1",
			state: "s1",
			scope: ["chat:write", "commands"],
		})
		expect.soft(endpoints.status).toHaveLength(1)
		expect.soft(endpoints.link).toHaveLength(0)
	})

	it("reports an error when the workspace connection is refused", async ({
		expect,
	}) => {
		const endpoints = mockSlackEndpoints({ refuse: true })

		const wrapper = await mountSlack({ code: "c1", state: "s1" })

		expect.soft(wrapper.text()).toContain(t("apps.slack-connect.error"))
		expect.soft(endpoints.connect).toHaveLength(1)
		expect.soft(endpoints.status).toHaveLength(0)
		expect.soft(endpoints.link).toHaveLength(0)
	})

	it("links the Slack user when the callback names one", async ({ expect }) => {
		const endpoints = mockSlackEndpoints()

		const wrapper = await mountSlack({ user: "U1", state: "s1" })

		expect.soft(wrapper.text()).toContain(t("apps.slack-connect.success"))
		expect.soft(endpoints.link).toHaveLength(1)
		expect.soft(endpoints.link[0]?.query).toEqual({ user: "U1", state: "s1" })
		expect.soft(endpoints.connect).toHaveLength(0)
		expect.soft(endpoints.status).toHaveLength(0)
	})

	it("reports an error when the user link is refused", async ({ expect }) => {
		const endpoints = mockSlackEndpoints({ refuse: true })

		const wrapper = await mountSlack({ user: "U1", state: "s1" })

		expect.soft(wrapper.text()).toContain(t("apps.slack-connect.error"))
		expect.soft(endpoints.link).toHaveLength(1)
		expect.soft(endpoints.connect).toHaveLength(0)
		expect.soft(endpoints.status).toHaveLength(0)
	})
})

// ofetch retries a GET once on a 500, so a refusal is a 403
function mockSlackEndpoints(options: { refuse?: boolean } = {}) {
	const respond = () => {
		if (options.refuse) {
			throw createError({ statusCode: 403 })
		}

		return {}
	}

	return {
		status: mockEndpoint("GET", "/api/slack", () => ({
			connected: true,
			configured: true,
		})),
		connect: mockEndpoint("GET", "/api/slack/connect", respond),
		link: mockEndpoint("GET", "/api/slack/users/link", respond),
	}
}

async function mountSlack(query: LocationQuery) {
	useRouteMock.mockReturnValue({ query: query })

	const wrapper = await mountSuspended(Slack)
	await settleMutations()

	return wrapper
}
