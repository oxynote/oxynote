import type { VueWrapper } from "@vue/test-utils"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import { toast } from "vue-sonner"
import type { VariableTreeResponse } from "~/utils"
import {
	clearQueryCache,
	disposeMockEndpoints,
	mockEndpoint,
} from "~/composables/api/test-helpers"
import {
	emitFromNth,
	findButtonByText,
	mountUnderTooltipProvider,
	raisedToasts,
	refuseForRole,
	renderedIconNames,
	settleMutations,
	t,
} from "../test-helpers"
import DraggableRow from "./DraggableRow.vue"
import EditList from "./EditList.vue"
import type { DraggableItem } from "./variables"
import {
	GROUP_DEPLOYS,
	GROUP_ENV,
	GROUP_METRICS,
	VAR_ENV,
	VAR_RANGE,
	VAR_REGION,
	builtinTree,
	openAllGroups,
	fullTree,
	mockTree,
} from "./test-helpers"

vi.mock("vue-sonner", () => ({
	toast: { custom: vi.fn(), dismiss: vi.fn() },
}))

// the collapse state, the drag store, the query cache and the mocked toast
// are all shared by every mount in the file
describe("<EditList>", { concurrent: false }, () => {
	beforeEach(() => {
		clearQueryCache()
		openAllGroups()
		vi.mocked(toast.custom).mockClear()
	})
	afterEach(disposeMockEndpoints)

	describe("the built-in group", () => {
		it("shows a lock and the built-in tag instead of a handle and actions", async ({
			expect,
		}) => {
			const wrapper = await mountList(fullTree())

			const header = groupHeader(wrapper, "Metrics")

			expect(renderedIconNames(header)).toContain("mingcute:lock-line")
			expect(renderedIconNames(header)).not.toContain("mingcute:dots-line")
			expect(header.text()).toContain(t("variable.builtin.tag"))
			expect(actionButtons(header)).toHaveLength(0)
		})

		it("offers no add variable row", async ({ expect }) => {
			const wrapper = await mountList(builtinTree())

			expect(wrapper.text()).not.toContain(t("variable.actions.add-variable"))
		})

		it("locks its variables without a handle or a delete action", async ({
			expect,
		}) => {
			const wrapper = await mountList(fullTree())

			const row = rowOf(wrapper, VAR_RANGE)

			expect(renderedIconNames(row)).toContain("mingcute:lock-line")
			expect(renderedIconNames(row)).not.toContain("mingcute:dots-line")
			expect(row.text()).toContain("$metric_range")
			expect(row.findAll("button").map((b) => b.text())).toEqual([
				t("variable.actions.open-variable", { name: "Metric time range" }),
			])
		})

		it("keeps itself and its variables out of drag and drop", async ({
			expect,
		}) => {
			const wrapper = await mountList(fullTree())

			const items = draggedItems(wrapper)

			expect(items[GROUP_METRICS]).toMatchObject({ draggable: false })
			expect(items[VAR_RANGE]).toMatchObject({ draggable: false })
			expect(items[GROUP_METRICS]?.dragGroup).not.toBe(
				items[GROUP_DEPLOYS]?.dragGroup,
			)
			expect(items[VAR_RANGE]?.dragGroup).not.toBe(items[VAR_ENV]?.dragGroup)
		})
	})

	describe("a group of their own", () => {
		it("carries a handle and a rename action", async ({ expect }) => {
			const wrapper = await mountList(fullTree())

			const header = groupHeader(wrapper, "Deploys")

			expect(renderedIconNames(header)).toContain("mingcute:dots-line")
			expect(actionButtons(header).map((b) => b.text())).toEqual([
				t("variable.actions.rename-group", { name: "Deploys" }),
			])
		})

		it("offers only its add variable row while empty", async ({ expect }) => {
			const wrapper = await mountList(fullTree())

			const group = groupOf(wrapper, "Deploys")

			expect(group.text()).not.toContain(t("variable.empty.group"))
			expect(group.text()).toContain(t("variable.actions.add-variable"))
		})

		it("shows each variable with its key, a handle and an open action", async ({
			expect,
		}) => {
			const wrapper = await mountList(fullTree())

			const row = rowOf(wrapper, VAR_ENV)

			expect(renderedIconNames(row)).toContain("mingcute:dots-line")
			expect(row.text()).toContain("$env")
			expect(row.findAll("button").map((b) => b.text())).toEqual([
				t("variable.actions.open-variable", { name: "Environment" }),
			])
		})
	})

	describe("when only the built-in variables exist", () => {
		it("explains that no groups of their own exist yet", async ({ expect }) => {
			const wrapper = await mountList(builtinTree())

			expect(wrapper.text()).toContain(t("variable.empty.no-groups-title"))
			expect(wrapper.text()).toContain(
				t("variable.empty.no-groups-description"),
			)
			expect(wrapper.text()).toContain(t("variable.actions.add-group"))
		})
	})

	it("leaves out the no groups note once a group exists", async ({
		expect,
	}) => {
		const wrapper = await mountList(fullTree())

		expect(wrapper.text()).not.toContain(t("variable.empty.no-groups-title"))
	})

	it.for([
		{
			name: "asks to open a variable from its chevron",
			input: () => t("variable.actions.open-variable", { name: "Region" }),
			expected: { event: "open-variable", payload: [VAR_REGION] },
		},
		{
			name: "asks to rename a group from its pencil",
			input: () => t("variable.actions.rename-group", { name: "Environment" }),
			expected: { event: "edit-group", payload: [GROUP_ENV] },
		},
		{
			name: "asks for a new variable from a group's add variable row",
			input: () => t("variable.actions.add-variable"),
			expected: { event: "new-variable", payload: [GROUP_ENV] },
		},
		{
			name: "asks for a new group from the add group row",
			input: () => t("variable.actions.add-group"),
			expected: { event: "new-group", payload: [] },
		},
	])("$name", async ({ input, expected }, { expect }) => {
		const wrapper = await mountList(fullTree())

		await findButtonByText(wrapper, input()).trigger("click")

		expect(wrapper.findComponent(EditList).emitted(expected.event)).toEqual([
			expected.payload,
		])
	})

	describe("drag and drop", () => {
		it("moves a group before the group it is dropped on", async ({
			expect,
		}) => {
			mockTree(fullTree())
			const calls = mockEndpoint(
				"PUT",
				"/api/variables/groups/tree",
				() => null,
			)
			const wrapper = await mountList(fullTree())

			dropRow(wrapper, GROUP_DEPLOYS, {
				parentId: null,
				insertBeforeId: GROUP_ENV,
			})
			await settleMutations()

			expect
				.soft(calls.map((call) => call.body))
				.toEqual([{ id: GROUP_DEPLOYS, sortIndex: 0 }])
		})

		it("moves a variable to the end of the group whose add row it is dropped on", async ({
			expect,
		}) => {
			mockTree(fullTree())
			const calls = mockEndpoint("PUT", "/api/variables/tree", () => null)
			const wrapper = await mountList(fullTree())

			dropRow(wrapper, VAR_ENV, {
				parentId: GROUP_DEPLOYS,
				insertBeforeId: null,
			})
			await settleMutations()

			expect
				.soft(calls.map((call) => call.body))
				.toEqual([{ id: VAR_ENV, groupId: GROUP_DEPLOYS, sortIndex: 0 }])
		})

		it("sends nothing when a drop leaves the order as it was", async ({
			expect,
		}) => {
			const groupMoves = mockEndpoint(
				"PUT",
				"/api/variables/groups/tree",
				() => null,
			)
			const variableMoves = mockEndpoint(
				"PUT",
				"/api/variables/tree",
				() => null,
			)
			const wrapper = await mountList(fullTree())

			dropRow(wrapper, GROUP_ENV, {
				parentId: null,
				insertBeforeId: GROUP_DEPLOYS,
			})
			dropRow(wrapper, VAR_ENV, {
				parentId: GROUP_ENV,
				insertBeforeId: VAR_REGION,
			})
			dropRow(wrapper, VAR_ENV, { parentId: null, insertBeforeId: null })
			await settleMutations()

			expect.soft(groupMoves).toHaveLength(0)
			expect.soft(variableMoves).toHaveLength(0)
		})

		it("raises a toast when a move fails", async ({ expect }) => {
			mockTree(fullTree())
			const calls = mockEndpoint("PUT", "/api/variables/tree", () => {
				throw createError({ statusCode: 500 })
			})
			const wrapper = await mountList(fullTree())

			dropRow(wrapper, VAR_ENV, {
				parentId: GROUP_DEPLOYS,
				insertBeforeId: null,
			})
			await settleMutations()

			expect.soft(calls).toHaveLength(1)
			expect.soft(raisedToasts()).toEqual([
				expect.objectContaining({
					type: "error",
					title: t("variable.errors.move-failed"),
				}),
			])
		})

		it("shows only the access denied toast for a refused variable move", async ({
			expect,
		}) => {
			mockTree(fullTree())
			mockEndpoint("PUT", "/api/variables/tree", (_call, event) =>
				refuseForRole(event),
			)
			const wrapper = await mountList(fullTree())

			dropRow(wrapper, VAR_ENV, {
				parentId: GROUP_DEPLOYS,
				insertBeforeId: null,
			})
			await settleMutations()

			expect(raisedToasts()).toMatchObject([
				{ type: "error", title: t("general.errors.role-not-allowed") },
			])
		})

		it("shows only the access denied toast for a refused group move", async ({
			expect,
		}) => {
			mockTree(fullTree())
			mockEndpoint("PUT", "/api/variables/groups/tree", (_call, event) =>
				refuseForRole(event),
			)
			const wrapper = await mountList(fullTree())

			dropRow(wrapper, GROUP_DEPLOYS, {
				parentId: null,
				insertBeforeId: GROUP_ENV,
			})
			await settleMutations()

			expect(raisedToasts()).toMatchObject([
				{ type: "error", title: t("general.errors.role-not-allowed") },
			])
		})
	})
})

function mountList(tree: VariableTreeResponse) {
	return mountUnderTooltipProvider(EditList, { props: { tree: tree } })
}

function groupHeader(wrapper: VueWrapper, name: string) {
	const header = components(wrapper, "VariablesGroupHeader").find(
		(h) => h.get("[data-slot='collapsible-trigger']").text() === name,
	)
	if (!header) {
		throw new Error(`no group header named "${name}"`)
	}

	return header
}

// the header's own collapse trigger is a button too
function actionButtons(header: VueWrapper) {
	return header.findAll("button:not([data-slot='collapsible-trigger'])")
}

function groupOf(wrapper: VueWrapper, name: string) {
	const group = wrapper
		.findAll("[data-slot='collapsible']")
		.find((g) => g.get("[data-slot='collapsible-trigger']").text() === name)
	if (!group) {
		throw new Error(`no group named "${name}"`)
	}

	return group
}

// a draggable row is identified by the item it hands to drag and drop. A
// group's add variable row carries the group's id too, but it is the only
// row that takes drops into itself.
function rowIndex(wrapper: VueWrapper, id: string) {
	const index = components(wrapper, "VariablesDraggableRow").findIndex(
		(row) => {
			const item = (row.props() as { item: DraggableItem }).item

			return item.id === id && !item.acceptsChildren
		},
	)
	if (index === -1) {
		throw new Error(`no draggable row for "${id}"`)
	}

	return index
}

function rowOf(wrapper: VueWrapper, id: string) {
	const row = components(wrapper, "VariablesDraggableRow")[
		rowIndex(wrapper, id)
	]
	if (!row) {
		throw new Error(`no row for "${id}"`)
	}

	return row
}

function draggedItems(wrapper: VueWrapper) {
	return Object.fromEntries(
		components(wrapper, "VariablesDraggableRow")
			.map((row) => (row.props() as { item: DraggableItem }).item)
			.filter((item) => !item.acceptsChildren)
			.map((item) => [item.id, item]),
	)
}

// a drop is confirmed on the dragged row, which hands on where it landed
function dropRow(
	wrapper: VueWrapper,
	id: string,
	target: { parentId: string | null; insertBeforeId: string | null },
) {
	emitFromNth(wrapper, DraggableRow, rowIndex(wrapper, id), "move", target)
}

// eslint's ts program types every found component as any; the wrappers
// are plain ones for everything these tests do with them
function components(wrapper: VueWrapper, name: string) {
	return wrapper.findAllComponents({ name: name }) as VueWrapper[]
}
