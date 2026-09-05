import { afterEach, describe, it, vi } from "vitest"
import {
	DEFAULT_HIGHLIGHT_OVERLAY_PADDING,
	useHighlightOverlay,
} from "./highlight-overlay"

// happy-dom measures every element as a zero-sized box at the origin, so
// the target reports the geometry the panel is derived from
function targetAt(rect: {
	left: number
	top: number
	width: number
	height: number
}): HTMLElement {
	const el = document.createElement("div")
	document.body.appendChild(el)

	vi.spyOn(el, "getBoundingClientRect").mockReturnValue({
		...rect,
		right: rect.left + rect.width,
		bottom: rect.top + rect.height,
		x: rect.left,
		y: rect.top,
		toJSON: () => ({}),
	})

	return el
}

function panels(): NodeListOf<Element> {
	return document.body.querySelectorAll("[aria-hidden='true'].z-editor-overlay")
}

// the panels live on the shared body, so these tests cannot interleave
describe("useHighlightOverlay", { concurrent: false }, () => {
	afterEach(() => {
		document.body.replaceChildren()
	})

	it("covers the target's box widened by the padding", ({ expect }) => {
		const { show } = useHighlightOverlay()
		const target = targetAt({ left: 100, top: 40, width: 300, height: 60 })

		show(target, DEFAULT_HIGHLIGHT_OVERLAY_PADDING)

		const panel = panels()[0] as HTMLElement | undefined
		expect(panel?.style.left).toBe("95px")
		expect(panel?.style.top).toBe("35px")
		expect(panel?.style.width).toBe("310px")
		expect(panel?.style.height).toBe("70px")
	})

	it("grows with a target that has wrapped onto another line", ({ expect }) => {
		const { show } = useHighlightOverlay()
		const target = targetAt({ left: 0, top: 0, width: 200, height: 96 })

		show(target, DEFAULT_HIGHLIGHT_OVERLAY_PADDING)

		expect((panels()[0] as HTMLElement | undefined)?.style.height).toBe("106px")
	})

	it("applies each side's padding on its own", ({ expect }) => {
		const { show } = useHighlightOverlay()
		const target = targetAt({ left: 100, top: 40, width: 300, height: 60 })

		show(target, {
			extraLeft: 30,
			extraRight: 1,
			extraTop: 8,
			extraBottom: 2,
		})

		const panel = panels()[0] as HTMLElement | undefined
		expect(panel?.style.left).toBe("70px")
		expect(panel?.style.top).toBe("32px")
		expect(panel?.style.width).toBe("331px")
		expect(panel?.style.height).toBe("70px")
	})

	it("keeps one panel when shown again", ({ expect }) => {
		const { show } = useHighlightOverlay()
		const target = targetAt({ left: 0, top: 0, width: 10, height: 10 })

		show(target, DEFAULT_HIGHLIGHT_OVERLAY_PADDING)
		show(target, DEFAULT_HIGHLIGHT_OVERLAY_PADDING)

		expect(panels()).toHaveLength(1)
	})

	it("takes the panel away when hidden", ({ expect }) => {
		const { show, hide } = useHighlightOverlay()
		const target = targetAt({ left: 0, top: 0, width: 10, height: 10 })
		show(target, DEFAULT_HIGHLIGHT_OVERLAY_PADDING)

		hide()

		expect(panels()).toHaveLength(0)
	})

	it("hides without a panel to take away", ({ expect }) => {
		const { hide } = useHighlightOverlay()

		expect(() => {
			hide()
		}).not.toThrow()
	})
})
