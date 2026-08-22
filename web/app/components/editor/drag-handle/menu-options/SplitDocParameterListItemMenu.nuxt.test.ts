import { afterEach, describe, it } from "vitest"
import SplitDocParameterListItemMenu from "./SplitDocParameterListItemMenu.vue"
import { hoveredBlock, mountMenuOptions } from "./test-helpers"
import {
	commandArgs,
	commandNames,
	makeEditor,
} from "../../test-helpers/node-view"
import { clearTeleportedOverlays, menuItem, t } from "~/components/test-helpers"

const ADD_ABOVE =
	"editor.drag-handle.options.split-doc-parameter-list-item.add-above-block"
const ADD_BELOW =
	"editor.drag-handle.options.split-doc-parameter-list-item.add-below-block"

// the menu body is teleported into a shared <body>, so these tests
// cannot interleave
describe("<SplitDocParameterListItemMenu>", { concurrent: false }, () => {
	afterEach(clearTeleportedOverlays)

	it("offers to add a parameter on either side", async ({ expect }) => {
		const { editor } = makeEditor()

		await mountMenuOptions(SplitDocParameterListItemMenu, {
			editor: editor,
			hovered: hoveredBlock(9),
		})

		expect(menuItem(t(ADD_ABOVE))).toBeDefined()
		expect(menuItem(t(ADD_BELOW))).toBeDefined()
	})

	it.for([
		{ label: ADD_ABOVE, side: "above" },
		{ label: ADD_BELOW, side: "below" },
	])(
		"inserts a parameter $side the hovered one",
		async ({ label, side }, { expect }) => {
			const { editor, commands } = makeEditor()
			await mountMenuOptions(SplitDocParameterListItemMenu, {
				editor: editor,
				hovered: hoveredBlock(9),
			})

			menuItem(t(label)).click()

			expect(commandNames(commands)).toEqual([
				"focus",
				"insertParameterListItemOnLeftSide",
				"run",
			])
			expect(
				commandArgs(commands, "insertParameterListItemOnLeftSide"),
			).toEqual([9, side])
		},
	)

	it("does nothing while no block is hovered", async ({ expect }) => {
		const { editor, commands } = makeEditor()
		await mountMenuOptions(SplitDocParameterListItemMenu, {
			editor: editor,
			hovered: null,
		})

		menuItem(t(ADD_ABOVE)).click()

		expect(commands).toEqual([])
	})
})
