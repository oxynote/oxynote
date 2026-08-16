import { InputRule } from "@tiptap/core"
import type { EditorState } from "@tiptap/pm/state"
import type { Node as ProseMirrorNode } from "@tiptap/pm/model"
import { cn } from "~/lib/utils"
import { ListItem } from "@tiptap/extension-list"
import { ImageBlock } from "~/components/editor/blocks/image"

export const SCROLL_HIGHLIGHT_EDITOR_CLASS = "editor-scroll-highlight-container"
const SCROLL_HIGHLIGHT_OVERLAY_CLASS = "editor-scroll-highlight"
const SCROLL_HIGHLIGHT_VERTICAL_OFFSET = 5
const SCROLL_HIGHLIGHT_HORIZONTAL_OFFSET = 5

export interface NodeOverlayOffset {
	extraTop: number
	extraBottom: number
	extraLeft: number
	extraRight: number
}

export function nodeOverlayOffset(nodeDOM: HTMLElement): NodeOverlayOffset {
	const defaults: NodeOverlayOffset = {
		extraTop: 0,
		extraBottom: 0,
		extraLeft: 0,
		extraRight: 0,
	}

	// Check data-node-type attribute first (set by Tiptap node views)
	const nodeType = nodeDOM.getAttribute("data-type")

	switch (nodeType) {
		case ListItem.name:
			return { ...defaults, extraLeft: 25 }
		case ImageBlock.name: {
			const img =
				nodeDOM.querySelector("img") ??
				nodeDOM.firstElementChild?.querySelector("img")

			if (!img) {
				return defaults
			}

			const rootRect = nodeDOM.getBoundingClientRect()
			const rootWidth =
				rootRect.width ||
				nodeDOM.offsetWidth ||
				nodeDOM.parentElement?.getBoundingClientRect().width ||
				0

			const imgRect = img.getBoundingClientRect()
			const imgWidth =
				imgRect.width ||
				img.offsetWidth ||
				img.parentElement?.getBoundingClientRect().width ||
				0

			return { ...defaults, extraRight: imgWidth - rootWidth }
		}
	}

	// Fallback to tag name
	switch (nodeDOM.tagName) {
		case "LI": {
			const isTaskItem =
				nodeDOM.hasAttribute("data-checked") ||
				nodeDOM.querySelector('input[type="checkbox"]') !== null
			if (!isTaskItem) {
				return { ...defaults, extraLeft: 25 }
			}
		}
	}

	return defaults
}

export function highlightTiptapScrollElement(
	elementId: string,
	element: HTMLElement,
) {
	if (typeof window === "undefined") {
		return
	}

	if (typeof document === "undefined") {
		return
	}

	const target =
		element.isConnected || !elementId
			? element
			: (document.getElementById(elementId) ?? element)

	if (!target.isConnected) {
		return
	}

	// Remove any existing overlays first, cleaning up their observers
	document
		.querySelectorAll(`.${SCROLL_HIGHLIGHT_OVERLAY_CLASS}`)
		.forEach((node) => {
			const el = node as HTMLElement & { _cleanup?: () => void }
			el._cleanup?.()
			node.remove()
		})

	// Get the container early so we can calculate relative positions
	const container = document.querySelector(`.${SCROLL_HIGHLIGHT_EDITOR_CLASS}`)
	if (!container) {
		return
	}

	const overlay = document.createElement("div")
	overlay.className = cn(
		SCROLL_HIGHLIGHT_OVERLAY_CLASS,
		"absolute pointer-events-none rounded-md z-editor-overlay bg-drag-target/40 data-hidden:opacity-0 transition-opacity duration-250",
	)

	// Append immediately to prevent duplicates from race conditions
	container.appendChild(overlay)

	let retried = false

	const updateOverlayPosition = () => {
		const rect = target.getBoundingClientRect()
		const containerRect = container.getBoundingClientRect()
		const width =
			rect.width ||
			target.offsetWidth ||
			target.parentElement?.getBoundingClientRect().width ||
			0
		const height = rect.height || target.offsetHeight || 0

		if ((width === 0 || height === 0) && !retried) {
			retried = true
			requestAnimationFrame(updateOverlayPosition)
			return
		}

		if (width === 0 || height === 0) {
			return
		}

		const offsets = nodeOverlayOffset(target)

		// Calculate position relative to the container, accounting for its scroll position
		const scrollTop = container instanceof HTMLElement ? container.scrollTop : 0
		const scrollLeft =
			container instanceof HTMLElement ? container.scrollLeft : 0

		const top = rect.top - containerRect.top + scrollTop
		const left = rect.left - containerRect.left + scrollLeft

		overlay.style.top = `${top - offsets.extraTop - SCROLL_HIGHLIGHT_VERTICAL_OFFSET}px`
		overlay.style.left = `${left - offsets.extraLeft - SCROLL_HIGHLIGHT_HORIZONTAL_OFFSET}px`
		overlay.style.height = `${height + offsets.extraTop + offsets.extraBottom + SCROLL_HIGHLIGHT_VERTICAL_OFFSET * 2}px`
		overlay.style.width = `${width + offsets.extraLeft + offsets.extraRight + SCROLL_HIGHLIGHT_HORIZONTAL_OFFSET * 2}px`
	}

	// Use ResizeObserver to efficiently update overlay on element size changes
	const resizeObserver = new ResizeObserver(() => {
		requestAnimationFrame(updateOverlayPosition)
	})
	resizeObserver.observe(target)

	// Store cleanup function on the overlay element for later disposal
	;(overlay as HTMLElement & { _cleanup?: () => void })._cleanup = () => {
		resizeObserver.disconnect()
	}

	// Initial position update
	requestAnimationFrame(updateOverlayPosition)
}

export function clearTiptapScrollElementHighlightOverlays() {
	if (typeof window === "undefined") {
		return
	}

	if (typeof document === "undefined") {
		return
	}

	document
		.querySelectorAll(`.${SCROLL_HIGHLIGHT_OVERLAY_CLASS}`)
		.forEach((node) => {
			node.setAttribute("data-hidden", "")
		})

	setTimeout(() => {
		document
			.querySelectorAll(`.${SCROLL_HIGHLIGHT_OVERLAY_CLASS}`)
			.forEach((node) => {
				// Disconnect ResizeObserver before removing
				const overlay = node as HTMLElement & { _cleanup?: () => void }
				overlay._cleanup?.()

				node.remove()
			})
	}, 300)
}

export function isCursorInsideTiptapNode(
	state: EditorState,
	nodeTypeNames: string[],
): boolean {
	const { from, to } = state.selection
	let found = false

	state.doc.nodesBetween(from, to, (node: ProseMirrorNode) => {
		if (nodeTypeNames.includes(node.type.name)) {
			found = true
			return false
		}

		return !found
	})

	return found
}

export function isCursorInsideTiptapMark(
	state: EditorState,
	markName: string,
): boolean {
	const markType = state.schema.marks[markName]
	if (!markType) {
		return false
	}

	const { from, to } = state.selection
	let found = false

	state.doc.nodesBetween(from, to, (node: ProseMirrorNode) => {
		if (markType.isInSet(node.marks)) {
			found = true
			return false
		}

		return !found
	})

	return found
}

export function tiptapNodePosByCursor(
	state: EditorState,
	nodeTypeNames: string[],
) {
	const { $from } = state.selection
	for (let depth = $from.depth; depth > 0; depth--) {
		const node = $from.node(depth)
		if (nodeTypeNames.includes(node.type.name)) {
			const pos = $from.before(depth)
			return { pos, node }
		}
	}
	return null
}

export function isNodeInsideTiptapNode(
	state: EditorState,
	nodePos: number,
	nodeTypeNames: string[],
) {
	const pos = state.doc.resolve(nodePos)

	for (let depth = pos.depth; depth > 0; depth--) {
		if (nodeTypeNames.includes(pos.node(depth).type.name)) {
			return true
		}
	}

	return false
}

export function preventTiptapInputRuleInNode(
	rule: InputRule,
	allowedContainerNodes: string[],
	rootOnly = false,
) {
	return new InputRule({
		find: rule.find,
		handler: (props) => {
			const { state } = props
			const $pos = state.selection.$from

			if (rootOnly && $pos.depth != 1) {
				return
			}

			// walk up the tree and block the rule if inside any blocked
			// container node
			if (allowedContainerNodes.length) {
				for (let depth = $pos.depth; depth > 0; depth--) {
					const node = $pos.node(depth)
					if (!allowedContainerNodes.includes(node.type.name)) {
						return // block
					}
				}
			}

			return rule.handler(props)
		},
	})
}
