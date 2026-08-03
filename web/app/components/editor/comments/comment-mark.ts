import { type Editor, Mark, getMarkRange } from "@tiptap/core"
import {
	type EditorState,
	Plugin,
	PluginKey,
	TextSelection,
} from "@tiptap/pm/state"
import { fixUserSelectionAroundKeywordColor } from "../blocks/code-block/keyword"
import { pendingCommentBelongsToActiveUser } from "./utils"
import { COMMENT_MARK_NAME, DIFF_TEXT_ADDED_MARK_NAME } from "../mark-names"
import { DIFF_COMMENT_TX_META } from "../diff/diff-content-lock"
import { DiffStatus, type PositionMap } from "../diff/position-map"
import type { DocumentComment } from "~/utils/api/comment"
import type { Node as PMNode } from "@tiptap/pm/model"

export interface CommentAttrs {
	commentId: string
}

export interface TextCommentIndicator {
	commentId: string
	top: number
	left: number
	forcedHighlight: boolean
}

export interface TextCommentIndicatorState {
	indicators: TextCommentIndicator[]
	hoveredCommentId: string | null
	container: HTMLElement | null
}

export interface CommentMarkOptions {
	onIndicatorStateChange?: (state: TextCommentIndicatorState) => void
}

declare module "@tiptap/core" {
	interface Commands<ReturnType> {
		comment: {
			addCommentMark: (attrs: CommentAttrs) => ReturnType
			updateCommentMarkId: (oldId: string, newId: string) => ReturnType
			removeCommentMark: (id?: string) => ReturnType
			setCommentMarkForcedHighlight: (
				commentId: string,
				active: boolean,
			) => ReturnType
			refreshTextCommentIndicators: () => ReturnType
		}
	}
}

export const CommentMark = Mark.create<CommentMarkOptions>({
	name: COMMENT_MARK_NAME,
	inclusive: false,
	priority: 1000,
	excludes: "",
	addOptions() {
		return {}
	},
	addStorage() {
		return {
			forcedHighlights: new Set<string>(),
			updateForcedHighlightStyle: null as (() => void) | null,
			refreshIndicators: null as (() => void) | null,
		}
	},
	addAttributes() {
		return {
			commentId: {
				default: null,
				parseHTML: (element) => element.getAttribute("data-comment-id"),
				renderHTML: (attrs) => {
					return {
						"data-comment-id": attrs.commentId,
					}
				},
			},
		}
	},
	parseHTML() {
		return [{ tag: "span[data-comment-id]" }]
	},
	renderHTML({ HTMLAttributes }) {
		return [
			"span",
			{
				...HTMLAttributes,
				class:
					"comment-mark bg-comment-highlight/12 border-b-2 border-comment-highlight/35 transition-all duration-200",
			},
			0,
		]
	},
	addProseMirrorPlugins() {
		const storage = this.storage
		const onIndicatorStateChange = this.options.onIndicatorStateChange

		let indicatorState: TextCommentIndicatorState = {
			indicators: [],
			hoveredCommentId: null,
			container: null,
		}

		function notifyStateChange() {
			onIndicatorStateChange?.({ ...indicatorState })
		}

		return [
			new Plugin({
				key: new PluginKey("commentHover"),
				view(editorView) {
					let currentHoveredId: string | null = null
					const styleEl = document.createElement("style")
					document.head.appendChild(styleEl)

					// Forced highlight style element
					const forcedStyleEl = document.createElement("style")
					document.head.appendChild(forcedStyleEl)

					// Container for text comment indicators
					const container = document.createElement("div")
					container.className =
						"text-comment-indicator-container absolute inset-0 pointer-events-none"
					editorView.dom.parentElement?.insertBefore(container, editorView.dom)
					indicatorState.container = container

					function updateForcedHighlightStyle() {
						const ids = Array.from(storage.forcedHighlights)
						if (ids.length === 0) {
							forcedStyleEl.textContent = ""
							return
						}
						const selectors = ids
							.map((id) => `[data-comment-id="${id}"]`)
							.join(", ")
						forcedStyleEl.textContent = `${selectors} {
							background-color: color-mix(in oklab, var(--comment-highlight) 26%, transparent) !important;
							border-color: color-mix(in oklab, var(--comment-highlight) 90%, transparent) !important;
						}`
					}

					storage.updateForcedHighlightStyle = updateForcedHighlightStyle

					function clearHover() {
						if (currentHoveredId) {
							styleEl.textContent = ""
							currentHoveredId = null
							indicatorState.hoveredCommentId = null
							notifyStateChange()
						}
					}

					function setHover(commentId: string) {
						if (commentId === currentHoveredId) {
							return
						}

						currentHoveredId = commentId
						indicatorState.hoveredCommentId = commentId
						notifyStateChange()
						styleEl.textContent = `[data-comment-id="${commentId}"] {
							background-color: color-mix(in oklab, var(--comment-highlight) 26%, transparent) !important;
							border-color: color-mix(in oklab, var(--comment-highlight) 90%, transparent) !important;
						}`
					}

					function handleMouseEnter(event: MouseEvent) {
						const target = event.target as HTMLElement
						if (!target.hasAttribute?.("data-comment-id")) {
							return
						}

						const commentId = target.getAttribute("data-comment-id")
						if (commentId) {
							setHover(commentId)
						}
					}

					function handleMouseLeave(event: MouseEvent) {
						const target = event.target as HTMLElement
						if (!target.hasAttribute?.("data-comment-id")) {
							return
						}

						const relatedTarget = event.relatedTarget as HTMLElement | null
						const newCommentEl = relatedTarget?.closest?.("[data-comment-id]")
						const newCommentId = newCommentEl?.getAttribute("data-comment-id")

						if (newCommentId !== currentHoveredId) {
							clearHover()
						}
					}

					// Compute indicator positions for the first span of each
					// unique text comment.
					function updateIndicators() {
						const editorRect = editorView.dom.getBoundingClientRect()
						const seenIds = new Set<string>()
						const indicators: TextCommentIndicator[] = []

						editorView.dom
							.querySelectorAll("[data-comment-id]")
							.forEach((el) => {
								const commentId = el.getAttribute("data-comment-id")
								if (!commentId || seenIds.has(commentId)) {
									return
								}

								seenIds.add(commentId)

								// Use getClientRects() to get the rect of only the first
								// line. getBoundingClientRect() on a multi-line inline span
								// returns the bounding box of all lines, causing the
								// indicator to appear at the leftmost edge of any line
								// rather than where the comment text actually starts.
								const rects = el.getClientRects()

								// skip elements hidden by v-show (e.g. inside a
								// collapsed mermaid block).
								if (rects.length === 0) {
									return
								}

								const rect = rects[0]!
								indicators.push({
									commentId,
									top: rect.top - editorRect.top + editorView.dom.scrollTop,
									left: rect.left - editorRect.left,
									forcedHighlight: storage.forcedHighlights.has(commentId),
								})
							})

						indicatorState.indicators = indicators
						notifyStateChange()
					}

					storage.refreshIndicators = updateIndicators

					editorView.dom.addEventListener("mouseenter", handleMouseEnter, true)
					editorView.dom.addEventListener("mouseleave", handleMouseLeave, true)

					updateIndicators()

					window.addEventListener("resize", updateIndicators)
					window.addEventListener("scroll", updateIndicators)
					const resizeObserver = new ResizeObserver(updateIndicators)
					resizeObserver.observe(editorView.dom)

					return {
						update(view, prevState) {
							if (!view.state.doc.eq(prevState.doc)) {
								updateIndicators()
							}
						},
						destroy() {
							editorView.dom.removeEventListener(
								"mouseenter",
								handleMouseEnter,
								true,
							)
							editorView.dom.removeEventListener(
								"mouseleave",
								handleMouseLeave,
								true,
							)
							window.removeEventListener("resize", updateIndicators)
							window.removeEventListener("scroll", updateIndicators)
							resizeObserver.disconnect()
							styleEl.remove()
							forcedStyleEl.remove()
							container.remove()
							storage.updateForcedHighlightStyle = null
							storage.refreshIndicators = null
							indicatorState = {
								indicators: [],
								hoveredCommentId: null,
								container: null,
							}
							notifyStateChange()
							clearHover()
						},
					}
				},
			}),
		]
	},
	addCommands() {
		return {
			addCommentMark:
				(attrs: CommentAttrs) =>
				({ chain, state }) => {
					const markType = state.schema.marks[this.name]
					if (!markType) {
						return false
					}

					const { from, to } = state.selection
					if (from === to) {
						return false
					}

					const res = chain().setMark(this.name, attrs).run()

					// Note: this is a workaround for a selection bug where
					// after adding a comment mark inside a
					// decorated (keyword syntax) node, the selection expands.
					fixUserSelectionAroundKeywordColor()

					return res
				},
			updateCommentMarkId:
				(oldId: string, newId: string) =>
				({ state, tr, dispatch }) => {
					const markType = state.schema.marks[this.name]
					if (!markType) {
						return false
					}

					let found = false

					state.doc.descendants((node, pos) => {
						if (!node.isText) {
							return
						}

						// check if this node has the mark we're looking for
						const targetMark = node.marks.find(
							(mark) =>
								mark.type === markType && mark.attrs.commentId === oldId,
						)

						if (targetMark) {
							const $from = state.doc.resolve(pos)
							const range = getMarkRange($from, markType, targetMark.attrs)

							if (range) {
								const newAttrs = {
									...targetMark.attrs,
									commentId: newId,
								}
								const newMark = markType.create(newAttrs)

								// remove the old mark and add the new one at
								// the exact range
								tr.removeMark(range.from, range.to, targetMark)
								tr.addMark(range.from, range.to, newMark)

								found = true

								return false
							}
						}
					})

					if (!found) {
						return false
					}

					if (dispatch) {
						dispatch(tr)
					}

					return true
				},
			removeCommentMark:
				(id?: string) =>
				({ state, tr, dispatch }) => {
					const markType = state.schema.marks[this.name]
					if (!markType) {
						return false
					}

					if (!id) {
						const { from, to } = state.selection
						if (from === to) {
							return false
						}

						tr.removeMark(from, to, markType)

						if (dispatch) {
							dispatch(tr)
						}

						return true
					}

					state.doc.descendants((node, pos) => {
						if (!node.isText) {
							return
						}

						for (const m of node.marks) {
							if (m.type === markType && m.attrs.commentId === id) {
								tr.removeMark(pos, pos + node.nodeSize, markType)
								break
							}
						}
					})

					if (!tr.docChanged) {
						return false
					}

					if (dispatch) {
						dispatch(tr)
					}

					return true
				},

			setCommentMarkForcedHighlight:
				(commentId: string, active: boolean) => () => {
					const storage = this.storage

					if (active) {
						storage.forcedHighlights.add(commentId)
					} else {
						storage.forcedHighlights.delete(commentId)
					}

					storage.updateForcedHighlightStyle?.()
					storage.refreshIndicators?.()

					return true
				},

			refreshTextCommentIndicators: () => () => {
				this.storage.refreshIndicators?.()
				return true
			},
		}
	},
})

export function findCommentMarkAtPos(state: any, pos: number) {
	const markType = state.schema.marks[COMMENT_MARK_NAME]
	if (!markType) {
		return null
	}

	if (pos < 0 || pos > state.doc.content.size) {
		return null
	}

	const $pos = state.doc.resolve(pos)
	const direct = $pos.marks().find((m: any) => m.type === markType)
	if (direct) {
		const range = getMarkRange($pos, markType, direct.attrs)
		return range ? { ...range, attrs: direct.attrs } : null
	}

	const range = getMarkRange($pos, markType)
	if (range) {
		const node = state.doc.nodeAt(range.from)
		const mark = node?.marks?.find((m: any) => m.type === markType)
		return mark ? { ...range, attrs: mark.attrs } : null
	}

	return null
}

export function findCommentMarkById(
	state: EditorState,
	commentId: string,
): { from: number; to: number; attrs: CommentAttrs } | null {
	const markType = state.schema.marks[COMMENT_MARK_NAME]
	if (!markType) {
		return null
	}

	let result: { from: number; to: number; attrs: CommentAttrs } | null = null

	state.doc.descendants((node, pos) => {
		if (result) {
			return false
		}

		if (!node.isText) {
			return
		}

		const mark = node.marks.find(
			(m) => m.type === markType && m.attrs.commentId === commentId,
		)

		if (mark) {
			const $pos = state.doc.resolve(pos)
			const range = getMarkRange($pos, markType, mark.attrs)
			if (range) {
				result = { ...range, attrs: mark.attrs as CommentAttrs }
			}
		}
	})

	return result
}

export function findSelectedCommentMark(state: EditorState) {
	const { from, empty } = state.selection
	if (!empty) {
		return null
	}

	const direct = findCommentMarkAtPos(state, from)
	if (direct) {
		return direct
	}

	return null
}

export function deletePendingCommentMarks(
	editor: Editor,
	activeUserId: string,
	onDelete?: (id: string) => void,
	isDiffEditor?: boolean,
	excludeId?: string | null,
) {
	const { state, view } = editor
	const { tr } = state

	const markType = state.schema.marks[COMMENT_MARK_NAME]
	if (!markType) {
		return false
	}

	state.doc.descendants((node, pos) => {
		if (!node.marks?.length) {
			return
		}

		node.marks.forEach((mark) => {
			if (mark.type.name !== COMMENT_MARK_NAME) {
				return
			}

			const id = mark.attrs.commentId

			if (
				pendingCommentBelongsToActiveUser(id, activeUserId) &&
				id !== excludeId
			) {
				tr.removeMark(pos, pos + node.nodeSize, mark.type)
				onDelete?.(id)
			}
		})
	})

	if (tr.steps.length > 0) {
		if (isDiffEditor) {
			tr.setMeta(DIFF_COMMENT_TX_META, true)
		}

		view.dispatch(
			tr.setSelection(TextSelection.near(tr.doc.resolve(tr.selection.from))),
		)
	}
}

// convert a merged-doc offset (relative to a node's start) to an
// offset into the original (old/deleted) text only. inside an
// expanded modified node the content interleaves equal, added, and
// removed text. added text does not exist in the original, so we
// skip it when counting. this produces a stable offset that
// survives diff recomputes.
export function mergedOffsetToOriginalOffset(
	node: PMNode,
	mergedOffset: number,
): number {
	const addedType = node.type.schema.marks[DIFF_TEXT_ADDED_MARK_NAME]
	let mergedCursor = 0
	let originalCursor = 0

	// +1 because ProseMirror offsets inside a node start after the
	// open token
	const targetMerged = mergedOffset - 1

	for (let i = 0; i < node.childCount; i++) {
		const child = node.child(i)

		if (!child.isText) {
			mergedCursor += child.nodeSize
			originalCursor += child.nodeSize
			continue
		}

		const isAdded = !!addedType && child.marks.some((m) => m.type === addedType)
		const len = child.nodeSize

		if (mergedCursor + len > targetMerged) {
			// target falls inside this text node
			const intra = targetMerged - mergedCursor

			if (isAdded) {
				return originalCursor + 1
			}

			return originalCursor + intra + 1
		}

		mergedCursor += len

		if (!isAdded) {
			originalCursor += len
		}
	}

	return originalCursor + 1
}

// convert an original-text offset back to a merged-doc offset
// (relative to a node's start). the inverse of
// mergedOffsetToOriginalOffset. walks the expanded node's children,
// skipping added text for the original counter, to find where the
// original character lands in the merged layout.
//
// side controls boundary behaviour at delete/insert junctions:
//   "start" (default) — maps past any following added text
//     (use for fromOffset: "find the removed char at this original pos")
//   "end" — stops immediately after the last non-added char
//     (use for toOffset: "exclusive end right after the removed text")
export function originalOffsetToMergedOffset(
	node: PMNode,
	originalOffset: number,
	side: "start" | "end" = "start",
): number {
	const addedType = node.type.schema.marks[DIFF_TEXT_ADDED_MARK_NAME]
	let mergedCursor = 0
	let originalCursor = 0

	// -1 because the incoming offset includes the open token
	const targetOriginal = originalOffset - 1

	for (let i = 0; i < node.childCount; i++) {
		const child = node.child(i)

		if (!child.isText) {
			mergedCursor += child.nodeSize
			originalCursor += child.nodeSize
			continue
		}

		const isAdded = !!addedType && child.marks.some((m) => m.type === addedType)
		const len = child.nodeSize

		if (!isAdded) {
			if (originalCursor + len > targetOriginal) {
				const intra = targetOriginal - originalCursor
				return mergedCursor + intra + 1
			}

			originalCursor += len
		}

		mergedCursor += len

		// in "end" mode, when the original cursor has exactly reached
		// the target after a non-added child, return immediately
		// without advancing past any following added text.
		if (side === "end" && !isAdded && originalCursor === targetOriginal) {
			return mergedCursor + 1
		}
	}

	return mergedCursor + 1
}

// collect contiguous ranges within a node that carry a comment mark
// with the given commentId. returns absolute positions.
function findExistingCommentRanges(
	node: PMNode,
	nodeAbsStart: number,
	commentId: string,
): { from: number; to: number }[] {
	const commentMarkType = node.type.schema.marks[COMMENT_MARK_NAME]

	if (!commentMarkType) {
		return []
	}

	const ranges: { from: number; to: number }[] = []
	const contentStart = nodeAbsStart + 1
	let cursor = contentStart
	let rangeStart: number | null = null

	for (let i = 0; i < node.childCount; i++) {
		const child = node.child(i)
		const childStart = cursor
		const childEnd = cursor + child.nodeSize

		const hasMatch =
			child.isText &&
			child.marks.some(
				(m) => m.type === commentMarkType && m.attrs.commentId === commentId,
			)

		if (hasMatch) {
			if (rangeStart === null) {
				rangeStart = childStart
			}
		} else {
			if (rangeStart !== null) {
				ranges.push({ from: rangeStart, to: childStart })
				rangeStart = null
			}
		}

		cursor = childEnd
	}

	if (rangeStart !== null) {
		ranges.push({ from: rangeStart, to: cursor })
	}

	return ranges
}

// re-inject comment marks for deleted-content comments after a diff
// recompute. the recompute replaces the entire doc, wiping marks that
// only exist in the diff editor. this function reads saved comments
// with textAnchors in their deletionContext and re-applies the marks.
// textAnchor offsets are stored relative to the original (old) text,
// so they must be mapped to merged-doc positions via
// originalOffsetToMergedOffset for modified (inline-expanded) nodes.
//
// snap logic: when an anchor has snapFrom/snapTo, its boundary
// extends toward the nearest neighboring range (another anchor's
// base range OR an existing comment mark in the doc) for the same
// comment within the same node. this bridges gaps caused by text
// moving between existing and deleted states across diff recomputes.
export function reinjectDeletedContentMarks(
	diffEditor: Editor,
	_positionMap: PositionMap,
	comments: DocumentComment[],
) {
	const markType = diffEditor.state.schema.marks[COMMENT_MARK_NAME]
	if (!markType) {
		return
	}

	// build a uid → {startPos, nodeSize} lookup for removed and
	// modified nodes. walks the entire document tree so that nested
	// nodes inside unwrapped/transparent parents are included — the
	// top-level position map only tracks direct children of the doc.
	const uidToEntry = new Map<
		string,
		{ startPos: number; nodeSize: number; node: PMNode }
	>()

	diffEditor.state.doc.descendants((node, pos) => {
		const status = node.attrs?.diffStatus
		const uid = node.attrs?.uid

		if (
			uid &&
			(status === DiffStatus.Removed || status === DiffStatus.Modified)
		) {
			uidToEntry.set(uid, { startPos: pos, nodeSize: node.nodeSize, node })
		}
	})

	if (uidToEntry.size === 0) {
		return
	}

	const { tr } = diffEditor.state
	let changed = false

	for (const comment of comments) {
		const anchors = comment.diffDeletionContext?.textAnchors
		if (!anchors?.length) {
			continue
		}

		// first pass: compute base ranges for all anchors, grouped
		// by node uid. this lets snap targets include sibling
		// anchors whose marks haven't been applied to the doc yet.
		const nodeGroups = new Map<
			string,
			{
				from: number
				to: number
				snapFrom: boolean
				snapTo: boolean
			}[]
		>()

		for (const anchor of anchors) {
			const entry = uidToEntry.get(anchor.nodeUid)
			if (!entry) {
				continue
			}

			const isModified = entry.node.attrs?.diffStatus === DiffStatus.Modified
			const fromOffset = isModified
				? originalOffsetToMergedOffset(entry.node, anchor.fromOffset, "start")
				: anchor.fromOffset
			const toOffset = isModified
				? originalOffsetToMergedOffset(entry.node, anchor.toOffset, "end")
				: anchor.toOffset

			const from = entry.startPos + fromOffset
			const to = entry.startPos + toOffset
			const entryEnd = entry.startPos + entry.nodeSize

			if (from >= to || from < entry.startPos || to > entryEnd) {
				continue
			}

			let group = nodeGroups.get(anchor.nodeUid)

			if (!group) {
				group = []
				nodeGroups.set(anchor.nodeUid, group)
			}

			group.push({
				from,
				to,
				snapFrom: !!anchor.snapFrom,
				snapTo: !!anchor.snapTo,
			})
		}

		// second pass: for each node, build the full set of snap
		// reference ranges (sibling anchors + existing doc marks),
		// apply snap, and add marks.
		for (const [nodeUid, ranges] of nodeGroups) {
			const entry = uidToEntry.get(nodeUid)!
			const isModified = entry.node.attrs?.diffStatus === DiffStatus.Modified
			const needsSnap = isModified && ranges.some((r) => r.snapFrom || r.snapTo)

			// combine base ranges with existing comment marks in the
			// doc. sort by from so snap lookups can find the nearest
			// neighbor efficiently.
			let snapTargets: { from: number; to: number }[] = []

			if (needsSnap) {
				const existingRanges = findExistingCommentRanges(
					entry.node,
					entry.startPos,
					comment.id,
				)

				snapTargets = [
					...ranges.map((r) => ({ from: r.from, to: r.to })),
					...existingRanges,
				]
				snapTargets.sort((a, b) => a.from - b.from)
			}

			for (const range of ranges) {
				let { from, to } = range

				if (needsSnap && range.snapFrom) {
					// find the nearest range to the left: the largest
					// `to` among ranges that end at or before our from.
					for (const t of snapTargets) {
						if (t.to <= from) {
							from = t.to
						}
					}
				}

				if (needsSnap && range.snapTo) {
					// find the nearest range to the right: the smallest
					// `from` among ranges that start at or after our to.
					for (const t of snapTargets) {
						if (t.from >= to) {
							to = t.from
							break
						}
					}
				}

				if (from >= to) {
					continue
				}

				const mark = markType.create({ commentId: comment.id })
				tr.addMark(from, to, mark)
				changed = true
			}
		}
	}

	if (changed) {
		tr.setMeta(DIFF_COMMENT_TX_META, true)
		diffEditor.view.dispatch(tr)
	}
}
