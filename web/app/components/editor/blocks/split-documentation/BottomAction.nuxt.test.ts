import type { VueWrapper } from "@vue/test-utils"
import { describe, it } from "vitest"
import BottomAction from "./BottomAction.vue"
import {
	findButtonByText,
	mountUnderTooltipProvider,
	renderedIconNames,
	t,
} from "~/components/test-helpers"

const SHORTCUT = {
	keyboardKey: { macOS: "⌘+K", other: "Ctrl+K" },
	i18nKey: null,
}

function mountAction(props: Record<string, unknown>) {
	return mountUnderTooltipProvider(BottomAction, { props: props })
}

// the emitted button-click payloads, in the order they were emitted
function clicks(wrapper: VueWrapper): unknown[] {
	return (
		wrapper.findComponent(BottomAction).emitted("button-click") ?? []
	).map((args) => args[0])
}

describe("<SplitDocumentationBottomAction>", () => {
	it("labels the first button with the given text", async ({ expect }) => {
		const wrapper = await mountAction({
			buttonText: t(
				"editor.split-documentation.left-side-bottom-action-button",
			),
		})

		expect(wrapper.text()).toContain(
			t("editor.split-documentation.left-side-bottom-action-button"),
		)
	})

	it("shows a plus icon on the first button by default", async ({ expect }) => {
		const wrapper = await mountAction({
			buttonText: t(
				"editor.split-documentation.left-side-bottom-action-button",
			),
		})

		expect(renderedIconNames(wrapper)).toEqual(["lucide:circle-plus"])
	})

	it("shows the given icon on the first button", async ({ expect }) => {
		const wrapper = await mountAction({
			buttonText: t(
				"editor.split-documentation.left-side-bottom-action-button",
			),
			buttonIcon: "lucide:list-plus",
		})

		expect(renderedIconNames(wrapper)).toEqual(["lucide:list-plus"])
	})

	it("reports a first-button click", async ({ expect }) => {
		const wrapper = await mountAction({
			buttonText: t(
				"editor.split-documentation.left-side-bottom-action-button",
			),
		})

		await findButtonByText(
			wrapper,
			t("editor.split-documentation.left-side-bottom-action-button"),
		).trigger("click")

		expect(clicks(wrapper)).toEqual(["first"])
	})

	it("renders only one button when no second text is given", async ({
		expect,
	}) => {
		const wrapper = await mountAction({
			buttonText: t(
				"editor.split-documentation.right-side-bottom-action-buttons.add-code",
			),
		})

		expect(wrapper.findAll("button")).toHaveLength(1)
	})

	it("renders the second button when its text is given", async ({ expect }) => {
		const wrapper = await mountAction({
			buttonText: t(
				"editor.split-documentation.right-side-bottom-action-buttons.add-code",
			),
			secondButtonText: t(
				"editor.split-documentation.right-side-bottom-action-buttons.add-metrics",
			),
		})

		expect(wrapper.findAll("button")).toHaveLength(2)
		expect(wrapper.text()).toContain(
			t(
				"editor.split-documentation.right-side-bottom-action-buttons.add-metrics",
			),
		)
	})

	it("shows a plus icon on the second button by default", async ({
		expect,
	}) => {
		const wrapper = await mountAction({
			buttonText: t(
				"editor.split-documentation.right-side-bottom-action-buttons.add-code",
			),
			buttonIcon: "lucide:code",
			secondButtonText: t(
				"editor.split-documentation.right-side-bottom-action-buttons.add-metrics",
			),
		})

		expect(renderedIconNames(wrapper)).toEqual([
			"lucide:code",
			"lucide:circle-plus",
		])
	})

	it("shows the given icon on the second button", async ({ expect }) => {
		const wrapper = await mountAction({
			buttonText: t(
				"editor.split-documentation.right-side-bottom-action-buttons.add-code",
			),
			buttonIcon: "lucide:code",
			secondButtonText: t(
				"editor.split-documentation.right-side-bottom-action-buttons.add-metrics",
			),
			secondButtonIcon: "lucide:chart-line",
		})

		expect(renderedIconNames(wrapper)).toEqual([
			"lucide:code",
			"lucide:chart-line",
		])
	})

	it("reports a second-button click", async ({ expect }) => {
		const wrapper = await mountAction({
			buttonText: t(
				"editor.split-documentation.right-side-bottom-action-buttons.add-code",
			),
			secondButtonText: t(
				"editor.split-documentation.right-side-bottom-action-buttons.add-metrics",
			),
		})

		await findButtonByText(
			wrapper,
			t(
				"editor.split-documentation.right-side-bottom-action-buttons.add-metrics",
			),
		).trigger("click")

		expect(clicks(wrapper)).toEqual(["second"])
	})

	it("wraps a button carrying a shortcut in a tooltip trigger", async ({
		expect,
	}) => {
		const wrapper = await mountAction({
			buttonText: t(
				"editor.split-documentation.left-side-bottom-action-button",
			),
			buttonShortcut: SHORTCUT,
		})

		expect(wrapper.findAll("[data-slot='tooltip-trigger']")).toHaveLength(1)
	})

	it("swallows a mousedown on the overlay background itself", async ({
		expect,
	}) => {
		const wrapper = await mountAction({
			buttonText: t(
				"editor.split-documentation.left-side-bottom-action-button",
			),
		})
		const event = new MouseEvent("mousedown", {
			bubbles: true,
			cancelable: true,
		})

		wrapper.get("div").element.dispatchEvent(event)

		expect(event.defaultPrevented).toBe(true)
	})

	it("lets a mousedown on the button through", async ({ expect }) => {
		const wrapper = await mountAction({
			buttonText: t(
				"editor.split-documentation.left-side-bottom-action-button",
			),
		})
		const event = new MouseEvent("mousedown", {
			bubbles: true,
			cancelable: true,
		})

		findButtonByText(
			wrapper,
			t("editor.split-documentation.left-side-bottom-action-button"),
		).element.dispatchEvent(event)

		expect(event.defaultPrevented).toBe(false)
	})
})
