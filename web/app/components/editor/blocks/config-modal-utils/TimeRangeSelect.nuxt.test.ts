import { mountSuspended } from "@nuxt/test-utils/runtime"
import {
	enableAutoUnmount,
	flushPromises,
	type VueWrapper,
} from "@vue/test-utils"
import { afterEach, beforeEach, describe, it } from "vitest"
import TimeRangeSelect from "./TimeRangeSelect.vue"
import { TimeRangePreset } from "~/utils/time-range"
import {
	METRIC_TIME_RANGE_REFERENCE,
	type TimeRangeSetting,
} from "~/utils/variable"
import { at, clearTeleportedOverlays, t } from "~/components/test-helpers"

// the open list is teleported into <body>, which every mount shares
describe("<TimeRangeSelect>", { concurrent: false }, () => {
	enableAutoUnmount(afterEach)

	beforeEach(() => {
		clearTeleportedOverlays()
	})

	it.for<{
		name: string
		input: TimeRangeSetting | null
		expected: () => string
	}>([
		{
			name: "labels a preset with its name",
			input: TimeRangePreset.Last24Hours,
			expected: () => t("general.time-range-options.last_24_hours"),
		},
		{
			name: "labels the built-in reference as the shared setting",
			input: METRIC_TIME_RANGE_REFERENCE,
			expected: () => t("editor.config-modal-utils.variable-time-range"),
		},
		{
			name: "leaves the label empty without a time range",
			input: null,
			expected: () => "",
		},
	])("$name", async ({ input, expected }, { expect }) => {
		const wrapper = await mountSelect({ modelValue: input })

		expect(buttons(wrapper)).toHaveLength(1)
		expect(at(buttons(wrapper), 0).text()).toBe(expected())
	})

	it("lists custom time first, then the built-in reference and presets", async ({
		expect,
	}) => {
		const wrapper = await mountSelect({
			modelValue: TimeRangePreset.Last24Hours,
		})

		await openList(wrapper)

		expect(optionLabels()).toEqual([
			t("general.custom-time-range.title"),
			t("editor.config-modal-utils.variable-time-range"),
			...Object.values(TimeRangePreset).map((preset) =>
				t(`general.time-range-options.${preset}`),
			),
		])
	})

	it.for([
		{
			name: "same day",
			from: new Date(2026, 10, 11, 5, 38),
			to: new Date(2026, 10, 11, 5, 42),
			expected: "Nov 11 5:38 – 5:42",
		},
		{
			name: "different days",
			from: new Date(2026, 10, 11, 5, 38),
			to: new Date(2026, 10, 23, 4, 2),
			expected: "Nov 11 5:38 – Nov 23 4:02",
		},
		{
			name: "different years",
			from: new Date(2026, 11, 31, 23, 58),
			to: new Date(2027, 0, 1, 0, 2),
			expected: "Dec 31, 2026 23:58 – Jan 1, 2027 0:02",
		},
	])(
		"uses a compact label for $name",
		async ({ from, to, expected }, { expect }) => {
			const wrapper = await mountSelect({
				modelValue: `custom:${from.toISOString()}/${to.toISOString()}`,
			})
			expect(at(buttons(wrapper), 0).text()).toBe(expected)
		},
	)

	it("opens a custom window without changing the stored range", async ({
		expect,
	}) => {
		const wrapper = await mountSelect({ modelValue: TimeRangePreset.Last1Hour })
		await openList(wrapper)
		pickOption(t("general.custom-time-range.title"))
		await flushPromises()

		expect(
			document.body.querySelector("[role='dialog']")?.textContent,
		).toContain(t("general.custom-time-range.title"))
		expect(wrapper.emitted("update:modelValue")).toBeUndefined()
	})

	it("reopens a saved custom window for editing", async ({ expect }) => {
		const wrapper = await mountSelect({
			modelValue: "custom:2026-03-28T09:30:15.123Z/2026-03-29T18:45:30.456Z",
		})
		await openList(wrapper)
		pickOption(t("general.custom-time-range.title"))
		await flushPromises()

		expect(document.body.querySelector("[role='dialog']")).not.toBeNull()
		expect(wrapper.emitted("update:modelValue")).toBeUndefined()
	})

	it("stores the time range once one is picked", async ({ expect }) => {
		const wrapper = await mountSelect({
			modelValue: TimeRangePreset.Last24Hours,
		})
		await openList(wrapper)

		pickOption(t("editor.config-modal-utils.variable-time-range"))
		// the item selects only after two ticks of its own
		await flushPromises()

		expect(wrapper.emitted("update:modelValue")).toEqual([
			[METRIC_TIME_RANGE_REFERENCE],
		])
	})

	it.for([
		{ input: false, expected: false },
		{ input: true, expected: true },
	])(
		"blocks pointer input on the trigger when disabled is $input",
		async ({ input, expected }, { expect }) => {
			const wrapper = await mountSelect({
				modelValue: TimeRangePreset.Last24Hours,
				disabled: input,
			})

			expect(
				at(buttons(wrapper), 0).classes().includes("pointer-events-none"),
			).toBe(expected)
		},
	)

	it("keeps a disabled trigger fully opaque in both themes", async ({
		expect,
	}) => {
		const wrapper = await mountSelect({
			modelValue: TimeRangePreset.Last24Hours,
			disabled: true,
		})

		expect(at(buttons(wrapper), 0).classes()).toEqual(
			expect.arrayContaining([
				"disabled:opacity-100",
				"dark:disabled:opacity-100",
			]),
		)
	})

	it("opens no list from the keyboard while disabled", async ({ expect }) => {
		const wrapper = await mountSelect({
			modelValue: TimeRangePreset.Last24Hours,
			disabled: true,
		})

		await openList(wrapper)

		expect.soft(at(buttons(wrapper), 0).attributes("disabled")).toBeDefined()
		expect.soft(optionLabels()).toEqual([])
	})

	it("tints nothing and shows no old time range outside a modification", async ({
		expect,
	}) => {
		const wrapper = await mountSelect({
			modelValue: TimeRangePreset.Last24Hours,
		})

		expect(buttons(wrapper)).toHaveLength(1)
		expect(at(buttons(wrapper), 0).classes()).not.toContain(
			"bg-diff-field-added",
		)
	})

	it.for<{
		name: string
		oldValue: TimeRangeSetting | null
		expected: () => string
	}>([
		{
			name: "shows the time range a diff replaced beside the new one",
			oldValue: TimeRangePreset.Last1Hour,
			expected: () => t("general.time-range-options.last_1_hour"),
		},
		{
			name: "shows an empty old time range when a diff set the first one",
			oldValue: null,
			expected: () => "",
		},
	])("$name", async ({ oldValue, expected }, { expect }) => {
		const wrapper = await mountSelect({
			modelValue: TimeRangePreset.Last24Hours,
			oldValue: oldValue,
		})

		expect(buttons(wrapper)).toHaveLength(2)

		const current = at(buttons(wrapper), 0)
		const old = at(buttons(wrapper), 1)

		expect
			.soft(current.text())
			.toBe(t("general.time-range-options.last_24_hours"))
		expect.soft(current.classes()).toContain("bg-diff-field-added")
		expect.soft(old.text()).toBe(expected())
		expect.soft(old.classes()).toContain("bg-diff-field-removed")
	})
})

function mountSelect(props: {
	modelValue: TimeRangeSetting | null
	disabled?: boolean
	oldValue?: TimeRangeSetting | null
}) {
	return mountSuspended(TimeRangeSelect, {
		props: { disabled: false, ...props },
	})
}

function buttons(wrapper: VueWrapper) {
	return wrapper.findAll("button")
}

async function openList(wrapper: VueWrapper) {
	await at(buttons(wrapper), 0).trigger("keydown", { key: "Enter" })
	await nextTick()
}

function optionLabels(): string[] {
	return Array.from(document.body.querySelectorAll("[role='option']")).map(
		(o) => o.textContent.trim(),
	)
}

function pickOption(label: string) {
	Array.from(document.body.querySelectorAll("[role='option']"))
		.find((o) => o.textContent.trim() === label)
		?.dispatchEvent(
			new KeyboardEvent("keydown", { key: "Enter", bubbles: true }),
		)
}
