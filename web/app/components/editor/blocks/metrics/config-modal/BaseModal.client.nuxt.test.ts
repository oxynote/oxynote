import { mountSuspended } from "@nuxt/test-utils/runtime"
import { enableAutoUnmount } from "@vue/test-utils"
import { afterEach, beforeEach, describe, it } from "vitest"
import BaseModal from "./BaseModal.client.vue"
import ConfigSidebar from "./ConfigSidebar.vue"
import { stubChartColorContext } from "../test-helpers"
import {
	defaultMetricConfig,
	RefreshInterval,
	TimeRangePreset,
	type MetricConfig,
} from "../utils"
import {
	clearQueryCache,
	disposeMockEndpoints,
	makeXid,
	mockEndpoint,
} from "~/composables/api/test-helpers"
import {
	emitFrom,
	stubViewportMatches,
	t,
	teleportedButton,
} from "~/components/test-helpers"
import { DiffStatus } from "~/components/editor/diff/position-map"

const DOCUMENT_ID = makeXid("doc")
const BRANCH_ID = makeXid("branch")

let uidCounter = 0

function nextUid(): string {
	uidCounter++

	return `metric-modal-${uidCounter}`
}

// the modal reads everything it shows out of the editor store, which is
// where the metric block publishes its config
function activateBlock(
	options: {
		config?: MetricConfig
		diffStatus?: DiffStatus
		oldConfig?: MetricConfig | null
	} = {},
): string {
	const uid = nextUid()
	const store = useEditorStore()

	store.updateActiveDocumentId(DOCUMENT_ID)
	store.updateActiveBranchId(BRANCH_ID)
	store.setMetricBlockConfig(uid, options.config ?? defaultMetricConfig())

	if (options.diffStatus) {
		store.setMetricBlockDiffInfo(
			uid,
			options.diffStatus,
			options.oldConfig ?? null,
		)
	}

	store.activateMetricBlockConfig(uid)

	return uid
}

function mountModal() {
	return mountSuspended(BaseModal)
}

function dialogText(): string {
	return document.body.querySelector("[role='dialog']")?.textContent ?? ""
}

function dialogOpen(): boolean {
	return document.body.querySelector("[role='dialog']") !== null
}

// the editor store, the editable flag, the viewport stub and the
// teleported dialog body are all shared, so these tests cannot interleave
describe("<MetricConfigBaseModal>", { concurrent: false }, () => {
	// the dialog body is teleported into <body> and vue keeps patching it
	// for as long as the modal is mounted, so each test's modal is taken
	// down properly rather than ripped out of the dom
	enableAutoUnmount(afterEach)

	beforeEach(() => {
		clearQueryCache()
		stubChartColorContext()
		stubViewportMatches(true)
		mockEndpoint("GET", "/api/data-sources", () => [])
		useEditorMeta().setEditable(true)
		useEditorStore().setReviewableDiffActive(false)
		useEditorStore().activateMetricBlockConfig(null)
	})

	afterEach(disposeMockEndpoints)

	it("stays shut while no block is being configured", async ({ expect }) => {
		await mountModal()

		expect(dialogOpen()).toBe(false)
	})

	it("opens for the block being configured", async ({ expect }) => {
		activateBlock()

		await mountModal()

		expect(dialogOpen()).toBe(true)
		expect(dialogText()).toContain(
			t("editor.metrics.config.modal-title-normal"),
		)
	})

	it("titles itself as read-only in read mode", async ({ expect }) => {
		useEditorMeta().setEditable(false)
		activateBlock()

		await mountModal()

		expect(dialogText()).toContain(
			t("editor.metrics.config.modal-title-readonly"),
		)
	})

	it.for([
		{
			status: DiffStatus.Added,
			expectedKey: "editor.metrics.config.modal-title-diff-added",
		},
		{
			status: DiffStatus.Removed,
			expectedKey: "editor.metrics.config.modal-title-diff-removed",
		},
		{
			status: DiffStatus.Modified,
			expectedKey: "editor.metrics.config.modal-title-diff-modified",
		},
	])(
		"titles itself for a $status block in a diff",
		async ({ status, expectedKey }, { expect }) => {
			activateBlock({ diffStatus: status })

			await mountModal()

			expect(dialogText()).toContain(t(expectedKey))
		},
	)

	it("shows the block's time range and refresh interval", async ({
		expect,
	}) => {
		activateBlock({
			config: {
				...defaultMetricConfig(),
				timeRange: TimeRangePreset.Last24Hours,
				refreshInterval: RefreshInterval.S30,
			},
		})

		await mountModal()

		expect(dialogText()).toContain(
			t("editor.metrics.config.time-range-options.last_24_hours"),
		)
		expect(dialogText()).toContain(
			t("editor.metrics.config.refresh-interval-options-short.30s"),
		)
	})

	it("shows the time range a diff replaced beside the new one", async ({
		expect,
	}) => {
		activateBlock({
			config: {
				...defaultMetricConfig(),
				timeRange: TimeRangePreset.Last24Hours,
			},
			diffStatus: DiffStatus.Modified,
			oldConfig: {
				...defaultMetricConfig(),
				timeRange: TimeRangePreset.Last1Hour,
			},
		})

		await mountModal()

		expect(dialogText()).toContain(
			t("editor.metrics.config.time-range-options.last_24_hours"),
		)
		expect(dialogText()).toContain(
			t("editor.metrics.config.time-range-options.last_1_hour"),
		)
	})

	it("shows the refresh interval a diff replaced beside the new one", async ({
		expect,
	}) => {
		activateBlock({
			config: {
				...defaultMetricConfig(),
				refreshInterval: RefreshInterval.S30,
			},
			diffStatus: DiffStatus.Modified,
			oldConfig: {
				...defaultMetricConfig(),
				refreshInterval: RefreshInterval.H1,
			},
		})

		await mountModal()

		expect(dialogText()).toContain(
			t("editor.metrics.config.refresh-interval-options-short.30s"),
		)
		expect(dialogText()).toContain(
			t("editor.metrics.config.refresh-interval-options-short.1h"),
		)
	})

	it("shows the settings only once on a wide screen", async ({ expect }) => {
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
		await nextTick()

		expect(useEditorStore().activeMetricBlockConfig).toBeNull()
	})

	it("stands aside when the reader asks for the data source settings", async ({
		expect,
	}) => {
		activateBlock()
		const wrapper = await mountModal()

		emitFrom(wrapper, ConfigSidebar, "open-settings")
		await nextTick()

		expect(useEditorStore().activeMetricBlockConfig).toBeNull()
		expect(wrapper.emitted("open-settings")).toHaveLength(1)
	})
})
