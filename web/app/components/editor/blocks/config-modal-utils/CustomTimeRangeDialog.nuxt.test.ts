import { mountSuspended } from "@nuxt/test-utils/runtime"
import { DOMWrapper, enableAutoUnmount, flushPromises } from "@vue/test-utils"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import { format } from "date-fns"
import CustomTimeRangeDialog from "./CustomTimeRangeDialog.vue"
import {
	clearTeleportedOverlays,
	t,
	teleportedButton,
} from "~/components/test-helpers"
import { serializeCustomTimeRange } from "~/utils/time-range"

const FROM = new Date(2026, 2, 28, 9, 30, 15, 123)
const TO = new Date(2026, 2, 29, 18, 45, 30, 456)
const RANGE = serializeCustomTimeRange(FROM, TO)

// dialogs share the document's teleport target
describe("<CustomTimeRangeDialog>", { concurrent: false }, () => {
	enableAutoUnmount(afterEach)
	beforeEach(clearTeleportedOverlays)

	it("restores a saved range in the local timezone without losing precision", async ({
		expect,
	}) => {
		await mountDialog()

		expect(input("from").element.value).toBe(
			format(FROM, "yyyy-MM-dd'T'HH:mm:ss.SSS"),
		)
		expect(input("to").element.value).toBe(
			format(TO, "yyyy-MM-dd'T'HH:mm:ss.SSS"),
		)
		expect(document.body.textContent).toContain(
			t("general.custom-time-range.description", {
				timeZone: Intl.DateTimeFormat().resolvedOptions().timeZone,
			}),
		)
	})

	it("applies the edited local times as UTC instants", async ({ expect }) => {
		const wrapper = await mountDialog()
		const edited = new Date(2026, 2, 28, 10, 15, 20, 789)
		await input("from").setValue(format(edited, "yyyy-MM-dd'T'HH:mm:ss.SSS"))
		teleportedButton(t("general.custom-time-range.apply")).click()
		await flushPromises()

		expect
			.soft(wrapper.emitted("apply"))
			.toEqual([[serializeCustomTimeRange(edited, TO)]])
		expect.soft(wrapper.emitted("update:open")).toEqual([[false]])
	})

	it("cancels without applying the edited range", async ({ expect }) => {
		const wrapper = await mountDialog()
		await input("from").setValue("2025-01-01T10:00")
		teleportedButton(t("general.custom-time-range.cancel")).click()
		await flushPromises()

		expect.soft(wrapper.emitted("apply")).toBeUndefined()
		expect.soft(wrapper.emitted("update:open")).toEqual([[false]])
	})

	it.for([
		{ name: "missing", value: "" },
		{ name: "equal", value: format(TO, "yyyy-MM-dd'T'HH:mm:ss.SSS") },
		{ name: "inverted", value: "2030-01-01T10:00" },
	])("refuses a $name start time", async ({ value }, { expect }) => {
		const wrapper = await mountDialog()
		await input("from").setValue(value)

		expect
			.soft(teleportedButton(t("general.custom-time-range.apply")).disabled)
			.toBe(true)
		expect
			.soft(document.body.querySelector("[role='alert']")?.textContent.trim())
			.toBe(t("general.custom-time-range.invalid"))
		const form = document.body.querySelector("form")
		if (!form) {
			throw new Error("Time range form is missing")
		}

		await new DOMWrapper(form).trigger("submit")
		expect.soft(wrapper.emitted("apply")).toBeUndefined()
	})

	it("discards an unapplied draft when reopened", async ({ expect }) => {
		const wrapper = await mountDialog()
		await input("from").setValue("2025-01-01T10:00")
		await wrapper.setProps({ open: false })
		await wrapper.setProps({ open: true })
		await flushPromises()

		expect(input("from").element.value).toBe(
			format(FROM, "yyyy-MM-dd'T'HH:mm:ss.SSS"),
		)
	})

	it("rejects a local time skipped by daylight saving", async ({ expect }) => {
		vi.stubEnv("TZ", "America/New_York")
		await mountDialog()
		await input("from").setValue("2026-03-08T02:30")

		expect(
			teleportedButton(t("general.custom-time-range.apply")).disabled,
		).toBe(true)
	})
})

function mountDialog() {
	return mountSuspended(CustomTimeRangeDialog, {
		props: { open: true, timeRange: RANGE },
	})
}

function input(boundary: "from" | "to") {
	const label = Array.from(document.body.querySelectorAll("label")).find(
		(element) =>
			element.textContent.trim() === t(`general.custom-time-range.${boundary}`),
	)
	const element = document.getElementById(
		label?.htmlFor ?? "",
	) as HTMLInputElement

	return new DOMWrapper(element)
}
