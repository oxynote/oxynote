import { cn } from "@/lib/utils"
import Document from "@tiptap/extension-document"
import type { Editor } from "@tiptap/core"
import {
	TaskItem,
	TaskList,
	BulletList,
	OrderedList,
	ListItem,
	ListKeymap,
} from "@tiptap/extension-list"
import Heading from "@tiptap/extension-heading"
import Blockquote from "@tiptap/extension-blockquote"
import Paragraph from "@tiptap/extension-paragraph"
import Text from "@tiptap/extension-text"
import HorizontalRule from "@tiptap/extension-horizontal-rule"
import Underline from "@tiptap/extension-underline"
import Bold from "@tiptap/extension-bold"
import Code from "@tiptap/extension-code"
import Italic from "@tiptap/extension-italic"
import Strike from "@tiptap/extension-strike"
import Link from "@tiptap/extension-link"
import { CodeBlock } from "../blocks/code-block"
import { deletePendingCommentMarks } from "./comment-mark"
import { deletePendingNodeComments } from "./node-comment-extension"
import { COMMENT_MARK_NAME } from "../mark-names"
import { DIFF_COMMENT_TX_META } from "../diff/diff-content-lock"

export const PENDING_COMMENT_ID = "pending"

let pendingCounter = 0

export function createPendingCommentId(userId: string): string {
	return `${PENDING_COMMENT_ID}-${userId}-${++pendingCounter}`
}

export function isPendingCommentId(commentId: string): boolean {
	return commentId.startsWith(`${PENDING_COMMENT_ID}-`)
}

export function pendingCommentBelongsToActiveUser(
	commentId: string,
	activeUserId: string,
): boolean {
	return commentId.startsWith(`${PENDING_COMMENT_ID}-${activeUserId}-`)
}

export const CommentExtensions = [
	Document,
	Text,
	Paragraph,
	Heading.configure({ levels: [1, 2, 3] }).extend({
		marks: "",
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
	HorizontalRule,
	Blockquote,
	ListItem,
	ListKeymap,
	BulletList,
	OrderedList,
	TaskList,
	TaskItem.configure({ nested: true }),
	CodeBlock.extend({
		marks: "",
	}).configure({
		type: "comment",
	}),
]

export const CommentClass = cn(
	"prose prose-neutral dark:prose-invert focus:outline-none w-full max-w-none text-2base bg-transparent",
	"prose-blockquote:[quotes:none] prose-blockquote:not-italic prose-blockquote:font-normal prose-blockquote:m-0",
	"prose-h1:text-[1.5em] prose-h2:text-[1.3em] prose-h3:text-[1.1em]",
)

export function deletePendingCommentData(
	editor: Editor,
	activeUserId: string,
	onDelete?: (id: string) => void,
	isDiffEditor?: boolean,
	excludeId?: string | null,
) {
	if (!activeUserId) {
		return
	}

	deletePendingCommentMarks(
		editor,
		activeUserId,
		onDelete,
		isDiffEditor,
		excludeId,
	)
	deletePendingNodeComments(
		editor,
		activeUserId,
		onDelete,
		isDiffEditor,
		excludeId,
	)
}

// deletes the comment mark or node attribute for a specific comment
// from the given editor after a comment is resolved or deleted.
export function deleteCommentFromEditor(
	editor: Editor,
	commentId: string,
	isTextComment: boolean,
	isDiffEditor?: boolean,
) {
	const { state, view } = editor
	const { tr } = state

	if (isTextComment) {
		const markType = state.schema.marks[COMMENT_MARK_NAME]
		if (!markType) return

		state.doc.descendants((node, pos) => {
			if (!node.isText) return

			const mark = node.marks.find(
				(m) => m.type === markType && m.attrs.commentId === commentId,
			)

			if (mark) {
				tr.removeMark(pos, pos + node.nodeSize, mark)
			}
		})
	} else {
		state.doc.descendants((node, pos) => {
			if (node.attrs.nodeCommentId === commentId) {
				tr.setNodeMarkup(pos, undefined, {
					...node.attrs,
					nodeCommentId: null,
				})
			}
		})
	}

	if (tr.steps.length > 0) {
		if (isDiffEditor) {
			tr.setMeta(DIFF_COMMENT_TX_META, true)
		}

		view.dispatch(tr)
	}
}
