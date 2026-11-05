import { mountSuspended } from "@nuxt/test-utils/runtime"
import {
	enableAutoUnmount,
	flushPromises,
	type VueWrapper,
} from "@vue/test-utils"
import { afterEach, beforeEach, describe, it } from "vitest"
import RefreshIntervalSelect from "./RefreshIntervalSelect.vue"
import { RefreshInterval } from "~/utils/time-range"
import {
	METRIC_REFRESH_INTERVAL_REFERENCE,
	type RefreshIntervalSetting,
} from "~/utils/variable"
import { at, clearTeleportedOverlays, t } from "~/components/test-helpers"

// the open list is teleported into <body>, which every mount shares
describe("<RefreshIntervalSelect>", { concurrent: false }, () => {
	enableAutoUnmount(afterEach)

	beforeEach(() => {
		clearTeleportedOverlays()
	})

	it.for<{
		name: string
		input: RefreshIntervalSetting | null
		expected: () => string
	}>([
		{
			name: "labels an interval with its short name",
			input: RefreshInterval.S30,
			expected: () => t("general.refresh-interval-options-short.30s"),
		},
		{
			name: "labels the built-in reference as the shared setting",
			input: METRIC_REFRESH_INTERVAL_REFERENCE,
			expected: () => t("editor.config-modal-utils.variable-refresh-interval"),
		},
		{
			name: "leaves the label empty without a refresh interval",
			input: null,
			expected: () => "",
		},
	])("$name", async ({ input, expected }, { expect }) => {
		const wrapper = await mountSelect({ modelValue: input })

		expect(buttons(wrapper)).toHaveLength(1)
		expect(at(buttons(wrapper), 0).text()).toBe(expected())
	})

	it("lists the built-in reference first, then every interval", async ({
		expect,
	}) => {
		const wrapper = await mountSelect({
			modelValue: RefreshInterval.S30,
		})

		await openList(wrapper)

		expect(optionLabels()).toEqual([
			t("editor.config-modal-utils.variable-refresh-interval"),
			...Object.values(RefreshInterval).map((interval) =>
				t(`general.refresh-interval-options.${interval}`),
			),
		])
	})

	it("stores the refresh interval once one is picked", async ({ expect }) => {
		const wrapper = await mountSelect({
			modelValue: RefreshInterval.S30,
		})
		await openList(wrapper)

		pickOption(t("editor.config-modal-utils.variable-refresh-interval"))
		// the item selects only after two ticks of its own
		await flushPromises()

		expect(wrapper.emitted("update:modelValue")).toEqual([
			[METRIC_REFRESH_INTERVAL_REFERENCE],
		])
	})

	it.for([
		{ input: false, expected: false },
		{ input: true, expected: true },
	])(
		"blocks pointer input on the trigger when disabled is $input",
		async ({ input, expected }, { expect }) => {
			const wrapper = await mountSelect({
				modelValue: RefreshInterval.S30,
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
			modelValue: RefreshInterval.S30,
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
			modelValue: RefreshInterval.S30,
			disabled: true,
		})

		await openList(wrapper)

		expect.soft(at(buttons(wrapper), 0).attributes("disabled")).toBeDefined()
		expect.soft(optionLabels()).toEqual([])
	})

	it("tints nothing and shows no old refresh interval outside a modification", async ({
		expect,
	}) => {
		const wrapper = await mountSelect({
			modelValue: RefreshInterval.S30,
		})

		expect(buttons(wrapper)).toHaveLength(1)
		expect(at(buttons(wrapper), 0).classes()).not.toContain(
			"bg-diff-field-added",
		)
	})

	it.for<{
		name: string
		oldValue: RefreshIntervalSetting | null
		expected: () => string
	}>([
		{
			name: "shows the refresh interval a diff replaced beside the new one",
			oldValue: RefreshInterval.H1,
			expected: () => t("general.refresh-interval-options-short.1h"),
		},
		{
			name: "shows an empty old refresh interval when a diff set the first one",
			oldValue: null,
			expected: () => "",
		},
	])("$name", async ({ oldValue, expected }, { expect }) => {
		const wrapper = await mountSelect({
			modelValue: RefreshInterval.S30,
			oldValue: oldValue,
		})

		expect(buttons(wrapper)).toHaveLength(2)

		const current = at(buttons(wrapper), 0)
		const old = at(buttons(wrapper), 1)

		expect
			.soft(current.text())
			.toBe(t("general.refresh-interval-options-short.30s"))
		expect.soft(current.classes()).toContain("bg-diff-field-added")
		expect.soft(old.text()).toBe(expected())
		expect.soft(old.classes()).toContain("bg-diff-field-removed")
	})
})

function mountSelect(props: {
	modelValue: RefreshIntervalSetting | null
	disabled?: boolean
	oldValue?: RefreshIntervalSetting | null
}) {
	return mountSuspended(RefreshIntervalSelect, {
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
