import { mountSuspended } from "@nuxt/test-utils/runtime"
import { describe, it } from "vitest"
import ToastMessage from "./ToastMessage.vue"
import { renderedIconNames } from "./test-helpers"

describe("<ToastMessage>", () => {
	it("shows the title", async ({ expect }) => {
		const wrapper = await mountSuspended(ToastMessage, {
			props: { type: "success", title: "Workspace saved" },
		})

		expect(wrapper.text()).toContain("Workspace saved")
	})

	it("shows the description alongside the title", async ({ expect }) => {
		const wrapper = await mountSuspended(ToastMessage, {
			props: {
				type: "success",
				title: "Workspace saved",
				description: "Everyone sees the new name now",
			},
		})

		expect(wrapper.text()).toContain("Everyone sees the new name now")
	})

	it("renders only the title when no description is given", async ({
		expect,
	}) => {
		const wrapper = await mountSuspended(ToastMessage, {
			props: { type: "success", title: "Workspace saved" },
		})

		expect(wrapper.findAll(".text-muted-foreground")).toHaveLength(0)
	})

	it.for([
		{ type: "success", expected: "mingcute:check-circle-fill" },
		{ type: "error", expected: "mingcute:close-circle-fill" },
		{ type: "info", expected: "mingcute:information-fill" },
		{ type: "warning", expected: "mingcute:warning-fill" },
	] as const)(
		"marks a $type message with its own icon",
		async ({ type, expected }, { expect }) => {
			const wrapper = await mountSuspended(ToastMessage, {
				props: { type: type, title: "Title" },
			})

			expect(renderedIconNames(wrapper)[0]).toBe(expected)
		},
	)

	it.for([
		{ type: "success", expected: "text-status-success" },
		{ type: "error", expected: "text-status-error" },
		{ type: "info", expected: "text-status-info" },
		{ type: "warning", expected: "text-status-warning" },
	] as const)(
		"colours a $type message with its own status colour",
		async ({ type, expected }, { expect }) => {
			const wrapper = await mountSuspended(ToastMessage, {
				props: { type: type, title: "Title" },
			})

			expect(wrapper.get(".iconify").classes()).toContain(expected)
		},
	)

	it("emits close when the close button is pressed", async ({ expect }) => {
		const wrapper = await mountSuspended(ToastMessage, {
			props: { type: "info", title: "Heads up" },
		})

		await wrapper.get("button").trigger("click")

		expect(wrapper.emitted("close")).toHaveLength(1)
	})

	it("does not emit close before the button is pressed", async ({ expect }) => {
		const wrapper = await mountSuspended(ToastMessage, {
			props: { type: "info", title: "Heads up" },
		})

		expect(wrapper.emitted("close")).toBeUndefined()
	})
})
