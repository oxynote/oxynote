import { getSchema } from "@tiptap/core"
import Paragraph from "@tiptap/extension-paragraph"
import Text from "@tiptap/extension-text"
import type { Node as PMNode } from "@tiptap/pm/model"
import { mockNuxtImport } from "@nuxt/test-utils/runtime"
import { beforeEach, describe, it, onTestFinished, vi } from "vitest"
import GridView from "./GridView.vue"
import { ContentDocument } from "../document"
import { BLOCK_GRID_NAME, METRIC_BLOCK_NAME } from "../node-names"
import { mountNodeView } from "../../test-helpers/node-view"
import {
	BlockGridStub,
	GridMetricBlockStub,
	nodeType,
} from "~/components/editor/test-helpers"

// happy-dom lays nothing out, so the grid never learns its width. The
// tests set it by hand.
const gridContent = vi.hoisted(() => ({
	width: null as unknown as { value: number },
}))
mockNuxtImport("useElementSize", () => () => ({ width: gridContent.width }))

const BlockGrid = BlockGridStub.extend({
	addAttributes() {
		return { diffStatus: { default: null } }
	},
})

const MetricBlock = GridMetricBlockStub.extend({
	addAttributes() {
		return { ...this.parent?.(), uid: { default: null } }
	},
})

const schema = getSchema([
	ContentDocument,
	Paragraph,
	Text,
	BlockGrid,
	MetricBlock,
])

// the stand-in for the measured width is shared by every mount
describe("<GridView>", { concurrent: false }, () => {
	beforeEach(() => {
		gridContent.width = ref(0)
	})

	it("marks itself as the metric grid", async ({ expect }) => {
		const wrapper = await mountNodeView(GridView, { node: grid() })

		const root = wrapper.get("[data-node-view-wrapper]")

		expect(root.attributes("data-type")).toBe("block-grid")
		expect(root.classes()).toContain("block-grid")
	})

	it("exposes the node's diff status on the wrapper", async ({ expect }) => {
		const wrapper = await mountNodeView(GridView, {
			node: grid({ diffStatus: "added" }),
		})

		expect(
			wrapper.get("[data-node-view-wrapper]").attributes("data-diff-status"),
		).toBe("added")
	})

	it("lays the blocks out in a grid that Chromium reads row by row", async ({
		expect,
	}) => {
		const wrapper = await mountNodeView(GridView, { node: grid() })

		expect(wrapper.get("[data-node-view-content]").classes()).toEqual(
			expect.arrayContaining([
				"block-grid-content",
				"grid",
				"[reading-flow:grid-rows]",
			]),
		)
	})

	it.for([
		{
			name: "lists the blocks in reading order for a grid 64rem wide",
			input: 1024,
			expected: "c a b",
		},
		{
			name: "lists the blocks in their stacked order for a narrower grid",
			input: 1023,
			expected: "a b c",
		},
	])("$name", async ({ input, expected }, { expect }) => {
		gridContent.width.value = input

		const wrapper = await mountNodeView(GridView, { node: grid() })

		expect(
			wrapper.get("[data-node-view-content]").attributes("aria-owns"),
		).toBe(expected)
	})

	it("follows the grid's width as it changes", async ({ expect }) => {
		const wrapper = await mountNodeView(GridView, { node: grid() })

		gridContent.width.value = 1024
		await nextTick()

		expect(
			wrapper.get("[data-node-view-content]").attributes("aria-owns"),
		).toBe("c a b")
	})

	it("leaves aria-owns out of a grid with no blocks", async ({ expect }) => {
		const wrapper = await mountNodeView(GridView, {
			node: nodeType(schema, BLOCK_GRID_NAME).create(),
		})

		expect(
			wrapper.get("[data-node-view-content]").attributes("aria-owns"),
		).toBeUndefined()
	})

	it("decides between the layouts again only when the width changes", async ({
		expect,
	}) => {
		const wrapper = await mountNodeView(GridView, { node: grid() })
		const getComputedStyle = vi.spyOn(window, "getComputedStyle")

		await wrapper.setProps({ node: grid({ diffStatus: "added" }) })

		expect(getComputedStyle).not.toHaveBeenCalled()
	})

	it("moves Tab on past a block whose uid another block repeats", async ({
		expect,
	}) => {
		const wrapper = await mountNodeView(GridView, {
			node: nodeType(schema, BLOCK_GRID_NAME).create(null, [
				block("a", {}),
				block("a", {}),
				block("b", {}),
			]),
		})
		const content = wrapper.get("[data-node-view-content]").element
		const buttons = ["a", "a", "b"].map((uid) => {
			const element = document.createElement("div")
			const button = document.createElement("button")

			element.id = uid
			element.append(button)
			content.append(element)

			return button
		})
		const root = wrapper.element as HTMLElement
		document.body.append(root)
		onTestFinished(() => {
			root.remove()
		})
		buttons[1]?.focus()

		buttons[1]?.dispatchEvent(
			new KeyboardEvent("keydown", {
				key: "Tab",
				bubbles: true,
				cancelable: true,
			}),
		)

		expect(document.activeElement).toBe(buttons[2])
	})
})

// a grid whose wide layout reads c, a, b and whose stacked layout reads
// a, b, c. The tall block c is on the left. Block a is to its right, and
// block b is below it.
function grid(attrs: Record<string, unknown> = {}): PMNode {
	return nodeType(schema, BLOCK_GRID_NAME).create(attrs, [
		block("a", { widePosX: 12, widePosY: 0, width: 12, stackPos: 0 }),
		block("b", { widePosX: 0, widePosY: 2, width: 12, stackPos: 1 }),
		block("c", { widePosX: 0, widePosY: 0, width: 12, height: 2, stackPos: 2 }),
	])
}

function block(uid: string, attrs: Record<string, unknown>): PMNode {
	return nodeType(schema, METRIC_BLOCK_NAME).create({ uid: uid, ...attrs })
}
