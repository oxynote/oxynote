import type { Editor } from "@tiptap/core"
import type { ResolvedPos, Node } from "@tiptap/pm/model"
import type { EditorView } from "@tiptap/pm/view"
import type { EditorState } from "@tiptap/pm/state"
import {
	DRAG_DISABLED_EXCEPT_RULES,
	DRAG_HANDLE_APPROX_WIDTH_PX,
	DRAG_HANDLE_IGNORE_CLASS,
	DRAG_HANDLE_IGNORE_SELF_CLASS,
	DRAGGABLE_NODE_TYPES,
} from "./config"

export function isDraggableNodeType(typeName: string): boolean {
	return DRAGGABLE_NODE_TYPES.has(typeName)
}

export function shouldShowDragHandle(
	$pos: ResolvedPos,
	depth: number,
	nodeTypeName?: string,
): boolean {
	const typeName = nodeTypeName ?? $pos.node(depth).type.name

	// check if ancestor nodes disable drag handles for their descendants
	for (let d = Math.min(depth - 1, $pos.depth); d > 0; d--) {
		const allowedTypes = DRAG_DISABLED_EXCEPT_RULES.get($pos.node(d).type.name)
		if (allowedTypes && !allowedTypes.has(typeName)) {
			return false
		}
	}
	return true
}

export interface DraggableNodeResult {
	node: Node
	pos: number
	depth: number
	dom: HTMLElement
}

export interface FindDraggableNodeOptions {
	leftMarginWidth?: number
}

/**
 * Get element's vertical bounds including margins.
 */
function findVerticalBounds(el: HTMLElement): { top: number; bottom: number } {
	const rect = el.getBoundingClientRect()
	const style = getComputedStyle(el)
	return {
		top: rect.top - (parseFloat(style.marginTop) || 0),
		bottom: rect.bottom + (parseFloat(style.marginBottom) || 0),
	}
}

/**
 * Check if any ignored element contains this Y coordinate.
 */
function isIgnoredAtY(container: Element, y: number): boolean {
	const ignored = container.getElementsByClassName(DRAG_HANDLE_IGNORE_CLASS)
	for (let i = 0; i < ignored.length; i++) {
		const bounds = findVerticalBounds(ignored[i] as HTMLElement)
		if (y >= bounds.top && y <= bounds.bottom) {
			return true
		}
	}
	return false
}

const HR_HIT_THRESHOLD_PX = 8

/**
 * <hr> elements are only 1px tall so elementFromPoint and margin-based
 * detection almost always miss them. Scan all <hr> elements in the editor
 * and return one whose center is within threshold of Y.
 */
function findNearbyHr(
	view: EditorView,
	state: EditorState,
	y: number,
): DraggableNodeResult | null {
	const hrs = view.dom.querySelectorAll("hr")

	for (const hr of hrs) {
		const rect = hr.getBoundingClientRect()
		const hrCenter = (rect.top + rect.bottom) / 2

		if (Math.abs(y - hrCenter) <= HR_HIT_THRESHOLD_PX) {
			return findFromElement(view, state, hr)
		}
	}

	return null
}

/**
 * Check if element or ancestor has ignore class.
 */
function hasIgnoreClass(element: Element, stopAt: Element): boolean {
	let current: Element | null = element
	while (current && current !== stopAt) {
		if (current.classList.contains(DRAG_HANDLE_IGNORE_CLASS)) {
			return true
		}
		current = current.parentElement
	}
	return false
}

/**
 * List container elements that should always be ignored for drag handle detection.
 * These are ProseMirror's default DOM output for list nodes - you never want to
 * show a drag handle for the list container itself, only for list items.
 * Note: tagName returns uppercase in HTML.
 */
const IGNORED_LIST_CONTAINER_TAGS = new Set(["UL", "OL"])

/**
 * List item elements. When doing Y-based detection (margin hover), we prefer
 * the deepest list item at a given Y position.
 */
const LIST_ITEM_TAGS = new Set(["LI"])

/**
 * Check if the Y position is in the "trailing margin" of a list item -
 * the area below all its actual content (including nested lists).
 * We want to ignore this area for parent list items to avoid jarring
 * drag handle jumps when moving out of nested lists.
 */
function isInListItemTrailingMargin(li: HTMLElement, y: number): boolean {
	// Check if this list item has any nested ul/ol anywhere within it
	// (not just direct children, because TaskItem wraps content in a div)
	const nestedLists = li.querySelectorAll("ul, ol")
	if (nestedLists.length === 0) {
		// No nested lists - no special handling needed
		return false
	}

	// Find the bottom of the last nested list
	let lastNestedBottom = 0
	for (const list of nestedLists) {
		const bounds = findVerticalBounds(list as HTMLElement)
		if (bounds.bottom > lastNestedBottom) {
			lastNestedBottom = bounds.bottom
		}
	}

	// If Y is below all nested content, we're in the trailing margin
	return y > lastNestedBottom
}

/**
 * Find the deepest list item at a given Y position within the entire editor.
 * This searches ALL list items in the document and returns the one with the
 * highest nesting level whose bounds contain Y.
 */
function findDeepestListItemAtY(
	view: EditorView,
	state: EditorState,
	y: number,
): DraggableNodeResult | null {
	const allListItems = view.dom.querySelectorAll("li")

	let deepestLi: HTMLElement | null = null
	let deepestLevel = -1

	for (const li of allListItems) {
		const bounds = findVerticalBounds(li as HTMLElement)
		if (y >= bounds.top && y <= bounds.bottom) {
			// Skip if we're in the trailing margin of a parent list item
			if (isInListItemTrailingMargin(li as HTMLElement, y)) {
				continue
			}

			// Count nesting level by counting ancestor <li> elements
			let level = 0
			let parent = li.parentElement
			while (parent && parent !== view.dom) {
				if (LIST_ITEM_TAGS.has(parent.tagName)) {
					level++
				}
				parent = parent.parentElement
			}

			if (level >= deepestLevel) {
				deepestLevel = level
				deepestLi = li as HTMLElement
			}
		}
	}

	if (deepestLi) {
		return findFromElement(view, state, deepestLi)
	}

	return null
}

/**
 * Find draggable node from a DOM element.
 * If hovering over a list item, checks for nested list items at the cursor's Y position
 * and returns the deepest one instead of the parent.
 */
function findFromElement(
	view: EditorView,
	state: EditorState,
	element: Element,
	cursorY?: number,
): DraggableNodeResult | null {
	// First, if we have a cursorY and the element is or is inside a list item,
	// find the deepest list item at that Y position in the entire editor
	if (cursorY !== undefined) {
		const isInListItem =
			LIST_ITEM_TAGS.has(element.tagName) || element.closest("li")

		if (isInListItem) {
			const deepestListItem = findDeepestListItemAtY(view, state, cursorY)
			if (deepestListItem) {
				return deepestListItem
			}
		}
	}

	// For atom nodes: check if we're inside a node view wrapper first.
	// Atom nodes have no content, so posAtDOM on their inner elements won't resolve
	// to the atom node itself. We need to find the wrapper and handle it specially.
	const nodeViewWrapper = element.closest("[data-node-view-wrapper]")
	if (nodeViewWrapper && nodeViewWrapper !== view.dom) {
		try {
			const wrapperPos = view.posAtDOM(nodeViewWrapper, 0)
			if (wrapperPos >= 0) {
				const nodeAtPos = state.doc.nodeAt(wrapperPos)
				if (nodeAtPos && DRAGGABLE_NODE_TYPES.has(nodeAtPos.type.name)) {
					const dom = view.nodeDOM(wrapperPos) as HTMLElement | null
					if (
						dom === nodeViewWrapper &&
						dom?.nodeType === 1 &&
						!dom.classList.contains(DRAG_HANDLE_IGNORE_SELF_CLASS)
					) {
						const $pos = state.doc.resolve(wrapperPos)
						if (
							shouldShowDragHandle($pos, $pos.depth + 1, nodeAtPos.type.name)
						) {
							return {
								node: nodeAtPos,
								pos: wrapperPos,
								depth: $pos.depth + 1,
								dom,
							}
						}
					}
				}
			}
		} catch {
			// posAtDOM can throw, fall through to regular traversal
		}
	}

	let current: Element | null = element

	while (current && current !== view.dom) {
		let pos: number
		try {
			pos = view.posAtDOM(current, 0)
		} catch {
			current = current.parentElement
			continue
		}

		if (pos < 0) {
			current = current.parentElement
			continue
		}

		let $pos: ResolvedPos
		try {
			$pos = state.doc.resolve(pos)
		} catch {
			current = current.parentElement
			continue
		}

		// For atom nodes (nodes with no content), posAtDOM returns a position at the
		// node boundary, not "inside" the node. The depth traversal loop below won't
		// find atom nodes because they're not ancestors of the resolved position.
		// Check if this element is a TipTap node view wrapper or a native
		// ProseMirror atom element (e.g. <hr>) and try to find an atom node
		// directly at this position using nodeAt().
		const nodeAtPos = state.doc.nodeAt(pos)
		if (
			nodeAtPos &&
			DRAGGABLE_NODE_TYPES.has(nodeAtPos.type.name) &&
			(current.hasAttribute("data-node-view-wrapper") || nodeAtPos.isAtom)
		) {
			const dom = view.nodeDOM(pos) as HTMLElement | null
			if (
				dom === current &&
				dom?.nodeType === 1 &&
				!dom.classList.contains(DRAG_HANDLE_IGNORE_SELF_CLASS) &&
				shouldShowDragHandle($pos, $pos.depth + 1, nodeAtPos.type.name)
			) {
				return { node: nodeAtPos, pos, depth: $pos.depth + 1, dom }
			}
		}

		for (let d = $pos.depth; d > 0; d--) {
			const node = $pos.node(d)
			const typeName = node.type.name
			const rules = DRAG_DISABLED_EXCEPT_RULES.get(typeName)

			if (
				shouldShowDragHandle($pos, d) &&
				((rules?.size === 0 && DRAGGABLE_NODE_TYPES.has(typeName)) ||
					DRAGGABLE_NODE_TYPES.has(typeName))
			) {
				const nodePos = $pos.before(d)
				const dom = view.nodeDOM(nodePos) as HTMLElement | null
				if (dom?.nodeType === 1) {
					// Skip this node if its DOM has ignore-self class
					// This means the node's container ignores direct hovers but allows children
					if (dom.classList.contains(DRAG_HANDLE_IGNORE_SELF_CLASS)) {
						continue
					}

					return { node, pos: nodePos, depth: d, dom }
				}
			}
		}

		current = current.parentElement
	}

	return null
}

/**
 * Find node by Y position (for margin hover).
 * For nested lists, prefers the deepest list item whose Y bounds contain the point.
 */
function findByY(
	view: EditorView,
	state: EditorState,
	container: Element,
	y: number,
): DraggableNodeResult | null {
	for (let i = 0; i < container.children.length; i++) {
		const child = container.children[i] as HTMLElement
		if (child.nodeType !== 1) {
			continue
		}

		const bounds = findVerticalBounds(child)
		if (y < bounds.top || y > bounds.bottom) {
			continue
		}

		// Check for fully ignored elements at this Y within this subtree
		if (
			child.classList.contains(DRAG_HANDLE_IGNORE_CLASS) ||
			isIgnoredAtY(child, y)
		) {
			return null
		}

		// Check nested children first
		const nested = findByY(view, state, child, y)
		if (nested) {
			return nested
		}

		// For list items, also check if any nested list items match the Y bounds
		// This handles the case where we're hovering in the margin area of a nested list
		if (LIST_ITEM_TAGS.has(child.tagName)) {
			// Skip if we're in the trailing margin of a parent list item
			if (isInListItemTrailingMargin(child, y)) {
				continue
			}

			const nestedListItem = findNestedListItemAtY(view, state, child, y)
			if (nestedListItem) {
				return nestedListItem
			}
		}

		// If this element has ignore-self class, don't return a result for it
		// but children have already been checked above
		if (child.classList.contains(DRAG_HANDLE_IGNORE_SELF_CLASS)) {
			continue
		}

		const result = findFromElement(view, state, child)

		// If fully disabled, return immediately
		if (result) {
			const rules = DRAG_DISABLED_EXCEPT_RULES.get(result.node.type.name)
			if (rules?.size === 0) {
				return result
			}
		}

		// Return result only after checking nested children
		if (result) {
			return result
		}
	}

	return null
}

/**
 * Find the deepest nested list item within a container that contains the given Y.
 * Searches ALL descendant list items and returns the deepest one whose bounds contain Y.
 */
function findNestedListItemAtY(
	view: EditorView,
	state: EditorState,
	container: HTMLElement,
	y: number,
): DraggableNodeResult | null {
	// Find ALL descendant list items (not just direct children)
	const allListItems = container.querySelectorAll("li")

	// Find the deepest list item that contains Y
	// We track the deepest by checking nesting level
	let deepestLi: HTMLElement | null = null
	let deepestLevel = -1

	for (const li of allListItems) {
		const bounds = findVerticalBounds(li as HTMLElement)
		if (y >= bounds.top && y <= bounds.bottom) {
			// Skip if we're in the trailing margin of this list item
			if (isInListItemTrailingMargin(li as HTMLElement, y)) {
				continue
			}

			// Count nesting level by counting ancestor <li> elements within container
			let level = 0
			let parent = li.parentElement
			while (parent && parent !== container) {
				if (LIST_ITEM_TAGS.has(parent.tagName)) {
					level++
				}
				parent = parent.parentElement
			}

			// Use >= to allow siblings at the same level to override each other
			// This works because only one sibling's Y bounds should contain the cursor
			// at any given time (siblings are vertically stacked)
			if (level >= deepestLevel) {
				deepestLevel = level
				deepestLi = li as HTMLElement
			}
		}
	}

	if (deepestLi) {
		// Don't pass Y here - we've already found the deepest LI, no need to check again
		return findFromElement(view, state, deepestLi)
	}

	return null
}

/**
 * Finds the draggable node at the given screen coordinates.
 */
export function findDraggableNodeAtCoords(
	editor: Editor,
	x: number,
	y: number,
	options: FindDraggableNodeOptions = {},
): DraggableNodeResult | null {
	const { leftMarginWidth = 0 } = options
	const { view, state } = editor
	const editorLeft = view.dom.getBoundingClientRect().left
	const inLeftMargin = leftMarginWidth > 0 && x < editorLeft + leftMarginWidth

	// Check for ignored elements at this Y
	if (isIgnoredAtY(view.dom, y)) {
		return null
	}

	// <hr> elements are only 1px tall so elementFromPoint and margin-based
	// detection almost always miss them from above. Do an early scan for any
	// <hr> whose center is close to the cursor Y.
	const nearbyHr = findNearbyHr(view, state, y)
	if (nearbyHr) {
		return nearbyHr
	}

	// Not in margin: use elementFromPoint
	if (!inLeftMargin) {
		// First, we try finding element with an offset. Since drag
		// handle occupies some space, it gives a better UX when drag
		// handle appears before hovering on the actual element. If
		// we can't find an element with an offset, we try finding without.
		let element = document.elementFromPoint(x + DRAG_HANDLE_APPROX_WIDTH_PX, y)
		if (!element) {
			element = document.elementFromPoint(x, y)
		}

		if (element) {
			// Check if element has ignore-self class - return null (no drag handle)
			if (element.classList.contains(DRAG_HANDLE_IGNORE_SELF_CLASS)) {
				return null
			}

			// For list containers (ul/ol), fall back to Y-based detection
			// This handles hovering in whitespace between list items
			if (IGNORED_LIST_CONTAINER_TAGS.has(element.tagName)) {
				return findByY(view, state, view.dom, y)
			}

			if (!hasIgnoreClass(element, view.dom)) {
				const result = findFromElement(view, state, element, y)
				if (result) {
					return result
				}
			}
		}
	}

	// In margin or fallback: find by Y
	return findByY(view, state, view.dom, y)
}

export function findDraggableNodeAtPos(
	editor: Editor,
	pos: number,
): DraggableNodeResult | null {
	const { view, state } = editor

	let $pos: ResolvedPos
	try {
		$pos = state.doc.resolve(pos)
	} catch {
		return null
	}

	for (let d = $pos.depth; d > 0; d--) {
		const node = $pos.node(d)
		const typeName = node.type.name
		const rules = DRAG_DISABLED_EXCEPT_RULES.get(typeName)

		if (
			shouldShowDragHandle($pos, d) &&
			((rules?.size === 0 && DRAGGABLE_NODE_TYPES.has(typeName)) ||
				DRAGGABLE_NODE_TYPES.has(typeName))
		) {
			const nodePos = $pos.before(d)
			const dom = view.nodeDOM(nodePos) as HTMLElement | null
			if (dom?.nodeType === 1) {
				return { node, pos: nodePos, depth: d, dom }
			}
		}
	}

	return null
}
