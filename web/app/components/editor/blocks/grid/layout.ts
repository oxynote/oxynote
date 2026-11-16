import type { Node as PMNode } from "@tiptap/pm/model"
import type { Transaction } from "@tiptap/pm/state"
import {
	cellChanges,
	gridItemOf,
	type GridRect,
	layoutGrid,
	stackOrder,
	stackPlaces,
	stackPosOf,
} from "@oxynote/common/grid"

// gridLayoutOf and gridStackOf keep the layouts of each grid node here
const gridLayouts = new WeakMap<PMNode, readonly Readonly<GridRect>[]>()
const gridStacks = new WeakMap<PMNode, readonly number[]>()

// gridLayoutOf lays out a grid node's children in the wide layout. A node
// never changes, so its layout is kept for as long as the node lives.
export function gridLayoutOf(grid: PMNode): readonly Readonly<GridRect>[] {
	const kept = gridLayouts.get(grid)
	if (kept) {
		return kept
	}

	const rects = layoutGrid(
		grid.children.map((child) => gridItemOf(child.attrs)),
	)

	gridLayouts.set(grid, rects)

	return rects
}

// gridStackOf lists the indexes of a grid node's children in the order of
// the stacked layout. It keeps that order the same way as gridLayoutOf.
export function gridStackOf(grid: PMNode): readonly number[] {
	const kept = gridStacks.get(grid)
	if (kept) {
		return kept
	}

	const order = stackOrder(
		gridLayoutOf(grid),
		grid.children.map((child) => stackPosOf(child.attrs)),
	)

	gridStacks.set(grid, order)

	return order
}

// writeGridPlaces writes a layout and a stacked order to the blocks of the
// grid at gridPos. Either can be null to leave it as it is. A block gets
// only the attributes that change, so a collaborator's edits to the same
// block stay.
export function writeGridPlaces(
	tr: Transaction,
	gridPos: number,
	layout: readonly GridRect[] | null,
	stack: readonly number[] | null,
): void {
	const grid = tr.doc.nodeAt(gridPos)
	const places = stack ? stackPlaces(stack) : []

	grid?.forEach((child, offset, i) => {
		const rect = layout?.[i]
		const place = places[i]
		const changes: Record<string, number> = rect
			? cellChanges(child.attrs, rect)
			: {}

		if (place !== undefined && child.attrs.stackPos !== place) {
			changes.stackPos = place
		}

		if (Object.keys(changes).length > 0) {
			tr.setNodeMarkup(gridPos + 1 + offset, undefined, {
				...child.attrs,
				...changes,
			})
		}
	})
}
