import { mount } from "@vue/test-utils"
import { describe, it, vi } from "vitest"
import { defineComponent, h } from "vue"
import useIconPicker from "./useIconPicker"

// the composable needs a component instance for provide/inject, so every
// test mounts a small host and drives the captured context
function mountPicker() {
	let ctx!: ReturnType<typeof useIconPicker>

	const Host = defineComponent({
		setup() {
			ctx = useIconPicker()

			return () => h("div")
		},
	})

	mount(Host)

	return ctx
}

describe("useIconPicker", () => {
	it("opens the picker anchored to an element", ({ expect }) => {
		const ctx = mountPicker()
		const anchorEl = document.createElement("div")

		ctx.openIconPicker(anchorEl, vi.fn())

		expect(ctx.open.value).toBe(true)
		expect(ctx.anchor.value).toBe(anchorEl)
	})

	it("closes the picker", ({ expect }) => {
		const ctx = mountPicker()
		ctx.openIconPicker(document.createElement("div"), vi.fn())

		ctx.closeIconPicker()

		expect(ctx.open.value).toBe(false)
	})

	it("reports the selected icon and closes", ({ expect }) => {
		const ctx = mountPicker()
		const onSelect = vi.fn()
		ctx.openIconPicker(document.createElement("div"), onSelect)

		ctx.selectIcon("mdi:home")

		expect(onSelect).toHaveBeenCalledExactlyOnceWith("mdi:home")
		expect(ctx.open.value).toBe(false)
	})

	it("ignores a selection before the picker was ever opened", ({ expect }) => {
		const ctx = mountPicker()

		ctx.selectIcon("mdi:home")

		expect(ctx.open.value).toBe(false)
	})

	it("shares one context with nested consumers", ({ expect }) => {
		let parentCtx!: ReturnType<typeof useIconPicker>
		let childCtx!: ReturnType<typeof useIconPicker>

		const Child = defineComponent({
			setup() {
				childCtx = useIconPicker()

				return () => h("span")
			},
		})
		const Parent = defineComponent({
			setup() {
				parentCtx = useIconPicker()

				return () => h(Child)
			},
		})

		mount(Parent)

		expect(childCtx).toBe(parentCtx)
	})
})
