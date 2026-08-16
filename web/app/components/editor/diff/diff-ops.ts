import { diffWordsWithSpace, wordsWithSpaceDiff } from "diff"
import type { DiffToken } from "./diff-tokens"

export interface DiffOp {
	type: "equal" | "insert" | "delete"
	tokens: DiffToken[]
}

// compute a word-level diff between old and new token arrays.
// uses jsdiff's diffWordsWithSpace for clean, contiguous change
// blocks that respect word boundaries, then refines "equal" regions
// to detect mark changes (e.g. bold/italic added or removed) —
// characters whose text matches but whose marks differ are emitted
// as delete+insert pairs so they appear as changed in the diff.
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

// walk an "equal" region and detect mark changes. any character
// whose marks differ is treated as changed, and the changed range
// is expanded to cover full words (split on whitespace) so that
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
	// build a boolean mask: true = marks differ at this position
	const changed = new Uint8Array(len)
	for (let i = 0; i < len; i++) {
		if (
			!marksMatch(
				// eslint-disable-next-line @typescript-eslint/no-non-null-assertion -- the equal region spans len tokens on both sides, so both reads are in bounds
				oldTokens[oldStart + i]!.marks,
				// eslint-disable-next-line @typescript-eslint/no-non-null-assertion -- the equal region spans len tokens on both sides, so both reads are in bounds
				newTokens[newStart + i]!.marks,
			)
		) {
			changed[i] = 1
		}
	}

	// expand changed regions to word boundaries using the same
	// tokenizer as diffWordsWithSpace so that mark-only changes
	// produce word-level diffs consistent with text changes.
	const wordTokens = wordsWithSpaceDiff.tokenize(text)

	let offset = 0
	for (const wordToken of wordTokens) {
		// eslint-disable-next-line @typescript-eslint/no-misused-spread -- the changed mask is indexed by code point, matching DiffToken granularity
		const tokenLen = [...wordToken].length
		const tokenEnd = offset + tokenLen

		// check if any character in this token has a mark change
		let hasChange = false
		for (let j = offset; j < tokenEnd; j++) {
			if (changed[j]) {
				hasChange = true
				break
			}
		}

		// if so, mark the entire token as changed
		if (hasChange) {
			for (let j = offset; j < tokenEnd; j++) {
				changed[j] = 1
			}
		}

		offset = tokenEnd
	}

	// emit runs based on the expanded mask
	let runStart = 0
	let runChanged = !!changed[0]

	for (let i = 1; i < len; i++) {
		const c = !!changed[i]
		if (c !== runChanged) {
			flushRun(
				ops,
				oldTokens,
				newTokens,
				oldStart,
				newStart,
				runStart,
				i,
				!runChanged,
			)
			runStart = i
			runChanged = c
		}
	}

	flushRun(
		ops,
		oldTokens,
		newTokens,
		oldStart,
		newStart,
		runStart,
		len,
		!runChanged,
	)
}

function flushRun(
	ops: DiffOp[],
	oldTokens: DiffToken[],
	newTokens: DiffToken[],
	oldStart: number,
	newStart: number,
	from: number,
	to: number,
	isEqual: boolean,
): void {
	if (from >= to) {
		return
	}

	if (isEqual) {
		ops.push({
			type: "equal",
			tokens: newTokens.slice(newStart + from, newStart + to),
		})
	} else {
		ops.push({
			type: "delete",
			tokens: oldTokens.slice(oldStart + from, oldStart + to),
		})
		ops.push({
			type: "insert",
			tokens: newTokens.slice(newStart + from, newStart + to),
		})
	}
}
