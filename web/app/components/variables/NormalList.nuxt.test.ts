import type { VueWrapper } from "@vue/test-utils"
import { beforeEach, describe, it } from "vitest"
import type { VariableTreeResponse } from "~/utils"
import { clearQueryCache, runInApp } from "~/composables/api/test-helpers"
import {
	at,
	findButtonByText,
	mountUnderTooltipProvider,
	seedAuthOrganization,
	seedAuthSession,
	t,
} from "../test-helpers"
import NormalList from "./NormalList.vue"
import ValueControl from "./ValueControl.vue"
import { useGroupOpenState } from "./group-state"
import {
	VAR_ENV,
	VAR_RANGE,
	VAR_REFRESH,
	builtinTree,
	openAllGroups,
	resetGroupState,
	GROUP_ENV,
	fullTree,
} from "./test-helpers"

// the collapse state is one persisted ref shared by every mount
describe("<NormalList>", { concurrent: false }, () => {
	beforeEach(() => {
		clearQueryCache()
		openAllGroups()
	})

	it("lists the variables the page uses under on this page", async ({
		expect,
	}) => {
		const wrapper = await mountList(fullTree(), [VAR_ENV])

		expect(sectionShape(wrapper)).toEqual([
			{
				heading: t("variable.sections.on-this-page"),
				groups: ["Environment"],
				variables: ["Environment"],
			},
			{
				heading: t("variable.sections.other"),
				groups: ["Metrics", "Environment", "Deploys"],
				variables: ["Metric time range", "Metric refresh interval", "Region"],
			},
		])
	})

	it("puts the built-in group first on a page with a metric chart", async ({
		expect,
	}) => {
		const wrapper = await mountList(fullTree(), [
			VAR_RANGE,
			VAR_REFRESH,
			VAR_ENV,
		])

		expect(at(sectionShape(wrapper), 0).groups).toEqual([
			"Metrics",
			"Environment",
		])
	})

	it("titles the only section as used on other pages while the page uses none", async ({
		expect,
	}) => {
		const wrapper = await mountList(fullTree(), [])

		expect(wrapper.findAll("h3").map((h) => h.text())).toEqual([
			t("variable.sections.used-on-other-pages"),
		])
	})

	it("lists every group in its global order without sections when not by page", async ({
		expect,
	}) => {
		const wrapper = await mountList(fullTree(), [VAR_ENV], false)

		expect(wrapper.findAll("h3")).toHaveLength(0)
		expect(variableNames(wrapper)).toEqual([
			"Metric time range",
			"Metric refresh interval",
			"Environment",
			"Region",
		])
	})

	it("shows an empty group under other with a note", async ({ expect }) => {
		const wrapper = await mountList(fullTree(), [])

		const other = wrapper.findAll("section").at(-1)

		expect(other?.text()).toContain("Deploys")
		expect(other?.text()).toContain(t("variable.empty.group"))
	})

	it("tags the built-in group as built-in", async ({ expect }) => {
		const wrapper = await mountList(fullTree(), [])

		expect(wrapper.text()).toContain(t("variable.builtin.tag"))
	})

	it("offers no drag handles or edit actions", async ({ expect }) => {
		const wrapper = await mountList(fullTree(), [])

		expect(wrapper.find(".i-mingcute\\:dots-line").exists()).toBe(false)
		expect(wrapper.text()).not.toContain(t("variable.actions.add-variable"))
	})

	it("starts only the built-in group open on this page", async ({ expect }) => {
		resetGroupState()

		const wrapper = await mountList(fullTree(), [
			VAR_RANGE,
			VAR_REFRESH,
			VAR_ENV,
		])

		expect(variableNames(wrapper)).toEqual([
			"Metric time range",
			"Metric refresh interval",
		])
	})

	it("starts every group closed among the other variables", async ({
		expect,
	}) => {
		resetGroupState()

		const wrapper = await mountList(fullTree(), [])

		expect(variableNames(wrapper)).toEqual([])
	})

	it("starts a group created since the page loaded open", async ({
		expect,
	}) => {
		resetGroupState()
		runInApp(() => useGroupOpenState().created.value.push(GROUP_ENV))

		const wrapper = await mountList(fullTree(), [])

		expect(variableNames(wrapper)).toEqual(["Environment", "Region"])
	})

	it("hides a group's variables once its header is pressed", async ({
		expect,
	}) => {
		const wrapper = await mountList(fullTree(), [])

		await wrapper.get("[data-slot='collapsible-trigger']").trigger("click")

		expect(variableNames(wrapper)).toEqual(["Environment", "Region"])
	})

	describe("when only the built-in variables exist", () => {
		it("offers to create more variables below them", async ({ expect }) => {
			const wrapper = await mountList(builtinTree(), [])

			expect(variableNames(wrapper)).toEqual([
				"Metric time range",
				"Metric refresh interval",
			])
			expect(wrapper.text()).toContain(t("variable.empty.builtin-only-title"))
			expect(wrapper.text()).toContain(
				t("variable.empty.builtin-only-description"),
			)
		})

		it("tells a viewer who adds variables and offers no button for it", async ({
			expect,
		}) => {
			seedAuthSession({ id: "u1" })
			seedAuthOrganization({ members: [{ userId: "u1", role: "viewer" }] })

			const wrapper = await mountList(builtinTree(), [])

			expect
				.soft(wrapper.text())
				.toContain(t("variable.empty.builtin-only-description-viewer"))
			expect
				.soft(() =>
					findButtonByText(wrapper, t("variable.empty.builtin-only-action")),
				)
				.toThrow()
		})

		it("asks for edit mode from the create button", async ({ expect }) => {
			const wrapper = await mountList(builtinTree(), [])

			await findButtonByText(
				wrapper,
				t("variable.empty.builtin-only-action"),
			).trigger("click")

			expect(wrapper.findComponent(NormalList).emitted("edit")).toHaveLength(1)
		})
	})

	it("leaves out the empty state once a group of their own exists", async ({
		expect,
	}) => {
		const wrapper = await mountList(fullTree(), [])

		expect(wrapper.text()).not.toContain(t("variable.empty.builtin-only-title"))
	})
})

function mountList(
	tree: VariableTreeResponse,
	usedIds: string[],
	byPage = true,
) {
	return mountUnderTooltipProvider(NormalList, {
		props: { tree: tree, usedIds: new Set(usedIds), byPage: byPage },
	})
}

function variableNames(wrapper: VueWrapper) {
	return wrapper
		.findAllComponents(ValueControl)
		.map((control) => control.get("label").text())
}

function sectionShape(wrapper: VueWrapper) {
	return wrapper.findAll("section").map((section) => ({
		heading: section.get("h3").text(),
		groups: section
			.findAll("[data-slot='collapsible-trigger']")
			.map((trigger) => trigger.text()),
		variables: section.findAll("label").map((label) => label.text()),
	}))
}
