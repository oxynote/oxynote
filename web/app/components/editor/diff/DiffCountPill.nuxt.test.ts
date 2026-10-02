import { mountSuspended } from "@nuxt/test-utils/runtime"
import type { VueWrapper } from "@vue/test-utils"
import { describe, it } from "vitest"
import DiffCountPill from "./DiffCountPill.vue"
import { t } from "~/components/test-helpers"

describe("<DiffCountPill>", () => {
	it("shows both counts and names them for a screen reader", async ({
		expect,
	}) => {
		const wrapper = await mountSuspended(DiffCountPill, {
			props: { removed: 2, added: 3 },
		})

		expect(visibleCounts(wrapper)).toEqual([
			t("editor.diff-change-marker.removed", { count: 2 }),
			t("editor.diff-change-marker.added", { count: 3 }),
		])
		expect(wrapper.get(".sr-only").text()).toBe(
			t("editor.diff-change-marker.label", { removed: 2, added: 3 }),
		)
	})

	it.for([
		{ name: "removals", removed: 0, added: 1, key: "added" },
		{ name: "additions", removed: 1, added: 0, key: "removed" },
	])(
		"leaves out the side with no $name",
		async ({ removed, added, key }, { expect }) => {
			const wrapper = await mountSuspended(DiffCountPill, {
				props: { removed: removed, added: added },
			})

			expect(visibleCounts(wrapper)).toEqual([
				t(`editor.diff-change-marker.${key}`, { count: 1 }),
			])
		},
	)

	it("stacks the counts when vertical", async ({ expect }) => {
		const wrapper = await mountSuspended(DiffCountPill, {
			props: { removed: 1, added: 1, vertical: true },
		})

		expect(wrapper.classes()).toContain("flex-col")
	})

	it.for([
		{
			name: "pads the outer edges of a stack more than the shared one",
			lead: false,
			expected: [
				["pt-0.75", "pb-0.5"],
				["pt-0.5", "pb-0.75"],
			],
		},
		{
			name: "pads the top of a stack under a lead-in as a shared edge",
			lead: true,
			expected: [
				["pt-0.5", "pb-0.5"],
				["pt-0.5", "pb-0.75"],
			],
		},
	])("$name", async ({ lead, expected }, { expect }) => {
		const wrapper = await mountSuspended(DiffCountPill, {
			props: { removed: 1, added: 1, vertical: true },
			slots: lead ? { default: () => h("i") } : {},
		})

		expect(
			wrapper
				.findAll("[aria-hidden='true']")
				.map((cell) =>
					cell
						.classes()
						.filter((c) => c.startsWith("pt-") || c.startsWith("pb-")),
				),
		).toEqual(expected)
	})

	it("pads a lone stacked count as an outer edge on both sides", async ({
		expect,
	}) => {
		const wrapper = await mountSuspended(DiffCountPill, {
			props: { removed: 0, added: 2, vertical: true },
		})

		expect(wrapper.get("[aria-hidden='true']").classes()).toEqual(
			expect.arrayContaining(["pt-0.75", "pb-0.75"]),
		)
	})

	it("puts its lead-in before the counts", async ({ expect }) => {
		const wrapper = await mountSuspended(DiffCountPill, {
			props: { removed: 1, added: 1 },
			slots: { default: () => h("i", { class: "lead" }) },
		})

		expect(wrapper.get(".lead").element.previousElementSibling).toBeNull()
	})
})

function visibleCounts(wrapper: VueWrapper): string[] {
	return wrapper.findAll("[aria-hidden='true']").map((el) => el.text())
}
