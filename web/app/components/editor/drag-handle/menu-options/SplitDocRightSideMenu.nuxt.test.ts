import { afterEach, describe, it } from "vitest"
import SplitDocRightSideMenu from "./SplitDocRightSideMenu.vue"
import { hoveredBlock, mountMenuOptions } from "./test-helpers"
import {
	commandArgs,
	commandNames,
	makeEditor,
} from "../../test-helpers/node-view"
import { SUPPRESS_SCROLL_TO_SELECTION_META } from "../../scroll-control"
import { clearTeleportedOverlays, menuItem, t } from "~/components/test-helpers"

// the menu body is teleported into a shared <body>, so these tests
// cannot interleave
describe("<SplitDocRightSideMenu>", { concurrent: false }, () => {
	afterEach(clearTeleportedOverlays)

	it("offers all four insertion points", async ({ expect }) => {
		const { editor } = makeEditor()

		await mountMenuOptions(SplitDocRightSideMenu, {
			editor: editor,
			hovered: hoveredBlock(4),
		})

		expect(document.body.querySelectorAll("[role^='menuitem']")).toHaveLength(4)
	})

	it.for([
		{
			label:
				"editor.drag-handle.options.split-doc-right-side.add-code-block-above-block",
			side: "above",
		},
		{
			label:
				"editor.drag-handle.options.split-doc-right-side.add-code-block-below-block",
			side: "below",
		},
	])(
		"inserts a code block $side the hovered one",
		async ({ label, side }, { expect }) => {
			const { editor, commands } = makeEditor()
			await mountMenuOptions(SplitDocRightSideMenu, {
				editor: editor,
				hovered: hoveredBlock(4),
			})

			menuItem(t(label)).click()

			expect(commandNames(commands)).toEqual([
				"focus",
				"insertBlockOnRightSide",
				"run",
			])
			expect(commandArgs(commands, "insertBlockOnRightSide")).toEqual([
				4,
				side,
				"code",
			])
		},
	)

	it.for([
		{
			label:
				"editor.drag-handle.options.split-doc-right-side.add-metrics-above-block",
			side: "above",
		},
		{
			label:
				"editor.drag-handle.options.split-doc-right-side.add-metrics-below-block",
			side: "below",
		},
	])(
		"inserts a metrics block $side the hovered one without scrolling",
		async ({ label, side }, { expect }) => {
			const { editor, commands } = makeEditor()
			await mountMenuOptions(SplitDocRightSideMenu, {
				editor: editor,
				hovered: hoveredBlock(4),
			})

			menuItem(t(label)).click()

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

		menuItem(
			t(
				"editor.drag-handle.options.split-doc-right-side.add-code-block-above-block",
			),
		).click()

		expect(commands).toEqual([])
	})
})
