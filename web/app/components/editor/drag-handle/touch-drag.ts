// a native drag only starts from a real pointer pressed on a draggable
// element, never from script. A touch drag replays the desktop drag's
// events instead, so the handle, the drop cursor, the gap zones and the
// drop all run as they do for a mouse.
//
// A block drops where the finger is, with one exception. Each list level
// takes drops from its own left edge rightwards, so how far left the drag
// goes picks the level a list item lands in. A finger holds the middle of
// the item, which would always pick the deepest level, so a list item's
// drag follows the start of its text, clear of any checkbox, and its first
// line, moved as far as the finger has.

// how close to the top or bottom of the screen the finger scrolls the page
const EDGE_ZONE_REM = 3
// how far the page scrolls each frame while the finger is in that zone
const EDGE_STEP_REM = 0.75
const PREVIEW_OPACITY = "0.7"
// how far inside a list item's text the drag point sits
const TEXT_INSET_REM = 0.25

export interface TouchDrag {
	move: (event: PointerEvent) => void
	drop: (event: PointerEvent) => void
	cancel: () => void
}

// startTouchDrag starts dragging the block the handle is on, measuring the
// finger's movement from where it first held the block. A list item is
// dragged by the start of its text and stays inside the editor's left
// edge, so a finger drifting off it drops at the outermost level. The
// preview, a copy of the block made once the drag is accepted, follows the
// finger. It returns null when a dragstart listener refuses the drag, as
// read mode does.
export function startTouchDrag(
	handle: HTMLElement,
	block: HTMLElement,
	copyBlock: () => HTMLElement,
	heldAt: { x: number; y: number },
	listItem: { textLeft: number; editorLeft: number } | null,
): TouchDrag | null {
	const rect = block.getBoundingClientRect()
	const inset = TEXT_INSET_REM * remSize()
	const handleRect = handle.getBoundingClientRect()
	const startX = listItem ? listItem.textLeft + inset : heldAt.x
	const startY = listItem ? handleRect.top + handleRect.height / 2 : heldAt.y
	const dataTransfer = new DataTransfer()
	if (
		!handle.dispatchEvent(dragEvent("dragstart", startX, startY, dataTransfer))
	) {
		return null
	}

	const preview = copyBlock()
	let x = heldAt.x
	let y = heldAt.y
	let dragX = startX
	let dragY = startY
	let frame: number | null = null

	// the preview lies over the drag point, and a copy may carry its own
	// pointer-events on every element. Inert keeps all of it out of the
	// hit test, so each move reaches what lies under the preview.
	preview.inert = true
	Object.assign(preview.style, {
		position: "fixed",
		left: `${rect.left}px`,
		top: `${rect.top}px`,
		width: `${rect.width}px`,
		margin: "0",
		opacity: PREVIEW_OPACITY,
		zIndex: "var(--z-index-draggable)",
	})
	document.body.append(preview)

	function follow(event: PointerEvent) {
		x = event.clientX
		y = event.clientY
		dragX = listItem
			? Math.max(listItem.editorLeft + inset, startX + x - heldAt.x)
			: x
		dragY = startY + y - heldAt.y
		preview.style.top = `${rect.top + y - heldAt.y}px`
		// the preview may shift left past the screen, but not right, where
		// it would widen the page
		preview.style.left = `${Math.min(rect.left + x - heldAt.x, document.documentElement.clientWidth - rect.width)}px`
	}

	function dragOver() {
		targetAt(dragX, dragY).dispatchEvent(
			dragEvent("dragover", dragX, dragY, dataTransfer),
		)
	}

	// the page moves under a still finger, so each step points the drag at
	// whatever is under it now
	function scrollAtEdge() {
		frame = null

		const step = edgeStep(y)
		if (!step) {
			return
		}

		window.scrollBy(0, step)
		dragOver()
		frame = requestAnimationFrame(scrollAtEdge)
	}

	function finish() {
		if (frame !== null) {
			cancelAnimationFrame(frame)
			frame = null
		}

		preview.remove()
		handle.dispatchEvent(dragEvent("dragend", dragX, dragY, dataTransfer))
	}

	return {
		move(event) {
			follow(event)
			dragOver()
			frame ??= requestAnimationFrame(scrollAtEdge)
		},
		drop(event) {
			follow(event)
			targetAt(dragX, dragY).dispatchEvent(
				dragEvent("drop", dragX, dragY, dataTransfer),
			)
			finish()
		},
		// a cancelled drag leaves its target before it ends, as a native one
		// does, which takes the drop cursor away
		cancel() {
			targetAt(dragX, dragY).dispatchEvent(
				dragEvent("dragleave", dragX, dragY, dataTransfer),
			)
			finish()
		},
	}
}

function targetAt(x: number, y: number): Element {
	return document.elementFromPoint(x, y) ?? document.body
}

function remSize(): number {
	return parseFloat(getComputedStyle(document.documentElement).fontSize)
}

function edgeStep(y: number): number {
	const rem = remSize()
	const zone = EDGE_ZONE_REM * rem
	if (y < zone) {
		return -EDGE_STEP_REM * rem
	}

	if (y > window.innerHeight - zone) {
		return EDGE_STEP_REM * rem
	}

	return 0
}

function dragEvent(
	type: string,
	clientX: number,
	clientY: number,
	dataTransfer: DataTransfer,
): DragEvent {
	return new DragEvent(type, {
		bubbles: true,
		cancelable: true,
		clientX,
		clientY,
		dataTransfer,
	})
}
