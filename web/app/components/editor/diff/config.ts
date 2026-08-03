import {
	CALLOUT_BLOCK_NAME,
	IMAGE_BLOCK_NAME,
	MERMAID_BLOCK_NAME,
	METRIC_BLOCK_NAME,
	METRIC_GRID_NAME,
	TITLED_CODE_BLOCK_NAME,
	SPLIT_DOCUMENTATION_NAME,
	SPLIT_DOCUMENTATION_LEFT_SIDE_NAME,
	SPLIT_DOCUMENTATION_RIGHT_SIDE_NAME,
	SPLIT_DOCUMENTATION_PARAMETER_LIST_NAME,
	SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_NAME,
	SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_HEADER_NAME,
} from "~/components/editor/blocks/node-names"
import Paragraph from "@tiptap/extension-paragraph"
import HorizontalRule from "@tiptap/extension-horizontal-rule"
import { COMMENT_MARK_NAME } from "~/components/editor/mark-names"
import { NODE_COMMENT_ID_ATTR } from "~/components/editor/comments/node-comment-extension"
import {
	BulletList,
	OrderedList,
	TaskList,
	ListItem,
	TaskItem,
} from "@tiptap/extension-list"
import type { MergeOptions } from "./compute-merged-document"
import type { OverlayPadding } from "./diff-decorations"

/**
 * node types treated as opaque — when modified, the whole node gets
 * a diff-modified class instead of character-level inline diffs.
 */
export const DEFAULT_OPAQUE_TYPES: string[] = [
	IMAGE_BLOCK_NAME,
	METRIC_BLOCK_NAME,
	HorizontalRule.name,
]

/**
 * node types that handle diff styling inside their node-view component
 * rather than via ProseMirror decorations on the root element.
 */
export const SELF_DECORATED_TYPES = new Set<string>([IMAGE_BLOCK_NAME])

// node types that receive a modifiedTextContent attribute during
// inline diff expansion. this lets their node-view components
// render the modified-only text (e.g. a mermaid preview) while
// the merged content keeps both added and removed marks for the
// inline diff view.
export const MODIFIED_TEXT_CONTENT_TYPES = new Set<string>([MERMAID_BLOCK_NAME])

export const DEFAULT_MERGE_OPTIONS: MergeOptions = {
	excludeMarks: [COMMENT_MARK_NAME],
	excludeAttributes: [NODE_COMMENT_ID_ATTR],
	useUidMatching: true,
	uidAttribute: "uid",
	unwrapTypes: [
		CALLOUT_BLOCK_NAME,
		SPLIT_DOCUMENTATION_NAME,
		SPLIT_DOCUMENTATION_LEFT_SIDE_NAME,
		SPLIT_DOCUMENTATION_RIGHT_SIDE_NAME,
		SPLIT_DOCUMENTATION_PARAMETER_LIST_NAME,
		SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_NAME,
		SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_HEADER_NAME,
		ListItem.name,
		TaskItem.name,
	],
	transparentTypes: [
		METRIC_GRID_NAME,
		BulletList.name,
		OrderedList.name,
		TaskList.name,
		TITLED_CODE_BLOCK_NAME,
	],
}

/**
 * node types that need an overlay widget instead of a simple class
 * decoration, because their visual indicators (bullets, numbers,
 * checkboxes) sit outside the node's padding box.
 */
export const OVERLAY_NODES = new Map<string, OverlayPadding>([
	[
		Paragraph.name,
		{ left: "0.2em", top: "0.2em", bottom: "0.2em", right: "0.2em" },
	],
	[
		ListItem.name,
		{ left: "1.825em", top: "0.2em", bottom: "0.2em", right: "0.2em" },
	],
	[
		TaskItem.name,
		{ left: "0.16em", top: "0.23em", bottom: "0.05em", right: "0.16em" },
	],
	[
		SPLIT_DOCUMENTATION_NAME,
		{ left: "0.2em", top: "0.3em", bottom: "0.3em", right: "0.2em" },
	],
	[
		SPLIT_DOCUMENTATION_PARAMETER_LIST_NAME,
		{ left: "0.2em", top: "0.2em", bottom: "0.2em", right: "0.2em" },
	],
	[
		SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_NAME,
		{ left: "0.2em", top: "0.2em", bottom: "0.2em", right: "0.2em" },
	],
])
