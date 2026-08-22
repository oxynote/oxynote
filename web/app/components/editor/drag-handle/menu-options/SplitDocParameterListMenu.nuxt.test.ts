import { afterEach, describe, it } from "vitest"
import SplitDocParameterListMenu from "./SplitDocParameterListMenu.vue"
import { hoveredBlock, mountMenuOptions } from "./test-helpers"
import {
	commandArgs,
	commandNames,
	makeEditor,
} from "../../test-helpers/node-view"
import { clearTeleportedOverlays, menuItem, t } from "~/components/test-helpers"

const ADD_ABOVE =
	"editor.drag-handle.options.split-doc-parameter-list.add-above-block"
const ADD_BELOW =
	"editor.drag-handle.options.split-doc-parameter-list.add-below-block"

// the menu body is teleported into a shared <body>, so these tests
// cannot interleave
describe("<SplitDocParameterListMenu>", { concurrent: false }, () => {
	afterEach(clearTeleportedOverlays)

	it("offers to add a parameter list on either side", async ({ expect }) => {
		const { editor } = makeEditor()

		await mountMenuOptions(SplitDocParameterListMenu, {
			editor: editor,
			hovered: hoveredBlock(7),
		})

		expect(menuItem(t(ADD_ABOVE))).toBeDefined()
		expect(menuItem(t(ADD_BELOW))).toBeDefined()
	})

	it.for([
		{ label: ADD_ABOVE, side: "above" },
		{ label: ADD_BELOW, side: "below" },
	])(
		"inserts a parameter list $side the hovered one",
		async ({ label, side }, { expect }) => {
			const { editor, commands } = makeEditor()
			await mountMenuOptions(SplitDocParameterListMenu, {
				editor: editor,
				hovered: hoveredBlock(7),
			})

			menuItem(t(label)).click()

			expect(commandNames(commands)).toEqual([
				"focus",
				"insertParameterListOnLeftSide",
				"run",
			])
			expect(commandArgs(commands, "insertParameterListOnLeftSide")).toEqual([
				7,
				side,
			])
		},
	)

	it("does nothing while no block is hovered", async ({ expect }) => {
		const { editor, commands } = makeEditor()
		await mountMenuOptions(SplitDocParameterListMenu, {
			editor: editor,
			hovered: null,
		})

		menuItem(t(ADD_ABOVE)).click()

		expect(commands).toEqual([])
	})
})
