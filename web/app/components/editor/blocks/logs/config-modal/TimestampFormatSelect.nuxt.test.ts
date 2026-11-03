import { mountSuspended } from "@nuxt/test-utils/runtime"
import type { VueWrapper } from "@vue/test-utils"
import { beforeEach, describe, it } from "vitest"
import TimestampFormatSelect from "./TimestampFormatSelect.vue"
import { LogTimestampFormat } from "../utils"
import {
	at,
	clearTeleportedOverlays,
	menuItem,
	t,
} from "~/components/test-helpers"
import { DiffStatus } from "~/components/editor/diff/position-map"

// the editable flag is a shared cookie state, the editor store is
// app-wide, and the menu body is teleported into a shared <body>, so these
// tests cannot interleave
describe("<TimestampFormatSelect>", { concurrent: false }, () => {
	beforeEach(() => {
		clearTeleportedOverlays()
		useEditorMeta().setEditable(true)
		useEditorStore().setReviewableDiffActive(false)
	})

	it.for(Object.values(LogTimestampFormat))(
		"shows the title and example of the %s format while closed",
		async (format, { expect }) => {
			const wrapper = await mountFormatSelect({ modelValue: format })

			expect(
				trigger(wrapper)
					.findAll("span")
					.map((part) => part.text())
					.filter(Boolean),
			).toEqual([titleOf(format), exampleOf(format)])
		},
	)

	it("offers every format with an example", async ({ expect }) => {
		const wrapper = await mountFormatSelect({
			modelValue: LogTimestampFormat.Standard,
		})

		await openMenu(wrapper)

		const items = menuItems()

		expect(items).toHaveLength(Object.values(LogTimestampFormat).length)
		Object.values(LogTimestampFormat).forEach((format, i) => {
			expect(at(items, i).textContent).toContain(titleOf(format))
			expect(at(items, i).textContent).toContain(exampleOf(format))
		})
	})

	it("checks the chosen format in the menu", async ({ expect }) => {
		const wrapper = await mountFormatSelect({
			modelValue: LogTimestampFormat.DetailedMilli,
		})

		await openMenu(wrapper)

		expect(
			menuItems().map(
				(item) => item.querySelector('[class~="i-lucide:check"]') !== null,
			),
		).toEqual(
			Object.values(LogTimestampFormat).map(
				(format) => format === LogTimestampFormat.DetailedMilli,
			),
		)
	})

	it("stores the format the reader picked", async ({ expect }) => {
		const wrapper = await mountFormatSelect({
			modelValue: LogTimestampFormat.Standard,
		})
		await openMenu(wrapper)

		menuItem(titleOf(LogTimestampFormat.DetailedNano)).click()
		await nextTick()

		expect(wrapper.emitted("update:modelValue")).toEqual([
			[LogTimestampFormat.DetailedNano],
		])
	})

	it("offers no menu in read mode", async ({ expect }) => {
		useEditorMeta().setEditable(false)

		const wrapper = await mountFormatSelect({
			modelValue: LogTimestampFormat.Standard,
		})

		expect(trigger(wrapper).attributes("disabled")).toBeDefined()
		expect(trigger(wrapper).classes()).toContain("pointer-events-none")
	})

	it("offers no menu while a reviewable diff is shown", async ({ expect }) => {
		useEditorStore().setReviewableDiffActive(true)

		const wrapper = await mountFormatSelect({
			modelValue: LogTimestampFormat.Standard,
		})

		expect(trigger(wrapper).attributes("disabled")).toBeDefined()
		expect(trigger(wrapper).classes()).toContain("pointer-events-none")
	})

	it.for([
		{
			name: "tints the field of a format a diff added",
			input: DiffStatus.Added,
			expected: ["bg-diff-field-added"],
		},
		{
			name: "tints the field of a format a diff removed",
			input: DiffStatus.Removed,
			expected: ["bg-diff-field-removed"],
		},
		{
			name: "leaves the field of a format outside a diff untinted",
			input: null,
			expected: [],
		},
	])("$name", async ({ input, expected }, { expect }) => {
		const wrapper = await mountFormatSelect({
			modelValue: LogTimestampFormat.Standard,
			diffStatus: input,
		})

		expect(
			trigger(wrapper)
				.classes()
				.filter((c) => c.startsWith("bg-diff-field-")),
		).toEqual(expected)
	})
})

function mountFormatSelect(props: {
	modelValue: LogTimestampFormat
	diffStatus?: DiffStatus | null
}) {
	return mountSuspended(TimestampFormatSelect, { props: props })
}

function trigger(wrapper: VueWrapper) {
	return wrapper.get("[data-slot='dropdown-menu-trigger']")
}

// reka opens a dropdown on the pointerdown and click a user's click sends
async function openMenu(wrapper: VueWrapper) {
	await trigger(wrapper).trigger("pointerdown", { button: 0 })
	await trigger(wrapper).trigger("click")
	await nextTick()
}

function menuItems(): HTMLElement[] {
	return Array.from(
		document.body.querySelectorAll<HTMLElement>("[role^='menuitem']"),
	)
}

function titleOf(format: LogTimestampFormat): string {
	return t(`editor.logs.config.timestamp-format-options.${format}.title`)
}

function exampleOf(format: LogTimestampFormat): string {
	return t(`editor.logs.config.timestamp-format-options.${format}.example`)
}
