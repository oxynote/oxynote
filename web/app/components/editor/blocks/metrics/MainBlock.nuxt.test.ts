import type { VueWrapper } from "@vue/test-utils"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import MainBlock from "./MainBlock.vue"
import { stubChartColorContext } from "./test-helpers"
import { MetricBlockWidth, TimeRangePreset, type MetricConfig } from "./utils"
import {
	makeEditor,
	makeNode,
	mountNodeView,
} from "../../test-helpers/node-view"
import { DiffStatus } from "~/components/editor/diff/position-map"
import {
	clearQueryCache,
	disposeMockEndpoints,
	makeXid,
} from "~/composables/api/test-helpers"

const DOCUMENT_ID = makeXid("doc")
const BRANCH_ID = makeXid("branch")

let uidCounter = 0

// the store keys every block's config by uid and keeps it for the whole
// document, so each test works on a uid of its own
function nextUid(): string {
	uidCounter++

	return `metric-block-${uidCounter}`
}

// the editor a metric block reads: the collaboration composable takes
// anything that is not a real Editor instance as the awareness provider,
// so the stand-in carries the awareness directly
function makeMetricEditor(
	options: {
		nodeAtPos?: { attrs: Record<string, unknown> } | null
		editing?: { uid: string; name: string; color: string } | null
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
		uid?: string
		attrs?: Record<string, unknown>
		updateAttributes?: (attrs: Record<string, unknown>) => void
		getPos?: () => number | undefined
		nodeAtPos?: { attrs: Record<string, unknown> } | null
		editing?: { uid: string; name: string; color: string } | null
	} = {},
) {
	const uid = options.uid ?? nextUid()

	return mountNodeView(MainBlock, {
		node: makeNode({ uid: uid, ...options.attrs }),
		editor: makeMetricEditor({
			nodeAtPos:
				"nodeAtPos" in options ? options.nodeAtPos : { attrs: { uid: uid } },
			editing: options.editing,
		}).editor,
		getPos: options.getPos ?? (() => 5),
		updateAttributes: options.updateAttributes ?? (() => undefined),
	})
}

// the config object the block published into the store, which is what the
// config modal reads and writes through
function storedConfig(uid: string): MetricConfig | undefined {
	return useEditorStore().metricBlockConfigs[DOCUMENT_ID]?.[BRANCH_ID]?.[uid]
}

function wrapperClasses(wrapper: VueWrapper): string[] {
	return wrapper.get("[data-node-view-wrapper]").classes()
}

// the editor store and the editable flag are shared app-wide, so these
// tests cannot interleave
describe("<MetricMainBlock>", { concurrent: false }, () => {
	beforeEach(() => {
		stubChartColorContext()
		clearQueryCache()
		useEditorMeta().setEditable(true)
		useEditorStore().setReviewableDiffActive(false)
		useEditorStore().updateActiveDocumentId(DOCUMENT_ID)
		useEditorStore().updateActiveBranchId(BRANCH_ID)
	})

	afterEach(disposeMockEndpoints)

	it("identifies the wrapper by the node's uid", async ({ expect }) => {
		const uid = nextUid()

		const wrapper = await mountBlock({ uid: uid })

		const root = wrapper.get("[data-node-view-wrapper]")

		expect(root.attributes("id")).toBe(uid)
		expect(root.attributes("data-uid")).toBe(uid)
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

	it.for([
		MetricBlockWidth.Compact,
		MetricBlockWidth.Standard,
		MetricBlockWidth.Wide,
	])("gives a %s block its own grid span", async (width, { expect }) => {
		const wrapper = await mountBlock({ attrs: { width: width } })

		expect(wrapperClasses(wrapper)).toContain(`metric-block-${width}`)
	})

	it("spans a block with no stored width like a standard one", async ({
		expect,
	}) => {
		const wrapper = await mountBlock()

		expect(wrapperClasses(wrapper)).toContain("metric-block-standard")
	})

	it("publishes the block's config for the config modal to read", async ({
		expect,
	}) => {
		const uid = nextUid()

		await mountBlock({
			uid: uid,
			attrs: { title: "Requests", timeRange: TimeRangePreset.Last1Hour },
		})

		expect(storedConfig(uid)).toEqual(
			expect.objectContaining({
				title: "Requests",
				timeRange: TimeRangePreset.Last1Hour,
			}),
		)
	})

	it("publishes a legacy block's config over its schema defaults", async ({
		expect,
	}) => {
		const uid = nextUid()

		await mountBlock({
			uid: uid,
			attrs: {
				title: "",
				config: {
					title: "Legacy",
					type: GenericQueryChartType.Bar,
					unit: { type: null, custom: "req/s" },
					axisBounds: { min: 0, max: 10 },
				},
			},
		})

		expect(storedConfig(uid)).toEqual(
			expect.objectContaining({
				title: "Legacy",
				visualizationType: GenericQueryChartType.Bar,
			}),
		)
		expect(storedConfig(uid)?.unit).toEqual({ type: null, custom: "req/s" })
		expect(storedConfig(uid)?.axisBounds).toEqual({ min: 0, max: 10 })
	})

	it("publishes nothing for a block shown inside a diff", async ({
		expect,
	}) => {
		const uid = nextUid()

		await mountBlock({ uid: uid, attrs: { diffStatus: DiffStatus.Added } })

		expect(storedConfig(uid)).toBeUndefined()
		expect(
			useEditorStore().metricBlockDiffStatuses[DOCUMENT_ID]?.[BRANCH_ID]?.[uid],
		).toBe(DiffStatus.Added)
	})

	it("publishes the previous config of a modified block", async ({
		expect,
	}) => {
		const uid = nextUid()

		await mountBlock({
			uid: uid,
			attrs: {
				diffStatus: DiffStatus.Modified,
				oldNode: { attrs: { title: "Before" } },
			},
		})

		expect(
			useEditorStore().metricBlockOldConfigs[DOCUMENT_ID]?.[BRANCH_ID]?.[uid],
		).toEqual(expect.objectContaining({ title: "Before" }))
	})

	it("publishes no previous config for an added block", async ({ expect }) => {
		const uid = nextUid()

		await mountBlock({ uid: uid, attrs: { diffStatus: DiffStatus.Added } })

		expect(
			useEditorStore().metricBlockOldConfigs[DOCUMENT_ID]?.[BRANCH_ID]?.[uid],
		).toBeNull()
	})

	it("publishes nothing while a reviewable diff is shown", async ({
		expect,
	}) => {
		useEditorStore().setReviewableDiffActive(true)
		const uid = nextUid()

		await mountBlock({ uid: uid })

		expect(storedConfig(uid)).toBeUndefined()
	})

	it("stores an edited field on the node", async ({ expect }) => {
		const updateAttributes = vi.fn()
		const uid = nextUid()
		await mountBlock({ uid: uid, updateAttributes: updateAttributes })

		const config = storedConfig(uid)
		if (config) {
			config.title = "Renamed"
		}

		expect(updateAttributes).toHaveBeenCalledTimes(1)
		expect(updateAttributes).toHaveBeenCalledWith({ title: "Renamed" })
	})

	it("flattens a legacy config on the first edit", async ({ expect }) => {
		const updateAttributes = vi.fn()
		const uid = nextUid()
		await mountBlock({
			uid: uid,
			updateAttributes: updateAttributes,
			attrs: {
				config: {
					title: "Legacy",
					dataSourceId: "ds-1",
					type: GenericQueryChartType.Gauge,
					unit: { type: null, custom: "req/s" },
				},
			},
		})

		const config = storedConfig(uid)
		if (config) {
			config.title = "Renamed"
		}

		expect(updateAttributes).toHaveBeenCalledTimes(1)
		expect(updateAttributes).toHaveBeenCalledWith(
			expect.objectContaining({
				title: "Renamed",
				dataSourceId: "ds-1",
				visualizationType: GenericQueryChartType.Gauge,
				unitCustom: "req/s",
				config: null,
			}),
		)
	})

	it("stores nothing while a reviewable diff is shown", async ({ expect }) => {
		const updateAttributes = vi.fn()
		const uid = nextUid()
		await mountBlock({ uid: uid, updateAttributes: updateAttributes })
		const config = storedConfig(uid)
		useEditorStore().setReviewableDiffActive(true)

		if (config) {
			config.title = "Renamed"
		}

		expect(updateAttributes).toHaveBeenCalledTimes(0)
	})

	it("stores nothing for a block with no resolvable position", async ({
		expect,
	}) => {
		const updateAttributes = vi.fn()
		const uid = nextUid()
		await mountBlock({
			uid: uid,
			updateAttributes: updateAttributes,
			getPos: () => undefined,
		})

		const config = storedConfig(uid)
		if (config) {
			config.title = "Renamed"
		}

		expect(updateAttributes).toHaveBeenCalledTimes(0)
	})

	it("stores nothing for a block that has moved away", async ({ expect }) => {
		const updateAttributes = vi.fn()
		const uid = nextUid()
		await mountBlock({
			uid: uid,
			updateAttributes: updateAttributes,
			nodeAtPos: { attrs: { uid: "somebody-else" } },
		})

		const config = storedConfig(uid)
		if (config) {
			config.title = "Renamed"
		}

		expect(updateAttributes).toHaveBeenCalledTimes(0)
	})

	it("stores nothing for a block that has been deleted", async ({ expect }) => {
		const updateAttributes = vi.fn()
		const uid = nextUid()
		await mountBlock({
			uid: uid,
			updateAttributes: updateAttributes,
			nodeAtPos: null,
		})

		const config = storedConfig(uid)
		if (config) {
			config.title = "Renamed"
		}

		expect(updateAttributes).toHaveBeenCalledTimes(0)
	})

	it("drops the old block's entries when its uid changes", async ({
		expect,
	}) => {
		const uid = nextUid()
		const nextUidValue = nextUid()
		const wrapper = await mountBlock({ uid: uid })

		await wrapper.setProps({ node: makeNode({ uid: nextUidValue }) })

		expect(storedConfig(uid)).toBeUndefined()
		expect(storedConfig(nextUidValue)).toBeDefined()
	})

	it("names the collaborator editing the block", async ({ expect }) => {
		const uid = nextUid()

		const wrapper = await mountBlock({
			uid: uid,
			editing: { uid: uid, name: "Ada", color: "#ff0000" },
		})

		expect(wrapper.text()).toContain("Ada")
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
	})
})
