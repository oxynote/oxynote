import { mountSuspended } from "@nuxt/test-utils/runtime"
import { beforeEach, describe, it } from "vitest"
import ColorSelect from "./ColorSelect.vue"
import { at } from "~/components/test-helpers"
import { stubChartColorContext, stubThresholdPalette } from "../test-helpers"

let palette: string[] = []

function mountSelect(color?: string) {
	return mountSuspended(ColorSelect, { props: { modelValue: color } })
}

// the palette lives on the document element, shared by every mount, so
// these tests cannot interleave
describe("<ColorSelect>", { concurrent: false }, () => {
	beforeEach(() => {
		stubChartColorContext()
		palette = stubThresholdPalette()
	})

	it("offers every colour of the threshold palette", async ({ expect }) => {
		const wrapper = await mountSelect()

		const swatches = wrapper.findAll("button div")

		expect(swatches).toHaveLength(palette.length)
		expect(swatches.map((s) => s.attributes("style"))).toEqual(
			palette.map((color) => `background-color: ${color};`),
		)
	})

	it("picks the colour the reader clicked", async ({ expect }) => {
		const wrapper = await mountSelect(at(palette, 0))

		await at(wrapper.findAll("button"), 3).trigger("click")

		expect(wrapper.emitted("update:modelValue")).toEqual([[at(palette, 3)]])
	})
})
