import type { JSONContent } from "@tiptap/core"
import { hashBlockContent, type HashOptions } from "./content-hash"
import { lcs } from "./lcs"
import { buildPositionMap, type PositionMap, DiffStatus } from "./position-map"
import { DEFAULT_MERGE_OPTIONS } from "./config"
import { expandInlineDiffs } from "./inline-diff-expansion"

export interface MergeOptions extends HashOptions {
	/** use block uid attribute for primary matching (default: true) */
	useUidMatching?: boolean
	/** the attribute name for unique block IDs (default: 'uid') */
	uidAttribute?: string
	/**
	 * node types whose children should be individually diffed when the
	 * parent is modified. the parent keeps its own diff status for
	 * added/removed/unchanged.
	 */
	unwrapTypes?: string[]
	/**
	 * node types that are pure wrappers — they never carry diff status
	 * themselves. always unwrapped regardless of status, with the status
	 * propagated to (or computed for) children.
	 */
	transparentTypes?: string[]
}

export interface MergedResult {
	doc: JSONContent
	positionMap: PositionMap
}

interface BlockInfo {
	node: JSONContent
	hash: string
	uid: string | null
	sourceIndex: number
}

/**
 * deep-clone a JSONContent node and inject diff attributes without
 * mutating the original.
 */
function injectDiffAttrs(
	node: JSONContent,
	attrs: {
		diffStatus: DiffStatus
		modifiedIndex?: number | null
		originalIndex?: number | null
		oldNode?: JSONContent | null
	},
): JSONContent {
	const cloned: JSONContent = clone(node)
	cloned.attrs = {
		...cloned.attrs,
		diffStatus: attrs.diffStatus,
		modifiedIndex: attrs.modifiedIndex ?? null,
		originalIndex: attrs.originalIndex ?? null,
		oldNode: attrs.oldNode ?? null,
	}
	return cloned
}

function extractBlocks(doc: JSONContent, options: MergeOptions): BlockInfo[] {
	const blocks = doc.content ?? []
	const uidAttr = options.uidAttribute ?? "uid"

	return blocks.map((node, i) => ({
		node,
		hash: hashBlockContent(node, options),
		uid: (node.attrs?.[uidAttr] as string | undefined) ?? null,
		sourceIndex: i,
	}))
}

/**
 * core merge logic: match blocks from original and modified, then build
 * a merged array with diff attributes. extracted so it can be reused
 * recursively for child-level diffing of unwrapped types.
 */
function mergeBlocks(
	original: JSONContent,
	modified: JSONContent,
	options: MergeOptions,
): JSONContent[] {
	const originalBlocks = extractBlocks(original, options)
	const modifiedBlocks = extractBlocks(modified, options)

	// track which original blocks have been matched
	const matchedOriginalIndices = new Set<number>()
	// maps modified index → matched original BlockInfo
	const modifiedToOriginal = new Map<number, BlockInfo>()

	// step 1: uid-based matching
	if (options.useUidMatching) {
		const originalByUid = new Map<string, BlockInfo>()
		for (const block of originalBlocks) {
			if (block.uid) {
				originalByUid.set(block.uid, block)
			}
		}

		for (const [mi, modifiedBlock] of modifiedBlocks.entries()) {
			if (!modifiedBlock.uid) {
				continue
			}

			const originalBlock = originalByUid.get(modifiedBlock.uid)
			if (
				originalBlock &&
				!matchedOriginalIndices.has(originalBlock.sourceIndex)
			) {
				modifiedToOriginal.set(mi, originalBlock)
				matchedOriginalIndices.add(originalBlock.sourceIndex)
			}
		}
	}

	// step 2: LCS fallback on unmatched blocks
	const unmatchedOriginalBlocks = originalBlocks.filter(
		(b) => !matchedOriginalIndices.has(b.sourceIndex),
	)
	const unmatchedModifiedIndices: number[] = []
	for (let mi = 0; mi < modifiedBlocks.length; mi++) {
		if (!modifiedToOriginal.has(mi)) {
			unmatchedModifiedIndices.push(mi)
		}
	}

	if (
		unmatchedOriginalBlocks.length > 0 &&
		unmatchedModifiedIndices.length > 0
	) {
		const unmatchedModifiedBlocks = unmatchedModifiedIndices.flatMap((mi) => {
			const block = modifiedBlocks[mi]
			return block ? [block] : []
		})

		const lcsMatches = lcs(
			unmatchedOriginalBlocks,
			unmatchedModifiedBlocks,
			(a, b) => a.hash === b.hash,
		)

		for (const [oi, mi] of lcsMatches) {
			const originalBlock = unmatchedOriginalBlocks[oi]
			const modifiedIdx = unmatchedModifiedIndices[mi]

			if (!originalBlock || modifiedIdx === undefined) {
				continue
			}

			modifiedToOriginal.set(modifiedIdx, originalBlock)
			matchedOriginalIndices.add(originalBlock.sourceIndex)
		}
	}

	// step 3: unmatch order-breaking pairs so reordered nodes within
	// the same parent show as removed + added instead of unchanged.
	// find the longest increasing subsequence (LIS) of original indices
	// in modified order — pairs not in the LIS have moved position.
	if (modifiedToOriginal.size > 0) {
		const matchedPairs: [number, number][] = []
		for (let mi = 0; mi < modifiedBlocks.length; mi++) {
			const orig = modifiedToOriginal.get(mi)
			if (orig) {
				matchedPairs.push([mi, orig.sourceIndex])
			}
		}

		// LIS of original indices = LCS(origIndices, sorted(origIndices))
		const origIndices = matchedPairs.map(([, oi]) => oi)
		const sorted = [...origIndices].sort((a, b) => a - b)
		const lisMatches = lcs(origIndices, sorted, (a, b) => a === b)
		const lisSet = new Set(lisMatches.map(([i]) => i))

		for (const [i, [mi, origSourceIdx]] of matchedPairs.entries()) {
			if (!lisSet.has(i)) {
				modifiedToOriginal.delete(mi)
				matchedOriginalIndices.delete(origSourceIdx)
			}
		}
	}

	// step 4: build merged blocks
	const removedOriginalBlocks = originalBlocks.filter(
		(b) => !matchedOriginalIndices.has(b.sourceIndex),
	)

	const mergedContent: JSONContent[] = []
	let removedIdx = 0

	for (const [mi, modifiedBlock] of modifiedBlocks.entries()) {
		const matchedOriginal = modifiedToOriginal.get(mi)

		// insert any removed blocks that should appear before this modified block
		if (matchedOriginal) {
			while (removedIdx < removedOriginalBlocks.length) {
				const removed = removedOriginalBlocks[removedIdx]

				if (!removed) {
					break
				}

				if (removed.sourceIndex < matchedOriginal.sourceIndex) {
					mergedContent.push(
						injectDiffAttrs(removed.node, {
							diffStatus: DiffStatus.Removed,
							originalIndex: removed.sourceIndex,
						}),
					)
					removedIdx++
				} else {
					break
				}
			}
		}

		// emit the modified block
		if (matchedOriginal) {
			if (modifiedBlock.hash === matchedOriginal.hash) {
				mergedContent.push(
					injectDiffAttrs(modifiedBlock.node, {
						diffStatus: DiffStatus.Unchanged,
						modifiedIndex: mi,
						originalIndex: matchedOriginal.sourceIndex,
					}),
				)
			} else {
				mergedContent.push(
					injectDiffAttrs(modifiedBlock.node, {
						diffStatus: DiffStatus.Modified,
						modifiedIndex: mi,
						originalIndex: matchedOriginal.sourceIndex,
						oldNode: matchedOriginal.node,
					}),
				)
			}
		} else {
			mergedContent.push(
				injectDiffAttrs(modifiedBlock.node, {
					diffStatus: DiffStatus.Added,
					modifiedIndex: mi,
				}),
			)
		}
	}

	// append any remaining removed blocks at the end
	while (removedIdx < removedOriginalBlocks.length) {
		const removed = removedOriginalBlocks[removedIdx]

		if (!removed) {
			break
		}

		mergedContent.push(
			injectDiffAttrs(removed.node, {
				diffStatus: DiffStatus.Removed,
				originalIndex: removed.sourceIndex,
			}),
		)
		removedIdx++
	}

	// step 5a: always unwrap transparent types (e.g. metricGrid) —
	// they never carry diff status, only their children do.
	if (options.transparentTypes?.length) {
		for (const [i, block] of mergedContent.entries()) {
			if (!block.type || !options.transparentTypes.includes(block.type)) {
				continue
			}

			mergedContent[i] = unwrapBlock(block, options)
		}
	}

	// step 5b: unwrap compound types only when modified — the parent
	// keeps its own status for added/removed/unchanged.
	if (options.unwrapTypes?.length) {
		for (const [i, block] of mergedContent.entries()) {
			if (
				block.attrs?.diffStatus !== DiffStatus.Modified ||
				!block.type ||
				!options.unwrapTypes.includes(block.type)
			) {
				continue
			}

			mergedContent[i] = unwrapBlock(block, options, true)
		}
	}

	return mergedContent
}

/**
 * for an unwrap-type block, replace its children with individually
 * diffed versions. for added/removed/unchanged blocks this propagates
 * the status to each child. for modified blocks it runs the full
 * matching algorithm on children recursively.
 *
 * when keepOldNodeAndStatus is true the parent retains its oldNode and diffStatus attributes so
 * that node-view components can compare their own attrs (e.g. icon
 * changes on callout blocks) against the original version.
 */
function unwrapBlock(
	block: JSONContent,
	options: MergeOptions,
	keepOldNodeAndStatus = false,
): JSONContent {
	const status = block.attrs?.diffStatus as DiffStatus
	const cloned: JSONContent = clone(block)

	if (status === DiffStatus.Modified && block.attrs?.oldNode) {
		const originalNode: JSONContent = block.attrs.oldNode as JSONContent
		const originalDoc = { type: "doc", content: originalNode.content ?? [] }
		const modifiedDoc = { type: "doc", content: cloned.content ?? [] }
		cloned.content = mergeBlocks(originalDoc, modifiedDoc, options)
	} else if (cloned.content) {
		cloned.content = cloned.content.map((child, i) =>
			injectDiffAttrs(child, {
				diffStatus: status,
				originalIndex:
					status === DiffStatus.Removed || status === DiffStatus.Unchanged
						? i
						: null,
				modifiedIndex:
					status === DiffStatus.Added || status === DiffStatus.Unchanged
						? i
						: null,
			}),
		)
	}

	// clear parent-level diff attrs — children carry their own
	cloned.attrs = { ...cloned.attrs }
	delete cloned.attrs.originalIndex
	delete cloned.attrs.modifiedIndex

	if (!keepOldNodeAndStatus) {
		delete cloned.attrs.oldNode
		delete cloned.attrs.diffStatus
	}

	return cloned
}

/**
 * compute a merged document from original and modified JSON documents.
 * the result contains all blocks from both versions in document order,
 * each annotated with diff metadata.
 *
 * matching strategy:
 * 1. primary: match blocks by uid attribute (O(n) hash map)
 * 2. fallback: LCS on content hashes for remaining unmatched blocks
 *
 * document order follows the modified version, with removed blocks
 * inserted at their relative position from original.
 */
export function computeMergedDocument(
	original: JSONContent,
	modified: JSONContent,
	options: MergeOptions = DEFAULT_MERGE_OPTIONS,
): MergedResult {
	const opts = { ...DEFAULT_MERGE_OPTIONS, ...options }
	const mergedContent = mergeBlocks(original, modified, opts)

	const doc: JSONContent = {
		type: "doc",
		content: mergedContent,
	}

	// expand modified textblocks so deleted text becomes real content
	// with marks instead of zero-width widget decorations
	const expandedDoc = expandInlineDiffs(doc)

	return {
		doc: expandedDoc,
		positionMap: buildPositionMap(expandedDoc),
	}
}
