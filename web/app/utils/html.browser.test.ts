import { afterEach, describe, it } from "vitest"
import { waitForHtmlElementById } from "./html"

// the page DOM is shared between tests, so they cannot interleave
describe("waitForHtmlElementById", { concurrent: false }, () => {
	afterEach(() => {
		document.body.innerHTML = ""
	})

	it("resolves an element that is already present", async ({ expect }) => {
		const el = document.createElement("div")
		el.id = "present"
		document.body.appendChild(el)

		await expect(waitForHtmlElementById("present", 1000)).resolves.toBe(el)
	})

	it("resolves an element that appears before the timeout", async ({
		expect,
	}) => {
		const pending = waitForHtmlElementById("late", 1000)

		// let the first check miss, then attach the element for the next
		// animation frame of the polling loop
		await new Promise(requestAnimationFrame)
		const el = document.createElement("div")
		el.id = "late"
		document.body.appendChild(el)

		await expect(pending).resolves.toBe(el)
	})

	it("resolves null once the timeout elapses without the element", async ({
		expect,
	}) => {
		// awaiting the unit's own promise is the concrete signal; the
		// timeout budget is what bounds the polling loop
		await expect(waitForHtmlElementById("never", 50)).resolves.toBeNull()
	})
})
