import CodeBlockLowlight, {
	type CodeBlockLowlightOptions,
} from "@tiptap/extension-code-block-lowlight"
import { mergeAttributes, Node } from "@tiptap/core"
import { createLowlight } from "lowlight"
import { CommentMark, AddedMark, DeletedMark } from "./marks.js"

const lowlight = createLowlight()

export const CodeBlockTitle = Node.create({
	name: "codeBlockTitle",
	content: "text*",
	marks: `${CommentMark.name} ${AddedMark.name} ${DeletedMark.name}`,
	group: "block",
	selectable: true,
	defining: true,
	isolating: true,
	parseHTML() {
		return [{ tag: `div[data-type="code-block-title"]` }]
	},
	renderHTML({ HTMLAttributes }) {
		return [
			"div",
			mergeAttributes(HTMLAttributes, {
				"data-type": "code-block-title",
			}),
			0,
		]
	},
})

export interface CodeBlockOptions extends CodeBlockLowlightOptions {
	type: "document" | "comment"
}

export const CodeBlock = CodeBlockLowlight.extend<CodeBlockOptions>({
	name: CodeBlockLowlight.name,
	defining: true,
	isolating: false,
	selectable: true,
	marks: `${CommentMark.name} ${AddedMark.name} ${DeletedMark.name}`,
	whitespace: "pre",
	parseHTML() {
		return [
			{
				tag: `pre[data-type="code-block"]`,
				preserveWhitespace: "full",
			},
		]
	},
	renderHTML({
		node,
		HTMLAttributes,
	}: {
		node: { attrs: Record<string, unknown> }
		HTMLAttributes: Record<string, any>
	}) {
		const language = node.attrs.language as string | null

		return [
			"pre",
			mergeAttributes(HTMLAttributes, {
				"data-type": "code-block",
			}),
			[
				"code",
				{
					class: language
						? `${this.options.languageClassPrefix ?? ""}${language}`
						: null,
				},
				0,
			],
		]
	},
	addOptions(): CodeBlockOptions {
		const parent = this.parent ? this.parent() : null
		return {
			languageClassPrefix: "language-",
			exitOnArrowDown: true,
			tabSize: 2,
			HTMLAttributes: {},
			...parent,
			defaultLanguage: null,
			enableTabIndentation: true,
			exitOnTripleEnter: false,
			lowlight: lowlight,
			type: "document",
		}
	},
})

export const TitledCodeBlock = Node.create({
	name: "titledCodeBlock",
	group: "block",
	content: `${CodeBlockTitle.name} ${CodeBlock.name}`,
	defining: true,
	isolating: true,
	selectable: false,
	parseHTML() {
		return [{ tag: `div[data-type="titled-code-block"]` }]
	},
	renderHTML({ HTMLAttributes }) {
		return [
			"div",
			mergeAttributes(HTMLAttributes, {
				"data-type": "titled-code-block",
			}),
			0,
		]
	},
})
