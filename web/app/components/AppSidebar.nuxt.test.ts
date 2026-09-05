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
	at,
	clearTeleportedOverlays,
	emitFrom,
	emitFromNth,
	menuItem,
	mockAuthEndpoint,
	mountUnderSidebarProvider,
	seedAuthOrganization,
	seedCapabilities,
	seedPersistentState,
	settleMutations,
	stubViewportMatches,
	t,
} from "./test-helpers"

vi.mock("vue-sonner", () => ({
	toast: { custom: vi.fn(), dismiss: vi.fn() },
}))

// signing out navigates, and the test app has no login page to land on
const navigateToMock = vi.hoisted(() => vi.fn())
mockNuxtImport("navigateTo", () => navigateToMock)

const DOC_ID = "doc1".padEnd(20, "0")
const BRANCH_ID = "br1".padEnd(20, "0")
const TAG_A = "taga".padEnd(20, "0")
const TAG_B = "tagb".padEnd(20, "0")

function tagElement(id: string, tagName: string, color: string) {
	return {
		id: id,
		tagName: tagName,
		color: color,
		hidden: false,
		documents: [treeElement()],
	}
}

function treeElement() {
	return {
		id: DOC_ID,
		documentName: "Runbook",
		icon: "lucide:file",
		protected: false,
		defaultBranchId: BRANCH_ID,
		children: null,
	}
}

// AppSidebar refreshes the tree, the organization and the unread count on
// mount, so every one of them needs an endpoint behind it
function stubQueries(
	options: {
		tree?: unknown[]
		tags?: unknown[]
		unread?: number
		gitHub?: { connected: boolean; configured: boolean }
		slack?: { connected: boolean; configured: boolean }
	} = {},
) {
	mockEndpoint("GET", "/api/documents/tree", () => options.tree ?? [])
	mockEndpoint("GET", "/api/tags/tree", () => options.tags ?? [])
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

// each section mounts its own root <SidebarNestedGroup> and every expanded
// row mounts another one below it, so a section's group is the first one
// rendering a row that belongs to it
function groupIndexShowing(
	wrapper: Awaited<ReturnType<typeof mountSidebar>>,
	text: string,
) {
	const index = wrapper
		.findAllComponents({ name: "SidebarNestedGroup" })
		.findIndex((group) => group.text().includes(text))
	if (index === -1) {
		throw new Error(`no sidebar group rendering "${text}"`)
	}

	return index
}

function tagsGroup(wrapper: Awaited<ReturnType<typeof mountSidebar>>) {
	const group = wrapper.findAll("[data-slot='sidebar-group']").find((el) => {
		const label = el.find("[data-slot='sidebar-group-label']")

		return label.exists() && label.text() === t("sidebar.sections.tags.heading")
	})
	if (!group) {
		throw new Error("no tags section")
	}

	return group
}

// every row starts collapsed in this suite, so the tags section renders
// its tags and nothing below them
function tagRows(wrapper: Awaited<ReturnType<typeof mountSidebar>>) {
	return tagsGroup(wrapper).findAll("[data-item-id]")
}

function tagNames(wrapper: Awaited<ReturnType<typeof mountSidebar>>) {
	return tagRows(wrapper).map((row) => row.text())
}

function tagIds(wrapper: Awaited<ReturnType<typeof mountSidebar>>) {
	return tagRows(wrapper).map((row) => row.attributes("data-item-id"))
}

// opens the options menu of the row whose own label carries the given
// text. A row element wraps the rows nested under it, so the match is made
// against its own button rather than its whole subtree.
async function openRowMenu(
	wrapper: Awaited<ReturnType<typeof mountSidebar>>,
	text: string,
) {
	const row = wrapper
		.findAll("[data-item-id]")
		.find((el) =>
			el.get("[data-slot='sidebar-menu-button']").text().includes(text),
		)
	if (!row) {
		throw new Error(`no sidebar row labelled "${text}"`)
	}

	await row.get("[data-slot='dropdown-menu-trigger']").trigger("click")
}

// expands the first tag so the documents carrying it render
async function openFirstTag(wrapper: Awaited<ReturnType<typeof mountSidebar>>) {
	await at(
		at(tagRows(wrapper), 0).findAll("[data-sidebar='menu-action']"),
		1,
	).trigger("click")
	await nextTick()
}

// the rows of whichever options menu is currently open
function openMenuRows() {
	return Array.from(document.body.querySelectorAll("[role^='menuitem']")).map(
		(el) => el.textContent.trim(),
	)
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
		// the collapse state is a persisted singleton every mount in the file
		// shares, and an empty one makes the first section that mounts with
		// expandable rows open them — seeding it non-empty keeps every row
		// closed instead, whatever the tree behind it looks like
		seedPersistentState("sidebar-item-collapse", { seeded: 1 })
		seedAuthOrganization({
			id: "org-1",
			name: "Acme Corp",
			slug: "acme-corp",
			logo: "",
			members: [{ id: "m1", userId: "u1", user: { name: "Ada" } }],
			invitations: [],
		})
	})

	afterEach(() => {
		disposeMockEndpoints()
		// the options menus teleport into the shared <body> and their ids
		// repeat across mounts, so a leftover would answer the next lookup
		clearTeleportedOverlays()
	})

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

		expect(wrapper.text()).toContain(t("sidebar.sections.top.search-button"))
		expect(wrapper.text()).toContain(t("sidebar.sections.top.inbox"))
	})

	it("shows the unread notification count on the inbox row", async ({
		expect,
	}) => {
		stubQueries({ unread: 4 })

		const wrapper = await mountSidebar()
		await settleMutations()

		expect(
			itemNamed(wrapper, t("sidebar.sections.top.inbox")).text(),
		).toContain("4")
	})

	it("marks the inbox row active while the notification sidebar is open", async ({
		expect,
	}) => {
		stubQueries()

		const wrapper = await mountSidebar({ notificationSidebarOpen: true })

		expect(
			itemNamed(wrapper, t("sidebar.sections.top.inbox")).attributes(
				"data-active",
			),
		).toBe("true")
	})

	it("lists the workspace pages under their own heading", async ({
		expect,
	}) => {
		stubQueries({ tree: [treeElement()] })

		const wrapper = await mountSidebar()
		await settleMutations()

		expect(wrapper.text()).toContain(
			t("sidebar.sections.main-workspace.heading"),
		)
		expect(wrapper.text()).toContain("Runbook")
	})

	it("shows a placeholder row when the workspace has no pages", async ({
		expect,
	}) => {
		stubQueries()

		const wrapper = await mountSidebar()
		await settleMutations()

		expect(wrapper.text()).toContain(
			t("sidebar.sections.main-workspace.heading-action-title"),
		)
	})

	it("asks to open the search modal from the search row", async ({
		expect,
	}) => {
		stubQueries()
		const wrapper = await mountSidebar()

		await itemNamed(wrapper, t("sidebar.sections.top.search-button")).trigger(
			"click",
		)

		expect(
			document.body.querySelector("[data-slot='dialog-content']"),
		).not.toBeNull()
	})

	it("asks to toggle the notification sidebar from the inbox row", async ({
		expect,
	}) => {
		stubQueries()
		const wrapper = await mountSidebar()

		await itemNamed(wrapper, t("sidebar.sections.top.inbox")).trigger("click")

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
			throw createError({ statusCode: 500 })
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

			expect(wrapper.text()).toContain(
				t("sidebar.sections.next-steps.items.invite-team-members"),
			)
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

			expect(wrapper.text()).not.toContain(
				t("sidebar.sections.next-steps.items.invite-team-members"),
			)
		})

		it("opens the member settings from the invite row", async ({ expect }) => {
			stubQueries()
			const wrapper = await mountSidebar()
			await settleMutations()

			await itemNamed(
				wrapper,
				t("sidebar.sections.next-steps.items.invite-team-members"),
			).trigger("click")

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

			expect(wrapper.text()).toContain(
				t("sidebar.sections.next-steps.items.connect-github"),
			)
		})

		it("hides the github integration on a deployment without it", async ({
			expect,
		}) => {
			stubQueries({})
			seedCapabilities({ github: false })

			const wrapper = await mountSidebar()
			await settleMutations()

			expect(wrapper.text()).not.toContain(
				t("sidebar.sections.next-steps.items.connect-github"),
			)
		})

		it("offers the slack integration while it is unconnected", async ({
			expect,
		}) => {
			stubQueries({ slack: { connected: false, configured: true } })

			const wrapper = await mountSidebar()
			await settleMutations()

			expect(wrapper.text()).toContain(
				t("sidebar.sections.next-steps.items.connect-slack"),
			)
		})

		it("hides the slack integration on a deployment without it", async ({
			expect,
		}) => {
			stubQueries({})
			seedCapabilities({ slack: false })

			const wrapper = await mountSidebar()
			await settleMutations()

			expect(wrapper.text()).not.toContain(
				t("sidebar.sections.next-steps.items.connect-slack"),
			)
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

			expect(wrapper.text()).not.toContain(
				t("sidebar.sections.next-steps.heading"),
			)
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

			await itemNamed(
				wrapper,
				t("sidebar.sections.next-steps.items.connect-github"),
			).trigger("click")
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
				throw createError({ statusCode: 500 })
			})
			const wrapper = await mountSidebar()
			await settleMutations()

			await itemNamed(
				wrapper,
				t("sidebar.sections.next-steps.items.connect-github"),
			).trigger("click")
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

			await itemNamed(
				wrapper,
				t("sidebar.sections.next-steps.items.connect-slack"),
			).trigger("click")
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
				throw createError({ statusCode: 500 })
			})
			const wrapper = await mountSidebar()
			await settleMutations()

			await itemNamed(
				wrapper,
				t("sidebar.sections.next-steps.items.connect-slack"),
			).trigger("click")
			await settleMutations()

			expect(toast.custom).toHaveBeenCalledTimes(1)
		})
	})

	// the section only renders once core answers with tags, so every test
	// in here supplies its own
	function seededTags() {
		return [
			tagElement(TAG_A, "Production", "#22c55e"),
			tagElement(TAG_B, "Staging", "#f97316"),
		]
	}

	describe("tags", { concurrent: false }, () => {
		it("lists the tags under their own heading", async ({ expect }) => {
			stubQueries({ tags: seededTags() })

			const wrapper = await mountSidebar()
			await settleMutations()

			expect(wrapper.text()).toContain(t("sidebar.sections.tags.heading"))
			expect(tagNames(wrapper).length).toBeGreaterThan(1)
		})

		it("marks every tag with its own colour", async ({ expect }) => {
			stubQueries({ tags: seededTags() })

			const wrapper = await mountSidebar()
			await settleMutations()

			const colors = tagRows(wrapper).map(
				(row) => row.find("span[style]").attributes("style") ?? "",
			)

			expect(colors.every((color) => color.includes("background-color"))).toBe(
				true,
			)
			expect(new Set(colors).size).toBe(colors.length)
		})

		it("lists the documents carrying a tag once it is opened", async ({
			expect,
		}) => {
			stubQueries({ tags: seededTags() })
			const wrapper = await mountSidebar()
			await settleMutations()
			const closed = tagRows(wrapper).length

			await at(
				at(tagRows(wrapper), 0).findAll("[data-sidebar='menu-action']"),
				1,
			).trigger("click")
			await nextTick()

			expect(tagRows(wrapper).length).toBeGreaterThan(closed)
		})

		it("reorders the tags without moving anything in the page tree", async ({
			expect,
		}) => {
			stubQueries({ tree: [treeElement()], tags: seededTags() })
			const tagCalls = mockEndpoint("PUT", "/api/tags/tree", () => ({}))
			const treeCalls = mockEndpoint("PUT", "/api/documents/tree", () => ({}))
			const wrapper = await mountSidebar()
			await settleMutations()
			const names = tagNames(wrapper)

			emitFromNth(
				wrapper,
				"SidebarNestedGroup",
				groupIndexShowing(wrapper, at(names, 0)),
				"update-location",
				{
					id: at(tagIds(wrapper), names.length - 1),
					parentId: null,
					insertBeforeId: at(tagIds(wrapper), 0),
				},
			)
			await settleMutations()

			// the move goes to the tag endpoint, resolved to the index the
			// dropped-on tag held, and the page tree is left alone
			expect(tagCalls).toHaveLength(1)
			expect(tagCalls[0]?.body).toEqual({ id: TAG_B, sortIndex: 0 })
			expect(treeCalls).toHaveLength(0)
			expect(toast.custom).toHaveBeenCalledTimes(0)
		})

		it("hides a tag from its options menu", async ({ expect }) => {
			stubQueries({ tags: seededTags() })
			const calls = mockEndpoint(
				"PUT",
				`/api/tags/${TAG_A}/visibility`,
				() => ({}),
			)
			const wrapper = await mountSidebar()
			await settleMutations()

			await openRowMenu(wrapper, "Production")
			menuItem(t("sidebar.item-dropdown-menu-buttons.hide-tag")).click()
			await settleMutations()

			expect(calls).toHaveLength(1)
			expect(calls[0]?.body).toEqual({ hidden: true })
		})

		it("deletes a tag from its options menu", async ({ expect }) => {
			stubQueries({ tags: seededTags() })
			const calls = mockEndpoint("DELETE", `/api/tags/${TAG_A}`, () => ({}))
			const wrapper = await mountSidebar()
			await settleMutations()

			await openRowMenu(wrapper, "Production")
			menuItem(t("sidebar.item-dropdown-menu-buttons.delete-tag")).click()
			await settleMutations()

			expect(calls).toHaveLength(1)
		})

		// the tree lists a document under a tag by its default branch, so
		// that is the branch the row detaches
		it("detaches a document's default branch from the tag listing it", async ({
			expect,
		}) => {
			stubQueries({ tags: seededTags() })
			const calls = mockEndpoint(
				"DELETE",
				`/api/documents/${DOC_ID}/branches/${BRANCH_ID}/tags/${TAG_A}`,
				() => ({}),
			)
			const wrapper = await mountSidebar()
			await settleMutations()

			await openFirstTag(wrapper)
			await openRowMenu(wrapper, "Runbook")
			menuItem(t("sidebar.item-dropdown-menu-buttons.remove-tag")).click()
			await settleMutations()

			expect(calls).toHaveLength(1)
		})

		it("offers a tagged document only its detach action", async ({
			expect,
		}) => {
			stubQueries({ tags: seededTags() })
			const wrapper = await mountSidebar()
			await settleMutations()

			await openFirstTag(wrapper)
			await openRowMenu(wrapper, "Runbook")

			expect(openMenuRows()).toEqual([
				t("sidebar.item-dropdown-menu-buttons.remove-tag"),
			])
		})

		it("offers a tag its own two actions", async ({ expect }) => {
			stubQueries({ tags: seededTags() })
			const wrapper = await mountSidebar()
			await settleMutations()

			await openRowMenu(wrapper, "Production")

			expect(openMenuRows()).toEqual([
				t("sidebar.item-dropdown-menu-buttons.hide-tag"),
				t("sidebar.item-dropdown-menu-buttons.delete-tag"),
			])
		})

		it("toggles a tag's visibility from the section heading", async ({
			expect,
		}) => {
			stubQueries({
				tags: [
					tagElement(TAG_A, "Production", "#22c55e"),
					{ ...tagElement(TAG_B, "Staging", "#f97316"), hidden: true },
				],
			})
			const calls = mockEndpoint(
				"PUT",
				`/api/tags/${TAG_B}/visibility`,
				() => ({}),
			)
			const wrapper = await mountSidebar()
			await settleMutations()

			// the section's own heading control, not a row's options menu
			await tagsGroup(wrapper)
				.get("[data-sidebar='group-action']")
				.trigger("click")
			menuItem("Staging").click()
			await settleMutations()

			expect(calls).toHaveLength(1)
			expect(calls[0]?.body).toEqual({ hidden: false })
		})

		it("keeps the section while every tag is hidden", async ({ expect }) => {
			stubQueries({
				tags: [
					{ ...tagElement(TAG_A, "Production", "#22c55e"), hidden: true },
					{ ...tagElement(TAG_B, "Staging", "#f97316"), hidden: true },
				],
			})

			const wrapper = await mountSidebar()
			await settleMutations()

			// no rows are left, but the section says so and keeps the control
			// that brings one back
			expect(tagNames(wrapper)).toEqual([])
			expect(tagsGroup(wrapper).text()).toContain(
				t("sidebar.sections.tags.all-hidden"),
			)
			expect(
				tagsGroup(wrapper).find("[data-sidebar='group-action']").exists(),
			).toBe(true)
		})

		it("keeps a hidden tag out of the section", async ({ expect }) => {
			stubQueries({
				tags: [
					{ ...tagElement(TAG_A, "Production", "#22c55e"), hidden: true },
					tagElement(TAG_B, "Staging", "#f97316"),
				],
			})

			const wrapper = await mountSidebar()
			await settleMutations()

			const names = tagNames(wrapper)
			expect(names).toHaveLength(1)
			expect(at(names, 0)).toContain("Staging")
		})

		it("warns when a tag cannot be moved", async ({ expect }) => {
			stubQueries({ tags: seededTags() })
			const wrapper = await mountSidebar()
			await settleMutations()
			const names = tagNames(wrapper)

			emitFromNth(
				wrapper,
				"SidebarNestedGroup",
				groupIndexShowing(wrapper, at(names, 0)),
				"update-location",
				{
					id: "nosuchtag00000000001",
					parentId: null,
					insertBeforeId: null,
				},
			)
			await settleMutations()

			expect(toast.custom).toHaveBeenCalledTimes(1)
			expect(tagNames(wrapper)).toEqual(names)
		})
	})

	describe("page moves", { concurrent: false }, () => {
		it("persists a move requested by the page tree", async ({ expect }) => {
			stubQueries({ tree: [treeElement()] })
			const calls = mockEndpoint("PUT", "/api/documents/tree", () => ({}))
			const wrapper = await mountSidebar()
			await settleMutations()

			emitFromNth(
				wrapper,
				"SidebarNestedGroup",
				groupIndexShowing(wrapper, "Runbook"),
				"update-location",
				{ id: DOC_ID, parentId: null, insertBeforeId: null },
			)
			await settleMutations()

			expect(calls).toHaveLength(1)
			expect(toast.custom).toHaveBeenCalledTimes(0)
		})

		it("warns when the move cannot be persisted", async ({ expect }) => {
			stubQueries({ tree: [treeElement()] })
			mockEndpoint("PUT", "/api/documents/tree", () => {
				throw createError({ statusCode: 500 })
			})
			const wrapper = await mountSidebar()
			await settleMutations()

			emitFromNth(
				wrapper,
				"SidebarNestedGroup",
				groupIndexShowing(wrapper, "Runbook"),
				"update-location",
				{ id: DOC_ID, parentId: null, insertBeforeId: null },
			)
			await settleMutations()

			expect(toast.custom).toHaveBeenCalledTimes(1)
		})

		it("passes a deletion request on to its parent", async ({ expect }) => {
			stubQueries({ tree: [treeElement()] })
			const wrapper = await mountSidebar()
			await settleMutations()

			await openRowMenu(wrapper, "Runbook")
			menuItem(t("sidebar.item-dropdown-menu-buttons.delete-page")).click()
			await nextTick()

			expect(sidebar(wrapper).emitted("delete-document")).toEqual([[DOC_ID]])
		})

		it("passes a duplication request on to its parent", async ({ expect }) => {
			stubQueries({ tree: [treeElement()] })
			const wrapper = await mountSidebar()
			await settleMutations()

			await openRowMenu(wrapper, "Runbook")
			menuItem(t("sidebar.item-dropdown-menu-buttons.duplicate-page")).click()
			await nextTick()

			expect(sidebar(wrapper).emitted("duplicate-document")).toEqual([[DOC_ID]])
		})

		it("asks for a sub page from the options menu", async ({ expect }) => {
			stubQueries({ tree: [treeElement()] })
			const wrapper = await mountSidebar()
			await settleMutations()

			await openRowMenu(wrapper, "Runbook")
			menuItem(t("sidebar.item-dropdown-menu-buttons.add-sub-page")).click()
			await nextTick()

			expect(sidebar(wrapper).emitted("create-document")).toEqual([[DOC_ID]])
		})
	})
})
