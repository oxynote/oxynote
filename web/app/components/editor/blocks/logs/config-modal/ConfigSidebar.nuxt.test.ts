import { mountSuspended } from "@nuxt/test-utils/runtime"
import type { DOMWrapper, VueWrapper } from "@vue/test-utils"
import { afterEach, beforeEach, describe, it } from "vitest"
import ConfigSidebar from "./ConfigSidebar.vue"
import ConfigField from "../../config-modal-utils/ConfigField.vue"
import DataSourceSelect from "../../config-modal-utils/DataSourceSelect.vue"
import TimestampFormatSelect from "./TimestampFormatSelect.vue"
import OrderSelect from "./OrderSelect.vue"
import {
	defaultLogConfig,
	LogOrder,
	LogTimestampFormat,
	type LogConfig,
} from "../utils"
import {
	at,
	clearTeleportedOverlays,
	emitFrom,
	t,
} from "~/components/test-helpers"
import {
	clearQueryCache,
	disposeMockEndpoints,
	makeXid,
	mockEndpoint,
} from "~/composables/api/test-helpers"
import { DiffStatus } from "~/components/editor/diff/position-map"

const NEW_DATA_SOURCE_ID = makeXid("dsnew")
const OLD_DATA_SOURCE_ID = makeXid("dsold")

// the editable flag is a shared cookie state, the editor store and the
// query cache are app-wide, and the menus are teleported into a shared
// <body>, so these tests cannot interleave
describe("<ConfigSidebar>", { concurrent: false }, () => {
	beforeEach(() => {
		clearTeleportedOverlays()
		clearQueryCache()
		mockEndpoint("GET", "/api/data-sources", () => [])
		useEditorMeta().setEditable(true)
		useEditorStore().setReviewableDiffActive(false)
	})

	afterEach(disposeMockEndpoints)

	it("labels every setting it offers", async ({ expect }) => {
		const wrapper = await mountSidebar()

		expect(wrapper.text()).toContain(t("editor.logs.config.title-label"))
		expect(wrapper.text()).toContain(
			t("editor.logs.config.timestamp-format-label"),
		)
		expect(wrapper.text()).toContain(t("editor.logs.config.order-label"))
	})

	it("hands the data source picker the config outside a diff", async ({
		expect,
	}) => {
		const config = configWith({ dataSourceId: NEW_DATA_SOURCE_ID })

		const wrapper = await mountSidebar({ modelValue: config })

		const selects = wrapper.findAllComponents(DataSourceSelect)

		expect(selects).toHaveLength(1)
		expect(at(selects, 0).props("modelValue")).toEqual(config)
		expect(at(selects, 0).props("diffMode")).toBe(false)
		expect(at(selects, 0).props("diffStatus")).toBeNull()
		expect(at(selects, 0).props("types")).toEqual([DataSourceType.Loki])
	})

	it("asks the host to open the data source settings", async ({ expect }) => {
		const wrapper = await mountSidebar()

		emitFrom(wrapper, DataSourceSelect, "open-settings")
		await nextTick()

		expect(wrapper.emitted("open-settings")).toHaveLength(1)
	})

	it("shows the title the config carries", async ({ expect }) => {
		const wrapper = await mountSidebar({
			modelValue: configWith({ title: "API errors" }),
		})

		const inputs = titleInputs(wrapper)

		expect(inputs).toHaveLength(1)
		expect(inputValue(at(inputs, 0))).toBe("API errors")
		expect(at(inputs, 0).attributes("readonly")).toBeUndefined()
	})

	it("stores a title the reader typed", async ({ expect }) => {
		const config = configWith()
		const wrapper = await mountSidebar({ modelValue: config })

		await at(titleInputs(wrapper), 0).setValue("API errors")

		expect(config.title).toBe("API errors")
	})

	it("prompts for a title while it is empty", async ({ expect }) => {
		const wrapper = await mountSidebar()

		expect(wrapper.classes()).not.toContain("pb-2")
		expect(at(titleInputs(wrapper), 0).attributes("placeholder")).toBe(
			t("editor.logs.config.title-placeholder"),
		)
	})

	it("reports an empty title as empty in read mode", async ({ expect }) => {
		useEditorMeta().setEditable(false)

		const wrapper = await mountSidebar()

		const input = at(titleInputs(wrapper), 0)

		expect(wrapper.classes()).toContain("pb-2")
		expect(input.attributes("placeholder")).toBe(
			t("editor.logs.config.title-empty-value-placeholder"),
		)
		expect(input.attributes("readonly")).toBeDefined()
	})

	it("reports an empty title as empty while a reviewable diff is shown", async ({
		expect,
	}) => {
		useEditorStore().setReviewableDiffActive(true)

		const wrapper = await mountSidebar()

		const input = at(titleInputs(wrapper), 0)

		expect(wrapper.classes()).toContain("pb-2")
		expect(input.attributes("placeholder")).toBe(
			t("editor.logs.config.title-empty-value-placeholder"),
		)
		expect(input.attributes("readonly")).toBeDefined()
	})

	it("shows the timestamp format the config carries", async ({ expect }) => {
		const wrapper = await mountSidebar({
			modelValue: configWith({
				timestampFormat: LogTimestampFormat.DetailedMilli,
			}),
		})

		const selects = wrapper.findAllComponents(TimestampFormatSelect)

		expect(selects).toHaveLength(1)
		expect(at(selects, 0).props("modelValue")).toBe(
			LogTimestampFormat.DetailedMilli,
		)
		expect(at(selects, 0).props("diffStatus")).toBeNull()
	})

	it("stores the timestamp format the reader picked", async ({ expect }) => {
		const config = configWith()
		const wrapper = await mountSidebar({ modelValue: config })

		emitFrom(
			wrapper,
			TimestampFormatSelect,
			"update:modelValue",
			LogTimestampFormat.DetailedNano,
		)
		await nextTick()

		expect(config.timestampFormat).toBe(LogTimestampFormat.DetailedNano)
	})

	it("shows the order the config carries", async ({ expect }) => {
		const wrapper = await mountSidebar({
			modelValue: configWith({ order: LogOrder.OldestFirst }),
		})

		const select = wrapper.getComponent(OrderSelect)

		expect(select.props("modelValue")).toBe(LogOrder.OldestFirst)
		expect(select.props("isModified")).toBe(false)
		expect(select.props("oldOrder")).toBeNull()
	})

	it("stores the order the reader picked", async ({ expect }) => {
		const config = configWith()
		const wrapper = await mountSidebar({ modelValue: config })

		emitFrom(wrapper, OrderSelect, "update:modelValue", LogOrder.OldestFirst)
		await nextTick()

		expect(config.order).toBe(LogOrder.OldestFirst)
	})

	describe("in a diff", () => {
		it("shows the data source a diff replaced beside the new one", async ({
			expect,
		}) => {
			const config = configWith({ dataSourceId: NEW_DATA_SOURCE_ID })
			const oldConfig = configWith({ dataSourceId: OLD_DATA_SOURCE_ID })

			const wrapper = await mountSidebar({
				modelValue: config,
				diffStatus: DiffStatus.Modified,
				oldConfig: oldConfig,
			})

			const selects = wrapper.findAllComponents(DataSourceSelect)

			expect(selects).toHaveLength(2)
			expect(at(selects, 0).props("modelValue")).toEqual(config)
			expect(at(selects, 0).props("diffMode")).toBe(true)
			expect(at(selects, 0).props("diffStatus")).toBe(DiffStatus.Added)
			expect(at(selects, 1).props("modelValue")).toBeUndefined()
			expect(at(selects, 1).props("oldConfig")).toEqual(oldConfig)
			expect(at(selects, 1).props("diffMode")).toBe(true)
			expect(at(selects, 1).props("diffStatus")).toBe(DiffStatus.Removed)
			expect(at(selects, 0).props("types")).toEqual([DataSourceType.Loki])
			expect(at(selects, 1).props("types")).toEqual([DataSourceType.Loki])
		})

		it("shows the title a diff replaced beside the new one", async ({
			expect,
		}) => {
			const wrapper = await mountSidebar({
				modelValue: configWith({ title: "After" }),
				diffStatus: DiffStatus.Modified,
				oldConfig: configWith({ title: "Before" }),
			})

			const inputs = titleInputs(wrapper)

			expect(inputs).toHaveLength(2)
			expect(inputValue(at(inputs, 0))).toBe("After")
			expect(at(inputs, 0).classes()).toContain("bg-diff-field-added")
			expect(at(inputs, 0).attributes("readonly")).toBeDefined()
			expect(inputValue(at(inputs, 1))).toBe("Before")
			expect(at(inputs, 1).classes()).toContain("bg-diff-field-removed")
			expect(at(inputs, 1).attributes("readonly")).toBeDefined()
			expect(at(inputs, 1).attributes("placeholder")).toBe(
				t("editor.logs.config.title-empty-value-placeholder"),
			)
		})

		it("shows the timestamp format a diff replaced beside the new one", async ({
			expect,
		}) => {
			const wrapper = await mountSidebar({
				modelValue: configWith({
					timestampFormat: LogTimestampFormat.Detailed,
				}),
				diffStatus: DiffStatus.Modified,
				oldConfig: configWith({
					timestampFormat: LogTimestampFormat.Standard,
				}),
			})

			const selects = wrapper.findAllComponents(TimestampFormatSelect)

			expect(selects).toHaveLength(2)
			expect(at(selects, 0).props("modelValue")).toBe(
				LogTimestampFormat.Detailed,
			)
			expect(at(selects, 0).props("diffStatus")).toBe(DiffStatus.Added)
			expect(at(selects, 1).props("modelValue")).toBe(
				LogTimestampFormat.Standard,
			)
			expect(at(selects, 1).props("diffStatus")).toBe(DiffStatus.Removed)
		})

		it("shows the order a diff replaced", async ({ expect }) => {
			const wrapper = await mountSidebar({
				modelValue: configWith({ order: LogOrder.OldestFirst }),
				diffStatus: DiffStatus.Modified,
				oldConfig: configWith({ order: LogOrder.NewestFirst }),
			})

			const select = wrapper.getComponent(OrderSelect)

			expect(select.props("modelValue")).toBe(LogOrder.OldestFirst)
			expect(select.props("isModified")).toBe(true)
			expect(select.props("oldOrder")).toBe(LogOrder.NewestFirst)
		})

		it.for([
			{
				name: "leaves the fields a modification kept unmarked",
				input: () => ({
					diffStatus: DiffStatus.Modified,
					oldConfig: configWith({ dataSourceId: NEW_DATA_SOURCE_ID }),
				}),
			},
			{
				name: "compares nothing for a block the diff added",
				input: () => ({
					diffStatus: DiffStatus.Added,
					oldConfig: changedConfig(),
				}),
			},
			{
				name: "compares nothing for a modification without the old config",
				input: () => ({ diffStatus: DiffStatus.Modified, oldConfig: null }),
			},
		])("$name", async ({ input }, { expect }) => {
			const wrapper = await mountSidebar({
				modelValue: configWith({ dataSourceId: NEW_DATA_SOURCE_ID }),
				...input(),
			})

			expect(diffMarks(wrapper)).toEqual({
				dataSources: [{ diffMode: true, diffStatus: null }],
				titles: [{ tints: [], readonly: false }],
				formats: [null],
				order: { isModified: false, oldOrder: null },
			})
		})
	})
})

function configWith(overrides: Partial<LogConfig> = {}): LogConfig {
	return { ...defaultLogConfig(), ...overrides }
}

// differs from configWith({ dataSourceId: NEW_DATA_SOURCE_ID }) in every
// field the sidebar compares
function changedConfig(): LogConfig {
	return configWith({
		title: "Before",
		dataSourceId: OLD_DATA_SOURCE_ID,
		timestampFormat: LogTimestampFormat.DetailedNano,
		order: LogOrder.OldestFirst,
	})
}

function mountSidebar(props: Record<string, unknown> = {}) {
	return mountSuspended(ConfigSidebar, {
		props: { modelValue: configWith(), ...props },
	})
}

// the fields are labelled through ConfigField's label slot, which is the
// only stable way to tell one input from the next
function titleInputs(wrapper: VueWrapper): DOMWrapper<HTMLInputElement>[] {
	const label = t("editor.logs.config.title-label")
	const field = wrapper
		.findAllComponents(ConfigField)
		.find((f) => f.text().startsWith(label))
	if (!field) {
		throw new Error(`no config field labelled "${label}"`)
	}

	return field.findAll("input")
}

function inputValue(input: DOMWrapper<HTMLInputElement>): string {
	return input.element.value
}

// how each field shows the diff: one entry per copy of the field
function diffMarks(wrapper: VueWrapper) {
	const order = wrapper.getComponent(OrderSelect)

	return {
		dataSources: wrapper.findAllComponents(DataSourceSelect).map((s) => ({
			diffMode: s.props("diffMode") as boolean,
			diffStatus: s.props("diffStatus") as DiffStatus | null,
		})),
		titles: titleInputs(wrapper).map((input) => ({
			tints: input.classes().filter((c) => c.startsWith("bg-diff-field-")),
			readonly: input.attributes("readonly") !== undefined,
		})),
		formats: wrapper
			.findAllComponents(TimestampFormatSelect)
			.map((s) => s.props("diffStatus") as DiffStatus | null),
		order: {
			isModified: order.props("isModified") as boolean,
			oldOrder: order.props("oldOrder") as LogOrder | null,
		},
	}
}
