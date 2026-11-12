import { mountSuspended } from "@nuxt/test-utils/runtime"
import { enableAutoUnmount, flushPromises } from "@vue/test-utils"
import { afterEach, beforeEach, describe, it } from "vitest"
import TimeRangeSelect from "./TimeRangeSelect.vue"
import { getLocalTimeZone } from "@internationalized/date"
import { differenceInCalendarDays } from "date-fns"
import { TimeRangePreset, serializeCustomTimeRange } from "~/utils/time-range"
import { METRIC_TIME_RANGE_REFERENCE } from "~/utils/variable"
import {
	clearTeleportedOverlays,
	at,
	t,
	teleportedButton,
	seedPersistentState,
} from "~/components/test-helpers"
import { clearQueryCache, seedQueryData } from "~/composables/api/test-helpers"
import { VARIABLE_QUERY_KEYS } from "~/composables/api/useVariableAPI"
import { variableTree } from "../metrics/test-helpers"

// popovers and variable queries share app-wide state
describe("<TimeRangeSelect>", { concurrent: false }, () => {
	enableAutoUnmount(afterEach)
	beforeEach(() => {
		clearTeleportedOverlays()
		clearQueryCache()
		seedQueryData(VARIABLE_QUERY_KEYS.tree, variableTree())
		seedPersistentState("custom-time-range-history", [])
		seedPersistentState("custom-time-range-zone", getLocalTimeZone())
	})
	it.for([
		{
			name: "same-day",
			from: new Date(2026, 10, 11, 5, 38),
			to: new Date(2026, 10, 11, 5, 42),
			sameDay: true,
			year: false,
		},
		{
			name: "cross-day",
			from: new Date(2026, 10, 11, 5, 38),
			to: new Date(2026, 10, 23, 4, 2),
			sameDay: false,
			year: false,
		},
		{
			name: "cross-year",
			from: new Date(2026, 11, 31, 23, 58),
			to: new Date(2027, 0, 1, 0, 2),
			sameDay: false,
			year: true,
		},
	])(
		"keeps a compact $name label",
		async ({ from, to, sameDay, year }, { expect }) => {
			const wrapper = await mountSuspended(TimeRangeSelect, {
				props: {
					modelValue: serializeCustomTimeRange(from, to),
					disabled: false,
				},
			})
			const { d } = useNuxtApp().$i18n
			const options: Intl.DateTimeFormatOptions = {
				month: "short",
				day: "numeric",
				...(year ? { year: "numeric" as const } : {}),
			}
			const startTime = `${from.getHours()}:${String(from.getMinutes()).padStart(2, "0")}`
			const endTime = `${to.getHours()}:${String(to.getMinutes()).padStart(2, "0")}`
			expect(wrapper.get("button").text()).toBe(
				t("general.custom-time-range.label", {
					from: `${d(from, options)} ${startTime}`,
					to: sameDay ? endTime : `${d(to, options)} ${endTime}`,
				}),
			)
		},
	)
	it("initializes the absolute inputs from the resolved global range", async ({
		expect,
	}) => {
		const wrapper = await mountSuspended(TimeRangeSelect, {
			props: { modelValue: METRIC_TIME_RANGE_REFERENCE, disabled: false },
		})
		await wrapper.get("button").trigger("click")
		await flushPromises()
		const inputs = Array.from(
			document.body.querySelectorAll<HTMLInputElement>(
				'input[type="datetime-local"]',
			),
		)
		expect(
			differenceInCalendarDays(
				new Date(at(inputs, 1).value),
				new Date(at(inputs, 0).value),
			),
		).toBe(7)
	})

	it("opens absolute inputs and relative presets together", async ({
		expect,
	}) => {
		const wrapper = await mountSuspended(TimeRangeSelect, {
			props: { modelValue: TimeRangePreset.Last1Hour, disabled: false },
		})
		await wrapper.get("button").trigger("click")
		await flushPromises()
		expect(
			document.body.querySelectorAll('input[type="datetime-local"]'),
		).toHaveLength(2)
		expect(document.body.textContent).toContain(
			t("general.custom-time-range.absolute"),
		)
		expect(document.body.textContent).toContain(
			t("general.custom-time-range.relative"),
		)
		expect(wrapper.emitted("update:modelValue")).toBeUndefined()
	})
	it.for([TimeRangePreset.Last5Minutes, METRIC_TIME_RANGE_REFERENCE])(
		"applies %s directly",
		async (value, { expect }) => {
			const wrapper = await mountSuspended(TimeRangeSelect, {
				props: { modelValue: TimeRangePreset.Last1Hour, disabled: false },
			})
			await wrapper.get("button").trigger("click")
			await flushPromises()
			const text =
				value === METRIC_TIME_RANGE_REFERENCE
					? t("editor.config-modal-utils.variable-time-range")
					: t(`general.time-range-options.${value}`)
			teleportedButton(text).click()
			await flushPromises()
			expect(wrapper.emitted("update:modelValue")).toEqual([[value]])
		},
	)
	it("discards a draft when closed without applying", async ({ expect }) => {
		const saved = serializeCustomTimeRange(
			new Date("2026-11-11T05:38:15.123Z"),
			new Date("2026-11-11T05:42:30.456Z"),
		)
		const wrapper = await mountSuspended(TimeRangeSelect, {
			props: { modelValue: saved, disabled: false },
		})
		await wrapper.get("button").trigger("click")
		await flushPromises()
		const original = Array.from(
			document.body.querySelectorAll<HTMLInputElement>(
				'input[type="datetime-local"]',
			),
		).map((input) => input.value)
		const input = document.body.querySelector<HTMLInputElement>(
			'input[type="datetime-local"]',
		)
		if (!input) throw new Error("From input missing")
		input.value = "2025-01-01T10:00"
		input.dispatchEvent(new Event("input", { bubbles: true }))
		await wrapper.get("button").trigger("click")
		await flushPromises()
		await wrapper.get("button").trigger("click")
		await flushPromises()
		expect(
			Array.from(
				document.body.querySelectorAll<HTMLInputElement>(
					'input[type="datetime-local"]',
				),
			).map((element) => element.value),
		).toEqual(original)
		expect(wrapper.emitted("update:modelValue")).toBeUndefined()
	})
	it("formats its trigger in the selected UTC zone", async ({ expect }) => {
		seedPersistentState("custom-time-range-zone", "UTC")
		const from = new Date("2026-11-11T05:38:15.123Z")
		const to = new Date("2026-11-11T05:42:30.456Z")
		const wrapper = await mountSuspended(TimeRangeSelect, {
			props: {
				modelValue: serializeCustomTimeRange(from, to),
				disabled: false,
			},
		})
		const { d } = useNuxtApp().$i18n
		expect(wrapper.get("button").text()).toBe(
			t("general.custom-time-range.label", {
				from: `${d(from, { month: "short", day: "numeric", timeZone: "UTC" })} 5:38`,
				to: "5:42",
			}),
		)
	})

	it("disables the trigger in read mode", async ({ expect }) => {
		const wrapper = await mountSuspended(TimeRangeSelect, {
			props: { modelValue: TimeRangePreset.Last1Hour, disabled: true },
		})
		expect(wrapper.get("button").attributes("disabled")).toBeDefined()
	})
})
