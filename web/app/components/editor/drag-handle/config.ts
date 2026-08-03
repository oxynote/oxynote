import type { ComputePositionConfig } from "@floating-ui/dom"
import type { Editor } from "@tiptap/core"
import {
	ParameterList,
	ParameterListItem,
} from "../blocks/split-documentation/parameter-list"
import {
	TaskList,
	ListItem,
	BulletList,
	OrderedList,
	TaskItem,
} from "@tiptap/extension-list"
import {
	SplitDocumentation,
	SplitDocumentationLeftSide,
	SplitDocumentationRightSide,
} from "../blocks/split-documentation"
import { CalloutBlock } from "../blocks/callout"
import Paragraph from "@tiptap/extension-paragraph"
import { ImageBlock } from "../blocks/image"
import Heading from "@tiptap/extension-heading"
import Blockquote from "@tiptap/extension-blockquote"
import HorizontalRule from "@tiptap/extension-horizontal-rule"
import { MetricGrid } from "../blocks/metrics"
import {
	CODE_BLOCK_NAME,
	FIGMA_BLOCK_NAME,
	MERMAID_BLOCK_NAME,
	METRIC_BLOCK_NAME,
	TITLED_CODE_BLOCK_NAME,
} from "../blocks/node-names"

// we need to be careful with this value because it might trigger the
// drag handle of the nearby nodes even when you are in another node
// (especially when it comes to nodes that are close to each other like
// MetricBlocks in MetricGrid; with values above 10 when you try to click/hover
// on the settings icon of a MetricBlock sometimes the drag handle of the
// neighboring MetricBlock shows up instead)
export const DRAG_HANDLE_APPROX_WIDTH_PX = 9

export const DEBUG_SHOW_GAPS = false

// Nodes that "absorb" their children for drag purposes
// (children should not be treated as separate drag targets).
export const DRAG_COMPOSITE_DRAG_NODES = new Set([
	TITLED_CODE_BLOCK_NAME,
	ParameterListItem.name,
	CalloutBlock.name,
	METRIC_BLOCK_NAME,
	TaskItem.name,
	ListItem.name,
	Blockquote.name,
])

// Node types whose children should have gap zones.
export const DRAG_CONTAINER_NODE_TYPES = new Set([
	SplitDocumentationLeftSide.name,
	SplitDocumentationRightSide.name,
	ParameterList.name,
	BulletList.name,
	OrderedList.name,
	TaskList.name,
	MetricGrid.name,
])

/**
 * Configuration for gap zones based on child node type.
 */
export interface GapZoneConfig {
	/** Height of the gap zone (CSS value) - used for horizontal gaps */
	height: string
	/** Width of the gap zone (CSS value) - used for vertical gaps (e.g., MetricGrid) */
	width?: string
	/** Whether this is a vertical gap zone (for horizontal layouts like MetricGrid) */
	verticalGap?: boolean
	/** Whether to also add a gap zone before this node if it's the first child */
	includeBeforeFirst?: boolean
	/** Vertical offset for the first gap decoration (before first child).
	 * Mostly useful in isolated contexts, like ParameterLists in SplitDocumentationLeftSide */
	yOffsetFirst?: number
	/** Vertical offset for middle gap decorations (between children)
	 * Mostly useful in isolated contexts, like ParameterLists in SplitDocumentationLeftSide */
	yOffsetMiddle?: number
	/** Vertical offset for the last gap decoration (after last child)
	 * Mostly useful in isolated contexts, like ParameterLists in SplitDocumentationLeftSide */
	yOffsetLast?: number
	/** Horizontal offset for the first gap decoration (before first child) - used for vertical gaps */
	xOffsetFirst?: number
	/** Horizontal offset for middle gap decorations (between children) - used for vertical gaps */
	xOffsetMiddle?: number
	/** Horizontal offset for the last gap decoration (after last child) - used for vertical gaps */
	xOffsetLast?: number
	/** Left inset - positive values shrink from left, negative values extend (CSS px value) */
	leftInset?: number
	/** Right inset - positive values shrink from right, negative values extend (CSS px value) */
	rightInset?: number
	/** Debug color for the gap zone (CSS color value, only shown when DEBUG_SHOW_GAPS is true) */
	debugColor?: string
}

/**
 * Get gap zone configuration based on the child node type.
 * Gap zones are useful when trying to increase sensitivity for dropping
 * nodes between specific nodes.
 * For list items, indentation level can be provided to customize config per depth.
 * For MetricBlock inside MetricGrid, returns vertical gap config.
 * Return null to skip creating a gap zone for this child.
 */
export function gapZoneConfigByType(
	childNodeType: string,
	indentationLevel?: number,
	parentNodeType?: string,
): GapZoneConfig | null {
	// Special case: MetricBlock inside MetricGrid uses vertical gaps
	if (
		childNodeType === METRIC_BLOCK_NAME &&
		parentNodeType === MetricGrid.name
	) {
		return {
			height: "100%",
			width: "1rem",
			verticalGap: true,
			includeBeforeFirst: true,
			xOffsetFirst: 0,
			xOffsetMiddle: 0,
			xOffsetLast: 0,
			debugColor: "rgba(128, 0, 200, 0.3)",
		}
	}

	switch (childNodeType) {
		case Heading.name:
			return {
				height: "1.5rem",
				includeBeforeFirst: true,
				yOffsetFirst: 3,
				yOffsetMiddle: 3,
				debugColor: "rgba(128, 64, 0, 0.2)",
			}
		case Paragraph.name:
			return {
				height: "1.5rem",
				includeBeforeFirst: true,
				debugColor: "rgba(0, 64, 128, 0.2)",
			}
		case ImageBlock.name:
			return {
				height: "1.5rem",
				includeBeforeFirst: true,
				debugColor: "rgba(0, 64, 128, 0.2)",
			}
		case SplitDocumentation.name:
			return {
				height: "1.5rem",
				includeBeforeFirst: true,
				debugColor: "rgba(128, 64, 0, 0.2)",
			}
		case TITLED_CODE_BLOCK_NAME:
		case MERMAID_BLOCK_NAME:
		case FIGMA_BLOCK_NAME:
			return {
				height: "2rem",
				includeBeforeFirst: true,
				debugColor: "rgba(0, 255, 0, 0.2)",
			}
		case CODE_BLOCK_NAME:
			return {
				height: "1.5rem",
				includeBeforeFirst: true,
				yOffsetFirst: 3,
				yOffsetLast: -3,
				debugColor: "rgba(0, 255, 0, 0.2)",
			}
		case Blockquote.name:
			return {
				height: "1.5rem",
				includeBeforeFirst: true,
				debugColor: "rgba(128, 255, 64, 0.2)",
			}
		case ParameterList.name:
			return {
				height: "2rem",
				includeBeforeFirst: true,
				yOffsetFirst: 0,
				yOffsetMiddle: 4,
				yOffsetLast: 0,
				debugColor: "rgba(0, 0, 255, 0.2)",
			}
		case ParameterListItem.name:
			return {
				height: "2rem",
				includeBeforeFirst: true,
				yOffsetFirst: 0,
				yOffsetMiddle: 8,
				yOffsetLast: 0,
				debugColor: "rgba(255, 0, 0, 0.2)",
			}
		case CalloutBlock.name:
			return {
				height: "1.5rem",
				includeBeforeFirst: true,
				debugColor: "rgba(128, 0, 64, 0.2)",
			}
		case METRIC_BLOCK_NAME:
			return {
				height: "1.5rem",
				includeBeforeFirst: true,
				debugColor: "rgba(128, 0, 64, 0.2)",
			}
		case MetricGrid.name:
			return {
				height: "1.5rem",
				includeBeforeFirst: true,
				debugColor: "rgba(128, 64, 64, 0.2)",
			}
		case HorizontalRule.name:
			return {
				height: "1.5rem",
				includeBeforeFirst: true,
				debugColor: "rgba(0, 128, 128, 0.2)",
			}
		case BulletList.name:
			return {
				height: "1.5rem",
				includeBeforeFirst: true,
				debugColor: "rgba(64, 0, 64, 0.2)",
			}
		case OrderedList.name:
			return {
				height: "1.5rem",
				includeBeforeFirst: true,
				debugColor: "rgba(64, 128, 0, 0.2)",
			}
		case ListItem.name:
			if (indentationLevel !== undefined && indentationLevel > 0) {
				// Nested list items
				return {
					height: "1rem",
					includeBeforeFirst: true,
					yOffsetFirst: 2,
					yOffsetMiddle: 0,
					yOffsetLast: -6,
					leftInset: -15,
					debugColor: `rgba(128, 0, ${Math.min(255, 128 + indentationLevel * 40)}, 0.2)`,
				}
			}

			// Root level list items
			return {
				height: "1rem",
				includeBeforeFirst: true,
				yOffsetFirst: 2,
				yOffsetMiddle: 0,
				yOffsetLast: 0,
				debugColor: "rgba(128, 0, 128, 0.2)",
			}
		case TaskList.name:
			return {
				height: "1.5rem",
				includeBeforeFirst: true,
				debugColor: "rgba(128, 128, 0, 0.2)",
			}
		case TaskItem.name:
			if (indentationLevel !== undefined && indentationLevel > 0) {
				// Nested list items
				return {
					height: "1rem",
					includeBeforeFirst: true,
					yOffsetFirst: 2,
					yOffsetMiddle: 0,
					yOffsetLast: -6,
					debugColor: `rgba(128, 0, ${Math.min(255, 128 + indentationLevel * 40)}, 0.2)`,
				}
			}

			// Root level list items
			return {
				height: "1rem",
				includeBeforeFirst: true,
				yOffsetFirst: 2,
				yOffsetMiddle: 0,
				yOffsetLast: 0,
				debugColor: "rgba(128, 0, 128, 0.2)",
			}
		default:
			return null
	}
}

export interface CursorOffset {
	before: number // vertical offset when cursor is before the node
	after: number // vertical offset when cursor is after the node
	left?: number // horizontal inset from left edge
	right?: number // horizontal inset from right edge
}

/**
 * Get cursor offset based on the adjacent node type.
 * For list items, indentation level can be provided to customize offsets per depth.
 */
export function cursorOffsetByType(
	nodeTypeName: string | null,
	indentationLevel: number,
): CursorOffset {
	switch (nodeTypeName) {
		case SplitDocumentation.name:
			return { before: 10, after: -7 }
		case ListItem.name:
			if (indentationLevel > 0) {
				return { before: 3, after: -2, left: -20 }
			}

			return { before: 5.5, after: -2, left: -20 }
		case TaskItem.name:
			if (indentationLevel > 0) {
				return { before: 6, after: -1.5, left: 5 }
			}

			return { before: 6, after: -1, left: 5 }
		case TITLED_CODE_BLOCK_NAME:
		case MERMAID_BLOCK_NAME:
			return { before: 9, after: -7 }
		case CODE_BLOCK_NAME:
			return { before: 10, after: -7 }
		case CalloutBlock.name:
			return { before: 10, after: -7 }
		case Blockquote.name:
			return { before: 11, after: -7 }
		case METRIC_BLOCK_NAME:
			return { before: 10, after: -7 }
		case MetricGrid.name:
			return { before: 12, after: -7 }
		case Heading.name:
			return { before: 10, after: -7 }
		case Paragraph.name:
			return { before: 10, after: -7 }
		case BulletList.name:
		case OrderedList.name:
		case TaskList.name:
			return { before: 10, after: -7 }
		case HorizontalRule.name:
			return { before: 12, after: -12 }
		default:
			return { before: 7, after: -7 }
	}
}

// List node types for tracking indentation level
export const LIST_NODE_TYPES = new Set([
	BulletList.name,
	OrderedList.name,
	TaskList.name,
])

/**
 * Mapping from list item types to their default parent list types.
 * Used when dropping a list item where a list wrapper is needed and
 * no original list type is available.
 */
export const LIST_ITEM_TO_DEFAULT_LIST_TYPE: Map<string, string> = new Map([
	[ListItem.name, BulletList.name],
	[TaskItem.name, TaskList.name],
])

/**
 * Mapping from node types that require a wrapper to their wrapper types.
 * Used when dropping a node at a position that doesn't accept it directly.
 */
export const NODE_TO_WRAPPER_TYPE: Map<string, string> = new Map([
	[METRIC_BLOCK_NAME, MetricGrid.name],
])

/**
 * List item types that can appear in multiple list container types.
 * For these, we need to track the original parent list type when dragging.
 */
export const LIST_ITEM_TYPES = new Set([ListItem.name, TaskItem.name])

/**
 * Elements with this class will prevent drag handles from appearing,
 * including on parent elements.
 */
export const DRAG_HANDLE_IGNORE_CLASS = "drag-handle-ignore"

/**
 * Elements with this class will prevent drag handles from appearing
 * when hovering directly on them, but children can still trigger drag handles.
 * Use this for container elements where you want gaps to be ignored but
 * child elements should still be draggable.
 */
export const DRAG_HANDLE_IGNORE_SELF_CLASS = "drag-handle-ignore-self"

export const DRAGGABLE_NODE_TYPES = new Set([
	Paragraph.name,
	ImageBlock.name,
	Heading.name,
	ListItem.name,
	TaskItem.name,
	Blockquote.name,
	CalloutBlock.name,
	CODE_BLOCK_NAME,
	TITLED_CODE_BLOCK_NAME,
	MERMAID_BLOCK_NAME,
	FIGMA_BLOCK_NAME,
	SplitDocumentation.name,
	ParameterList.name,
	ParameterListItem.name,
	METRIC_BLOCK_NAME,
	HorizontalRule.name,
])

/**
 * Nodes that disable drag handles for descendants.
 * Empty set = disable ALL descendants.
 */
export const DRAG_DISABLED_EXCEPT_RULES: Map<string, Set<string>> = new Map([
	[TaskItem.name, new Set([TaskItem.name, ListItem.name])],
	[ListItem.name, new Set([TaskItem.name, ListItem.name])],
	[CalloutBlock.name, new Set([])],
	[METRIC_BLOCK_NAME, new Set([])],
	[MetricGrid.name, new Set([METRIC_BLOCK_NAME])],
	[Blockquote.name, new Set([])],
	[
		SplitDocumentation.name,
		new Set([
			ParameterList.name,
			ParameterListItem.name,
			CODE_BLOCK_NAME,
			TITLED_CODE_BLOCK_NAME,
			CalloutBlock.name,
			ListItem.name,
			TaskItem.name,
			Paragraph.name,
			METRIC_BLOCK_NAME,
		]),
	],
	[ParameterListItem.name, new Set([])],
	[TITLED_CODE_BLOCK_NAME, new Set([])],
])

export function handlePositionByNodeType(
	editor: Editor,
	pos: number,
): ComputePositionConfig & { yOffset: number; xOffset: number } {
	const node = editor.state.doc.nodeAt(pos)
	const res: ComputePositionConfig & { yOffset: number; xOffset: number } = {
		strategy: "absolute",
		placement: "left-start",
		yOffset: -1,
		xOffset: 0,
	}

	switch (node?.type.name) {
		case Heading.name:
			switch (node.attrs.level) {
				case 1:
					res.yOffset = 0
					break
				case 2:
					res.yOffset = 2
					break
				case 3:
					res.yOffset = 3
					break
			}

			break
		case ListItem.name:
			res.xOffset = -25
			break
		case ImageBlock.name:
		case FIGMA_BLOCK_NAME:
			res.yOffset = 4
			break
		case HorizontalRule.name:
			res.placement = "left"
			res.yOffset = 0
			break
		case TITLED_CODE_BLOCK_NAME:
			res.yOffset = 3
			break
	}

	return res
}

export function highlightOverlayByNodeType(
	editor: Editor,
	pos: number,
	target: HTMLElement,
): {
	extraLeft: number
	extraRight: number
	extraTop: number
	extraBottom: number
} {
	const node = editor.state.doc.nodeAt(pos)
	const res = {
		extraLeft: 5,
		extraRight: 5,
		extraTop: 5,
		extraBottom: 5,
	}

	switch (node?.type.name) {
		case ListItem.name:
			res.extraLeft = 30
			break
		case HorizontalRule.name:
			res.extraTop = 8
			res.extraBottom = 8
			break
		case ImageBlock.name: {
			const img =
				target.querySelector("img") ??
				target.firstElementChild?.querySelector?.("img")

			if (!img) {
				break
			}

			const rootRect = target.getBoundingClientRect()
			const rootWidth =
				rootRect.width ||
				target.offsetWidth ||
				target.parentElement?.getBoundingClientRect().width ||
				0

			const imgRect = img.getBoundingClientRect()
			const imgWidth =
				imgRect.width ||
				img.offsetWidth ||
				img.parentElement?.getBoundingClientRect().width ||
				0

			res.extraRight = imgWidth - rootWidth + 5

			break
		}
		case FIGMA_BLOCK_NAME: {
			const iframe = target.querySelector("iframe")

			if (!iframe) {
				break
			}

			const rootRect = target.getBoundingClientRect()
			const rootWidth =
				rootRect.width ||
				target.offsetWidth ||
				target.parentElement?.getBoundingClientRect().width ||
				0

			const iframeRect = iframe.getBoundingClientRect()
			const iframeWidth =
				iframeRect.width ||
				iframe.offsetWidth ||
				iframe.parentElement?.getBoundingClientRect().width ||
				0

			res.extraRight = iframeWidth - rootWidth + 7

			break
		}
	}

	return res
}
