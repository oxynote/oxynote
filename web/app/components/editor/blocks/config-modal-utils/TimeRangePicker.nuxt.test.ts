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
import {
	seedPersistentState,
	t,
	emitFrom,
	emitFromNth,
} from "~/components/test-helpers"
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

	it.for(["2026-11-01T05:30:00.000Z", "2026-11-01T06:30:00.000Z"])(
		"preserves saved overlap occurrence %s when applying and changing zones",
		async (start, { expect }) => {
			setLocalTimeZone("America/New_York")
			seedPersistentState("custom-time-range-zone", "browser")
			const saved = serializeCustomTimeRange(
				new Date(start),
				new Date("2026-11-01T07:30:00.000Z"),
			)
			const wrapper = await mountSuspended(TimeRangePicker, {
				props: { timeRange: saved },
			})
			expect(
				wrapper.get('button[type="submit"]').attributes("disabled"),
			).toBeUndefined()
			await wrapper.get("form").trigger("submit")
			expect(wrapper.emitted("apply")).toEqual([[saved]])
			emitFromNth(wrapper, Select, 1, "update:modelValue", "UTC")
			await flushPromises()
			expect(
				new Date(
					wrapper.get<HTMLInputElement>("input").element.value + "Z",
				).toISOString(),
			).toBe(start)
			await wrapper.get("form").trigger("submit")
			expect(wrapper.emitted("apply")).toEqual([[saved], [saved]])
		},
	)
	it("lets the user explicitly choose the later occurrence of an edited overlap", async ({
		expect,
	}) => {
		setLocalTimeZone("America/New_York")
		seedPersistentState("custom-time-range-zone", "browser")
		const saved = serializeCustomTimeRange(
			new Date("2026-11-01T05:30:00.000Z"),
			new Date("2026-11-01T07:30:00.000Z"),
		)
		const wrapper = await mountSuspended(TimeRangePicker, {
			props: { timeRange: saved },
		})
		await wrapper.get("input").setValue("2026-11-01T01:35")
		expect(
			wrapper.get('button[type="submit"]').attributes("disabled"),
		).toBeDefined()
		emitFrom(
			wrapper,
			Select,
			"update:modelValue",
			String(new Date("2026-11-01T06:35:00.000Z").getTime()),
		)
		await flushPromises()
		await wrapper.get("form").trigger("submit")
		expect(wrapper.emitted("apply")).toEqual([
			[
				serializeCustomTimeRange(
					new Date("2026-11-01T06:35:00.000Z"),
					new Date("2026-11-01T07:30:00.000Z"),
				),
			],
		])
	})
	it("keeps edited fields when a global reference finishes loading", async ({
		expect,
	}) => {
		const wrapper = await mountSuspended(TimeRangePicker, {
			props: { timeRange: null },
		})
		await wrapper.get("input").setValue("2025-01-01T10:00")
		await wrapper.setProps({ timeRange: RANGE })
		expect(wrapper.get<HTMLInputElement>("input").element.value).toBe(
			"2025-01-01T10:00",
		)
	})
	it("updates untouched fields when a global reference finishes loading", async ({
		expect,
	}) => {
		const wrapper = await mountSuspended(TimeRangePicker, {
			props: { timeRange: null },
		})
		await wrapper.setProps({ timeRange: RANGE })
		expect(wrapper.get<HTMLInputElement>("input").element.value).toBe(
			format(FROM, "yyyy-MM-dd'T'HH:mm:ss.SSS"),
		)
	})
	it("applies newly edited local boundaries as exact UTC instants", async ({
		expect,
	}) => {
		setLocalTimeZone("Asia/Kolkata")
		seedPersistentState("custom-time-range-zone", "browser")
		const wrapper = await mountSuspended(TimeRangePicker, {
			props: { timeRange: RANGE },
		})
		const inputs = wrapper.findAll("input")
		await inputs[0]?.setValue("2026-11-12T10:00:15.123")
		await inputs[1]?.setValue("2026-11-12T10:30:30.456")
		await wrapper.get("form").trigger("submit")
		expect(wrapper.emitted("apply")).toEqual([
			[
				serializeCustomTimeRange(
					new Date("2026-11-12T04:30:15.123Z"),
					new Date("2026-11-12T05:00:30.456Z"),
				),
			],
		])
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
