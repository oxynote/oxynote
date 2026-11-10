import type { JSONContent } from "@tiptap/core"
import {
	gridItemOf,
	layoutGrid,
	stackOrder,
	stackPlaces,
	stackPosOf,
} from "@oxynote/common/grid"
import {
	BLOCK_GRID_NAMES,
	gridSizeLabel,
	type Translate,
} from "~/components/editor/blocks/grid/utils"
import {
	type AttributeChange,
	type FieldChange,
	formatChange,
} from "./change-count"
import { DiffStatus } from "./position-map"

// a grid block's row and column in the wide layout, counted from zero
interface GridPlace {
	row: number
	column: number
}

// withGridPlaces gives every block of a document's grids the places where
// it shows. gridPlace holds its row and column in the wide layout, and
// stackPlace holds its place in the stacked layout. The diff compares grid
// blocks by these places and not by the cells they store. So a block that
// shows in the same place stays unchanged, even when its cells differ. But
// a block that a drop pushed aside counts as changed.
export function withGridPlaces(doc: JSONContent): JSONContent {
	return {
		...doc,
		content: doc.content?.map((node) =>
			node.type && BLOCK_GRID_NAMES.has(node.type) ? placeBlocks(node) : node,
		),
	}
}

// removedAfterGrid moves the removed blocks of a merged grid into a copy of
// the grid, right after it. In their old cells they would push the other
// blocks aside. Each block keeps its old places in oldNode, so its change
// marker lists where it was.
export function removedAfterGrid(grid: JSONContent): JSONContent[] {
	const blocks = grid.content ?? []
	const removed = blocks.filter(
		(block) => block.attrs?.diffStatus === DiffStatus.Removed,
	)

	if (removed.length === 0 || removed.length === blocks.length) {
		return [grid]
	}

	const uid = grid.attrs?.uid as string | null | undefined

	return [
		{ ...grid, content: blocks.filter((block) => !removed.includes(block)) },
		{
			...grid,
			attrs: { ...grid.attrs, uid: uid ? `${uid}-removed` : null },
			content: removed.map((block) => ({
				...block,
				attrs: {
					...block.attrs,
					widePosX: null,
					widePosY: null,
					stackPos: null,
					gridPlace: null,
					stackPlace: null,
					oldNode: block,
				},
			})),
		},
	]
}

// gridItemRow returns the card row of a grid block's size or place, or null
// for any other change
export function gridItemRow(
	change: AttributeChange,
	t: Translate,
	locale: string,
): FieldChange | null {
	const row = (label: string, format: (value: unknown) => string) => ({
		label: t(`editor.diff-change-marker.grid-item.${label}`),
		...formatChange(change, format),
	})

	switch (change.name) {
		case "width":
			return row("width", (value) => gridSizeLabel(t, "width", value as number))
		case "height":
			return row("height", (value) =>
				gridSizeLabel(t, "height", value as number),
			)
		case "gridPlace":
			return row("wide-layout", (value) => {
				const place = value as GridPlace

				return t("editor.diff-change-marker.grid-item.wide-place", {
					row: place.row + 1,
					column: place.column + 1,
				})
			})
		case "stackPlace":
			return row("stacked-layout", (value) => {
				const place = (value as number) + 1
				const form = new Intl.PluralRules(locale, { type: "ordinal" }).select(
					place,
				)

				return t(`editor.diff-change-marker.grid-item.stacked-place.${form}`, {
					place: place,
				})
			})
		default:
			return null
	}
}

function placeBlocks(grid: JSONContent): JSONContent {
	const blocks = grid.content ?? []
	const rects = layoutGrid(blocks.map((block) => gridItemOf(block.attrs ?? {})))
	const stack = stackPlaces(
		stackOrder(
			rects,
			blocks.map((block) => stackPosOf(block.attrs ?? {})),
		),
	)

	return {
		...grid,
		content: blocks.map((block, i) => {
			const rect = rects[i] ?? { x: 0, y: 0 }

			return {
				...block,
				attrs: {
					...block.attrs,
					gridPlace: { row: rect.y, column: rect.x },
					stackPlace: stack[i],
				},
			}
		}),
	}
}
