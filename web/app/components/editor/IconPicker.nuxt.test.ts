import { mountSuspended } from "@nuxt/test-utils/runtime"
import { beforeEach, describe, it } from "vitest"
import IconPicker from "./IconPicker.vue"
import { renderedIconNames } from "~/components/test-helpers"

function mountPicker(props: Record<string, unknown> = {}) {
	return mountSuspended(IconPicker, { props: { icon: null, ...props } })
}

// the editable flag is a shared cookie state and the editor store is
// app-wide, so these tests cannot interleave
describe("<IconPicker>", { concurrent: false }, () => {
	beforeEach(() => {
		useEditorMeta().setEditable(true)
		useEditorStore().setReviewableDiffActive(false)
	})

	it("shows the icon it was given", async ({ expect }) => {
		const wrapper = await mountPicker({ icon: "mingcute:tag-fill" })

		expect(renderedIconNames(wrapper)).toEqual(["mingcute:tag-fill"])
	})

	it("shows nothing while there is no icon", async ({ expect }) => {
		const wrapper = await mountPicker()

		expect(renderedIconNames(wrapper)).toEqual([])
	})

	it("stays inert while there is no icon to change", async ({ expect }) => {
		const wrapper = await mountPicker()

		expect(wrapper.get("button").attributes("data-disabled")).toBe("")
	})

	it("can be pressed once it has an icon", async ({ expect }) => {
		const wrapper = await mountPicker({ icon: "mingcute:tag-fill" })

		expect(wrapper.get("button").attributes("data-disabled")).toBeUndefined()
	})

	it("stays inert in read mode", async ({ expect }) => {
		useEditorMeta().setEditable(false)

		const wrapper = await mountPicker({ icon: "mingcute:tag-fill" })

		expect(wrapper.get("button").attributes("data-disabled")).toBe("")
	})

	it("stays inert while a reviewable diff is shown", async ({ expect }) => {
		useEditorStore().setReviewableDiffActive(true)

		const wrapper = await mountPicker({ icon: "mingcute:tag-fill" })

		expect(wrapper.get("button").attributes("data-disabled")).toBe("")
	})

	it("renders at full size by default", async ({ expect }) => {
		const wrapper = await mountPicker({ icon: "mingcute:tag-fill" })

		expect(wrapper.get("button").attributes("data-small")).toBe("false")
	})

	it("renders small when the host asks for it", async ({ expect }) => {
		const wrapper = await mountPicker({
			icon: "mingcute:tag-fill",
			size: "icon-xsm",
		})

		expect(wrapper.get("button").attributes("data-small")).toBe("true")
	})

	it("highlights an icon a diff changed", async ({ expect }) => {
		const wrapper = await mountPicker({
			icon: "mingcute:tag-fill",
			isModified: true,
		})

		expect(wrapper.get("button").classes()).toContain("bg-diff-added")
	})

	it("leaves an unchanged icon unhighlighted", async ({ expect }) => {
		const wrapper = await mountPicker({ icon: "mingcute:tag-fill" })

		expect(wrapper.get("button").classes()).not.toContain("bg-diff-added")
	})

	it("reports nothing on its own when pressed", async ({ expect }) => {
		const wrapper = await mountPicker({ icon: "mingcute:tag-fill" })

		await wrapper.get("button").trigger("click")

		expect(wrapper.emitted("select")).toBeUndefined()
	})
})
