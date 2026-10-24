import { afterEach, describe, it } from "vitest"
import SplitDocRightSideMenu from "./SplitDocRightSideMenu.vue"
import { hoveredBlock, mountMenuOptions } from "./test-helpers"
import {
	commandArgs,
	commandNames,
	makeEditor,
} from "../../test-helpers/node-view"
import { SUPPRESS_SCROLL_TO_SELECTION_META } from "../../scroll-control"
import {
	clearTeleportedOverlays,
	menuItem,
	openSubMenu,
	t,
} from "~/components/test-helpers"

const ADD_ABOVE =
	"editor.drag-handle.options.split-doc-right-side.add-above-block"
const ADD_BELOW =
	"editor.drag-handle.options.split-doc-right-side.add-below-block"
const CODE = "editor.drag-handle.options.split-doc-right-side.block-types.code"
const METRICS =
	"editor.drag-handle.options.split-doc-right-side.block-types.metrics"
const DIAGRAM =
	"editor.drag-handle.options.split-doc-right-side.block-types.diagram"

// the menu body is teleported into a shared <body>, so these tests
// cannot interleave
describe("<SplitDocRightSideMenu>", { concurrent: false }, () => {
	afterEach(clearTeleportedOverlays)

	it("offers one sub menu per side", async ({ expect }) => {
		const { editor } = makeEditor()

		await mountMenuOptions(SplitDocRightSideMenu, {
			editor: editor,
			hovered: hoveredBlock(4),
		})

		expect(menuLabels()).toEqual([t(ADD_ABOVE), t(ADD_BELOW)])
	})

	it.for([
		{ sideLabel: ADD_ABOVE, side: "above" },
		{ sideLabel: ADD_BELOW, side: "below" },
	])(
		"offers the three block types to add $side",
		async ({ sideLabel }, { expect }) => {
			const { editor } = makeEditor()
			await mountMenuOptions(SplitDocRightSideMenu, {
				editor: editor,
				hovered: hoveredBlock(4),
			})

			await openSubMenu(t(sideLabel))

			expect(
				menuLabels().filter(
					(label) => label !== t(ADD_ABOVE) && label !== t(ADD_BELOW),
				),
			).toEqual([t(CODE), t(METRICS), t(DIAGRAM)])
		},
	)

	it.for([
		{ sideLabel: ADD_ABOVE, side: "above", label: CODE, blockType: "code" },
		{ sideLabel: ADD_BELOW, side: "below", label: CODE, blockType: "code" },
		{
			sideLabel: ADD_ABOVE,
			side: "above",
			label: DIAGRAM,
			blockType: "diagram",
		},
		{
			sideLabel: ADD_BELOW,
			side: "below",
			label: DIAGRAM,
			blockType: "diagram",
		},
	])(
		"inserts a $blockType block $side the hovered one",
		async ({ sideLabel, side, label, blockType }, { expect }) => {
			const { editor, commands } = makeEditor()
			await mountMenuOptions(SplitDocRightSideMenu, {
				editor: editor,
				hovered: hoveredBlock(4),
			})
			await openSubMenu(t(sideLabel))

			menuItem(t(label)).click()

			expect(commandNames(commands)).toEqual([
				"focus",
				"insertBlockOnRightSide",
				"run",
			])
			expect(commandArgs(commands, "insertBlockOnRightSide")).toEqual([
				4,
				side,
				blockType,
			])
		},
	)

	it.for([
		{ sideLabel: ADD_ABOVE, side: "above" },
		{ sideLabel: ADD_BELOW, side: "below" },
	])(
		"inserts a metrics block $side the hovered one without scrolling",
		async ({ sideLabel, side }, { expect }) => {
			const { editor, commands } = makeEditor()
			await mountMenuOptions(SplitDocRightSideMenu, {
				editor: editor,
				hovered: hoveredBlock(4),
			})
			await openSubMenu(t(sideLabel))

			menuItem(t(METRICS)).click()

			expect(commandNames(commands)).toEqual([
				"setMeta",
				"focus",
				"insertBlockOnRightSide",
				"run",
			])
			expect(commandArgs(commands, "setMeta")).toEqual([
				SUPPRESS_SCROLL_TO_SELECTION_META,
				true,
			])
			expect(commandArgs(commands, "insertBlockOnRightSide")).toEqual([
				4,
				side,
				"metrics",
			])
		},
	)

	it("does nothing while no block is hovered", async ({ expect }) => {
		const { editor, commands } = makeEditor()
		await mountMenuOptions(SplitDocRightSideMenu, {
			editor: editor,
			hovered: null,
		})
		await openSubMenu(t(ADD_ABOVE))

		menuItem(t(CODE)).click()

		expect(commands).toEqual([])
	})
})

function menuLabels(): string[] {
	return Array.from(
		document.body.querySelectorAll<HTMLElement>("[role^='menuitem']"),
	).map((item) => item.textContent.trim())
}
