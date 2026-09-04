import { afterEach, beforeEach, describe, it, vi } from "vitest"
import { toast } from "vue-sonner"
import {
	clearQueryCache,
	disposeMockEndpoints,
	mockEndpoint,
	seedQueryData,
} from "~/composables/api/test-helpers"
import DocumentHeader from "./DocumentHeader.vue"
import {
	at,
	findButtonByText,
	menuItem,
	mountUnderSidebarProvider,
	seedCapabilities,
	settleMutations,
	t,
} from "./test-helpers"

vi.mock("vue-sonner", () => ({
	toast: { custom: vi.fn(), dismiss: vi.fn() },
}))

const DOC_ID = "doc1".padEnd(20, "0")
const MAIN_BRANCH = "main".padEnd(20, "0")
const DRAFT_BRANCH = "draft".padEnd(20, "0")
// the divider between the document options and the assistant button is a
// bare div, and the header draws no other one
const ASSISTANT_SEPARATOR_SELECTOR = "div.w-px"

const BREADCRUMBS = [
	{ id: DOC_ID, name: "Runbook", href: "/Runbook", icon: "lucide:file" },
]

// the branch mutations invalidate the branch list and the document tree on
// success; without endpoints behind them the refetch rejects and takes the
// mutation down with it
let branches: unknown[] = []

function seedBranches(list: unknown[]) {
	branches = list
	seedQueryData(["documents", DOC_ID, "branches"], list)
}

function mainOnly() {
	seedBranches([{ branchId: MAIN_BRANCH, branch: "main", default: true }])
}

function mainAndDraft() {
	seedBranches([
		{ branchId: MAIN_BRANCH, branch: "main", default: true },
		{ branchId: DRAFT_BRANCH, branch: "draft", default: false },
	])
}

function mountHeader(props: Record<string, unknown> = {}) {
	return mountUnderSidebarProvider(DocumentHeader, {
		props: {
			allInitialSectionsLoaded: true,
			breadcrumbs: BREADCRUMBS,
			timestamps: null,
			...props,
		},
	})
}

async function openOptionsMenu(
	wrapper: Awaited<ReturnType<typeof mountHeader>>,
) {
	const triggers = wrapper.findAll("[data-slot='dropdown-menu-trigger']")

	await at(triggers, triggers.length - 1).trigger("click")
}

// the editor store, the query cache and the vue-sonner module mock are all
// app-wide singletons every mount in the file shares, and the menus are
// teleported into the shared <body>
describe("<DocumentHeader>", { concurrent: false }, () => {
	beforeEach(() => {
		clearQueryCache()
		vi.mocked(toast.custom).mockReset()
		mockEndpoint("GET", `/api/documents/${DOC_ID}/branches`, () => branches)
		mockEndpoint("GET", "/api/documents/tree", () => [])

		const editorStore = useEditorStore()
		editorStore.activeDocumentId = DOC_ID
		editorStore.activeBranchId = MAIN_BRANCH
		editorStore.mappedDefaultBranchId = MAIN_BRANCH
		editorStore.aiAssistantOpen = false
		useEditorMeta().setEditable(true)
	})

	afterEach(disposeMockEndpoints)

	it("stays blank until the initial sections have loaded", async ({
		expect,
	}) => {
		mainOnly()

		const wrapper = await mountHeader({ allInitialSectionsLoaded: false })

		expect(wrapper.findComponent(DocumentHeader).text()).toBe("")
	})

	it("shows the document breadcrumbs", async ({ expect }) => {
		mainOnly()

		const wrapper = await mountHeader()

		expect(wrapper.text()).toContain("Runbook")
	})

	it("omits the breadcrumbs while the document is still loading", async ({
		expect,
	}) => {
		mainOnly()

		const wrapper = await mountHeader({ breadcrumbs: null })

		expect(wrapper.text()).not.toContain("Runbook")
	})

	it("shows when the document was last edited", async ({ expect }) => {
		mainOnly()

		const wrapper = await mountHeader({
			timestamps: {
				[MAIN_BRANCH]: {
					updated: { at: "2026-03-14T12:00:00Z", user: { name: "Ada" } },
					created: { at: "2026-03-01T12:00:00Z", user: { name: "Ada" } },
				},
			},
		})

		expect(wrapper.text()).toContain("Edited Mar 14")
	})

	it("omits the timestamp when the branch has none", async ({ expect }) => {
		mainOnly()

		const wrapper = await mountHeader({ timestamps: {} })

		expect(wrapper.text()).not.toContain("Edited")
	})

	it("offers a read/edit toggle while the document is not reviewable", async ({
		expect,
	}) => {
		mainOnly()

		const wrapper = await mountHeader()

		expect(wrapper.text()).toContain(t("editor.navbar.toggle-edit-mode"))
	})

	it("switches from edit mode to read mode", async ({ expect }) => {
		mainOnly()
		const wrapper = await mountHeader()

		await findButtonByText(
			wrapper,
			t("editor.navbar.toggle-edit-mode"),
		).trigger("click")

		expect(wrapper.text()).toContain(t("editor.navbar.toggle-read-mode"))
	})

	it("switches from read mode back to edit mode", async ({ expect }) => {
		mainOnly()
		useEditorMeta().setEditable(false)
		const wrapper = await mountHeader()

		await findButtonByText(
			wrapper,
			t("editor.navbar.toggle-read-mode"),
		).trigger("click")

		expect(wrapper.text()).toContain(t("editor.navbar.toggle-edit-mode"))
	})

	it("offers a version picker once the document is reviewable", async ({
		expect,
	}) => {
		mainAndDraft()

		const wrapper = await mountHeader()

		expect(wrapper.text()).toContain(
			t("editor.navbar.document-modes.main.title"),
		)
	})

	it("names the draft version when the draft branch is active", async ({
		expect,
	}) => {
		mainAndDraft()
		useEditorStore().activeBranchId = DRAFT_BRANCH

		const wrapper = await mountHeader()

		expect(wrapper.text()).toContain(
			t("editor.navbar.document-modes.draft.title"),
		)
	})

	it("falls back to the main version when the active branch is unknown", async ({
		expect,
	}) => {
		mainAndDraft()
		useEditorStore().activeBranchId = "gone".padEnd(20, "0")

		const wrapper = await mountHeader()

		expect(wrapper.text()).toContain(
			t("editor.navbar.document-modes.main.title"),
		)
	})

	it("switches to the draft branch from the version picker", async ({
		expect,
	}) => {
		mainAndDraft()
		const editorStore = useEditorStore()
		const wrapper = await mountHeader()
		await wrapper.get("[data-slot='dropdown-menu-trigger']").trigger("click")

		menuItem(t("editor.navbar.document-modes.draft.title")).click()
		await nextTick()

		expect(editorStore.activeBranchId).toBe(DRAFT_BRANCH)
		expect(editorStore.targetBranchId).toBe(MAIN_BRANCH)
	})

	it("switches back to the main branch from the version picker", async ({
		expect,
	}) => {
		mainAndDraft()
		const editorStore = useEditorStore()
		editorStore.activeBranchId = DRAFT_BRANCH
		const wrapper = await mountHeader()
		await wrapper.get("[data-slot='dropdown-menu-trigger']").trigger("click")

		menuItem(t("editor.navbar.document-modes.main.title")).click()
		await nextTick()

		expect(editorStore.activeBranchId).toBe(MAIN_BRANCH)
		expect(editorStore.targetBranchId).toBeNull()
	})

	it("asks to duplicate the document from the options menu", async ({
		expect,
	}) => {
		mainOnly()
		const wrapper = await mountHeader()
		await openOptionsMenu(wrapper)

		menuItem(t("editor.navbar.document-options.duplicate.title")).click()
		await nextTick()

		expect(
			wrapper.findComponent(DocumentHeader).emitted("duplicate-document"),
		).toHaveLength(1)
	})

	it("asks to delete the document from the options menu", async ({
		expect,
	}) => {
		mainOnly()
		const wrapper = await mountHeader()
		await openOptionsMenu(wrapper)

		menuItem(t("editor.navbar.document-options.delete.title")).click()
		await nextTick()

		expect(
			wrapper.findComponent(DocumentHeader).emitted("delete-document"),
		).toHaveLength(1)
	})

	it("opens the review workflow by branching off main and protecting it", async ({
		expect,
	}) => {
		mainOnly()
		const created = mockEndpoint(
			"POST",
			`http://test.local/auth-realtime/api/documents/${DOC_ID}/branches`,
			() => ({ branchId: DRAFT_BRANCH }),
		)
		const updated = mockEndpoint(
			"PUT",
			`http://test.local/auth-realtime/api/documents/${DOC_ID}/branches/${MAIN_BRANCH}`,
			() => ({}),
		)
		const wrapper = await mountHeader()
		await openOptionsMenu(wrapper)

		menuItem(
			t("editor.navbar.document-options.review-workflow.enable-title"),
		).click()
		await settleMutations()

		expect(created).toHaveLength(1)
		expect(created[0]?.body).toEqual({
			branch: "draft",
			sourceBranchId: MAIN_BRANCH,
		})
		expect(updated[0]?.body).toEqual({ protected: true })
	})

	it("closes the review workflow by dropping every draft and unprotecting main", async ({
		expect,
	}) => {
		mainAndDraft()
		const deleted = mockEndpoint(
			"DELETE",
			`/api/documents/${DOC_ID}/branches/${DRAFT_BRANCH}`,
			() => ({}),
		)
		const updated = mockEndpoint(
			"PUT",
			`http://test.local/auth-realtime/api/documents/${DOC_ID}/branches/${MAIN_BRANCH}`,
			() => ({}),
		)
		const wrapper = await mountHeader()
		await openOptionsMenu(wrapper)

		menuItem(
			t("editor.navbar.document-options.review-workflow.disable-title"),
		).click()
		await settleMutations()

		expect(deleted).toHaveLength(1)
		expect(updated[0]?.body).toEqual({ protected: false })
	})

	it("warns when the reviewability change fails", async ({ expect }) => {
		mainOnly()
		mockEndpoint(
			"POST",
			`http://test.local/auth-realtime/api/documents/${DOC_ID}/branches`,
			() => {
				throw createError({ statusCode: 500 })
			},
		)
		const wrapper = await mountHeader()
		await openOptionsMenu(wrapper)

		menuItem(
			t("editor.navbar.document-options.review-workflow.enable-title"),
		).click()
		await settleMutations()

		expect(toast.custom).toHaveBeenCalledTimes(1)
	})

	it("leaves the branches alone when no document is active", async ({
		expect,
	}) => {
		mainOnly()
		const created = mockEndpoint(
			"POST",
			`http://test.local/auth-realtime/api/documents/${DOC_ID}/branches`,
			() => ({ branchId: DRAFT_BRANCH }),
		)
		const wrapper = await mountHeader()
		await openOptionsMenu(wrapper)
		useEditorStore().activeDocumentId = null

		menuItem(
			t("editor.navbar.document-options.review-workflow.enable-title"),
		).click()
		await settleMutations()

		expect(created).toHaveLength(0)
	})

	it("hides the assistant button and its separator on a deployment without the assistant", async ({
		expect,
	}) => {
		mainOnly()
		seedCapabilities({ aiAssistant: { status: AssistantStatus.Inactive } })

		const wrapper = await mountHeader()

		expect(wrapper.text()).not.toContain(t("editor.navbar.open-ai-assistant"))
		expect(wrapper.findAll(ASSISTANT_SEPARATOR_SELECTOR)).toHaveLength(0)
	})

	it("offers the assistant button behind a separator when the assistant runs", async ({
		expect,
	}) => {
		mainOnly()
		seedCapabilities({
			aiAssistant: { status: AssistantStatus.ActiveButWeak, model: "small" },
		})

		const wrapper = await mountHeader()

		expect(wrapper.text()).toContain(t("editor.navbar.open-ai-assistant"))
		expect(wrapper.findAll(ASSISTANT_SEPARATOR_SELECTOR)).toHaveLength(1)
	})

	it("publishes the header height so the page can offset content", async ({
		expect,
	}) => {
		mainOnly()

		await mountHeader()

		expect(
			document.documentElement.style.getPropertyValue(
				"--document-header-height",
			),
		).toBe("0px")
	})
})
