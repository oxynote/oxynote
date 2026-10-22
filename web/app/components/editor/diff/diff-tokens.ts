import type { JSONContent } from "@tiptap/core"
import type { Node as PMNode } from "@tiptap/pm/model"
import Link from "@tiptap/extension-link"
import Text from "@tiptap/extension-text"
import { COMMENT_MARK_NAME } from "~/components/editor/mark-names"

export interface DiffToken {
	/** single character */
	text: string
	/** marks on this character (JSON representation) */
	marks: JSONContent[]
}

// the marks the diff compares: no comments, and links by href only. No
// one edits the other link attributes, and old links carry a stale class.
function comparedMarks(marks: JSONContent[]): JSONContent[] {
	if (marks.length === 0) {
		return marks
	}
	return marks
		.filter((m) => m.type !== COMMENT_MARK_NAME)
		.map((m) =>
			m.type === Link.name
				? { type: m.type, attrs: { href: m.attrs?.href as unknown } }
				: m,
		)
}

/** extract character-level tokens from a JSON textblock node's inline content */
export function extractTokensFromJSON(node: JSONContent): DiffToken[] {
	const tokens: DiffToken[] = []

	if (!node.content) {
		return tokens
	}

	for (const child of node.content) {
		if (child.type !== Text.name || !child.text) {
			continue
		}

		const marks = comparedMarks(child.marks ?? [])
		for (const char of child.text) {
			tokens.push({ text: char, marks })
		}
	}

	return tokens
}

/** extract character-level tokens from a live ProseMirror textblock node */
export function extractTokensFromPMNode(node: PMNode): DiffToken[] {
	const tokens: DiffToken[] = []

	node.forEach((child) => {
		if (!child.isText || !child.text) {
			return
		}
		const marks = comparedMarks(
			child.marks.map((m) => m.toJSON() as JSONContent),
		)
		for (const char of child.text) {
			tokens.push({ text: char, marks })
		}
	})

	return tokens
}
