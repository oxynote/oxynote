import { Extension, isiOS, isMacOS } from "@tiptap/core"
import { type EditorState, Plugin, Selection } from "prosemirror-state"
import { Fragment, type Node, type Schema, type Slice } from "prosemirror-model"
import type { EditorView } from "@tiptap/pm/view"
import { isDraggableNodeType } from "./node-detection"
import {
	getGapDropPosition,
	findGapElementByPos,
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
import { deleteEmptyBlockGrids, withoutGridPlaces } from "../blocks/grid"
import { BLOCK_GRID_NAMES } from "../blocks/grid/utils"
import {
	executeGridDrop,
	findGridDrop,
	type GridDrop,
	gridDropLineStyle,
} from "./grid-drop"

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

// Find the appropriate insert position respecting nesting ("snaps" to either
// before or after the nearest node at some depth).
function findDropPosition(
	view: EditorView,
	clientX: number,
	clientY: number,
): number | null {
	// Check if we're dragging a list item (for normalizing list boundaries)
	const isDraggingListItem = lastParentListType !== null

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

	// an atom block in the container the drag started from, such as a
	// chart in a split documentation's right side, is a target of its own:
	// the cursor's height over it picks before or after. The search below
	// would climb past the container, which the check further down then
	// refuses.
	const atom = coords.inside >= 0 ? state.doc.nodeAt(coords.inside) : null
	if (atom?.isAtom && sourceContainerStart !== null) {
		const $atom = state.doc.resolve(coords.inside)
		const atomDOM = view.nodeDOM(coords.inside)

		if (
			$atom.depth > 0 &&
			$atom.before() === sourceContainerStart &&
			atomDOM instanceof HTMLElement
		) {
			const rect = atomDOM.getBoundingClientRect()

			return clientY < rect.top + rect.height / 2
				? coords.inside
				: coords.inside + atom.nodeSize
		}
	}

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
 * Wraps nodes in their required wrapper (e.g., grid item -> block grid)
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

		// Check if node wrapping would work (e.g., grid item -> block grid)
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
// the pointer, and whether a drop there moves the dragged content
let lastMouse: { x: number; y: number; moves: boolean } | null = null
let lastSlice: Slice | null = null
let lastInsertPos: number | null = null
let lastParentListType: string | null = null
let lastSourceWrapperBounds: { start: number; end: number } | null = null
let dropHandled = false
// the inputs of the last grid drop search and the drop it found. Both
// dragover handlers search on every event. The browser also repeats
// dragover while the pointer rests. So a search with the same inputs
// reuses the last drop.
let lastGridQuery: {
	doc: Node
	slice: Slice | null
	moving: boolean
	x: number
	y: number
	top: number
	drop: GridDrop | null | undefined
} | null = null

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

// endDrag hides the cursor and clears the state kept during the drag
function endDrag() {
	hideCursor()
	lastSlice = null
	lastMouse = null
	lastInsertPos = null
	lastParentListType = null
	lastSourceWrapperBounds = null
	lastGridQuery = null
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

	// a grid block dropped here does not keep its old cells, so it takes
	// the first free cells where it lands
	const content = withoutGridPlaces(
		slice.content,
		BLOCK_GRID_NAMES.has(tr.doc.resolve(pos).parent.type.name),
	)

	// Get the content to insert, wrapping list items if needed
	let contentToInsert = wrapListItemsIfNeeded(
		tr.doc,
		state.schema,
		pos,
		content,
		parentListType,
	)

	// If list wrapping didn't change anything, try node wrapping
	if (contentToInsert === content) {
		contentToInsert = wrapNodesIfNeeded(tr.doc, state.schema, pos, content)
	}

	const nodesToInsert: Node[] = []
	for (let i = 0; i < contentToInsert.childCount; i++) {
		nodesToInsert.push(contentToInsert.child(i))
	}

	tr = tr.replaceWith(pos, pos, nodesToInsert)
	tr = deleteEmptyBlockGrids(tr)

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

	endDrag()

	return true
}

// dragMoves tells whether a drop moves the dragged content or copies it.
// Holding the copy key makes it copy. ProseMirror decides the same way on
// drop.
function dragMoves(view: EditorView, event: DragEvent): boolean {
	const copies = isMacOS() || isiOS() ? event.altKey : event.ctrlKey

	return !!view.dragging && !copies
}

// Shared dragover logic used by both ProseMirror and global handlers.
// Validates the drop position and shows/hides the cursor accordingly.
// Returns true if cursor was shown at a valid position.
function updateDragCursor(
	view: EditorView,
	x: number,
	y: number,
	slice: Slice,
	moves: boolean,
): boolean {
	const drop = showGridDrop(view, x, y, slice, moves)
	if (drop !== undefined) {
		return drop !== null
	}

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

	// Don't allow dropping adjacent to the source wrapper (e.g., block grid)
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
	showCursorAt(view, insertPos)
	return true
}

// showGridDrop shows the line of a grid drop at the point. It returns
// undefined when the point is over no grid or no grid block is dragged,
// so the caller looks for an ordinary drop position instead.
function showGridDrop(
	view: EditorView,
	x: number,
	y: number,
	slice: Slice | null,
	moving: boolean,
): GridDrop | null | undefined {
	// the editor's top moves when the page scrolls under a resting pointer
	const top = view.dom.getBoundingClientRect().top
	const query = lastGridQuery
	const drop =
		query?.doc === view.state.doc &&
		query.slice === slice &&
		query.moving === moving &&
		query.x === x &&
		query.y === y &&
		query.top === top
			? query.drop
			: findGridDrop(view, x, y, slice, moving)

	lastGridQuery = { doc: view.state.doc, slice, moving, x, y, top, drop }

	if (drop === undefined) {
		return undefined
	}

	lastInsertPos = null

	if (!drop) {
		hideCursor()

		return null
	}

	ensureCursorElemExists(view)

	if (cursorEl) {
		Object.assign(
			cursorEl.style,
			gridDropLineStyle(drop.line, view.dom.getBoundingClientRect()),
		)
	}

	return drop
}

// runGridDrop writes the grid drop at the point, once per drag. It returns
// undefined when the point is over no grid or no grid block is dragged.
function runGridDrop(
	view: EditorView,
	x: number,
	y: number,
	slice: Slice,
	moved: boolean,
	onBeforeDrop?: () => void,
): boolean | undefined {
	const drop = findGridDrop(view, x, y, slice, moved)

	if (drop === undefined) {
		return undefined
	}

	if (!drop || dropHandled) {
		return false
	}

	dropHandled = true
	onBeforeDrop?.()

	const executed = executeGridDrop(view, drop, slice, moved)

	endDrag()

	return executed
}

function showCursorAt(view: EditorView, pos: number) {
	ensureCursorElemExists(view)

	const editorRect = view.dom.getBoundingClientRect()
	const $pos = view.state.doc.resolve(pos)
	const isAfterLastChild = $pos.index($pos.depth) === $pos.parent.childCount

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
	const gapElement = findGapElementByPos(view.state, pos)

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

							lastMouse = {
								x: e.clientX,
								y: e.clientY,
								moves: dragMoves(view, e),
							}

							repositionGapZones()

							const slice = getCurrentSlice(view)
							if (slice) {
								updateDragCursor(
									view,
									e.clientX,
									e.clientY,
									slice,
									lastMouse.moves,
								)
							}

							return false
						},
						dragend() {
							endDrag()
							clearRepositionCooldown()
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
								lastGridQuery = null
							}

							return false
						},
					},
					handleDrop(view, e, slice, moved) {
						const gridDropped = runGridDrop(
							view,
							e.clientX,
							e.clientY,
							view.dragging?.slice ?? slice,
							moved,
							onBeforeDrop,
						)

						if (gridDropped !== undefined) {
							e.preventDefault()

							return true
						}

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

						// Don't allow dropping adjacent to the source wrapper (e.g., block grid)
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
						// a moved block that lands on itself moves nothing. Left to
						// prosemirror, the drop would still delete and reinsert it.
						return executed || moved
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
							endDrag()
							dropHandled = false
						}
					}

					// Why we need TWO dragover handlers (ProseMirror + global):
					//
					// ProseMirror's handleDOMEvents.dragover only receives events that bubble
					// through its DOM tree. When dragging over NodeViews with contenteditable="false"
					// (like a grid item), the browser handles drag events differently and they don't
					// reliably propagate to ProseMirror's handlers.
					//
					// The ProseMirror handler (above) is responsible for:
					// - Handling dragover events that successfully propagate through ProseMirror's DOM
					// - Showing the cursor on first valid position (when events reach it)
					//
					// The global handler is responsible for:
					// - Initializing AND updating cursor when events miss ProseMirror (e.g., over grid items)
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
						lastMouse = {
							x: e.clientX,
							y: e.clientY,
							moves: dragMoves(view, e),
						}

						// Check if there's an active drag with valid content
						// view.dragging is set by ProseMirror when a drag starts from within the editor
						const slice = getCurrentSlice(view)
						if (!slice) return

						// Ensure cursor element exists
						ensureCursorElemExists(view)

						const shown = updateDragCursor(
							view,
							e.clientX,
							e.clientY,
							slice,
							lastMouse.moves,
						)

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
						const gridSlice = getCurrentSlice(view)

						if (
							lastGridQuery?.drop &&
							gridSlice &&
							cursorEl?.style.display !== "none"
						) {
							runGridDrop(
								view,
								e.clientX,
								e.clientY,
								gridSlice,
								dragMoves(view, e) && !view.state.selection.empty,
								onBeforeDrop,
							)
							e.preventDefault()

							return
						}

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

								if (
									showGridDrop(
										view,
										lastMouse.x,
										lastMouse.y,
										slice,
										lastMouse.moves,
									) !== undefined
								) {
									return
								}
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
								showCursorAt(view, insertPos)
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
