import { afterEach, describe, it, vi } from "vitest"
import MetricBlockMenu from "./MetricBlockMenu.vue"
import { hoveredBlock, mountMenuOptions } from "./test-helpers"
import { makeEditor } from "../../test-helpers/node-view"
import { MetricBlockWidth } from "../../blocks/metrics/utils"
import { clearTeleportedOverlays, menuItem, t } from "~/components/test-helpers"

// the editor a resize goes through: the transaction it builds records
// which node it re-marked and with what
function makeResizeEditor(
	nodeAtPos: { attrs: Record<string, unknown> } | null,
) {
	const setNodeMarkup = vi.fn(
		(pos: number, _type: unknown, attrs: Record<string, unknown>) => ({
			pos: pos,
			attrs: attrs,
		}),
	)
	const dispatch = vi.fn()
	const { editor } = makeEditor({
		state: {
			doc: { nodeAt: () => nodeAtPos },
			tr: { setNodeMarkup: setNodeMarkup },
		},
		view: { dispatch: dispatch },
	})

	return { editor, setNodeMarkup, dispatch }
}

async function openSizeMenu() {
	const trigger = menuItem(t("editor.drag-handle.options.metric-block.width"))
	trigger.dispatchEvent(new PointerEvent("pointerdown", { bubbles: true }))
	trigger.click()
	await nextTick()
	await nextTick()
}

// the menu bodies are teleported into a shared <body>, so these tests
// cannot interleave
describe("<MetricBlockMenu>", { concurrent: false }, () => {
	afterEach(clearTeleportedOverlays)

	it("offers to resize the block", async ({ expect }) => {
		const { editor } = makeResizeEditor({ attrs: {} })

		await mountMenuOptions(MetricBlockMenu, {
			editor: editor,
			hovered: hoveredBlock(5),
		})

		expect(
			menuItem(t("editor.drag-handle.options.metric-block.width")),
		).toBeDefined()
	})

	it("offers the three block widths", async ({ expect }) => {
		const { editor } = makeResizeEditor({ attrs: {} })
		await mountMenuOptions(MetricBlockMenu, {
			editor: editor,
			hovered: hoveredBlock(5),
		})

		await openSizeMenu()

		expect(
			menuItem(
				t("editor.drag-handle.options.metric-block.width-options.compact"),
			),
		).toBeDefined()
		expect(
			menuItem(
				t("editor.drag-handle.options.metric-block.width-options.standard"),
			),
		).toBeDefined()
		expect(
			menuItem(t("editor.drag-handle.options.metric-block.width-options.wide")),
		).toBeDefined()
	})

	it("marks the width the block already has", async ({ expect }) => {
		const { editor } = makeResizeEditor({ attrs: {} })
		await mountMenuOptions(MetricBlockMenu, {
			editor: editor,
			hovered: hoveredBlock(5, { attrs: { width: MetricBlockWidth.Wide } }),
		})

		await openSizeMenu()

		expect(
			menuItem(
				t("editor.drag-handle.options.metric-block.width-options.wide"),
			).querySelector(".i-lucide\\:check"),
		).not.toBeNull()
		expect(
			menuItem(
				t("editor.drag-handle.options.metric-block.width-options.compact"),
			).querySelector(".i-lucide\\:check"),
		).toBeNull()
	})

	it("treats a block with no stored width as a standard one", async ({
		expect,
	}) => {
		const { editor } = makeResizeEditor({ attrs: {} })
		await mountMenuOptions(MetricBlockMenu, {
			editor: editor,
			hovered: hoveredBlock(5),
		})

		await openSizeMenu()

		expect(
			menuItem(
				t("editor.drag-handle.options.metric-block.width-options.standard"),
			).querySelector(".i-lucide\\:check"),
		).not.toBeNull()
	})

	it("resizes the block to the width that was picked", async ({ expect }) => {
		const { editor, setNodeMarkup, dispatch } = makeResizeEditor({
			attrs: { uid: "metric-1", width: MetricBlockWidth.Standard },
		})
		await mountMenuOptions(MetricBlockMenu, {
			editor: editor,
			hovered: hoveredBlock(5),
		})
		await openSizeMenu()

		menuItem(
			t("editor.drag-handle.options.metric-block.width-options.compact"),
		).click()

		expect(setNodeMarkup).toHaveBeenCalledTimes(1)
		expect(setNodeMarkup).toHaveBeenCalledWith(5, null, {
			uid: "metric-1",
			width: MetricBlockWidth.Compact,
		})
		expect(dispatch).toHaveBeenCalledTimes(1)
	})

	it("resizes nothing when the block is no longer there", async ({
		expect,
	}) => {
		const { editor, setNodeMarkup, dispatch } = makeResizeEditor(null)
		await mountMenuOptions(MetricBlockMenu, {
			editor: editor,
			hovered: hoveredBlock(5),
		})
		await openSizeMenu()

		menuItem(
			t("editor.drag-handle.options.metric-block.width-options.compact"),
		).click()

		expect(setNodeMarkup).toHaveBeenCalledTimes(0)
		expect(dispatch).toHaveBeenCalledTimes(0)
	})

	it("resizes nothing while no block is hovered", async ({ expect }) => {
		const { editor, dispatch } = makeResizeEditor({ attrs: {} })
		await mountMenuOptions(MetricBlockMenu, {
			editor: editor,
			hovered: null,
		})
		await openSizeMenu()

		menuItem(
			t("editor.drag-handle.options.metric-block.width-options.compact"),
		).click()

		expect(dispatch).toHaveBeenCalledTimes(0)
	})
})
