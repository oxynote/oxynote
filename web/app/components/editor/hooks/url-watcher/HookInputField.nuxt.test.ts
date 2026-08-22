import { mountSuspended } from "@nuxt/test-utils/runtime"
import { describe, it } from "vitest"
import HookInputField from "./HookInputField.vue"
import { t } from "~/components/test-helpers"

function mountField(props: Record<string, unknown> = {}) {
	return mountSuspended(HookInputField, {
		props: {
			placeholder: t("editor.hooks.url-watcher.url-input-placeholder"),
			...props,
		},
		slots: { label: () => t("editor.hooks.url-watcher.url-input-label") },
	})
}

describe("<URLWatcherHookInputField>", () => {
	it("labels the field", async ({ expect }) => {
		const wrapper = await mountField()

		expect(wrapper.text()).toBe(t("editor.hooks.url-watcher.url-input-label"))
	})

	it("prompts with the placeholder it was given", async ({ expect }) => {
		const wrapper = await mountField()

		expect(wrapper.get("input").attributes("placeholder")).toBe(
			t("editor.hooks.url-watcher.url-input-placeholder"),
		)
	})

	it("shows the value it was given", async ({ expect }) => {
		const wrapper = await mountField({ modelValue: "https://oxynote.test" })

		expect((wrapper.get("input").element as HTMLInputElement).value).toBe(
			"https://oxynote.test",
		)
	})

	it("reports what the reader typed", async ({ expect }) => {
		const wrapper = await mountField()

		await wrapper.get("input").setValue("https://oxynote.test")

		expect(wrapper.emitted("update:modelValue")).toEqual([
			["https://oxynote.test"],
		])
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

	it("asks the browser to validate the url", async ({ expect }) => {
		const wrapper = await mountField()

		expect(wrapper.get("input").attributes("type")).toBe("url")
	})
})
