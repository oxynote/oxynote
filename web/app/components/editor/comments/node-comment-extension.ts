import { Extension, type Editor } from "@tiptap/core"
import type { Node } from "@tiptap/pm/model"
import { type EditorState, Plugin, PluginKey } from "@tiptap/pm/state"
import { cn } from "~/lib/utils"
import { isPendingCommentId, pendingCommentBelongsToActiveUser } from "./utils"
import { DIFF_COMMENT_TX_META } from "../diff/diff-content-lock"
import { DiffStatus, type PositionMap } from "../diff/position-map"
import type { DocumentComment } from "~/utils/api/comment"

export const NODE_COMMENT_ID_ATTR = "nodeCommentId"

export interface NodeCommentAttrs {
	nodeCommentId: string
}

interface NodeCommentOverlay {
	nodeCommentId: string
	top: number
	left: number
	width: number
	height: number
	forcedHighlight: boolean
}

export interface NodeCommentOverlayState {
	overlays: NodeCommentOverlay[]
	hoveredNodeCommentId: string | null
	container: HTMLElement | null
}

export interface NodeCommentOptions {
	types: string[]
	onOverlayStateChange?: (state: NodeCommentOverlayState) => void
	onNodeCommentClick?: (nodeCommentId: string) => void
}

export interface NodeCommentStorage {
	forcedHighlights: Set<string>
	updateOverlays: (() => void) | null
}

declare module "@tiptap/core" {
	interface Commands<ReturnType> {
		nodeComment: {
			addNodeComment: (pos: number, attrs: NodeCommentAttrs) => ReturnType
			updateNodeCommentId: (oldId: string, newId: string) => ReturnType
			removeNodeComment: (pos: number) => ReturnType
			hasNodeComment: (pos: number) => ReturnType
			setNodeCommentForcedHighlight: (
				nodeCommentId: string,
				active: boolean,
			) => ReturnType
			refreshNodeCommentOverlays: () => ReturnType
		}
	}
}

const COMMENT_HIGHLIGHT_VERTICAL_OFFSET = 5
const COMMENT_HIGHLIGHT_HORIZONTAL_OFFSET = 5

export const NodeComment = Extension.create<
	NodeCommentOptions,
	NodeCommentStorage
>({
	name: "nodeComment",
	addOptions() {
		return {
			types: [],
		}
	},
	addStorage() {
		return {
			forcedHighlights: new Set<string>(),
			updateOverlays: null as (() => void) | null,
		}
	},
	addGlobalAttributes() {
		return [
			{
				types: this.options.types,
				attributes: {
					nodeCommentId: {
						default: null,
						parseHTML: (element) =>
							element.getAttribute("data-node-comment-id"),
						renderHTML: (attrs) => {
							if (!attrs.nodeCommentId) {
								return {}
							}
							return {
								"data-node-comment-id": attrs.nodeCommentId as string,
							}
						},
					},
				},
			},
		]
	},
	addCommands() {
		return {
			addNodeComment:
				(pos: number, attrs: NodeCommentAttrs) =>
				({ tr, dispatch }) => {
					const node = tr.doc.nodeAt(pos)
					if (!node) {
						return false
					}

					if (!dispatch) {
						return false
					}

					tr.setNodeMarkup(pos, undefined, {
						...node.attrs,
						nodeCommentId: attrs.nodeCommentId,
					})

					return true
				},

			removeNodeComment:
				(pos: number) =>
				({ tr, dispatch }) => {
					const node = tr.doc.nodeAt(pos)
					if (!node) {
						return false
					}

					if (dispatch) {
						tr.setNodeMarkup(pos, undefined, {
							...node.attrs,
							nodeCommentId: null,
						})
					}

					return true
				},

			updateNodeCommentId:
				(oldId: string, newId: string) =>
				({ state, tr, dispatch }) => {
					let found = false

					state.doc.descendants((node, pos) => {
						if (found) {
							return false
						}

						if (node.attrs.nodeCommentId === oldId) {
							tr.setNodeMarkup(pos, undefined, {
								...node.attrs,
								nodeCommentId: newId,
							})
							found = true

							return false
						}
					})

					// eslint-disable-next-line @typescript-eslint/no-unnecessary-condition -- found is set inside the synchronous descendants callback, which TS cannot see
					if (!found) {
						return false
					}

					if (dispatch) {
						dispatch(tr)
					}

					return true
				},

			hasNodeComment:
				(pos: number) =>
				({ state }) => {
					const node = state.doc.nodeAt(pos)
					const id = node?.attrs.nodeCommentId as string | null | undefined
					return !!id && !isPendingCommentId(id)
				},

			setNodeCommentForcedHighlight:
				(nodeCommentId: string, active: boolean) => () => {
					const storage = this.storage

					if (active) {
						storage.forcedHighlights.add(nodeCommentId)
					} else {
						storage.forcedHighlights.delete(nodeCommentId)
					}

					storage.updateOverlays?.()

					return true
				},

			refreshNodeCommentOverlays: () => () => {
				this.storage.updateOverlays?.()
				return true
			},
		}
	},
	addProseMirrorPlugins() {
		const pluginKey = new PluginKey("nodeCommentDecorations")
		const onOverlayStateChange = this.options.onOverlayStateChange
		const onNodeCommentClick = this.options.onNodeCommentClick
		const storage = this.storage

		// Track current overlay state for the callback
		let overlayState: NodeCommentOverlayState = {
			overlays: [],
			hoveredNodeCommentId: null,
			container: null,
		}

		function notifyStateChange() {
			onOverlayStateChange?.({ ...overlayState })
		}

		return [
			new Plugin({
				key: pluginKey,
				view(editorView) {
					const container = document.createElement("div")
					container.className = cn(
						"node-comment-overlay-container absolute inset-0 pointer-events-none",
					)
					editorView.dom.parentElement?.insertBefore(container, editorView.dom)
					overlayState.container = container

					// Hover state management using a single style element for performance
					let currentHoveredId: string | null = null
					const hoverStyleEl = document.createElement("style")
					document.head.appendChild(hoverStyleEl)

					// Track attached listeners for cleanup
					interface NodeHandlers {
						enter: () => void
						leave: (e: Event) => void
						markEnter: (e: Event) => void
						markLeave: (e: Event) => void
						click: (e: Event) => void
					}
					const attachedNodes = new Map<Element, NodeHandlers>()
					let suppressHover = false
					let activeNodeCommentId: string | null = null

					function clearHover() {
						if (currentHoveredId) {
							hoverStyleEl.textContent = ""
							currentHoveredId = null
							overlayState.hoveredNodeCommentId = null
							notifyStateChange()
						}
					}

					function setHover(nodeCommentId: string) {
						if (nodeCommentId === currentHoveredId) {
							return
						}

						currentHoveredId = nodeCommentId
						overlayState.hoveredNodeCommentId = nodeCommentId
						notifyStateChange()
						hoverStyleEl.textContent = `.node-comment-overlay[data-node-comment-id="${nodeCommentId}"] {
							background-color: color-mix(in oklab, var(--comment-highlight) 26%, transparent) !important;
						}`
					}

					function detachAllListeners() {
						attachedNodes.forEach((h, node) => {
							node.removeEventListener("mouseenter", h.enter)
							node.removeEventListener("mouseleave", h.leave)
							node.removeEventListener("mouseenter", h.markEnter, true)
							node.removeEventListener("mouseleave", h.markLeave, true)
							node.removeEventListener("click", h.click, true)
						})
						attachedNodes.clear()
					}

					function attachListenersToCommentedNodes() {
						detachAllListeners()

						editorView.dom
							.querySelectorAll("[data-node-comment-id]")
							.forEach((node) => {
								const nodeCommentId = node.getAttribute("data-node-comment-id")
								if (!nodeCommentId) return

								const enter = () => {
									activeNodeCommentId = nodeCommentId
									if (!suppressHover) {
										setHover(nodeCommentId)
									}
								}
								const leave = (e: Event) => {
									const relatedTarget = (e as MouseEvent).relatedTarget
									// Check if moving to another commented node (including parent)
									const targetCommentNode =
										relatedTarget instanceof Element
											? relatedTarget.closest("[data-node-comment-id]")
											: null
									const targetCommentId = targetCommentNode?.getAttribute(
										"data-node-comment-id",
									)

									if (targetCommentId && targetCommentId !== nodeCommentId) {
										// Moving to a different commented node - activate its hover
										activeNodeCommentId = targetCommentId
										if (!suppressHover) {
											setHover(targetCommentId)
										}
									} else if (!targetCommentId) {
										// Leaving commented nodes entirely
										activeNodeCommentId = null
										suppressHover = false
										clearHover()
									}
								}
								const markEnter = (e: Event) => {
									if (
										(e.target as HTMLElement).classList.contains("comment-mark")
									) {
										suppressHover = true
										clearHover()
									}
								}
								const markLeave = (e: Event) => {
									if (
										(e.target as HTMLElement).classList.contains("comment-mark")
									) {
										suppressHover = false
										if (activeNodeCommentId) {
											setHover(activeNodeCommentId)
										}
									}
								}

								const click = (e: Event) => {
									// Don't fire if clicking on a comment mark inside this node
									const target = e.target
									if (
										target instanceof Element &&
										target.closest(".comment-mark")
									) {
										return
									}

									onNodeCommentClick?.(nodeCommentId)
								}

								node.addEventListener("mouseenter", enter)
								node.addEventListener("mouseleave", leave)
								node.addEventListener("mouseenter", markEnter, true)
								node.addEventListener("mouseleave", markLeave, true)
								node.addEventListener("click", click, true)
								attachedNodes.set(node, {
									enter,
									leave,
									markEnter,
									markLeave,
									click,
								})
							})
					}

					function updateOverlays() {
						// Only remove overlay elements, preserve Vue Teleport content
						container
							.querySelectorAll(".node-comment-overlay")
							.forEach((el) => {
								el.remove()
							})

						const state = editorView.state
						const newOverlays: NodeCommentOverlay[] = []

						// Iterate over document to find nodes with nodeCommentId
						// (instead of using decorations which conflict with Vue node views)
						state.doc.descendants((node, pos) => {
							const nodeCommentId = node.attrs.nodeCommentId as string | null
							if (!nodeCommentId) {
								return
							}

							const nodeDOM = editorView.nodeDOM(pos)
							if (!nodeDOM || !(nodeDOM instanceof HTMLElement)) {
								return
							}

							const nodeRect = nodeDOM.getBoundingClientRect()
							const editorRect = editorView.dom.getBoundingClientRect()

							const offsets = nodeOverlayOffset(nodeDOM)
							const top =
								nodeRect.top -
								editorRect.top +
								editorView.dom.scrollTop -
								COMMENT_HIGHLIGHT_VERTICAL_OFFSET -
								offsets.extraTop
							const left =
								nodeRect.left -
								editorRect.left -
								COMMENT_HIGHLIGHT_HORIZONTAL_OFFSET -
								offsets.extraLeft
							const width =
								nodeRect.width +
								COMMENT_HIGHLIGHT_HORIZONTAL_OFFSET * 2 +
								offsets.extraLeft +
								offsets.extraRight
							const height =
								nodeRect.height +
								COMMENT_HIGHLIGHT_VERTICAL_OFFSET * 2 +
								offsets.extraTop +
								offsets.extraBottom
							const forcedHighlight =
								storage.forcedHighlights.has(nodeCommentId)

							newOverlays.push({
								nodeCommentId: nodeCommentId,
								top,
								left,
								width,
								height,
								forcedHighlight,
							})

							const overlay = document.createElement("div")
							overlay.className = cn(
								"node-comment-overlay absolute bg-comment-highlight/12 pointer-events-none rounded-md z-editor-overlay transition-all data-forced-highlight:bg-comment-highlight/26",
							)
							overlay.setAttribute("data-node-comment-id", nodeCommentId)

							if (forcedHighlight) {
								overlay.setAttribute("data-forced-highlight", "")
							}

							overlay.style.top = `${top}px`
							overlay.style.left = `${left}px`
							overlay.style.width = `${width}px`
							overlay.style.height = `${height}px`

							container.appendChild(overlay)
						})

						overlayState.overlays = newOverlays
						notifyStateChange()

						// Re-attach listeners after DOM updates
						attachListenersToCommentedNodes()
					}

					updateOverlays()
					storage.updateOverlays = updateOverlays

					// Resize and scroll handlers
					window.addEventListener("resize", updateOverlays)
					window.addEventListener("scroll", updateOverlays)

					const resizeObserver = new ResizeObserver(updateOverlays)
					resizeObserver.observe(editorView.dom)

					return {
						update(view, prevState) {
							// Update overlays when document changes
							if (view.state.doc.eq(prevState.doc)) {
								return
							}

							updateOverlays()
						},
						destroy() {
							window.removeEventListener("resize", updateOverlays)
							window.removeEventListener("scroll", updateOverlays)
							resizeObserver.disconnect()
							detachAllListeners()
							hoverStyleEl.remove()
							container.remove()
							storage.updateOverlays = null
							overlayState = {
								overlays: [],
								hoveredNodeCommentId: null,
								container: null,
							}
							notifyStateChange()
						},
					}
				},
			}),
		]
	},
})

export function deletePendingNodeComments(
	editor: Editor,
	activeUserId: string,
	onDelete?: (id: string) => void,
	isDiffEditor?: boolean,
	excludeId?: string | null,
) {
	const { state, view } = editor
	const { tr } = state

	state.doc.descendants((node, pos) => {
		const id = node.attrs.nodeCommentId as string | null
		if (!id) {
			return
		}

		if (
			pendingCommentBelongsToActiveUser(id, activeUserId) &&
			id !== excludeId
		) {
			tr.setNodeMarkup(pos, undefined, {
				...node.attrs,
				nodeCommentId: null,
			})
			onDelete?.(id)
		}
	})

	if (tr.steps.length > 0) {
		if (isDiffEditor) {
			tr.setMeta(DIFF_COMMENT_TX_META, true)
		}

		view.dispatch(tr)
	}
}

export interface NodeCommentMatch {
	pos: number
	node: Node
	nodeCommentId: string
}

export function findNodeCommentAtPos(
	state: EditorState,
	pos: number,
): NodeCommentMatch | null {
	const node = state.doc.nodeAt(pos)
	if (!node?.attrs.nodeCommentId) {
		return null
	}

	return {
		pos,
		node,
		nodeCommentId: node.attrs.nodeCommentId as string,
	}
}

export function findNodeCommentById(
	state: EditorState,
	id: string,
): NodeCommentMatch | null {
	let result: NodeCommentMatch | null = null

	state.doc.descendants((node, pos) => {
		if (result) return false

		if (node.attrs.nodeCommentId === id) {
			result = { pos, node, nodeCommentId: id }
			return false
		}
	})

	return result
}

// re-inject nodeCommentId attributes for deleted-content node comments
// after a diff recompute. similar to reinjectDeletedContentMarks but
// operates on node attributes instead of inline marks.
export function reinjectDeletedContentNodeComments(
	diffEditor: Editor,
	_positionMap: PositionMap,
	comments: DocumentComment[],
) {
	// build a uid → {startPos, nodeSize} lookup for removed nodes
	// and transparent wrappers around removed content. walks the
	// entire document tree so that nested nodes inside unwrapped/
	// transparent parents are included — the top-level position
	// map only tracks direct children of the doc.
	const uidToEntry = new Map<string, { startPos: number; nodeSize: number }>()

	diffEditor.state.doc.descendants((node, pos) => {
		const uid = node.attrs.uid as string | undefined
		if (!uid) {
			return true
		}

		if (node.attrs.diffStatus === DiffStatus.Removed) {
			uidToEntry.set(uid, { startPos: pos, nodeSize: node.nodeSize })
			return true
		}

		// transparent wrapper nodes (e.g. titledCodeBlock) have their
		// diffStatus stripped during merge and propagated to children.
		// detect them by checking that the wrapper has no status itself
		// but all children are removed.
		if (!node.attrs.diffStatus && node.childCount > 0) {
			let allRemoved = true
			for (let i = 0; i < node.childCount; i++) {
				if (node.child(i).attrs.diffStatus !== DiffStatus.Removed) {
					allRemoved = false
					break
				}
			}

			if (allRemoved) {
				uidToEntry.set(uid, { startPos: pos, nodeSize: node.nodeSize })
			}
		}

		return true
	})

	if (uidToEntry.size === 0) {
		return
	}

	const { tr } = diffEditor.state
	let changed = false

	for (const comment of comments) {
		// node comments on deleted content have deletionContext but no textAnchors
		if (
			!comment.diffDeletionContext ||
			comment.diffDeletionContext.textAnchors?.length
		) {
			continue
		}

		const entry = uidToEntry.get(comment.anchorBlockId)
		if (!entry) {
			continue
		}

		const node = diffEditor.state.doc.nodeAt(entry.startPos)
		if (!node || node.attrs.nodeCommentId) {
			continue
		}

		tr.setNodeMarkup(entry.startPos, undefined, {
			...node.attrs,
			nodeCommentId: comment.id,
		})
		changed = true
	}

	if (changed) {
		tr.setMeta(DIFF_COMMENT_TX_META, true)
		diffEditor.view.dispatch(tr)
	}
}
