import { mountSuspended } from "@nuxt/test-utils/runtime"
import type { VueWrapper } from "@vue/test-utils"
import { beforeEach, describe, it } from "vitest"
import OrderSelect from "./OrderSelect.vue"
import { LogOrder } from "../utils"
import { at, t } from "~/components/test-helpers"

const NEWEST_FIRST = 0
const OLDEST_FIRST = 1

// the editable flag is a shared cookie state and the editor store is
// app-wide, so these tests cannot interleave
describe("<OrderSelect>", { concurrent: false }, () => {
	beforeEach(() => {
		useEditorMeta().setEditable(true)
		useEditorStore().setReviewableDiffActive(false)
	})

	it("offers both orders for editing", async ({ expect }) => {
		const wrapper = await mountOrderSelect({
			modelValue: LogOrder.NewestFirst,
		})

		expect(buttons(wrapper).map((b) => b.text())).toEqual([
			t("editor.logs.config.order-options.newest_first"),
			t("editor.logs.config.order-options.oldest_first"),
		])
		expect(buttons(wrapper).map((b) => b.attributes("aria-disabled"))).toEqual([
			"false",
			"false",
		])
	})

	it.for([
		{ input: LogOrder.NewestFirst, expected: ["true", "false"] },
		{ input: LogOrder.OldestFirst, expected: ["false", "true"] },
	])(
		"presses the button of the $input order",
		async ({ input, expected }, { expect }) => {
			const wrapper = await mountOrderSelect({ modelValue: input })

			expect(buttons(wrapper).map((b) => b.attributes("aria-pressed"))).toEqual(
				expected,
			)
			expect(
				buttons(wrapper).map((b) => b.classes().includes("border-primary")),
			).toEqual(expected.map((pressed) => pressed === "true"))
		},
	)

	it.for([
		{
			index: OLDEST_FIRST,
			input: LogOrder.NewestFirst,
			expected: LogOrder.OldestFirst,
		},
		{
			index: NEWEST_FIRST,
			input: LogOrder.OldestFirst,
			expected: LogOrder.NewestFirst,
		},
	])(
		"stores the $expected order when picked",
		async ({ index, input, expected }, { expect }) => {
			const wrapper = await mountOrderSelect({ modelValue: input })

			await at(buttons(wrapper), index).trigger("click")

			expect(wrapper.emitted("update:modelValue")).toEqual([[expected]])
		},
	)

	it("disables both orders in read mode", async ({ expect }) => {
		useEditorMeta().setEditable(false)

		const wrapper = await mountOrderSelect({
			modelValue: LogOrder.NewestFirst,
		})

		expect(buttons(wrapper).map((b) => b.attributes("aria-disabled"))).toEqual([
			"true",
			"true",
		])
		expect(
			buttons(wrapper).every((b) =>
				b.classes().includes("pointer-events-none"),
			),
		).toBe(true)
	})

	it("changes nothing in read mode", async ({ expect }) => {
		useEditorMeta().setEditable(false)
		const wrapper = await mountOrderSelect({
			modelValue: LogOrder.NewestFirst,
		})

		await at(buttons(wrapper), OLDEST_FIRST).trigger("click")

		expect(wrapper.emitted("update:modelValue")).toBeUndefined()
	})

	it("changes nothing while a reviewable diff is shown", async ({ expect }) => {
		useEditorStore().setReviewableDiffActive(true)
		const wrapper = await mountOrderSelect({
			modelValue: LogOrder.NewestFirst,
		})

		await at(buttons(wrapper), OLDEST_FIRST).trigger("click")

		expect(wrapper.emitted("update:modelValue")).toBeUndefined()
	})

	it("marks the order a diff chose and the one it replaced", async ({
		expect,
	}) => {
		const wrapper = await mountOrderSelect({
			modelValue: LogOrder.OldestFirst,
			isModified: true,
			oldOrder: LogOrder.NewestFirst,
		})

		const oldest = at(buttons(wrapper), OLDEST_FIRST)
		const newest = at(buttons(wrapper), NEWEST_FIRST)

		expect(oldest.classes()).toContain("bg-diff-field-added")
		expect(oldest.classes()).not.toContain("border-primary")
		expect(newest.classes()).toContain("bg-diff-field-removed")
		expect(newest.classes()).not.toContain("bg-diff-field-added")
	})

	it("leaves both orders untinted outside a modification", async ({
		expect,
	}) => {
		const wrapper = await mountOrderSelect({
			modelValue: LogOrder.OldestFirst,
			oldOrder: LogOrder.NewestFirst,
		})

		expect(
			buttons(wrapper).flatMap((b) =>
				b.classes().filter((c) => c.startsWith("bg-diff-field-")),
			),
		).toEqual([])
		expect(at(buttons(wrapper), OLDEST_FIRST).classes()).toContain(
			"border-primary",
		)
	})
})

function mountOrderSelect(props: {
	modelValue: LogOrder
	isModified?: boolean
	oldOrder?: LogOrder | null
}) {
	return mountSuspended(OrderSelect, { props: props })
}

function buttons(wrapper: VueWrapper) {
	return wrapper.findAll("button")
}
