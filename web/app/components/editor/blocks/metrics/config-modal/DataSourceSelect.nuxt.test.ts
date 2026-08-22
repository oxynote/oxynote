import { mountSuspended } from "@nuxt/test-utils/runtime"
import type { VueWrapper } from "@vue/test-utils"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import DataSourceSelect from "./DataSourceSelect.vue"
import { stubChartColorContext } from "../test-helpers"
import { defaultMetricConfig, type MetricConfig } from "../utils"
import {
	clearQueryCache,
	disposeMockEndpoints,
	makeXid,
	mockEndpoint,
} from "~/composables/api/test-helpers"
import {
	clearTeleportedOverlays,
	menuItem,
	t,
	WAIT_FOR_OPTIONS,
} from "~/components/test-helpers"
import { DiffStatus } from "~/components/editor/diff/position-map"

const PROMETHEUS_ID = makeXid("dsa")
const BROKEN_ID = makeXid("dsb")

function dataSources() {
	return [
		{
			id: PROMETHEUS_ID,
			name: "Prod Prometheus",
			url: "https://prom.test",
			type: DataSourceType.Prometheus,
			status: DataSourceStatus.Success,
		},
		{
			id: BROKEN_ID,
			name: "Broken",
			url: "https://broken.test",
			type: DataSourceType.Prometheus,
			status: DataSourceStatus.Unreachable,
		},
	]
}

function configWith(dataSourceId: string | null): MetricConfig {
	return { ...defaultMetricConfig(), dataSourceId: dataSourceId }
}

function mountSelect(props: Record<string, unknown> = {}) {
	return mountSuspended(DataSourceSelect, { props: props })
}

async function openMenu(wrapper: VueWrapper) {
	await wrapper
		.get("[data-slot='dropdown-menu-trigger']")
		.trigger("pointerdown", {
			button: 0,
		})
	await wrapper.get("[data-slot='dropdown-menu-trigger']").trigger("click")
	await nextTick()
}

// the editable flag is a shared cookie state, the editor store and the
// query cache are app-wide, and the menu body is teleported into a shared
// <body>, so these tests cannot interleave
describe("<DataSourceSelect>", { concurrent: false }, () => {
	beforeEach(() => {
		clearTeleportedOverlays()
		clearQueryCache()
		stubChartColorContext()
		useEditorMeta().setEditable(true)
		useEditorStore().setReviewableDiffActive(false)
	})

	afterEach(disposeMockEndpoints)

	it("invites the reader to pick a data source when none is set", async ({
		expect,
	}) => {
		mockEndpoint("GET", "/api/data-sources", () => dataSources())

		const wrapper = await mountSelect({ modelValue: configWith(null) })

		expect(wrapper.text()).toContain(
			t("editor.metrics.config.data-source-label-missing"),
		)
		expect(wrapper.text()).toContain(
			t("editor.metrics.config.data-source-description-missing"),
		)
	})

	it("says only that none is set in read mode", async ({ expect }) => {
		mockEndpoint("GET", "/api/data-sources", () => dataSources())
		useEditorMeta().setEditable(false)

		const wrapper = await mountSelect({ modelValue: configWith(null) })

		expect(wrapper.text()).toContain(
			t("editor.metrics.config.data-source-label-missing"),
		)
		expect(wrapper.text()).not.toContain(
			t("editor.metrics.config.data-source-description-missing"),
		)
	})

	it("names the chosen data source and its address", async ({ expect }) => {
		mockEndpoint("GET", "/api/data-sources", () => dataSources())

		const wrapper = await mountSelect({
			modelValue: configWith(PROMETHEUS_ID),
		})

		await vi.waitFor(() => {
			expect(wrapper.text()).toContain("Prod Prometheus")
		}, WAIT_FOR_OPTIONS)
		expect(wrapper.text()).toContain("https://prom.test")
	})

	it("reports a data source that no longer exists", async ({ expect }) => {
		mockEndpoint("GET", "/api/data-sources", () => [])

		const wrapper = await mountSelect({
			modelValue: configWith(makeXid("gone")),
		})

		await vi.waitFor(() => {
			expect(wrapper.text()).toContain(
				t("editor.metrics.config.data-source-label-deleted"),
			)
		}, WAIT_FOR_OPTIONS)
	})

	it("names the data source of the version a diff replaced", async ({
		expect,
	}) => {
		mockEndpoint("GET", "/api/data-sources", () => dataSources())

		const wrapper = await mountSelect({
			oldConfig: configWith(PROMETHEUS_ID),
		})

		await vi.waitFor(() => {
			expect(wrapper.text()).toContain("Prod Prometheus")
		}, WAIT_FOR_OPTIONS)
	})

	it.for([
		{ status: DiffStatus.Added, expected: "bg-diff-field-added" },
		{ status: DiffStatus.Removed, expected: "bg-diff-field-removed" },
	])(
		"tints a $status data source field",
		async ({ status, expected }, { expect }) => {
			mockEndpoint("GET", "/api/data-sources", () => dataSources())

			const wrapper = await mountSelect({
				modelValue: configWith(null),
				diffStatus: status,
			})

			expect(
				wrapper.get("[data-slot='dropdown-menu-trigger']").classes(),
			).toContain(expected)
		},
	)

	it("offers every connected data source", async ({ expect }) => {
		mockEndpoint("GET", "/api/data-sources", () => dataSources())
		const wrapper = await mountSelect({ modelValue: configWith(null) })
		await vi.waitFor(() => {
			expect(wrapper.text()).toContain(
				t("editor.metrics.config.data-source-label-missing"),
			)
		}, WAIT_FOR_OPTIONS)

		await openMenu(wrapper)

		await vi.waitFor(() => {
			expect(menuItem("Prod Prometheus")).toBeDefined()
		}, WAIT_FOR_OPTIONS)
		expect(menuItem("Broken").getAttribute("data-disabled")).not.toBeNull()
	})

	it("stores the data source the reader picked", async ({ expect }) => {
		mockEndpoint("GET", "/api/data-sources", () => dataSources())
		const config = configWith(null)
		const wrapper = await mountSelect({ modelValue: config })
		await vi.waitFor(() => {
			expect(wrapper.text()).toContain(
				t("editor.metrics.config.data-source-label-missing"),
			)
		}, WAIT_FOR_OPTIONS)
		await openMenu(wrapper)
		await vi.waitFor(() => {
			expect(menuItem("Prod Prometheus")).toBeDefined()
		}, WAIT_FOR_OPTIONS)

		menuItem("Prod Prometheus").click()
		await nextTick()

		expect(config.dataSourceId).toBe(PROMETHEUS_ID)
	})

	it("points at the settings when there is nothing to pick", async ({
		expect,
	}) => {
		mockEndpoint("GET", "/api/data-sources", () => [])
		const wrapper = await mountSelect({ modelValue: configWith(null) })

		await openMenu(wrapper)

		const link = Array.from(
			document.body.querySelectorAll<HTMLButtonElement>("button"),
		).find((b) =>
			b.textContent.includes(
				t("editor.metrics.config.data-source-no-options.placeholder"),
			),
		)
		link?.click()
		await nextTick()

		expect(wrapper.emitted("open-settings")).toHaveLength(1)
	})

	it("offers no menu in read mode", async ({ expect }) => {
		mockEndpoint("GET", "/api/data-sources", () => dataSources())
		useEditorMeta().setEditable(false)

		const wrapper = await mountSelect({ modelValue: configWith(null) })

		expect(
			wrapper.get("[data-slot='dropdown-menu-trigger']").attributes("disabled"),
		).toBeDefined()
	})
})
