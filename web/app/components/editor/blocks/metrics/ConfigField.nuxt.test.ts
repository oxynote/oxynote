import { mountSuspended } from "@nuxt/test-utils/runtime"
import { describe, it } from "vitest"
import ConfigField from "./ConfigField.vue"
import { t } from "~/components/test-helpers"

function mountField(slots: Record<string, () => unknown>) {
	return mountSuspended(ConfigField, { slots: slots })
}

describe("<ConfigField>", () => {
	it("shows the label above the field", async ({ expect }) => {
		const wrapper = await mountField({
			label: () => t("editor.metrics.config.data-source-label"),
			default: () => h("input"),
		})

		expect(wrapper.text()).toBe(t("editor.metrics.config.data-source-label"))
	})

	it("renders the field contents", async ({ expect }) => {
		const wrapper = await mountField({
			label: () => t("editor.metrics.config.data-source-label"),
			default: () => h("input", { id: "field" }),
		})

		expect(wrapper.find("input#field").exists()).toBe(true)
	})

	it("renders nothing but the frame when no slots are filled", async ({
		expect,
	}) => {
		const wrapper = await mountField({})

		expect(wrapper.text()).toBe("")
		expect(wrapper.findAll("div")).toHaveLength(3)
	})
})
