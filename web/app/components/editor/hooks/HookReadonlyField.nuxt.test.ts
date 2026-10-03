import { mountSuspended } from "@nuxt/test-utils/runtime"
import { describe, it } from "vitest"
import HookReadonlyField from "./HookReadonlyField.vue"
import { DiffStatus } from "../diff/position-map"

const ROWS = [
	{ value: "https://new.test", status: DiffStatus.Added },
	{ value: "https://old.test", status: DiffStatus.Removed },
	{ value: "https://same.test", status: DiffStatus.Unchanged },
]

describe("<HookReadonlyField>", () => {
	it("shows each value read only under the label", async ({ expect }) => {
		const wrapper = await mountSuspended(HookReadonlyField, {
			props: { label: "Link", rows: ROWS },
		})

		const fields = wrapper.findAll("[role='textbox']")
		expect(wrapper.text()).toContain("Link")
		expect(fields.map((field) => field.text())).toEqual([
			"https://new.test",
			"https://old.test",
			"https://same.test",
		])
		expect(fields.map((field) => field.attributes("aria-readonly"))).toEqual([
			"true",
			"true",
			"true",
		])
		expect(fields.map((field) => field.attributes("aria-label"))).toEqual([
			"Link",
			"Link",
			"Link",
		])
		expect(fields[0]?.classes()).toContain("bg-diff-field-added")
		expect(fields[1]?.classes()).toContain("bg-diff-field-removed")
		expect(fields[2]?.classes()).not.toContain("bg-diff-field-added")
		expect(fields[2]?.classes()).not.toContain("bg-diff-field-removed")
	})

	it("strikes through only the values the branch dropped", async ({
		expect,
	}) => {
		const wrapper = await mountSuspended(HookReadonlyField, {
			props: { label: "Link", rows: ROWS },
		})

		expect(
			wrapper
				.findAll("[role='textbox']")
				.map((field) => field.classes("line-through")),
		).toEqual([false, true, false])
	})

	it.for([
		{
			name: "green when the hook was added",
			input: DiffStatus.Added,
			expected: "bg-diff-field-added",
		},
		{
			name: "red when the hook was removed",
			input: DiffStatus.Removed,
			expected: "bg-diff-field-removed",
		},
	])("marks every value $name", async ({ input, expected }, { expect }) => {
		const wrapper = await mountSuspended(HookReadonlyField, {
			props: { label: "Link", rows: ROWS, diffStatus: input },
		})

		expect(
			wrapper
				.findAll("[role='textbox']")
				.map((field) => field.classes(expected)),
		).toEqual([true, true, true])
	})

	it("keeps the marks of each value in a modified hook", async ({ expect }) => {
		const wrapper = await mountSuspended(HookReadonlyField, {
			props: { label: "Link", rows: ROWS, diffStatus: DiffStatus.Modified },
		})

		const fields = wrapper.findAll("[role='textbox']")
		expect(fields[0]?.classes()).toContain("bg-diff-field-added")
		expect(fields[1]?.classes()).toContain("bg-diff-field-removed")
		expect(fields[2]?.classes()).not.toContain("bg-diff-field-added")
		expect(fields[2]?.classes()).not.toContain("bg-diff-field-removed")
	})
})
