import { afterEach, beforeEach, describe, it, vi } from "vitest"
import {
	SCROLL_HIGHLIGHT_EDITOR_CLASS,
	clearTiptapScrollElementHighlightOverlays,
	highlightTiptapScrollElement,
	nodeOverlayOffset,
} from "./tiptap"

const OVERLAY_SELECTOR = ".editor-scroll-highlight"

// the page DOM and the stubbed globals (requestAnimationFrame,
// ResizeObserver, timers) are shared between tests, so they cannot
// interleave
describe("nodeOverlayOffset", { concurrent: false }, () => {
	afterEach(() => {
		document.body.innerHTML = ""
	})

	function attach<T extends HTMLElement>(el: T): T {
		document.body.appendChild(el)
		return el
	}

	it("returns zero offsets for a plain element", ({ expect }) => {
		expect(nodeOverlayOffset(attach(document.createElement("div")))).toEqual({
			extraTop: 0,
			extraBottom: 0,
			extraLeft: 0,
			extraRight: 0,
		})
	})

	it("pads list items on the left", ({ expect }) => {
		const el = attach(document.createElement("div"))
		el.setAttribute("data-type", "listItem")

		expect(nodeOverlayOffset(el)).toMatchObject({ extraLeft: 25 })
	})

	it("extends image blocks by the real image overhang", ({ expect }) => {
		const el = attach(document.createElement("div"))
		el.setAttribute("data-type", "imageBlock")
		el.style.width = "200px"
		const img = document.createElement("img")
		img.style.display = "block"
		img.style.width = "320px"
		el.appendChild(img)

		expect(nodeOverlayOffset(el)).toMatchObject({ extraRight: 120 })
	})

	it("returns zero offsets for an image block without an image", ({
		expect,
	}) => {
		const el = attach(document.createElement("div"))
		el.setAttribute("data-type", "imageBlock")

		expect(nodeOverlayOffset(el)).toMatchObject({ extraRight: 0 })
	})

	it("pads plain li elements on the left", ({ expect }) => {
		expect(
			nodeOverlayOffset(attach(document.createElement("li"))),
		).toMatchObject({ extraLeft: 25 })
	})

	it("leaves task list items unpadded", ({ expect }) => {
		const el = attach(document.createElement("li"))
		el.setAttribute("data-checked", "false")

		expect(nodeOverlayOffset(el)).toMatchObject({ extraLeft: 0 })
	})

	it("leaves li elements containing a checkbox unpadded", ({ expect }) => {
		const el = attach(document.createElement("li"))
		const checkbox = document.createElement("input")
		checkbox.type = "checkbox"
		el.appendChild(checkbox)

		expect(nodeOverlayOffset(el)).toMatchObject({ extraLeft: 0 })
	})
})

describe("highlightTiptapScrollElement", { concurrent: false }, () => {
	class ResizeObserverStub {
		static instances: ResizeObserverStub[] = []

		observe = vi.fn()
		disconnect = vi.fn()

		constructor() {
			ResizeObserverStub.instances.push(this)
		}
	}

	beforeEach(() => {
		ResizeObserverStub.instances = []
		vi.stubGlobal("ResizeObserver", ResizeObserverStub)

		// run animation frames synchronously so positioning is immediate
		// and deterministic
		vi.stubGlobal(
			"requestAnimationFrame",
			(cb: FrameRequestCallback): number => {
				cb(0)
				return 0
			},
		)
	})

	afterEach(() => {
		document.body.innerHTML = ""
	})

	// a fixed-position container at (10, 10) with an absolutely
	// positioned target at (40, 20) inside it, so every rect the
	// overlay math reads comes from real layout
	function setupDom() {
		const container = document.createElement("div")
		container.className = SCROLL_HIGHLIGHT_EDITOR_CLASS
		container.style.position = "fixed"
		container.style.top = "10px"
		container.style.left = "10px"
		container.style.width = "400px"
		container.style.height = "300px"

		const target = document.createElement("div")
		target.style.position = "absolute"
		target.style.top = "20px"
		target.style.left = "40px"
		target.style.width = "100px"
		target.style.height = "20px"

		container.appendChild(target)
		document.body.appendChild(container)

		return { container, target }
	}

	function overlays() {
		return document.querySelectorAll(OVERLAY_SELECTOR)
	}

	it("does nothing without a highlight container", ({ expect }) => {
		const target = document.createElement("div")
		document.body.appendChild(target)

		highlightTiptapScrollElement("", target)

		expect(overlays()).toHaveLength(0)
	})

	it("does nothing for a disconnected element without an id fallback", ({
		expect,
	}) => {
		setupDom()

		highlightTiptapScrollElement("", document.createElement("div"))

		expect(overlays()).toHaveLength(0)
	})

	it("positions the overlay over the target's real layout", ({ expect }) => {
		const { target } = setupDom()

		highlightTiptapScrollElement("", target)

		const overlay = overlays()[0] as HTMLElement
		expect(overlay).toBeDefined()
		expect(ResizeObserverStub.instances[0]?.observe).toHaveBeenCalledWith(
			target,
		)
		// target rect (50, 30) relative to container rect (10, 10), minus
		// the 5px highlight offsets; size 100x20 plus 5px on each side
		expect(overlay.style.top).toBe("15px")
		expect(overlay.style.left).toBe("35px")
		expect(overlay.style.height).toBe("30px")
		expect(overlay.style.width).toBe("110px")
	})

	it("falls back to looking the target up by id", ({ expect }) => {
		const { target } = setupDom()
		target.id = "block-1"

		highlightTiptapScrollElement("block-1", document.createElement("div"))

		expect(overlays()).toHaveLength(1)
	})

	it("replaces an existing overlay and disconnects its observer", ({
		expect,
	}) => {
		const { target } = setupDom()

		highlightTiptapScrollElement("", target)
		highlightTiptapScrollElement("", target)

		expect(overlays()).toHaveLength(1)
		expect(ResizeObserverStub.instances[0]?.disconnect).toHaveBeenCalledTimes(1)
	})

	it("leaves the overlay unpositioned when the target has no size", ({
		expect,
	}) => {
		const { target } = setupDom()
		target.style.display = "none"

		highlightTiptapScrollElement("", target)

		const overlay = overlays()[0] as HTMLElement
		expect(overlay).toBeDefined()
		expect(overlay.style.top).toBe("")
	})
})

describe(
	"clearTiptapScrollElementHighlightOverlays",
	{ concurrent: false },
	() => {
		afterEach(() => {
			document.body.innerHTML = ""
		})

		function appendOverlay() {
			const overlay = document.createElement("div") as HTMLElement & {
				_cleanup?: () => void
			}
			overlay.className = "editor-scroll-highlight"
			overlay._cleanup = vi.fn()
			document.body.appendChild(overlay)

			return overlay
		}

		it("hides overlays immediately and removes them after the fade", ({
			expect,
		}) => {
			vi.useFakeTimers()

			try {
				const overlay = appendOverlay()

				clearTiptapScrollElementHighlightOverlays()

				expect(overlay.hasAttribute("data-hidden")).toBe(true)
				expect(overlay.isConnected).toBe(true)

				vi.advanceTimersByTime(300)

				expect(overlay.isConnected).toBe(false)
				expect(overlay._cleanup).toHaveBeenCalledTimes(1)
			} finally {
				vi.useRealTimers()
			}
		})
	},
)
