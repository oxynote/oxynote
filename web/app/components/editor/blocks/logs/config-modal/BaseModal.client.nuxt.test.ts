import { mockNuxtImport, mountSuspended } from "@nuxt/test-utils/runtime"
import { enableAutoUnmount, flushPromises } from "@vue/test-utils"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import BaseModal from "./BaseModal.client.vue"
import BottomConfig from "./BottomConfig.vue"
import ConfigSidebar from "./ConfigSidebar.vue"
import DataView from "../view/DataView.vue"
import { defaultLogConfig, LogTimestampFormat, type LogConfig } from "../utils"
import {
	clearQueryCache,
	disposeMockEndpoints,
	makeXid,
	mockEndpoint,
	seedQueryData,
} from "~/composables/api/test-helpers"
import { VARIABLE_QUERY_KEYS } from "~/composables/api/useVariableAPI"
import {
	emitFrom,
	stubViewportMatches,
	t,
	teleportedButton,
} from "~/components/test-helpers"
import { DiffStatus } from "~/components/editor/diff/position-map"
import { RefreshInterval, TimeRangePreset } from "~/utils/time-range"
import {
	METRIC_REFRESH_INTERVAL_REFERENCE,
	METRIC_TIME_RANGE_REFERENCE,
} from "~/utils/variable"

// happy-dom lays nothing out, so the real scroller would show no row
vi.mock("vue-virtual-scroller", async () =>
	(await import("../test-helpers")).virtualScrollerStub(),
)

const DOCUMENT_ID = makeXid("doc")
const BRANCH_ID = makeXid("branch")
const BLOCK_UID = "log-block"

// happy-dom lays nothing out, so the time controls never learn their
// width. The tests set it by hand.
const timeOptions = vi.hoisted(() => ({
	width: null as unknown as { value: number },
}))
mockNuxtImport("useElementSize", () => () => ({ width: timeOptions.width }))

// the editor store, the editable flag, the viewport stub, the measured
// width and the teleported dialog body are all shared, so these tests
// cannot interleave
describe("<LogConfigBaseModal>", { concurrent: false }, () => {
	// the dialog body is teleported into <body> and vue keeps patching it
	// for as long as the modal is mounted, so each test's modal is taken
	// down properly rather than ripped out of the dom
	enableAutoUnmount(afterEach)

	beforeEach(() => {
		timeOptions.width = ref(0)
		clearQueryCache()
		seedQueryData(VARIABLE_QUERY_KEYS.tree, [])
		stubViewportMatches(true)
		mockEndpoint("GET", "/api/data-sources", () => [])
		useEditorMeta().setEditable(true)

		const store = useEditorStore()

		// leaving the document drops the configs an earlier test stored
		store.updateActiveDocumentId(null)
		store.updateActiveDocumentId(DOCUMENT_ID)
		store.updateActiveBranchId(BRANCH_ID)
		store.setReviewableDiffActive(false)
		store.activateLogBlockConfig(null)
	})

	afterEach(disposeMockEndpoints)

	it("stays shut while no block is being configured", async ({ expect }) => {
		await mountModal()

		expect(dialog()).toBeNull()
	})

	it("opens for the block being configured", async ({ expect }) => {
		activateBlock()

		const wrapper = await mountModal()

		expect(dialogTitle()).toBe(t("editor.logs.config.modal-title-normal"))
		expect(dialogDescription()).toBe(
			t("editor.logs.config.modal-description-normal"),
		)
		expect(wrapper.findComponent(ConfigSidebar).exists()).toBe(true)
		expect(wrapper.findComponent(BottomConfig).exists()).toBe(true)
	})

	it("titles itself as read-only in read mode", async ({ expect }) => {
		useEditorMeta().setEditable(false)
		activateBlock()

		await mountModal()

		expect(dialogTitle()).toBe(t("editor.logs.config.modal-title-readonly"))
		expect(dialogDescription()).toBe(
			t("editor.logs.config.modal-description-readonly"),
		)
	})

	it("titles itself as read-only while a reviewable diff is shown", async ({
		expect,
	}) => {
		useEditorStore().setReviewableDiffActive(true)
		activateBlock()

		await mountModal()

		expect(dialogTitle()).toBe(t("editor.logs.config.modal-title-readonly"))
		expect(dialogDescription()).toBe(
			t("editor.logs.config.modal-description-readonly"),
		)
	})

	it.for([
		{
			input: DiffStatus.Unchanged,
			expected: "editor.logs.config.modal-title-diff-unchanged",
		},
		{
			input: DiffStatus.Added,
			expected: "editor.logs.config.modal-title-diff-added",
		},
		{
			input: DiffStatus.Removed,
			expected: "editor.logs.config.modal-title-diff-removed",
		},
		{
			input: DiffStatus.Modified,
			expected: "editor.logs.config.modal-title-diff-modified",
		},
	])(
		"titles itself for a $input block in a diff",
		async ({ input, expected }, { expect }) => {
			useEditorStore().setReviewableDiffActive(true)
			activateBlock({ diffStatus: input })

			await mountModal()

			expect(dialogTitle()).toBe(t(expected))
			expect(dialogDescription()).toBe(
				t("editor.logs.config.modal-description-readonly"),
			)
		},
	)

	it("shows no settings while the block has no stored config", async ({
		expect,
	}) => {
		useEditorStore().activateLogBlockConfig(BLOCK_UID)

		const wrapper = await mountModal()

		expect(dialogTitle()).toBe(t("editor.logs.config.modal-title-normal"))
		expect(wrapper.findComponent(ConfigSidebar).exists()).toBe(false)
		expect(wrapper.findComponent(BottomConfig).exists()).toBe(false)
	})

	it("shows no settings without an active branch", async ({ expect }) => {
		activateBlock()
		useEditorStore().updateActiveBranchId(null)

		const wrapper = await mountModal()

		expect(dialog()).not.toBeNull()
		expect(wrapper.findComponent(ConfigSidebar).exists()).toBe(false)
		expect(wrapper.findComponent(BottomConfig).exists()).toBe(false)
	})

	it("hands the block's config to the settings and the queries", async ({
		expect,
	}) => {
		const config = configWith({ title: "API errors" })
		activateBlock({ config: config })

		const wrapper = await mountModal()

		const sidebar = wrapper.getComponent(ConfigSidebar)
		const bottom = wrapper.getComponent(BottomConfig)

		expect(sidebar.props()).toEqual({
			modelValue: config,
			diffStatus: null,
			oldConfig: null,
		})
		expect(bottom.props()).toEqual({
			modelValue: config,
			diffStatus: null,
			oldConfig: null,
		})
	})

	it.for([
		{ name: "wide", input: true },
		{ name: "narrow", input: false },
	])(
		"hands the diff to the settings and the queries on a $name screen",
		async ({ input }, { expect }) => {
			stubViewportMatches(input)
			const config = configWith({ title: "After" })
			const oldConfig = configWith({ title: "Before" })
			useEditorStore().setReviewableDiffActive(true)
			activateBlock({
				config: config,
				diffStatus: DiffStatus.Modified,
				oldConfig: oldConfig,
			})

			const wrapper = await mountModal()

			const sidebar = wrapper.getComponent(ConfigSidebar)
			const bottom = wrapper.getComponent(BottomConfig)

			expect(sidebar.props()).toEqual({
				modelValue: config,
				diffStatus: DiffStatus.Modified,
				oldConfig: oldConfig,
			})
			expect(bottom.props()).toEqual({
				modelValue: config,
				diffStatus: DiffStatus.Modified,
				oldConfig: oldConfig,
			})
		},
	)

	it("previews the block's lines without the view's own action button", async ({
		expect,
	}) => {
		activateBlock({ config: configWith({ title: "API errors" }) })

		await mountModal()

		expect(dialog()?.querySelector(".text-2base")?.textContent).toBe(
			"API errors",
		)
		expect(
			dialog()?.querySelector("[data-slot='empty-title']")?.textContent,
		).toBe(t("editor.logs.status.data-source-not-selected.title"))
		expect(dialog()?.textContent).not.toContain(
			t("editor.logs.status.data-source-not-selected.normal-action-button"),
		)
	})

	it("keeps the preview's header clear of the time controls", async ({
		expect,
	}) => {
		timeOptions.width.value = 160
		activateBlock()

		await mountModal()

		expect(
			dialog()?.querySelector<HTMLElement>(".text-2base")?.parentElement?.style
				.paddingRight,
		).toBe("calc(10rem + 0.125rem)")
	})

	it("shows the block's time range and refresh interval", async ({
		expect,
	}) => {
		activateBlock({
			config: configWith({
				timeRange: TimeRangePreset.Last24Hours,
				refreshInterval: RefreshInterval.S30,
			}),
		})

		await mountModal()

		const timeRange = t("general.time-range-options.last_24_hours")
		const refreshInterval = t("general.refresh-interval-options-short.30s")

		expect(teleportedButton(timeRange).textContent.trim()).toBe(timeRange)
		expect(teleportedButton(refreshInterval).textContent.trim()).toBe(
			refreshInterval,
		)
	})

	it.for([
		{
			name: "stores the time range picked in the modal on the block",
			trigger: () => t("general.time-range-options.last_24_hours"),
			option: () => t("editor.config-modal-utils.variable-time-range"),
			expected: {
				timeRange: METRIC_TIME_RANGE_REFERENCE,
				refreshInterval: RefreshInterval.S30,
			},
		},
		{
			name: "stores the refresh interval picked in the modal on the block",
			trigger: () => t("general.refresh-interval-options-short.30s"),
			option: () => t("editor.config-modal-utils.variable-refresh-interval"),
			expected: {
				timeRange: TimeRangePreset.Last24Hours,
				refreshInterval: METRIC_REFRESH_INTERVAL_REFERENCE,
			},
		},
	])("$name", async ({ trigger, option, expected }, { expect }) => {
		activateBlock({
			config: configWith({
				timeRange: TimeRangePreset.Last24Hours,
				refreshInterval: RefreshInterval.S30,
			}),
		})
		await mountModal()
		await openSetting(trigger())

		const item = Array.from(
			document.body.querySelectorAll("[role='option'], button"),
		).find((element) => element.textContent.trim() === option())
		if (item instanceof HTMLButtonElement) {
			item.click()
		} else {
			item?.dispatchEvent(
				new KeyboardEvent("keydown", { key: "Enter", bubbles: true }),
			)
		}
		// the item selects only after two ticks of its own
		await flushPromises()

		expect(storedConfig()?.timeRange).toBe(expected.timeRange)
		expect(storedConfig()?.refreshInterval).toBe(expected.refreshInterval)
	})

	it.for([
		{
			name: "lets the reader change both selects",
			input: () => undefined,
			expected: false,
		},
		{
			name: "disables both selects in read mode",
			input: () => {
				useEditorMeta().setEditable(false)
			},
			expected: true,
		},
		{
			name: "disables both selects while a reviewable diff is shown",
			input: () => {
				useEditorStore().setReviewableDiffActive(true)
			},
			expected: true,
		},
	])("$name", async ({ input, expected }, { expect }) => {
		input()
		activateBlock({
			config: configWith({
				timeRange: TimeRangePreset.Last24Hours,
				refreshInterval: RefreshInterval.S30,
			}),
		})

		await mountModal()

		expect(
			teleportedButton(
				t("general.time-range-options.last_24_hours"),
			).classList.contains("pointer-events-none"),
		).toBe(expected)
		expect(
			teleportedButton(
				t("general.refresh-interval-options-short.30s"),
			).classList.contains("pointer-events-none"),
		).toBe(expected)
	})

	it.for([
		{
			name: "one row on a wide screen with no changed time",
			input: { wide: true, changed: false },
			expected: { header: undefined, options: "h-7" },
		},
		{
			name: "two rows on a wide screen with a changed time",
			input: { wide: true, changed: true },
			expected: { header: "h-12", options: "h-11" },
		},
		{
			name: "two rows on a narrow screen with no changed time",
			input: { wide: false, changed: false },
			expected: { header: "h-12", options: "h-11" },
		},
		{
			name: "four rows on a narrow screen with a changed time",
			input: { wide: false, changed: true },
			expected: { header: "h-22", options: "h-21" },
		},
	])(
		"sizes the preview's header for $name",
		async ({ input, expected }, { expect }) => {
			stubViewportMatches(input.wide)
			activateBlock({
				config: configWith({ timeRange: TimeRangePreset.Last24Hours }),
				diffStatus: DiffStatus.Modified,
				oldConfig: configWith({
					timeRange: input.changed
						? TimeRangePreset.Last1Hour
						: TimeRangePreset.Last24Hours,
				}),
			})

			const wrapper = await mountModal()

			expect(wrapper.findComponent(DataView).props("headerClass")).toBe(
				expected.header,
			)
			expect(
				teleportedButton(t("general.time-range-options.last_24_hours"))
					.closest(".absolute")
					?.classList.contains(expected.options),
			).toBe(true)
		},
	)

	it.for([
		{ name: "hides the preview's search while a diff is shown", input: true },
		{ name: "keeps the preview's search outside a diff", input: false },
	])("$name", async ({ input }, { expect }) => {
		useEditorStore().setReviewableDiffActive(input)
		activateBlock()

		const wrapper = await mountModal()

		expect(wrapper.findComponent(DataView).props("hideSearch")).toBe(input)
	})

	it("shows the old value only of the setting a modified diff changed", async ({
		expect,
	}) => {
		useEditorStore().setReviewableDiffActive(true)
		activateBlock({
			config: configWith({
				timeRange: TimeRangePreset.Last24Hours,
				refreshInterval: RefreshInterval.S30,
			}),
			diffStatus: DiffStatus.Modified,
			oldConfig: configWith({
				timeRange: TimeRangePreset.Last1Hour,
				refreshInterval: RefreshInterval.S30,
			}),
		})

		await mountModal()

		const refreshLabel = t("general.refresh-interval-options-short.30s")

		expect
			.soft(
				teleportedButton(t("general.time-range-options.last_24_hours"))
					.classList,
			)
			.toContain("bg-diff-field-added")
		expect
			.soft(
				teleportedButton(t("general.time-range-options.last_1_hour")).classList,
			)
			.toContain("bg-diff-field-removed")
		expect
			.soft(
				Array.from(document.body.querySelectorAll("button")).filter(
					(b) => b.textContent.trim() === refreshLabel,
				),
			)
			.toHaveLength(1)
		expect
			.soft(teleportedButton(refreshLabel).classList)
			.not.toContain("bg-diff-field-added")
	})

	it("stores a title typed in the modal on the block", async ({ expect }) => {
		activateBlock()
		const wrapper = await mountModal()

		await wrapper.getComponent(ConfigSidebar).get("input").setValue("Slow")

		expect(storedConfig()?.title).toBe("Slow")
	})

	it("shows the settings beside the queries on a wide screen", async ({
		expect,
	}) => {
		activateBlock()

		const wrapper = await mountModal()

		expect(
			document.body.querySelectorAll("[role='dialog'] .w-70"),
		).toHaveLength(1)
		expect(wrapper.findAllComponents(ConfigSidebar)).toHaveLength(1)
	})

	it("folds the settings into the body on a narrow screen", async ({
		expect,
	}) => {
		stubViewportMatches(false)
		activateBlock()

		const wrapper = await mountModal()

		expect(
			document.body.querySelectorAll("[role='dialog'] .w-70"),
		).toHaveLength(0)
		expect(wrapper.findAllComponents(ConfigSidebar)).toHaveLength(1)
	})

	it("closes when the reader dismisses it", async ({ expect }) => {
		activateBlock()
		await mountModal()

		teleportedButton(t("general.modal-close-screen-reader-hint")).click()
		await flushPromises()

		expect(useEditorStore().activeLogBlockConfig).toBeNull()
		expect(dialog()).toBeNull()
	})

	it.for([
		{ name: "wide", input: true },
		{ name: "narrow", input: false },
	])(
		"stands aside for the data source settings on a $name screen",
		async ({ input }, { expect }) => {
			stubViewportMatches(input)
			activateBlock()
			const wrapper = await mountModal()

			emitFrom(wrapper, ConfigSidebar, "open-settings")
			await nextTick()

			expect(useEditorStore().activeLogBlockConfig).toBeNull()
			expect(wrapper.emitted("open-settings")).toHaveLength(1)
		},
	)

	// the dialog starts listening for outside presses a tick after it
	// opens, and fake timers are app-wide
	describe("when the reader presses outside", { concurrent: false }, () => {
		beforeEach(() => {
			vi.useFakeTimers()
		})

		afterEach(() => {
			vi.useRealTimers()
		})

		it.for([
			{
				name: "stays open on a press in a toast",
				input: (area: HTMLElement) => {
					area.setAttribute("data-sonner-toaster", "")
				},
				expected: BLOCK_UID,
			},
			{
				name: "stays open on a press in the query suggestions",
				input: (area: HTMLElement) => {
					area.classList.add("cm-tooltip-autocomplete")
				},
				expected: BLOCK_UID,
			},
			{
				name: "closes on a press anywhere else",
				input: () => undefined,
				expected: null,
			},
		])("$name", async ({ input, expected }, { expect, onTestFinished }) => {
			activateBlock()
			await mountModal()
			await vi.advanceTimersByTimeAsync(0)
			const area = document.createElement("div")
			const target = document.createElement("span")
			input(area)
			area.append(target)
			document.body.append(area)
			onTestFinished(() => {
				area.remove()
			})

			target.dispatchEvent(new PointerEvent("pointerdown", { bubbles: true }))
			await vi.advanceTimersByTimeAsync(0)
			await nextTick()

			expect(useEditorStore().activeLogBlockConfig).toBe(expected)
		})
	})
})

function configWith(overrides: Partial<LogConfig> = {}): LogConfig {
	return {
		...defaultLogConfig(),
		timestampFormat: LogTimestampFormat.Numeric,
		order: LogOrder.Asc,
		...overrides,
	}
}

// the modal reads everything it shows out of the editor store, which is
// where the log block publishes its config
function activateBlock(
	options: {
		config?: LogConfig
		diffStatus?: DiffStatus
		oldConfig?: LogConfig | null
	} = {},
) {
	const store = useEditorStore()

	store.setLogBlockConfig(BLOCK_UID, options.config ?? configWith())

	if (options.diffStatus) {
		store.setLogBlockDiffInfo(
			BLOCK_UID,
			options.diffStatus,
			options.oldConfig ?? null,
		)
	}

	store.activateLogBlockConfig(BLOCK_UID)
}

function storedConfig(): LogConfig | undefined {
	return useEditorStore().logBlockConfigs[DOCUMENT_ID]?.[BRANCH_ID]?.[BLOCK_UID]
}

function mountModal() {
	return mountSuspended(BaseModal)
}

// the dialog is teleported into <body>, out of the wrapper's reach
function dialog(): HTMLElement | null {
	return document.body.querySelector("[role='dialog']")
}

function dialogTitle(): string {
	return labelText("aria-labelledby")
}

function dialogDescription(): string {
	return labelText("aria-describedby")
}

function labelText(attribute: string): string {
	const id = dialog()?.getAttribute(attribute)
	if (!id) {
		throw new Error(`the dialog has no ${attribute}`)
	}

	return document.getElementById(id)?.textContent.trim() ?? ""
}

async function openSetting(label: string) {
	const button = teleportedButton(label)
	if (button.getAttribute("aria-haspopup") === "dialog") {
		button.click()
	} else {
		button.dispatchEvent(
			new KeyboardEvent("keydown", { key: "Enter", bubbles: true }),
		)
	}
	await nextTick()
}
