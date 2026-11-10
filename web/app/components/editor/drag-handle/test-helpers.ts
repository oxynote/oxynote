import {
	type Editor,
	Extension,
	type JSONContent,
	Node as TiptapNode,
} from "@tiptap/core"
import type { Plugin } from "@tiptap/pm/state"
import {
	BLOCK_GRID_NAME,
	LEGACY_METRIC_GRID_NAME,
	METRIC_BLOCK_NAME,
	SPLIT_DOCUMENTATION_LEFT_SIDE_NAME,
	SPLIT_DOCUMENTATION_NAME,
	SPLIT_DOCUMENTATION_RIGHT_SIDE_NAME,
} from "../blocks/node-names"
import { BlockGrid as EditorBlockGrid } from "../blocks/grid"
import {
	BlockGridStub,
	GridMetricBlockStub,
	LegacyMetricGridStub,
} from "../test-helpers"
import { GapDecorations } from "./gap-decorations"

// the grid styles of the drag suites. Inside .wide-grid a grid shows 24
// columns of 12px and places each block in its cells. Inside .narrow-grid
// it stacks its blocks in their stacked order. Rows are 40px tall and gaps
// are 4px.
export const GRID_STYLES = `
	[data-type="block-grid"] {
		display: flex;
		flex-wrap: wrap;
		gap: 12px;
		min-height: 24px;
		margin: 8px 0;
	}
	[data-type="metric-block"] {
		width: 100px;
		height: 40px;
	}
	.wide-grid [data-type="block-grid"] {
		display: grid;
		grid-template-columns: repeat(24, 12px);
		grid-auto-rows: 40px;
		gap: 4px;
	}
	.wide-grid [data-grid-cell] {
		grid-column: var(--grid-col) / span var(--grid-w);
		grid-row: var(--grid-row) / span var(--grid-h);
		width: auto;
		height: auto;
	}
	.narrow-grid [data-type="block-grid"] {
		display: grid;
		grid-template-columns: minmax(0, 1fr);
		grid-auto-rows: 40px;
		gap: 4px;
	}
	.narrow-grid [data-grid-cell] {
		order: var(--grid-order);
		grid-column: 1 / -1;
		grid-row: span var(--grid-h);
		width: auto;
		height: auto;
	}
`

// the grids and their blocks as the drag suites lay them out. A grid is
// one element that holds its blocks. A block carries a title, so a test
// can find it by that title.
export const RenderedBlockGrid = BlockGridStub.extend({
	selectable: false,

	renderHTML() {
		return ["div", { "data-type": "block-grid" }, 0]
	},
})

export const RenderedLegacyMetricGrid = LegacyMetricGridStub.extend({
	selectable: false,

	renderHTML() {
		return ["div", { "data-type": "block-grid" }, 0]
	},
})

export const TitledMetricBlock = GridMetricBlockStub.extend({
	defining: true,
	selectable: false,

	addAttributes() {
		return {
			...this.parent?.(),
			title: { default: "" },
			uid: { default: null },
			nodeCommentId: { default: null },
		}
	},

	renderHTML({ HTMLAttributes }) {
		return ["div", { ...HTMLAttributes, "data-type": "metric-block" }]
	},
})

// the real grid's layout plugin, which it adds after its migration plugin.
// It hands every grid block its cells as CSS variables. The grid styles use
// them to place the block.
export const GridLayout = Extension.create({
	name: "gridLayout",
	addProseMirrorPlugins() {
		const plugins = EditorBlockGrid.config.addProseMirrorPlugins?.call({
			editor: this.editor,
		} as never)

		return plugins?.slice(1) ?? []
	},
})

// a split documentation whose right side holds metric blocks straight,
// with no grid around them, the way a new document's starts out
export const SplitDocumentationStub = TiptapNode.create({
	name: SPLIT_DOCUMENTATION_NAME,
	group: "block",
	content: `${SPLIT_DOCUMENTATION_LEFT_SIDE_NAME} ${SPLIT_DOCUMENTATION_RIGHT_SIDE_NAME}`,

	renderHTML() {
		return ["div", 0]
	},
})

export const SplitDocumentationLeftSideStub = TiptapNode.create({
	name: SPLIT_DOCUMENTATION_LEFT_SIDE_NAME,
	content: "paragraph+",

	renderHTML() {
		return ["div", 0]
	},
})

export const SplitDocumentationRightSideStub = TiptapNode.create({
	name: SPLIT_DOCUMENTATION_RIGHT_SIDE_NAME,
	content: `${METRIC_BLOCK_NAME}+`,

	renderHTML() {
		return ["div", 0]
	},
})

// addProseMirrorPlugins reads neither this.options nor this.editor, so
// a bare context stands in for the extension instance the editor
// normally binds it to
export function gapPlugin(): Plugin {
	const addProseMirrorPlugins = GapDecorations.config.addProseMirrorPlugins

	if (!addProseMirrorPlugins) {
		throw new Error("GapDecorations declares no prosemirror plugins")
	}

	const plugin = addProseMirrorPlugins.call({} as never)[0]

	if (!plugin) {
		throw new Error("GapDecorations produced no prosemirror plugin")
	}

	return plugin
}

export function bulletList(...items: JSONContent[]): JSONContent {
	return { type: "bulletList", content: items }
}

export function taskList(...items: JSONContent[]): JSONContent {
	return { type: "taskList", content: items }
}

export function blockGrid(...blocks: JSONContent[]): JSONContent {
	return { type: BLOCK_GRID_NAME, content: blocks }
}

export function legacyGrid(...blocks: JSONContent[]): JSONContent {
	return { type: LEGACY_METRIC_GRID_NAME, content: blocks }
}

export function metricBlock(
	title: string,
	attrs: Record<string, unknown> = {},
): JSONContent {
	return { type: METRIC_BLOCK_NAME, attrs: { title, ...attrs } }
}

export function posOfMetricBlock(editor: Editor, title: string): number {
	const matches: number[] = []

	editor.state.doc.descendants((node, pos) => {
		if (node.attrs.title === title) {
			matches.push(pos)
		}
	})

	const [first] = matches

	if (first === undefined) {
		throw new Error(`no metric block titled "${title}"`)
	}

	return first
}

export function blockElement(editor: Editor, title: string): HTMLElement {
	const element = editor.view.dom.querySelector<HTMLElement>(
		`[data-type="metric-block"][title="${title}"]`,
	)

	if (!element) {
		throw new Error(`no element for the metric block titled "${title}"`)
	}

	return element
}

// a point inside an element, as fractions of its width and height
export function pointIn(
	element: Element,
	u: number,
	v: number,
): { x: number; y: number } {
	const box = element.getBoundingClientRect()

	return { x: box.left + box.width * u, y: box.top + box.height * v }
}

// each block of a grid with the cells its attributes store
export function storedCells(editor: Editor, gridIndex = 0): string[] {
	return editor.state.doc
		.child(gridIndex)
		.children.map(
			(child) =>
				`${String(child.attrs.title)} ${String(child.attrs.widePosX)},${String(child.attrs.widePosY)} ${String(child.attrs.width)}x${String(child.attrs.height)}`,
		)
}

// each block of a grid with its place in the stacked layout
export function storedStackPlaces(editor: Editor, gridIndex = 0): string[] {
	return editor.state.doc
		.child(gridIndex)
		.children.map(
			(child) => `${String(child.attrs.title)} ${String(child.attrs.stackPos)}`,
		)
}
