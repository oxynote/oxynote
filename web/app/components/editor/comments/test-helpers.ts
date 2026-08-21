import type { CommandProps, Editor } from "@tiptap/core"
import { Schema, type Node as PMNode } from "@tiptap/pm/model"
import { EditorState, type Transaction } from "@tiptap/pm/state"
import { COMMENT_MARK_NAME, DIFF_TEXT_ADDED_MARK_NAME } from "../mark-names"
import { docBuilder, makeDispatchEditor } from "../test-helpers"
import type { DiffStatus } from "../diff/position-map"
import type { DocumentComment } from "~/utils/api/comment"

// minimal schema mirroring the pieces of the editor schema the comment
// helpers touch: the comment mark with its real name and semantics
// (non-inclusive, coexisting with other marks), the diff added mark,
// and block nodes carrying the nodeCommentId / uid / diffStatus attrs
const schema = new Schema<
	"doc" | "paragraph" | "wrapper" | "hardBreak" | "text",
	typeof COMMENT_MARK_NAME | typeof DIFF_TEXT_ADDED_MARK_NAME
>({
	nodes: {
		doc: { content: "block+" },
		paragraph: {
			group: "block",
			content: "inline*",
			attrs: {
				nodeCommentId: { default: null },
				uid: { default: null },
				diffStatus: { default: null },
			},
		},
		wrapper: {
			group: "block",
			content: "block+",
			attrs: {
				nodeCommentId: { default: null },
				uid: { default: null },
				diffStatus: { default: null },
			},
		},
		hardBreak: { group: "inline", inline: true },
		text: { group: "inline" },
	},
	marks: {
		[COMMENT_MARK_NAME]: {
			attrs: { commentId: { default: null } },
			inclusive: false,
			excludes: "",
		},
		[DIFF_TEXT_ADDED_MARK_NAME]: {},
	},
})

export interface BlockAttrs {
	nodeCommentId?: string | null
	uid?: string | null
	diffStatus?: DiffStatus | null
}

export const doc = docBuilder(schema)

export function paragraph(
	attrs: BlockAttrs | null,
	...children: PMNode[]
): PMNode {
	return schema.nodes.paragraph.create(attrs, children)
}

export function wrapper(
	attrs: BlockAttrs | null,
	...children: PMNode[]
): PMNode {
	return schema.nodes.wrapper.create(attrs, children)
}

export function hardBreak(): PMNode {
	return schema.nodes.hardBreak.create()
}

export function text(str: string): PMNode {
	return schema.text(str)
}

export function commented(str: string, commentId: string): PMNode {
	return schema.text(str, [
		schema.marks[COMMENT_MARK_NAME].create({ commentId }),
	])
}

// a text node carrying both marks, standing in for a comment run that
// another mark splits into several text nodes
export function commentedAdded(str: string, commentId: string): PMNode {
	return schema.text(str, [
		schema.marks[COMMENT_MARK_NAME].create({ commentId }),
		schema.marks[DIFF_TEXT_ADDED_MARK_NAME].create(),
	])
}

export function added(str: string): PMNode {
	return schema.text(str, [schema.marks[DIFF_TEXT_ADDED_MARK_NAME].create()])
}

export function makeEditor(docNode: PMNode): {
	editor: Editor
	dispatched: Transaction[]
	state: () => EditorState
} {
	return makeDispatchEditor(EditorState.create({ doc: docNode }))
}

// run a raw tiptap command the way the command manager does: hand it
// the state's own transaction plus a no-op dispatch flag, then apply
// the transaction when the command ran with dispatch and produced
// steps. tiptap's real dispatch is also just a truthiness marker —
// the manager dispatches the shared transaction itself. Commands that
// delegate to a command chain receive the stub given in chain.
export function runCommand(
	command: ((props: CommandProps) => boolean) | undefined,
	state: EditorState,
	{
		dispatch = true,
		chain,
	}: { dispatch?: boolean; chain?: () => unknown } = {},
): { result: boolean; state: EditorState; tr: Transaction } {
	if (!command) {
		throw new Error("command is not defined")
	}

	const tr = state.tr
	const result = command({
		state,
		tr,
		dispatch: dispatch ? () => undefined : undefined,
		chain,
	} as unknown as CommandProps)

	const next = dispatch && tr.steps.length > 0 ? state.apply(tr) : state

	return { result, state: next, tr }
}

// compresses a doc into [text, commentId] pairs for readable
// assertions on comment marks
export function commentShape(docNode: PMNode): [string, string | null][] {
	const shape: [string, string | null][] = []

	docNode.descendants((node) => {
		if (!node.isText) {
			return
		}

		const mark = node.marks.find((m) => m.type.name === COMMENT_MARK_NAME)
		shape.push([
			node.text ?? "",
			mark ? (mark.attrs.commentId as string) : null,
		])
	})

	return shape
}

// compresses a doc into [textContent, nodeCommentId] pairs, one per
// block node, for readable assertions on node comments
export function nodeCommentShape(docNode: PMNode): [string, string | null][] {
	const shape: [string, string | null][] = []

	docNode.descendants((node) => {
		if (!node.isBlock) {
			return
		}

		shape.push([
			node.textContent,
			(node.attrs.nodeCommentId as string | null) ?? null,
		])
	})

	return shape
}

export function makeComment(
	overrides: Partial<DocumentComment> & Pick<DocumentComment, "id">,
): DocumentComment {
	return {
		organizationId: "org-1",
		documentId: "doc-1",
		branchId: "branch-1",
		anchorBlockId: "block-1",
		userId: "user-1",
		resolved: false,
		content: {},
		createdAt: "2026-01-01T00:00:00Z",
		...overrides,
	}
}
