import { describe, it as base, vi } from "vitest"
import { useTooltipPin } from "./tooltip-pin"

const UNPINNED = {
	triggerOn: "mousemove",
	enterable: false,
	alwaysShowContent: false,
}
const PINNED = { triggerOn: "none", enterable: true, alwaysShowContent: true }
// where the test clicks the chart, and the actions that show the tooltip
// again there and hide it
const CLICK = { offsetX: 120, offsetY: 40 }
const SHOW_TIP = { type: "showTip", x: 120, y: 40 }
const HIDE_TIP = { type: "hideTip" }

const it = base.extend<{ pin: ReturnType<typeof setupPin> }>({
	// eslint-disable-next-line no-empty-pattern -- vitest reads the fixtures a test needs from this destructuring
	pin: async ({}, use) => {
		const pin = setupPin()
		await use(pin)
		pin.scope.stop()
		pin.container.remove()
	},
})

// the pin listens on the document, so a press or a key in one test would
// reach the pin of another running at the same time
describe("useTooltipPin", { concurrent: false }, () => {
	it("keeps the tooltip inside the chart", ({ expect, pin }) => {
		const option = pin.result.tooltipOption.value

		expect(option.confine).toBe(true)
		expect(option.appendTo).toBeUndefined()
		// a tooltip wider than a narrow chart would still run off its edge
		expect(option.className?.split(" ")).toContain("max-w-full")
	})

	it("follows the cursor until it is pinned", ({ expect, pin }) => {
		expect(pinState(pin)).toEqual(UNPINNED)
		expect(pin.result.pinned.value).toBe(false)
	})

	it("stays at the clicked instant so its list can be scrolled", async ({
		expect,
		pin,
	}) => {
		await pin.result.togglePin(CLICK)

		expect(pinState(pin)).toEqual(PINNED)
		expect.soft(pin.result.pinned.value).toBe(true)
		expect
			.soft(pin.chart.containPixel)
			.toHaveBeenCalledExactlyOnceWith("grid", [120, 40])
		expect.soft(pin.chart.dispatchAction.mock.calls).toEqual([[SHOW_TIP]])
	})

	it("ignores a click outside the plot, such as on the legend", async ({
		expect,
		pin,
	}) => {
		pin.chart.containPixel.mockReturnValue(false)

		await pin.result.togglePin(CLICK)

		expect(pinState(pin)).toEqual(UNPINNED)
		expect
			.soft(pin.chart.containPixel)
			.toHaveBeenCalledExactlyOnceWith("grid", [120, 40])
		expect.soft(pin.chart.dispatchAction).not.toHaveBeenCalled()
	})

	it("follows the cursor again after a second click", async ({
		expect,
		pin,
	}) => {
		await pin.result.togglePin(CLICK)

		await pin.result.togglePin(CLICK)

		expect(pinState(pin)).toEqual(UNPINNED)
		expect
			.soft(pin.chart.dispatchAction.mock.calls)
			.toEqual([[SHOW_TIP], [HIDE_TIP]])
	})

	it("lets go when Escape is pressed", async ({ expect, pin }) => {
		await pin.result.togglePin(CLICK)

		document.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape" }))
		await nextTick()

		expect(pinState(pin)).toEqual(UNPINNED)
		expect
			.soft(pin.chart.dispatchAction.mock.calls)
			.toEqual([[SHOW_TIP], [HIDE_TIP]])
	})

	it("ignores other keys", async ({ expect, pin }) => {
		await pin.result.togglePin(CLICK)

		document.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter" }))
		await nextTick()

		expect(pinState(pin)).toEqual(PINNED)
		expect.soft(pin.chart.dispatchAction.mock.calls).toEqual([[SHOW_TIP]])
	})

	it("lets go on a press outside the chart", async ({ expect, pin }) => {
		await pin.result.togglePin(CLICK)

		document.body.dispatchEvent(
			new PointerEvent("pointerdown", { bubbles: true }),
		)
		await nextTick()

		expect(pinState(pin)).toEqual(UNPINNED)
		expect
			.soft(pin.chart.dispatchAction.mock.calls)
			.toEqual([[SHOW_TIP], [HIDE_TIP]])
	})

	it("stays pinned on a press inside the chart", async ({ expect, pin }) => {
		await pin.result.togglePin(CLICK)

		pin.inner.dispatchEvent(new PointerEvent("pointerdown", { bubbles: true }))
		await nextTick()

		expect(pinState(pin)).toEqual(PINNED)
		expect.soft(pin.chart.dispatchAction.mock.calls).toEqual([[SHOW_TIP]])
	})

	it("hides nothing on a press outside while unpinned", async ({
		expect,
		pin,
	}) => {
		document.body.dispatchEvent(
			new PointerEvent("pointerdown", { bubbles: true }),
		)
		await nextTick()

		expect(pinState(pin)).toEqual(UNPINNED)
		expect.soft(pin.chart.dispatchAction).not.toHaveBeenCalled()
	})
})

// a chart stand-in inside a container attached to the page, with the pin
// set up in its own scope so the fixture can take its listeners down
function setupPin() {
	const chart = {
		dispatchAction: vi.fn(),
		containPixel: vi.fn().mockReturnValue(true),
	}
	const container = document.createElement("div")
	const inner = document.createElement("div")
	container.append(inner)
	document.body.append(container)
	const scope = effectScope()
	const result = scope.run(() =>
		useTooltipPin(shallowRef(chart), shallowRef(container)),
	)
	if (!result) {
		throw new Error("the pin scope returned nothing")
	}

	return { chart, container, inner, scope, result }
}

function pinState(pin: ReturnType<typeof setupPin>) {
	const option = pin.result.tooltipOption.value

	return {
		triggerOn: option.triggerOn,
		enterable: option.enterable,
		alwaysShowContent: option.alwaysShowContent,
	}
}
