import { afterEach, beforeEach, describe, it, vi } from "vitest"
import { toast } from "vue-sonner"
import {
	clearQueryCache,
	disposeMockEndpoints,
	mockEndpoint,
	seedQueryData,
} from "~/composables/api/test-helpers"
import { mockNuxtImport } from "@nuxt/test-utils/runtime"
import AppSidebar from "./AppSidebar.vue"
import {
	emitFrom,
	mockAuthEndpoint,
	mountUnderSidebarProvider,
	seedAuthOrganization,
	settleMutations,
	stubViewportMatches,
} from "./test-helpers"

vi.mock("vue-sonner", () => ({
	toast: { custom: vi.fn(), dismiss: vi.fn() },
}))

// signing out navigates, and the test app has no login page to land on
const navigateToMock = vi.hoisted(() => vi.fn())
mockNuxtImport("navigateTo", () => navigateToMock)

const DOC_ID = "doc1".padEnd(20, "0")

function treeElement() {
	return {
		id: DOC_ID,
		documentName: "Runbook",
		icon: "lucide:file",
		protected: false,
		children: null,
	}
}

// AppSidebar refreshes the tree, the organization and the unread count on
// mount, so every one of them needs an endpoint behind it
function stubQueries(
	options: {
		tree?: unknown[]
		unread?: number
		gitHub?: { connected: boolean; configured: boolean }
		slack?: { connected: boolean; configured: boolean }
	} = {},
) {
	mockEndpoint("GET", "/api/documents/tree", () => options.tree ?? [])
	mockEndpoint("GET", "/api/notifications/count", () => ({
		count: options.unread ?? 0,
	}))
	seedQueryData(
		["github", "connected"],
		options.gitHub ?? { connected: true, configured: true },
	)
	seedQueryData(
		["slack", "connected"],
		options.slack ?? { connected: true, configured: true },
	)
}

function mountSidebar(props: Record<string, unknown> = {}) {
	return mountUnderSidebarProvider(AppSidebar, {
		props: {
			allInitialSectionsLoaded: true,
			notificationSidebarOpen: false,
			...props,
		},
	})
}

function sidebar(wrapper: Awaited<ReturnType<typeof mountSidebar>>) {
	return wrapper.findComponent(AppSidebar)
}

function itemNamed(
	wrapper: Awaited<ReturnType<typeof mountSidebar>>,
	name: string,
) {
	const item = wrapper
		.findAll("[data-slot='sidebar-menu-button']")
		.find((el) => el.text().includes(name))
	if (!item) {
		throw new Error(`no sidebar item rendering "${name}"`)
	}

	return item
}

// the query cache, the editor store and the vue-sonner module mock are
// app-wide singletons every mount in the file shares
describe("<AppSidebar>", { concurrent: false }, () => {
	beforeEach(() => {
		clearQueryCache()
		vi.mocked(toast.custom).mockReset()
		navigateToMock.mockReset()
		// the sidebar renders into a sheet on narrow viewports; the wide
		// layout is the one these tests read
		stubViewportMatches(false)
		seedAuthOrganization({
			id: "org-1",
			name: "Acme Corp",
			slug: "acme-corp",
			logo: "",
			members: [{ id: "m1", userId: "u1", user: { name: "Ada" } }],
			invitations: [],
		})
	})

	afterEach(disposeMockEndpoints)

	it("stays blank until the initial sections have loaded", async ({
		expect,
	}) => {
		stubQueries()

		const wrapper = await mountSidebar({ allInitialSectionsLoaded: false })

		expect(sidebar(wrapper).text()).toBe("")
	})

	it("shows the workspace header", async ({ expect }) => {
		stubQueries()

		const wrapper = await mountSidebar()

		expect(wrapper.text()).toContain("Acme Corp")
	})

	it("offers search and the inbox at the top", async ({ expect }) => {
		stubQueries()

		const wrapper = await mountSidebar()

		expect(wrapper.text()).toContain("Search")
		expect(wrapper.text()).toContain("Inbox")
	})

	it("shows the unread notification count on the inbox row", async ({
		expect,
	}) => {
		stubQueries({ unread: 4 })

		const wrapper = await mountSidebar()
		await settleMutations()

		expect(itemNamed(wrapper, "Inbox").text()).toContain("4")
	})

	it("marks the inbox row active while the notification sidebar is open", async ({
		expect,
	}) => {
		stubQueries()

		const wrapper = await mountSidebar({ notificationSidebarOpen: true })

		expect(itemNamed(wrapper, "Inbox").attributes("data-active")).toBe("true")
	})

	it("lists the workspace pages under their own heading", async ({
		expect,
	}) => {
		stubQueries({ tree: [treeElement()] })

		const wrapper = await mountSidebar()
		await settleMutations()

		expect(wrapper.text()).toContain("Workspace")
		expect(wrapper.text()).toContain("Runbook")
	})

	it("shows a placeholder row when the workspace has no pages", async ({
		expect,
	}) => {
		stubQueries()

		const wrapper = await mountSidebar()
		await settleMutations()

		expect(wrapper.text()).toContain("Add Page")
	})

	it("asks to open the search modal from the search row", async ({
		expect,
	}) => {
		stubQueries()
		const wrapper = await mountSidebar()

		await itemNamed(wrapper, "Search").trigger("click")

		expect(
			document.body.querySelector("[data-slot='dialog-content']"),
		).not.toBeNull()
	})

	it("asks to toggle the notification sidebar from the inbox row", async ({
		expect,
	}) => {
		stubQueries()
		const wrapper = await mountSidebar()

		await itemNamed(wrapper, "Inbox").trigger("click")

		expect(sidebar(wrapper).emitted("toggle-notifications")).toHaveLength(1)
	})

	it("asks to create a page from the workspace heading action", async ({
		expect,
	}) => {
		stubQueries()
		const wrapper = await mountSidebar()

		await wrapper.get("[data-sidebar='group-action']").trigger("click")

		expect(sidebar(wrapper).emitted("create-document")).toEqual([[null]])
	})

	it("asks to create a page from the workspace header", async ({ expect }) => {
		stubQueries()
		const wrapper = await mountSidebar()

		emitFrom(wrapper, "AppSidebarHeader", "create-new-item")
		await nextTick()

		expect(sidebar(wrapper).emitted("create-document")).toEqual([[null]])
	})

	it("asks to open the settings from the workspace header", async ({
		expect,
	}) => {
		stubQueries()
		const wrapper = await mountSidebar()

		emitFrom(wrapper, "AppSidebarHeader", "open-settings")
		await nextTick()

		expect(sidebar(wrapper).emitted("open-settings")).toEqual([[null]])
	})

	it("sends the user to the login page after signing out", async ({
		expect,
	}) => {
		stubQueries()
		mockAuthEndpoint("sign-out", () => ({ success: true }))
		const wrapper = await mountSidebar()

		emitFrom(wrapper, "AppSidebarHeader", "log-out")
		await settleMutations()

		expect(navigateToMock).toHaveBeenCalledExactlyOnceWith({ name: "login" })
		expect(toast.custom).toHaveBeenCalledTimes(0)
	})

	it("warns and stays put when signing out fails", async ({ expect }) => {
		stubQueries()
		mockAuthEndpoint("sign-out", () => {
			throw new Error("boom")
		})
		const wrapper = await mountSidebar()

		emitFrom(wrapper, "AppSidebarHeader", "log-out")
		await settleMutations()

		expect(toast.custom).toHaveBeenCalledTimes(1)
		expect(navigateToMock).toHaveBeenCalledTimes(0)
	})

	it("announces that every section has loaded", async ({ expect }) => {
		stubQueries()

		const wrapper = await mountSidebar()
		await settleMutations()

		expect(sidebar(wrapper).emitted("initial-load-complete")).toHaveLength(1)
	})

	describe("next steps", { concurrent: false }, () => {
		it("invites the user to add team members while alone", async ({
			expect,
		}) => {
			stubQueries()

			const wrapper = await mountSidebar()
			await settleMutations()

			expect(wrapper.text()).toContain("Invite Team Members")
		})

		it("drops the invite row once the workspace has more members", async ({
			expect,
		}) => {
			seedAuthOrganization({
				id: "org-1",
				name: "Acme Corp",
				members: [
					{ id: "m1", userId: "u1", user: { name: "Ada" } },
					{ id: "m2", userId: "u2", user: { name: "Linus" } },
				],
				invitations: [],
			})
			stubQueries()

			const wrapper = await mountSidebar()
			await settleMutations()

			expect(wrapper.text()).not.toContain("Invite Team Members")
		})

		it("opens the member settings from the invite row", async ({ expect }) => {
			stubQueries()
			const wrapper = await mountSidebar()
			await settleMutations()

			await itemNamed(wrapper, "Invite Team Members").trigger("click")

			expect(sidebar(wrapper).emitted("open-settings")).toEqual([
				["org-members"],
			])
		})

		it("offers the github integration while it is unconnected", async ({
			expect,
		}) => {
			stubQueries({ gitHub: { connected: false, configured: true } })

			const wrapper = await mountSidebar()
			await settleMutations()

			expect(wrapper.text()).toContain("Activate GitHub Integration")
		})

		it("hides the github integration on a deployment without it", async ({
			expect,
		}) => {
			stubQueries({ gitHub: { connected: false, configured: false } })

			const wrapper = await mountSidebar()
			await settleMutations()

			expect(wrapper.text()).not.toContain("Activate GitHub Integration")
		})

		it("offers the slack integration while it is unconnected", async ({
			expect,
		}) => {
			stubQueries({ slack: { connected: false, configured: true } })

			const wrapper = await mountSidebar()
			await settleMutations()

			expect(wrapper.text()).toContain("Activate Slack Integration")
		})

		it("hides the slack integration on a deployment without it", async ({
			expect,
		}) => {
			stubQueries({ slack: { connected: false, configured: false } })

			const wrapper = await mountSidebar()
			await settleMutations()

			expect(wrapper.text()).not.toContain("Activate Slack Integration")
		})

		it("hides the whole section once there is nothing left to do", async ({
			expect,
		}) => {
			seedAuthOrganization({
				id: "org-1",
				name: "Acme Corp",
				members: [
					{ id: "m1", userId: "u1", user: { name: "Ada" } },
					{ id: "m2", userId: "u2", user: { name: "Linus" } },
				],
				invitations: [],
			})
			stubQueries()

			const wrapper = await mountSidebar()
			await settleMutations()

			expect(wrapper.text()).not.toContain("Next Steps")
		})

		it("opens the github install page in the browser", async ({ expect }) => {
			stubQueries({ gitHub: { connected: false, configured: true } })
			mockEndpoint("GET", "/api/github/install", () => ({
				url: "https://github.test/install",
			}))
			const open = vi.fn()
			vi.stubGlobal("open", open)
			const wrapper = await mountSidebar()
			await settleMutations()

			await itemNamed(wrapper, "Activate GitHub Integration").trigger("click")
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
			stubQueries({ gitHub: { connected: false, configured: true } })
			mockEndpoint("GET", "/api/github/install", () => {
				throw new Error("boom")
			})
			const wrapper = await mountSidebar()
			await settleMutations()

			await itemNamed(wrapper, "Activate GitHub Integration").trigger("click")
			await settleMutations()

			expect(toast.custom).toHaveBeenCalledTimes(1)
		})

		it("opens the slack install page in the browser", async ({ expect }) => {
			stubQueries({ slack: { connected: false, configured: true } })
			mockEndpoint("GET", "/api/slack/install", () => ({
				url: "https://slack.test/install",
			}))
			const open = vi.fn()
			vi.stubGlobal("open", open)
			const wrapper = await mountSidebar()
			await settleMutations()

			await itemNamed(wrapper, "Activate Slack Integration").trigger("click")
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
			stubQueries({ slack: { connected: false, configured: true } })
			mockEndpoint("GET", "/api/slack/install", () => {
				throw new Error("boom")
			})
			const wrapper = await mountSidebar()
			await settleMutations()

			await itemNamed(wrapper, "Activate Slack Integration").trigger("click")
			await settleMutations()

			expect(toast.custom).toHaveBeenCalledTimes(1)
		})
	})

	describe("page moves", { concurrent: false }, () => {
		it("persists a move requested by the page tree", async ({ expect }) => {
			stubQueries({ tree: [treeElement()] })
			const calls = mockEndpoint("PUT", "/api/documents/tree", () => ({}))
			const wrapper = await mountSidebar()
			await settleMutations()

			emitFrom(wrapper, "SidebarNestedGroup", "update-location", {
				id: DOC_ID,
				parentId: null,
				insertBeforeId: null,
			})
			await settleMutations()

			expect(calls).toHaveLength(1)
			expect(toast.custom).toHaveBeenCalledTimes(0)
		})

		it("warns when the move cannot be persisted", async ({ expect }) => {
			stubQueries({ tree: [treeElement()] })
			mockEndpoint("PUT", "/api/documents/tree", () => {
				throw new Error("boom")
			})
			const wrapper = await mountSidebar()
			await settleMutations()

			emitFrom(wrapper, "SidebarNestedGroup", "update-location", {
				id: DOC_ID,
				parentId: null,
				insertBeforeId: null,
			})
			await settleMutations()

			expect(toast.custom).toHaveBeenCalledTimes(1)
		})

		it("passes a deletion request on to its parent", async ({ expect }) => {
			stubQueries()
			const wrapper = await mountSidebar()
			await settleMutations()

			emitFrom(wrapper, "SidebarNestedGroup", "delete", { id: DOC_ID })
			await nextTick()

			expect(sidebar(wrapper).emitted("delete-document")).toEqual([[DOC_ID]])
		})

		it("passes a duplication request on to its parent", async ({ expect }) => {
			stubQueries()
			const wrapper = await mountSidebar()
			await settleMutations()

			emitFrom(wrapper, "SidebarNestedGroup", "duplicate", { id: DOC_ID })
			await nextTick()

			expect(sidebar(wrapper).emitted("duplicate-document")).toEqual([[DOC_ID]])
		})
	})
})
