import {
	type ComputePositionConfig,
	type VirtualElement,
	computePosition,
} from "@floating-ui/dom"
import type { Editor } from "@tiptap/core"
import { isChangeOrigin } from "@tiptap/extension-collaboration"
import type { Node } from "@tiptap/pm/model"
import {
	type EditorState,
	type Transaction,
	type SelectionRange,
	Plugin,
	PluginKey,
	NodeSelection,
} from "@tiptap/pm/state"
import {
	absolutePositionToRelativePosition,
	relativePositionToAbsolutePosition,
	ySyncPluginKey,
	type ProsemirrorBinding,
} from "@tiptap/y-tiptap"
import type * as Y from "yjs"
import {
	getSelectionRanges,
	NodeRangeSelection,
} from "@tiptap/extension-node-range"
import { Fragment, Slice } from "@tiptap/pm/model"
import type { HocuspocusProvider } from "@hocuspocus/provider"
import { findDraggableNodeAtCoords } from "./node-detection.js"
import type { DragHandleDragging } from "./drag"
import { enableGapZones, disableGapZones } from "./gap-decorations"
import {
	handlePositionByNodeType,
	LIST_ITEM_TYPES,
	LIST_NODE_TYPES,
	NODE_TO_WRAPPER_TYPE,
} from "./config.js"
import {
	setDraggingNodeInAwareness,
	isNodeBeingDraggedByOther,
} from "../collaboration"

// ySyncPluginKey is declared as PluginKey<any> upstream; this is the slice of
// its state that the drag handle reads
interface YSyncPluginState {
	doc: Y.Doc
	type: Y.XmlFragment
	binding: ProsemirrorBinding
}

const findRelativePos = (
	state: EditorState,
	absolutePos: number,
): Y.RelativePosition | null => {
	const ystate = ySyncPluginKey.getState(state) as YSyncPluginState | undefined
	if (!ystate) {
		return null
	}

	return absolutePositionToRelativePosition(
		absolutePos,
		ystate.type,
		ystate.binding.mapping,
	) as Y.RelativePosition
}

const findAbsolutePos = (
	state: EditorState,
	relativePos: Y.RelativePosition | null,
) => {
	const ystate = ySyncPluginKey.getState(state) as YSyncPluginState | undefined
	if (!ystate) {
		return -1
	}

	// a resolved position of 0 is the document start, which is where a
	// handle on the first node legitimately lands; only a null means the
	// relative position no longer maps anywhere
	return (
		relativePositionToAbsolutePosition(
			ystate.doc,
			ystate.type,
			relativePos,
			ystate.binding.mapping,
		) ?? -1
	)
}

function getCSSText(element: Element) {
	let value = ""
	const style = getComputedStyle(element)

	for (const property of style) {
		value += `${property}:${style.getPropertyValue(property)};`
	}

	return value
}

function cloneElement(node: HTMLElement) {
	const clonedNode = node.cloneNode(true) as HTMLElement
	const sourceElements = [
		node,
		...Array.from(node.getElementsByTagName("*")),
	] as HTMLElement[]
	const targetElements = [
		clonedNode,
		...Array.from(clonedNode.getElementsByTagName("*")),
	] as HTMLElement[]

	sourceElements.forEach((sourceElement, index) => {
		const targetEl = targetElements[index]

		// If the source is a canvas, copy its bitmap into a fresh canvas
		if (sourceElement instanceof HTMLCanvasElement) {
			try {
				const newCanvas = document.createElement("canvas")

				// copy intrinsic size
				newCanvas.width = sourceElement.width
				newCanvas.height = sourceElement.height

				// copy drawn pixels
				const ctx = newCanvas.getContext("2d")
				if (ctx) {
					ctx.drawImage(sourceElement, 0, 0)
				}

				// copy computed styles
				newCanvas.style.cssText = getCSSText(sourceElement)

				// Replace the cloned canvas node with the new canvas that contains pixels
				if (targetEl?.parentNode) {
					targetEl.parentNode.replaceChild(newCanvas, targetEl)
					// update the corresponding entry in targetElements so later code
					// that references this array sees the replacement
					targetElements[index] = newCanvas
				}
			} catch {
				// If anything fails, fall back to copying styles onto the cloned element
				if (targetEl) {
					targetEl.style.cssText = getCSSText(sourceElement)
				}
			}
		} else {
			if (targetEl) {
				targetEl.style.cssText = getCSSText(sourceElement)
			}
		}
	})

	// Now that styles are copied, remove any decorator widgets from the clone.
	clonedNode
		.querySelectorAll<HTMLElement>(`[${CLONE_IGNORE_ATTR}="true"]`)
		.forEach((el) => {
			el.remove()
		})

	return clonedNode
}

function removeNode(node: HTMLElement) {
	node.parentNode?.removeChild(node)
}

/**
 * Find the parent list type for a list item at the given position.
 * Returns the list type name (e.g., "bulletList", "orderedList", "taskList") or null.
 */
function findParentListType(
	doc: Node,
	nodePos: number,
	node: Node,
): string | null {
	// Only look for parent list type if this is a list item
	if (!LIST_ITEM_TYPES.has(node.type.name)) {
		return null
	}

	try {
		// Resolve position inside the node (nodePos + 1) to ensure we get
		// the full ancestor chain including the list item's parent list
		const $pos = doc.resolve(nodePos + 1)

		// Walk up the tree to find the parent list
		for (let d = $pos.depth; d > 0; d--) {
			const ancestor = $pos.node(d)
			if (LIST_NODE_TYPES.has(ancestor.type.name)) {
				return ancestor.type.name
			}
		}
	} catch {
		// Ignore errors
	}

	return null
}

/**
 * Find the source wrapper bounds if dragging a node that requires a wrapper.
 * Returns { start, end } positions of the wrapper, or null if not applicable.
 */
function findSourceWrapperBounds(
	doc: Node,
	nodePos: number,
	node: Node,
): { start: number; end: number } | null {
	const wrapperTypeName = NODE_TO_WRAPPER_TYPE.get(node.type.name)
	if (!wrapperTypeName) {
		return null
	}

	try {
		const $pos = doc.resolve(nodePos + 1)

		for (let d = $pos.depth; d > 0; d--) {
			const ancestor = $pos.node(d)
			if (ancestor.type.name === wrapperTypeName) {
				return {
					start: $pos.before(d),
					end: $pos.after(d),
				}
			}
		}
	} catch {
		// Ignore errors
	}

	return null
}

// handleDragData renders a drag image and sets up the drag event for
// dragging nodes.
function handleDragData(event: DragEvent, editor: Editor, nodePos: number) {
	const { view } = editor

	if (!event.dataTransfer) {
		return
	}

	const { doc, tr } = view.state
	const { empty, $from, $to } = view.state.selection

	const node = doc.nodeAt(nodePos)
	if (!node) {
		return
	}

	const nodeEndPos = nodePos + node.nodeSize
	const selectionRanges = empty ? [] : getSelectionRanges($from, $to, 0)
	const shouldDragFullSelection =
		!empty &&
		selectionRanges.some((range) => {
			return range.$from.pos === nodePos && range.$to.pos === nodeEndPos
		})

	let slice: Slice
	let ranges: SelectionRange[]

	if (shouldDragFullSelection) {
		// The dragged node is one of the selected nodes - use full selection
		// shouldDragFullSelection can only be true when some() matched, so the
		// range list is non-empty here
		// eslint-disable-next-line @typescript-eslint/no-non-null-assertion -- see above
		const from = selectionRanges[0]!.$from.pos
		// eslint-disable-next-line @typescript-eslint/no-non-null-assertion -- see above
		const to = selectionRanges[selectionRanges.length - 1]!.$to.pos
		const selection = NodeRangeSelection.create(doc, from, to)
		slice = selection.content()
		ranges = selectionRanges
		tr.setSelection(selection)
	} else {
		// Single node drag - use NodeSelection for exact node
		slice = new Slice(Fragment.from(node), 0, 0)
		ranges = [{ $from: doc.resolve(nodePos), $to: doc.resolve(nodeEndPos) }]

		// Use NodeSelection for single node (doesn't expand to siblings)
		const selection = NodeSelection.create(doc, nodePos)
		tr.setSelection(selection)
	}

	const wrapper = document.createElement("div")

	ranges.forEach((range) => {
		const element = view.nodeDOM(range.$from.pos) as HTMLElement | null
		if (element) {
			const clonedElement = cloneElement(element)
			wrapper.append(clonedElement)
		}
	})

	wrapper.style.position = "absolute"
	wrapper.style.top = "-10000px"
	document.body.append(wrapper)

	event.dataTransfer.clearData()
	event.dataTransfer.setDragImage(wrapper, 0, 0)

	// Find the parent list type if dragging a list item
	const parentListType = findParentListType(doc, nodePos, node)

	// Find the source wrapper bounds if dragging a wrapped node (e.g., MetricBlock in MetricGrid)
	const sourceWrapperBounds = findSourceWrapperBounds(doc, nodePos, node)

	// Store the slice, parent list type, and wrapper bounds in view.dragging
	// The parentListType will be used when wrapping list items at the drop location
	// The sourceWrapperBounds will be used to prevent dropping adjacent to the source wrapper
	const dragging: DragHandleDragging = {
		slice,
		move: true,
		parentListType,
		sourceWrapperBounds,
	}
	view.dragging = dragging
	view.dispatch(tr)

	document.addEventListener(
		"drop",
		() => {
			removeNode(wrapper)
		},
		{ once: true },
	)
}

export interface DragHandlePluginProps {
	pluginKey?: PluginKey | string
	editor: Editor
	element: HTMLElement
	provider?: HocuspocusProvider | null | undefined
	onNodeChange?: (data: {
		editor: Editor
		node: Node | null
		pos: number
		depth: number
	}) => void
	onElementDragStart?: (e: DragEvent) => void
	onElementDragEnd?: (e: DragEvent) => void
	onDragCancel?: () => void
	computePositionConfig?:
		| (ComputePositionConfig & { yOffset?: number; xOffset?: number })
		| ((
				editor: Editor,
				pos: number,
		  ) => ComputePositionConfig & { yOffset?: number; xOffset?: number })
	getReferencedVirtualElement?: () => VirtualElement | null
	locked?: boolean
}

interface DragHandlePluginState {
	depth: number
	lastWasRemote: boolean
}

const dragHandlePluginDefaultKey = new PluginKey("dragHandle")

export const DragHandlePlugin = ({
	pluginKey = dragHandlePluginDefaultKey,
	element,
	editor,
	provider,
	getReferencedVirtualElement,
	onNodeChange,
	onElementDragStart,
	onElementDragEnd,
	onDragCancel,
	locked,
}: DragHandlePluginProps) => {
	const wrapper = document.createElement("div")
	let currentNode: Node | null = null
	let currentNodePos = -1
	let currentNodeRelPos: Y.RelativePosition | null = null // needed for yjs mapping
	let animationFrameId: number | null = null
	let pendingMouseCoords: { x: number; y: number } | null = null
	let draggingNodeUid: string | null = null // tracks the UID of the node currently being dragged

	// resolve the plugin key once for use throughout the plugin
	const resolvedPluginKey: PluginKey<DragHandlePluginState> =
		typeof pluginKey === "string" ? new PluginKey(pluginKey) : pluginKey

	function hideHandle() {
		element.style.visibility = "hidden"
		element.style.pointerEvents = "none"
	}

	function showHandle() {
		element.style.visibility = ""
		element.style.pointerEvents = "auto"
	}

	function repositionDragHandle(dom: Element, currentNodePos: number) {
		const virtualElement = getReferencedVirtualElement?.() ?? {
			getBoundingClientRect: () => dom.getBoundingClientRect(),
		}
		const config = handlePositionByNodeType(editor, currentNodePos)

		void computePosition(virtualElement, element, config).then((val) => {
			Object.assign(element.style, {
				position: val.strategy,
				left: `${val.x + config.xOffset}px`,
				top: `${val.y + config.yOffset}px`,
			})
		})
	}

	function onDragStart(e: DragEvent) {
		if (currentNodePos === -1) {
			return
		}

		// Get the dragged node to check if another user is dragging it
		const draggedNode = editor.state.doc.nodeAt(currentNodePos)
		if (!draggedNode) {
			return
		}

		// Check if another user is dragging the same node (by UID)
		const nodeUid = draggedNode.attrs.uid as string | null | undefined
		if (nodeUid && isNodeBeingDraggedByOther(provider, nodeUid)) {
			// Another user is dragging this node - prevent the drag
			e.preventDefault()

			// call the cancel callback to reset cursor and other UI state
			onDragCancel?.()

			return
		}

		onElementDragStart?.(e)
		handleDragData(e, editor, currentNodePos)

		// Broadcast that we're dragging this node
		if (nodeUid) {
			draggingNodeUid = nodeUid
			setDraggingNodeInAwareness(provider, nodeUid)
		}

		enableGapZones(editor, draggedNode, currentNodePos)

		// disable pointer events on the handle to avoid flickering during drag;
		// we need to do this after the drag event has started otherwise the
		// drag might not start
		setTimeout(() => {
			element.style.pointerEvents = "none"
		}, 0)
	}

	function clearDragAwareness() {
		if (draggingNodeUid) {
			setDraggingNodeInAwareness(provider, null)
			draggingNodeUid = null
		}
	}

	/**
	 * Cancel an in-progress drag due to a remote change moving/deleting the node.
	 * This clears the drag state and dispatches a synthetic dragend event.
	 */
	function cancelDragDueToRemoteChange() {
		// clear the drag state on the view
		if (editor.view.dragging) {
			editor.view.dragging = null
		}

		// clear awareness
		clearDragAwareness()

		// disable gap zones
		disableGapZones()

		// call the cancel callback to reset cursor and other UI state
		onDragCancel?.()

		// dispatch a synthetic dragend event to clean up any drag-related UI
		const dragEndEvent = new DragEvent("dragend", {
			bubbles: true,
			cancelable: true,
		})
		element.dispatchEvent(dragEndEvent)
	}

	function onDragEnd(e: DragEvent) {
		onElementDragEnd?.(e)
		disableGapZones()
		clearDragAwareness()

		element.style.pointerEvents = "auto"

		// redetect which node is under the cursor after the drop completes
		setTimeout(() => {
			const result = findDraggableNodeAtCoords(editor, e.clientX, e.clientY)
			if (!result) {
				hideHandle()

				currentNode = null
				currentNodePos = -1

				onNodeChange?.({ editor, node: null, pos: -1, depth: 0 })

				return
			}

			currentNode = result.node
			currentNodePos = result.pos
			currentNodeRelPos = findRelativePos(editor.view.state, currentNodePos)

			onNodeChange?.({
				editor,
				node: currentNode,
				pos: currentNodePos,
				depth: result.depth,
			})
			repositionDragHandle(result.dom, currentNodePos)
			showHandle()
		}, 10)
	}

	element.addEventListener("dragstart", onDragStart)
	element.addEventListener("dragend", onDragEnd)

	wrapper.appendChild(element)
	hideHandle()

	return {
		unbind() {
			element.removeEventListener("dragstart", onDragStart)
			element.removeEventListener("dragend", onDragEnd)

			if (animationFrameId) {
				cancelAnimationFrame(animationFrameId)
				animationFrameId = null
				pendingMouseCoords = null
			}
		},
		plugin: new Plugin<DragHandlePluginState>({
			key: resolvedPluginKey,

			state: {
				init() {
					return { depth: 0, lastWasRemote: false }
				},
				apply(
					tr: Transaction,
					value: DragHandlePluginState,
					_oldState: EditorState,
					state: EditorState,
				) {
					const isLocked = tr.getMeta("lockDragHandle") as boolean | undefined
					const hideDragHandle = tr.getMeta("hideDragHandle") as
						boolean | undefined

					// track whether this transaction is a remote change for the view update
					const isRemote = tr.docChanged && isChangeOrigin(tr)

					if (isLocked !== undefined) {
						locked = isLocked
					}

					if (hideDragHandle) {
						hideHandle()

						locked = false
						currentNode = null
						currentNodePos = -1

						onNodeChange?.({ editor, node: null, pos: -1, depth: 0 })

						return {
							...value,
							depth: 0,
							lastWasRemote: isRemote,
						}
					}

					if (tr.docChanged && currentNodePos !== -1) {
						if (isChangeOrigin(tr)) {
							// For remote Y.js changes, use relative positions to track the node.
							// Don't use ProseMirror's mapping as it may incorrectly report
							// the position as deleted during Y.js conflict resolution.
							const newPos = findAbsolutePos(state, currentNodeRelPos)

							// if relative position couldn't be resolved, the node was truly deleted
							if (newPos === -1) {
								hideHandle()
								currentNode = null
								currentNodePos = -1
								onNodeChange?.({ editor, node: null, pos: -1, depth: 0 })
								return {
									...value,
									depth: 0,
									lastWasRemote: isRemote,
								}
							}

							// if we're dragging and the node moved due to a remote change,
							// cancel the drag to prevent duplication/node merging issues
							if (draggingNodeUid && newPos !== currentNodePos) {
								cancelDragDueToRemoteChange()
							}

							if (newPos !== currentNodePos) {
								currentNodePos = newPos
								// update currentNode to match the new position in the updated doc
								currentNode = state.doc.nodeAt(currentNodePos)
							}
						} else {
							// For local changes, use ProseMirror's mapping
							const mapResult = tr.mapping.mapResult(currentNodePos)
							if (mapResult.deleted) {
								// node was deleted - hide handle
								hideHandle()

								currentNode = null
								currentNodePos = -1
								onNodeChange?.({ editor, node: null, pos: -1, depth: 0 })
								return {
									...value,
									depth: 0,
									lastWasRemote: isRemote,
								}
							}

							const newPos = tr.mapping.map(currentNodePos)
							if (newPos !== currentNodePos) {
								currentNodePos = newPos
								currentNodeRelPos = findRelativePos(state, currentNodePos)
							}
						}
					}

					return { ...value, lastWasRemote: isRemote }
				},
			},
			view: (view) => {
				element.draggable = editor.isEditable
				element.style.pointerEvents = "auto"

				editor.view.dom.parentElement?.appendChild(wrapper)

				wrapper.style.pointerEvents = "none"
				wrapper.style.position = "absolute"
				wrapper.style.top = "0"
				wrapper.style.left = "0"

				return {
					update(_, oldState) {
						element.draggable = editor.isEditable

						if (locked) {
							element.draggable = false
							return // don't reposition to another node while menu is open/closing
						}

						if (view.state.doc.eq(oldState.doc) || currentNodePos === -1) {
							return
						}

						// Skip repositioning for remote changes to prevent flickering.
						// During live collaboration, remote changes can cause rapid
						// document updates that temporarily make DOM lookups unstable.
						// The handle position is already tracked via Y.js relative
						// positions in apply(), and will be repositioned on next
						// mousemove or local change.
						const pluginState = resolvedPluginKey.getState(view.state)
						if (pluginState?.lastWasRemote) {
							return
						}

						// Just reposition the handle to the current node's DOM position.
						// Don't re-detect which node to show the handle for - that would
						// cause flickering during live collaboration when text changes shift
						// positions slightly. The node detection only happens on mousemove.
						const dom = view.nodeDOM(currentNodePos) as HTMLElement | null
						if (dom?.nodeType === 1) {
							repositionDragHandle(dom, currentNodePos)
						}
					},
					destroy() {
						// clear awareness if we were mid-drag
						clearDragAwareness()

						if (animationFrameId) {
							cancelAnimationFrame(animationFrameId)
							animationFrameId = null
							pendingMouseCoords = null
						}

						removeNode(wrapper)
					},
				}
			},
			props: {
				handleDOMEvents: {
					keydown(view) {
						if (locked) {
							return false
						}

						if (view.hasFocus()) {
							hideHandle()
							currentNode = null
							currentNodePos = -1
							onNodeChange?.({ editor, node: null, pos: -1, depth: 0 })

							return false
						}

						return false
					},
					mouseleave(_view, e) {
						if (locked) {
							return false
						}

						if (e.target && !wrapper.contains(e.relatedTarget as HTMLElement)) {
							hideHandle()

							currentNode = null
							currentNodePos = -1

							onNodeChange?.({ editor, node: null, pos: -1, depth: 0 })
						}

						return false
					},
					mousemove(view, e) {
						if (locked) {
							return false
						}

						pendingMouseCoords = { x: e.clientX, y: e.clientY }

						if (animationFrameId) {
							return false
						}

						animationFrameId = requestAnimationFrame(() => {
							animationFrameId = null

							if (!pendingMouseCoords) {
								return
							}

							const { x, y } = pendingMouseCoords
							pendingMouseCoords = null

							// Use the new nested-aware finder
							const result = findDraggableNodeAtCoords(editor, x, y)
							if (!result) {
								return
							}

							const { node, pos: nodePos, depth, dom: domNode } = result

							// Compare by position only, not by node reference, to avoid
							// false positives when the same logical node gets a new object
							// reference after document updates
							if (nodePos !== currentNodePos) {
								currentNode = node
								currentNodePos = nodePos
								currentNodeRelPos = findRelativePos(view.state, currentNodePos)

								onNodeChange?.({
									editor,
									node: currentNode,
									pos: currentNodePos,
									depth: depth,
								})
								repositionDragHandle(domNode, currentNodePos)
								showHandle()
							}
						})

						return false
					},
				},
			},
		}),
	}
}
