import { mountSuspended } from "@nuxt/test-utils/runtime"
import { describe, it } from "vitest"
import FilterChips from "./FilterChips.vue"
import type { LogFilter } from "../logql"
import { findButtonByText, t } from "~/components/test-helpers"

describe("<FilterChips>", () => {
	it("shows each filter's key, comparison and value with a remove button", async ({
		expect,
	}) => {
		const wrapper = await mountChips(makeFilters())

		expect(
			wrapper
				.findAll(".rounded-md.border")
				.map((chip) =>
					chip.findAll(".shrink-0, .truncate").map((p) => p.text()),
				),
		).toEqual([
			["env", "=", "prod", t("editor.logs.view.remove-filter", { key: "env" })],
			[
				"status",
				"≠",
				"500",
				t("editor.logs.view.remove-filter", { key: "status" }),
			],
		])
	})

	it("removes one filter with its remove button", async ({ expect }) => {
		const filters = makeFilters()
		const wrapper = await mountChips(filters)

		await findButtonByText(
			wrapper,
			t("editor.logs.view.remove-filter", { key: "env" }),
		).trigger("click")

		expect(wrapper.emitted("update:modelValue")).toEqual([[[filters[1]]]])
	})

	it("removes every filter with the clear button", async ({ expect }) => {
		const wrapper = await mountChips(makeFilters())

		await findButtonByText(
			wrapper,
			t("editor.logs.view.clear-filters"),
		).trigger("click")

		expect(wrapper.emitted("update:modelValue")).toEqual([[[]]])
	})
})

function makeFilters(): LogFilter[] {
	return [
		{ key: "env", value: "prod", indexed: true, exclude: false },
		{ key: "status", value: "500", indexed: false, exclude: true },
	]
}

function mountChips(filters: LogFilter[]) {
	return mountSuspended(FilterChips, { props: { modelValue: filters } })
}
