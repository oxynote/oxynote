import Document from "@tiptap/extension-document"
import Paragraph from "@tiptap/extension-paragraph"
import Text from "@tiptap/extension-text"
import Heading from "@tiptap/extension-heading"
import Blockquote from "@tiptap/extension-blockquote"
import HorizontalRule from "@tiptap/extension-horizontal-rule"
import Bold from "@tiptap/extension-bold"
import Code from "@tiptap/extension-code"
import Italic from "@tiptap/extension-italic"
import Link from "@tiptap/extension-link"
import Strike from "@tiptap/extension-strike"
import Underline from "@tiptap/extension-underline"
import UniqueID from "@tiptap/extension-unique-id"
import {
	TaskItem,
	TaskList,
	BulletList,
	OrderedList,
	ListItem,
	ListKeymap,
} from "@tiptap/extension-list"
import type { Extensions } from "@tiptap/core"
import { nanoid } from "nanoid"

// Custom blocks
import { CalloutBlock } from "./callout.js"
import { MermaidBlock } from "./mermaid.js"
import { MetricBlock, MetricGrid } from "./metric.js"
import { ImageBlock } from "./image.js"
import { FigmaBlock } from "./figma.js"
import { CodeBlock, CodeBlockTitle, TitledCodeBlock } from "./code-block.js"
import {
	SplitDocumentation,
	SplitDocumentationLeftSide,
	SplitDocumentationRightSide,
} from "./split-documentation.js"
import {
	ParameterList,
	ParameterListHeader,
	ParameterListItem,
	ParameterListItemHeader,
	ParameterListItemHeaderType,
	ParameterListItemHeaderTitle,
} from "./parameter-list.js"

// Custom marks
import { AddedMark, DeletedMark, CommentMark } from "./marks.js"

const contentExtensionsWithIDs: Extensions = [
	Paragraph,
	Heading.configure({ levels: [1, 2, 3] }).extend({
		marks: `${CommentMark.name}`,
	}),
	HorizontalRule,
	Blockquote,
	ListItem,
	ListKeymap,
	BulletList,
	OrderedList,
	TaskList,
	TaskItem.configure({ nested: true }),
	CalloutBlock,
	SplitDocumentation,
	SplitDocumentationLeftSide,
	SplitDocumentationRightSide,
	TitledCodeBlock,
	MetricBlock,
	MetricGrid,
	ImageBlock,
	FigmaBlock,
	CodeBlock,
	MermaidBlock,
	CodeBlockTitle,
	ParameterList,
	ParameterListHeader,
	ParameterListItem,
	ParameterListItemHeader,
	ParameterListItemHeaderType,
	ParameterListItemHeaderTitle,
]

/**
 * Get all Tiptap extensions used in the bifrost editor
 * This matches the schema used by the frontend for document compatibility
 */
export function getEditorExtensions(): Extensions {
	return [
		...contentExtensionsWithIDs,

		// Base extensions
		Document,
		Text,

		// Text formatting marks
		Bold,
		Code,
		Italic,
		Strike,
		Underline,

		// Link
		Link.configure({
			openOnClick: false,
			HTMLAttributes: {
				class: "text-primary underline underline-offset-2 hover:text-primary/80 cursor-pointer",
			},
		}).extend({
			inclusive: false,
		}),

		// Custom marks
		CommentMark,
		AddedMark,
		DeletedMark,
		UniqueID.configure({
			types: contentExtensionsWithIDs.map((v) => v.name),
			attributeName: "uid",
			generateID: () => nanoid(),
		}),
	]
}

// Export all blocks and marks for individual use
export * from "./callout.js"
export * from "./code-block.js"
export * from "./split-documentation.js"
export * from "./parameter-list.js"
export * from "./marks.js"
