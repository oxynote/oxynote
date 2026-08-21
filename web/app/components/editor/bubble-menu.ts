import type { Editor } from "@tiptap/core"
import type { EditorState } from "@tiptap/pm/state"
import type { EditorView } from "@tiptap/pm/view"
import { isTextSelection } from "@tiptap/core"
import Bold from "@tiptap/extension-bold"
import Code from "@tiptap/extension-code"
import Italic from "@tiptap/extension-italic"
import Link from "@tiptap/extension-link"
import Strike from "@tiptap/extension-strike"
import Underline from "@tiptap/extension-underline"
import Heading from "@tiptap/extension-heading"
import {
	ParameterListHeader,
	ParameterListItemHeaderTitle,
	ParameterListItemHeaderType,
} from "./blocks/split-documentation/parameter-list"
import { COMMENT_MARK_NAME } from "./mark-names"
import { CODE_BLOCK_NAME, TITLED_CODE_BLOCK_NAME } from "./blocks/node-names"

// [] means no items are whitelisted
function whitelistBubbleMenuItemsByContext(state: EditorState): string[] {
	const allowComment = !isCursorInsideTiptapMark(state, COMMENT_MARK_NAME)

	const baseNodes = [
		Bold.name,
		Code.name,
		Italic.name,
		Link.name,
		Strike.name,
		Underline.name,
	]

	if (allowComment) {
		baseNodes.push(COMMENT_MARK_NAME)
	}

	if (
		isCursorInsideTiptapNode(state, [
			Heading.name,
			TITLED_CODE_BLOCK_NAME,
			CODE_BLOCK_NAME,
			ParameterListHeader.name,
			ParameterListItemHeaderTitle.name,
			ParameterListItemHeaderType.name,
		])
	) {
		if (allowComment) {
			return [COMMENT_MARK_NAME]
		}

		return []
	}

	return baseNodes
}

export function isBubbleMenuItemAllowedByContext(
	state: EditorState,
	itemName: string,
): boolean {
	return whitelistBubbleMenuItemsByContext(state).includes(itemName)
}

export function shouldShowBubbleMenu(opts: {
	editor: Editor
	state: EditorState
	view: EditorView
	from: number
	to: number
}): boolean {
	if (!whitelistBubbleMenuItemsByContext(opts.state).length) {
		return false
	}

	return defaultShouldShow(opts)
}

// Based on: https://github.com/ueberdosis/tiptap/blob/main/packages/extension-bubble-menu/src/bubble-menu-plugin.ts#L173
function defaultShouldShow({
	editor,
	view,
	state,
	from,
	to,
}: {
	editor: Editor
	state: EditorState
	view: EditorView
	from: number
	to: number
}): boolean {
	const { doc, selection } = state
	const { empty } = selection
	const isEmptyTextBlock =
		!doc.textBetween(from, to).length || !isTextSelection(selection)

	if (!view.hasFocus() || empty || isEmptyTextBlock || !editor.isEditable) {
		return false
	}

	return true
}
