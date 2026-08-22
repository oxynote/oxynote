import { afterEach, describe, it } from "vitest"
import DefaultOptions from "./DefaultOptions.vue"
import { hoveredBlock, mountMenuOptions } from "./test-helpers"
import {
	commandArgs,
	commandNames,
	makeEditor,
} from "../../test-helpers/node-view"
import { METRIC_BLOCK_NAME, METRIC_GRID_NAME } from "../../blocks/node-names"
import { clearTeleportedOverlays, menuItem, t } from "~/components/test-helpers"

const ADD_ABOVE = "editor.drag-handle.options.default.add-above-block"
const ADD_BELOW = "editor.drag-handle.options.default.add-below-block"

// the editor a neighbour insert reads the document through
function makeDocEditor(
	node: { typeName: string; nodeSize: number } | null,
	gridBounds: { before: number; after: number } | null = null,
) {
	return makeEditor({
		state: {
			doc: {
				nodeAt: () =>
					node
						? { type: { name: node.typeName }, nodeSize: node.nodeSize }
						: null,
				resolve: () => ({
					depth: gridBounds ? 1 : 0,
					node: () => ({ type: { name: METRIC_GRID_NAME } }),
					before: () => gridBounds?.before ?? 0,
					after: () => gridBounds?.after ?? 0,
				}),
			},
		},
	})
}

// the menu body is teleported into a shared <body>, so these tests
// cannot interleave
describe("<DefaultOptions>", { concurrent: false }, () => {
	afterEach(clearTeleportedOverlays)

	it("offers to add a block on either side", async ({ expect }) => {
		const { editor } = makeDocEditor({ typeName: "paragraph", nodeSize: 6 })

		await mountMenuOptions(DefaultOptions, {
			editor: editor,
			hovered: hoveredBlock(10),
		})

		expect(menuItem(t(ADD_ABOVE))).toBeDefined()
		expect(menuItem(t(ADD_BELOW))).toBeDefined()
	})

	it("adds an empty slash paragraph above the hovered block", async ({
		expect,
	}) => {
		const { editor, commands } = makeDocEditor({
			typeName: "paragraph",
			nodeSize: 6,
		})
		await mountMenuOptions(DefaultOptions, {
			editor: editor,
			hovered: hoveredBlock(10),
		})

		menuItem(t(ADD_ABOVE)).click()

		expect(commandNames(commands)).toEqual([
			"focus",
			"insertContentAt",
			"setTextSelection",
			"run",
		])
		expect(commandArgs(commands, "insertContentAt")).toEqual([
			10,
			{ type: "paragraph", content: [{ type: "text", text: "/" }] },
		])
		expect(commandArgs(commands, "setTextSelection")).toEqual([12])
	})

	it("adds the new block after the hovered one when asked", async ({
		expect,
	}) => {
		const { editor, commands } = makeDocEditor({
			typeName: "paragraph",
			nodeSize: 6,
		})
		await mountMenuOptions(DefaultOptions, {
			editor: editor,
			hovered: hoveredBlock(10),
		})

		menuItem(t(ADD_BELOW)).click()

		expect(commandArgs(commands, "insertContentAt")?.[0]).toBe(16)
	})

	it("adds a sibling list item next to a list item", async ({ expect }) => {
		const { editor, commands } = makeDocEditor({
			typeName: "listItem",
			nodeSize: 4,
		})
		await mountMenuOptions(DefaultOptions, {
			editor: editor,
			hovered: hoveredBlock(10),
		})

		menuItem(t(ADD_ABOVE)).click()

		expect(commandArgs(commands, "insertContentAt")).toEqual([
			10,
			{ type: "listItem", content: [{ type: "paragraph", content: [] }] },
		])
		expect(commandArgs(commands, "setTextSelection")).toEqual([11])
	})

	it("adds an unchecked sibling next to a task item", async ({ expect }) => {
		const { editor, commands } = makeDocEditor({
			typeName: "taskItem",
			nodeSize: 4,
		})
		await mountMenuOptions(DefaultOptions, {
			editor: editor,
			hovered: hoveredBlock(10),
		})

		menuItem(t(ADD_ABOVE)).click()

		expect(commandArgs(commands, "insertContentAt")).toEqual([
			10,
			{
				type: "taskItem",
				attrs: { checked: false },
				content: [{ type: "paragraph", content: [] }],
			},
		])
	})

	it("adds the block outside the grid a metric block sits in", async ({
		expect,
	}) => {
		const { editor, commands } = makeDocEditor(
			{ typeName: METRIC_BLOCK_NAME, nodeSize: 2 },
			{ before: 4, after: 20 },
		)
		await mountMenuOptions(DefaultOptions, {
			editor: editor,
			hovered: hoveredBlock(10),
		})

		menuItem(t(ADD_BELOW)).click()

		expect(commandArgs(commands, "insertContentAt")?.[0]).toBe(20)
	})

	it("adds nothing when the hovered block is no longer there", async ({
		expect,
	}) => {
		const { editor, commands } = makeDocEditor(null)
		await mountMenuOptions(DefaultOptions, {
			editor: editor,
			hovered: hoveredBlock(10),
		})

		menuItem(t(ADD_ABOVE)).click()

		expect(commands).toEqual([])
	})

	it("does nothing while no block is hovered", async ({ expect }) => {
		const { editor, commands } = makeDocEditor({
			typeName: "paragraph",
			nodeSize: 6,
		})
		await mountMenuOptions(DefaultOptions, {
			editor: editor,
			hovered: null,
		})

		menuItem(t(ADD_ABOVE)).click()

		expect(commands).toEqual([])
	})
})
