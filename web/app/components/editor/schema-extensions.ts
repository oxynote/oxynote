/**
 * shared schema extensions used by both the content editor and the
 * diff editor. extracted here to avoid duplication.
 */
import type { Extensions } from "@tiptap/core"
import { nanoid } from "nanoid"
import { cn } from "@/lib/utils"
import Heading from "@tiptap/extension-heading"
import Blockquote from "@tiptap/extension-blockquote"
import Paragraph from "@tiptap/extension-paragraph"
import HorizontalRule from "@tiptap/extension-horizontal-rule"
import Bold from "@tiptap/extension-bold"
import Code from "@tiptap/extension-code"
import Italic from "@tiptap/extension-italic"
import Link from "@tiptap/extension-link"
import Strike from "@tiptap/extension-strike"
import Underline from "@tiptap/extension-underline"
import {
	TaskItem,
	TaskList,
	BulletList,
	OrderedList,
	ListItem,
	ListKeymap,
} from "@tiptap/extension-list"
import {
	SplitDocumentation,
	SplitDocumentationLeftSide,
	SplitDocumentationRightSide,
} from "./blocks/split-documentation"
import { CalloutBlock } from "./blocks/callout"
import { CodeBlock, CodeBlockTitle, TitledCodeBlock } from "./blocks/code-block"
import { MetricBlock, MetricGrid } from "./blocks/metrics"
import { ImageBlock } from "./blocks/image"
import {
	ParameterList,
	ParameterListHeader,
	ParameterListItem,
	ParameterListItemHeader,
	ParameterListItemHeaderType,
	ParameterListItemHeaderTitle,
} from "./blocks/split-documentation/parameter-list"
import UniqueID from "./tiptap-utils/unique-id"
import { CommentMark, type CommentMarkOptions } from "./comments/comment-mark"
import { COMMENT_MARK_NAME } from "./mark-names"
import { defaultContentPlaceholder } from "./placeholder"
import { MermaidBlock } from "./blocks/mermaid"
import { FigmaBlock } from "./blocks/figma"

/**
 * UniqueID extension with DOM id attribute rendering.
 * nodes get both `data-uid` and `id` HTML attributes.
 */
export const UniqueIDWithDomId = UniqueID.extend({
	addGlobalAttributes() {
		return [
			{
				types: this.options.types,
				attributes: {
					[this.options.attributeName]: {
						default: null,
						parseHTML: (element) =>
							element.getAttribute(`data-${this.options.attributeName}`) ||
							element.getAttribute("id"),
						renderHTML: (attributes) => {
							const value = attributes[this.options.attributeName] as
								| string
								| null
								| undefined
							if (!value) {
								return {}
							}

							return {
								[`data-${this.options.attributeName}`]: value,
								id: value,
							}
						},
					},
				},
			},
		]
	},
})

/**
 * schema-level extensions that define content node types and the
 * UniqueID extension. shared between the normal content editor and
 * the diff editor so that all custom node views render correctly in
 * both.
 */
const nodeExtensions: Extensions = [
	Paragraph,
	Heading.configure({ levels: [1, 2, 3] }).extend({
		marks: COMMENT_MARK_NAME,
	}),
	Bold,
	Code,
	Italic,
	Link.configure({
		openOnClick: false,
		HTMLAttributes: {
			class:
				"text-primary underline underline-offset-2 hover:text-primary/80 cursor-pointer",
		},
	}).extend({
		inclusive: false,
	}),
	Strike,
	Underline,
	HorizontalRule.extend({
		addInputRules() {
			const rules = this.parent?.() ?? []
			return rules.map((rule) => preventTiptapInputRuleInNode(rule, [], true))
		},
	}),
	Blockquote,
	ListItem,
	ListKeymap,
	BulletList,
	OrderedList,
	TaskList,
	TaskItem.configure({ nested: true }),
	CalloutBlock,
	MetricGrid,
	MetricBlock,
	ImageBlock,
	MermaidBlock,
	FigmaBlock,
	SplitDocumentation,
	SplitDocumentationLeftSide,
	SplitDocumentationRightSide,
	TitledCodeBlock,
	CodeBlock,
	CodeBlockTitle,
	ParameterList,
	ParameterListHeader,
	ParameterListItem,
	ParameterListItemHeader,
	ParameterListItemHeaderType,
	ParameterListItemHeaderTitle,
]

export function contentExtensionsWithIDs(
	t: (i18nPath: string) => string,
	isEditingDisabled: MaybeRef<boolean>,
	commentMarkOptions: CommentMarkOptions,
): Extensions {
	return [
		...nodeExtensions,
		CommentMark.configure(commentMarkOptions),
		UniqueIDWithDomId.configure({
			types: nodeExtensions.map((v) => v.name),
			attributeName: "uid",
			generateID: (): string => nanoid(),
		}),
		defaultContentPlaceholder(t, isEditingDisabled),
	]
}

/**
 * shared prose/layout classes applied to the .tiptap editor element.
 * used by both the content editor and the diff editor.
 */
export const editorProseClass = cn(
	"focus:outline-none w-full max-w-none text-2base bg-transparent",
	"[&>*]:mx-5 lg:[&>*]:mx-12.5 mb-[0.8rem] lg:mb-[2rem]",
	"prose prose-neutral dark:prose-invert",
	"prose-blockquote:[quotes:none] prose-blockquote:not-italic prose-blockquote:font-normal",
	"prose-h1:text-[1.5em] prose-h2:text-[1.3em] prose-h3:text-[1.1em]",
	"prose-p:break-normal prose-p:wrap-break-word prose-li:break-normal prose-li:wrap-break-word",
)
