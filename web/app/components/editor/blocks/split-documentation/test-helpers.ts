import type { Editor, JSONContent } from "@tiptap/core"
import { getSchema, Mark, Node as TiptapNode } from "@tiptap/core"
import Document from "@tiptap/extension-document"
import Paragraph from "@tiptap/extension-paragraph"
import Text from "@tiptap/extension-text"
import type { Node as PMNode, Schema } from "@tiptap/pm/model"
import { COMMENT_MARK_NAME } from "../../mark-names"
import {
	jsonDocBuilder,
	makeCursorEditor,
	nodeKeyboardShortcut as keyboardShortcut,
} from "~/components/editor/test-helpers"
import {
	SPLIT_DOCUMENTATION_NAME,
	SPLIT_DOCUMENTATION_PARAMETER_LIST_HEADER_NAME,
	SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_HEADER_NAME,
	SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_HEADER_TITLE_NAME,
	SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_HEADER_TYPE_NAME,
	SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_NAME,
	SPLIT_DOCUMENTATION_PARAMETER_LIST_NAME,
} from "../node-names"
import {
	ParameterList,
	ParameterListHeader,
	ParameterListItem,
	ParameterListItemHeader,
	ParameterListItemHeaderTitle,
	ParameterListItemHeaderType,
} from "./parameter-list"

// the real SplitDocumentation sides require headings, lists, callouts,
// and code blocks, which would drag half the editor into these suites.
// The keyboard handlers only check for an ancestor with the split
// documentation name, so a permissive stand-in with the real name is
// enough. Parameter lists stay optional in both stand-ins so deleting
// a whole list remains schema-valid, like in the real left side.
const SplitDocumentationStub = TiptapNode.create({
	name: SPLIT_DOCUMENTATION_NAME,
	group: "block",
	content: `${SPLIT_DOCUMENTATION_PARAMETER_LIST_NAME}*`,
})

const DocumentStub = Document.extend({
	content: `(block | ${SPLIT_DOCUMENTATION_PARAMETER_LIST_NAME})+`,
})

const CommentMarkStub = Mark.create({ name: COMMENT_MARK_NAME })

export const paramListSchema = getSchema([
	DocumentStub,
	Text,
	Paragraph,
	CommentMarkStub,
	SplitDocumentationStub,
	ParameterList,
	ParameterListItem,
	ParameterListItemHeader,
	ParameterListItemHeaderTitle,
	ParameterListItemHeaderType,
	ParameterListHeader,
])

function textContent(text: string): JSONContent[] {
	return text === "" ? [] : [{ type: "text", text }]
}

export function paramItem(
	title: string,
	type: string,
	body: string,
): JSONContent {
	return {
		type: SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_NAME,
		content: [
			{
				type: SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_HEADER_NAME,
				content: [
					{
						type: SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_HEADER_TITLE_NAME,
						content: textContent(title),
					},
					{
						type: SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_HEADER_TYPE_NAME,
						content: textContent(type),
					},
				],
			},
			{ type: "paragraph", content: textContent(body) },
		],
	}
}

export function paramList(
	header: string,
	...items: JSONContent[]
): JSONContent {
	return {
		type: SPLIT_DOCUMENTATION_PARAMETER_LIST_NAME,
		content: [
			{
				type: SPLIT_DOCUMENTATION_PARAMETER_LIST_HEADER_NAME,
				content: textContent(header),
			},
			...items,
		],
	}
}

export function splitDoc(...lists: JSONContent[]): JSONContent {
	return { type: SPLIT_DOCUMENTATION_NAME, content: lists }
}

export const docNode = jsonDocBuilder(paramListSchema)

export function makeEditor(doc: PMNode, cursorPos: number): Editor {
	return makeCursorEditor(doc, cursorPos).editor
}

// the suites in this directory drive every shortcut against the
// parameter list schema, so it is the default here
export function nodeKeyboardShortcut(
	extension: TiptapNode,
	editor: Editor,
	key: string,
	schema: Schema = paramListSchema,
): (props: { editor: Editor }) => boolean {
	return keyboardShortcut(extension, editor, key, schema)
}
