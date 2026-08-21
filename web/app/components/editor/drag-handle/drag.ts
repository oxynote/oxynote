import { Extension } from "@tiptap/core"
import { type EditorState, Plugin, Selection } from "prosemirror-state"
import {
	Fragment,
	type Node,
	type ResolvedPos,
	type Schema,
	type Slice,
} from "prosemirror-model"
import type { EditorView } from "@tiptap/pm/view"
import { isDraggableNodeType } from "./node-detection"
import {
	getGapDropPosition,
	findGapElementsByPos,
	repositionGapZones,
	clearRepositionCooldown,
} from "./gap-decorations"
import {
	cursorOffsetByType,
	DRAG_COMPOSITE_DRAG_NODES,
	DRAG_CONTAINER_NODE_TYPES,
	LIST_NODE_TYPES,
	LIST_ITEM_TO_DEFAULT_LIST_TYPE,
	DRAG_HANDLE_IGNORE_SELF_CLASS,
	NODE_TO_WRAPPER_TYPE,
} from "./config"
import { deleteEmptyMetricGrids, MetricGrid } from "../blocks/metrics"
import { METRIC_BLOCK_NAME } from "../blocks/node-names"

// extra drag metadata that handleDragData in handle-plugin.ts stores on
// view.dragging alongside ProseMirror's own slice/move fields. The extra
// fields are optional because external drags produce plain dragging objects
// without them.
export interface DragHandleDragging {
	slice: Slice
	move: boolean
	parentListType?: string | null
	sourceWrapperBounds?: { start: number; end: number } | null
}

function getDragging(view: EditorView): DragHandleDragging | null {
	return view.dragging
}

/**
 * Normalize a drop position at list boundaries.
 * If the position is right after a list (at root/parent level),
 * return the position at the end of the list (after the last list item).
 * If the position is right before a list (at root/parent level),
 * return the position at the start of the list (before the first list item).
 * This reduces flickering between nested and root-level drop zones.
 *
 * Only applies when dragging list items of the SAME type as the adjacent list.
 * When dragging a different list type (e.g., task list over bullet list),
 * we keep the root-level position to treat the list as atomic.
 */
function normalizeListBoundaryPosition(
	doc: Node,
	pos: number,
	isDraggingListItem: boolean,
): number {
	// Only normalize when dragging list items
	if (!isDraggingListItem) {
		return pos
	}

	try {
		const $pos = doc.resolve(pos)
		const nodeBefore = $pos.nodeBefore
		const nodeAfter = $pos.nodeAfter

		// Check if there's a same-type list immediately before this position
		if (
			nodeBefore &&
			LIST_NODE_TYPES.has(nodeBefore.type.name) &&
			lastParentListType === nodeBefore.type.name
		) {
			// Position is right after a same-type list - return position inside the list
			return pos - 1
		}

		// Check if there's a same-type list immediately after this position
		if (
			nodeAfter &&
			LIST_NODE_TYPES.has(nodeAfter.type.name) &&
			lastParentListType === nodeAfter.type.name
		) {
			// Position is right before a same-type list - return position inside the list
			return pos + 1
		}
	} catch {
		// Ignore errors, return original position
	}

	return pos
}

/**
 * Check if we're dragging a MetricBlock based on the current slice.
 */
function isDraggingMetricBlock(view: EditorView): boolean {
	const slice = getCurrentSlice(view)
	if (!slice || slice.content.childCount === 0) {
		return false
	}
	const firstChild = slice.content.firstChild
	return firstChild?.type.name === METRIC_BLOCK_NAME
}

/**
 * Find the MetricGrid element at the given coordinates.
 * Returns null if cursor is not within any MetricGrid bounds.
 */
function findMetricGridAtCoords(
	view: EditorView,
	clientX: number,
	clientY: number,
	options?: { includeMargins?: boolean },
): { gridElement: HTMLElement; gridPos: number; inMargin?: boolean } | null {
	const { doc } = view.state
	const includeMargins = options?.includeMargins ?? false

	// Find all MetricGrid nodes in the document
	const metricGrids: { pos: number; node: Node }[] = []
	doc.descendants((node: Node, pos: number) => {
		if (node.type.name === MetricGrid.name) {
			metricGrids.push({ pos, node })
			return false // Don't descend into MetricGrid
		}
		return true
	})

	// Check each MetricGrid's DOM bounds
	for (const { pos } of metricGrids) {
		const gridDOM = view.nodeDOM(pos)
		if (!gridDOM || !(gridDOM instanceof HTMLElement)) {
			continue
		}

		const rect = gridDOM.getBoundingClientRect()

		// Check if cursor is within this grid's bounds
		if (
			clientX >= rect.left &&
			clientX <= rect.right &&
			clientY >= rect.top &&
			clientY <= rect.bottom
		) {
			return { gridElement: gridDOM, gridPos: pos }
		}

		// When includeMargins is true, also match if cursor is within Y bounds
		// but outside X bounds (in the side margins). This allows MetricBlock
		// drag to show cursor at row edges when hovering in margins.
		if (includeMargins && clientY >= rect.top && clientY <= rect.bottom) {
			return { gridElement: gridDOM, gridPos: pos, inMargin: true }
		}
	}

	return null
}

/**
 * Find drop position within a MetricGrid using X-axis midpoint logic.
 * MetricGrid uses a horizontal layout, so we compare clientX against each MetricBlock's center.
 * For multi-row grids, first filters to the row containing clientY.
 *
 * Returns the document position where the drop should occur, or null if invalid.
 *
 * @param view - The editor view
 * @param clientX - Mouse X coordinate
 * @param clientY - Mouse Y coordinate (used to determine which row)
 * @param gridPos - Document position of the MetricGrid
 * @param gridElement - The DOM element of the MetricGrid
 */
function findMetricGridDropPosition(
	view: EditorView,
	clientX: number,
	clientY: number,
	gridPos: number,
	_gridElement: HTMLElement,
): number | null {
	const { doc } = view.state
	const gridNode = doc.nodeAt(gridPos)

	if (gridNode?.type.name !== MetricGrid.name) {
		return null
	}

	// Get all MetricBlock children with their positions and DOM elements
	const allBlocks: {
		pos: number
		node: Node
		dom: HTMLElement
		rect: DOMRect
	}[] = []

	gridNode.forEach((child: Node, childOffset: number) => {
		const childPos = gridPos + 1 + childOffset // +1 for grid's opening tag
		const childDOM = view.nodeDOM(childPos)

		if (childDOM && childDOM instanceof HTMLElement) {
			allBlocks.push({
				pos: childPos,
				node: child,
				dom: childDOM,
				rect: childDOM.getBoundingClientRect(),
			})
		}
	})

	if (allBlocks.length === 0) {
		// Empty grid - return position at start of grid content
		return gridPos + 1
	}

	// Group blocks by row based on their vertical position
	// Blocks are on the same row if their vertical centers are within tolerance
	const ROW_TOLERANCE = 10
	const rows: (typeof allBlocks)[] = []

	for (const block of allBlocks) {
		const blockCenterY = block.rect.top + block.rect.height / 2

		// Find existing row that this block belongs to
		let foundRow = false
		for (const row of rows) {
			const rowStart = row[0]
			if (rowStart) {
				const rowCenterY = rowStart.rect.top + rowStart.rect.height / 2
				if (Math.abs(blockCenterY - rowCenterY) < ROW_TOLERANCE) {
					row.push(block)
					foundRow = true
					break
				}
			}
		}

		if (!foundRow) {
			rows.push([block])
		}
	}

	// Find which row the cursor is in based on clientY
	let targetRow: typeof allBlocks | null = null

	for (const row of rows) {
		if (row.length === 0) continue

		// Get row bounds (min top, max bottom across all blocks in row)
		const rowTop = Math.min(...row.map((b) => b.rect.top))
		const rowBottom = Math.max(...row.map((b) => b.rect.bottom))

		if (clientY >= rowTop && clientY <= rowBottom) {
			targetRow = row
			break
		}
	}

	// If cursor is not directly over any row, find the closest row
	if (!targetRow) {
		let minDistance = Infinity
		for (const row of rows) {
			if (row.length === 0) continue

			const rowTop = Math.min(...row.map((b) => b.rect.top))
			const rowBottom = Math.max(...row.map((b) => b.rect.bottom))
			const rowCenterY = (rowTop + rowBottom) / 2
			const distance = Math.abs(clientY - rowCenterY)

			if (distance < minDistance) {
				minDistance = distance
				targetRow = row
			}
		}
	}

	if (!targetRow || targetRow.length === 0) {
		// Fallback: use all blocks
		targetRow = allBlocks
	}

	// Sort row blocks by X position (left to right)
	targetRow.sort((a, b) => a.rect.left - b.rect.left)

	const firstBlock = targetRow[0]
	const lastBlock = targetRow[targetRow.length - 1]

	// targetRow is never empty here (allBlocks is the non-empty fallback), so
	// this only satisfies noUncheckedIndexedAccess
	if (!firstBlock || !lastBlock) {
		return null
	}

	// Check if cursor is in the left margin (before the first block of this row)
	if (clientX < firstBlock.rect.left) {
		return firstBlock.pos
	}

	// Check if cursor is in the trailing empty space (after the last block of this row)
	if (clientX > lastBlock.rect.right) {
		return lastBlock.pos + lastBlock.node.nodeSize
	}

	// Find which block the cursor is over or between using X-axis midpoint
	for (const [i, block] of targetRow.entries()) {
		const midpointX = block.rect.left + block.rect.width / 2

		// Check if cursor is over this block
		if (clientX >= block.rect.left && clientX <= block.rect.right) {
			// Use X-axis midpoint to determine before or after
			if (clientX < midpointX) {
				return block.pos
			} else {
				return block.pos + block.node.nodeSize
			}
		}

		// Check if cursor is in the gap between this block and the next
		const nextBlock = targetRow[i + 1]
		if (
			nextBlock &&
			clientX > block.rect.right &&
			clientX < nextBlock.rect.left
		) {
			// In the gap - return position after this block (before next)
			return block.pos + block.node.nodeSize
		}
	}

	// Fallback: return position after the last block
	return lastBlock.pos + lastBlock.node.nodeSize
}

// Find the appropriate insert position respecting nesting ("snaps" to either
// before or after the nearest node at some depth).
function findDropPosition(
	view: EditorView,
	clientX: number,
	clientY: number,
): number | null {
	// Check if we're dragging a list item (for normalizing list boundaries)
	const isDraggingListItem = lastParentListType !== null

	// Special handling for MetricBlock: if dragging a MetricBlock and cursor is
	// within a MetricGrid (or in its side margins), use MetricGrid-specific drop logic.
	// includeMargins allows showing cursor at row edges when hovering in editor margins.
	if (isDraggingMetricBlock(view)) {
		const metricGridInfo = findMetricGridAtCoords(view, clientX, clientY, {
			includeMargins: true,
		})

		if (metricGridInfo) {
			const { gridElement, gridPos } = metricGridInfo

			// Check if this is the source grid (the grid we're dragging from)
			// We still allow dropping within the same grid, just not at the exact source position
			const sourceWrapperBounds = getSourceWrapperBounds(view)

			// Get drop position within the grid
			const dropPos = findMetricGridDropPosition(
				view,
				clientX,
				clientY,
				gridPos,
				gridElement,
			)

			if (dropPos === null) {
				return null
			}

			// If this is the source grid, exclude the immediate before/after positions of the dragged block
			if (sourceWrapperBounds) {
				// Get the current selection to find the dragged block's position
				const { selection } = view.state
				if (!selection.empty) {
					const draggedBlockPos = selection.from
					const draggedBlockEnd = selection.to

					// Don't allow dropping at the exact source position (before or after the dragged block)
					if (dropPos === draggedBlockPos || dropPos === draggedBlockEnd) {
						return null
					}
				}
			}

			return dropPos
		}
	}

	// First, check if we're hovering over a gap zone decoration
	const elementAtPoint = document.elementFromPoint(clientX, clientY)

	// Check for gap zones FIRST (they have drag-handle-ignore-self but should still work)
	const gapPos = getGapDropPosition(elementAtPoint, view.state)
	if (gapPos !== null) {
		// Check if this gap is inside a list of a DIFFERENT type than we're dragging
		// If so, calculate position before/after the enclosing list (treat as atomic)
		if (isDraggingListItem) {
			try {
				const $gapPos = view.state.doc.resolve(gapPos)
				for (let d = 1; d <= $gapPos.depth; d++) {
					const node = $gapPos.node(d)
					if (
						LIST_NODE_TYPES.has(node.type.name) &&
						node.type.name !== lastParentListType
					) {
						// Found enclosing list of different type - return before/after it
						const listStart = $gapPos.before(d)
						const listEnd = $gapPos.after(d)
						// Use the list's DOM to determine upper/lower half
						const listDOM = view.nodeDOM(listStart)
						let atomicPos: number
						if (listDOM && listDOM instanceof HTMLElement) {
							const rect = listDOM.getBoundingClientRect()
							const midpoint = rect.top + rect.height / 2
							atomicPos = clientY < midpoint ? listStart : listEnd
						} else {
							// Fallback: use position distance
							atomicPos =
								gapPos - listStart < listEnd - gapPos ? listStart : listEnd
						}
						// Normalize to attach to adjacent same-type list if present
						return normalizeListBoundaryPosition(
							view.state.doc,
							atomicPos,
							isDraggingListItem,
						)
					}
				}
			} catch {
				// ignore
			}
		}

		// Not inside a different list - use the gap position normally
		return normalizeListBoundaryPosition(
			view.state.doc,
			gapPos,
			isDraggingListItem,
		)
	}

	// Don't show drop cursor if hovering directly on an element with drag-handle-ignore class
	// (but not for gap zones, which are handled above)
	if (elementAtPoint?.classList.contains(DRAG_HANDLE_IGNORE_SELF_CLASS)) {
		return null
	}

	// Track the source node's parent container for later validation.
	// We'll use this to prevent dropping at ancestor positions when dragging
	// from inside a container like SplitDocumentationRightSide.
	let sourceContainerStart: number | null = null
	let sourceContainerEnd: number | null = null
	const { selection } = view.state
	if (!selection.empty) {
		const sourceNodePos = selection.from
		try {
			const $sourcePos = view.state.doc.resolve(sourceNodePos)
			// Find the source node's parent container (a node in DRAG_CONTAINER_NODE_TYPES)
			for (let d = $sourcePos.depth; d > 0; d--) {
				const ancestor = $sourcePos.node(d)
				if (DRAG_CONTAINER_NODE_TYPES.has(ancestor.type.name)) {
					sourceContainerStart = $sourcePos.before(d)
					sourceContainerEnd = $sourcePos.after(d)
					break
				}
			}
		} catch {
			// Ignore errors
		}
	}

	const coords = view.posAtCoords({ left: clientX, top: clientY })
	if (!coords) {
		return null
	}

	const { state } = view
	const $pos = state.doc.resolve(coords.pos)

	// Find the deepest composite node (e.g., innermost ListItem for nested lists),
	// or the deepest draggable node if no composite node is found.
	// Once we find a composite node, only other composite nodes can override it
	// (this prevents targeting paragraphs inside list items)
	//
	// When NOT dragging a list item, or when dragging a list item of a DIFFERENT type,
	// treat lists as atomic units - stop at the list level and don't descend into
	// list items. This allows dropping above/below entire lists based on list midpoint.
	let targetDepth = 1 // Default to top-level
	let foundComposite = false

	for (let d = 1; d <= $pos.depth; d++) {
		const node = $pos.node(d)

		// Check if this is a list node
		if (LIST_NODE_TYPES.has(node.type.name)) {
			// Treat list as atomic if:
			// 1. Not dragging a list item at all, OR
			// 2. Dragging a list item but of a different list type
			const isDifferentListType = lastParentListType !== node.type.name
			if (!isDraggingListItem || isDifferentListType) {
				targetDepth = d
				break // Don't go deeper into the list
			}
		}

		if (DRAG_COMPOSITE_DRAG_NODES.has(node.type.name)) {
			// Composite nodes always take precedence - track the deepest one
			targetDepth = d
			foundComposite = true
		} else if (!foundComposite && isDraggableNodeType(node.type.name)) {
			// Only consider non-composite draggables if we haven't found a composite yet
			targetDepth = d
		}
	}

	// Get boundaries at the target depth (before/after the node)
	let before: number
	let after: number

	try {
		before = $pos.before(targetDepth)
		after = $pos.after(targetDepth)
	} catch {
		return normalizeListBoundaryPosition(
			state.doc,
			coords.pos,
			isDraggingListItem,
		)
	}

	// Use the node's DOM element for midpoint calculation
	const nodeDOM = view.nodeDOM(before)

	let result: number
	if (nodeDOM && nodeDOM instanceof HTMLElement) {
		const rect = nodeDOM.getBoundingClientRect()
		const midpoint = rect.top + rect.height / 2
		result = clientY < midpoint ? before : after
	} else {
		result =
			Math.abs(coords.pos - before) <= Math.abs(after - coords.pos)
				? before
				: after
	}

	// If we started from inside a container (like SplitDocumentationRightSide),
	// prevent dropping at positions that would place the node outside that container
	// at an ancestor level. This forces the use of gap zones for reordering within
	// the container, and only allows ancestor drops when genuinely moving outside.
	// We check if the result position is at or outside the container boundaries.
	if (
		sourceContainerStart !== null &&
		sourceContainerEnd !== null &&
		(result <= sourceContainerStart || result >= sourceContainerEnd)
	) {
		return null
	}

	// Normalize list boundary positions to reduce flickering (only for list items)
	return normalizeListBoundaryPosition(state.doc, result, isDraggingListItem)
}

/**
 * Ensure the position is at a valid block boundary for insertion.
 * If the position is inside a text node or inline content, adjust it
 * to be at the proper block boundary.
 */
function ensureBlockBoundary(doc: Node, pos: number): number {
	try {
		const $pos = doc.resolve(pos)

		// If we're at depth 0, we're at the doc level - that's fine
		if ($pos.depth === 0) {
			return pos
		}

		// Check if we're inside a text node (parentOffset > 0 and parent has text content)
		// In that case, we should move to the block boundary
		if ($pos.parentOffset > 0 && $pos.parent.isTextblock) {
			// We're inside a textblock - move to after this block
			return $pos.after($pos.depth)
		}

		// Check if the position is between blocks at the parent level
		// $pos.nodeBefore and $pos.nodeAfter tell us if there are adjacent nodes
		const nodeBefore = $pos.nodeBefore
		const nodeAfter = $pos.nodeAfter

		// If there's no node before or after at this position, we might be at an edge
		// Check if we need to go up a level
		if (!nodeBefore && !nodeAfter && $pos.depth > 1) {
			// We're at an empty position - try the parent level
			if ($pos.index($pos.depth - 1) === 0) {
				return $pos.before($pos.depth)
			}
		}

		return pos
	} catch {
		return pos
	}
}

/**
 * Check if the slice content consists of list item(s) that need wrapping.
 * Returns the list type name to use for wrapping, or null if not list items.
 * If parentListType is provided, uses that; otherwise falls back to default.
 */
function getListWrapperType(
	content: Fragment,
	parentListType?: string | null,
): string | null {
	const firstChild = content.firstChild
	if (!firstChild) {
		return null
	}

	// Check if this is a list item type
	const defaultListType = LIST_ITEM_TO_DEFAULT_LIST_TYPE.get(
		firstChild.type.name,
	)
	if (!defaultListType) {
		return null
	}

	// Verify all children are list item types
	for (let i = 0; i < content.childCount; i++) {
		const child = content.child(i)
		if (!LIST_ITEM_TO_DEFAULT_LIST_TYPE.has(child.type.name)) {
			return null
		}
	}

	// Use the original parent list type if provided, otherwise use the default
	return parentListType ?? defaultListType
}

/**
 * Wraps list items in their appropriate list container if the target position
 * cannot directly accept list items. Returns the content to insert (either
 * wrapped or original).
 */
function wrapListItemsIfNeeded(
	doc: Node,
	schema: Schema,
	insertPos: number,
	content: Fragment,
	parentListType?: string | null,
): Fragment {
	const listTypeName = getListWrapperType(content, parentListType)
	if (!listTypeName) {
		// Not list items, return as-is
		return content
	}

	try {
		const $insert = doc.resolve(insertPos)
		const parent = $insert.parent
		const index = $insert.index()

		// Check if the parent can accept the list items directly
		if (parent.canReplace(index, index, content)) {
			return content
		}

		// Parent can't accept list items directly - wrap them in a list
		const listType = schema.nodes[listTypeName]
		if (!listType) {
			return content
		}

		const wrappedList = listType.create(null, content)
		const wrappedContent = Fragment.from(wrappedList)

		// Verify the wrapped content can be inserted
		if (parent.canReplace(index, index, wrappedContent)) {
			return wrappedContent
		}
	} catch {
		// Fall through to return original content
	}

	return content
}

/**
 * Wraps nodes in their required wrapper (e.g., MetricBlock -> MetricGrid)
 * if the target position cannot directly accept them.
 */
function wrapNodesIfNeeded(
	doc: Node,
	schema: Schema,
	insertPos: number,
	content: Fragment,
): Fragment {
	const firstChild = content.firstChild
	if (!firstChild) {
		return content
	}

	// Check if this node type needs a wrapper
	const wrapperTypeName = NODE_TO_WRAPPER_TYPE.get(firstChild.type.name)
	if (!wrapperTypeName) {
		return content
	}

	// Verify all children are the same type (can be wrapped together)
	for (let i = 0; i < content.childCount; i++) {
		const child = content.child(i)
		if (NODE_TO_WRAPPER_TYPE.get(child.type.name) !== wrapperTypeName) {
			return content
		}
	}

	try {
		const $insert = doc.resolve(insertPos)
		const parent = $insert.parent
		const index = $insert.index()

		// Check if the parent can accept the nodes directly
		if (parent.canReplace(index, index, content)) {
			return content
		}

		// Parent can't accept nodes directly - wrap them
		const wrapperType = schema.nodes[wrapperTypeName]
		if (!wrapperType) {
			return content
		}

		const wrapped = wrapperType.create(null, content)
		const wrappedContent = Fragment.from(wrapped)

		// Verify the wrapped content can be inserted
		if (parent.canReplace(index, index, wrappedContent)) {
			return wrappedContent
		}
	} catch {
		// Fall through to return original content
	}

	return content
}

/**
 * Check if content can be inserted at a position, considering list item and node wrapping.
 */
function canInsertAtWithWrapping(
	state: EditorState,
	insertPos: number,
	slice: Slice | null,
	parentListType?: string | null,
): boolean {
	if (!slice) {
		return true
	}

	try {
		const $insert = state.doc.resolve(insertPos)
		const parent = $insert.parent
		const index = $insert.index()

		// First try direct insertion
		if (parent.canReplace(index, index, slice.content)) {
			return true
		}

		// If direct insertion fails, check if list wrapping would work
		const wrappedListContent = wrapListItemsIfNeeded(
			state.doc,
			state.schema,
			insertPos,
			slice.content,
			parentListType,
		)
		if (wrappedListContent !== slice.content) {
			return parent.canReplace(index, index, wrappedListContent)
		}

		// Check if node wrapping would work (e.g., MetricBlock -> MetricGrid)
		const wrappedNodeContent = wrapNodesIfNeeded(
			state.doc,
			state.schema,
			insertPos,
			slice.content,
		)
		if (wrappedNodeContent !== slice.content) {
			return parent.canReplace(index, index, wrappedNodeContent)
		}

		return false
	} catch {
		return false
	}
}

let cursorEl: HTMLElement | null = null
let lastMouse: { x: number; y: number } | null = null
let lastSlice: Slice | null = null
let lastInsertPos: number | null = null
let lastParentListType: string | null = null
let lastSourceWrapperBounds: { start: number; end: number } | null = null
let dropHandled = false

function ensureCursorElemExists(view: EditorView) {
	if (!cursorEl) {
		cursorEl = document.createElement("div")
		cursorEl.className = "pm-root-dropcursor"

		Object.assign(cursorEl.style, {
			position: "absolute",
			pointerEvents: "none",
			height: "0.1875rem",
			display: "none",
			backgroundColor: "var(--drag-target)",
			borderRadius: "0.125rem",
			zIndex: "50",
		})

		const mount = (view.dom.parentNode as HTMLElement | null) ?? document.body
		mount.appendChild(cursorEl)
	}
}

function hideCursor() {
	if (cursorEl) {
		cursorEl.style.display = "none"
	}
}

// Shared drop execution logic used by both ProseMirror and global handlers.
// Returns true if the drop was executed successfully.
function executeDrop(
	view: EditorView,
	insertPos: number,
	slice: Slice,
	moved: boolean,
	parentListType: string | null,
	onBeforeDrop?: () => void,
): boolean {
	// Guard against double execution when both handlers fire
	if (dropHandled) {
		return false
	}

	dropHandled = true

	const { state, dispatch } = view
	let pos = insertPos
	let tr = state.tr

	if (moved && !state.selection.empty) {
		const selectionFrom = state.selection.from
		const selectionTo = state.selection.to

		// Don't allow dropping inside the selection
		if (pos >= selectionFrom && pos <= selectionTo) {
			return false
		}

		tr = tr.deleteSelection()
		pos = tr.mapping.map(pos)
	}

	// Ensure the insert position is at a proper block boundary
	pos = ensureBlockBoundary(tr.doc, pos)

	// Get the content to insert, wrapping list items if needed
	let contentToInsert = wrapListItemsIfNeeded(
		tr.doc,
		state.schema,
		pos,
		slice.content,
		parentListType,
	)

	// If list wrapping didn't change anything, try node wrapping
	if (contentToInsert === slice.content) {
		contentToInsert = wrapNodesIfNeeded(
			tr.doc,
			state.schema,
			pos,
			slice.content,
		)
	}

	const nodesToInsert: Node[] = []
	for (let i = 0; i < contentToInsert.childCount; i++) {
		nodesToInsert.push(contentToInsert.child(i))
	}

	tr = tr.replaceWith(pos, pos, nodesToInsert)
	tr = deleteEmptyMetricGrids(tr, state.schema)

	const insertedSize = contentToInsert.size
	if (insertedSize > 0) {
		try {
			const $insertPos = tr.doc.resolve(pos + 1)
			tr = tr.setSelection(Selection.near($insertPos, 1))
		} catch {
			tr = tr.setSelection(Selection.atEnd(tr.doc))
		}
	}

	// notify callback if provided
	onBeforeDrop?.()

	dispatch(tr)

	// Clean up after successful drop
	hideCursor()
	lastSlice = null
	lastMouse = null
	lastInsertPos = null
	lastParentListType = null
	lastSourceWrapperBounds = null

	return true
}

// Shared dragover logic used by both ProseMirror and global handlers.
// Validates the drop position and shows/hides the cursor accordingly.
// Returns true if cursor was shown at a valid position.
function updateDragCursor(
	view: EditorView,
	x: number,
	y: number,
	slice: Slice,
): boolean {
	const parentListType = getParentListType(view)
	const sourceWrapperBounds = getSourceWrapperBounds(view)
	const insertPos = findDropPosition(view, x, y)

	if (
		insertPos == null ||
		!canInsertAtWithWrapping(view.state, insertPos, slice, parentListType)
	) {
		hideCursor()
		lastInsertPos = null
		return false
	}

	// Don't allow dropping adjacent to the source wrapper (e.g., MetricGrid)
	if (
		sourceWrapperBounds &&
		(insertPos === sourceWrapperBounds.start ||
			insertPos === sourceWrapperBounds.end)
	) {
		hideCursor()
		lastInsertPos = null
		return false
	}

	const { selection } = view.state
	if (!selection.empty) {
		const selectionFrom = selection.from
		const selectionTo = selection.to

		// Don't allow dropping at the boundaries or inside the dragged content
		if (insertPos >= selectionFrom && insertPos <= selectionTo) {
			hideCursor()
			lastInsertPos = null
			return false
		}
	}

	lastInsertPos = insertPos
	showCursorAt(view, insertPos, y)
	return true
}

function showCursorAt(view: EditorView, pos: number, clientY?: number) {
	ensureCursorElemExists(view)

	const editorRect = view.dom.getBoundingClientRect()
	const $pos = view.state.doc.resolve(pos)
	const isAfterLastChild = $pos.index($pos.depth) === $pos.parent.childCount

	// Check if this position is inside a MetricGrid - if so, render vertical cursor
	const isInsideMetricGrid = $pos.parent.type.name === MetricGrid.name

	if (isInsideMetricGrid) {
		// Render vertical cursor for MetricGrid
		showVerticalCursorAt(view, pos, $pos, editorRect, clientY)
		return
	}

	// Find the adjacent node to determine the offset
	let adjacentNodeType: string | null = null
	if (isAfterLastChild && $pos.parent.childCount > 0) {
		// Get the last child's type
		adjacentNodeType = $pos.parent.child($pos.parent.childCount - 1).type.name
	} else if ($pos.index($pos.depth) < $pos.parent.childCount) {
		// Get the next node's type
		adjacentNodeType = $pos.parent.child($pos.index($pos.depth)).type.name
	}

	// Calculate indentation level by counting list ancestors
	let indentationLevel = 0
	for (let d = 1; d <= $pos.depth; d++) {
		if (LIST_NODE_TYPES.has($pos.node(d).type.name)) {
			indentationLevel++
		}
	}
	// Subtract 1 because the first list doesn't count as indented
	indentationLevel = Math.max(0, indentationLevel - 1)

	const cursorOffset = cursorOffsetByType(adjacentNodeType, indentationLevel)
	const offset = isAfterLastChild ? cursorOffset.after : cursorOffset.before

	let left = editorRect.left
	let width = editorRect.right - editorRect.left
	let top: number

	// Check if there's a gap zone at this position - use its siblings for positioning
	const gapElements = findGapElementsByPos(view.state, pos)
	const gapElement = gapElements[0] ?? null

	if (gapElement) {
		// Use the gap element's wrapper's siblings to find adjacent nodes
		const wrapper = gapElement.parentElement
		const prevSibling = wrapper?.previousElementSibling as HTMLElement | null
		const nextSibling = wrapper?.nextElementSibling as HTMLElement | null

		if (isAfterLastChild && prevSibling) {
			// After last child: position below the previous sibling
			const rect = prevSibling.getBoundingClientRect()
			left = rect.left
			width = rect.width
			top = rect.bottom - editorRect.top - offset
		} else if (nextSibling) {
			// Before a node: position above the next sibling
			const rect = nextSibling.getBoundingClientRect()
			left = rect.left
			width = rect.width
			top = rect.top - editorRect.top - offset
		} else {
			// Fallback to gap element center
			const gapRect = gapElement.getBoundingClientRect()
			left = gapRect.left
			width = gapRect.width
			top = gapRect.top + gapRect.height / 2 - editorRect.top
		}
	} else {
		// No gap zone - use coordsAtPos
		const coords = view.coordsAtPos(pos)
		top = coords.top - editorRect.top - offset

		try {
			// Find the adjacent node for horizontal positioning
			// If we're after the last child, use the last child
			// Otherwise, use the next child at this position
			let nodePos: number
			if (isAfterLastChild && $pos.parent.childCount > 0) {
				// Get position of the last child
				const lastChildIndex = $pos.parent.childCount - 1
				nodePos = $pos.posAtIndex(lastChildIndex)
			} else if ($pos.index($pos.depth) < $pos.parent.childCount) {
				// Get position of the next child
				nodePos = $pos.posAtIndex($pos.index($pos.depth))
			} else {
				// Fallback to current depth
				const depth = Math.max(1, $pos.depth)
				nodePos = $pos.before(depth)
			}

			const nodeDOM = view.nodeDOM(nodePos)

			if (nodeDOM && nodeDOM instanceof HTMLElement) {
				const nodeRect = nodeDOM.getBoundingClientRect()
				left = nodeRect.left
				width = nodeRect.width
			}
		} catch {
			// use defaults
		}
	}

	// Apply horizontal insets
	const leftInset = cursorOffset.left ?? 0
	const rightInset = cursorOffset.right ?? 0
	left = left + leftInset
	width = width - leftInset - rightInset

	// Reset to horizontal cursor style
	// eslint-disable-next-line @typescript-eslint/no-non-null-assertion -- showCursorAt calls ensureCursorElemExists before reaching this point
	Object.assign(cursorEl!.style, {
		top: `${top}px`,
		left: `${left - editorRect.left}px`,
		width: `${width}px`,
		height: "0.1875rem",
		display: "block",
	})
}

/**
 * Show a vertical cursor for MetricGrid drop positions.
 * The cursor is positioned between MetricBlock children using X-axis positioning.
 * For multi-row grids, positions at the correct Y coordinate.
 * Height is determined by:
 * - Before first child: height of first child
 * - In the middle: height of tallest child (or adjacent block at row boundaries)
 * - After last child: height of last child
 *
 * @param clientY - Mouse Y position, used to determine which row to show cursor on at boundaries
 */
function showVerticalCursorAt(
	view: EditorView,
	_pos: number,
	$pos: ResolvedPos,
	editorRect: DOMRect,
	clientY?: number,
) {
	const childIndex = $pos.index($pos.depth)
	const isAfterLastChild = childIndex === $pos.parent.childCount
	const isBeforeFirstChild = childIndex === 0 && !isAfterLastChild
	const gridNode = $pos.parent

	// Find the grid DOM element
	let gridDOM: HTMLElement | null = null
	try {
		const gridPos = $pos.before($pos.depth)
		gridDOM = view.nodeDOM(gridPos) as HTMLElement | null
	} catch {
		// Ignore
	}

	const DEFAULT_HEIGHT = 0

	// Calculate cursor height and top position based on position context
	let cursorHeight: number = DEFAULT_HEIGHT
	let cursorTop: number = gridDOM?.getBoundingClientRect().top ?? editorRect.top
	// Track if we're at a row boundary and should show cursor on the previous row
	let cursorOnPrevRow = false

	if (gridDOM && gridNode.childCount > 0) {
		if (isBeforeFirstChild) {
			// Before first child: use height and position of first child
			const firstChildPos = $pos.posAtIndex(0)
			const firstChildDOM = view.nodeDOM(firstChildPos) as HTMLElement | null
			if (firstChildDOM) {
				const rect = firstChildDOM.getBoundingClientRect()
				cursorHeight = rect.height
				cursorTop = rect.top
			}
		} else if (isAfterLastChild) {
			// After last child: use height and position of last child
			const lastChildIndex = gridNode.childCount - 1
			const lastChildPos = $pos.posAtIndex(lastChildIndex)
			const lastChildDOM = view.nodeDOM(lastChildPos) as HTMLElement | null
			if (lastChildDOM) {
				const rect = lastChildDOM.getBoundingClientRect()
				cursorHeight = rect.height
				cursorTop = rect.top
			}
		} else {
			// In the middle: check if this is a row boundary
			const prevChildPos = $pos.posAtIndex(childIndex - 1)
			const prevChildDOM = view.nodeDOM(prevChildPos) as HTMLElement | null
			const nextChildPos = $pos.posAtIndex(childIndex)
			const nextChildDOM = view.nodeDOM(nextChildPos) as HTMLElement | null

			if (prevChildDOM && nextChildDOM) {
				const prevRect = prevChildDOM.getBoundingClientRect()
				const nextRect = nextChildDOM.getBoundingClientRect()

				// Check if this is a row boundary (next is below prev)
				const isRowBoundary = nextRect.top > prevRect.bottom - 10

				if (isRowBoundary && clientY !== undefined) {
					// At a row boundary - use clientY to decide which row
					const rowGapMidpoint = (prevRect.bottom + nextRect.top) / 2

					if (clientY < rowGapMidpoint) {
						// Mouse is on the previous row - show cursor after prev child
						cursorTop = prevRect.top
						cursorHeight = prevRect.height
						cursorOnPrevRow = true
					} else {
						// Mouse is on the next row - show cursor before next child
						cursorTop = nextRect.top
						// Use tallest child height for middle positions
						let maxHeight = 0
						for (let i = 0; i < gridNode.childCount; i++) {
							const cPos = $pos.posAtIndex(i)
							const cDOM = view.nodeDOM(cPos) as HTMLElement | null
							if (cDOM) {
								maxHeight = Math.max(
									maxHeight,
									cDOM.getBoundingClientRect().height,
								)
							}
						}
						cursorHeight = maxHeight > 0 ? maxHeight : DEFAULT_HEIGHT
					}
				} else {
					// Not a row boundary or no clientY - position at next child's row
					cursorTop = nextRect.top
					let maxHeight = 0
					for (let i = 0; i < gridNode.childCount; i++) {
						const cPos = $pos.posAtIndex(i)
						const cDOM = view.nodeDOM(cPos) as HTMLElement | null
						if (cDOM) {
							maxHeight = Math.max(
								maxHeight,
								cDOM.getBoundingClientRect().height,
							)
						}
					}
					cursorHeight = maxHeight > 0 ? maxHeight : DEFAULT_HEIGHT
				}
			} else if (nextChildDOM) {
				cursorTop = nextChildDOM.getBoundingClientRect().top
				let maxHeight = 0
				for (let i = 0; i < gridNode.childCount; i++) {
					const cPos = $pos.posAtIndex(i)
					const cDOM = view.nodeDOM(cPos) as HTMLElement | null
					if (cDOM) {
						maxHeight = Math.max(maxHeight, cDOM.getBoundingClientRect().height)
					}
				}
				cursorHeight = maxHeight > 0 ? maxHeight : DEFAULT_HEIGHT
			}
		}
	}

	let cursorLeft: number

	// Try to use gap zone for positioning (maintains consistent offsets)
	// For row boundaries, look for the secondary gap element
	const gapElements = findGapElementsByPos(view.state, _pos)

	let targetGapElement: HTMLElement | null = null
	for (const gapEl of gapElements) {
		if (gapEl.getAttribute("data-gap-orientation") !== "vertical") continue
		if (gapEl.style.display === "none") continue

		const isSecondary = gapEl.classList.contains("pm-gap-zone-secondary")

		// For cursorOnPrevRow, prefer the secondary gap (end of prev row)
		// Otherwise prefer the primary gap
		if (cursorOnPrevRow && isSecondary) {
			targetGapElement = gapEl
			break
		} else if (!cursorOnPrevRow && !isSecondary) {
			targetGapElement = gapEl
			break
		}
		// Fallback: use any gap we find
		targetGapElement ??= gapEl
	}

	if (targetGapElement) {
		// Use the gap element's center position
		const gapRect = targetGapElement.getBoundingClientRect()
		cursorLeft = gapRect.left + gapRect.width / 2 - 1.5 // Center the 3px cursor
	} else {
		// Fallback: Calculate position based on adjacent blocks
		if (cursorOnPrevRow && childIndex > 0) {
			// At row boundary, cursor should be after the previous child (end of row)
			const prevChildPos = $pos.posAtIndex(childIndex - 1)
			const prevChildDOM = view.nodeDOM(prevChildPos) as HTMLElement | null

			if (prevChildDOM) {
				const rect = prevChildDOM.getBoundingClientRect()
				cursorLeft = rect.right + 6 - 1.5 // Center in the gap (12px gap, 3px cursor)
			} else {
				cursorLeft = editorRect.left
			}
		} else if (isAfterLastChild && gridNode.childCount > 0) {
			// After last child - position after the last block
			const lastChildIndex = gridNode.childCount - 1
			const lastChildPos = $pos.posAtIndex(lastChildIndex)
			const lastChildDOM = view.nodeDOM(lastChildPos) as HTMLElement | null

			if (lastChildDOM) {
				const rect = lastChildDOM.getBoundingClientRect()
				cursorLeft = rect.right + 6 - 1.5 // Center in the gap (12px gap, 3px cursor)
			} else {
				cursorLeft = editorRect.left
			}
		} else if (childIndex < gridNode.childCount) {
			// Before a child - position before the next block
			const nextChildPos = $pos.posAtIndex(childIndex)
			const nextChildDOM = view.nodeDOM(nextChildPos) as HTMLElement | null

			if (nextChildDOM) {
				const rect = nextChildDOM.getBoundingClientRect()
				cursorLeft = rect.left - 6 - 1.5 // Center in the gap (12px gap, 3px cursor)
			} else {
				cursorLeft = editorRect.left
			}
		} else {
			// Fallback
			cursorLeft = editorRect.left
		}
	}

	// Apply vertical cursor style
	// eslint-disable-next-line @typescript-eslint/no-non-null-assertion -- showCursorAt calls ensureCursorElemExists before delegating here
	Object.assign(cursorEl!.style, {
		top: `${cursorTop - editorRect.top}px`,
		left: `${cursorLeft - editorRect.left}px`,
		width: "0.1875rem", // 3px width (same as horizontal height)
		height: `${cursorHeight}px`,
		display: "block",
	})
}

// Helper to get the current drag slice from either lastSlice or view.dragging
function getCurrentSlice(view: EditorView): Slice | null {
	// First check view.dragging (set by drag handle)
	if (view.dragging?.slice) {
		return view.dragging.slice
	}
	// Fall back to lastSlice (set by transformPasted for external content)
	return lastSlice
}

// Helper to get the parent list type from view.dragging (if available)
// Also updates lastParentListType for use during drop when view.dragging may be cleared
function getParentListType(view: EditorView): string | null {
	const parentListType = getDragging(view)?.parentListType ?? null
	// Always update lastParentListType when we have drag info from view.dragging
	// This ensures it's set to null when dragging non-list items
	if (view.dragging !== null) {
		lastParentListType = parentListType
	}
	return lastParentListType
}

// Helper to get the source wrapper bounds from view.dragging (if available)
// Also updates lastSourceWrapperBounds for use during drop when view.dragging may be cleared
function getSourceWrapperBounds(
	view: EditorView,
): { start: number; end: number } | null {
	const sourceWrapperBounds = getDragging(view)?.sourceWrapperBounds ?? null
	if (view.dragging !== null) {
		lastSourceWrapperBounds = sourceWrapperBounds
	}
	return lastSourceWrapperBounds
}

export interface DragOptions {
	onBeforeDrop?: () => void
}

export const Drag = Extension.create<DragOptions>({
	name: "drag",
	addOptions() {
		return {
			onBeforeDrop: undefined,
		}
	},
	addProseMirrorPlugins() {
		const { onBeforeDrop } = this.options
		return [
			new Plugin({
				props: {
					handleDOMEvents: {
						dragover(view, e) {
							// Reset dropHandled at drag start to allow the next drop
							if (!lastMouse) {
								dropHandled = false
							}

							lastMouse = { x: e.clientX, y: e.clientY }

							repositionGapZones()

							const slice = getCurrentSlice(view)
							if (slice) {
								updateDragCursor(view, e.clientX, e.clientY, slice)
							}

							return false
						},
						dragend() {
							hideCursor()
							clearRepositionCooldown()

							lastSlice = null
							lastMouse = null
							lastInsertPos = null
							lastParentListType = null
							lastSourceWrapperBounds = null
							dropHandled = false

							return false
						},
						drop(_view) {
							hideCursor()
							clearRepositionCooldown()

							lastSlice = null
							lastMouse = null
							lastInsertPos = null
							// Note: Don't clear lastParentListType or lastSourceWrapperBounds here - handleDrop needs them
							// They will be cleared after handleDrop completes

							return false
						},
						dragleave(view, e) {
							const editorElement = view.dom
							// not the imported prosemirror Node: this file shadows the DOM
							// one, and contains() needs the DOM node
							const relatedTarget = e.relatedTarget as HTMLElement | null

							if (!relatedTarget || !editorElement.contains(relatedTarget)) {
								hideCursor()
								lastInsertPos = null
							}

							return false
						},
					},
					handleDrop(view, e, slice, moved) {
						let insertPos = lastInsertPos

						insertPos ??= findDropPosition(view, e.clientX, e.clientY)

						// Use the slice from view.dragging if available (internal drag)
						// otherwise use the slice passed to handleDrop (external drop)
						const actualSlice = view.dragging?.slice ?? slice

						// Get the parent list type from the drag data (if available)
						const parentListType = getParentListType(view)
						const sourceWrapperBounds = getSourceWrapperBounds(view)

						if (
							insertPos == null ||
							!canInsertAtWithWrapping(
								view.state,
								insertPos,
								actualSlice,
								parentListType,
							)
						) {
							e.preventDefault()
							return true
						}

						// Don't allow dropping adjacent to the source wrapper (e.g., MetricGrid)
						if (
							sourceWrapperBounds &&
							(insertPos === sourceWrapperBounds.start ||
								insertPos === sourceWrapperBounds.end)
						) {
							e.preventDefault()
							return true
						}

						const executed = executeDrop(
							view,
							insertPos,
							actualSlice,
							moved,
							parentListType,
							onBeforeDrop,
						)

						e.preventDefault()
						return executed
					},
					transformPasted(slice) {
						lastSlice = slice
						return slice
					},
				},
				view(view) {
					ensureCursorElemExists(view)

					const handleGlobalMouseUp = () => {
						if (cursorEl && cursorEl.style.display !== "none") {
							hideCursor()
							lastSlice = null
							lastMouse = null
							lastInsertPos = null
							lastParentListType = null
							lastSourceWrapperBounds = null
							dropHandled = false
						}
					}

					// Why we need TWO dragover handlers (ProseMirror + global):
					//
					// ProseMirror's handleDOMEvents.dragover only receives events that bubble
					// through its DOM tree. When dragging over NodeViews with contenteditable="false"
					// (like MetricBlock), the browser handles drag events differently and they don't
					// reliably propagate to ProseMirror's handlers.
					//
					// The ProseMirror handler (above) is responsible for:
					// - Handling dragover events that successfully propagate through ProseMirror's DOM
					// - Showing the cursor on first valid position (when events reach it)
					//
					// The global handler is responsible for:
					// - Initializing AND updating cursor when events miss ProseMirror (e.g., over MetricBlocks)
					// - Always tracking lastMouse position for accurate drop placement
					// - Calling preventDefault() on dragover to allow drops
					//
					// Both dragover handlers may fire for the same event - this is harmless as
					// showCursorAt just overwrites CSS styles.
					//
					// For DROP events, both handlers use the shared executeDrop function which has
					// a guard to prevent double execution.
					const handleGlobalDragOver = (e: DragEvent) => {
						// Reset dropHandled at drag start to allow the next drop
						// (mirrors the logic in ProseMirror's dragover handler)
						if (!lastMouse) {
							dropHandled = false
						}

						// Always track mouse position for accurate drop placement
						lastMouse = { x: e.clientX, y: e.clientY }

						// Check if there's an active drag with valid content
						// view.dragging is set by ProseMirror when a drag starts from within the editor
						const slice = getCurrentSlice(view)
						if (!slice) return

						// Ensure cursor element exists
						ensureCursorElemExists(view)

						const shown = updateDragCursor(view, e.clientX, e.clientY, slice)

						// CRITICAL: Must preventDefault on dragover to allow global drop
						// Without this, the browser rejects drop events on this element
						if (shown) {
							e.preventDefault()
						}
					}

					// Global drop handler for the same reason as global dragover - ProseMirror's
					// handleDrop doesn't receive events when dropping over contenteditable="false" NodeViews.
					// Uses the same executeDrop function as the ProseMirror handler, with a guard to
					// prevent double execution if both handlers fire.
					const handleGlobalDrop = (e: DragEvent) => {
						// Only handle if we have an active drag with a valid insert position
						if (
							lastInsertPos == null ||
							!cursorEl ||
							cursorEl.style.display === "none"
						) {
							return
						}

						const slice = getCurrentSlice(view)
						if (!slice) {
							return
						}

						const parentListType = getParentListType(view)
						const sourceWrapperBounds = getSourceWrapperBounds(view)

						if (
							!canInsertAtWithWrapping(
								view.state,
								lastInsertPos,
								slice,
								parentListType,
							)
						) {
							return
						}

						if (
							sourceWrapperBounds &&
							(lastInsertPos === sourceWrapperBounds.start ||
								lastInsertPos === sourceWrapperBounds.end)
						) {
							return
						}

						// Check if this is a move (internal drag with selection)
						const moved = !!view.dragging?.move && !view.state.selection.empty

						// Don't reset dropHandled here - let ProseMirror handler go first if it fires
						executeDrop(
							view,
							lastInsertPos,
							slice,
							moved,
							parentListType,
							onBeforeDrop,
						)

						e.preventDefault()
					}

					document.addEventListener("mouseup", handleGlobalMouseUp)
					document.addEventListener("dragover", handleGlobalDragOver)
					document.addEventListener("drop", handleGlobalDrop)

					return {
						update() {
							if (lastMouse && cursorEl && cursorEl.style.display !== "none") {
								const slice = getCurrentSlice(view)
								const parentListType = getParentListType(view)
								const sourceWrapperBounds = getSourceWrapperBounds(view)
								const insertPos = findDropPosition(
									view,
									lastMouse.x,
									lastMouse.y,
								)

								if (
									insertPos == null ||
									!canInsertAtWithWrapping(
										view.state,
										insertPos,
										slice,
										parentListType,
									)
								) {
									hideCursor()
									lastInsertPos = null
									return
								}

								// Don't allow dropping adjacent to the source wrapper
								if (
									sourceWrapperBounds &&
									(insertPos === sourceWrapperBounds.start ||
										insertPos === sourceWrapperBounds.end)
								) {
									hideCursor()
									lastInsertPos = null
									return
								}

								const { selection } = view.state
								if (!selection.empty) {
									const selectionFrom = selection.from
									const selectionTo = selection.to

									if (
										insertPos === selectionFrom ||
										insertPos === selectionTo
									) {
										hideCursor()
										lastInsertPos = null
										return
									}
								}

								lastInsertPos = insertPos
								showCursorAt(view, insertPos, lastMouse.y)
							}
						},
						destroy() {
							document.removeEventListener("mouseup", handleGlobalMouseUp)
							document.removeEventListener("dragover", handleGlobalDragOver)
							document.removeEventListener("drop", handleGlobalDrop)

							if (cursorEl?.parentNode) {
								cursorEl.parentNode.removeChild(cursorEl)
							}

							cursorEl = null
							lastSlice = null
							lastMouse = null
							lastInsertPos = null
						},
					}
				},
			}),
		]
	},
})
