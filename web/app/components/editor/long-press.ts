const LONG_PRESS_DELAY = 500
const MOVE_THRESHOLD = 10
const RELEASE_CLICK_DELAY = 800
const INTERACTIVE_TARGETS =
	'a, button, input, textarea, select, [role="button"]'

export function bindTouchLongPress(
	element: HTMLElement,
	onLongPress: (event: PointerEvent) => boolean,
): () => void {
	const controller = new AbortController()
	const options = { capture: true, signal: controller.signal }
	let pending: PointerEvent | null = null
	let timer: ReturnType<typeof setTimeout> | undefined
	let suppressUntil = 0

	function cancel() {
		clearTimeout(timer)
		pending = null
		suppressUntil = 0
	}

	function onPointerDown(event: PointerEvent) {
		if (
			event.pointerType !== "touch" ||
			!event.isPrimary ||
			event.button !== 0 ||
			event.defaultPrevented ||
			(event.target instanceof Element &&
				event.target.closest(INTERACTIVE_TARGETS)) ||
			document.getSelection()?.isCollapsed === false
		) {
			return
		}

		pending = event
		timer = setTimeout(() => {
			if (onLongPress(event)) {
				suppressUntil = Infinity
			}
		}, LONG_PRESS_DELAY)
	}

	function onPointerMove(event: PointerEvent) {
		if (
			!suppressUntil &&
			pending?.pointerId === event.pointerId &&
			Math.hypot(
				event.clientX - pending.clientX,
				event.clientY - pending.clientY,
			) >= MOVE_THRESHOLD
		) {
			cancel()
		}
	}

	function onScroll() {
		if (!suppressUntil) {
			cancel()
		}
	}

	function onPointerUp(event: PointerEvent) {
		if (pending?.pointerId !== event.pointerId) {
			return
		}

		clearTimeout(timer)
		pending = null

		if (suppressUntil) {
			suppressUntil = Date.now() + RELEASE_CLICK_DELAY
			// menu items can turn pointerup into a programmatic click, before
			// the browser emits its own release click.
			event.preventDefault()
			event.stopImmediatePropagation()
		}
	}

	function onPointerCancel(event: PointerEvent) {
		if (pending?.pointerId === event.pointerId) {
			clearTimeout(timer)
			pending = null
			// native long-press handling can cancel the pointer before its
			// context menu arrives, without a later pointerup to end the hold.
			suppressUntil = suppressUntil ? Date.now() + RELEASE_CLICK_DELAY : 0
		}
	}

	function onContextMenu(event: MouseEvent) {
		if (Date.now() < suppressUntil) {
			event.preventDefault()
		}
	}

	function onClick(event: MouseEvent) {
		if (event.detail && Date.now() < suppressUntil) {
			cancel()
			event.preventDefault()
			event.stopImmediatePropagation()
		}
	}

	// a new pointer cancels multi-touch gestures and clears suppression before
	// the next deliberate tap, including a tap on the newly opened menu.
	document.addEventListener("pointerdown", cancel, options)
	element.addEventListener("pointerdown", onPointerDown, options)
	document.addEventListener("pointermove", onPointerMove, options)
	document.addEventListener("pointerup", onPointerUp, options)
	document.addEventListener("pointercancel", onPointerCancel, options)
	// a menu can appear under the finger, so the release click may target its
	// portal outside the element that received the original touch. Its native
	// context menu can be retargeted there too.
	document.addEventListener("contextmenu", onContextMenu, options)
	document.addEventListener("click", onClick, options)
	window.addEventListener("scroll", onScroll, options)
	window.addEventListener("blur", cancel, { signal: controller.signal })

	return () => {
		cancel()
		controller.abort()
	}
}
