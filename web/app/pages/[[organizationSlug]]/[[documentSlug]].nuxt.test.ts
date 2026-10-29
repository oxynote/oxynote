import { mockNuxtImport, mountSuspended } from "@nuxt/test-utils/runtime"
import { enableAutoUnmount, type VueWrapper } from "@vue/test-utils"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import type { RouteMiddleware } from "#app"
import type { RouteLocationNormalized } from "vue-router"
import { toast } from "vue-sonner"
import {
	clearQueryCache,
	disposeMockEndpoints,
	makeXid,
	mockEndpoint,
	readQueryData,
	runInApp,
	seedQueryData,
} from "~/composables/api/test-helpers"
import DocumentEditor from "./[[documentSlug]].vue"
import {
	emitFrom,
	raisedToasts,
	seedAuthOrganization,
	seedCapabilities,
	settleMutations,
	t,
} from "../../components/test-helpers"

vi.mock("vue-sonner", () => ({
	toast: { custom: vi.fn(), dismiss: vi.fn() },
}))

// route.query is a readonly proxy, so the route itself is what a test
// arranges. It is reactive, so a test can move it after the mount.
const { useRouteMock, navigateToMock } = vi.hoisted(() => ({
	useRouteMock: vi.fn(),
	navigateToMock: vi.fn(),
}))

mockNuxtImport("useRoute", () => useRouteMock)
mockNuxtImport("navigateTo", () => navigateToMock)

const DOC_ID = makeXid("doca")
const OTHER_DOC_ID = makeXid("docb")
const CHILD_DOC_ID = makeXid("docc")
const COPY_DOC_ID = makeXid("docd")
const DEFAULT_BRANCH_ID = makeXid("main")
const DRAFT_BRANCH_ID = makeXid("draft")
const TAG_ID = makeXid("tag")

const ORGANIZATION = {
	slug: "Acme",
	members: [{ user: { id: "user-1", name: "Ada" } }],
}

// the sidebar, header, editors and modals have suites of their own. The
// page's contract with them is the props it hands down and what it does
// with the events they send up.
const CHILD_STUBS = {
	AppSidebar: true,
	DocumentHeader: true,
	EditorDocumentContainer: true,
	EditorNoDocumentIndicator: true,
	SettingsBaseModal: true,
	SearchModal: true,
	ShortcutModal: true,
	EditorBlocksMetricsConfigModalBaseModal: true,
	DocumentDeletionModal: true,
	TagDeletionModal: true,
	AIChatSidebar: true,
	NotificationBox: true,
	VariablesPanel: true,
}

describe("document-editor middleware", { concurrent: false }, () => {
	// restoreMocks does not touch hand-made vi.fn() singletons in vitest 4
	beforeEach(() => {
		clearQueryCache()
		navigateToMock.mockReset()
	})

	afterEach(disposeMockEndpoints)

	it("sends a visitor without a workspace to login and back", async ({
		expect,
	}) => {
		seedAuthOrganization(null)

		await runMiddleware(documentRoute(`Intro-${DOC_ID}`, "Acme"))

		expect(navigateToMock).toHaveBeenCalledTimes(1)
		expect(navigateToMock).toHaveBeenCalledWith(
			{
				name: "login",
				query: { next: encodeURIComponent(`/Acme/Intro-${DOC_ID}`) },
			},
			{ replace: true },
		)
	})

	it("loads the capabilities before the page renders", async ({ expect }) => {
		seedWorkspace()
		clearCapabilities()
		const capabilityCalls = mockEndpoint("GET", "/api/capabilities", () => ({
			github: true,
			slack: true,
			changeDetection: true,
			mcp: true,
			aiAssistant: { status: AssistantStatus.Active, model: "test-model" },
		}))

		await runMiddleware(documentRoute(`Intro-${DOC_ID}`, "Acme"))

		expect(capabilityCalls).toHaveLength(1)
		expect(navigateToMock).not.toHaveBeenCalled()
	})

	it("lets the empty workspace view through when there are no documents", async ({
		expect,
	}) => {
		seedWorkspace({ tree: [] })

		const result = await runMiddleware(documentRoute(undefined, "Acme"))

		expect(result).toBeUndefined()
		expect(navigateToMock).not.toHaveBeenCalled()
	})

	it.for([
		{
			name: "opens the first document when the url names none",
			input: undefined,
		},
		{
			name: "opens the first document when the slug carries no id",
			input: "Intro",
		},
		{
			name: "opens the first document when the slug names one outside the tree",
			input: `Ghost-${makeXid("ghost")}`,
		},
	])("$name", async ({ input }, { expect }) => {
		seedWorkspace()

		await runMiddleware(documentRoute(input, "Acme"))

		expect(navigateToMock).toHaveBeenCalledTimes(1)
		expect(navigateToMock).toHaveBeenCalledWith(`/Acme/Intro-${DOC_ID}`, {
			replace: true,
		})
	})

	it.for([
		{
			name: "rewrites a url without the workspace name",
			input: { organizationSlug: undefined, documentSlug: `Intro-${DOC_ID}` },
		},
		{
			name: "rewrites a url naming an outdated workspace",
			input: { organizationSlug: "Old", documentSlug: `Intro-${DOC_ID}` },
		},
		{
			name: "rewrites a url naming an outdated document",
			input: { organizationSlug: "Acme", documentSlug: `Old-${DOC_ID}` },
		},
		{
			name: "rewrites a url naming the document by its id alone",
			input: { organizationSlug: "Acme", documentSlug: DOC_ID },
		},
	])("$name", async ({ input }, { expect }) => {
		seedWorkspace()

		await runMiddleware({
			...documentRoute(input.documentSlug, input.organizationSlug),
			query: { branch: DRAFT_BRANCH_ID },
			hash: "#block-1",
		})

		expect(navigateToMock).toHaveBeenCalledTimes(1)
		expect(navigateToMock).toHaveBeenCalledWith(
			{
				path: `/Acme/Intro-${DOC_ID}`,
				query: { branch: DRAFT_BRANCH_ID },
				hash: "#block-1",
			},
			{ replace: true },
		)
	})

	it("lets a url through when its names match", async ({ expect }) => {
		seedWorkspace()

		const result = await runMiddleware(documentRoute(`Intro-${DOC_ID}`, "Acme"))

		expect(result).toBeUndefined()
		expect(navigateToMock).not.toHaveBeenCalled()
	})

	it("lets an untitled document through by its id alone", async ({
		expect,
	}) => {
		seedWorkspace({ tree: [treeElement(DOC_ID, "")] })

		const result = await runMiddleware(documentRoute(DOC_ID, "Acme"))

		expect(result).toBeUndefined()
		expect(navigateToMock).not.toHaveBeenCalled()
	})
})

// the route, vue-sonner and navigateTo mocks, the query cache, the editor
// store and the websocket store are app-wide singletons every mount in the
// file shares
describe("<document-editor>", { concurrent: false }, () => {
	enableAutoUnmount(afterEach)

	beforeEach(() => {
		clearQueryCache()
		useRouteMock.mockReset()
		navigateToMock.mockReset()
		vi.mocked(toast.custom).mockReset()
		vi.spyOn(useRouter(), "replace").mockResolvedValue(undefined)
		useEditorStore().updateActiveDocumentId(null)
		useWebSocketStateStore().state = null
		runInApp(() => usePageReady()).value = false
		// the page records every branch it opens
		mockBranchViews(DEFAULT_BRANCH_ID, () => null)
		mockBranchViews(DRAFT_BRANCH_ID, () => null)
	})

	afterEach(() => {
		vi.useRealTimers()
		disposeMockEndpoints()
	})

	describe("when the url names a document", { concurrent: false }, () => {
		it("shows the document and makes it the active one", async ({ expect }) => {
			seedWorkspace()
			arrangeRoute()

			const wrapper = await mountPage()

			expect(child(wrapper, "EditorDocumentContainer").exists()).toBe(true)
			expect(child(wrapper, "EditorNoDocumentIndicator").exists()).toBe(false)
			expect(useEditorStore().activeDocumentId).toBe(DOC_ID)
		})

		it("names the document in the page title", async ({ expect }) => {
			seedWorkspace()
			arrangeRoute()

			await mountPage()
			await settleMutations()

			expect(document.title).toBe(
				t("general.document-page-title", { suffix: "Intro" }),
			)
		})

		it("hands the header the path down to a nested document", async ({
			expect,
		}) => {
			seedWorkspace({
				tree: [
					treeElement(DOC_ID, "Intro", [treeElement(CHILD_DOC_ID, "Setup")]),
				],
			})
			arrangeRoute({ documentSlug: `Setup-${CHILD_DOC_ID}` })

			const wrapper = await mountPage()

			expect(child(wrapper, "DocumentHeader").props("breadcrumbs")).toEqual([
				{
					id: DOC_ID,
					name: "Intro",
					icon: "mingcute:document-2-fill",
					href: `/Intro-${DOC_ID}`,
				},
				{
					id: CHILD_DOC_ID,
					name: "Setup",
					icon: "mingcute:document-2-fill",
					href: `/Setup-${CHILD_DOC_ID}`,
				},
			])
		})

		it("shows the empty workspace view when the url names no document", async ({
			expect,
		}) => {
			seedWorkspace({ tree: [] })
			arrangeRoute({ documentSlug: undefined })

			const wrapper = await mountPage()

			expect(child(wrapper, "EditorNoDocumentIndicator").exists()).toBe(true)
			expect(child(wrapper, "EditorDocumentContainer").exists()).toBe(false)
			expect(child(wrapper, "DocumentHeader").props("breadcrumbs")).toEqual([])
			expect(useEditorStore().activeDocumentId).toBeNull()
		})

		it("uses the default title when the url names no document", async ({
			expect,
		}) => {
			seedWorkspace({ tree: [] })
			arrangeRoute({ documentSlug: undefined })

			await mountPage()
			await settleMutations()

			expect(document.title).toBe(t("general.default-page-title"))
		})

		it("offers the assistant only while it is enabled", async ({ expect }) => {
			seedWorkspace()
			seedCapabilities({
				aiAssistant: { status: AssistantStatus.Inactive, model: "" },
			})
			arrangeRoute()

			const wrapper = await mountPage()

			expect(child(wrapper, "AIChatSidebar").exists()).toBe(false)
		})

		it("reports every section loaded once the sidebar and the editor are", async ({
			expect,
		}) => {
			seedWorkspace()
			arrangeRoute()
			const wrapper = await mountPage()
			expect(
				child(wrapper, "AppSidebar").props("allInitialSectionsLoaded"),
			).toBe(false)

			await finishLoading(wrapper)

			expect(
				child(wrapper, "AppSidebar").props("allInitialSectionsLoaded"),
			).toBe(true)
			expect(
				child(wrapper, "DocumentHeader").props("allInitialSectionsLoaded"),
			).toBe(true)
			expect(child(wrapper, "DocumentDeletionModal").exists()).toBe(true)
		})

		it("marks the page ready once the document has faded in", async ({
			expect,
		}) => {
			seedWorkspace()
			arrangeRoute()
			const wrapper = await mountPage()

			emitFrom(wrapper, { name: "EditorDocumentContainer" }, "fade-in-complete")

			expect(runInApp(() => usePageReady()).value).toBe(true)
		})

		it("marks the page loading again when another document opens", async ({
			expect,
		}) => {
			seedWorkspace({
				tree: [
					treeElement(DOC_ID, "Intro"),
					treeElement(OTHER_DOC_ID, "Other"),
				],
			})
			const route = arrangeRoute()
			const wrapper = await mountPage()
			emitFrom(wrapper, { name: "EditorDocumentContainer" }, "fade-in-complete")

			route.params.documentSlug = `Other-${OTHER_DOC_ID}`
			await nextTick()

			expect(runInApp(() => usePageReady()).value).toBe(false)
			expect(useEditorStore().activeDocumentId).toBe(OTHER_DOC_ID)
		})

		it("opens the first document once the open one leaves the tree", async ({
			expect,
		}) => {
			seedWorkspace({
				tree: [
					treeElement(DOC_ID, "Intro"),
					treeElement(OTHER_DOC_ID, "Other"),
				],
			})
			arrangeRoute()
			await mountPage()

			seedQueryData(["documents", "tree"], [treeElement(OTHER_DOC_ID, "Other")])
			await settleMutations()

			expect(navigateToMock).toHaveBeenCalledTimes(1)
			expect(navigateToMock).toHaveBeenCalledWith(
				`/Acme/Other-${OTHER_DOC_ID}`,
				{ replace: true },
			)
		})

		it("returns to the workspace once its last document leaves the tree", async ({
			expect,
		}) => {
			seedWorkspace()
			arrangeRoute()
			await mountPage()

			seedQueryData(["documents", "tree"], [])
			await settleMutations()

			expect(navigateToMock).toHaveBeenCalledTimes(1)
			expect(navigateToMock).toHaveBeenCalledWith("/Acme", { replace: true })
		})
	})

	describe("when the document has branches", { concurrent: false }, () => {
		it("opens the default branch when the url names none", async ({
			expect,
		}) => {
			seedWorkspace({ branches: twoBranches() })
			arrangeRoute()

			await mountPage()

			const store = useEditorStore()
			expect.soft(store.activeBranchId).toBe(DEFAULT_BRANCH_ID)
			expect.soft(store.targetBranchId).toBeNull()
			expect.soft(store.mappedDefaultBranchId).toBe(DEFAULT_BRANCH_ID)
			expect
				.soft(store.preloadedBranchIds)
				.toEqual([DEFAULT_BRANCH_ID, DRAFT_BRANCH_ID])
			expect.soft(store.branchReviewableActionsActive).toBe(false)
			expect.soft(pageReplaces()).toEqual([])
		})

		it("opens the branch the url names, diffed against the default one", async ({
			expect,
		}) => {
			seedWorkspace({ branches: twoBranches() })
			arrangeRoute({ query: { branch: DRAFT_BRANCH_ID } })

			await mountPage()

			const store = useEditorStore()
			expect.soft(store.activeBranchId).toBe(DRAFT_BRANCH_ID)
			expect.soft(store.targetBranchId).toBe(DEFAULT_BRANCH_ID)
			expect.soft(store.mappedDefaultBranchId).toBe(DEFAULT_BRANCH_ID)
			expect.soft(store.branchReviewableActionsActive).toBe(true)
			expect.soft(pageReplaces()).toEqual([])
		})

		it("drops a branch the document does not have from the url", async ({
			expect,
		}) => {
			seedWorkspace({ branches: twoBranches() })
			arrangeRoute({ query: { branch: "gone", tab: "1" } })

			await mountPage()

			expect(useEditorStore().activeBranchId).toBe(DEFAULT_BRANCH_ID)
			expect(pageReplaces()).toEqual([
				{
					path: `/Acme/Intro-${DOC_ID}`,
					query: { branch: undefined, tab: "1" },
					hash: "",
				},
			])
		})

		it("opens the branch a changed url names", async ({ expect }) => {
			seedWorkspace({ branches: twoBranches() })
			const route = arrangeRoute()
			await mountPage()

			route.query = { branch: DRAFT_BRANCH_ID }
			await nextTick()

			expect(useEditorStore().activeBranchId).toBe(DRAFT_BRANCH_ID)
			expect(useEditorStore().targetBranchId).toBe(DEFAULT_BRANCH_ID)
		})

		it("drops an unknown branch a changed url names", async ({ expect }) => {
			seedWorkspace({ branches: twoBranches() })
			const route = arrangeRoute()
			await mountPage()

			route.query = { branch: "gone" }
			await nextTick()

			expect(useEditorStore().activeBranchId).toBe(DEFAULT_BRANCH_ID)
			expect(pageReplaces()).toEqual([
				{
					path: `/Acme/Intro-${DOC_ID}`,
					query: { branch: undefined },
					hash: "",
				},
			])
		})

		it("opens no branch when the document has no default one", async ({
			expect,
		}) => {
			seedWorkspace({ branches: [branch(DRAFT_BRANCH_ID, false)] })
			arrangeRoute()

			await mountPage()

			const store = useEditorStore()
			expect.soft(store.activeBranchId).toBeNull()
			expect.soft(store.targetBranchId).toBeNull()
			expect.soft(store.mappedDefaultBranchId).toBeNull()
			expect.soft(store.preloadedBranchIds).toEqual([DRAFT_BRANCH_ID])
			expect.soft(pageReplaces()).toEqual([])
		})

		it("keeps the open branch when the branch list is refetched", async ({
			expect,
		}) => {
			seedWorkspace({ branches: twoBranches() })
			arrangeRoute()
			await mountPage()
			useEditorStore().updateActiveBranchId(DRAFT_BRANCH_ID)
			await nextTick()

			seedQueryData(["documents", DOC_ID, "branches"], twoBranches())
			await nextTick()

			expect(useEditorStore().activeBranchId).toBe(DRAFT_BRANCH_ID)
		})

		it("falls back to the default branch when the open one goes away", async ({
			expect,
		}) => {
			seedWorkspace({ branches: twoBranches() })
			arrangeRoute({ query: { branch: DRAFT_BRANCH_ID } })
			await mountPage()

			seedQueryData(
				["documents", DOC_ID, "branches"],
				[branch(DEFAULT_BRANCH_ID, true)],
			)
			await nextTick()

			expect(useEditorStore().activeBranchId).toBe(DEFAULT_BRANCH_ID)
			expect(useEditorStore().targetBranchId).toBeNull()
			expect(useEditorStore().branchReviewableActionsActive).toBe(false)
		})

		it("names a branch switched to in the url and drops the linked block", async ({
			expect,
		}) => {
			seedWorkspace({ branches: twoBranches() })
			arrangeRoute({ hash: "#block-1" })
			await mountPage()

			useEditorStore().updateActiveBranchId(DRAFT_BRANCH_ID)
			await nextTick()

			expect(pageReplaces()).toEqual([
				{
					path: `/Acme/Intro-${DOC_ID}`,
					query: { branch: DRAFT_BRANCH_ID },
					hash: "",
				},
			])
		})

		it("records each branch the user opens once", async ({ expect }) => {
			seedWorkspace({ branches: twoBranches() })
			const defaultViews = mockBranchViews(DEFAULT_BRANCH_ID, () => null)
			const draftViews = mockBranchViews(DRAFT_BRANCH_ID, () => null)
			arrangeRoute()
			await mountPage()

			useEditorStore().updateActiveBranchId(DRAFT_BRANCH_ID)
			await settleMutations()
			seedQueryData(["documents", DOC_ID, "branches"], twoBranches())
			await settleMutations()

			expect(defaultViews).toHaveLength(1)
			expect(draftViews).toHaveLength(1)
		})

		it("logs a branch view the server refused", async ({ expect }) => {
			const consoleError = vi
				.spyOn(console, "error")
				.mockImplementation(() => undefined)
			seedWorkspace({ branches: twoBranches() })
			mockBranchViews(DEFAULT_BRANCH_ID, () => {
				throw createError({ statusCode: 500 })
			})
			arrangeRoute()

			await mountPage()
			await settleMutations()

			expect(consoleError).toHaveBeenCalledWith(
				"Failed to record branch view:",
				expect.anything(),
			)
		})

		it("switches to the default branch after a merge", async ({ expect }) => {
			seedWorkspace({ branches: twoBranches() })
			arrangeRoute({ query: { branch: DRAFT_BRANCH_ID } })
			const wrapper = await mountPage()

			emitFrom(wrapper, { name: "EditorDocumentContainer" }, "branch-merged")

			expect(useEditorStore().activeBranchId).toBe(DEFAULT_BRANCH_ID)
			expect(useEditorStore().targetBranchId).toBeNull()
		})

		it("ignores a merge before the branches have loaded", async ({
			expect,
		}) => {
			seedWorkspace({ branches: [] })
			arrangeRoute()
			const wrapper = await mountPage()

			emitFrom(wrapper, { name: "EditorDocumentContainer" }, "branch-merged")

			expect(useEditorStore().activeBranchId).toBeNull()
			expect(useEditorStore().targetBranchId).toBeNull()
		})
	})

	describe(
		"when the document's metadata changes",
		{ concurrent: false },
		() => {
			it("listens for the open document's changes until it goes away", async ({
				expect,
			}) => {
				const { subscribe, unsubscribe } = stubWebSocket()
				seedWorkspace()
				arrangeRoute()
				const wrapper = await mountPage()

				wrapper.unmount()

				expect(subscribe).toHaveBeenCalledTimes(1)
				expect(subscribe).toHaveBeenCalledWith(
					makeWsDocumentMetadataChangeTopic(DOC_ID),
					expect.any(Function),
				)
				expect(unsubscribe).toHaveBeenCalledTimes(1)
			})

			it("hands the header who created and changed the branch", async ({
				expect,
			}) => {
				const { subscribe } = stubWebSocket()
				seedWorkspace()
				arrangeRoute()
				const wrapper = await mountPage()

				publishMetadata(subscribe, {
					branchId: DEFAULT_BRANCH_ID,
					documentName: "Intro",
					protected: false,
					createdAt: "2026-01-01T10:00:00Z",
					createdBy: "user-1",
					updatedAt: "2026-01-02T10:00:00Z",
					lastUpdatedBy: "user-gone",
				})
				await nextTick()

				expect(child(wrapper, "DocumentHeader").props("timestamps")).toEqual({
					[DEFAULT_BRANCH_ID]: {
						created: {
							at: new Date("2026-01-01T10:00:00Z"),
							user: { id: "user-1", name: "Ada" },
						},
						updated: { at: new Date("2026-01-02T10:00:00Z"), user: null },
					},
				})
			})

			it("dates a branch never changed by its creation", async ({ expect }) => {
				const { subscribe } = stubWebSocket()
				seedWorkspace()
				arrangeRoute()
				const wrapper = await mountPage()

				publishMetadata(subscribe, {
					branchId: DEFAULT_BRANCH_ID,
					documentName: "Intro",
					protected: false,
					createdAt: "2026-01-01T10:00:00Z",
					createdBy: "user-gone",
					lastUpdatedBy: "user-1",
				})
				await nextTick()

				expect(child(wrapper, "DocumentHeader").props("timestamps")).toEqual({
					[DEFAULT_BRANCH_ID]: {
						created: { at: new Date("2026-01-01T10:00:00Z"), user: null },
						updated: {
							at: new Date("2026-01-01T10:00:00Z"),
							user: { id: "user-1", name: "Ada" },
						},
					},
				})
			})

			it("marks the document protected when its default branch becomes so", async ({
				expect,
			}) => {
				const { subscribe } = stubWebSocket()
				seedWorkspace({ branches: twoBranches() })
				arrangeRoute()
				await mountPage()

				publishMetadata(subscribe, {
					branchId: DEFAULT_BRANCH_ID,
					documentName: "Intro",
					protected: true,
					createdAt: "2026-01-01T10:00:00Z",
				})

				expect(readQueryData(["documents", "tree"])).toEqual([
					treeElement(DOC_ID, "Intro", null, true),
				])
			})

			it.for([
				{
					name: "leaves the document unprotected when another branch is protected",
					input: { branchId: DRAFT_BRANCH_ID, protected: true },
				},
				{
					name: "leaves the document as it is when protection does not change",
					input: { branchId: DEFAULT_BRANCH_ID, protected: false },
				},
			])("$name", async ({ input }, { expect }) => {
				const { subscribe } = stubWebSocket()
				seedWorkspace({ branches: twoBranches() })
				arrangeRoute()
				await mountPage()

				publishMetadata(subscribe, {
					...input,
					documentName: "Intro",
					createdAt: "2026-01-01T10:00:00Z",
				})

				expect(readQueryData(["documents", "tree"])).toEqual([
					treeElement(DOC_ID, "Intro"),
				])
			})
		},
	)

	describe(
		"when the sidebar opens a panel or modal",
		{ concurrent: false },
		() => {
			it("opens the inbox", async ({ expect }) => {
				seedWorkspace()
				arrangeRoute()
				const wrapper = await mountPage()

				emitFrom(wrapper, { name: "AppSidebar" }, "toggle-notifications")
				await nextTick()

				expect(
					child(wrapper, "AppSidebar").props("notificationSidebarOpen"),
				).toBe(true)
				expect(
					contextSidebar(wrapper, t("sidebar.notification-sheet.title")).props(
						"modelValue",
					),
				).toBe(true)
			})

			it("closes the inbox when asked twice", async ({ expect }) => {
				seedWorkspace()
				arrangeRoute()
				const wrapper = await mountPage()
				emitFrom(wrapper, { name: "AppSidebar" }, "toggle-notifications")
				await nextTick()

				emitFrom(wrapper, { name: "AppSidebar" }, "toggle-notifications")
				await nextTick()

				expect(
					child(wrapper, "AppSidebar").props("notificationSidebarOpen"),
				).toBe(false)
			})

			it("closes the inbox when the variables panel opens", async ({
				expect,
			}) => {
				seedWorkspace()
				arrangeRoute()
				const wrapper = await mountPage()
				emitFrom(wrapper, { name: "AppSidebar" }, "toggle-notifications")
				await nextTick()

				emitFrom(wrapper, { name: "AppSidebar" }, "toggle-variables")
				await nextTick()

				const sidebar = child(wrapper, "AppSidebar")
				expect(sidebar.props("variableSidebarOpen")).toBe(true)
				expect(sidebar.props("notificationSidebarOpen")).toBe(false)
			})

			it("closes the variables panel when the inbox opens", async ({
				expect,
			}) => {
				seedWorkspace()
				arrangeRoute()
				const wrapper = await mountPage()
				emitFrom(wrapper, { name: "AppSidebar" }, "toggle-variables")
				await nextTick()

				emitFrom(wrapper, { name: "AppSidebar" }, "toggle-notifications")
				await nextTick()

				const sidebar = child(wrapper, "AppSidebar")
				expect(sidebar.props("notificationSidebarOpen")).toBe(true)
				expect(sidebar.props("variableSidebarOpen")).toBe(false)
			})

			it("closes the variables panel when asked twice", async ({ expect }) => {
				seedWorkspace()
				arrangeRoute()
				const wrapper = await mountPage()
				emitFrom(wrapper, { name: "AppSidebar" }, "toggle-variables")
				await nextTick()

				emitFrom(wrapper, { name: "AppSidebar" }, "toggle-variables")
				await nextTick()

				expect(child(wrapper, "AppSidebar").props("variableSidebarOpen")).toBe(
					false,
				)
			})

			it("keeps the variables panel open while a variable is being edited", async ({
				expect,
			}) => {
				seedWorkspace()
				arrangeRoute()
				const wrapper = await openVariableEditing()

				emitFrom(wrapper, { name: "AppSidebar" }, "toggle-variables")
				await nextTick()

				const sidebar = child(wrapper, "AppSidebar")
				expect(sidebar.props("variableSidebarOpen")).toBe(true)
				expect(sidebar.props("variableEditing")).toBe(true)
			})

			it("warns instead of opening the inbox while a variable is being edited", async ({
				expect,
			}) => {
				seedWorkspace()
				arrangeRoute()
				const wrapper = await openVariableEditing()

				emitFrom(wrapper, { name: "AppSidebar" }, "toggle-notifications")
				await nextTick()

				expect(raisedToasts()).toMatchObject([
					{ type: "warning", title: t("variable.editing-toast.title") },
				])
				expect(
					child(wrapper, "AppSidebar").props("notificationSidebarOpen"),
				).toBe(false)
			})

			it.for([
				{
					name: "opens the search",
					input: "toggle-search",
					expected: "SearchModal",
				},
				{
					name: "opens the shortcuts",
					input: "toggle-shortcuts",
					expected: "ShortcutModal",
				},
				{
					name: "opens the settings",
					input: "toggle-settings",
					expected: "SettingsBaseModal",
				},
			])("$name", async ({ input, expected }, { expect }) => {
				seedWorkspace()
				arrangeRoute()
				const wrapper = await mountPage()

				emitFrom(wrapper, { name: "AppSidebar" }, input)
				await nextTick()

				expect(child(wrapper, expected).props("modelValue")).toBe(true)
			})

			it.for([
				{
					name: "opens the settings section the sidebar names",
					input: { from: "AppSidebar", target: "org-members" },
					expected: "org-members",
				},
				{
					name: "opens the settings section the document names",
					input: { from: "EditorDocumentContainer", target: "github" },
					expected: "github",
				},
				{
					name: "opens the data sources from the metric settings",
					input: {
						from: "EditorBlocksMetricsConfigModalBaseModal",
						target: undefined,
					},
					expected: "metric-data-sources",
				},
			])("$name", async ({ input, expected }, { expect }) => {
				seedWorkspace()
				arrangeRoute()
				const wrapper = await mountPage()

				emitFrom(wrapper, { name: input.from }, "open-settings", input.target)
				await nextTick()

				expect(child(wrapper, "SettingsBaseModal").props("modelValue")).toBe(
					expected,
				)
			})
		},
	)

	describe(
		"when the document is renamed or restyled",
		{ concurrent: false },
		() => {
			it("renames the document everywhere and in the url", async ({
				expect,
			}) => {
				seedWorkspace({ tags: [tagWith(treeElement(DOC_ID, "Intro"))] })
				arrangeRoute({ query: { tab: "1" }, hash: "#block-1" })
				const wrapper = await mountPage()

				emitFrom(
					wrapper,
					{ name: "EditorDocumentContainer" },
					"updated-live-name",
					"Guide",
				)

				expect
					.soft(readQueryData(["documents", "tree"]))
					.toEqual([treeElement(DOC_ID, "Guide")])
				expect
					.soft(readQueryData(["tags", "tree"]))
					.toEqual([tagWith(treeElement(DOC_ID, "Guide"))])
				expect.soft(pageReplaces()).toEqual([
					{
						path: `/Acme/Guide-${DOC_ID}`,
						query: { tab: "1" },
						hash: "#block-1",
					},
				])
			})

			it("names a document whose name was cleared", async ({ expect }) => {
				seedWorkspace()
				arrangeRoute()
				const wrapper = await mountPage()

				emitFrom(
					wrapper,
					{ name: "EditorDocumentContainer" },
					"updated-live-name",
					"",
				)

				expect(readQueryData(["documents", "tree"])).toEqual([
					treeElement(DOC_ID, t("editor.new-document-name")),
				])
			})

			it("keeps the linked block a later navigation added in the url", async ({
				expect,
			}) => {
				seedWorkspace()
				const route = arrangeRoute()
				const wrapper = await mountPage()
				route.hash = "#block-2"

				emitFrom(
					wrapper,
					{ name: "EditorDocumentContainer" },
					"updated-live-name",
					"Guide",
				)

				expect(pageReplaces()).toEqual([
					{ path: `/Acme/Guide-${DOC_ID}`, query: {}, hash: "#block-2" },
				])
			})

			it("changes the document's icon everywhere", async ({ expect }) => {
				seedWorkspace({ tags: [tagWith(treeElement(DOC_ID, "Intro"))] })
				arrangeRoute()
				const wrapper = await mountPage()

				emitFrom(
					wrapper,
					{ name: "EditorDocumentContainer" },
					"updated-live-icon",
					"mingcute:book-2-fill",
				)

				const renamed = {
					...treeElement(DOC_ID, "Intro"),
					icon: "mingcute:book-2-fill",
				}
				expect.soft(readQueryData(["documents", "tree"])).toEqual([renamed])
				expect.soft(readQueryData(["tags", "tree"])).toEqual([tagWith(renamed)])
				expect.soft(pageReplaces()).toEqual([])
			})

			it("follows a renamed workspace in the url", async ({ expect }) => {
				seedWorkspace()
				arrangeRoute({ query: { tab: "1" } })
				const wrapper = await mountPage()
				seedAuthOrganization({ ...ORGANIZATION, slug: "Beta" })

				emitFrom(
					wrapper,
					{ name: "SettingsBaseModal" },
					"refresh-organization-slug",
				)

				expect(pageReplaces()).toEqual([
					{
						path: `/Beta/Intro-${DOC_ID}`,
						query: { tab: "1" },
						hash: "",
					},
				])
			})

			it("ignores a renamed workspace without an open document", async ({
				expect,
			}) => {
				seedWorkspace({ tree: [] })
				arrangeRoute({ documentSlug: undefined })
				const wrapper = await mountPage()

				emitFrom(
					wrapper,
					{ name: "SettingsBaseModal" },
					"refresh-organization-slug",
				)

				expect(pageReplaces()).toEqual([])
			})

			it("drops the linked block once the user works in the document", async ({
				expect,
			}) => {
				seedWorkspace()
				arrangeRoute({ query: { tab: "1" }, hash: "#block-1" })
				const wrapper = await mountPage()

				emitFrom(wrapper, { name: "EditorDocumentContainer" }, "click")

				expect(pageReplaces()).toEqual([
					{ path: `/Acme/Intro-${DOC_ID}`, query: { tab: "1" }, hash: "" },
				])
			})

			it("leaves the url alone when no block is linked", async ({ expect }) => {
				seedWorkspace()
				arrangeRoute()
				const wrapper = await mountPage()

				emitFrom(wrapper, { name: "EditorDocumentContainer" }, "keydown")

				expect(pageReplaces()).toEqual([])
			})
		},
	)

	describe("when a document is created", { concurrent: false }, () => {
		it("creates it under the parent the sidebar names", async ({ expect }) => {
			seedWorkspace({
				tree: [
					treeElement(DOC_ID, "Intro"),
					treeElement(OTHER_DOC_ID, "Other"),
				],
			})
			const creations = mockEndpoint("POST", "/api/documents", () => ({
				id: COPY_DOC_ID,
			}))
			mockEndpoint("GET", "/api/documents/tree", () => [
				treeElement(DOC_ID, "Intro"),
				treeElement(OTHER_DOC_ID, "Other"),
			])
			arrangeRoute()
			const wrapper = await mountPage()
			vi.useFakeTimers()

			emitFrom(wrapper, { name: "AppSidebar" }, "create-document", DOC_ID)
			await settleCreation()

			expect(creations).toHaveLength(1)
			expect(creations[0]?.body).toEqual({
				name: t("editor.new-document-name"),
				icon: "mingcute:document-2-fill",
				parentId: DOC_ID,
			})
			expect(navigateToMock).not.toHaveBeenCalled()
			expect(toast.custom).not.toHaveBeenCalled()
		})

		it("shows the spinner while the document is created", async ({
			expect,
		}) => {
			seedWorkspace({ tree: [] })
			mockEndpoint("POST", "/api/documents", () => ({ id: DOC_ID }))
			arrangeRoute({ documentSlug: undefined })
			const wrapper = await mountPage()
			vi.useFakeTimers()

			emitFrom(
				wrapper,
				{ name: "EditorNoDocumentIndicator" },
				"create-document",
			)
			await nextTick()

			expect(
				child(wrapper, "EditorNoDocumentIndicator").props("createLoading"),
			).toBe(true)
		})

		it("opens the workspace's first document once it exists", async ({
			expect,
		}) => {
			seedWorkspace({ tree: [] })
			const creations = mockEndpoint("POST", "/api/documents", () => ({
				id: DOC_ID,
			}))
			mockEndpoint("GET", "/api/documents/tree", () => [
				treeElement(DOC_ID, t("editor.new-document-name")),
			])
			arrangeRoute({ documentSlug: undefined })
			const wrapper = await mountPage()
			vi.useFakeTimers()

			emitFrom(
				wrapper,
				{ name: "EditorNoDocumentIndicator" },
				"create-document",
			)
			await settleCreation()

			expect(creations).toHaveLength(1)
			expect(creations[0]?.body).toEqual({
				name: t("editor.new-document-name"),
				icon: "mingcute:document-2-fill",
				parentId: null,
			})
			expect(navigateToMock).toHaveBeenCalledTimes(1)
			expect(navigateToMock).toHaveBeenCalledWith(
				`/Acme/${createNameSlugWithId(t("editor.new-document-name"), DOC_ID)}`,
				{ replace: true },
			)
			expect(
				child(wrapper, "EditorNoDocumentIndicator").props("createLoading"),
			).toBe(false)
		})

		it("reports a document the server refused to create", async ({
			expect,
		}) => {
			seedWorkspace()
			mockEndpoint("POST", "/api/documents", () => {
				throw createError({ statusCode: 500 })
			})
			arrangeRoute()
			const wrapper = await mountPage()
			vi.useFakeTimers()

			emitFrom(wrapper, { name: "AppSidebar" }, "create-document", null)
			await settleCreation()

			expect(raisedToasts()).toMatchObject([
				{ type: "error", title: t("sidebar.errors.create-document-failed") },
			])
			expect(navigateToMock).not.toHaveBeenCalled()
			expect(readQueryData(["documents", "tree"])).toEqual([
				treeElement(DOC_ID, "Intro"),
			])
		})

		it("drops a pending deletion when a document is created", async ({
			expect,
		}) => {
			seedWorkspace({
				tree: [
					treeElement(DOC_ID, "Intro"),
					treeElement(OTHER_DOC_ID, "Other"),
				],
			})
			mockEndpoint("POST", "/api/documents", () => ({ id: COPY_DOC_ID }))
			arrangeRoute()
			const wrapper = await mountPage()
			await finishLoading(wrapper)
			emitFrom(wrapper, { name: "AppSidebar" }, "delete-document", OTHER_DOC_ID)
			await nextTick()

			emitFrom(wrapper, { name: "AppSidebar" }, "create-document", null)
			await nextTick()

			expect(
				child(wrapper, "DocumentDeletionModal").props("modelValue"),
			).toBeNull()
		})
	})

	describe("when a document is deleted", { concurrent: false }, () => {
		it("asks before deleting the document the sidebar names", async ({
			expect,
		}) => {
			seedWorkspace({
				tree: [
					treeElement(DOC_ID, "Intro"),
					treeElement(OTHER_DOC_ID, "Other"),
				],
			})
			const deletions = mockDocumentDeletion(OTHER_DOC_ID)
			arrangeRoute()
			const wrapper = await mountPage()
			await finishLoading(wrapper)

			emitFrom(wrapper, { name: "AppSidebar" }, "delete-document", OTHER_DOC_ID)
			await nextTick()

			expect(pendingDeletion(wrapper, "DocumentDeletionModal").name).toBe(
				"Other",
			)
			expect(deletions).toHaveLength(0)
		})

		it("deletes another document and confirms it", async ({ expect }) => {
			seedWorkspace({
				tree: [
					treeElement(DOC_ID, "Intro"),
					treeElement(OTHER_DOC_ID, "Other"),
				],
			})
			const deletions = mockDocumentDeletion(OTHER_DOC_ID)
			mockEndpoint("GET", "/api/documents/tree", () => [
				treeElement(DOC_ID, "Intro"),
			])
			arrangeRoute()
			const wrapper = await mountPage()
			await finishLoading(wrapper)
			emitFrom(wrapper, { name: "AppSidebar" }, "delete-document", OTHER_DOC_ID)
			await nextTick()

			await pendingDeletion(wrapper, "DocumentDeletionModal").deleteDocument()

			expect(deletions).toHaveLength(1)
			expect(navigateToMock).not.toHaveBeenCalled()
			expect(raisedToasts()).toMatchObject([
				{
					type: "success",
					title: t("editor.document-deletion-modal.deletion-success", {
						name: "Other",
					}),
				},
			])
		})

		it("moves to another document before deleting the open one", async ({
			expect,
		}) => {
			seedWorkspace({
				tree: [
					treeElement(DOC_ID, "Intro"),
					treeElement(OTHER_DOC_ID, "Other"),
				],
			})
			const deletions = mockDocumentDeletion(DOC_ID)
			mockEndpoint("GET", "/api/documents/tree", () => [
				treeElement(OTHER_DOC_ID, "Other"),
			])
			followRedirects(arrangeRoute())
			const wrapper = await mountPage()
			await finishLoading(wrapper)
			emitFrom(wrapper, { name: "DocumentHeader" }, "delete-document")
			await nextTick()

			await pendingDeletion(wrapper, "DocumentDeletionModal").deleteDocument()

			expect(navigateToMock).toHaveBeenCalledTimes(1)
			expect(navigateToMock).toHaveBeenCalledWith(
				`/Acme/Other-${OTHER_DOC_ID}`,
				{
					replace: true,
				},
			)
			expect(deletions).toHaveLength(1)
		})

		it("returns to the workspace once its last document is deleted", async ({
			expect,
		}) => {
			seedWorkspace()
			const deletions = mockDocumentDeletion(DOC_ID)
			mockEndpoint("GET", "/api/documents/tree", () => [])
			followRedirects(arrangeRoute())
			const wrapper = await mountPage()
			await finishLoading(wrapper)
			emitFrom(wrapper, { name: "DocumentHeader" }, "delete-document")
			await nextTick()

			await pendingDeletion(wrapper, "DocumentDeletionModal").deleteDocument()

			expect(deletions).toHaveLength(1)
			// once to leave the document, once more when the tree comes back empty
			expect(navigateToMock.mock.calls).toEqual([
				["/Acme", { replace: true }],
				["/Acme", { replace: true }],
			])
		})

		it("reports a deletion the server refused", async ({ expect }) => {
			seedWorkspace({
				tree: [
					treeElement(DOC_ID, "Intro"),
					treeElement(OTHER_DOC_ID, "Other"),
				],
			})
			mockEndpoint("DELETE", `/api/documents/${OTHER_DOC_ID}`, () => {
				throw createError({ statusCode: 500 })
			})
			arrangeRoute()
			const wrapper = await mountPage()
			await finishLoading(wrapper)
			emitFrom(wrapper, { name: "AppSidebar" }, "delete-document", OTHER_DOC_ID)
			await nextTick()

			await pendingDeletion(wrapper, "DocumentDeletionModal").deleteDocument()

			expect(raisedToasts()).toMatchObject([
				{ type: "error", title: t("editor.errors.deletion-failed") },
			])
			expect(navigateToMock).not.toHaveBeenCalled()
		})

		it("asks nothing for a document outside the tree", async ({ expect }) => {
			seedWorkspace()
			arrangeRoute()
			const wrapper = await mountPage()
			await finishLoading(wrapper)

			emitFrom(
				wrapper,
				{ name: "AppSidebar" },
				"delete-document",
				makeXid("ghost"),
			)
			await nextTick()

			expect(
				child(wrapper, "DocumentDeletionModal").props("modelValue"),
			).toBeNull()
		})

		it("asks nothing when no document is open", async ({ expect }) => {
			seedWorkspace()
			const route = arrangeRoute({ documentSlug: undefined })
			const wrapper = await mountPage()
			emitFrom(wrapper, { name: "DocumentHeader" }, "delete-document")
			await nextTick()

			// the modal renders only with an open document, so one is opened
			// to see what it was handed
			route.params.documentSlug = `Intro-${DOC_ID}`
			await nextTick()
			await finishLoading(wrapper)

			expect(
				child(wrapper, "DocumentDeletionModal").props("modelValue"),
			).toBeNull()
		})

		it("asks before deleting a tag, then deletes it", async ({ expect }) => {
			seedWorkspace()
			const deletions = mockEndpoint(
				"DELETE",
				`/api/tags/${TAG_ID}`,
				() => null,
			)
			mockEndpoint("GET", "/api/tags/tree", () => [])
			arrangeRoute()
			const wrapper = await mountPage()
			emitFrom(wrapper, { name: "AppSidebar" }, "delete-tag", {
				id: TAG_ID,
				name: "Ops",
			})
			await nextTick()
			expect(pendingDeletion(wrapper, "TagDeletionModal").name).toBe("Ops")

			await pendingDeletion(wrapper, "TagDeletionModal").deleteTag()

			expect(deletions).toHaveLength(1)
			expect(toast.custom).not.toHaveBeenCalled()
		})

		it("reports a tag deletion the server refused", async ({ expect }) => {
			seedWorkspace()
			mockEndpoint("DELETE", `/api/tags/${TAG_ID}`, () => {
				throw createError({ statusCode: 500 })
			})
			arrangeRoute()
			const wrapper = await mountPage()
			emitFrom(wrapper, { name: "AppSidebar" }, "delete-tag", {
				id: TAG_ID,
				name: "Ops",
			})
			await nextTick()

			await pendingDeletion(wrapper, "TagDeletionModal").deleteTag()

			expect(raisedToasts()).toMatchObject([
				{ type: "error", title: t("sidebar.errors.delete-tag-failed") },
			])
		})
	})

	describe("when a document is duplicated", { concurrent: false }, () => {
		it("opens the copy of the document the sidebar names", async ({
			expect,
		}) => {
			seedWorkspace({
				tree: [
					treeElement(DOC_ID, "Intro"),
					treeElement(OTHER_DOC_ID, "Other"),
				],
			})
			const duplicates = mockDuplication(OTHER_DOC_ID)
			arrangeRoute()
			const wrapper = await mountPage()

			await callListener(
				wrapper,
				"AppSidebar",
				"onDuplicateDocument",
				OTHER_DOC_ID,
			)

			expect(duplicates).toHaveLength(1)
			expect(navigateToMock).toHaveBeenCalledTimes(1)
			expect(navigateToMock).toHaveBeenCalledWith(`/Acme/${COPY_DOC_ID}`, {
				replace: true,
			})
		})

		it("duplicates the open document when the header asks", async ({
			expect,
		}) => {
			seedWorkspace()
			const duplicates = mockDuplication(DOC_ID)
			arrangeRoute()
			const wrapper = await mountPage()

			await callListener(wrapper, "DocumentHeader", "onDuplicateDocument")

			expect(duplicates).toHaveLength(1)
			expect(navigateToMock).toHaveBeenCalledWith(`/Acme/${COPY_DOC_ID}`, {
				replace: true,
			})
		})

		it("stays put when the server answers without a copy", async ({
			expect,
		}) => {
			seedWorkspace()
			mockEndpoint("POST", `/api/documents/${DOC_ID}/duplicate`, () => null)
			mockEndpoint("GET", "/api/documents/tree", () => [
				treeElement(DOC_ID, "Intro"),
			])
			mockEndpoint("GET", "/api/tags/tree", () => [])
			arrangeRoute()
			const wrapper = await mountPage()

			await callListener(wrapper, "DocumentHeader", "onDuplicateDocument")

			expect(navigateToMock).not.toHaveBeenCalled()
			expect(toast.custom).not.toHaveBeenCalled()
		})

		it("reports a duplication the server refused", async ({ expect }) => {
			seedWorkspace()
			mockEndpoint("POST", `/api/documents/${DOC_ID}/duplicate`, () => {
				throw createError({ statusCode: 500 })
			})
			arrangeRoute()
			const wrapper = await mountPage()

			await callListener(wrapper, "DocumentHeader", "onDuplicateDocument")

			expect(raisedToasts()).toMatchObject([
				{ type: "error", title: t("editor.errors.duplication-failed") },
			])
			expect(navigateToMock).not.toHaveBeenCalled()
		})

		it("duplicates nothing when no document is open", async ({ expect }) => {
			seedWorkspace({ tree: [] })
			const duplicates = mockDuplication(DOC_ID)
			arrangeRoute({ documentSlug: undefined })
			const wrapper = await mountPage()

			await callListener(wrapper, "DocumentHeader", "onDuplicateDocument")

			expect(duplicates).toHaveLength(0)
			expect(navigateToMock).not.toHaveBeenCalled()
		})
	})
})

// pageMiddleware is the guard definePageMeta attached to the page's
// route, the way the router would run it before rendering
function pageMiddleware() {
	const route = useRouter()
		.getRoutes()
		.find((r) => r.name === "document-editor")
	const middleware = route?.meta.middleware

	if (typeof middleware !== "function") {
		throw new Error("the document page registers no inline middleware")
	}

	return middleware as RouteMiddleware
}

// the guard creates queries, which need the app context to inject their
// defaults
function runMiddleware(to: RouteLocationNormalized) {
	return runInApp(() => pageMiddleware()(to, to))
}

function documentRoute(
	documentSlug: string | undefined,
	organizationSlug?: string,
) {
	const path = ["", organizationSlug, documentSlug].filter(
		(p) => p !== undefined,
	)

	return {
		params: { organizationSlug, documentSlug },
		query: {},
		hash: "",
		fullPath: path.join("/"),
	} as unknown as RouteLocationNormalized
}

function arrangeRoute(
	route: {
		documentSlug?: string
		query?: Record<string, string>
		hash?: string
	} = {},
) {
	const documentSlug =
		"documentSlug" in route ? route.documentSlug : `Intro-${DOC_ID}`
	const arranged = reactive({
		params: {
			organizationSlug: "Acme",
			documentSlug: documentSlug,
		},
		query: route.query ?? {},
		hash: route.hash ?? "",
		path: documentSlug ? `/Acme/${documentSlug}` : "/Acme",
	})

	useRouteMock.mockReturnValue(arranged)

	return arranged
}

function treeElement(
	id: string,
	name: string,
	children: DocumentTreeElement[] | null = null,
	isProtected = false,
): DocumentTreeElement {
	return {
		id: id,
		documentName: name,
		icon: "mingcute:document-2-fill",
		children: children,
		protected: isProtected,
	}
}

function tagWith(document: DocumentTreeElement): TagTreeElement {
	return {
		id: TAG_ID,
		tagName: "Ops",
		color: "blue",
		hidden: false,
		documents: [document],
	}
}

function branch(id: string, isDefault: boolean): DocumentBranch {
	return {
		branchId: id,
		branchName: isDefault ? "main" : "draft",
		documentName: "Intro",
		icon: "mingcute:document-2-fill",
		protected: false,
		default: isDefault,
		createdAt: "2026-01-01T10:00:00Z",
		updatedAt: "2026-01-01T10:00:00Z",
	}
}

function twoBranches() {
	return [branch(DEFAULT_BRANCH_ID, true), branch(DRAFT_BRANCH_ID, false)]
}

// fresh cache entries answer every refresh, so nothing goes to the network
// unless a test registers an endpoint for it
function seedWorkspace(
	workspace: {
		tree?: DocumentTreeElement[]
		branches?: DocumentBranch[]
		tags?: TagTreeElement[]
	} = {},
) {
	seedAuthOrganization(ORGANIZATION)
	seedCapabilities()
	seedQueryData(
		["documents", "tree"],
		workspace.tree ?? [treeElement(DOC_ID, "Intro")],
	)
	seedQueryData(
		["documents", DOC_ID, "branches"],
		workspace.branches ?? [branch(DEFAULT_BRANCH_ID, true)],
	)
	seedQueryData(["tags", "tree"], workspace.tags ?? [])
}

function clearCapabilities() {
	runInApp(() => {
		const queryCache = useQueryCache()

		queryCache.getEntries({ key: ["capabilities"] }).forEach((entry) => {
			queryCache.remove(entry)
		})
	})
}

function mockBranchViews(branchId: string, respond: () => unknown) {
	return mockEndpoint(
		"POST",
		`/api/documents/${DOC_ID}/branches/${branchId}/views`,
		respond,
	)
}

function mockDocumentDeletion(id: string) {
	mockEndpoint("GET", "/api/tags/tree", () => [])

	return mockEndpoint("DELETE", `/api/documents/${id}`, () => null)
}

function mockDuplication(id: string) {
	mockEndpoint("GET", "/api/documents/tree", () => [
		treeElement(DOC_ID, "Intro"),
		treeElement(COPY_DOC_ID, "Intro copy"),
	])
	mockEndpoint("GET", "/api/tags/tree", () => [])

	return mockEndpoint("POST", `/api/documents/${id}/duplicate`, () => ({
		id: COPY_DOC_ID,
	}))
}

function stubWebSocket() {
	const unsubscribe = vi.fn()
	const subscribe = vi.fn(
		(_topic: string, _fn: (payload: object) => void) => unsubscribe,
	)

	useWebSocketStateStore().state = { subscribe: subscribe } as never

	return { subscribe, unsubscribe }
}

function publishMetadata(
	subscribe: ReturnType<typeof stubWebSocket>["subscribe"],
	payload: WSDocumentMetadataPayload,
) {
	const listener = subscribe.mock.calls[0]?.[1]
	if (!listener) {
		throw new Error("the page subscribed to no metadata changes")
	}

	listener(payload)
}

function mountPage() {
	return mountSuspended(DocumentEditor, { global: { stubs: CHILD_STUBS } })
}

function child(wrapper: VueWrapper, name: string) {
	return wrapper.findComponent({ name: name })
}

// the inbox and the variables panel each sit in a context sidebar of
// their own, told apart by their title
function contextSidebar(wrapper: VueWrapper, title: string) {
	const sidebar = wrapper
		.findAllComponents({ name: "ExtraContextSidebar" })
		.find((candidate) => candidate.props("title") === title)
	if (!sidebar) {
		throw new Error(`no context sidebar titled "${title}"`)
	}

	return sidebar
}

async function openVariableEditing() {
	const wrapper = await mountPage()

	emitFrom(wrapper, { name: "AppSidebar" }, "toggle-variables")
	await nextTick()
	emitFrom(wrapper, { name: "VariablesPanel" }, "update:editing", true)
	await nextTick()

	return wrapper
}

async function finishLoading(wrapper: VueWrapper) {
	emitFrom(wrapper, { name: "AppSidebar" }, "initial-load-complete")
	emitFrom(
		wrapper,
		{ name: "EditorDocumentContainer" },
		"initial-load-complete",
	)
	await nextTick()
}

function pendingDeletion(
	wrapper: VueWrapper,
	modal: "DocumentDeletionModal",
): {
	name: string
	deleteDocument: () => Promise<void>
}

function pendingDeletion(
	wrapper: VueWrapper,
	modal: "TagDeletionModal",
): {
	name: string
	deleteTag: () => Promise<void>
}

function pendingDeletion(wrapper: VueWrapper, modal: string) {
	const pending: unknown = child(wrapper, modal).props("modelValue")
	if (!pending) {
		throw new Error(`the ${modal} has nothing to delete`)
	}

	return pending
}

// a creation holds its spinner for delay(500), and the request and the
// tree refetch after it each need their own turn of the frozen clock
async function settleCreation() {
	await vi.advanceTimersByTimeAsync(500)
	await settleMutations()
	await vi.advanceTimersByTimeAsync(0)
	await settleMutations()
}

// mountSuspended points the test router at "/" with a plain string. The
// page always replaces the url with a location object.
function pageReplaces() {
	return vi
		.mocked(useRouter())
		.replace.mock.calls.map(([to]) => to)
		.filter((to) => typeof to === "object")
}

// the stub's listener is the page's own handler, so calling it hands back
// the handler's promise, which an emit would drop
async function callListener(
	wrapper: VueWrapper,
	name: string,
	listener: string,
	...args: unknown[]
) {
	// eslint-disable-next-line @typescript-eslint/no-unsafe-member-access -- findComponent by name types the stub as any, while $attrs is a plain record
	const handler: unknown = child(wrapper, name).vm.$attrs[listener]
	if (typeof handler !== "function") {
		throw new Error(`the ${name} stub has no ${listener} listener`)
	}

	await (handler as (...a: unknown[]) => unknown)(...args)
}

// the router follows a redirect before the page goes on, so the url stops
// naming the document it left
function followRedirects(route: ReturnType<typeof arrangeRoute>) {
	navigateToMock.mockImplementation((to: string) => {
		route.params.documentSlug = to.split("/")[2]
	})
}
