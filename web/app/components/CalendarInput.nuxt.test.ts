import { mountSuspended } from "@nuxt/test-utils/runtime"
import { CalendarDate } from "@internationalized/date"
import { beforeEach, describe, it, vi } from "vitest"
import CalendarInput from "./CalendarInput.vue"
import { clearTeleportedOverlays, t } from "./test-helpers"

function mountInput(props: {
	placeholder: string
	modelValue?: CalendarDate
	availableFromTomorrow?: boolean
}) {
	return mountSuspended(CalendarInput, { props: props })
}

// the popover body is teleported to <body>, which every mount in the file
// shares — reading it there is the only way to see the calendar
function popoverText() {
	return document.body.textContent
}

// the popover body is teleported into the shared <body>, and the frozen
// clock the availability tests need is a global, so these cannot interleave
describe("<CalendarInput>", { concurrent: false }, () => {
	beforeEach(clearTeleportedOverlays)

	it("shows the placeholder while nothing is picked", async ({ expect }) => {
		const wrapper = await mountInput({
			placeholder: t("editor.hooks.time-expiration.calendar-placeholder"),
		})

		expect(wrapper.text()).toContain(
			t("editor.hooks.time-expiration.calendar-placeholder"),
		)
	})

	it("shows the picked date instead of the placeholder", async ({ expect }) => {
		const wrapper = await mountInput({
			placeholder: t("editor.hooks.time-expiration.calendar-placeholder"),
			modelValue: new CalendarDate(2026, 3, 14),
		})

		expect(wrapper.text()).toContain("March 14, 2026")
		expect(wrapper.text()).not.toContain(
			t("editor.hooks.time-expiration.calendar-placeholder"),
		)
	})

	it("greys out the trigger while nothing is picked", async ({ expect }) => {
		const wrapper = await mountInput({
			placeholder: t("editor.hooks.time-expiration.calendar-placeholder"),
		})

		expect(wrapper.get("button").classes()).toContain("text-muted-foreground")
	})

	it("keeps the trigger in the normal colour once a date is picked", async ({
		expect,
	}) => {
		const wrapper = await mountInput({
			placeholder: t("editor.hooks.time-expiration.calendar-placeholder"),
			modelValue: new CalendarDate(2026, 3, 14),
		})

		expect(wrapper.get("button").classes()).not.toContain(
			"text-muted-foreground",
		)
	})

	it("keeps the calendar closed until the trigger is pressed", async ({
		expect,
	}) => {
		await mountInput({
			placeholder: t("editor.hooks.time-expiration.calendar-placeholder"),
		})

		expect(popoverText()).not.toContain("March")
	})

	it("opens the calendar when the trigger is pressed", async ({ expect }) => {
		const wrapper = await mountInput({
			placeholder: t("editor.hooks.time-expiration.calendar-placeholder"),
			modelValue: new CalendarDate(2026, 3, 14),
		})

		await wrapper.get("button").trigger("click")
		await nextTick()

		expect(wrapper.get("button").attributes("data-state")).toBe("open")
		expect(document.body.querySelector("table")).not.toBeNull()
	})

	it("closes the calendar once a date is picked", async ({ expect }) => {
		const wrapper = await mountInput({
			placeholder: t("editor.hooks.time-expiration.calendar-placeholder"),
			modelValue: new CalendarDate(2026, 3, 14),
		})
		await wrapper.get("button").trigger("click")
		await nextTick()

		await wrapper.setProps({ modelValue: new CalendarDate(2026, 3, 15) })
		await nextTick()

		expect(wrapper.get("button").attributes("data-state")).toBe("closed")
	})

	it("reports the day the user clicks in the calendar", async ({ expect }) => {
		const wrapper = await mountInput({
			placeholder: t("editor.hooks.time-expiration.calendar-placeholder"),
			modelValue: new CalendarDate(2026, 3, 14),
		})
		await wrapper.get("button").trigger("click")
		await nextTick()

		const day = document.body.querySelector<HTMLElement>(
			"[data-reka-calendar-cell-trigger][data-value='2026-03-20']",
		)
		day?.click()
		await nextTick()

		expect(String(wrapper.emitted("update:modelValue")?.at(-1)?.[0])).toBe(
			"2026-03-20",
		)
	})

	it("leaves every day selectable by default", async ({ expect }) => {
		vi.setSystemTime(new Date("2026-03-14T12:00:00Z"))
		const wrapper = await mountInput({
			placeholder: t("editor.hooks.time-expiration.calendar-placeholder"),
			modelValue: new CalendarDate(2026, 3, 14),
		})

		await wrapper.get("button").trigger("click")
		await nextTick()

		expect(
			document.body.querySelector("[data-value='2026-03-14'][data-disabled]"),
		).toBeNull()
	})

	it("disables today and earlier when only tomorrow onwards is available", async ({
		expect,
	}) => {
		vi.setSystemTime(new Date("2026-03-14T12:00:00Z"))
		const wrapper = await mountInput({
			placeholder: t("editor.hooks.time-expiration.calendar-placeholder"),
			modelValue: new CalendarDate(2026, 3, 20),
			availableFromTomorrow: true,
		})

		await wrapper.get("button").trigger("click")
		await nextTick()

		expect(
			document.body.querySelector("[data-value='2026-03-14'][data-disabled]"),
		).not.toBeNull()
		expect(
			document.body.querySelector("[data-value='2026-03-15'][data-disabled]"),
		).toBeNull()
	})
})
