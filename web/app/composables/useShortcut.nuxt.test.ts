import { mount } from "@vue/test-utils"
import { describe, it, vi } from "vitest"
import { defineComponent, h, nextTick } from "vue"
import useShortcut from "./useShortcut"

// the composable needs a component instance for provide/inject and scope
// disposal, so every test mounts a small host
function mountShortcut(
	shortcut?: { macOS: string; other: string },
	handler?: () => void,
) {
	const Host = defineComponent({
		setup() {
			useShortcut(shortcut, handler)

			return () => h("div")
		},
	})

	return mount(Host)
}

function pressKey(key: string, code?: string) {
	window.dispatchEvent(new KeyboardEvent("keydown", { key, code }))
}

function releaseKey(key: string, code?: string) {
	window.dispatchEvent(new KeyboardEvent("keyup", { key, code }))
}

// the tests drive shared window key state — the tests cannot interleave,
// and each releases the keys it pressed
describe("useShortcut", { concurrent: false }, () => {
	// with shift held the browser reports "|" as the key for the backslash;
	// the shortcut still matches because punctuation is bound by its code
	it("matches shifted punctuation by its key code", async ({ expect }) => {
		const handler = vi.fn()
		const wrapper = mountShortcut(
			{ macOS: "⌘+⇧+\\", other: "Ctrl+Shift+\\" },
			handler,
		)

		pressKey("Control")
		pressKey("Shift")
		pressKey("|", "Backslash")
		await nextTick()

		expect(handler).toHaveBeenCalledTimes(1)

		releaseKey("|", "Backslash")
		releaseKey("Shift")
		releaseKey("Control")
		wrapper.unmount()
	})

	// ⌘⇧\ holds every key of ⌘\, so without this the sidebar would toggle
	// on the inbox shortcut
	it("ignores the combo while a modifier outside it is held", async ({
		expect,
	}) => {
		const handler = vi.fn()
		const wrapper = mountShortcut({ macOS: "⌘+K", other: "Ctrl+K" }, handler)

		pressKey("Control")
		pressKey("Shift")
		pressKey("K", "KeyK")
		await nextTick()

		expect(handler).toHaveBeenCalledTimes(0)

		releaseKey("K", "KeyK")
		releaseKey("Shift")
		releaseKey("Control")
		wrapper.unmount()
	})

	it("fires the handler when the shortcut combo is pressed", async ({
		expect,
	}) => {
		const handler = vi.fn()
		// the os type defaults to "other" in tests, so the non-mac shortcut
		// applies
		const wrapper = mountShortcut({ macOS: "⌘+K", other: "Ctrl+K" }, handler)

		pressKey("Control")
		pressKey("k")
		// whenever() observes the combo ref through a pre-flush watcher
		await nextTick()

		expect(handler).toHaveBeenCalledTimes(1)

		releaseKey("k")
		releaseKey("Control")
		wrapper.unmount()
	})

	it("ignores a partial combo", async ({ expect }) => {
		const handler = vi.fn()
		const wrapper = mountShortcut({ macOS: "⌘+K", other: "Ctrl+K" }, handler)

		pressKey("k")
		await nextTick()

		expect(handler).toHaveBeenCalledTimes(0)

		releaseKey("k")
		wrapper.unmount()
	})

	it("stops firing after the scope is disposed", async ({ expect }) => {
		const handler = vi.fn()
		const wrapper = mountShortcut({ macOS: "⌘+K", other: "Ctrl+K" }, handler)
		wrapper.unmount()

		pressKey("Control")
		pressKey("k")
		await nextTick()

		expect(handler).toHaveBeenCalledTimes(0)

		releaseKey("k")
		releaseKey("Control")
	})

	it("provides the magic keys instance without a shortcut", ({ expect }) => {
		const wrapper = mountShortcut()

		expect(() => {
			pressKey("k")
			releaseKey("k")
		}).not.toThrow()

		wrapper.unmount()
	})
})
