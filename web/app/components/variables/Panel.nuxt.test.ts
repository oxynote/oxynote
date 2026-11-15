import type { VueWrapper } from "@vue/test-utils"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import { toast } from "vue-sonner"
import {
	clearQueryCache,
	disposeMockEndpoints,
	mockDeferredEndpoint,
	mockEndpoint,
} from "~/composables/api/test-helpers"
import { stubThemeColorContext } from "../editor/test-helpers/theme"
import {
	clearTeleportedOverlays,
	emitFrom,
	findButtonByText,
	mountUnderTooltipProvider,
	raisedToasts,
	refuseForRole,
	seedAuthOrganization,
	seedAuthSession,
	settleMutations,
	t,
	teleportedButton,
} from "../test-helpers"
import EditList from "./EditList.vue"
import GroupForm from "./GroupForm.vue"
import NormalList from "./NormalList.vue"
import Panel from "./Panel.vue"
import VariableForm from "./VariableForm.vue"
import {
	GROUP_DEPLOYS,
	GROUP_ENV,
	TREE_URL,
	VAR_REGION,
	builtinTree,
	openAllGroups,
	fullTree,
	mockTree,
} from "./test-helpers"

vi.mock("vue-sonner", () => ({
	toast: { custom: vi.fn(), dismiss: vi.fn() },
}))

// the query cache, the collapse state, the websocket store, the mocked
// toast, the fake timers and the dialog teleported into <body> are all
// shared by every mount in the file
describe("<Panel>", { concurrent: false }, () => {
	beforeEach(() => {
		clearQueryCache()
		clearTeleportedOverlays()
		openAllGroups()
		vi.mocked(toast.custom).mockClear()
	})
	afterEach(() => {
		vi.useRealTimers()
		disposeMockEndpoints()
	})

	describe("modes", () => {
		it("opens in normal mode with only the values", async ({ expect }) => {
			mockTree(fullTree())

			const wrapper = await mountPanel()

			expect(wrapper.text()).toContain(t("variable.intro.normal"))
			expect(wrapper.findComponent(NormalList).exists()).toBe(true)
			expect(wrapper.findComponent(EditList).exists()).toBe(false)
		})

		it("toggles between sections by page and one list", async ({ expect }) => {
			mockTree(fullTree())
			const wrapper = await mountPanel()
			const before = wrapper
				.findComponent(NormalList)
				.props("byPage") as boolean

			await findButtonByText(
				wrapper,
				t("variable.actions.show-one-list"),
			).trigger("click")

			expect(before).toBe(true)
			expect(wrapper.findComponent(NormalList).props("byPage")).toBe(false)
			expect(
				findButtonByText(wrapper, t("variable.actions.show-by-page")).exists(),
			).toBe(true)
		})

		it("explains the current order in the toggle's tooltip", async ({
			expect,
		}) => {
			mockTree(fullTree())
			const wrapper = await mountPanel()

			await findButtonByText(
				wrapper,
				t("variable.actions.show-one-list"),
			).trigger("focus")
			await nextTick()

			expect(
				document.body.querySelector("[data-slot='tooltip-content']")
					?.textContent,
			).toContain(t("variable.by-page-tooltip.on-now"))
		})

		it("switches to edit mode from the pencil", async ({ expect }) => {
			mockTree(fullTree())
			const wrapper = await mountPanel()

			await findButtonByText(wrapper, t("variable.actions.edit")).trigger(
				"click",
			)

			expect(wrapper.text()).toContain(t("variable.intro.edit"))
			expect(wrapper.findComponent(EditList).exists()).toBe(true)
			expect(wrapper.findComponent(NormalList).exists()).toBe(false)
			expect(
				findButtonByText(wrapper, t("variable.actions.done")).exists(),
			).toBe(true)
		})

		it("stays open and says so while in edit mode", async ({ expect }) => {
			mockTree(fullTree())
			const wrapper = await mountPanel()

			await findButtonByText(wrapper, t("variable.actions.edit")).trigger(
				"click",
			)

			expect(wrapper.findComponent(Panel).emitted("update:editing")).toEqual([
				[true],
			])
			expect(wrapper.text()).not.toContain(t("variable.actions.close"))
		})

		it("returns to normal mode from done", async ({ expect }) => {
			mockTree(fullTree())
			const wrapper = await mountPanel()
			await findButtonByText(wrapper, t("variable.actions.edit")).trigger(
				"click",
			)

			await findButtonByText(wrapper, t("variable.actions.done")).trigger(
				"click",
			)

			expect(wrapper.text()).toContain(t("variable.intro.normal"))
			expect(wrapper.findComponent(NormalList).exists()).toBe(true)
		})

		it("switches to edit mode from the create more variables button", async ({
			expect,
		}) => {
			mockTree(builtinTree())
			const wrapper = await mountPanel()

			await findButtonByText(
				wrapper,
				t("variable.empty.builtin-only-action"),
			).trigger("click")

			expect(wrapper.findComponent(EditList).exists()).toBe(true)
			expect(wrapper.text()).toContain(t("variable.empty.no-groups-title"))
		})
	})

	describe("for a viewer", { concurrent: false }, () => {
		beforeEach(() => {
			seedAuthSession({ id: "u1" })
			seedAuthOrganization({ members: [{ userId: "u1", role: "viewer" }] })
		})

		it("offers no edit mode", async ({ expect }) => {
			mockTree(fullTree())

			const wrapper = await mountPanel()

			expect(() =>
				findButtonByText(wrapper, t("variable.actions.edit")),
			).toThrow()
		})
	})

	it("leaves edit mode once the reader's role turns to viewer", async ({
		expect,
	}) => {
		mockTree(fullTree())
		seedAuthSession({ id: "u1" })
		seedAuthOrganization({ members: [{ userId: "u1", role: "editor" }] })
		const wrapper = await mountEditing()

		seedAuthOrganization({ members: [{ userId: "u1", role: "viewer" }] })
		await nextTick()

		expect.soft(wrapper.findComponent(EditList).exists()).toBe(false)
		expect.soft(wrapper.findComponent(NormalList).exists()).toBe(true)
		expect
			.soft(wrapper.findComponent(Panel).emitted("update:editing"))
			.toEqual([[true], [false]])
	})

	describe("forms", () => {
		it("opens a new variable in the group whose add row was pressed", async ({
			expect,
		}) => {
			mockTree(fullTree())
			const wrapper = await mountEditing()

			await findButtonByText(
				wrapper,
				t("variable.actions.add-variable"),
			).trigger("click")

			const form = wrapper.findComponent(VariableForm)
			expect(form.props("variableId")).toBeNull()
			expect(form.props("groupId")).toBe(GROUP_ENV)
		})

		it("opens a variable from its chevron", async ({ expect }) => {
			mockTree(fullTree())
			const wrapper = await mountEditing()

			await findButtonByText(
				wrapper,
				t("variable.actions.open-variable", { name: "Region" }),
			).trigger("click")

			expect(wrapper.findComponent(VariableForm).props("variableId")).toBe(
				VAR_REGION,
			)
		})

		it.for([
			{ name: "opens a new group from the add group row", input: null },
			{ name: "opens a group from its pencil", input: GROUP_ENV },
		])("$name", async ({ input }, { expect }) => {
			stubThemeColorContext()
			mockTree(fullTree())
			const wrapper = await mountEditing()

			await findButtonByText(
				wrapper,
				input
					? t("variable.actions.rename-group", { name: "Environment" })
					: t("variable.actions.add-group"),
			).trigger("click")

			expect(wrapper.findComponent(GroupForm).props("groupId")).toBe(input)
		})

		it("returns to edit mode once a form closes", async ({ expect }) => {
			mockTree(fullTree())
			const wrapper = await mountEditing()
			await findButtonByText(
				wrapper,
				t("variable.actions.add-variable"),
			).trigger("click")

			emitFrom(wrapper, VariableForm, "close")
			await nextTick()

			expect(wrapper.findComponent(VariableForm).exists()).toBe(false)
			expect(wrapper.findComponent(EditList).exists()).toBe(true)
		})
	})

	describe("deleting a variable", () => {
		it("asks first, naming the variable and how its references will show", async ({
			expect,
		}) => {
			mockTree(fullTree())
			const wrapper = await mountEditing()

			await askToDeleteRegion(wrapper)

			expect(dialogText()).toContain("Region")
			expect(dialogText()).toContain("$region")
		})

		it("deletes the variable once confirmed", async ({ expect }) => {
			mockTree(fullTree())
			const calls = mockEndpoint(
				"DELETE",
				`/api/variables/${VAR_REGION}`,
				() => null,
			)
			const wrapper = await mountEditing()
			await askToDeleteRegion(wrapper)

			await confirmDeletion()

			expect.soft(calls).toHaveLength(1)
			expect.soft(raisedToasts()).toEqual([
				expect.objectContaining({
					type: "success",
					title: t("variable.toasts.deleted", { name: "Region" }),
				}),
			])
		})

		it("returns to the list when the open variable is deleted", async ({
			expect,
		}) => {
			mockTree(fullTree())
			mockEndpoint("DELETE", `/api/variables/${VAR_REGION}`, () => null)
			const wrapper = await mountEditing()
			await findButtonByText(
				wrapper,
				t("variable.actions.open-variable", { name: "Region" }),
			).trigger("click")
			await findButtonByText(
				wrapper,
				t("variable.variable-form.delete-button"),
			).trigger("click")

			await confirmDeletion()

			expect(wrapper.findComponent(VariableForm).exists()).toBe(false)
			expect(wrapper.findComponent(EditList).exists()).toBe(true)
		})

		it("raises a toast and stays put when the deletion fails", async ({
			expect,
		}) => {
			mockTree(fullTree())
			const calls = mockEndpoint(
				"DELETE",
				`/api/variables/${VAR_REGION}`,
				() => {
					throw createError({ statusCode: 500 })
				},
			)
			const wrapper = await mountEditing()
			await findButtonByText(
				wrapper,
				t("variable.actions.open-variable", { name: "Region" }),
			).trigger("click")
			await findButtonByText(
				wrapper,
				t("variable.variable-form.delete-button"),
			).trigger("click")

			await confirmDeletion()

			expect.soft(calls).toHaveLength(1)
			expect.soft(wrapper.findComponent(VariableForm).exists()).toBe(true)
			expect.soft(raisedToasts()).toEqual([
				expect.objectContaining({
					type: "error",
					title: t("variable.errors.delete-failed"),
				}),
			])
		})

		it("shows only the access denied toast for a refused deletion", async ({
			expect,
		}) => {
			mockTree(fullTree())
			mockEndpoint("DELETE", `/api/variables/${VAR_REGION}`, (_call, event) =>
				refuseForRole(event),
			)
			const wrapper = await mountEditing()
			await findButtonByText(
				wrapper,
				t("variable.actions.open-variable", { name: "Region" }),
			).trigger("click")
			await findButtonByText(
				wrapper,
				t("variable.variable-form.delete-button"),
			).trigger("click")

			await confirmDeletion()

			expect.soft(wrapper.findComponent(VariableForm).exists()).toBe(true)
			expect
				.soft(raisedToasts())
				.toMatchObject([
					{ type: "error", title: t("general.errors.role-not-allowed") },
				])
		})
	})

	describe("deleting a group", () => {
		it("deletes an empty group once confirmed and returns to the list", async ({
			expect,
		}) => {
			stubThemeColorContext()
			mockTree(fullTree())
			const calls = mockEndpoint(
				"DELETE",
				`/api/variables/groups/${GROUP_DEPLOYS}`,
				() => null,
			)
			const wrapper = await mountEditing()
			await askToDeleteGroup(wrapper, "Deploys")

			await confirmDeletion("variable.deletion-modal.group-confirm-button")

			expect.soft(calls).toHaveLength(1)
			expect.soft(wrapper.findComponent(GroupForm).exists()).toBe(false)
			expect.soft(raisedToasts()).toEqual([
				expect.objectContaining({
					type: "success",
					title: t("variable.toasts.deleted", { name: "Deploys" }),
				}),
			])
		})

		it("raises a toast and stays put when the group cannot be deleted", async ({
			expect,
		}) => {
			stubThemeColorContext()
			mockTree(fullTree())
			const calls = mockEndpoint(
				"DELETE",
				`/api/variables/groups/${GROUP_DEPLOYS}`,
				() => {
					throw createError({ statusCode: 500 })
				},
			)
			const wrapper = await mountEditing()
			await askToDeleteGroup(wrapper, "Deploys")

			await confirmDeletion("variable.deletion-modal.group-confirm-button")

			expect.soft(calls).toHaveLength(1)
			expect.soft(wrapper.findComponent(GroupForm).exists()).toBe(true)
			expect.soft(raisedToasts()).toEqual([
				expect.objectContaining({
					type: "error",
					title: t("variable.errors.group-delete-failed"),
				}),
			])
		})

		it("shows only the access denied toast for a refused group deletion", async ({
			expect,
		}) => {
			stubThemeColorContext()
			mockTree(fullTree())
			mockEndpoint(
				"DELETE",
				`/api/variables/groups/${GROUP_DEPLOYS}`,
				(_call, event) => refuseForRole(event),
			)
			const wrapper = await mountEditing()
			await askToDeleteGroup(wrapper, "Deploys")

			await confirmDeletion("variable.deletion-modal.group-confirm-button")

			expect.soft(wrapper.findComponent(GroupForm).exists()).toBe(true)
			expect
				.soft(raisedToasts())
				.toMatchObject([
					{ type: "error", title: t("general.errors.role-not-allowed") },
				])
		})
	})

	it("asks to be closed from its header", async ({ expect }) => {
		mockTree(fullTree())
		const wrapper = await mountPanel()

		await findButtonByText(wrapper, t("variable.actions.close")).trigger(
			"click",
		)

		expect(
			wrapper.findComponent(Panel).emitted("close-variable-panel"),
		).toHaveLength(1)
	})

	it("shows a spinner while the variables load", async ({ expect }) => {
		const request = mockDeferredEndpoint("GET", TREE_URL)

		const wrapper = await mountPanel()
		await request.reached

		expect(wrapper.find(".i-svg-spinners\\:blocks-shuffle-3").exists()).toBe(
			true,
		)
		request.resolve([])
	})

	it("says when the variables cannot be loaded", async ({ expect }) => {
		mockEndpoint("GET", TREE_URL, () => {
			throw createError({ statusCode: 500 })
		})

		const wrapper = await mountPanel()
		await settleMutations()

		expect(wrapper.text()).toContain(t("variable.errors.load-failed"))
	})
})

function mountPanel() {
	return mountUnderTooltipProvider(Panel, {
		props: { usedVariableIds: new Set() },
	})
}

async function mountEditing() {
	const wrapper = await mountPanel()
	await settleMutations()
	await findButtonByText(wrapper, t("variable.actions.edit")).trigger("click")

	return wrapper
}

function dialogText() {
	return (
		document.body.querySelector("[data-slot='dialog-content']")?.textContent ??
		""
	)
}

async function askToDeleteRegion(wrapper: VueWrapper) {
	await findButtonByText(
		wrapper,
		t("variable.actions.open-variable", { name: "Region" }),
	).trigger("click")
	await findButtonByText(
		wrapper,
		t("variable.variable-form.delete-button"),
	).trigger("click")
}

// the dialog holds its spinner for 300ms before the deletion goes out
async function confirmDeletion(
	label = "variable.deletion-modal.variable-confirm-button",
) {
	vi.useFakeTimers()
	teleportedButton(t(label)).click()
	await vi.advanceTimersByTimeAsync(300)
	await settleMutations()
	vi.useRealTimers()
	await settleMutations()
}

async function askToDeleteGroup(wrapper: VueWrapper, name: string) {
	await findButtonByText(
		wrapper,
		t("variable.actions.rename-group", { name: name }),
	).trigger("click")
	await findButtonByText(
		wrapper,
		t("variable.group-form.delete-button"),
	).trigger("click")
}
