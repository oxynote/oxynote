import type { Range, Editor } from "@tiptap/core"
import {
	insertSplitDocumentation,
	SplitDocumentation,
	SplitDocumentationLeftSide,
	SplitDocumentationRightSide,
} from "../blocks/split-documentation"
import Heading from "@tiptap/extension-heading"
import type { EditorState } from "@tiptap/pm/state"
import { TextSelection } from "@tiptap/pm/state"
import { BulletList, OrderedList, TaskList } from "@tiptap/extension-list"
import { ParameterList } from "../blocks/split-documentation/parameter-list"
import { CalloutBlock } from "../blocks/callout"
import { insertMetricBlock } from "../blocks/metrics"
import { ImageBlock } from "../blocks/image"
import {
	extractHocuspocusProviderFromEditor,
	isRangeBeingEditedByOther,
} from "../collaboration"
import {
	CODE_BLOCK_NAME,
	FIGMA_BLOCK_NAME,
	METRIC_BLOCK_NAME,
	MERMAID_BLOCK_NAME,
} from "../blocks/node-names"
import { CodeBlock } from "../blocks/code-block"
import HorizontalRule from "@tiptap/extension-horizontal-rule"
import { MermaidBlock } from "../blocks/mermaid"
import { FigmaBlock } from "../blocks/figma"

export enum CommandGroup {
	Text = "text",
	List = "list",
	BasicBlock = "basic-block",
	PowerBlock = "power-block",
}

export function commandGroupSortIndex(group: CommandGroup): number {
	switch (group) {
		case CommandGroup.Text:
			return 0
		case CommandGroup.List:
			return 1
		case CommandGroup.BasicBlock:
			return 2
		case CommandGroup.PowerBlock:
			return 3
	}
}

export interface CommandData {
	editor: Editor
	range: Range
}

export interface CommandItem {
	title: string
	nodeType: string
	icon: string
	group: CommandGroup
	shortcut?: string
	disabled?: boolean
	specialContext?: boolean
	command: (data: CommandData) => void
}

export function allowSlashItemsByContext(state: EditorState): boolean {
	const res = whitelistSlashItemsByContext(state)
	return res.length > 0
}

// [] means no items are whitelisted
function whitelistSlashItemsByContext(state: EditorState): string[] {
	if (isCursorInsideTiptapNode(state, [SplitDocumentationLeftSide.name])) {
		if (
			isCursorInsideTiptapNode(state, [
				Heading.name,
				ParameterList.name,
				BulletList.name,
				OrderedList.name,
				TaskList.name,
			])
		) {
			return []
		}

		return [
			BulletList.name,
			OrderedList.name,
			TaskList.name,
			ParameterList.name,
			CalloutBlock.name,
		]
	} else if (isCursorInsideTiptapNode(state, [CalloutBlock.name])) {
		return [
			BulletList.name,
			OrderedList.name,
			TaskList.name,
			ParameterList.name,
		]
	} else if (
		isCursorInsideTiptapNode(state, [
			CODE_BLOCK_NAME,
			SplitDocumentationRightSide.name,
			BulletList.name,
			OrderedList.name,
			TaskList.name,
		])
	) {
		return []
	}

	return allNormalItems.map((v) => v.nodeType)
}

const allItems: CommandItem[] = [
	// text group
	{
		title: "Heading 1",
		nodeType: Heading.name,
		icon: "lucide:heading-1",
		group: CommandGroup.Text,
		shortcut: "#",
		command: ({ editor, range }: CommandData) => {
			const $pos = editor.state.doc.resolve(range.from)
			const provider = extractHocuspocusProviderFromEditor(editor)
			if (
				isRangeBeingEditedByOther(
					provider,
					editor.state.doc,
					$pos.before(),
					$pos.after(),
				)
			) {
				return
			}

			editor
				.chain()
				.focus()
				.deleteRange(range)
				.setNode("heading", { level: 1 })
				.run()
		},
	},
	{
		title: "Heading 2",
		nodeType: Heading.name,
		icon: "lucide:heading-2",
		group: CommandGroup.Text,
		shortcut: "##",
		command: ({ editor, range }: CommandData) => {
			const $pos = editor.state.doc.resolve(range.from)
			const provider = extractHocuspocusProviderFromEditor(editor)
			if (
				isRangeBeingEditedByOther(
					provider,
					editor.state.doc,
					$pos.before(),
					$pos.after(),
				)
			) {
				return
			}

			editor
				.chain()
				.focus()
				.deleteRange(range)
				.setNode("heading", { level: 2 })
				.run()
		},
	},
	{
		title: "Heading 3",
		nodeType: Heading.name,
		icon: "lucide:heading-3",
		group: CommandGroup.Text,
		shortcut: "###",
		command: ({ editor, range }: CommandData) => {
			const $pos = editor.state.doc.resolve(range.from)
			const provider = extractHocuspocusProviderFromEditor(editor)
			if (
				isRangeBeingEditedByOther(
					provider,
					editor.state.doc,
					$pos.before(),
					$pos.after(),
				)
			) {
				return
			}

			editor
				.chain()
				.focus()
				.deleteRange(range)
				.setNode("heading", { level: 3 })
				.run()
		},
	},
	// list group
	{
		title: "Bulleted list",
		nodeType: BulletList.name,
		icon: "lucide:list",
		group: CommandGroup.List,
		shortcut: "-",
		command: ({ editor, range }: CommandData) => {
			const $pos = editor.state.doc.resolve(range.from)
			const provider = extractHocuspocusProviderFromEditor(editor)
			if (
				isRangeBeingEditedByOther(
					provider,
					editor.state.doc,
					$pos.before(),
					$pos.after(),
				)
			) {
				return
			}

			editor.chain().focus().deleteRange(range).toggleBulletList().run()
		},
	},
	{
		title: "Numbered list",
		nodeType: OrderedList.name,
		icon: "lucide:list-ordered",
		group: CommandGroup.List,
		shortcut: "1.",
		command: ({ editor, range }: CommandData) => {
			const $pos = editor.state.doc.resolve(range.from)
			const provider = extractHocuspocusProviderFromEditor(editor)
			if (
				isRangeBeingEditedByOther(
					provider,
					editor.state.doc,
					$pos.before(),
					$pos.after(),
				)
			) {
				return
			}

			editor.chain().focus().deleteRange(range).toggleOrderedList().run()
		},
	},
	{
		title: "Checklist",
		nodeType: TaskList.name,
		icon: "lucide:list-checks",
		group: CommandGroup.List,
		shortcut: "[]",
		command: ({ editor, range }: CommandData) => {
			const $pos = editor.state.doc.resolve(range.from)
			const provider = extractHocuspocusProviderFromEditor(editor)
			if (
				isRangeBeingEditedByOther(
					provider,
					editor.state.doc,
					$pos.before(),
					$pos.after(),
				)
			) {
				return
			}

			editor.chain().focus().deleteRange(range).toggleTaskList().run()
		},
	},
	// basic block group
	{
		title: "Code Block",
		nodeType: CodeBlock.name,
		icon: "lucide:square-code",
		group: CommandGroup.BasicBlock,
		shortcut: "```",
		command: ({ editor, range }: CommandData) => {
			const { state, view } = editor
			const { schema } = state

			const codeBlockNode = schema.nodes[CodeBlock.name]
			if (!codeBlockNode) {
				return
			}

			const $pos = state.doc.resolve(range.from)

			const paragraphStart = $pos.before()
			const paragraphEnd = $pos.after()

			// block if another user is editing in this range
			const provider = extractHocuspocusProviderFromEditor(editor)
			if (
				isRangeBeingEditedByOther(
					provider,
					state.doc,
					paragraphStart,
					paragraphEnd,
				)
			) {
				return
			}

			const codeBlock = codeBlockNode.create()

			const tr = state.tr.delete(paragraphStart, paragraphEnd)
			tr.insert(paragraphStart, codeBlock)

			// place caret inside the new code block
			const resolvedPos = tr.doc.resolve(paragraphStart + 1)
			tr.setSelection(TextSelection.create(tr.doc, resolvedPos.pos))

			view.dispatch(tr)
		},
	},
	{
		title: "Callout",
		nodeType: CalloutBlock.name,
		icon: "lucide:square-m",
		group: CommandGroup.BasicBlock,
		shortcut: "!!",
		command: ({ editor, range }: CommandData) => {
			const { state, view } = editor
			const { schema } = state

			const calloutNode = schema.nodes[CalloutBlock.name]
			const paragraphNode = schema.nodes.paragraph

			if (!calloutNode || !paragraphNode) {
				return
			}

			const $pos = state.doc.resolve(range.from)

			const paragraphStart = $pos.before()
			const paragraphEnd = $pos.after()

			// block if another user is editing in this range
			const provider = extractHocuspocusProviderFromEditor(editor)
			if (
				isRangeBeingEditedByOther(
					provider,
					state.doc,
					paragraphStart,
					paragraphEnd,
				)
			) {
				return
			}

			const paragraph = paragraphNode.create()
			const callout = calloutNode.create(null, [paragraph])

			const tr = state.tr.delete(paragraphStart, paragraphEnd)
			tr.insert(paragraphStart, callout)
			tr.setSelection(TextSelection.near(tr.doc.resolve(paragraphStart + 2)))

			view.dispatch(tr)
		},
	},
	{
		title: "Image",
		nodeType: ImageBlock.name,
		icon: "lucide:image",
		group: CommandGroup.BasicBlock,
		command: ({ editor, range }: CommandData) => {
			const { state, view } = editor
			const { schema } = state

			const imageNode = schema.nodes[ImageBlock.name]
			if (!imageNode) {
				return
			}

			const $pos = state.doc.resolve(range.from)

			const paragraphStart = $pos.before()
			const paragraphEnd = $pos.after()

			// block if another user is editing in this range
			const provider = extractHocuspocusProviderFromEditor(editor)
			if (
				isRangeBeingEditedByOther(
					provider,
					state.doc,
					paragraphStart,
					paragraphEnd,
				)
			) {
				return
			}

			const image = imageNode.create()

			const tr = state.tr.delete(paragraphStart, paragraphEnd)
			tr.insert(paragraphStart, image)

			view.dispatch(tr)
		},
	},
	{
		title: "Figma Embed",
		nodeType: FigmaBlock.name,
		icon: "simple-icons:figma",
		group: CommandGroup.BasicBlock,
		command: ({ editor, range }: CommandData) => {
			const { state, view } = editor
			const { schema } = state

			const figmaNode = schema.nodes[FIGMA_BLOCK_NAME]
			if (!figmaNode) {
				return
			}

			const $pos = state.doc.resolve(range.from)

			const paragraphStart = $pos.before()
			const paragraphEnd = $pos.after()

			const provider = extractHocuspocusProviderFromEditor(editor)
			if (
				isRangeBeingEditedByOther(
					provider,
					state.doc,
					paragraphStart,
					paragraphEnd,
				)
			) {
				return
			}

			const block = figmaNode.create()

			const tr = state.tr.delete(paragraphStart, paragraphEnd)
			tr.insert(paragraphStart, block)

			view.dispatch(tr)
		},
	},
	{
		title: "Divider",
		nodeType: HorizontalRule.name,
		icon: "lucide:minus",
		group: CommandGroup.BasicBlock,
		shortcut: "---",
		command: ({ editor, range }: CommandData) => {
			const { state, view } = editor
			const { schema } = state

			const hrNode = schema.nodes[HorizontalRule.name]
			if (!hrNode) {
				return
			}

			const $pos = state.doc.resolve(range.from)

			const paragraphStart = $pos.before()
			const paragraphEnd = $pos.after()

			// block if another user is editing in this range
			const provider = extractHocuspocusProviderFromEditor(editor)
			if (
				isRangeBeingEditedByOther(
					provider,
					state.doc,
					paragraphStart,
					paragraphEnd,
				)
			) {
				return
			}

			const hr = hrNode.create()

			const tr = state.tr.delete(paragraphStart, paragraphEnd)
			tr.insert(paragraphStart, hr)

			view.dispatch(tr)
		},
	},
	// power block group
	{
		title: "Mermaid Diagram",
		nodeType: MermaidBlock.name,
		icon: "lucide:network",
		group: CommandGroup.PowerBlock,
		command: ({ editor, range }: CommandData) => {
			const { state, view } = editor
			const { schema } = state

			const mermaidNode = schema.nodes[MERMAID_BLOCK_NAME]
			if (!mermaidNode) {
				return
			}

			const $pos = state.doc.resolve(range.from)

			const paragraphStart = $pos.before()
			const paragraphEnd = $pos.after()

			const provider = extractHocuspocusProviderFromEditor(editor)
			if (
				isRangeBeingEditedByOther(
					provider,
					state.doc,
					paragraphStart,
					paragraphEnd,
				)
			) {
				return
			}

			const block = mermaidNode.create()

			const tr = state.tr.delete(paragraphStart, paragraphEnd)
			tr.insert(paragraphStart, block)

			const resolvedPos = tr.doc.resolve(paragraphStart + 1)
			tr.setSelection(TextSelection.create(tr.doc, resolvedPos.pos))

			view.dispatch(tr)
		},
	},
	{
		title: "Split Documentation",
		nodeType: SplitDocumentation.name,
		icon: "lucide:square-split-horizontal",
		group: CommandGroup.PowerBlock,
		shortcut: "||",
		command: ({ editor, range }: CommandData) => {
			// block if another user is editing in the paragraph being replaced
			const $pos = editor.state.doc.resolve(range.from)
			const provider = extractHocuspocusProviderFromEditor(editor)
			if (
				isRangeBeingEditedByOther(
					provider,
					editor.state.doc,
					$pos.before(),
					$pos.after(),
				)
			) {
				return
			}

			insertSplitDocumentation(editor, range)
		},
	},
	{
		title: "Live Metrics",
		nodeType: METRIC_BLOCK_NAME,
		icon: "lucide:chart-line",
		group: CommandGroup.PowerBlock,
		shortcut: "%%",
		command: ({ editor, range }: CommandData) => {
			// block if another user is editing in the paragraph being replaced
			const $pos = editor.state.doc.resolve(range.from)
			const provider = extractHocuspocusProviderFromEditor(editor)
			if (
				isRangeBeingEditedByOther(
					provider,
					editor.state.doc,
					$pos.before(),
					$pos.after(),
				)
			) {
				return
			}

			insertMetricBlock(editor, range)
		},
	},
]
const allNormalItems = allItems.filter((v) => !v.specialContext)

export function filterSlashItems({
	query,
	editor,
}: {
	query: string
	editor: Editor
}): CommandItem[] {
	// NOTE: the group order is determined by the group enum, but the item order
	// within the group is established here.
	return allItems.filter((item) => {
		const res = whitelistSlashItemsByContext(editor.state)
		return (
			res.includes(item.nodeType) &&
			item.title.toLowerCase().includes(query.toLowerCase())
		)
	})
}
