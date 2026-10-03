import { mountSuspended } from "@nuxt/test-utils/runtime"
import { beforeEach, describe, it } from "vitest"
import HookMenuFooter from "./HookMenuFooter.vue"
import {
	findButtonByText,
	renderedIconNames,
	t,
} from "~/components/test-helpers"

// the editor store is shared, so these tests cannot interleave
describe("<HookMenuFooter>", { concurrent: false }, () => {
	beforeEach(() => {
		useEditorStore().setActiveBranchProtected(false)
	})

	it("confirms a new hook", async ({ expect }) => {
		const wrapper = await mountSuspended(HookMenuFooter, {
			props: { mode: "create" },
		})

		await findButtonByText(wrapper, t("editor.hooks.create")).trigger("click")

		expect(wrapper.emitted("submit")).toEqual([[]])
		expect(wrapper.emitted("delete")).toBeUndefined()
		expect(wrapper.findAll("button")).toHaveLength(1)
	})

	it("keeps the confirm button disabled while the host says so", async ({
		expect,
	}) => {
		const wrapper = await mountSuspended(HookMenuFooter, {
			props: { mode: "create", submitDisabled: true },
		})

		expect(
			findButtonByText(wrapper, t("editor.hooks.create")).attributes(
				"disabled",
			),
		).toBeDefined()
	})

	it("updates an existing hook", async ({ expect }) => {
		const wrapper = await mountSuspended(HookMenuFooter, {
			props: { mode: "edit" },
		})

		await findButtonByText(wrapper, t("editor.hooks.update")).trigger("click")

		expect(wrapper.emitted("submit")).toEqual([[]])
		expect(wrapper.emitted("delete")).toBeUndefined()
		expect(renderedIconNames(wrapper)).toEqual([
			"mingcute:save-2-line",
			"mingcute:delete-2-line",
		])
	})

	it("deletes an existing hook", async ({ expect }) => {
		const wrapper = await mountSuspended(HookMenuFooter, {
			props: { mode: "edit" },
		})

		await findButtonByText(wrapper, t("editor.hooks.delete")).trigger("click")

		expect(wrapper.emitted("delete")).toEqual([[]])
		expect(wrapper.emitted("submit")).toBeUndefined()
	})

	it("saves under the label and icon it is given", async ({ expect }) => {
		const wrapper = await mountSuspended(HookMenuFooter, {
			props: {
				mode: "edit",
				submitLabel: t("editor.hooks.renew"),
				submitIcon: "mingcute:check-fill",
			},
		})

		expect(findButtonByText(wrapper, t("editor.hooks.renew")).exists()).toBe(
			true,
		)
		expect(renderedIconNames(wrapper)).toEqual([
			"mingcute:check-fill",
			"mingcute:delete-2-line",
		])
		expect(wrapper.text()).not.toContain(t("editor.hooks.update"))
	})

	it("tells a reader in reading mode how to change the hook", async ({
		expect,
	}) => {
		const wrapper = await mountSuspended(HookMenuFooter, {
			props: { mode: "read-only" },
		})

		expect(wrapper.text()).toBe(t("editor.hooks.read-only-mode"))
		expect(renderedIconNames(wrapper)).toEqual(["mingcute:lock-line"])
		expect(wrapper.findAll("button")).toHaveLength(0)
	})

	it("sends a reader on a protected branch to the draft branch", async ({
		expect,
	}) => {
		useEditorStore().setActiveBranchProtected(true)

		const wrapper = await mountSuspended(HookMenuFooter, {
			props: { mode: "read-only" },
		})

		expect(wrapper.text()).toBe(t("editor.hooks.read-only-protected"))
		expect(wrapper.findAll("button")).toHaveLength(0)
	})

	it("tells how to leave the diff to edit", async ({ expect }) => {
		const wrapper = await mountSuspended(HookMenuFooter, {
			props: { mode: "diff" },
		})

		const toggle = t("editor.name-editor.review-workflow.show-diff")
		expect(collapsed(wrapper.text())).toBe(
			t("editor.hooks.showing-changes", { toggle: toggle }),
		)
		expect(wrapper.get(".font-semibold").text()).toBe(toggle)
		expect(renderedIconNames(wrapper)).toEqual(["mingcute:git-compare-line"])
		expect(wrapper.findAll("button")).toHaveLength(0)
	})
})

// the bold part sits on lines of its own in the template, which leaves
// spaces around it in the text
function collapsed(text: string): string {
	return text.replace(/\s+/g, " ")
}
