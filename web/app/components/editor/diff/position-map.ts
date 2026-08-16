import type { JSONContent } from "@tiptap/core"
import type { MarkType, Node as PMNode } from "@tiptap/pm/model"

export enum DiffStatus {
	Unchanged = "unchanged",
	Added = "added",
	Removed = "removed",
	Modified = "modified",
}

export interface MappedPosition {
	source: "original" | "modified"
	blockIndex: number
	offsetInBlock: number
}

export interface PositionMapEntry {
	source: "original" | "modified"
	blockIndex: number
	/** start position (in the merged ProseMirror doc, 0-based from doc start) */
	startPos: number
	/** node size including open/close tokens */
	nodeSize: number
	diffStatus: DiffStatus
	/** block uid from the source document (null if absent) */
	uid: string | null
}

export type PositionMap = PositionMapEntry[]

/**
 * build a position map from the merged document's top-level nodes.
 * each entry records the origin, block index in the original
 * document, and the position range in the merged editor.
 *
 * positions are estimated from the JSON structure: each top-level node
 * contributes 1 (open token) + content size + 1 (close token). the doc
 * node itself adds 1 opening token offset.
 *
 * note: for exact ProseMirror positions, rebuild this from the actual
 * editor doc after setContent. this JSON-based version is a close
 * approximation useful for mapping.
 */
export function buildPositionMap(mergedDoc: JSONContent): PositionMap {
	const entries: PositionMap = []
	const blocks = mergedDoc.content ?? []

	let pos = 0

	for (const block of blocks) {
		const attrs = block.attrs ?? {}
		const diffStatus = (attrs.diffStatus ?? DiffStatus.Unchanged) as DiffStatus
		const modifiedIndex = (attrs.modifiedIndex ?? null) as number | null
		const originalIndex = (attrs.originalIndex ?? null) as number | null

		let source: "original" | "modified"
		let blockIndex: number

		if (diffStatus === DiffStatus.Removed) {
			source = "original"
			blockIndex = originalIndex ?? 0
		} else {
			source = "modified"
			blockIndex = modifiedIndex ?? 0
		}

		// estimate node size: open + close tokens (2) + content
		const nodeSize = estimateNodeSize(block)

		entries.push({
			source,
			blockIndex,
			startPos: pos,
			nodeSize,
			diffStatus,
			uid: (attrs.uid ?? null) as string | null,
		})

		pos += nodeSize
	}

	return entries
}

/** rough size estimate for a JSON node (open + close + inline content) */
function estimateNodeSize(node: JSONContent): number {
	if (!node.content || node.content.length === 0) {
		// leaf or empty node: open + close
		return 2
	}

	let size = 2 // open + close tokens for this node
	for (const child of node.content) {
		if (child.type === "text") {
			size += (child.text ?? "").length
		} else {
			size += estimateNodeSize(child)
		}
	}
	return size
}

/**
 * build a position map from the actual ProseMirror document after
 * setContent. this uses real node sizes and positions instead of
 * JSON estimates, ensuring accurate position mapping between the
 * diff editor and the source documents.
 */
export function buildPositionMapFromDoc(doc: PMNode): PositionMap {
	const entries: PositionMap = []

	doc.forEach((node, offset) => {
		const pos = offset
		const attrs = node.attrs
		const diffStatus = (attrs.diffStatus ?? DiffStatus.Unchanged) as DiffStatus
		const modifiedIndex = (attrs.modifiedIndex ?? null) as number | null
		const originalIndex = (attrs.originalIndex ?? null) as number | null

		let source: "original" | "modified"
		let blockIndex: number

		if (diffStatus === DiffStatus.Removed) {
			source = "original"
			blockIndex = originalIndex ?? 0
		} else {
			source = "modified"
			blockIndex = modifiedIndex ?? 0
		}

		entries.push({
			source,
			blockIndex,
			startPos: pos,
			nodeSize: node.nodeSize,
			diffStatus,
			uid: (attrs.uid ?? null) as string | null,
		})
	})

	return entries
}

/**
 * resolve a position inside the merged editor to its source document
 * and block-local offset.
 */
export function resolvePosition(
	map: PositionMap,
	pos: number,
): MappedPosition | null {
	for (const entry of map) {
		const endPos = entry.startPos + entry.nodeSize
		if (pos >= entry.startPos && pos < endPos) {
			return {
				source: entry.source,
				blockIndex: entry.blockIndex,
				offsetInBlock: pos - entry.startPos,
			}
		}
	}
	return null
}

export interface SelectionSegment {
	status: DiffStatus
	nodeUid: string
	/** absolute start position in the merged doc */
	from: number
	/** absolute end position in the merged doc */
	to: number
	/** offset from the node's start position */
	fromOffset: number
	/** offset from the node's start position */
	toOffset: number
}

/**
 * split a selection range into segments grouped by diff status and
 * node. each segment covers the portion of the selection that falls
 * within a single position map entry.
 *
 * when doc and diffMarkTypes are provided, modified segments are
 * further sub-segmented by inline diff marks so that deleted,
 * added, and unchanged text each get their own segments.
 */
export function segmentSelectionByDiffStatus(
	map: PositionMap,
	from: number,
	to: number,
	doc?: PMNode,
	diffMarkTypes?: { removed?: MarkType; added?: MarkType },
): SelectionSegment[] {
	const segments: SelectionSegment[] = []

	for (const entry of map) {
		const entryEnd = entry.startPos + entry.nodeSize

		// skip entries entirely before or after the selection
		if (entryEnd <= from || entry.startPos >= to) {
			continue
		}

		const segFrom = Math.max(from, entry.startPos)
		const segTo = Math.min(to, entryEnd)

		if (segFrom >= segTo) {
			continue
		}

		if (entry.diffStatus === DiffStatus.Modified && doc && diffMarkTypes) {
			segments.push(
				...subSegmentByDiffMarks(doc, diffMarkTypes, entry, segFrom, segTo),
			)
			continue
		}

		segments.push({
			status: entry.diffStatus,
			nodeUid: entry.uid ?? "",
			from: segFrom,
			to: segTo,
			fromOffset: segFrom - entry.startPos,
			toOffset: segTo - entry.startPos,
		})
	}

	return segments
}

// split a modified segment into Removed, Added, and Unchanged
// sub-segments based on inline diff marks.
function subSegmentByDiffMarks(
	doc: PMNode,
	markTypes: { removed?: MarkType; added?: MarkType },
	entry: PositionMapEntry,
	selFrom: number,
	selTo: number,
): SelectionSegment[] {
	const result: SelectionSegment[] = []
	const uid = entry.uid ?? ""
	let cursor = selFrom

	doc.nodesBetween(selFrom, selTo, (node, pos) => {
		if (!node.isText) {
			return true
		}

		const nodeEnd = pos + node.nodeSize
		const overlapFrom = Math.max(selFrom, pos)
		const overlapTo = Math.min(selTo, nodeEnd)

		if (overlapFrom >= overlapTo) {
			return false
		}

		let status: DiffStatus

		if (node.marks.some((m) => m.type === markTypes.removed)) {
			status = DiffStatus.Removed
		} else if (node.marks.some((m) => m.type === markTypes.added)) {
			status = DiffStatus.Added
		} else {
			status = DiffStatus.Unchanged
		}

		// emit a segment for any gap before this text node
		if (cursor < overlapFrom) {
			result.push({
				status: DiffStatus.Modified,
				nodeUid: uid,
				from: cursor,
				to: overlapFrom,
				fromOffset: cursor - entry.startPos,
				toOffset: overlapFrom - entry.startPos,
			})
		}

		result.push({
			status,
			nodeUid: uid,
			from: overlapFrom,
			to: overlapTo,
			fromOffset: overlapFrom - entry.startPos,
			toOffset: overlapTo - entry.startPos,
		})

		cursor = overlapTo
		return false
	})

	// trailing segment after the last text node
	if (cursor < selTo) {
		result.push({
			status: DiffStatus.Modified,
			nodeUid: uid,
			from: cursor,
			to: selTo,
			fromOffset: cursor - entry.startPos,
			toOffset: selTo - entry.startPos,
		})
	}

	return result
}
