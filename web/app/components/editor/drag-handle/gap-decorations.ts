import { Extension, type Editor } from "@tiptap/core"
import { Plugin, PluginKey, type EditorState } from "prosemirror-state"
import { Decoration, DecorationSet } from "prosemirror-view"
import type { Node, NodeType } from "prosemirror-model"
import {
	DEBUG_SHOW_GAPS,
	DRAG_CONTAINER_NODE_TYPES,
	gapZoneConfigByType,
	LIST_ITEM_TO_DEFAULT_LIST_TYPE,
	LIST_ITEM_TYPES,
	LIST_NODE_TYPES,
	NODE_TO_WRAPPER_TYPE,
	type GapZoneConfig,
} from "./config"
import { METRIC_BLOCK_NAME } from "../blocks/node-names"

declare module "@tiptap/core" {
	interface Commands<ReturnType> {
		gapDecorations: {
			refreshGapDecorations: () => ReturnType
		}
	}
}

const gapDecorationPluginKey = new PluginKey<GapDecorationState>(
	"gapDecorations",
)

interface GapDecorationState {
	decorations: DecorationSet
	/** Map of decoration key to position for efficient lookup during updates */
	gapsByKey: Map<string, number>
}

interface GapInfo {
	nodeType: string
	pos: number
	height: string
	yOffset: number
	debugColor?: string
	/** List indentation level - 0 for non-lists or top-level lists, higher for nested lists */
	indentLevel: number
	/** Left inset - positive shrinks from left, negative extends */
	leftInset?: number
	/** Right inset - positive shrinks from right, negative extends */
	rightInset?: number
	/** Whether this is a vertical gap (for horizontal layouts like MetricGrid) */
	verticalGap?: boolean
	/** Width of vertical gap zone */
	width?: string
	/** Horizontal offset for vertical gaps */
	xOffset?: number
	/** Stable key for decoration reuse, based on parent node ID + child index */
	key: string
}

/** Measurements needed to position a vertical gap zone */
interface VerticalGapMeasurement {
	type: "vertical"
	elWidth: number
	parentRect: DOMRect
	gapLeft: number
	gapRight: number
	gapHeight: number
	gapTop: number
	showSecondary: boolean
	secondaryLeft: number
	secondaryTop: number
	secondaryHeight: number
	// For secondary element creation
	wrapper: HTMLElement
	secondaryEl: HTMLElement | null
	primaryKey: string
	primaryWidth: string
	primaryPointerEvents: string
	primaryZIndex: string
	debugColor: string | null
}

/** Flag to coalesce multiple reposition requests into a single frame */
let repositionScheduled = false

/** Flag indicating a reposition was requested during cooldown */
let repositionPending = false

/** Active cooldown timeout handle */
let cooldownTimeout: ReturnType<typeof setTimeout> | null = null

/** Cooldown period in milliseconds - subsequent calls batch until this expires */
const REPOSITION_COOLDOWN_MS = 3000

/**
 * Clears the repositioning cooldown state.
 * Call this when drag ends to allow immediate repositioning on next interaction.
 */
export function clearRepositionCooldown() {
	if (cooldownTimeout !== null) {
		clearTimeout(cooldownTimeout)
		cooldownTimeout = null
	}
	repositionPending = false
}

/**
 * Measure a vertical gap zone element without writing to DOM.
 * Uses X-axis positioning for horizontal layouts like MetricGrid.
 * For multi-row grids, calculates the correct Y coordinate.
 * When siblings wrap to different rows, calculates a secondary gap position
 * at the end of the previous row.
 *
 * Height is determined by:
 * - Before first child: height of first child
 * - Between children (same row): height of the tallest child in the grid
 * - Between children (row wrap): secondary gap uses prev height, primary uses tallest
 * - After last child: height of last child
 *
 * Returns null if element is not ready for positioning.
 */
function measureVerticalGapZone(
	el: HTMLElement,
): VerticalGapMeasurement | null {
	const wrapper = el.parentElement
	if (!wrapper || !el.isConnected || !wrapper.isConnected) {
		return null
	}

	const prevSibling = wrapper.previousElementSibling as HTMLElement | null
	const nextSibling = wrapper.nextElementSibling as HTMLElement | null
	const parentRect = el.offsetParent?.getBoundingClientRect()
	const gridParent = wrapper.parentElement

	if (!parentRect || !gridParent) {
		return null
	}

	// Get actual computed width in pixels
	const elWidth = el.offsetWidth

	// Get existing secondary gap element for row boundaries
	const secondaryEl = wrapper.querySelector<HTMLElement>(
		".pm-gap-zone-secondary",
	)

	let gapLeft: number
	let gapRight: number
	let gapHeight: number
	let gapTop: number
	let showSecondary = false
	let secondaryLeft = 0
	let secondaryTop = 0
	let secondaryHeight = 0

	if (prevSibling && nextSibling) {
		// Between two siblings: position relative to the next sibling (we're inserting "before" it)
		const nextRect = nextSibling.getBoundingClientRect()
		const prevRect = prevSibling.getBoundingClientRect()

		// Check if siblings are on different rows (next is below prev)
		const isWrapping = nextRect.top > prevRect.bottom - 10 // 10px tolerance

		if (isWrapping) {
			// Siblings are on different rows
			// Primary gap: position at the start of next sibling's row (before the next sibling)
			gapLeft = nextRect.left - elWidth
			gapRight = nextRect.left
			gapTop = nextRect.top - parentRect.top

			// Secondary gap: position at the end of prev sibling's row (after the prev sibling)
			showSecondary = true
			// Center the secondary gap in the space after prevSibling (similar to same-row gaps)
			const secondaryGapLeft = prevRect.right
			const secondaryGapRight = prevRect.right + elWidth
			const secondaryGapCenter = (secondaryGapLeft + secondaryGapRight) / 2
			secondaryLeft = secondaryGapCenter - elWidth / 2 - parentRect.left
			secondaryTop = prevRect.top - parentRect.top
			secondaryHeight = prevSibling.offsetHeight
		} else {
			// Same row - center in the horizontal gap
			gapLeft = prevRect.right
			gapRight = nextRect.left
			gapTop = nextRect.top - parentRect.top
		}

		// Find the tallest child in the grid
		let maxHeight = 0
		for (const child of gridParent.children) {
			if (child !== wrapper && !child.classList.contains("pm-gap-wrapper")) {
				maxHeight = Math.max(maxHeight, (child as HTMLElement).offsetHeight)
			}
		}
		gapHeight = maxHeight
	} else if (prevSibling) {
		// After last child: position to the right of prev sibling
		// Height = height of last child (prevSibling)
		const rect = prevSibling.getBoundingClientRect()
		gapLeft = rect.right
		gapRight = rect.right + elWidth
		gapHeight = prevSibling.offsetHeight
		gapTop = rect.top - parentRect.top
	} else if (nextSibling) {
		// Before first child: position to the left of next sibling
		// Height = height of first child (nextSibling)
		const rect = nextSibling.getBoundingClientRect()
		gapLeft = rect.left - elWidth
		gapRight = rect.left
		gapHeight = nextSibling.offsetHeight
		gapTop = rect.top - parentRect.top
	} else {
		return null
	}

	return {
		type: "vertical",
		elWidth,
		parentRect,
		gapLeft,
		gapRight,
		gapHeight,
		gapTop,
		showSecondary,
		secondaryLeft,
		secondaryTop,
		secondaryHeight,
		wrapper,
		secondaryEl,
		primaryKey: el.getAttribute("data-gap-key") || "",
		primaryWidth: el.style.width,
		primaryPointerEvents: el.style.pointerEvents,
		primaryZIndex: el.style.zIndex,
		debugColor: el.getAttribute("data-debug-color"),
	}
}

/**
 * Apply computed position to a vertical gap zone element.
 * Handles secondary gap element creation for row boundaries in multi-row grids.
 */
function applyVerticalGapPosition(
	el: HTMLElement,
	m: VerticalGapMeasurement,
	xOffset: number,
) {
	const gapCenter = (m.gapLeft + m.gapRight) / 2
	const left = gapCenter - m.elWidth / 2 - m.parentRect.left + xOffset

	el.style.left = `${left}px`
	el.style.height = `${m.gapHeight}px`
	el.style.top = `${m.gapTop}px`
	el.style.bottom = "auto"

	// Handle secondary gap element for row boundaries
	if (m.showSecondary) {
		let secondaryEl = m.secondaryEl
		if (!secondaryEl) {
			// Create secondary element if it doesn't exist
			secondaryEl = document.createElement("div")
			secondaryEl.classList.add("pm-gap-zone-secondary")
			secondaryEl.classList.add("pm-gap-zone")
			secondaryEl.classList.add("drag-handle-ignore-self")
			// Copy key attribute from primary
			secondaryEl.setAttribute("data-gap-key", m.primaryKey)
			secondaryEl.setAttribute("data-gap-orientation", "vertical")
			// Copy debug color attribute if present
			if (m.debugColor) {
				secondaryEl.setAttribute("data-debug-color", m.debugColor)
			}
			Object.assign(secondaryEl.style, {
				position: "absolute",
				width: m.primaryWidth,
				pointerEvents: m.primaryPointerEvents,
				zIndex: m.primaryZIndex,
			})
			m.wrapper.appendChild(secondaryEl)
		}
		// Update secondary element styles (including debug color if enabled)
		secondaryEl.style.display = "block"
		secondaryEl.style.left = `${m.secondaryLeft}px`
		secondaryEl.style.top = `${m.secondaryTop}px`
		secondaryEl.style.height = `${m.secondaryHeight}px`
		// Sync background color from primary (for debug mode)
		secondaryEl.style.backgroundColor = el.style.backgroundColor
	} else if (m.secondaryEl) {
		// Hide secondary element when not needed
		m.secondaryEl.style.display = "none"
	}
}

/** Measurements needed to position a horizontal gap zone */
interface HorizontalGapMeasurement {
	type: "horizontal"
	elHeight: number
	parentRect: DOMRect
	gapTop: number
	gapBottom: number
	siblingRect: DOMRect | null
	leftInset: number
	rightInset: number
	indentLevel: number
}

/**
 * Measure a horizontal gap zone element without writing to DOM.
 * Sets both vertical position and horizontal bounds based on siblings.
 * Returns null if element is not ready for positioning.
 */
function measureHorizontalGapZone(
	el: HTMLElement,
): HorizontalGapMeasurement | null {
	const wrapper = el.parentElement
	if (!wrapper || !el.isConnected || !wrapper.isConnected) {
		return null
	}

	const prevSibling = wrapper.previousElementSibling as HTMLElement | null
	const nextSibling = wrapper.nextElementSibling as HTMLElement | null
	const parentRect = el.offsetParent?.getBoundingClientRect()

	if (!parentRect) {
		return null
	}

	// Get actual computed height in pixels
	const elHeight = el.offsetHeight

	let gapTop: number
	let gapBottom: number
	let siblingRect: DOMRect | null = null

	if (prevSibling && nextSibling) {
		// Between two siblings: center in the gap
		gapTop = prevSibling.getBoundingClientRect().bottom
		gapBottom = nextSibling.getBoundingClientRect().top
		// Use the sibling that's more to the right (more indented) for width
		const prevRect = prevSibling.getBoundingClientRect()
		const nextRect = nextSibling.getBoundingClientRect()
		siblingRect = prevRect.left >= nextRect.left ? prevRect : nextRect
	} else if (prevSibling) {
		// After last child: position below prev sibling
		const rect = prevSibling.getBoundingClientRect()
		gapTop = rect.bottom
		gapBottom = rect.bottom + elHeight
		siblingRect = rect
	} else if (nextSibling) {
		// Before first child: position above next sibling
		const rect = nextSibling.getBoundingClientRect()
		gapTop = rect.top - elHeight
		gapBottom = rect.top
		siblingRect = rect
	} else {
		return null
	}

	// Get inset values from data attributes
	const leftInset = parseFloat(el.getAttribute("data-gap-left-inset") || "0")
	const rightInset = parseFloat(el.getAttribute("data-gap-right-inset") || "0")
	const indentLevel = parseInt(
		el.getAttribute("data-gap-indent-level") || "0",
		10,
	)

	return {
		type: "horizontal",
		elHeight,
		parentRect,
		gapTop,
		gapBottom,
		siblingRect,
		leftInset,
		rightInset,
		indentLevel,
	}
}

/**
 * Apply computed position to a horizontal gap zone element.
 * Sets horizontal bounds based on sibling dimensions and indent level.
 */
function applyHorizontalGapPosition(
	el: HTMLElement,
	m: HorizontalGapMeasurement,
	yOffset: number,
) {
	const gapCenter = (m.gapTop + m.gapBottom) / 2
	const top = gapCenter - m.elHeight / 2 - m.parentRect.top + yOffset
	el.style.top = `${top}px`

	// Set horizontal bounds based on sibling dimensions
	// Only for indented list items (indentLevel > 0) - top level spans full width
	if (m.indentLevel > 0 && m.siblingRect) {
		const left = m.siblingRect.left - m.parentRect.left + m.leftInset
		const right = m.parentRect.right - m.siblingRect.right + m.rightInset
		el.style.left = `${left}px`
		el.style.right = `${right}px`
	} else {
		// Top level: apply insets to full document width
		el.style.left = `${m.leftInset}px`
		el.style.right = `${m.rightInset}px`
	}
}

type GapMeasurement = VerticalGapMeasurement | HorizontalGapMeasurement

/**
 * Measure a gap zone element without writing to DOM.
 */
function measureGapZone(el: HTMLElement): GapMeasurement | null {
	const isVertical = el.getAttribute("data-gap-orientation") === "vertical"
	if (isVertical) {
		return measureVerticalGapZone(el)
	}

	return measureHorizontalGapZone(el)
}

/**
 * Apply computed position to a gap zone element.
 */
function applyGapPosition(
	el: HTMLElement,
	measurement: GapMeasurement,
	offset: number,
) {
	if (measurement.type === "vertical") {
		applyVerticalGapPosition(el, measurement, offset)
	} else {
		applyHorizontalGapPosition(el, measurement, offset)
	}
}

/**
 * Position a single gap zone element based on its siblings.
 * Used for initial positioning in createGapWidget.
 */
function positionGapZone(el: HTMLElement, yOffset: number) {
	const isVertical = el.getAttribute("data-gap-orientation") === "vertical"
	const measurement = measureGapZone(el)
	if (!measurement) return

	if (isVertical) {
		const xOffset = parseFloat(el.getAttribute("data-gap-x-offset") || "0")
		applyGapPosition(el, measurement, xOffset)
	} else {
		applyGapPosition(el, measurement, yOffset)
	}
}

/**
 * Check if a node type can be inserted at a gap position using ProseMirror's schema.
 * This uses the schema's contentMatch to determine valid drop targets.
 *
 * Also handles wrapping: if dragging a list item to a non-list location,
 * checks if wrapping the item in a list would be valid.
 *
 * @param parentNode - The parent container node
 * @param insertIndex - The index where the node would be inserted (gap position)
 * @param draggedNode - The node being dragged
 * @param originalListTypeName - The original parent list type name (if dragging from a list)
 * @returns true if the dragged node can be dropped at this gap (possibly wrapped)
 */
export function canDropAtGap(
	parentNode: Node,
	insertIndex: number,
	draggedNode: Node,
	originalListTypeName: string | null,
): boolean {
	// If dragging a list item, check list type compatibility
	if (LIST_ITEM_TYPES.has(draggedNode.type.name) && originalListTypeName) {
		// If dropping into a list, it must be the same type as the original
		if (LIST_NODE_TYPES.has(parentNode.type.name)) {
			if (parentNode.type.name !== originalListTypeName) {
				return false
			}
		}
	}

	// Try direct insertion
	if (canInsertNodeAt(parentNode, insertIndex, draggedNode.type)) {
		return true
	}

	// If dragging a list item and direct insert failed,
	// check if we can wrap it in the original list type (or default) and insert that
	if (LIST_ITEM_TYPES.has(draggedNode.type.name)) {
		// Prefer wrapping in original list type, fall back to default
		const wrapListTypeName =
			originalListTypeName ||
			LIST_ITEM_TO_DEFAULT_LIST_TYPE.get(draggedNode.type.name)
		if (wrapListTypeName) {
			const listType = draggedNode.type.schema.nodes[wrapListTypeName]
			if (listType && canInsertNodeAt(parentNode, insertIndex, listType)) {
				return true
			}
		}
	}

	// If node needs a wrapper (e.g., MetricBlock -> MetricGrid), check if wrapper can be inserted
	const wrapperTypeName = NODE_TO_WRAPPER_TYPE.get(draggedNode.type.name)
	if (wrapperTypeName) {
		const wrapperType = draggedNode.type.schema.nodes[wrapperTypeName]
		if (wrapperType && canInsertNodeAt(parentNode, insertIndex, wrapperType)) {
			return true
		}
	}

	return false
}

/**
 * Check if a node type can be inserted at a specific index in a parent node.
 * Uses ProseMirror's contentMatch to walk through existing children and verify
 * that the inserted type is valid at that position.
 */
function canInsertNodeAt(
	parentNode: Node,
	insertIndex: number,
	nodeType: NodeType,
): boolean {
	const parentType = parentNode.type

	// Get the content match at the insertion point
	let contentMatch = parentType.contentMatch

	// Advance through existing children up to the insert index
	for (let i = 0; i < insertIndex && i < parentNode.childCount; i++) {
		const child = parentNode.child(i)
		const nextMatch = contentMatch.matchType(child.type)
		if (!nextMatch) {
			return false
		}
		contentMatch = nextMatch
	}

	// Check if the node type is valid at this position
	const matchAfterInsert = contentMatch.matchType(nodeType)
	if (!matchAfterInsert) {
		return false
	}

	// Also verify remaining children can still follow after the inserted node
	let remainingMatch = matchAfterInsert
	for (let i = insertIndex; i < parentNode.childCount; i++) {
		const child = parentNode.child(i)
		const nextMatch = remainingMatch.matchType(child.type)
		if (!nextMatch) {
			return false
		}
		remainingMatch = nextMatch
	}

	return true
}

/**
 * Performs the actual gap zone repositioning.
 * Uses batched DOM reads/writes to minimize layout thrashing.
 */
function doRepositionGapZones() {
	const elements = document.querySelectorAll(
		".pm-gap-zone:not(.pm-gap-zone-secondary)",
	)
	if (elements.length === 0) {
		return
	}

	// Phase 1: Collect element data (minimal reads)
	const elementData: {
		el: HTMLElement
		isVertical: boolean
		offset: number
	}[] = []

	elements.forEach((el) => {
		const htmlEl = el as HTMLElement
		const isVertical =
			htmlEl.getAttribute("data-gap-orientation") === "vertical"
		const offset = isVertical
			? parseFloat(htmlEl.getAttribute("data-gap-x-offset") || "0")
			: parseFloat(htmlEl.dataset.yOffset || "0")
		elementData.push({ el: htmlEl, isVertical, offset })
	})

	// Phase 2: Batch all DOM measurements (reads)
	const measurements: (GapMeasurement | null)[] = elementData.map(({ el }) =>
		measureGapZone(el),
	)

	// Phase 3: Batch all DOM updates (writes)
	elementData.forEach(({ el, offset }, i) => {
		const measurement = measurements[i]
		if (measurement) {
			applyGapPosition(el, measurement, offset)
		}
	})
}

/**
 * Reposition all existing gap zones.
 *
 * Throttling behavior:
 * - First call executes immediately (in the next animation frame)
 * - Subsequent calls within 1 second are batched
 * - After the cooldown expires, if any calls were batched, one final reposition runs
 *
 * This prevents excessive repositioning during rapid events like drag or resize,
 * while ensuring the final state is always correct.
 */
export function repositionGapZones() {
	// If in cooldown period, mark as pending for later execution
	if (cooldownTimeout !== null) {
		repositionPending = true
		return
	}

	// Already scheduled for this frame
	if (repositionScheduled) {
		return
	}

	repositionScheduled = true

	requestAnimationFrame(() => {
		repositionScheduled = false
		doRepositionGapZones()

		// Start cooldown period
		cooldownTimeout = setTimeout(() => {
			cooldownTimeout = null
			// If calls came in during cooldown, execute one more time
			if (repositionPending) {
				repositionPending = false
				repositionGapZones()
			}
		}, REPOSITION_COOLDOWN_MS)
	})
}

/**
 * Collect gap positions for a container node's children.
 * Checks each child's type to determine if/how to add gap zones.
 * @param indentLevel - List indentation level (0 = non-list or top-level list, higher = nested lists)
 * @param parentNodeType - The type name of the parent container node (for context-aware config)
 * @param parentId - Stable ID of the parent node for generating decoration keys
 */
function collectGapsForContainer(
	node: Node,
	contentStart: number,
	gaps: GapInfo[],
	indentLevel = 0,
	parentNodeType?: string,
	parentId?: string,
): void {
	let offset = 0
	let isFirst = true
	let lastHeight: string | null = null
	let lastConfig: GapZoneConfig | null = null
	let lastChildId: string | null = null
	// Use parent ID or fall back to position-based key for stability
	const containerKey = parentId ?? `pos-${contentStart}`

	node.forEach((child) => {
		const childPos = contentStart + offset
		// Get stable child ID for key generation
		const childUid = child.attrs?.uid as string | undefined
		const childId = childUid ?? `idx-${offset}`
		const config = gapZoneConfigByType(
			child.type.name,
			indentLevel,
			parentNodeType,
		)

		if (config) {
			if (isFirst) {
				// First child: optionally add gap before it
				if (config.includeBeforeFirst) {
					gaps.push({
						pos: childPos,
						height: config.height,
						yOffset: config.yOffsetFirst ?? 0,
						debugColor: config.debugColor,
						nodeType: child.type.name,
						indentLevel,
						leftInset: config.leftInset,
						rightInset: config.rightInset,
						verticalGap: config.verticalGap,
						width: config.width,
						xOffset: config.xOffsetFirst ?? 0,
						// Key identifies the gap by the child it's before
						key: `${containerKey}:before:${childId}`,
					})
				}
			} else {
				// Not first child: add gap before this child
				// Use the larger height if previous child also had a config (for horizontal gaps)
				const height =
					lastHeight && lastHeight > config.height ? lastHeight : config.height
				gaps.push({
					pos: childPos,
					height,
					yOffset: config.yOffsetMiddle ?? 0,
					debugColor: config.debugColor,
					nodeType: child.type.name,
					indentLevel,
					leftInset: config.leftInset,
					rightInset: config.rightInset,
					verticalGap: config.verticalGap,
					width: config.width,
					xOffset: config.xOffsetMiddle ?? 0,
					// Key identifies the gap by the child it's before
					key: `${containerKey}:before:${childId}`,
				})
			}
			lastHeight = config.height
			lastConfig = config
			lastChildId = childId
		} else {
			lastHeight = null
			lastConfig = null
			lastChildId = childId
		}

		isFirst = false
		offset += child.nodeSize
	})

	// Add gap at the end of this container (after last child)
	if (node.childCount > 0 && lastHeight !== null && lastConfig !== null) {
		const lastChild = node.child(node.childCount - 1)
		gaps.push({
			pos: contentStart + node.content.size,
			height: lastHeight,
			yOffset: (lastConfig as GapZoneConfig).yOffsetLast ?? 0,
			debugColor: (lastConfig as GapZoneConfig).debugColor,
			nodeType: lastChild.type.name,
			indentLevel,
			leftInset: (lastConfig as GapZoneConfig).leftInset,
			rightInset: (lastConfig as GapZoneConfig).rightInset,
			verticalGap: (lastConfig as GapZoneConfig).verticalGap,
			width: (lastConfig as GapZoneConfig).width,
			xOffset: (lastConfig as GapZoneConfig).xOffsetLast ?? 0,
			// Key identifies the gap by being after the last child
			key: `${containerKey}:after:${lastChildId}`,
		})
	}
}

/**
 * Collects all gaps from the document with their stable keys.
 */
function collectAllGaps(doc: Node): GapInfo[] {
	const gaps: GapInfo[] = []

	// Handle doc-level gaps (between top-level nodes)
	// Use "doc" as the parent key for document-level gaps
	collectGapsForContainer(doc, 0, gaps, 0, undefined, "doc")

	// Track list ranges to determine indentation level
	// Only lists count toward indentation, not other containers
	const listRanges: {
		start: number
		end: number
		indentLevel: number
	}[] = []

	// First pass: collect all list positions and their node sizes
	const listInfo: { pos: number; size: number; isInList: boolean }[] = []
	doc.descendants((node, pos) => {
		if (LIST_NODE_TYPES.has(node.type.name)) {
			listInfo.push({ pos, size: node.nodeSize, isInList: true })
		}
		return true
	})

	// Calculate indentation level for each list based on list nesting only
	for (const { pos, size } of listInfo) {
		// Count how many existing lists this one is inside of
		let indentLevel = 0
		for (const range of listRanges) {
			// Check if this list starts inside another list's range
			if (pos > range.start && pos < range.end) {
				indentLevel = Math.max(indentLevel, range.indentLevel + 1)
			}
		}
		listRanges.push({ start: pos, end: pos + size, indentLevel })
	}

	// Create a map for quick lookup of list indent levels by position
	const indentByPos = new Map<number, number>()
	for (let i = 0; i < listInfo.length; i++) {
		const info = listInfo[i]
		const range = listRanges[i]
		if (info && range) {
			indentByPos.set(info.pos, range.indentLevel)
		}
	}

	// Second pass: collect gaps with proper indent levels
	doc.descendants((node, pos) => {
		if (DRAG_CONTAINER_NODE_TYPES.has(node.type.name)) {
			// For lists, use their calculated indent level
			// For non-lists (like SplitDocumentation sides), use 0
			const indentLevel = indentByPos.get(pos) ?? 0
			// contentStart is pos + 1 (after the node's opening tag)
			// Pass the parent node type for context-aware gap config (e.g., MetricGrid)
			// Use node's uid attribute if available, otherwise fall back to position-based key
			const nodeId = node.attrs?.uid as string | undefined
			collectGapsForContainer(
				node,
				pos + 1,
				gaps,
				indentLevel,
				node.type.name,
				nodeId ?? `type-${node.type.name}-pos-${pos}`,
			)
		}
		// Continue traversing
		return true
	})

	return gaps
}

/**
 * Creates a single gap decoration widget.
 */
function createGapWidget(gap: GapInfo, docSize: number): Decoration | null {
	const {
		pos,
		height,
		yOffset,
		debugColor,
		indentLevel,
		leftInset,
		rightInset,
		verticalGap,
		width,
		xOffset,
		key,
	} = gap

	// Skip invalid positions
	if (pos < 0 || pos > docSize) {
		return null
	}

	// Higher indentLevel = more to the right = higher z-index (more priority)
	// This allows dropping into nested lists when hovering over their indented area
	const BASE_Z_INDEX = 50
	const zIndex = BASE_Z_INDEX + indentLevel

	return Decoration.widget(
		pos,
		() => {
			// display: contents = wrapper doesn't participate in layout at all
			const wrapper = document.createElement("div")
			wrapper.classList.add("pm-gap-wrapper")
			Object.assign(wrapper.style, {
				display: "contents",
			})

			// Absolute element positions relative to nearest positioned ancestor
			const el = document.createElement("div")
			el.classList.add("pm-gap-zone")
			el.classList.add("drag-handle-ignore-self")
			el.setAttribute("data-gap-key", key)
			el.setAttribute("data-gap-indent-level", String(indentLevel))
			el.setAttribute("data-gap-left-inset", String(leftInset ?? 0))
			el.setAttribute("data-gap-right-inset", String(rightInset ?? 0))
			el.dataset.yOffset = String(yOffset)

			if (verticalGap) {
				el.setAttribute("data-gap-orientation", "vertical")
				el.setAttribute("data-gap-x-offset", String(xOffset ?? 0))
			}

			if (debugColor && DEBUG_SHOW_GAPS) {
				el.setAttribute("data-debug-color", debugColor)
			}

			if (verticalGap) {
				// Vertical gap for horizontal layouts (e.g., MetricGrid)
				Object.assign(el.style, {
					position: "absolute",
					top: "0",
					bottom: "0",
					width: width || "1rem",
					height: "auto",
					pointerEvents: "none", // Enabled only during drag
					zIndex: String(zIndex),
				})
			} else {
				// Standard horizontal gap
				Object.assign(el.style, {
					position: "absolute",
					left: "0",
					right: "0",
					height: height,
					pointerEvents: "none", // Enabled only during drag
					zIndex: String(zIndex),
				})
			}

			// Position the element in the middle of the gap
			requestAnimationFrame(() => {
				positionGapZone(el, yOffset)
			})

			wrapper.appendChild(el)
			return wrapper
		},
		{
			side: -1,
			// Use stable key based on parent node ID + child index for DOM reuse
			key,
		},
	)
}

/**
 * Creates widget decorations that fill the visual gaps between block nodes.
 * These invisible elements allow `posAtCoords` to detect drop positions in gaps.
 * Returns both the DecorationSet and a map of gap keys to positions.
 */
function createGapDecorations(doc: Node): {
	decorations: DecorationSet
	gapsByKey: Map<string, number>
} {
	const gaps = collectAllGaps(doc)
	const decorations: Decoration[] = []
	const gapsByKey = new Map<string, number>()

	for (const gap of gaps) {
		const widget = createGapWidget(gap, doc.content.size)
		if (widget) {
			decorations.push(widget)
			gapsByKey.set(gap.key, gap.pos)
		}
	}

	return {
		decorations: DecorationSet.create(doc, decorations),
		gapsByKey,
	}
}

/**
 * Incrementally updates gap decorations by comparing old and new gaps.
 * Reuses existing decorations where possible to avoid DOM recreation.
 *
 * The key insight: ProseMirror reuses DOM elements when decorations have
 * the same key at the same position. By using stable keys (based on node IDs)
 * and mapping positions through transactions, we can preserve DOM elements
 * even when the document structure changes.
 */
function updateGapDecorations(
	oldDecorations: DecorationSet,
	doc: Node,
	mapping: any,
): { decorations: DecorationSet; gapsByKey: Map<string, number> } {
	const newGaps = collectAllGaps(doc)
	const newGapsByKey = new Map<string, number>()
	const newGapsMap = new Map<string, GapInfo>()

	// Build lookup for new gaps
	for (const gap of newGaps) {
		newGapsByKey.set(gap.key, gap.pos)
		newGapsMap.set(gap.key, gap)
	}

	// First, map existing decorations through the transaction
	// This shifts positions based on the document changes
	let decorations = oldDecorations.map(mapping, doc)

	// Build a map of existing (mapped) decorations by key
	// Note: find(from, to) uses half-open range [from, to), so we use +1 to include
	// decorations at doc.content.size (the "after last" gap position)
	const existingByKey = new Map<string, Decoration>()
	const allFoundDecos: Decoration[] = []
	decorations.find(0, doc.content.size + 1).forEach((deco) => {
		allFoundDecos.push(deco)
		const spec = (deco as any).spec
		const key = spec?.key as string | undefined
		if (key) {
			if (DEBUG_SHOW_GAPS) {
				if (existingByKey.has(key)) {
					console.warn(`Duplicate key found: ${key}`)
				}
			}

			existingByKey.set(key, deco)
		} else if (DEBUG_SHOW_GAPS) {
			console.warn(`Decoration without key at pos ${deco.from}`)
		}
	})

	const toRemove: Decoration[] = []
	const toAdd: Decoration[] = []

	// Step 1: Find decorations to remove (keys that no longer exist in new gaps)
	for (const [key, deco] of existingByKey) {
		if (!newGapsByKey.has(key)) {
			toRemove.push(deco)
		}
	}

	// Step 2: Process new gaps
	for (const gap of newGaps) {
		const existingDeco = existingByKey.get(gap.key)
		if (!existingDeco) {
			// Gap doesn't exist, create new decoration
			const widget = createGapWidget(gap, doc.content.size)
			if (widget) {
				toAdd.push(widget)
			}
		} else {
			// Gap exists, check if position matches after mapping
			const existingPos = existingDeco.from
			if (existingPos !== gap.pos) {
				// Position changed after mapping - need to recreate
				// This happens when the mapping didn't preserve the logical position
				// (e.g., deleted content before this gap)
				toRemove.push(existingDeco)
				const widget = createGapWidget(gap, doc.content.size)
				if (widget) {
					toAdd.push(widget)
				}
			}
			// If position matches, the decoration is already correct
			// ProseMirror will reuse the DOM element since key matches at same position
		}
	}

	// Apply removals first, then additions
	if (toRemove.length > 0) {
		decorations = decorations.remove(toRemove)
	}

	if (toAdd.length > 0) {
		decorations = decorations.add(doc, toAdd)
	}
	return { decorations, gapsByKey: newGapsByKey }
}

/**
 * Check if an element is a gap zone and return its drop position.
 * Looks up the position from plugin state using the gap's key attribute.
 */
export function getGapDropPosition(
	element: Element | null,
	state: EditorState,
): number | null {
	if (!element) {
		return null
	}

	const pluginState = gapDecorationPluginKey.getState(state)
	if (!pluginState) {
		return null
	}

	const { gapsByKey } = pluginState

	// Check if element itself is a gap zone
	const key = element.getAttribute("data-gap-key")
	if (key) {
		return gapsByKey.get(key) ?? null
	}

	// Check parent (in case of nested elements)
	const parent = element.closest("[data-gap-key]")
	if (parent) {
		const parentKey = parent.getAttribute("data-gap-key")
		if (parentKey) {
			return gapsByKey.get(parentKey) ?? null
		}
	}

	return null
}

/**
 * Find all gap zone elements that correspond to a given position.
 * Does a reverse lookup through the plugin state to find the key(s) for the position,
 * then queries the DOM for elements with those keys.
 */
export function findGapElementsByPos(
	state: EditorState,
	pos: number,
): HTMLElement[] {
	const pluginState = gapDecorationPluginKey.getState(state)
	if (!pluginState) {
		return []
	}

	const { gapsByKey } = pluginState
	const elements: HTMLElement[] = []

	// Find all keys that map to this position
	for (const [key, p] of gapsByKey) {
		if (p === pos) {
			// Find all elements with this key (primary and secondary gaps share the same key)
			const els = document.querySelectorAll<HTMLElement>(
				`[data-gap-key="${key}"]`,
			)
			elements.push(...els)
		}
	}

	return elements
}

export function enableGapZones(
	editor: Editor,
	draggedNode: Node,
	draggedNodePos: number,
) {
	const doc = editor.state.doc
	const pluginState = gapDecorationPluginKey.getState(editor.state)
	if (!pluginState) {
		return
	}
	const { gapsByKey } = pluginState

	// Find the original list type if dragging from a list
	let originalListTypeName: string | null = null
	if (LIST_ITEM_TYPES.has(draggedNode.type.name)) {
		const $pos = doc.resolve(draggedNodePos)
		for (let d = $pos.depth; d >= 0; d--) {
			const ancestor = $pos.node(d)
			if (LIST_NODE_TYPES.has(ancestor.type.name)) {
				originalListTypeName = ancestor.type.name
				break
			}
		}
	}

	// Find the source wrapper position if dragging a node that has a wrapper type
	// (e.g., MetricBlock inside MetricGrid)
	let sourceWrapperStart: number | null = null
	let sourceWrapperEnd: number | null = null
	const wrapperTypeName = NODE_TO_WRAPPER_TYPE.get(draggedNode.type.name)
	if (wrapperTypeName) {
		const $pos = doc.resolve(draggedNodePos)
		for (let d = $pos.depth; d >= 0; d--) {
			const ancestor = $pos.node(d)
			if (ancestor.type.name === wrapperTypeName) {
				sourceWrapperStart = $pos.before(d)
				sourceWrapperEnd = $pos.after(d)
				break
			}
		}
	}

	// Calculate the positions immediately before and after the dragged node
	// (for excluding gap zones at the exact source position)
	const draggedNodeEndPos = draggedNodePos + draggedNode.nodeSize

	document.querySelectorAll(".pm-gap-zone[data-gap-key]").forEach((el) => {
		const htmlEl = el as HTMLElement
		const key = el.getAttribute("data-gap-key")
		if (!key) {
			return
		}

		const gapPos = gapsByKey.get(key)
		if (gapPos === undefined) {
			return
		}

		const isVerticalGap = el.getAttribute("data-gap-orientation") === "vertical"

		// For MetricBlock dragging: skip gap zones at the exact source position
		// (immediately before or after the dragged block)
		if (draggedNode.type.name === METRIC_BLOCK_NAME) {
			if (gapPos === draggedNodePos || gapPos === draggedNodeEndPos) {
				htmlEl.style.pointerEvents = "none"
				htmlEl.style.backgroundColor = "transparent"
				return
			}
		}

		// Skip gap zones adjacent to the source wrapper (before or after the entire MetricGrid)
		// but only for the external wrapper gap zones (not for internal gaps)
		if (
			sourceWrapperStart !== null &&
			sourceWrapperEnd !== null &&
			!isVerticalGap &&
			(gapPos === sourceWrapperStart || gapPos === sourceWrapperEnd)
		) {
			htmlEl.style.pointerEvents = "none"
			htmlEl.style.backgroundColor = "transparent"
			return
		}

		// Find the parent container and insertion index at this gap position
		const resolved = doc.resolve(gapPos)
		const parentNode = resolved.parent
		const insertIndex = resolved.index()

		// Use schema-based check to determine if drop is valid
		const canDrop = canDropAtGap(
			parentNode,
			insertIndex,
			draggedNode,
			originalListTypeName,
		)
		if (canDrop) {
			htmlEl.style.pointerEvents = "auto"
			if (DEBUG_SHOW_GAPS) {
				const customColor = el.getAttribute("data-debug-color")
				htmlEl.style.backgroundColor = customColor || "rgba(255, 0, 0, 0.2)"
			} else {
				htmlEl.style.backgroundColor = "transparent"
			}
		} else {
			htmlEl.style.pointerEvents = "none"
			htmlEl.style.backgroundColor = "transparent"
		}
	})
}

export function disableGapZones() {
	document.querySelectorAll(".pm-gap-zone").forEach((el) => {
		;(el as HTMLElement).style.pointerEvents = "none"
		;(el as HTMLElement).style.backgroundColor = "transparent"
	})
}

export function refreshGapDecorationsInBackground(editor: Ref<Editor | null>) {
	until(editor)
		.not.toBeNull()
		.then(() => {
			editor.value?.commands.refreshGapDecorations()
		})
}

const REFRESH_GAP_DECORATIONS_META = "refreshGapDecorations"

export const GapDecorations = Extension.create({
	name: "gapDecorations",
	// Low priority to run after other plugins (like UniqueID) have processed
	priority: 1,
	addCommands() {
		return {
			refreshGapDecorations:
				() =>
				({ tr, dispatch }: { tr: any; dispatch: any }) => {
					if (dispatch) {
						dispatch(tr.setMeta(REFRESH_GAP_DECORATIONS_META, true))
					}
					return true
				},
		}
	},
	addProseMirrorPlugins() {
		return [
			new Plugin({
				key: gapDecorationPluginKey,
				state: {
					init(_, { doc }): GapDecorationState {
						return createGapDecorations(doc)
					},
					apply(tr, pluginState, _oldState, newState): GapDecorationState {
						const { decorations } = pluginState

						// Manual refresh command
						if (tr.getMeta(REFRESH_GAP_DECORATIONS_META)) {
							return createGapDecorations(newState.doc)
						}

						if (!tr.docChanged) {
							return pluginState
						}

						// Always use incremental update to ensure DOM attributes stay in sync
						// with decoration positions. Just mapping decorations would leave
						// data-gap-drop-pos attributes stale, causing issues during drag-drop.
						return updateGapDecorations(decorations, newState.doc, tr.mapping)
					},
				},
				props: {
					decorations(state) {
						return gapDecorationPluginKey.getState(state)?.decorations
					},
				},
				view(editorView) {
					// Use ResizeObserver on the editor element to detect width changes
					// (e.g., sidebar toggle, panel resize)
					let resizeObserver: ResizeObserver | null = null
					if (typeof ResizeObserver !== "undefined") {
						resizeObserver = new ResizeObserver(() => {
							// repositionGapZones handles its own RAF scheduling and coalescing
							repositionGapZones()
						})
						resizeObserver.observe(editorView.dom)
					}

					return {
						destroy() {
							resizeObserver?.disconnect()
						},
					}
				},
			}),
		]
	},
})
