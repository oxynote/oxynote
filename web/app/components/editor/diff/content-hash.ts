import type { JSONContent } from "@tiptap/core"
import { COMMENT_MARK_NAME } from "~/components/editor/mark-names"
import { NODE_COMMENT_ID_ATTR } from "~/components/editor/comments/node-comment-extension"

export interface HashOptions {
	/** mark types to strip before hashing (e.g. ['comment']) */
	excludeMarks?: string[]
	/** node attribute keys to strip before hashing (e.g. ['nodeCommentId']) */
	excludeAttributes?: string[]
}

const DEFAULT_OPTIONS: HashOptions = {
	excludeMarks: [COMMENT_MARK_NAME],
	excludeAttributes: [NODE_COMMENT_ID_ATTR],
}

/**
 * strip comment-related marks and attributes from a node tree so they
 * don't produce false positives in the diff. also merges adjacent text
 * nodes that become identical after mark stripping.
 */
function stripExcluded(node: JSONContent, options: HashOptions): JSONContent {
	const { excludeMarks = [], excludeAttributes = [] } = options
	const result: JSONContent = { ...node }

	// strip excluded attributes
	if (result.attrs) {
		const attrs = { ...result.attrs }
		for (const key of excludeAttributes) {
			delete attrs[key]
		}
		result.attrs = attrs
	}

	// strip excluded marks
	if (result.marks) {
		result.marks = result.marks.filter((m) => !excludeMarks.includes(m.type))
		if (result.marks.length === 0) {
			delete result.marks
		}
	}

	// recurse into children and merge adjacent text nodes
	if (result.content) {
		const stripped = result.content.map((child) =>
			stripExcluded(child, options),
		)
		result.content = mergeAdjacentTextNodes(stripped)
	}

	return result
}

/**
 * merge consecutive text nodes that have identical marks (or no marks).
 * this prevents false diffs caused by comment mark boundaries splitting
 * text into multiple nodes.
 */
function mergeAdjacentTextNodes(nodes: JSONContent[]): JSONContent[] {
	if (nodes.length <= 1) {
		return nodes
	}

	const merged: JSONContent[] = nodes.slice(0, 1)
	for (const curr of nodes.slice(1)) {
		const prev = merged[merged.length - 1]

		if (
			prev?.type === "text" &&
			curr.type === "text" &&
			marksEqual(prev.marks, curr.marks)
		) {
			merged[merged.length - 1] = {
				...prev,
				text: (prev.text ?? "") + (curr.text ?? ""),
			}
		} else {
			merged.push(curr)
		}
	}

	return merged
}

function marksEqual(a: JSONContent["marks"], b: JSONContent["marks"]): boolean {
	if (!a?.length && !b?.length) {
		return true
	}

	if (!a?.length || !b?.length) {
		return false
	}

	if (a.length !== b.length) {
		return false
	}

	return jsonStableStringify(a) === jsonStableStringify(b)
}

/**
 * FNV-1a hash producing a hex string. fast, non-cryptographic, good
 * distribution for short-to-medium strings.
 */
function fnv1a(str: string): string {
	let hash = 0x811c9dc5
	for (let i = 0; i < str.length; i++) {
		hash ^= str.charCodeAt(i)
		hash = Math.imul(hash, 0x01000193)
	}

	return (hash >>> 0).toString(16)
}

/**
 * compute a content hash for a top-level block node. the hash is based
 * on the node's JSON representation after stripping excluded marks and
 * attributes, so that comment metadata doesn't cause false diffs.
 */
export function hashBlockContent(
	node: JSONContent,
	options: HashOptions = DEFAULT_OPTIONS,
): string {
	const stripped = stripExcluded(node, options)
	return fnv1a(jsonStableStringify(stripped))
}
