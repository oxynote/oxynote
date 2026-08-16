// expands modified textblocks in a merged document so that deleted
// text is injected as real content with a diffTextRemoved mark (and
// inserted text gets a diffTextAdded mark). this gives deleted text
// real ProseMirror positions, making it selectable and commentable.
//
// textblocks whose content includes non-text inline nodes (e.g.
// hard breaks) are left unexpanded — the DiffDecorations plugin
// handles them with the existing widget fallback.

import type { JSONContent } from "@tiptap/core"
import Text from "@tiptap/extension-text"
import { DiffStatus } from "./position-map"
import { extractTokensFromJSON, type DiffToken } from "./diff-tokens"
import { computeTokenDiff } from "./diff-ops"
import {
	DIFF_TEXT_ADDED_MARK_NAME,
	DIFF_TEXT_REMOVED_MARK_NAME,
} from "~/components/editor/mark-names"
import { MODIFIED_TEXT_CONTENT_TYPES } from "./config"

// extract character-level tokens preserving all marks (including
// comment marks that extractTokensFromJSON normally strips). used
// when building content for the new (modified) side so that active
// comments survive the expansion.
function extractTokensPreservingMarks(node: JSONContent): DiffToken[] {
	const tokens: DiffToken[] = []

	if (!node.content) {
		return tokens
	}

	for (const child of node.content) {
		if (child.type !== Text.name || !child.text) {
			continue
		}

		const marks = child.marks ?? []

		for (const char of child.text) {
			tokens.push({ text: char, marks })
		}
	}

	return tokens
}

// check whether a JSON node has only text children (or is empty).
function hasOnlyTextContent(node: JSONContent): boolean {
	if (!node.content || node.content.length === 0) {
		return true
	}

	return node.content.every((c) => c.type === Text.name)
}

// build JSON text nodes from diff tokens, grouping consecutive
// tokens with identical mark sets into single text nodes. when
// extraMark is provided it is appended to every token's marks.
function tokensToJSON(
	tokens: DiffToken[],
	extraMark?: JSONContent,
): JSONContent[] {
	const result: JSONContent[] = []
	let currentText = ""
	let currentMarks: JSONContent[] = []
	let currentKey = ""

	function flush() {
		if (currentText.length === 0) {
			return
		}

		const node: JSONContent = { type: "text", text: currentText }

		if (currentMarks.length > 0) {
			node.marks = currentMarks as { type: string }[]
		}

		result.push(node)
		currentText = ""
	}

	// consecutive tokens from the same source text node share the same
	// marks array reference (assigned in extractTokensFromJSON /
	// extractTokensPreservingMarks). we track the last reference with
	// prevTokenMarks so we can skip both the spread+push copy and the
	// jsonStableStringify serialization when the reference hasn't
	// changed. for a 500-char run from one text node this reduces 500
	// stringify calls and 500 array copies to 1 each.
	//
	// this is safe because extraMark is constant for the entire call —
	// if token.marks is the same reference, the spread+push would
	// produce an equivalent array and the same serialized key.
	let prevTokenMarks: JSONContent[] | null = null

	for (const token of tokens) {
		if (token.marks !== prevTokenMarks) {
			const marks = [...token.marks]

			if (extraMark) {
				marks.push(extraMark)
			}

			const key = marks.length > 0 ? jsonStableStringify(marks) : ""
			prevTokenMarks = token.marks

			if (key !== currentKey) {
				flush()
				currentMarks = marks
				currentKey = key
			}
		}

		currentText += token.text
	}

	flush()
	return result
}

// try to expand a single modified textblock. returns the expanded
// node with merged content and oldNode cleared, or null if the node
// cannot be expanded (non-text inline content or atom/non-textblock
// nodes with no content).
function expandTextblock(node: JSONContent): JSONContent | null {
	const oldNodeJSON = node.attrs?.oldNode as JSONContent | undefined
	if (!oldNodeJSON) {
		return null
	}

	// skip atom / leaf nodes that have no content — they are not
	// textblocks and should keep their oldNode for component-level
	// diffing (e.g. metricBlock).
	const nodeEmpty = !node.content || node.content.length === 0
	const oldEmpty = !oldNodeJSON.content || oldNodeJSON.content.length === 0

	if (nodeEmpty && oldEmpty) {
		return null
	}

	if (!hasOnlyTextContent(node) || !hasOnlyTextContent(oldNodeJSON)) {
		return null
	}

	// stripped tokens for comparisons (comment marks removed)
	const oldTokens = extractTokensFromJSON(oldNodeJSON)
	const newTokensDiff = extractTokensFromJSON(node)

	// full tokens for building the new-side content (preserves comments)
	const newTokensFull = extractTokensPreservingMarks(node)

	const ops = computeTokenDiff(oldTokens, newTokensDiff)

	const mergedContent: JSONContent[] = []
	let oldIdx = 0
	let newIdx = 0

	for (const op of ops) {
		const len = op.tokens.length

		if (op.type === "equal") {
			// use full marks from the new side to preserve comment marks
			mergedContent.push(
				...tokensToJSON(newTokensFull.slice(newIdx, newIdx + len)),
			)
			oldIdx += len
			newIdx += len
		} else if (op.type === "insert") {
			mergedContent.push(
				...tokensToJSON(newTokensFull.slice(newIdx, newIdx + len), {
					type: DIFF_TEXT_ADDED_MARK_NAME,
				}),
			)
			newIdx += len
		} else {
			// deleted text uses old tokens (stripped of comment marks —
			// comments from the published version should not appear)
			mergedContent.push(
				...tokensToJSON(oldTokens.slice(oldIdx, oldIdx + len), {
					type: DIFF_TEXT_REMOVED_MARK_NAME,
				}),
			)
			oldIdx += len
		}
	}

	const expandedAttrs: Record<string, unknown> = {
		...node.attrs,
		oldNode: null,
	}

	if (node.type && MODIFIED_TEXT_CONTENT_TYPES.has(node.type)) {
		expandedAttrs.modifiedTextContent = newTokensFull
			.map((t) => t.text)
			.join("")
	}

	return {
		...node,
		attrs: expandedAttrs,
		content: mergedContent.length > 0 ? mergedContent : undefined,
	}
}

// recursively walk the tree and expand modified textblocks.
function expandNode(node: JSONContent): JSONContent {
	if (node.attrs?.diffStatus === DiffStatus.Modified && node.attrs.oldNode) {
		const expanded = expandTextblock(node)

		if (expanded) {
			return expanded
		}
	}

	if (!node.content) {
		return node
	}

	let changed = false
	const newContent = node.content.map((child) => {
		const result = expandNode(child)

		if (result !== child) {
			changed = true
		}

		return result
	})

	// eslint-disable-next-line @typescript-eslint/no-unnecessary-condition -- changed is set inside the synchronous map callback, which TS narrowing does not track
	return changed ? { ...node, content: newContent } : node
}

// expand inline diffs in all modified textblocks of a merged document.
export function expandInlineDiffs(doc: JSONContent): JSONContent {
	return expandNode(doc)
}
