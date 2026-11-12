import { mountSuspended } from "@nuxt/test-utils/runtime"
import { afterEach, beforeEach, describe, it } from "vitest"
import { enableAutoUnmount, flushPromises } from "@vue/test-utils"
import {
	resetLocalTimeZone,
	setLocalTimeZone,
	getLocalTimeZone,
} from "@internationalized/date"
import { Select } from "#layers/ui/app/components/shadcn/ui/select"
import { format } from "date-fns"
import TimeRangePicker from "./TimeRangePicker.vue"
import { seedPersistentState, t, emitFrom } from "~/components/test-helpers"
import { serializeCustomTimeRange } from "~/utils/time-range"

const FROM = new Date("2026-11-11T05:38:15.123Z")
const TO = new Date("2026-11-11T05:42:30.456Z")
const RANGE = serializeCustomTimeRange(FROM, TO)

// recent ranges share persistent app state
describe("<TimeRangePicker>", { concurrent: false }, () => {
	enableAutoUnmount(afterEach)
	afterEach(resetLocalTimeZone)
	beforeEach(() => {
		seedPersistentState("custom-time-range-history", [])
		seedPersistentState("custom-time-range-zone", getLocalTimeZone())
	})
	it("restores the selected absolute window", async ({ expect }) => {
		const wrapper = await mountSuspended(TimeRangePicker, {
			props: { timeRange: RANGE },
		})
		expect(
			wrapper.get<HTMLInputElement>('input[type="datetime-local"]').element
				.value,
		).toBe(format(FROM, "yyyy-MM-dd'T'HH:mm:ss.SSS"))
	})
	it("applies exact UTC instants and remembers the range", async ({
		expect,
	}) => {
		const wrapper = await mountSuspended(TimeRangePicker, {
			props: { timeRange: RANGE },
		})
		await wrapper.get("form").trigger("submit")
		expect(wrapper.emitted("apply")).toEqual([[RANGE]])
		expect(wrapper.text()).not.toContain(
			t("general.custom-time-range.empty-recent"),
		)
	})
	it("converts the inputs to UTC without changing the selected instants", async ({
		expect,
	}) => {
		setLocalTimeZone("Asia/Kolkata")
		seedPersistentState("custom-time-range-zone", "Asia/Kolkata")
		seedPersistentState("custom-time-range-history", [RANGE])
		const wrapper = await mountSuspended(TimeRangePicker, {
			props: { timeRange: RANGE },
		})
		expect(wrapper.text()).toContain("11:08")
		emitFrom(wrapper, Select, "update:modelValue", "UTC")
		await flushPromises()
		expect(wrapper.text()).toContain("5:38")
		expect(wrapper.get<HTMLInputElement>("input").element.value).toBe(
			FROM.toISOString().slice(0, -1),
		)
		await wrapper.get("form").trigger("submit")
		expect(wrapper.emitted("apply")).toEqual([[RANGE]])
	})
	it("rejects times skipped by daylight saving", async ({ expect }) => {
		setLocalTimeZone("America/New_York")
		seedPersistentState("custom-time-range-zone", "America/New_York")
		const wrapper = await mountSuspended(TimeRangePicker, {
			props: { timeRange: RANGE },
		})
		await wrapper.get("input").setValue("2026-03-08T02:30")
		expect(
			wrapper.get('button[type="submit"]').attributes("disabled"),
		).toBeDefined()
	})

	it("reuses a previously applied absolute range", async ({ expect }) => {
		seedPersistentState("custom-time-range-history", [RANGE])
		const wrapper = await mountSuspended(TimeRangePicker, {
			props: { timeRange: null },
		})
		const recent = wrapper
			.findAll("button")
			.find((button) => button.text().includes("–"))
		expect(recent).toBeDefined()
		await recent?.trigger("click")
		expect(wrapper.emitted("apply")).toEqual([[RANGE]])
	})

	it("blocks inverted and missing dates", async ({ expect }) => {
		const wrapper = await mountSuspended(TimeRangePicker, {
			props: { timeRange: RANGE },
		})
		await wrapper.get("input").setValue("2030-01-01T00:00")
		await wrapper.get("form").trigger("submit")
		expect(
			wrapper.get('button[type="submit"]').attributes("disabled"),
		).toBeDefined()
		expect(wrapper.emitted("apply")).toBeUndefined()
		await wrapper.get("input").setValue("")
		expect(wrapper.get('[role="alert"]').text()).toBe(
			t("general.custom-time-range.invalid"),
		)
	})
})
