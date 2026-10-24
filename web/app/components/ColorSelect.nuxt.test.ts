import { mountSuspended } from "@nuxt/test-utils/runtime"
import { beforeEach, describe, it } from "vitest"
import ColorSelect from "./ColorSelect.vue"
import { at } from "~/components/test-helpers"
import { paletteColor } from "~/utils"
import {
	stubThemeColorContext,
	stubSelectableColors,
} from "./editor/test-helpers/theme"

let palette: string[] = []

function mountSelect(
	color?: string,
	props: { colors?: string[]; paint?: (color: string) => string } = {},
) {
	return mountSuspended(ColorSelect, {
		props: { modelValue: color, ...props },
	})
}

// the palette lives on the document element, shared by every mount, so
// these tests cannot interleave
describe("<ColorSelect>", { concurrent: false }, () => {
	beforeEach(() => {
		stubThemeColorContext()
		palette = stubSelectableColors()
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

	it("offers the given colours painted the way it is told", async ({
		expect,
	}) => {
		const wrapper = await mountSelect(undefined, {
			colors: ["red", "pink"],
			paint: paletteColor,
		})

		expect(
			wrapper.findAll("button div").map((s) => s.attributes("style")),
		).toEqual([
			"background-color: var(--selectable-color-1);",
			"background-color: var(--selectable-color-16);",
		])
	})

	it("picks the given colour the reader clicked", async ({ expect }) => {
		const wrapper = await mountSelect("red", {
			colors: ["red", "pink"],
			paint: paletteColor,
		})

		await at(wrapper.findAll("button"), 1).trigger("click")

		expect(wrapper.emitted("update:modelValue")).toEqual([["pink"]])
	})
})
