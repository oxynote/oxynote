import { mountSuspended } from "@nuxt/test-utils/runtime"
import { describe, it } from "vitest"
import HookReadonlyField from "./HookReadonlyField.vue"
import { DiffStatus } from "../diff/position-map"

describe("<HookReadonlyField>", () => {
	it("shows each value read only under the label", async ({ expect }) => {
		const wrapper = await mountSuspended(HookReadonlyField, {
			props: {
				label: "Link",
				rows: [
					{ value: "https://new.test", status: DiffStatus.Added },
					{ value: "https://old.test", status: DiffStatus.Removed },
					{ value: "https://same.test", status: DiffStatus.Unchanged },
				],
			},
		})

		const inputs = wrapper.findAll("input")
		expect(wrapper.text()).toContain("Link")
		expect(inputs.map((input) => input.element.value)).toEqual([
			"https://new.test",
			"https://old.test",
			"https://same.test",
		])
		expect(inputs.map((input) => input.attributes("readonly"))).toEqual([
			"",
			"",
			"",
		])
		expect(inputs.map((input) => input.attributes("aria-label"))).toEqual([
			"Link",
			"Link",
			"Link",
		])
		expect(inputs[0]?.classes()).toContain("bg-diff-field-added")
		expect(inputs[1]?.classes()).toContain("bg-diff-field-removed")
		expect(inputs[2]?.classes()).not.toContain("bg-diff-field-added")
		expect(inputs[2]?.classes()).not.toContain("bg-diff-field-removed")
	})
})
