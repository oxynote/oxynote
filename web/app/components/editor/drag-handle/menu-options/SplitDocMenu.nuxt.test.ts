import { afterEach, describe, it } from "vitest"
import SplitDocMenu from "./SplitDocMenu.vue"
import { hoveredBlock, mountMenuOptions } from "./test-helpers"
import {
	commandArgs,
	commandNames,
	makeEditor,
} from "../../test-helpers/node-view"
import { SUPPRESS_SCROLL_TO_SELECTION_META } from "../../scroll-control"
import { SPLIT_DOCUMENTATION_LEFT_SIDE_NAME } from "../../blocks/node-names"
import { clearTeleportedOverlays, menuItem, t } from "~/components/test-helpers"

const ADD_PARAMETERS = "editor.drag-handle.options.split-doc.add-parameter-list"
const INVERT = "editor.drag-handle.options.split-doc.invert-sides"

// a split documentation block holds a left and a right side; the left one
// is where a parameter list goes
function splitDocBlock(nodePos: number, leftSideFirst = true) {
	return hoveredBlock(nodePos, {
		children: leftSideFirst
			? [
					{ typeName: SPLIT_DOCUMENTATION_LEFT_SIDE_NAME, nodeSize: 4 },
					{ typeName: "splitDocumentationRightSide", nodeSize: 6 },
				]
			: [
					{ typeName: "splitDocumentationRightSide", nodeSize: 6 },
					{ typeName: SPLIT_DOCUMENTATION_LEFT_SIDE_NAME, nodeSize: 4 },
				],
	})
}

// the menu body is teleported into a shared <body>, so these tests
// cannot interleave
describe("<SplitDocMenu>", { concurrent: false }, () => {
	afterEach(clearTeleportedOverlays)

	it("offers to add parameters and to swap the sides", async ({ expect }) => {
		const { editor } = makeEditor()

		await mountMenuOptions(SplitDocMenu, {
			editor: editor,
			hovered: splitDocBlock(2),
		})

		expect(menuItem(t(ADD_PARAMETERS))).toBeDefined()
		expect(menuItem(t(INVERT))).toBeDefined()
	})

	it("appends a parameter list to the left side", async ({ expect }) => {
		const { editor, commands } = makeEditor()
		await mountMenuOptions(SplitDocMenu, {
			editor: editor,
			hovered: splitDocBlock(2),
		})

		menuItem(t(ADD_PARAMETERS)).click()

		expect(commandNames(commands)).toEqual([
			"focus",
			"appendParameterListOnLeftSide",
			"run",
		])
		expect(commandArgs(commands, "appendParameterListOnLeftSide")).toEqual([3])
	})

	it("finds the left side wherever it sits in the block", async ({
		expect,
	}) => {
		const { editor, commands } = makeEditor()
		await mountMenuOptions(SplitDocMenu, {
			editor: editor,
			hovered: splitDocBlock(2, false),
		})

		menuItem(t(ADD_PARAMETERS)).click()

		expect(commandArgs(commands, "appendParameterListOnLeftSide")).toEqual([9])
	})

	it("adds nothing to a block with no left side", async ({ expect }) => {
		const { editor, commands } = makeEditor()
		await mountMenuOptions(SplitDocMenu, {
			editor: editor,
			hovered: hoveredBlock(2),
		})

		menuItem(t(ADD_PARAMETERS)).click()

		expect(commands).toEqual([])
	})

	it("swaps the sides without scrolling the selection into view", async ({
		expect,
	}) => {
		const { editor, commands } = makeEditor()
		await mountMenuOptions(SplitDocMenu, {
			editor: editor,
			hovered: splitDocBlock(2),
		})

		menuItem(t(INVERT)).click()

		expect(commandNames(commands)).toEqual([
			"setMeta",
			"focus",
			"invertSplitDocumentation",
			"run",
		])
		expect(commandArgs(commands, "setMeta")).toEqual([
			SUPPRESS_SCROLL_TO_SELECTION_META,
			true,
		])
		expect(commandArgs(commands, "invertSplitDocumentation")).toEqual([2])
	})

	it("does nothing while no block is hovered", async ({ expect }) => {
		const { editor, commands } = makeEditor()
		await mountMenuOptions(SplitDocMenu, {
			editor: editor,
			hovered: null,
		})

		menuItem(t(ADD_PARAMETERS)).click()
		menuItem(t(INVERT)).click()

		expect(commands).toEqual([])
	})
})
