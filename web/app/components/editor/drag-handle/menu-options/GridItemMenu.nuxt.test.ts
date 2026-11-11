import { getSchema, Node as TiptapNode } from "@tiptap/core"
import Paragraph from "@tiptap/extension-paragraph"
import Text from "@tiptap/extension-text"
import type { Node as PMNode } from "@tiptap/pm/model"
import { EditorState, type Transaction } from "@tiptap/pm/state"
import { afterEach, describe, it, vi } from "vitest"
import GridItemMenu from "./GridItemMenu.vue"
import { hoveredBlock, mountMenuOptions } from "./test-helpers"
import { makeEditor } from "../../test-helpers/node-view"
import { ContentDocument } from "../../blocks/document"
import { BLOCK_GRID_NAME, METRIC_BLOCK_NAME } from "../../blocks/node-names"
import { GridItemWidth } from "@oxynote/common/grid"
import {
	BlockGridStub,
	GridMetricBlockStub,
	nodeType,
} from "~/components/editor/test-helpers"
import { clearTeleportedOverlays, menuItem, t } from "~/components/test-helpers"

const MetricBlock = GridMetricBlockStub.extend({
	addAttributes() {
		return { ...this.parent?.(), uid: { default: null } }
	},
})

// a container that is not a grid, such as the right side of a split
// documentation block
const Container = TiptapNode.create({
	name: "container",
	group: "block",
	content: `${METRIC_BLOCK_NAME}+`,
})

const schema = getSchema([
	ContentDocument,
	Paragraph,
	Text,
	BlockGridStub,
	MetricBlock,
	Container,
])

// the menu bodies are teleported into a shared <body>, so these tests
// cannot interleave
describe("<GridItemMenu>", { concurrent: false }, () => {
	afterEach(clearTeleportedOverlays)

	it.for([
		{
			name: "width",
			input: "width",
			expected: ["compact", "standard", "wide"],
		},
		{ name: "height", input: "height", expected: ["standard", "tall"] },
	])("offers every named $name", async ({ input, expected }, { expect }) => {
		await mountMenu(gridDoc(block("a"), block("b")), 1)

		await openSubmenu(t(`editor.drag-handle.options.grid-item.${input}`))

		for (const name of expected) {
			expect(
				menuItem(t(`editor.grid-item.${input}-options.${name}`)),
			).toBeDefined()
		}
	})

	it.for([
		{
			name: "the named width of a block saved before blocks stored columns",
			input: { attrs: { width: GridItemWidth.Wide }, menu: "width" },
			expected: "width-options.wide",
		},
		{
			name: "the standard width of a block with no size at all",
			input: { attrs: { width: null }, menu: "width" },
			expected: "width-options.standard",
		},
		{
			name: "the columns a block stores",
			input: { attrs: { width: 8 }, menu: "width" },
			expected: "width-options.compact",
		},
		{
			name: "the standard height of a block saved before blocks had one",
			input: { attrs: { height: null }, menu: "height" },
			expected: "height-options.standard",
		},
		{
			name: "the rows a block stores",
			input: { attrs: { height: 2 }, menu: "height" },
			expected: "height-options.tall",
		},
	])("marks $name", async ({ input, expected }, { expect }) => {
		await mountMenu(gridDoc(block("a", input.attrs)), 1, input.attrs)

		await openSubmenu(t(`editor.drag-handle.options.grid-item.${input.menu}`))

		expect(checked(t(`editor.grid-item.${expected}`))).toBe(true)
	})

	it("shows and marks the columns of a width the menu has no name for", async ({
		expect,
	}) => {
		await mountMenu(gridDoc(block("a", { width: 10 })), 1, { width: 10 })

		await openSubmenu(t("editor.drag-handle.options.grid-item.width"))

		expect(
			checked(t("editor.grid-item.width-options.columns", { count: 10 }, 10)),
		).toBe(true)
	})

	it("leaves the columns and rows of a named size out", async ({ expect }) => {
		await mountMenu(gridDoc(block("a")), 1)

		await openSubmenu(t("editor.drag-handle.options.grid-item.width"))

		expect(document.body.textContent).not.toContain(
			t("editor.grid-item.width-options.columns", { count: 12 }, 12),
		)
	})

	it("widens a block and moves the block that has no room down", async ({
		expect,
	}) => {
		const { dispatch, state } = await mountMenu(
			gridDoc(block("a"), block("b")),
			1,
		)
		await openSubmenu(t("editor.drag-handle.options.grid-item.width"))

		menuItem(t("editor.grid-item.width-options.wide")).click()

		expect(dispatch).toHaveBeenCalledTimes(1)
		expect(cells(applied(state, dispatch).child(0))).toEqual([
			"a 0,0 24x1",
			"b 0,1 12x1",
		])
	})

	it("makes a block tall and leaves the block beside it alone", async ({
		expect,
	}) => {
		const { dispatch, state } = await mountMenu(
			gridDoc(block("a"), block("b")),
			1,
		)
		await openSubmenu(t("editor.drag-handle.options.grid-item.height"))

		menuItem(t("editor.grid-item.height-options.tall")).click()

		expect(cells(applied(state, dispatch).child(0))).toEqual([
			"a 0,0 12x2",
			"b 12,0 12x1",
		])
	})

	it("keeps the height the block has now when the width changes", async ({
		expect,
	}) => {
		const { dispatch, state } = await mountMenu(
			gridDoc(block("a", { height: 2 }), block("b")),
			1,
			{ height: 1 },
		)
		await openSubmenu(t("editor.drag-handle.options.grid-item.width"))

		menuItem(t("editor.grid-item.width-options.wide")).click()

		expect(cells(applied(state, dispatch).child(0))).toEqual([
			"a 0,0 24x2",
			"b 0,2 12x1",
		])
	})

	it("writes only the blocks whose cells change", async ({ expect }) => {
		const placedA = { widePosX: 0, widePosY: 0, width: 12, height: 1 }
		const placedB = { widePosX: 12, widePosY: 0, width: 12, height: 1 }
		const { dispatch } = await mountMenu(
			gridDoc(block("a", placedA), block("b", placedB)),
			1,
			placedA,
		)
		await openSubmenu(t("editor.drag-handle.options.grid-item.height"))

		menuItem(t("editor.grid-item.height-options.tall")).click()

		expect(dispatch.mock.calls[0]?.[0].steps).toHaveLength(1)
	})

	it("leaves the stacked places alone in a grid that shows its columns", async ({
		expect,
	}) => {
		const { dispatch, state } = await mountMenu(
			gridDoc(block("a"), block("b"), block("c")),
			1,
		)
		await openSubmenu(t("editor.drag-handle.options.grid-item.width"))

		menuItem(t("editor.grid-item.width-options.wide")).click()

		expect(
			applied(state, dispatch)
				.child(0)
				.children.map((child): unknown => child.attrs.stackPos),
		).toEqual([null, null, null])
	})

	it("resizes a block outside a grid without giving it cells", async ({
		expect,
	}) => {
		const doc = nodeType(schema, "doc").create(null, [
			nodeType(schema, "container").create(null, [block("a")]),
		])
		const { dispatch, state } = await mountMenu(doc, 1)
		await openSubmenu(t("editor.drag-handle.options.grid-item.width"))

		menuItem(t("editor.grid-item.width-options.compact")).click()

		expect(cells(applied(state, dispatch).child(0))).toEqual([
			"a null,null 8x1",
		])
	})
})

function block(uid: string, attrs: Record<string, unknown> = {}): PMNode {
	return nodeType(schema, METRIC_BLOCK_NAME).create({ uid: uid, ...attrs })
}

function gridDoc(...blocks: PMNode[]): PMNode {
	return nodeType(schema, "doc").create(null, [
		nodeType(schema, BLOCK_GRID_NAME).create(null, blocks),
	])
}

// mounts the menu for the block at pos. The editor's dispatch records each
// transaction.
async function mountMenu(
	doc: PMNode,
	pos: number,
	attrs: Record<string, unknown> = {},
) {
	const state = EditorState.create({ doc: doc })
	const dispatch = vi.fn<(tr: Transaction) => void>()
	const { editor } = makeEditor({
		state: state,
		view: { dispatch: dispatch },
	})

	await mountMenuOptions(GridItemMenu, {
		editor: editor,
		hovered: hoveredBlock(pos, { attrs: attrs }),
	})

	return { state, dispatch }
}

async function openSubmenu(label: string) {
	const trigger = menuItem(label)
	trigger.dispatchEvent(new PointerEvent("pointerdown", { bubbles: true }))
	trigger.click()
	await nextTick()
	await nextTick()
}

function checked(label: string): boolean {
	return menuItem(label).querySelector(".i-lucide\\:check") !== null
}

// the document after the one transaction the menu dispatched
function applied(
	state: EditorState,
	dispatch: ReturnType<typeof vi.fn<(tr: Transaction) => void>>,
): PMNode {
	const tr = dispatch.mock.calls[0]?.[0]

	if (!tr) {
		throw new Error("the menu dispatched nothing")
	}

	return state.apply(tr).doc
}

function cells(parent: PMNode): string[] {
	return parent.children.map(
		(child) =>
			`${String(child.attrs.uid)} ${String(child.attrs.widePosX)},${String(child.attrs.widePosY)} ${String(child.attrs.width)}x${String(child.attrs.height)}`,
	)
}
