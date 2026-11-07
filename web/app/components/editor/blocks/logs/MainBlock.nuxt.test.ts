import { mockNuxtImport } from "@nuxt/test-utils/runtime"
import { enableAutoUnmount, type VueWrapper } from "@vue/test-utils"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import MainBlock from "./MainBlock.vue"
import { LogTimestampFormat, type LogConfig } from "./utils"
import { GridItemWidth } from "../grid/utils"
import {
	makeEditor,
	makeNode,
	mountNodeView,
} from "../../test-helpers/node-view"
import { makeLog, logsResult, settleLogRequests } from "./test-helpers"
import { DiffStatus } from "~/components/editor/diff/position-map"
import {
	clearQueryCache,
	disposeMockEndpoints,
	makeXid,
	mockEndpoint,
	seedQueryData,
} from "~/composables/api/test-helpers"
import { VARIABLE_QUERY_KEYS } from "~/composables/api/useVariableAPI"
import { RefreshInterval, TimeRangePreset } from "~/utils/time-range"
import { findButtonByText, t } from "~/components/test-helpers"

// happy-dom lays nothing out, so the real scroller would show no row
vi.mock("vue-virtual-scroller", async () =>
	(await import("./test-helpers")).virtualScrollerStub(),
)

// happy-dom has no intersection observer either, so the block would never
// count as near the viewport, and it reads only when near
mockNuxtImport("useElementVisibility", () => () => ref(true))

const DOCUMENT_ID = makeXid("doc")
const BRANCH_ID = makeXid("branch")
const UID = "log-block-1"
const DATA_SOURCE_ID = makeXid("ds")

// the block keeps watching the store after its test
enableAutoUnmount(afterEach)

// the editor store, the query cache and the editable flag are shared
// app-wide, so these tests cannot interleave
describe("<LogMainBlock>", { concurrent: false }, () => {
	beforeEach(() => {
		const editorStore = useEditorStore()

		clearQueryCache()
		seedQueryData(VARIABLE_QUERY_KEYS.tree, [])
		useEditorMeta().setEditable(true)
		// leaving the document drops every block entry the last test stored
		editorStore.updateActiveDocumentId(null)
		editorStore.updateActiveDocumentId(DOCUMENT_ID)
		editorStore.updateActiveBranchId(BRANCH_ID)
		editorStore.setReviewableDiffActive(false)
		editorStore.activateLogBlockConfig(null)
		// the view picks how to write its queries from the source's type
		mockEndpoint("GET", "/api/data-sources", () => [
			{ id: DATA_SOURCE_ID, name: "Loki", type: DataSourceType.Loki },
		])
	})

	afterEach(disposeMockEndpoints)

	it("identifies the wrapper by the node's uid", async ({ expect }) => {
		const wrapper = await mountBlock()

		const root = wrapper.get("[data-node-view-wrapper]")

		expect(root.attributes("id")).toBe(UID)
		expect(root.attributes("data-uid")).toBe(UID)
	})

	it("exposes the node's comment id and diff status on the wrapper", async ({
		expect,
	}) => {
		const wrapper = await mountBlock({
			attrs: { nodeCommentId: "comment-1", diffStatus: DiffStatus.Added },
		})

		const root = wrapper.get("[data-node-view-wrapper]")

		expect(root.attributes("data-node-comment-id")).toBe("comment-1")
		expect(root.attributes("data-diff-status")).toBe("added")
	})

	it.for([GridItemWidth.Compact, GridItemWidth.Standard, GridItemWidth.Wide])(
		"gives a %s block its own grid span",
		async (width, { expect }) => {
			const wrapper = await mountBlock({ attrs: { width: width } })

			expect(wrapperClasses(wrapper)).toContain(`grid-item-${width}`)
		},
	)

	it("spans a block with no stored width like a standard one", async ({
		expect,
	}) => {
		const wrapper = await mountBlock()

		expect(wrapperClasses(wrapper)).toContain("grid-item-standard")
	})

	it("publishes the block's config for the config modal to read", async ({
		expect,
	}) => {
		await mountBlock({
			attrs: {
				title: "API errors",
				dataSourceId: "ds-1",
				queries: [{ name: "Query 1", query: '{app="api"}' }],
				timeRange: TimeRangePreset.Last1Hour,
				refreshInterval: RefreshInterval.M1,
				timestampFormat: LogTimestampFormat.Numeric,
				order: LogOrder.Asc,
			},
		})

		expect(storedConfig()).toEqual({
			title: "API errors",
			dataSourceId: "ds-1",
			queries: [{ name: "Query 1", query: '{app="api"}' }],
			timeRange: TimeRangePreset.Last1Hour,
			refreshInterval: RefreshInterval.M1,
			timestampFormat: LogTimestampFormat.Numeric,
			order: LogOrder.Asc,
		})
		expect(storedDiffInfo()).toBeUndefined()
	})

	it("publishes nothing while no document is active", async ({ expect }) => {
		useEditorStore().updateActiveDocumentId(null)

		await mountBlock()

		expect(useEditorStore().logBlockConfigs).toEqual({})
		expect(useEditorStore().logBlockDiffStatuses).toEqual({})
		expect(useEditorStore().logBlockOldConfigs).toEqual({})
	})

	it("publishes only the diff status of a block shown inside a diff", async ({
		expect,
	}) => {
		await mountBlock({ attrs: { diffStatus: DiffStatus.Added } })

		expect(storedConfig()).toBeUndefined()
		expect(storedDiffInfo()).toEqual({
			diffStatus: DiffStatus.Added,
			oldConfig: null,
		})
	})

	it("publishes the previous config of a modified block", async ({
		expect,
	}) => {
		await mountBlock({
			attrs: {
				diffStatus: DiffStatus.Modified,
				oldNode: {
					attrs: { title: "Before", order: LogOrder.Asc },
				},
			},
		})

		expect(storedDiffInfo()).toEqual({
			diffStatus: DiffStatus.Modified,
			oldConfig: {
				title: "Before",
				dataSourceId: null,
				queries: null,
				timeRange: null,
				refreshInterval: null,
				timestampFormat: LogTimestampFormat.ReadableTime,
				order: LogOrder.Asc,
			},
		})
	})

	it("publishes no previous config for a modified block without its original", async ({
		expect,
	}) => {
		await mountBlock({ attrs: { diffStatus: DiffStatus.Modified } })

		expect(storedDiffInfo()).toEqual({
			diffStatus: DiffStatus.Modified,
			oldConfig: null,
		})
	})

	it("drops the diff info once the block leaves the diff", async ({
		expect,
	}) => {
		const wrapper = await mountBlock({
			attrs: { diffStatus: DiffStatus.Added },
		})

		await wrapper.setProps({ node: makeNode({ uid: UID }) })

		expect(storedDiffInfo()).toBeUndefined()
		expect(storedConfig()).toBeDefined()
	})

	it("counts a modified block's changes", async ({ expect }) => {
		const wrapper = await mountBlock({
			attrs: {
				title: "After",
				diffStatus: DiffStatus.Modified,
				oldNode: { attrs: { uid: UID, title: "Before" } },
			},
		})

		expect(wrapper.text()).toContain(
			t("editor.diff-change-marker.label", { removed: 1, added: 1 }),
		)
	})

	it("publishes nothing while a reviewable diff is shown", async ({
		expect,
	}) => {
		useEditorStore().setReviewableDiffActive(true)

		await mountBlock()

		expect(storedConfig()).toBeUndefined()
	})

	it.for<{ field: string; edit: Partial<LogConfig> }>([
		{ field: "title", edit: { title: "Renamed" } },
		{ field: "data source", edit: { dataSourceId: "ds-2" } },
		{
			field: "queries",
			edit: { queries: [{ name: "Query 1", query: '{job="api"}' }] },
		},
		{ field: "time range", edit: { timeRange: TimeRangePreset.Last24Hours } },
		{
			field: "refresh interval",
			edit: { refreshInterval: RefreshInterval.M15 },
		},
		{
			field: "timestamp format",
			edit: { timestampFormat: LogTimestampFormat.NumericMilli },
		},
		{ field: "order", edit: { order: LogOrder.Asc } },
	])("stores an edited $field on the node", async ({ edit }, { expect }) => {
		const updateAttributes = vi.fn()
		await mountBlock({ updateAttributes: updateAttributes })

		editStoredConfig(edit)

		expect(updateAttributes).toHaveBeenCalledTimes(1)
		expect(updateAttributes).toHaveBeenCalledWith(edit)
	})

	it("stores nothing in read mode", async ({ expect }) => {
		const updateAttributes = vi.fn()
		await mountBlock({ updateAttributes: updateAttributes })
		useEditorMeta().setEditable(false)

		editStoredConfig({ title: "Renamed" })

		expect(updateAttributes).toHaveBeenCalledTimes(0)
	})

	it("stores nothing while a reviewable diff is shown", async ({ expect }) => {
		const updateAttributes = vi.fn()
		await mountBlock({ updateAttributes: updateAttributes })
		useEditorStore().setReviewableDiffActive(true)

		editStoredConfig({ title: "Renamed" })

		expect(updateAttributes).toHaveBeenCalledTimes(0)
	})

	it.for([
		{ name: "no resolvable position", pos: undefined },
		{ name: "a negative position", pos: -1 },
	])("stores nothing for a block with $name", async ({ pos }, { expect }) => {
		const updateAttributes = vi.fn()
		await mountBlock({
			updateAttributes: updateAttributes,
			getPos: () => pos,
		})

		editStoredConfig({ title: "Renamed" })

		expect(updateAttributes).toHaveBeenCalledTimes(0)
	})

	it("stores nothing for a block that has moved away", async ({ expect }) => {
		const updateAttributes = vi.fn()
		await mountBlock({
			updateAttributes: updateAttributes,
			nodeAtPos: { attrs: { uid: "somebody-else" } },
		})

		editStoredConfig({ title: "Renamed" })

		expect(updateAttributes).toHaveBeenCalledTimes(0)
	})

	it("stores nothing for a block that has been deleted", async ({ expect }) => {
		const updateAttributes = vi.fn()
		await mountBlock({ updateAttributes: updateAttributes, nodeAtPos: null })

		editStoredConfig({ title: "Renamed" })

		expect(updateAttributes).toHaveBeenCalledTimes(0)
	})

	it("shows the block's lines under its title with the settings in the header", async ({
		expect,
	}) => {
		mockEndpoint("GET", `/api/data-sources/${DATA_SOURCE_ID}/logs`, () =>
			logsResult([makeLog("deploy finished", "11:57:00Z")]),
		)

		const wrapper = await mountBlock({ attrs: queryingAttrs() })
		await settleLogRequests()

		const title = wrapper.get(".text-2base")

		expect(title.text()).toBe("API errors")
		expect(title.element.parentElement?.textContent).toContain(
			t("editor.logs.config.modal-trigger-button-normal"),
		)
		expect(
			wrapper
				.findAll("button[aria-expanded] .flex-1.overflow-x-auto")
				.map((m) => m.text()),
		).toEqual(["deploy finished · env=prod"])
	})

	it.for([
		{
			name: "leaves out the search while a diff is shown",
			input: true,
			expected: false,
		},
		{ name: "shows the search outside a diff", input: false, expected: true },
	])("$name", async ({ input, expected }, { expect }) => {
		mockEndpoint("GET", `/api/data-sources/${DATA_SOURCE_ID}/logs`, () =>
			logsResult([makeLog("deploy finished", "11:57:00Z")]),
		)
		useEditorStore().setReviewableDiffActive(input)

		const wrapper = await mountBlock({ attrs: queryingAttrs() })
		await settleLogRequests()

		expect(
			wrapper
				.findAll("button")
				.some((b) => b.text().includes(t("editor.logs.view.search-label"))),
		).toBe(expected)
	})

	it("puts the change marker before the volume toggle and the settings after it", async ({
		expect,
	}) => {
		mockEndpoint("GET", `/api/data-sources/${DATA_SOURCE_ID}/logs`, () =>
			logsResult([makeLog("deploy finished", "11:57:00Z")]),
		)
		useEditorStore().setReviewableDiffActive(true)

		const wrapper = await mountBlock({
			attrs: {
				...queryingAttrs(),
				diffStatus: DiffStatus.Modified,
				oldNode: { attrs: { ...queryingAttrs(), uid: UID, title: "Before" } },
			},
		})
		await settleLogRequests()

		const header = wrapper.get(".text-2base").element.parentElement
		const marker = t("editor.diff-change-marker.label", {
			removed: 1,
			added: 1,
		})
		const settings = t("editor.logs.config.modal-trigger-button-readonly")
		expect(
			[...(header?.children ?? [])]
				.map((el) => {
					if (el.textContent.includes(marker)) {
						return "marker"
					}

					if (el.hasAttribute("aria-pressed")) {
						return "volume"
					}

					return el.textContent.includes(settings) ? "settings" : null
				})
				.filter(Boolean),
		).toEqual(["marker", "volume", "settings"])
	})

	it("opens the config modal from the view's empty state", async ({
		expect,
	}) => {
		const setNodeSelection = vi.fn()
		const wrapper = await mountBlock({ setNodeSelection: setNodeSelection })

		await findButtonByText(
			wrapper,
			t("editor.logs.status.data-source-not-selected.normal-action-button"),
		).trigger("click")

		expect(setNodeSelection).toHaveBeenCalledTimes(1)
		expect(setNodeSelection).toHaveBeenCalledWith(5)
		expect(useEditorStore().activeLogBlockConfig).toBe(UID)
	})

	it("opens the config modal from the settings button", async ({ expect }) => {
		const setNodeSelection = vi.fn()
		const wrapper = await mountBlock({ setNodeSelection: setNodeSelection })

		await findButtonByText(
			wrapper,
			t("editor.logs.config.modal-trigger-button-normal"),
		).trigger("click")

		expect(setNodeSelection).toHaveBeenCalledTimes(1)
		expect(setNodeSelection).toHaveBeenCalledWith(5)
		expect(useEditorStore().activeLogBlockConfig).toBe(UID)
	})

	it("labels the settings button as view-only in read mode", async ({
		expect,
	}) => {
		useEditorMeta().setEditable(false)
		const wrapper = await mountBlock()

		await findButtonByText(
			wrapper,
			t("editor.logs.config.modal-trigger-button-readonly"),
		).trigger("click")

		expect(useEditorStore().activeLogBlockConfig).toBe(UID)
	})

	it("opens the config modal for a block with no resolvable position", async ({
		expect,
	}) => {
		const setNodeSelection = vi.fn()
		const wrapper = await mountBlock({
			getPos: () => undefined,
			setNodeSelection: setNodeSelection,
		})

		await findButtonByText(
			wrapper,
			t("editor.logs.config.modal-trigger-button-normal"),
		).trigger("click")

		expect(setNodeSelection).toHaveBeenCalledTimes(0)
		expect(useEditorStore().activeLogBlockConfig).toBe(UID)
	})

	it("drops the old block's entries when its uid changes", async ({
		expect,
	}) => {
		const nextUid = "log-block-2"
		const wrapper = await mountBlock({
			attrs: { diffStatus: DiffStatus.Added },
		})

		await wrapper.setProps({ node: makeNode({ uid: nextUid }) })

		expect(storedConfig()).toBeUndefined()
		expect(storedDiffInfo()).toBeUndefined()
		expect(storedConfig(nextUid)).toBeDefined()
	})

	it("names the collaborator editing the block", async ({ expect }) => {
		const wrapper = await mountBlock({
			editing: { uid: UID, name: "Ada", color: "#ff0000" },
		})

		expect(wrapper.text()).toContain("Ada")
		expect(wrapperClasses(wrapper)).toContain("rounded-tl-none")
		expect(
			wrapper.get("[data-node-view-wrapper]").attributes("style"),
		).toContain("border-color: #ff0000")
	})

	it("names nobody when the collaborator is editing another block", async ({
		expect,
	}) => {
		const wrapper = await mountBlock({
			editing: { uid: "elsewhere", name: "Ada", color: "#ff0000" },
		})

		expect(wrapper.text()).not.toContain("Ada")
		expect(wrapperClasses(wrapper)).not.toContain("rounded-tl-none")
	})
})

// a block that reads its lines from a data source
function queryingAttrs(): Record<string, unknown> {
	return {
		title: "API errors",
		dataSourceId: DATA_SOURCE_ID,
		queries: [{ name: "Query 1", query: '{app="api"}' }],
		timeRange: TimeRangePreset.Last1Hour,
	}
}

// the editor a log block reads: the collaboration composable takes
// anything that is not a real Editor instance as the awareness provider,
// so the stand-in carries the awareness directly
function makeLogEditor(
	options: {
		nodeAtPos?: { attrs: Record<string, unknown> } | null
		editing?: { uid: string; name: string; color: string } | null
		setNodeSelection?: (pos: number) => void
	} = {},
) {
	const states = new Map<number, Record<string, unknown>>([[1, {}]])

	if (options.editing) {
		states.set(2, {
			editingNodeUid: { uid: options.editing.uid },
			user: { name: options.editing.name, color: options.editing.color },
		})
	}

	return makeEditor({
		commands: {
			setNodeSelection: options.setNodeSelection ?? (() => undefined),
		},
		state: {
			doc: {
				nodeAt: () =>
					options.nodeAtPos === undefined ? null : options.nodeAtPos,
			},
		},
		awareness: {
			clientID: 1,
			getStates: () => states,
			on: vi.fn(),
			off: vi.fn(),
		},
	})
}

function mountBlock(
	options: {
		attrs?: Record<string, unknown>
		updateAttributes?: (attrs: Record<string, unknown>) => void
		getPos?: () => number | undefined
		nodeAtPos?: { attrs: Record<string, unknown> } | null
		editing?: { uid: string; name: string; color: string } | null
		setNodeSelection?: (pos: number) => void
	} = {},
) {
	return mountNodeView(MainBlock, {
		node: makeNode({ uid: UID, ...options.attrs }),
		editor: makeLogEditor({
			nodeAtPos:
				"nodeAtPos" in options ? options.nodeAtPos : { attrs: { uid: UID } },
			editing: options.editing,
			setNodeSelection: options.setNodeSelection,
		}).editor,
		getPos: options.getPos ?? (() => 5),
		updateAttributes: options.updateAttributes ?? (() => undefined),
	})
}

// the config object the block published into the store, which is what the
// config modal reads and writes through
function storedConfig(uid = UID): LogConfig | undefined {
	return useEditorStore().logBlockConfigs[DOCUMENT_ID]?.[BRANCH_ID]?.[uid]
}

// the diff status and old config the block published, or undefined when it
// published neither
function storedDiffInfo(uid = UID) {
	const store = useEditorStore()
	const diffStatus = store.logBlockDiffStatuses[DOCUMENT_ID]?.[BRANCH_ID]?.[uid]
	const oldConfig = store.logBlockOldConfigs[DOCUMENT_ID]?.[BRANCH_ID]?.[uid]

	if (diffStatus === undefined && oldConfig === undefined) {
		return undefined
	}

	return { diffStatus, oldConfig }
}

// writes through the published config, as the config modal does
function editStoredConfig(edit: Partial<LogConfig>) {
	const config = storedConfig()

	if (!config) {
		throw new Error("the block published no config")
	}

	Object.assign(config, edit)
}

function wrapperClasses(wrapper: VueWrapper): string[] {
	return wrapper.get("[data-node-view-wrapper]").classes()
}
