import { mountSuspended } from "@nuxt/test-utils/runtime"
import { describe, it } from "vitest"
import ColorSelect from "./ColorSelect.vue"
import { at } from "~/components/test-helpers"
import { PALETTE_COLORS } from "~/utils"

function mountSelect(color?: string) {
	return mountSuspended(ColorSelect, { props: { modelValue: color } })
}

describe("<ColorSelect>", () => {
	it("paints every palette colour with its theme variable", async ({
		expect,
	}) => {
		const wrapper = await mountSelect()

		const swatches = wrapper.findAll("button div")

		expect(swatches).toHaveLength(PALETTE_COLORS.length)
		expect(swatches.map((s) => s.attributes("style"))).toEqual(
			PALETTE_COLORS.map(
				(_name, index) =>
					`background-color: var(--selectable-color-${index + 1});`,
			),
		)
	})

	it("picks the name of the colour the reader clicked", async ({ expect }) => {
		const wrapper = await mountSelect("red")

		await at(wrapper.findAll("button"), 3).trigger("click")

		expect(wrapper.emitted("update:modelValue")).toEqual([["yellow"]])
	})
})
