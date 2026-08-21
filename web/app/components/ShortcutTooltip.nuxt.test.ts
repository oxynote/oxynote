import { beforeEach, describe, it, vi } from "vitest"
import ShortcutTooltip from "./ShortcutTooltip.vue"
import {
	clearTeleportedOverlays,
	mountUnderTooltipProvider,
	openTooltipText,
} from "./test-helpers"

const HOVER_DELAY_MS = 800

const SHORTCUT = {
	keyboardKey: { macOS: "⌘+K", other: "Ctrl+K" },
	i18nKey: "shortcuts.keys.search-for-documents",
}

function mountTooltip(props: Record<string, unknown>) {
	return mountUnderTooltipProvider(ShortcutTooltip, {
		props: props,
		slots: { default: () => h("button", "Trigger") },
	})
}

// the host os lives in a nuxt useState singleton shared by every mount in
// the file, and the hover delay needs the global fake timers, so these
// tests cannot interleave
describe("<ShortcutTooltip>", { concurrent: false }, () => {
	beforeEach(clearTeleportedOverlays)

	it("renders its slot untouched when no shortcut is given", async ({
		expect,
	}) => {
		const wrapper = await mountTooltip({})

		expect(wrapper.text()).toBe("Trigger")
	})

	it("renders its slot when a shortcut is given", async ({ expect }) => {
		const wrapper = await mountTooltip({ shortcut: SHORTCUT })

		expect(wrapper.text()).toContain("Trigger")
	})

	it("keeps the tooltip closed until the hover delay has passed", async ({
		expect,
	}) => {
		vi.useFakeTimers()
		const wrapper = await mountTooltip({ shortcut: SHORTCUT })

		await wrapper.get("button").trigger("pointerenter")
		await vi.advanceTimersByTimeAsync(HOVER_DELAY_MS - 1)

		expect(openTooltipText(wrapper)).not.toContain("Ctrl")
	})

	it("shows the shortcut once the hover delay has passed", async ({
		expect,
	}) => {
		vi.useFakeTimers()
		const wrapper = await mountTooltip({ shortcut: SHORTCUT })

		await wrapper.get("button").trigger("pointerenter")
		await vi.advanceTimersByTimeAsync(HOVER_DELAY_MS)

		expect(openTooltipText(wrapper)).toContain("Ctrl")
		expect(openTooltipText(wrapper)).toContain("K")
	})

	it("labels the shortcut with its translated action name", async ({
		expect,
	}) => {
		vi.useFakeTimers()
		const wrapper = await mountTooltip({ shortcut: SHORTCUT })

		await wrapper.get("button").trigger("pointerenter")
		await vi.advanceTimersByTimeAsync(HOVER_DELAY_MS)

		expect(openTooltipText(wrapper)).toContain("Search for pages")
	})

	it("shows only the keys for a shortcut with no i18n key", async ({
		expect,
	}) => {
		vi.useFakeTimers()
		const wrapper = await mountTooltip({
			shortcut: { keyboardKey: SHORTCUT.keyboardKey, i18nKey: null },
		})

		await wrapper.get("button").trigger("pointerenter")
		await vi.advanceTimersByTimeAsync(HOVER_DELAY_MS)

		expect(openTooltipText(wrapper)).toBe("CtrlthenK")
	})

	it("cancels a pending open when the pointer leaves early", async ({
		expect,
	}) => {
		vi.useFakeTimers()
		const wrapper = await mountTooltip({ shortcut: SHORTCUT })
		await wrapper.get("button").trigger("pointerenter")
		await vi.advanceTimersByTimeAsync(HOVER_DELAY_MS - 100)

		await wrapper.get("button").trigger("pointerleave")
		await vi.advanceTimersByTimeAsync(HOVER_DELAY_MS)

		expect(openTooltipText(wrapper)).not.toContain("Ctrl")
	})

	it("hides an open tooltip when the pointer leaves", async ({ expect }) => {
		vi.useFakeTimers()
		const wrapper = await mountTooltip({ shortcut: SHORTCUT })
		await wrapper.get("button").trigger("pointerenter")
		await vi.advanceTimersByTimeAsync(HOVER_DELAY_MS)

		await wrapper.get("button").trigger("pointerleave")
		await nextTick()

		expect(openTooltipText(wrapper)).not.toContain("Ctrl")
	})

	it("shows the macOS keys when the host is a mac", async ({ expect }) => {
		vi.useFakeTimers()
		useDetectHost().setOsType(HostOsType.MacOS)

		try {
			const wrapper = await mountTooltip({ shortcut: SHORTCUT })
			await wrapper.get("button").trigger("pointerenter")
			await vi.advanceTimersByTimeAsync(HOVER_DELAY_MS)

			expect(openTooltipText(wrapper)).toContain("⌘")
			expect(openTooltipText(wrapper)).not.toContain("Ctrl")
		} finally {
			useDetectHost().setOsType(HostOsType.Other)
		}
	})
})
