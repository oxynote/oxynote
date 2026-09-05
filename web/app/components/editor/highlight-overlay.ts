import { cn } from "~/lib/utils"

// HighlightOverlayRect is the box the panel covers, in viewport
// coordinates. A DOMRect satisfies it, and so does a box assembled from
// several of them.
export interface HighlightOverlayRect {
	left: number
	top: number
	width: number
	height: number
}

// HighlightOverlayPadding widens the panel past the box it covers. A
// block's own padding is part of it, so the amount differs by node type.
export interface HighlightOverlayPadding {
	extraLeft: number
	extraRight: number
	extraTop: number
	extraBottom: number
}

export const DEFAULT_HIGHLIGHT_OVERLAY_PADDING: HighlightOverlayPadding = {
	extraLeft: 5,
	extraRight: 5,
	extraTop: 5,
	extraBottom: 5,
}

// useHighlightOverlay paints the translucent panel that marks what a
// handle points at — the block under the drag handle, the document under
// the name editor's hook handle.
//
// The panel is fixed to the viewport and appended to the body, so no
// editor ancestor can clip it, and the caller measures the box afresh
// each time it is shown: a title that has wrapped onto a second line is
// covered as it stands. That also means it does not follow the page, and
// the caller drops it on scroll.
export function useHighlightOverlay() {
	let element: HTMLElement | null = null

	function show(rect: HighlightOverlayRect, padding: HighlightOverlayPadding) {
		hide()

		const el = document.createElement("div")
		el.setAttribute("aria-hidden", "true")
		el.className = cn(
			"fixed left-0 top-0 w-0 h-0 pointer-events-none z-editor-overlay transition-none rounded-md bg-drag-target/20",
		)

		document.body.appendChild(el)
		element = el

		el.style.left = `${rect.left - padding.extraLeft}px`
		el.style.top = `${rect.top - padding.extraTop}px`
		el.style.width = `${rect.width + padding.extraLeft + padding.extraRight}px`
		el.style.height = `${rect.height + padding.extraTop + padding.extraBottom}px`
	}

	function hide() {
		element?.remove()
		element = null
	}

	return { show, hide }
}
