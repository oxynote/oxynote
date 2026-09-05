import { registerEndpoint } from "@nuxt/test-utils/runtime"
import type { HocuspocusProvider } from "@hocuspocus/provider"
import { enableAutoUnmount, type VueWrapper } from "@vue/test-utils"
import { setResponseStatus } from "h3"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import { toast } from "vue-sonner"
import { Awareness } from "y-protocols/awareness"
import * as Y from "yjs"
import NameEditor from "./NameEditor.vue"
import IconPicker from "./IconPicker.vue"
import DiffTitle from "./diff/DiffTitle.vue"
import ReviewerList from "./ReviewerList.vue"
import {
	clearQueryCache,
	disposeMockEndpoints,
	makeXid,
	mockEndpoint,
	seedQueryData,
	trackEndpointDisposal,
} from "~/composables/api/test-helpers"
import {
	clearTeleportedOverlays,
	emitFrom,
	findButtonByText,
	menuItem,
	mountUnderTooltipProvider,
	seedAuthOrganization,
	seedAuthSession,
	settleMutations,
	t,
	WAIT_FOR_OPTIONS,
} from "~/components/test-helpers"

vi.mock("vue-sonner", () => ({
	toast: { custom: vi.fn(), dismiss: vi.fn() },
}))

const DOCUMENT_ID = makeXid("doc")
const BRANCH_ID = makeXid("brancha")
const TARGET_BRANCH_ID = makeXid("branchb")
const ME = makeXid("usme")

const REVIEWABLE_ACTION_DELAY_MS = 300

interface Branch {
	ydoc: Y.Doc
	provider: HocuspocusProvider
}

function makeBranch(icon = "mingcute:tag-fill"): Branch {
	const ydoc = new Y.Doc()
	ydoc.getText("icon").insert(0, icon)

	return {
		ydoc: ydoc,
		provider: {
			document: ydoc,
			awareness: new Awareness(ydoc),
		} as unknown as HocuspocusProvider,
	}
}

function titleOf(ydoc: Y.Doc, text: string) {
	const paragraph = new Y.XmlElement("paragraph")
	paragraph.insert(0, [new Y.XmlText(text)])
	ydoc.getXmlFragment("name").insert(0, [paragraph])
}

// the merge request goes through the auth-realtime client, which asks for
// an absolute url; the path spelling is the one the test-time app answers
function mockMergeEndpoint(respond: () => unknown) {
	const calls: unknown[] = []
	const handler = () => {
		calls.push(true)

		return respond()
	}

	trackEndpointDisposal(
		registerEndpoint(
			`http://test.local/auth-realtime/api/documents/${DOCUMENT_ID}/merge`,
			{ method: "PUT", handler: handler },
		),
	)
	trackEndpointDisposal(
		registerEndpoint(`/auth-realtime/api/documents/${DOCUMENT_ID}/merge`, {
			method: "PUT",
			handler: handler,
		}),
	)

	return calls
}

function mountEditor(
	options: {
		active?: Branch
		target?: Branch | null
		documentHooks?: DocumentHook[]
		contentEditor?: unknown
	} = {},
) {
	const active = options.active ?? makeBranch()

	// the hook icon stack carries tooltips, whose context the app installs
	// once at page level
	return mountUnderTooltipProvider(NameEditor, {
		props: {
			documentHooks: options.documentHooks ?? [],
			activeBranchYdoc: active.ydoc,
			activeBranchProvider: active.provider,
			targetBranchProvider: options.target?.provider ?? null,
			contentEditor: options.contentEditor ?? null,
			userCaretDetails: { name: "Me", color: "#ff0000" },
			timestamps: {},
		},
	})
}

async function openActionMenu(wrapper: VueWrapper) {
	const trigger = actionMenuTrigger(wrapper)
	await trigger?.trigger("pointerdown", { button: 0 })
	await trigger?.trigger("click")
	await nextTick()
}

// the icon picker comes first, then the review action and the menu that
// switches between the review actions
function actionMenuTrigger(wrapper: VueWrapper) {
	return wrapper.findAll("button")[2]
}

function hookHandle(wrapper: VueWrapper) {
	return wrapper.get(".group\\/hook-handle")
}

// the panel marking the title is appended to the body, the way the drag
// handle marks the block it points at
function highlightPanels(): NodeListOf<Element> {
	return document.body.querySelectorAll("[aria-hidden='true'].z-editor-overlay")
}

// the wrapper is the tooltip provider the editor is mounted inside, so
// the editor's own events are read off the component
function emittedFrom(wrapper: VueWrapper, event: string): unknown[][] {
	return wrapper.findComponent(NameEditor).emitted(event) ?? []
}

// the editor store, the auth and query caches, the mocked toast module
// and the teleported overlays are all shared, so these tests cannot
// interleave
describe("<NameEditor>", { concurrent: false }, () => {
	enableAutoUnmount(afterEach)

	beforeEach(() => {
		clearTeleportedOverlays()
		clearQueryCache()
		vi.mocked(toast.custom).mockReset()
		useEditorMeta().setEditable(true)
		useEditorStore().updateActiveDocumentId(DOCUMENT_ID)
		useEditorStore().updateActiveBranchId(BRANCH_ID)
		useEditorStore().updateTargetBranchId(TARGET_BRANCH_ID)
		useEditorStore().setBranchReviewableActionsActive(false)
		useEditorStore().setReviewableDiffActive(false)
		seedAuthSession({ id: ME, name: "Me" })
		seedAuthOrganization({ members: [] })
		seedQueryData(["documents", DOCUMENT_ID, "branches"], [{ id: BRANCH_ID }])
		seedQueryData(
			["documents", DOCUMENT_ID, "branches", BRANCH_ID, "reviewers"],
			[],
		)
	})

	afterEach(() => {
		vi.useRealTimers()
		disposeMockEndpoints()
	})

	it("shows the icon the branch carries", async ({ expect }) => {
		const wrapper = await mountEditor({
			active: makeBranch("mingcute:at-fill"),
		})

		expect(wrapper.findComponent(IconPicker).props("icon")).toBe(
			"mingcute:at-fill",
		)
	})

	it("stores a newly picked icon on the branch", async ({ expect }) => {
		const active = makeBranch()
		const wrapper = await mountEditor({ active: active })

		emitFrom(wrapper, IconPicker, "select", "mingcute:hashtag-fill")
		await nextTick()

		expect(active.ydoc.getText("icon").toJSON()).toBe("mingcute:hashtag-fill")
		expect(emittedFrom(wrapper, "updated-live-icon").at(-1)).toEqual([
			"mingcute:hashtag-fill",
		])
	})

	it("follows an icon change made by a collaborator", async ({ expect }) => {
		const active = makeBranch()
		const wrapper = await mountEditor({ active: active })

		active.ydoc.getText("icon").delete(0, active.ydoc.getText("icon").length)
		active.ydoc.getText("icon").insert(0, "mingcute:at-fill")
		await nextTick()

		expect(wrapper.findComponent(IconPicker).props("icon")).toBe(
			"mingcute:at-fill",
		)
	})

	it("marks the icon as changed only while a diff is shown", async ({
		expect,
	}) => {
		const wrapper = await mountEditor({
			active: makeBranch("mingcute:at-fill"),
			target: makeBranch("mingcute:tag-fill"),
		})

		expect(wrapper.findComponent(IconPicker).props("isModified")).toBe(false)

		useEditorStore().setReviewableDiffActive(true)
		await nextTick()

		expect(wrapper.findComponent(IconPicker).props("isModified")).toBe(true)
	})

	it("hands the name editor to the host once it is ready", async ({
		expect,
	}) => {
		const wrapper = await mountEditor()

		await vi.waitFor(() => {
			expect(emittedFrom(wrapper, "editor-ready")).toHaveLength(1)
		}, WAIT_FOR_OPTIONS)
	})

	it("shows the name the branch carries", async ({ expect }) => {
		const active = makeBranch()
		titleOf(active.ydoc, "Payments runbook")

		const wrapper = await mountEditor({ active: active })

		await vi.waitFor(() => {
			expect(wrapper.text()).toContain("Payments runbook")
		}, WAIT_FOR_OPTIONS)
	})

	it("reports the name as it is edited", async ({ expect }) => {
		const active = makeBranch()
		const wrapper = await mountEditor({ active: active })
		await vi.waitFor(() => {
			expect(emittedFrom(wrapper, "editor-ready")).toHaveLength(1)
		}, WAIT_FOR_OPTIONS)

		titleOf(active.ydoc, "Renamed")

		await vi.waitFor(() => {
			expect(emittedFrom(wrapper, "updated-live-name").at(-1)).toEqual([
				"Renamed",
			])
		}, WAIT_FOR_OPTIONS)
	})

	it("shows the title diff instead of the editor while a diff is on", async ({
		expect,
	}) => {
		const wrapper = await mountEditor({ target: makeBranch() })

		expect(wrapper.findComponent(DiffTitle).exists()).toBe(false)

		useEditorStore().setReviewableDiffActive(true)
		await nextTick()

		expect(wrapper.findComponent(DiffTitle).exists()).toBe(true)
	})

	it("keeps the plain title when there is nothing to diff against", async ({
		expect,
	}) => {
		const wrapper = await mountEditor({ target: null })
		useEditorStore().setReviewableDiffActive(true)

		await nextTick()

		expect(wrapper.findComponent(DiffTitle).exists()).toBe(false)
	})

	it("shows the page's own hooks", async ({ expect }) => {
		const wrapper = await mountEditor({
			documentHooks: [
				{
					id: "hook-1",
					type: DocumentHookType.URLWatcher,
					blockId: null,
					score: "100",
					settings: { url: "https://oxynote.test" },
					state: { status: "active" },
				} as unknown as DocumentHook,
				{
					id: "hook-2",
					type: DocumentHookType.URLWatcher,
					blockId: "block-1",
					score: "100",
					settings: { url: "https://other.test" },
					state: { status: "active" },
				} as unknown as DocumentHook,
			],
		})

		expect(wrapper.text()).toContain(t("editor.hook-handle.screen-reader-hint"))
		expect(
			wrapper.find("[data-hook-status]").attributes("data-hook-status"),
		).toBe("fresh")
		expect(wrapper.find(".bg-hook-decoration").attributes("style")).toContain(
			"display: none",
		)
	})

	it("leaves the hook handle untinted on a page with no hooks", async ({
		expect,
	}) => {
		const wrapper = await mountEditor()

		expect(wrapper.find("[data-hook-status]").exists()).toBe(false)
	})

	it("marks the title when the hook handle is hovered", async ({ expect }) => {
		const wrapper = await mountEditor()

		await hookHandle(wrapper).trigger("mouseenter")

		expect(highlightPanels()).toHaveLength(1)
	})

	it("keeps the mark on while the hook menu is open", async ({ expect }) => {
		const wrapper = await mountEditor()
		const handle = hookHandle(wrapper)
		await handle.trigger("pointerdown", { button: 0 })
		await handle.trigger("click")
		await nextTick()

		await handle.trigger("mouseleave")

		expect(highlightPanels()).toHaveLength(1)
	})

	it("clears the mark when the pointer leaves the hook handle", async ({
		expect,
	}) => {
		const wrapper = await mountEditor()
		await hookHandle(wrapper).trigger("mouseenter")

		await hookHandle(wrapper).trigger("mouseleave")

		expect(highlightPanels()).toHaveLength(0)
	})

	it("takes the hook handle away while a diff is on", async ({ expect }) => {
		const wrapper = await mountEditor({ target: makeBranch() })

		useEditorStore().setReviewableDiffActive(true)
		await nextTick()

		expect(wrapper.text()).not.toContain(
			t("editor.hook-handle.screen-reader-hint"),
		)
	})

	it("marks the page when one of its hooks has fired", async ({ expect }) => {
		const wrapper = await mountEditor({
			documentHooks: [
				{
					id: "hook-1",
					type: DocumentHookType.URLWatcher,
					blockId: null,
					score: "0",
					settings: { url: "https://oxynote.test" },
					state: { status: "active" },
				} as unknown as DocumentHook,
			],
		})

		expect(
			wrapper.find("[data-hook-status]").attributes("data-hook-status"),
		).toBe("stale")
		expect(
			wrapper.find(".bg-hook-decoration").attributes("style"),
		).toBeUndefined()
	})

	it("offers no review actions while the branch has none", async ({
		expect,
	}) => {
		const wrapper = await mountEditor()

		expect(wrapper.text()).not.toContain(
			t("editor.name-editor.review-workflow.approve.title"),
		)
		expect(wrapper.text()).not.toContain(
			t("editor.name-editor.review-workflow.show-diff"),
		)
	})

	it("offers to approve a reviewable branch", async ({ expect }) => {
		useEditorStore().setBranchReviewableActionsActive(true)

		const wrapper = await mountEditor()

		expect(wrapper.text()).toContain(
			t("editor.name-editor.review-workflow.approve.title"),
		)
		expect(wrapper.text()).toContain(
			t("editor.name-editor.review-workflow.show-diff"),
		)
	})

	it("offers to revoke an approval the reader already gave", async ({
		expect,
	}) => {
		useEditorStore().setBranchReviewableActionsActive(true)
		seedQueryData(
			["documents", DOCUMENT_ID, "branches", BRANCH_ID, "reviewers"],
			[{ userId: ME, currentlyApproved: true }],
		)

		const wrapper = await mountEditor()

		expect(wrapper.text()).toContain(
			t("editor.name-editor.review-workflow.unapprove.title"),
		)
	})

	it("approves the branch", async ({ expect }) => {
		useEditorStore().setBranchReviewableActionsActive(true)
		const calls = mockEndpoint(
			"PUT",
			`/api/documents/${DOCUMENT_ID}/branches/${BRANCH_ID}/review-approve`,
			() => null,
		)
		const wrapper = await mountEditor()
		vi.useFakeTimers()

		await findButtonByText(
			wrapper,
			t("editor.name-editor.review-workflow.approve.title"),
		).trigger("click")
		await vi.advanceTimersByTimeAsync(REVIEWABLE_ACTION_DELAY_MS)
		await settleMutations()

		expect(calls).toHaveLength(1)
		expect(calls[0]?.body).toEqual({ approved: true })
		expect(toast.custom).toHaveBeenCalledTimes(1)
	})

	it("warns when the approval fails", async ({ expect }) => {
		useEditorStore().setBranchReviewableActionsActive(true)
		mockEndpoint(
			"PUT",
			`/api/documents/${DOCUMENT_ID}/branches/${BRANCH_ID}/review-approve`,
			(_c, event) => {
				setResponseStatus(event, 500)

				return { message: "boom" }
			},
		)
		const wrapper = await mountEditor()
		vi.useFakeTimers()

		await findButtonByText(
			wrapper,
			t("editor.name-editor.review-workflow.approve.title"),
		).trigger("click")
		await vi.advanceTimersByTimeAsync(REVIEWABLE_ACTION_DELAY_MS)
		await settleMutations()

		expect(toast.custom).toHaveBeenCalledTimes(1)
	})

	it("switches the action to merging the branch", async ({ expect }) => {
		useEditorStore().setBranchReviewableActionsActive(true)
		const wrapper = await mountEditor()

		await openActionMenu(wrapper)
		menuItem(t("editor.name-editor.review-workflow.merge.title")).click()
		await nextTick()

		expect(wrapper.text()).toContain(
			t("editor.name-editor.review-workflow.merge.title"),
		)
	})

	it("merges the branch", async ({ expect }) => {
		useEditorStore().setBranchReviewableActionsActive(true)
		const calls = mockMergeEndpoint(() => null)
		// merging invalidates the branch and reviewer queries, which fetch
		// again before the mutation settles
		mockEndpoint("GET", `/api/documents/${DOCUMENT_ID}/branches`, () => [
			{ id: BRANCH_ID },
		])
		mockEndpoint(
			"GET",
			`/api/documents/${DOCUMENT_ID}/branches/${BRANCH_ID}/reviewers`,
			() => [],
		)
		mockEndpoint(
			"GET",
			`/api/documents/${DOCUMENT_ID}/branches/${TARGET_BRANCH_ID}/reviewers`,
			() => [],
		)
		const wrapper = await mountEditor()
		await openActionMenu(wrapper)
		menuItem(t("editor.name-editor.review-workflow.merge.title")).click()
		await nextTick()

		// the merge invalidates queries that fetch again, so the run is
		// left on the real clock rather than being stepped through
		await findButtonByText(
			wrapper,
			t("editor.name-editor.review-workflow.merge.title"),
		).trigger("click")

		await vi.waitFor(() => {
			expect(emittedFrom(wrapper, "branch-merged")).toEqual([[false]])
		}, WAIT_FOR_OPTIONS)
		expect(calls).toHaveLength(1)
	})

	it("turns the diff on and off", async ({ expect }) => {
		useEditorStore().setBranchReviewableActionsActive(true)
		const wrapper = await mountEditor({ target: makeBranch() })

		await wrapper.get("[role='switch']").trigger("click")

		expect(useEditorStore().reviewableDiffActive).toBe(true)
		expect(emittedFrom(wrapper, "diff-mode-changed")).toEqual([[true]])
	})

	it("turns the diff off when the branch stops being reviewable", async ({
		expect,
	}) => {
		useEditorStore().setBranchReviewableActionsActive(true)
		await mountEditor({ target: makeBranch() })
		useEditorStore().setReviewableDiffActive(true)

		useEditorStore().setBranchReviewableActionsActive(false)
		await nextTick()

		expect(useEditorStore().reviewableDiffActive).toBe(false)
	})

	it("shows the reviewers only when the page has more than one branch", async ({
		expect,
	}) => {
		seedQueryData(
			["documents", DOCUMENT_ID, "branches"],
			[{ id: BRANCH_ID }, { id: TARGET_BRANCH_ID }],
		)

		const wrapper = await mountEditor()

		expect(wrapper.findComponent(ReviewerList).exists()).toBe(true)
	})

	it("hides the reviewers for a page with a single branch", async ({
		expect,
	}) => {
		const wrapper = await mountEditor()

		expect(wrapper.findComponent(ReviewerList).exists()).toBe(false)
	})
})
