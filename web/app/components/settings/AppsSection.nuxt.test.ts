import { mountSuspended } from "@nuxt/test-utils/runtime"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import { toast } from "vue-sonner"
import {
	clearQueryCache,
	disposeMockEndpoints,
	mockEndpoint,
	seedQueryData,
} from "~/composables/api/test-helpers"
import AppsSection from "./AppsSection.vue"
import { findButtonByText, settleMutations } from "../test-helpers"

vi.mock("vue-sonner", () => ({
	toast: { custom: vi.fn(), dismiss: vi.fn() },
}))

function seedApps(options: {
	gitHub?: { connected: boolean; configured: boolean }
	slack?: { connected: boolean; configured: boolean }
}) {
	seedQueryData(
		["github", "connected"],
		options.gitHub ?? { connected: false, configured: true },
	)
	seedQueryData(
		["slack", "connected"],
		options.slack ?? { connected: false, configured: true },
	)
}

function mountSection() {
	return mountSuspended(AppsSection)
}

// rows are told apart by the app they name; both offer the same actions
function row(wrapper: Awaited<ReturnType<typeof mountSection>>, app: string) {
	const found = wrapper
		.findAll("div.flex.w-full.items-center")
		.find((el) => el.text().includes(app))
	if (!found) {
		throw new Error(`no app row for "${app}"`)
	}

	return found
}

// the query cache and the vue-sonner module mock are app-wide singletons
// every mount in the file shares
describe("<AppsSection>", { concurrent: false }, () => {
	beforeEach(() => {
		clearQueryCache()
		vi.mocked(toast.custom).mockReset()
		mockEndpoint("GET", "/api/github", () => ({
			connected: false,
			configured: true,
		}))
		mockEndpoint("GET", "/api/slack", () => ({
			connected: false,
			configured: true,
		}))
	})

	afterEach(disposeMockEndpoints)

	it("lists both integrations on a deployment that has them", async ({
		expect,
	}) => {
		seedApps({})

		const wrapper = await mountSection()

		expect(wrapper.text()).toContain("GitHub")
		expect(wrapper.text()).toContain("Slack")
	})

	it("hides github on a deployment without it", async ({ expect }) => {
		seedApps({ gitHub: { connected: false, configured: false } })

		const wrapper = await mountSection()

		expect(wrapper.text()).not.toContain("GitHub")
	})

	it("hides slack on a deployment without it", async ({ expect }) => {
		seedApps({ slack: { connected: false, configured: false } })

		const wrapper = await mountSection()

		expect(wrapper.text()).not.toContain("Slack")
	})

	it("offers to connect an unconnected integration", async ({ expect }) => {
		seedApps({})

		const wrapper = await mountSection()

		expect(row(wrapper, "GitHub").text()).toContain("Connect")
	})

	it("reports a connected integration and offers to disconnect it", async ({
		expect,
	}) => {
		seedApps({ gitHub: { connected: true, configured: true } })

		const wrapper = await mountSection()

		expect(row(wrapper, "GitHub").text()).toContain("Connected")
		expect(row(wrapper, "GitHub").text()).toContain("Disconnect")
	})

	it("opens the github install page in the browser", async ({ expect }) => {
		seedApps({})
		mockEndpoint("GET", "/api/github/install", () => ({
			url: "https://github.test/install",
		}))
		const open = vi.fn()
		vi.stubGlobal("open", open)
		const wrapper = await mountSection()

		await row(wrapper, "GitHub").get("button").trigger("click")
		await settleMutations()

		expect(open).toHaveBeenCalledExactlyOnceWith(
			"https://github.test/install",
			"_blank",
			"noopener",
		)
	})

	it("warns when the github install url cannot be fetched", async ({
		expect,
	}) => {
		seedApps({})
		mockEndpoint("GET", "/api/github/install", () => {
			throw new Error("boom")
		})
		const wrapper = await mountSection()

		await row(wrapper, "GitHub").get("button").trigger("click")
		await settleMutations()

		expect(toast.custom).toHaveBeenCalledTimes(1)
	})

	it("opens the slack install page in the browser", async ({ expect }) => {
		seedApps({})
		mockEndpoint("GET", "/api/slack/install", () => ({
			url: "https://slack.test/install",
		}))
		const open = vi.fn()
		vi.stubGlobal("open", open)
		const wrapper = await mountSection()

		await row(wrapper, "Slack").get("button").trigger("click")
		await settleMutations()

		expect(open).toHaveBeenCalledExactlyOnceWith(
			"https://slack.test/install",
			"_blank",
			"noopener",
		)
	})

	it("warns when the slack install url cannot be fetched", async ({
		expect,
	}) => {
		seedApps({})
		mockEndpoint("GET", "/api/slack/install", () => {
			throw new Error("boom")
		})
		const wrapper = await mountSection()

		await row(wrapper, "Slack").get("button").trigger("click")
		await settleMutations()

		expect(toast.custom).toHaveBeenCalledTimes(1)
	})

	it("disconnects github", async ({ expect }) => {
		seedApps({ gitHub: { connected: true, configured: true } })
		const calls = mockEndpoint("DELETE", "/api/github", () => ({}))
		const wrapper = await mountSection()

		await findButtonByText(wrapper, "Disconnect").trigger("click")
		await settleMutations()

		expect(calls).toHaveLength(1)
		expect(toast.custom).toHaveBeenCalledTimes(0)
	})

	it("warns when github cannot be disconnected", async ({ expect }) => {
		seedApps({ gitHub: { connected: true, configured: true } })
		mockEndpoint("DELETE", "/api/github", () => {
			throw new Error("boom")
		})
		const wrapper = await mountSection()

		await findButtonByText(wrapper, "Disconnect").trigger("click")
		await settleMutations()

		expect(toast.custom).toHaveBeenCalledTimes(1)
	})

	it("disconnects slack", async ({ expect }) => {
		seedApps({ slack: { connected: true, configured: true } })
		const calls = mockEndpoint("DELETE", "/api/slack", () => ({}))
		const wrapper = await mountSection()

		await row(wrapper, "Slack").get("button").trigger("click")
		await settleMutations()

		expect(calls).toHaveLength(1)
		expect(toast.custom).toHaveBeenCalledTimes(0)
	})

	it("warns when slack cannot be disconnected", async ({ expect }) => {
		seedApps({ slack: { connected: true, configured: true } })
		mockEndpoint("DELETE", "/api/slack", () => {
			throw new Error("boom")
		})
		const wrapper = await mountSection()

		await row(wrapper, "Slack").get("button").trigger("click")
		await settleMutations()

		expect(toast.custom).toHaveBeenCalledTimes(1)
	})

	it("separates the two rows only when both are shown", async ({ expect }) => {
		seedApps({ slack: { connected: false, configured: false } })

		const wrapper = await mountSection()

		expect(wrapper.find(".my-3\\.5").exists()).toBe(false)
	})
})
