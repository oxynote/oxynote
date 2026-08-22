import { mountSuspended } from "@nuxt/test-utils/runtime"
import { describe, it } from "vitest"
import CommandButton from "./CommandButton.vue"
import { renderedIconNames } from "~/components/test-helpers"

function mountButton(props: Record<string, unknown> = {}) {
	return mountSuspended(CommandButton, {
		props: {
			item: { title: "Heading 1" },
			itemIndex: 0,
			selectedIndex: null,
			...props,
		},
	})
}

describe("<CommandButton>", () => {
	it("names the command", async ({ expect }) => {
		const wrapper = await mountButton()

		expect(wrapper.text()).toBe("Heading 1")
	})

	it("shows the command's icon", async ({ expect }) => {
		const wrapper = await mountButton({
			item: { title: "Heading 1", icon: "lucide:heading-1" },
		})

		expect(renderedIconNames(wrapper)).toEqual(["lucide:heading-1"])
	})

	it("shows no icon for a command without one", async ({ expect }) => {
		const wrapper = await mountButton()

		expect(renderedIconNames(wrapper)).toEqual([])
	})

	it("shows the command's keyboard shortcut", async ({ expect }) => {
		const wrapper = await mountButton({
			item: { title: "Heading 1", shortcut: "#" },
		})

		expect(wrapper.text()).toContain("#")
	})

	it("marks itself as the selected command", async ({ expect }) => {
		const wrapper = await mountButton({ itemIndex: 2, selectedIndex: 2 })

		expect(wrapper.get("button").attributes("data-selected")).toBe("")
	})

	it("stays unmarked while another command is selected", async ({ expect }) => {
		const wrapper = await mountButton({ itemIndex: 2, selectedIndex: 1 })

		expect(wrapper.get("button").attributes("data-selected")).toBeUndefined()
	})

	it("marks a disabled command", async ({ expect }) => {
		const wrapper = await mountButton({
			item: { title: "Heading 1", disabled: true },
		})

		expect(wrapper.get("button").attributes("data-disabled")).toBe("")
	})

	it("reports a click", async ({ expect }) => {
		const wrapper = await mountButton()

		await wrapper.get("button").trigger("click")

		expect(wrapper.emitted("click")).toHaveLength(1)
	})

	it("reports the pointer moving over it", async ({ expect }) => {
		const wrapper = await mountButton()

		await wrapper.get("button").trigger("mouseover")

		expect(wrapper.emitted("hover")).toHaveLength(1)
	})
})
