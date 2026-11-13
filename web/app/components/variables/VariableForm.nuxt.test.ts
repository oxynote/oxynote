import { mountSuspended } from "@nuxt/test-utils/runtime"
import type { VueWrapper } from "@vue/test-utils"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import { toast } from "vue-sonner"
import {
	clearQueryCache,
	disposeMockEndpoints,
	mockEndpoint,
} from "~/composables/api/test-helpers"
import { Select } from "#layers/ui/app/components/shadcn/ui/select"
import {
	WAIT_FOR_OPTIONS,
	emitFrom,
	findButtonByText,
	raisedToasts,
	t,
} from "../test-helpers"
import OptionsField from "./OptionsField.vue"
import TimeRangeSelect from "./TimeRangeSelect.vue"
import FormHeader from "./FormHeader.vue"
import VariableForm from "./VariableForm.vue"
import {
	GROUP_DEPLOYS,
	GROUP_ENV,
	OPTION_1M,
	OPTION_PROD,
	OPTION_STAGING,
	VAR_ENV,
	VAR_RANGE,
	VAR_REFRESH,
	builtinTree,
	fullTree,
	mockTree,
} from "./test-helpers"

vi.mock("vue-sonner", () => ({
	toast: { custom: vi.fn(), dismiss: vi.fn() },
}))

const VARIABLES_URL = "/api/variables"

// the query cache and the mocked toast are shared by every mount
describe("<VariableForm>", { concurrent: false }, () => {
	beforeEach(() => {
		clearQueryCache()
		vi.mocked(toast.custom).mockClear()
	})
	afterEach(disposeMockEndpoints)

	describe("when creating a variable", () => {
		it.for([
			{
				name: "offers an options list for a list",
				input: () => t("general.variable-types.list"),
				expected: () => t("variable.variable-form.options-hint"),
			},
			{
				name: "offers a number field for a number",
				input: () => t("general.variable-types.number"),
				expected: () => t("variable.variable-form.default-empty-hint"),
			},
			{
				name: "offers a text area for text",
				input: () => t("general.variable-types.text"),
				expected: () => t("variable.variable-form.default-empty-hint"),
			},
			{
				name: "offers the time range presets for a time range",
				input: () => t("general.variable-types.time-range"),
				expected: () => t("variable.variable-form.default-preset-hint"),
			},
		])("$name", async ({ input, expected }, { expect }) => {
			const wrapper = await mountNew()

			await typeButton(wrapper, input()).trigger("click")

			expect(typeButton(wrapper, input()).attributes("aria-pressed")).toBe(
				"true",
			)
			expect(defaultFieldShape(wrapper)).toEqual(defaultShapeFor(input()))
			expect(wrapper.text()).toContain(expected())
		})

		it("derives the key from the name until the key is edited", async ({
			expect,
		}) => {
			const wrapper = await mountNew()

			await field(wrapper, t("variable.variable-form.name-label")).setValue(
				"Hotfix threshold",
			)
			const derived = (keyInput(wrapper).element as HTMLInputElement).value
			await keyInput(wrapper).setValue("hotfix_max")
			await field(wrapper, t("variable.variable-form.name-label")).setValue(
				"Hotfix limit",
			)

			expect(derived).toBe("hotfix_threshold")
			expect((keyInput(wrapper).element as HTMLInputElement).value).toBe(
				"hotfix_max",
			)
		})

		it("explains how to reference a well formed key", async ({ expect }) => {
			const wrapper = await mountNew()

			await keyInput(wrapper).setValue("severity")

			expect(wrapper.text()).toContain(
				t("variable.variable-form.key-hint", { reference: "$severity" }),
			)
		})

		it("shows a malformed key's error in place of the hint", async ({
			expect,
		}) => {
			const wrapper = await mountNew()

			await keyInput(wrapper).setValue("Region!")

			// vee-validate validates on its own scheduler, which nothing signals
			await vi.waitFor(() => {
				expect(wrapper.text()).toContain(
					t("variable.variable-form.key-invalid"),
				)
			}, WAIT_FOR_OPTIONS)
			expect(wrapper.text()).not.toContain(
				t("variable.variable-form.key-hint", { reference: "$Region!" }),
			)
			expect(keyInput(wrapper).attributes("aria-invalid")).toBe("true")
		})

		it("reports a key another variable already uses", async ({ expect }) => {
			const wrapper = await mountNew()

			await keyInput(wrapper).setValue("env")

			// vee-validate validates on its own scheduler, which nothing signals
			await vi.waitFor(() => {
				expect(wrapper.text()).toContain(
					t("variable.variable-form.key-taken", { reference: "$env" }),
				)
			}, WAIT_FOR_OPTIONS)
		})

		it("asks for an option before it creates a list", async ({ expect }) => {
			const calls = mockEndpoint("POST", VARIABLES_URL, () => ({}))
			const wrapper = await mountNew()
			await field(wrapper, t("variable.variable-form.name-label")).setValue(
				"Severity",
			)

			await wrapper.get("form").trigger("submit")

			await waitForValidation(() => {
				expect(wrapper.text()).toContain(
					t("variable.variable-form.options-required"),
				)
			})
			expect.soft(calls).toHaveLength(0)
		})

		it("asks for a group while there is none to pick", async ({ expect }) => {
			const calls = mockEndpoint("POST", VARIABLES_URL, () => ({}))
			const wrapper = await mountSuspended(VariableForm, {
				props: { tree: builtinTree(), variableId: null, groupId: null },
			})
			await field(wrapper, t("variable.variable-form.name-label")).setValue(
				"Severity",
			)
			await addOption(wrapper, "low")

			await wrapper.get("form").trigger("submit")

			await waitForValidation(() => {
				expect(wrapper.text()).toContain(
					t("variable.variable-form.group-required"),
				)
			})
			expect.soft(calls).toHaveLength(0)
		})

		it.for([
			{ name: "a number", input: () => t("general.variable-types.number") },
			{ name: "text", input: () => t("general.variable-types.text") },
		])(
			"asks for a default before it creates $name",
			async ({ input }, { expect }) => {
				const calls = mockEndpoint("POST", VARIABLES_URL, () => ({}))
				const wrapper = await mountNew()
				await typeButton(wrapper, input()).trigger("click")
				await field(wrapper, t("variable.variable-form.name-label")).setValue(
					"Severity",
				)

				await wrapper.get("form").trigger("submit")

				await waitForValidation(() => {
					expect(wrapper.text()).toContain(
						t("variable.variable-form.default-required"),
					)
				})
				expect.soft(calls).toHaveLength(0)
			},
		)

		it("asks for a name before it creates a variable", async ({ expect }) => {
			const calls = mockEndpoint("POST", VARIABLES_URL, () => ({}))
			const wrapper = await mountNew()
			await addOption(wrapper, "low")

			await wrapper.get("form").trigger("submit")

			await waitForValidation(() => {
				expect(wrapper.text()).toContain(
					t("variable.variable-form.name-required"),
				)
				expect(wrapper.text()).toContain(
					t("variable.variable-form.key-required"),
				)
			})
			expect.soft(calls).toHaveLength(0)
		})

		it("creates a list variable with its options in order", async ({
			expect,
		}) => {
			mockTree(fullTree())
			const calls = mockEndpoint("POST", VARIABLES_URL, () => ({}))
			const wrapper = await mountNew()

			await field(wrapper, t("variable.variable-form.name-label")).setValue(
				"Severity",
			)
			await addOption(wrapper, "low")
			await addOption(wrapper, "high")
			await wrapper.get("form").trigger("submit")
			await waitForValidation(() => {
				expect(wrapper.emitted("close")).toHaveLength(1)
			})

			expect.soft(calls.map((call) => call.body)).toEqual([
				{
					groupId: GROUP_DEPLOYS,
					key: "severity",
					name: "Severity",
					description: null,
					type: "list",
					options: [{ value: "low" }, { value: "high" }],
				},
			])
			expect.soft(wrapper.emitted("close")).toHaveLength(1)
		})

		it("creates a number variable with its default", async ({ expect }) => {
			mockTree(fullTree())
			const calls = mockEndpoint("POST", VARIABLES_URL, () => ({}))
			const wrapper = await mountNew()

			await typeButton(wrapper, t("general.variable-types.number")).trigger(
				"click",
			)
			await field(wrapper, t("variable.variable-form.name-label")).setValue(
				"Hotfix threshold",
			)
			await field(wrapper, t("variable.variable-form.default-label")).setValue(
				"1234",
			)
			await wrapper.get("form").trigger("submit")
			await waitForValidation(() => {
				expect(wrapper.emitted("close")).toHaveLength(1)
			})

			expect.soft(calls.map((call) => call.body)).toEqual([
				{
					groupId: GROUP_DEPLOYS,
					key: "hotfix_threshold",
					name: "Hotfix threshold",
					description: null,
					type: "number",
					defaultValue: "1234",
				},
			])
		})

		it("starts a new time range at the last 24 hours", async ({ expect }) => {
			mockTree(fullTree())
			const calls = mockEndpoint("POST", VARIABLES_URL, () => ({}))
			const wrapper = await mountNew()

			await typeButton(wrapper, t("general.variable-types.time-range")).trigger(
				"click",
			)
			await field(wrapper, t("variable.variable-form.name-label")).setValue(
				"Incident window",
			)
			await wrapper.get("form").trigger("submit")
			await waitForValidation(() => {
				expect(wrapper.emitted("close")).toHaveLength(1)
			})

			expect.soft(calls[0]?.body).toMatchObject({
				type: "time-range",
				defaultValue: "last_24_hours",
			})
		})

		it("offers no delete", async ({ expect }) => {
			const wrapper = await mountNew()

			expect(wrapper.text()).not.toContain(
				t("variable.variable-form.delete-button"),
			)
		})
	})

	describe("when editing a variable", () => {
		it("keeps the key when the variable is renamed", async ({ expect }) => {
			const wrapper = await mountEdit(VAR_ENV)

			await field(wrapper, t("variable.variable-form.name-label")).setValue(
				"Stage",
			)

			expect((keyInput(wrapper).element as HTMLInputElement).value).toBe("env")
		})

		it("saves the variable with its options keeping their identity", async ({
			expect,
		}) => {
			mockTree(fullTree())
			const calls = mockEndpoint(
				"PUT",
				`${VARIABLES_URL}/${VAR_ENV}`,
				() => null,
			)
			const wrapper = await mountEdit(VAR_ENV)

			await wrapper.get("form").trigger("submit")
			await waitForValidation(() => {
				expect(wrapper.emitted("close")).toHaveLength(1)
			})

			expect.soft(calls.map((call) => call.body)).toEqual([
				{
					groupId: GROUP_ENV,
					key: "env",
					name: "Environment",
					description: null,
					type: "list",
					options: [
						{ id: OPTION_PROD, value: "production" },
						{ id: OPTION_STAGING, value: "staging" },
					],
				},
			])
			expect.soft(wrapper.emitted("close")).toHaveLength(1)
		})

		it("asks to delete the variable", async ({ expect }) => {
			const wrapper = await mountEdit(VAR_ENV)

			await findButtonByText(
				wrapper,
				t("variable.variable-form.delete-button"),
			).trigger("click")

			expect(wrapper.emitted("delete")?.[0]?.[0]).toMatchObject({ id: VAR_ENV })
		})

		it("warns how references show once the variable is gone", async ({
			expect,
		}) => {
			const wrapper = await mountEdit(VAR_ENV)

			expect(wrapper.text()).toContain(
				t("variable.variable-form.delete-hint", { reference: "$env" }),
			)
		})

		it("raises a toast when the variable cannot be saved", async ({
			expect,
		}) => {
			mockTree(fullTree())
			const calls = mockEndpoint("PUT", `${VARIABLES_URL}/${VAR_ENV}`, () => {
				throw createError({ statusCode: 500 })
			})
			const wrapper = await mountEdit(VAR_ENV)

			await wrapper.get("form").trigger("submit")
			await waitForValidation(() => {
				expect(raisedToasts()).toHaveLength(1)
			})

			expect.soft(calls).toHaveLength(1)
			expect.soft(wrapper.emitted("close")).toBeUndefined()
			expect.soft(raisedToasts()).toEqual([
				expect.objectContaining({
					type: "error",
					title: t("variable.errors.save-failed"),
				}),
			])
		})
	})

	describe("when the variable is built in", () => {
		it("explains and locks everything but the default", async ({ expect }) => {
			const wrapper = await mountEdit(VAR_RANGE)

			expect(wrapper.text()).toContain(t("variable.builtin.form-notice"))
			expect(
				field(wrapper, t("variable.variable-form.name-label")).attributes(
					"disabled",
				),
			).toBeDefined()
			expect(keyInput(wrapper).attributes("disabled")).toBeDefined()
			expect(
				field(wrapper, t("variable.variable-form.group-label")).attributes(
					"disabled",
				),
			).toBeDefined()
			expect(wrapper.find("[aria-pressed]").exists()).toBe(false)
			expect(wrapper.findComponent(TimeRangeSelect).exists()).toBe(true)
			expect(wrapper.text()).not.toContain(
				t("variable.variable-form.delete-button"),
			)
		})

		it("saves a new default", async ({ expect }) => {
			mockTree(fullTree())
			const calls = mockEndpoint(
				"PUT",
				`${VARIABLES_URL}/${VAR_RANGE}/default`,
				() => null,
			)
			const wrapper = await mountEdit(VAR_RANGE)

			emitFrom(wrapper, Select, "update:modelValue", "last_7_days")
			await nextTick()
			await wrapper.get("form").trigger("submit")
			await waitForValidation(() => {
				expect(wrapper.emitted("close")).toHaveLength(1)
			})

			expect
				.soft(calls.map((call) => call.body))
				.toEqual([{ value: "last_7_days" }])
			expect.soft(wrapper.emitted("close")).toHaveLength(1)
		})

		it("picks a list's default from its options", async ({ expect }) => {
			mockTree(fullTree())
			const calls = mockEndpoint(
				"PUT",
				`${VARIABLES_URL}/${VAR_REFRESH}/default`,
				() => null,
			)
			const wrapper = await mountEdit(VAR_REFRESH)

			emitFrom(wrapper, Select, "update:modelValue", OPTION_1M)
			await nextTick()
			await wrapper.get("form").trigger("submit")
			await waitForValidation(() => {
				expect(wrapper.emitted("close")).toHaveLength(1)
			})

			expect(wrapper.findComponent(OptionsField).exists()).toBe(false)
			expect
				.soft(calls.map((call) => call.body))
				.toEqual([{ value: OPTION_1M }])
		})

		it("closes without a request while the default is unchanged", async ({
			expect,
		}) => {
			const calls = mockEndpoint(
				"PUT",
				`${VARIABLES_URL}/${VAR_RANGE}/default`,
				() => null,
			)
			const wrapper = await mountEdit(VAR_RANGE)

			await wrapper.get("form").trigger("submit")
			await waitForValidation(() => {
				expect(wrapper.emitted("close")).toHaveLength(1)
			})

			expect.soft(calls).toHaveLength(0)
		})
	})

	it("goes back from its header while nothing was changed", async ({
		expect,
	}) => {
		const calls = mockEndpoint("PUT", `${VARIABLES_URL}/${VAR_ENV}`, () => null)
		const wrapper = await mountEdit(VAR_ENV)

		await findButtonByText(wrapper, t("variable.actions.back")).trigger("click")

		expect(wrapper.emitted("close")).toHaveLength(1)
		expect.soft(calls).toHaveLength(0)
	})

	it("saves from its header once something was changed", async ({ expect }) => {
		mockTree(fullTree())
		const calls = mockEndpoint("PUT", `${VARIABLES_URL}/${VAR_ENV}`, () => null)
		const wrapper = await mountEdit(VAR_ENV)
		await field(wrapper, t("variable.variable-form.name-label")).setValue(
			"Stage",
		)

		await findButtonByText(wrapper, t("variable.actions.save")).trigger("click")

		await waitForValidation(() => {
			expect(wrapper.emitted("close")).toHaveLength(1)
		})
		expect.soft(calls).toHaveLength(1)
		expect.soft(calls[0]?.body).toMatchObject({ name: "Stage", key: "env" })
	})

	it("swaps back for cancel and create once something was entered", async ({
		expect,
	}) => {
		const wrapper = await mountNew()
		const before = headerButtonTexts(wrapper)

		await field(wrapper, t("variable.variable-form.name-label")).setValue(
			"Severity",
		)

		expect(before).toEqual([t("variable.actions.back")])
		expect(headerButtonTexts(wrapper)).toEqual([
			t("variable.actions.cancel"),
			t("variable.actions.create"),
		])
	})

	it("drops the changes and goes back from cancel", async ({ expect }) => {
		const calls = mockEndpoint("PUT", `${VARIABLES_URL}/${VAR_ENV}`, () => null)
		const wrapper = await mountEdit(VAR_ENV)
		await field(wrapper, t("variable.variable-form.name-label")).setValue(
			"Stage",
		)

		await headerButton(wrapper, t("variable.actions.cancel")).trigger("click")

		expect(wrapper.emitted("close")).toHaveLength(1)
		expect.soft(calls).toHaveLength(0)
	})
})

function mountNew() {
	return mountSuspended(VariableForm, {
		props: { tree: fullTree(), variableId: null, groupId: GROUP_DEPLOYS },
	})
}

function mountEdit(variableId: string) {
	return mountSuspended(VariableForm, {
		props: { tree: fullTree(), variableId: variableId, groupId: null },
	})
}

// a field is found through the label pointing at it
function field(wrapper: VueWrapper, label: string) {
	const forId = wrapper
		.findAll("label")
		.find((l) => l.text() === label)
		?.attributes("for")
	if (!forId) {
		throw new Error(`no label "${label}"`)
	}

	return wrapper.get(`[id="${forId}"]`)
}

function keyInput(wrapper: VueWrapper) {
	return field(wrapper, t("variable.variable-form.key-label"))
}

function typeButton(wrapper: VueWrapper, label: string) {
	return findButtonByText(wrapper, label)
}

async function addOption(wrapper: VueWrapper, value: string) {
	const input = wrapper
		.findComponent(OptionsField)
		.get(`input[placeholder="${t("variable.variable-form.add-option")}"]`)

	await input.setValue(value)
	await input.trigger("keydown", { key: "Enter" })
}

function defaultFieldShape(wrapper: VueWrapper) {
	return {
		options: wrapper.findComponent(OptionsField).exists(),
		timeRange: wrapper.findComponent(TimeRangeSelect).exists(),
		textArea: wrapper.find("textarea").exists(),
		number: wrapper.find("input[inputmode='numeric']").exists(),
	}
}

function defaultShapeFor(type: string) {
	return {
		options: type === t("general.variable-types.list"),
		timeRange: type === t("general.variable-types.time-range"),
		textArea: type === t("general.variable-types.text"),
		number: type === t("general.variable-types.number"),
	}
}

// a submit runs vee-validate's validation first, on its own scheduler,
// and nothing signals when that is done
function waitForValidation(check: () => void) {
	return vi.waitFor(check, WAIT_FOR_OPTIONS)
}

function headerButton(wrapper: VueWrapper, label: string) {
	return findButtonByText(wrapper.findComponent(FormHeader), label)
}

function headerButtonTexts(wrapper: VueWrapper) {
	return (wrapper.findComponent(FormHeader) as VueWrapper)
		.findAll("button")
		.map((b) => b.text())
}
