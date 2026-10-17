import { afterEach, beforeEach, describe, it, vi } from "vitest"
import { bindTouchLongPress } from "./long-press"

const cleanups: (() => void)[] = []

// fake timers, global pointer listeners and document selection are shared.
describe("bindTouchLongPress", { concurrent: false }, () => {
	beforeEach(() => {
		vi.useFakeTimers()
	})

	afterEach(() => {
		for (const cleanup of cleanups.splice(0)) {
			cleanup()
		}

		document.getSelection()?.removeAllRanges()
		vi.useRealTimers()
	})

	it("opens once after a stationary primary touch lasts 500 ms", ({
		expect,
	}) => {
		const { element, callback } = mount()
		const event = pointer(element, "pointerdown")
		vi.advanceTimersByTime(499)
		expect(callback).not.toHaveBeenCalled()

		vi.advanceTimersByTime(501)

		expect.soft(callback).toHaveBeenCalledExactlyOnceWith(event)
		expect.soft(event.defaultPrevented).toBe(false)
	})

	it.for([
		{ name: "mouse", pointerType: "mouse", isPrimary: true, button: 0 },
		{ name: "pen", pointerType: "pen", isPrimary: true, button: 0 },
		{
			name: "secondary touch",
			pointerType: "touch",
			isPrimary: false,
			button: 0,
		},
		{
			name: "secondary button",
			pointerType: "touch",
			isPrimary: true,
			button: 2,
		},
	])("ignores $name presses", (init, { expect }) => {
		const { element, callback } = mount()

		pointer(element, "pointerdown", init)
		vi.advanceTimersByTime(500)

		expect(callback).not.toHaveBeenCalled()
	})

	it("leaves an already handled pointer press alone", ({ expect }) => {
		const { element, callback } = mount()
		const event = touchEvent("pointerdown")
		event.preventDefault()

		element.dispatchEvent(event)
		vi.advanceTimersByTime(500)

		expect(callback).not.toHaveBeenCalled()
	})

	it.for(["a", "button", "input", "textarea", "select", "[role=button]"])(
		"leaves %s controls and their children alone",
		(tag, { expect }) => {
			const { element, callback } = mount()
			const control = document.createElement(
				tag === "[role=button]" ? "div" : tag,
			)
			if (tag === "[role=button]") {
				control.setAttribute("role", "button")
			}
			const child = document.createElement("span")
			control.append(child)
			element.append(control)

			pointer(child, "pointerdown")
			vi.advanceTimersByTime(500)

			expect(callback).not.toHaveBeenCalled()
		},
	)

	it("allows noneditable atom wrappers to open their block menu", ({
		expect,
	}) => {
		const { element, callback } = mount()
		const atom = document.createElement("div")
		atom.contentEditable = "false"
		atom.dataset.nodeViewWrapper = ""
		element.append(atom)

		const event = pointer(atom, "pointerdown")
		vi.advanceTimersByTime(500)

		expect(callback).toHaveBeenCalledExactlyOnceWith(event)
	})

	it("preserves a text selection that exists before the touch", ({
		expect,
	}) => {
		const { element, callback } = mount()
		selectText(element)

		pointer(element, "pointerdown")
		vi.advanceTimersByTime(500)

		expect.soft(callback).not.toHaveBeenCalled()
		expect.soft(document.getSelection()?.toString()).toBe("block text")
	})

	it("does not clear a selection made during the hold", ({ expect }) => {
		const { element, callback } = mount()
		const event = pointer(element, "pointerdown")
		selectText(element)

		vi.advanceTimersByTime(500)

		expect.soft(callback).toHaveBeenCalledExactlyOnceWith(event)
		expect.soft(document.getSelection()?.toString()).toBe("block text")
	})

	it("allows small finger movements without preventing their default", ({
		expect,
	}) => {
		const { element, callback } = mount()
		const down = pointer(element, "pointerdown")
		const move = pointer(document, "pointermove", { clientX: 6, clientY: 7 })

		vi.advanceTimersByTime(500)

		expect.soft(callback).toHaveBeenCalledExactlyOnceWith(down)
		expect.soft(move.defaultPrevented).toBe(false)
	})

	it("cancels when the finger moves ten pixels and does not restart", ({
		expect,
	}) => {
		const { element, callback } = mount()
		pointer(element, "pointerdown")
		pointer(document, "pointermove", { clientX: 6, clientY: 8 })
		pointer(document, "pointermove")

		vi.advanceTimersByTime(500)

		expect(callback).not.toHaveBeenCalled()
	})

	it.for(["pointerup", "pointercancel"])(
		"cancels on %s outside the pressed element",
		(type, { expect }) => {
			const { element, callback } = mount()
			pointer(element, "pointerdown")

			const event = pointer(document, type)
			vi.advanceTimersByTime(500)

			expect.soft(callback).not.toHaveBeenCalled()
			expect.soft(event.defaultPrevented).toBe(false)
		},
	)

	it.for(["pointermove", "pointerup", "pointercancel"])(
		"ignores another pointer's %s",
		(type, { expect }) => {
			const { element, callback } = mount()
			const down = pointer(element, "pointerdown")

			pointer(document, type, { pointerId: 2, clientX: 100 })
			vi.advanceTimersByTime(500)

			expect(callback).toHaveBeenCalledExactlyOnceWith(down)
		},
	)

	it("cancels when a second finger touches outside the element", ({
		expect,
	}) => {
		const { element, callback } = mount()
		pointer(element, "pointerdown")

		pointer(document.body, "pointerdown", { pointerId: 2, isPrimary: false })
		vi.advanceTimersByTime(500)

		expect(callback).not.toHaveBeenCalled()
	})

	it.for(["scroll", "blur"])(
		"cancels when the window receives %s",
		(type, { expect }) => {
			const { element, callback } = mount()
			pointer(element, "pointerdown")

			window.dispatchEvent(new Event(type))
			vi.advanceTimersByTime(500)

			expect(callback).not.toHaveBeenCalled()
		},
	)

	it("cancels when a nested scroll container scrolls", ({ expect }) => {
		const { element, callback } = mount()
		pointer(element, "pointerdown")

		element.dispatchEvent(new Event("scroll"))
		vi.advanceTimersByTime(500)

		expect(callback).not.toHaveBeenCalled()
	})

	it("keeps native context menus before the long press succeeds", ({
		expect,
	}) => {
		const { element, callback } = mount()
		pointer(element, "pointerdown")

		const context = mouse(element, "contextmenu")

		expect.soft(callback).not.toHaveBeenCalled()
		expect.soft(context.defaultPrevented).toBe(false)
	})

	it("suppresses native context menus only after a successful hold", ({
		expect,
	}) => {
		const { element, callback } = mount()
		const down = pointer(element, "pointerdown")
		vi.advanceTimersByTime(500)

		const context = mouse(element, "contextmenu")

		expect.soft(callback).toHaveBeenCalledExactlyOnceWith(down)
		expect.soft(context.defaultPrevented).toBe(true)
	})

	it("suppresses a native context menu retargeted to the menu portal", ({
		expect,
	}) => {
		const { element, callback } = mount()
		const down = pointer(element, "pointerdown")
		vi.advanceTimersByTime(500)

		const context = mouse(document.body, "contextmenu")

		expect.soft(callback).toHaveBeenCalledExactlyOnceWith(down)
		expect.soft(context.defaultPrevented).toBe(true)
	})

	it("keeps portal context suppression bounded after a successful hold is cancelled", ({
		expect,
	}) => {
		const { element, callback } = mount()
		const down = pointer(element, "pointerdown")
		vi.advanceTimersByTime(500)
		pointer(element, "pointercancel")

		const context = mouse(document.body, "contextmenu")
		vi.advanceTimersByTime(800)
		const laterContext = mouse(document.body, "contextmenu")

		expect.soft(callback).toHaveBeenCalledExactlyOnceWith(down)
		expect.soft(context.defaultPrevented).toBe(true)
		expect.soft(laterContext.defaultPrevented).toBe(false)
	})

	it("suppresses a late release click after native handling cancels the hold", ({
		expect,
	}) => {
		const { element, callback } = mount()
		const down = pointer(element, "pointerdown")
		vi.advanceTimersByTime(500)
		const cancelled = pointer(document, "pointercancel")

		const click = mouse(document.body, "click")

		expect.soft(callback).toHaveBeenCalledExactlyOnceWith(down)
		expect.soft(cancelled.defaultPrevented).toBe(false)
		expect.soft(click.defaultPrevented).toBe(true)
	})

	it.for(["pointerup", "pointercancel"])(
		"allows a portal context menu from the next deliberate press after %s",
		(type, { expect }) => {
			const { element, callback } = mount()
			const down = pointer(element, "pointerdown")
			vi.advanceTimersByTime(500)
			pointer(document, type)
			pointer(document.body, "pointerdown")

			const context = mouse(document.body, "contextmenu")

			expect.soft(callback).toHaveBeenCalledExactlyOnceWith(down)
			expect.soft(context.defaultPrevented).toBe(false)
		},
	)

	it("allows portal context menus after the release window expires", ({
		expect,
	}) => {
		const { element, callback } = mount()
		const down = pointer(element, "pointerdown")
		vi.advanceTimersByTime(500)
		pointer(document, "pointerup")
		vi.advanceTimersByTime(800)

		const context = mouse(document.body, "contextmenu")

		expect.soft(callback).toHaveBeenCalledExactlyOnceWith(down)
		expect.soft(context.defaultPrevented).toBe(false)
	})

	it("leaves click and context menu behavior intact when the callback declines", ({
		expect,
	}) => {
		const { element, callback } = mount(false)
		const down = pointer(element, "pointerdown")
		vi.advanceTimersByTime(500)
		pointer(element, "pointerup")

		const context = mouse(element, "contextmenu")
		const click = mouse(element, "click")

		expect.soft(callback).toHaveBeenCalledExactlyOnceWith(down)
		expect.soft(context.defaultPrevented).toBe(false)
		expect.soft(click.defaultPrevented).toBe(false)
	})

	it("suppresses the release click even when it targets a menu portal", ({
		expect,
	}) => {
		const { element, callback } = mount()
		const down = pointer(element, "pointerdown")
		vi.advanceTimersByTime(500)
		pointer(document, "pointerup")
		const onClick = vi.fn()
		document.body.addEventListener("click", onClick, { once: true })
		cleanups.push(() => {
			document.body.removeEventListener("click", onClick)
		})

		const click = mouse(document.body, "click")

		expect.soft(callback).toHaveBeenCalledExactlyOnceWith(down)
		expect.soft(click.defaultPrevented).toBe(true)
		expect.soft(onClick).not.toHaveBeenCalled()
	})

	it("lets the next deliberate tap choose a menu item", ({ expect }) => {
		const { element, callback } = mount()
		const down = pointer(element, "pointerdown")
		vi.advanceTimersByTime(500)
		pointer(document, "pointerup")
		pointer(document.body, "pointerdown")
		pointer(document.body, "pointerup")

		const click = mouse(document.body, "click")

		expect.soft(callback).toHaveBeenCalledExactlyOnceWith(down)
		expect.soft(click.defaultPrevented).toBe(false)
	})

	it("does not activate a menu item beneath the finger on release", ({
		expect,
	}) => {
		const { element, callback } = mount()
		const item = document.createElement("button")
		document.body.append(item)
		cleanups.push(() => {
			item.remove()
		})
		const onClick = vi.fn()
		item.addEventListener("click", onClick)
		item.addEventListener("pointerup", () => {
			item.click()
		})
		const down = pointer(element, "pointerdown")
		vi.advanceTimersByTime(500)

		const up = pointer(item, "pointerup")

		expect.soft(callback).toHaveBeenCalledExactlyOnceWith(down)
		expect.soft(up.defaultPrevented).toBe(true)
		expect.soft(onClick).not.toHaveBeenCalled()
	})

	it("keeps release suppression when opening the menu moves focus", ({
		expect,
	}) => {
		const { element, callback } = mount()
		const item = document.createElement("button")
		document.body.append(item)
		cleanups.push(() => {
			item.remove()
		})
		element.focus()
		callback.mockImplementation(() => {
			item.focus()
			return true
		})
		const down = pointer(element, "pointerdown")
		vi.advanceTimersByTime(500)

		const up = pointer(item, "pointerup")

		expect.soft(callback).toHaveBeenCalledExactlyOnceWith(down)
		expect.soft(document.activeElement).toBe(item)
		expect.soft(up.defaultPrevented).toBe(true)
	})

	it("keeps release suppression when the finger moves after opening", ({
		expect,
	}) => {
		const { element, callback } = mount()
		const down = pointer(element, "pointerdown")
		vi.advanceTimersByTime(500)
		pointer(document, "pointermove", { clientX: 20 })

		const up = pointer(document, "pointerup", { clientX: 20 })

		expect.soft(callback).toHaveBeenCalledExactlyOnceWith(down)
		expect.soft(up.defaultPrevented).toBe(true)
	})

	it("keeps release suppression when the page scrolls after opening", ({
		expect,
	}) => {
		const { element, callback } = mount()
		const down = pointer(element, "pointerdown")
		vi.advanceTimersByTime(500)
		window.dispatchEvent(new Event("scroll"))

		const up = pointer(document, "pointerup")

		expect.soft(callback).toHaveBeenCalledExactlyOnceWith(down)
		expect.soft(up.defaultPrevented).toBe(true)
	})

	it("does not suppress keyboard clicks", ({ expect }) => {
		const { element, callback } = mount()
		const down = pointer(element, "pointerdown")
		vi.advanceTimersByTime(500)
		pointer(document, "pointerup")

		const click = mouse(element, "click", 0)

		expect.soft(callback).toHaveBeenCalledExactlyOnceWith(down)
		expect.soft(click.defaultPrevented).toBe(false)
	})

	it("expires release suppression when the browser emits no click", ({
		expect,
	}) => {
		const { element, callback } = mount()
		const down = pointer(element, "pointerdown")
		vi.advanceTimersByTime(500)
		pointer(document, "pointerup")
		vi.advanceTimersByTime(800)

		const click = mouse(element, "click")

		expect.soft(callback).toHaveBeenCalledExactlyOnceWith(down)
		expect.soft(click.defaultPrevented).toBe(false)
	})

	it("cancels pending work and removes listeners when disposed", ({
		expect,
	}) => {
		const { element, callback, dispose } = mount()
		pointer(element, "pointerdown")

		dispose()
		vi.advanceTimersByTime(500)
		pointer(element, "pointerdown")
		vi.advanceTimersByTime(500)

		expect(callback).not.toHaveBeenCalled()
	})

	it("removes release suppression when disposed after a hold", ({ expect }) => {
		const { element, callback, dispose } = mount()
		const down = pointer(element, "pointerdown")
		vi.advanceTimersByTime(500)

		dispose()
		const context = mouse(element, "contextmenu")
		const click = mouse(element, "click")

		expect.soft(callback).toHaveBeenCalledExactlyOnceWith(down)
		expect.soft(context.defaultPrevented).toBe(false)
		expect.soft(click.defaultPrevented).toBe(false)
	})
})

function mount(result = true) {
	const element = document.createElement("div")
	element.contentEditable = "true"
	element.textContent = "block text"
	document.body.append(element)
	const callback = vi.fn(() => result)
	const dispose = bindTouchLongPress(element, callback)
	cleanups.push(dispose, () => {
		element.remove()
	})

	return { element, callback, dispose }
}

function touchEvent(type: string, init: PointerEventInit = {}) {
	return new PointerEvent(type, {
		bubbles: true,
		cancelable: true,
		pointerType: "touch",
		pointerId: 1,
		isPrimary: true,
		button: 0,
		...init,
	})
}

function pointer(target: EventTarget, type: string, init?: PointerEventInit) {
	const event = touchEvent(type, init)
	target.dispatchEvent(event)

	return event
}

function mouse(target: EventTarget, type: string, detail = 1) {
	const event = new MouseEvent(type, {
		bubbles: true,
		cancelable: true,
		detail,
	})
	target.dispatchEvent(event)

	return event
}

function selectText(element: HTMLElement) {
	const range = document.createRange()
	range.selectNodeContents(element)
	document.getSelection()?.addRange(range)
}
