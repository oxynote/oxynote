import { diffWordsWithSpace, wordsWithSpaceDiff } from "diff"
import Link from "@tiptap/extension-link"
import type { DiffToken } from "./diff-tokens"

export type DiffOp =
	| { type: "equal" | "insert" | "delete"; tokens: DiffToken[] }
	// same text, new link target
	| { type: "relink"; tokens: DiffToken[]; oldHref: string }

// how one character's marks changed
enum MarkChange {
	None,
	Relinked,
	Other,
}

// compute a word-level diff between old and new token arrays.
// uses jsdiff's diffWordsWithSpace for clean, contiguous change
// blocks that respect word boundaries, then refines "equal" regions
// to detect mark changes (e.g. bold/italic added or removed) —
// characters whose text matches but whose marks differ are emitted
// as delete+insert pairs so they appear as changed in the diff. A
// changed link target alone is a relink, since both sides look alike.
export function computeTokenDiff(
	oldTokens: DiffToken[],
	newTokens: DiffToken[],
): DiffOp[] {
	const oldText = oldTokens.map((t) => t.text).join("")
	const newText = newTokens.map((t) => t.text).join("")

	const changes = diffWordsWithSpace(oldText, newText)
	const ops: DiffOp[] = []

	let oldIdx = 0
	let newIdx = 0

	for (const change of changes) {
		// spread iterates code points (not UTF-16 units) so emoji
		// and other surrogate pairs count as one, matching DiffToken
		// granularity
		// eslint-disable-next-line @typescript-eslint/no-misused-spread -- code point granularity is exactly what DiffToken uses
		const len = [...change.value].length

		if (change.removed) {
			ops.push({
				type: "delete",
				tokens: oldTokens.slice(oldIdx, oldIdx + len),
			})
			oldIdx += len
		} else if (change.added) {
			ops.push({
				type: "insert",
				tokens: newTokens.slice(newIdx, newIdx + len),
			})
			newIdx += len
		} else {
			// split "equal" regions at mark boundaries so that mark
			// changes (bold added, italic removed, etc.) show up as
			// delete+insert pairs
			splitEqualByMarks(
				ops,
				oldTokens,
				newTokens,
				oldIdx,
				newIdx,
				len,
				change.value,
			)
			oldIdx += len
			newIdx += len
		}
	}

	return ops
}

// compare two mark arrays for equality. uses reference check first
// (consecutive tokens from the same text node share the same array),
// then falls back to jsonStableStringify for different references.
function marksMatch(a: DiffToken["marks"], b: DiffToken["marks"]): boolean {
	if (a === b) {
		return true
	}

	if (a.length !== b.length) {
		return false
	}

	if (a.length === 0) {
		return true
	}

	return jsonStableStringify(a) === jsonStableStringify(b)
}

// walk an "equal" region and detect mark changes. A character whose
// link target alone changed is relinked. Any other mark change is
// expanded to cover full words (split on whitespace) so that
// mark-only changes produce word-level diffs consistent with how
// diffWordsWithSpace treats text changes.
function splitEqualByMarks(
	ops: DiffOp[],
	oldTokens: DiffToken[],
	newTokens: DiffToken[],
	oldStart: number,
	newStart: number,
	len: number,
	text: string,
): void {
	const changes = Array.from({ length: len }, (_, i) =>
		markChange(
			// eslint-disable-next-line @typescript-eslint/no-non-null-assertion -- the equal region spans len tokens on both sides, so both reads are in bounds
			oldTokens[oldStart + i]!.marks,
			// eslint-disable-next-line @typescript-eslint/no-non-null-assertion -- the equal region spans len tokens on both sides, so both reads are in bounds
			newTokens[newStart + i]!.marks,
		),
	)

	// expand other changes to word boundaries using the same
	// tokenizer as diffWordsWithSpace so that mark-only changes
	// produce word-level diffs consistent with text changes.
	const wordTokens = wordsWithSpaceDiff.tokenize(text)

	let offset = 0
	for (const wordToken of wordTokens) {
		// eslint-disable-next-line @typescript-eslint/no-misused-spread -- the changes are indexed by code point, matching DiffToken granularity
		const tokenEnd = offset + [...wordToken].length

		if (changes.slice(offset, tokenEnd).includes(MarkChange.Other)) {
			changes.fill(MarkChange.Other, offset, tokenEnd)
		}

		offset = tokenEnd
	}

	// emit one op per run of the same change. A relink run also ends where
	// the old target changes, so each part keeps its own.
	let runStart = 0

	for (const [i, change] of changes.entries()) {
		const runEnds =
			changes[i + 1] !== change ||
			(change === MarkChange.Relinked &&
				linkHref(oldTokens[oldStart + i]) !==
					linkHref(oldTokens[oldStart + i + 1]))
		if (runEnds) {
			flushRun(
				ops,
				oldTokens.slice(oldStart + runStart, oldStart + i + 1),
				newTokens.slice(newStart + runStart, newStart + i + 1),
				change,
			)
			runStart = i + 1
		}
	}
}

// relinked means linked on both sides with all other marks equal
function markChange(
	oldMarks: DiffToken["marks"],
	newMarks: DiffToken["marks"],
): MarkChange {
	if (marksMatch(oldMarks, newMarks)) {
		return MarkChange.None
	}

	const oldRest = oldMarks.filter((mark) => mark.type !== Link.name)
	const newRest = newMarks.filter((mark) => mark.type !== Link.name)
	if (
		oldRest.length < oldMarks.length &&
		newRest.length < newMarks.length &&
		marksMatch(oldRest, newRest)
	) {
		return MarkChange.Relinked
	}

	return MarkChange.Other
}

function linkHref(token: DiffToken | undefined): unknown {
	return token?.marks.find((mark) => mark.type === Link.name)?.attrs?.href
}

function flushRun(
	ops: DiffOp[],
	oldRun: DiffToken[],
	newRun: DiffToken[],
	change: MarkChange,
): void {
	switch (change) {
		case MarkChange.None:
			ops.push({ type: "equal", tokens: newRun })
			break
		case MarkChange.Relinked:
			ops.push({
				type: "relink",
				tokens: newRun,
				oldHref: linkHref(oldRun[0]) as string,
			})
			break
		case MarkChange.Other:
			ops.push({ type: "delete", tokens: oldRun })
			ops.push({ type: "insert", tokens: newRun })
			break
	}
}
