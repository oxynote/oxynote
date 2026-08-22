import { mountSuspended } from "@nuxt/test-utils/runtime"
import { describe, it } from "vitest"
import HookInputField from "./HookInputField.vue"
import { t } from "~/components/test-helpers"

function mountField(props: Record<string, unknown> = {}) {
	return mountSuspended(HookInputField, {
		props: {
			placeholder: t(
				"editor.hooks.container-image-watcher.image-input-placeholder",
			),
			...props,
		},
		slots: {
			label: () => t("editor.hooks.container-image-watcher.image-input-label"),
		},
	})
}

describe("<ContainerImageWatcherHookInputField>", () => {
	it("labels the field", async ({ expect }) => {
		const wrapper = await mountField()

		expect(wrapper.text()).toBe(
			t("editor.hooks.container-image-watcher.image-input-label"),
		)
	})

	it("prompts with the placeholder it was given", async ({ expect }) => {
		const wrapper = await mountField()

		expect(wrapper.get("input").attributes("placeholder")).toBe(
			t("editor.hooks.container-image-watcher.image-input-placeholder"),
		)
	})

	it("shows the value it was given", async ({ expect }) => {
		const wrapper = await mountField({ modelValue: "redis:7" })

		expect((wrapper.get("input").element as HTMLInputElement).value).toBe(
			"redis:7",
		)
	})

	it("reports what the reader typed", async ({ expect }) => {
		const wrapper = await mountField()

		await wrapper.get("input").setValue("redis:7")

		expect(wrapper.emitted("update:modelValue")).toEqual([["redis:7"]])
	})

	it("stays editable by default", async ({ expect }) => {
		const wrapper = await mountField()

		expect(wrapper.get("input").attributes("disabled")).toBeUndefined()
	})

	it("locks the field when the host asks it to", async ({ expect }) => {
		const wrapper = await mountField({ disabled: true })

		expect(wrapper.get("input").attributes("disabled")).toBeDefined()
	})

	it("sizes itself for a menu by default", async ({ expect }) => {
		const wrapper = await mountField()

		expect(wrapper.get("input").classes()).toContain("text-2sm")
	})

	it("takes the sizing the host asks for", async ({ expect }) => {
		const wrapper = await mountField({ inputClass: "h-10 text-base" })

		expect(wrapper.get("input").classes()).toContain("h-10")
		expect(wrapper.get("input").classes()).not.toContain("text-2sm")
	})

	it("takes any text as an image reference", async ({ expect }) => {
		const wrapper = await mountField()

		expect(wrapper.get("input").attributes("type")).toBeUndefined()
	})
})
