import { mountSuspended } from "@nuxt/test-utils/runtime"
import type { VueWrapper } from "@vue/test-utils"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import DataSourceSelect from "./DataSourceSelect.vue"
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
const LOKI_ID = makeXid("dsc")

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
		{
			id: LOKI_ID,
			name: "Prod Loki",
			url: "https://loki.test",
			type: DataSourceType.Loki,
			status: DataSourceStatus.Success,
		},
	]
}

function configWith(dataSourceId: string | null) {
	return { dataSourceId: dataSourceId }
}

function mountSelect(props: Record<string, unknown> = {}) {
	return mountSuspended(DataSourceSelect, {
		props: { types: [DataSourceType.Prometheus], ...props },
	})
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
			t("editor.config-modal-utils.data-source-label-missing"),
		)
		expect(wrapper.text()).toContain(
			t("editor.config-modal-utils.data-source-description-missing"),
		)
	})

	it("says only that none is set in read mode", async ({ expect }) => {
		mockEndpoint("GET", "/api/data-sources", () => dataSources())
		useEditorMeta().setEditable(false)

		const wrapper = await mountSelect({ modelValue: configWith(null) })

		expect(wrapper.text()).toContain(
			t("editor.config-modal-utils.data-source-label-missing"),
		)
		expect(wrapper.text()).not.toContain(
			t("editor.config-modal-utils.data-source-description-missing"),
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
				t("editor.config-modal-utils.data-source-label-deleted"),
			)
		}, WAIT_FOR_OPTIONS)
	})

	it("treats a data source of a type it does not accept as none set", async ({
		expect,
	}) => {
		mockEndpoint("GET", "/api/data-sources", () => dataSources())

		const wrapper = await mountSelect({ modelValue: configWith(LOKI_ID) })

		await vi.waitFor(() => {
			expect(wrapper.text()).toContain(
				t("editor.config-modal-utils.data-source-label-missing"),
			)
		}, WAIT_FOR_OPTIONS)
		expect(wrapper.text()).not.toContain("Prod Loki")
		expect(wrapper.text()).not.toContain(
			t("editor.config-modal-utils.data-source-label-deleted"),
		)
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

	it("offers every connected data source of the accepted types", async ({
		expect,
	}) => {
		mockEndpoint("GET", "/api/data-sources", () => dataSources())
		const wrapper = await mountSelect({ modelValue: configWith(null) })
		await vi.waitFor(() => {
			expect(wrapper.text()).toContain(
				t("editor.config-modal-utils.data-source-label-missing"),
			)
		}, WAIT_FOR_OPTIONS)

		await openMenu(wrapper)

		await vi.waitFor(() => {
			expect(menuItem("Prod Prometheus")).toBeDefined()
		}, WAIT_FOR_OPTIONS)
		expect(menuItem("Broken").getAttribute("data-disabled")).not.toBeNull()
		expect(document.body.textContent).not.toContain("Prod Loki")
	})

	it("offers only the data sources of the types it is given", async ({
		expect,
	}) => {
		mockEndpoint("GET", "/api/data-sources", () => dataSources())
		const wrapper = await mountSelect({
			modelValue: configWith(null),
			types: [DataSourceType.Loki],
		})
		await vi.waitFor(() => {
			expect(wrapper.text()).toContain(
				t("editor.config-modal-utils.data-source-label-missing"),
			)
		}, WAIT_FOR_OPTIONS)

		await openMenu(wrapper)

		await vi.waitFor(() => {
			expect(menuItem("Prod Loki")).toBeDefined()
		}, WAIT_FOR_OPTIONS)
		expect(document.body.textContent).not.toContain("Prod Prometheus")
		expect(document.body.textContent).not.toContain("Broken")
	})

	it("points at the settings when no data source has an accepted type", async ({
		expect,
	}) => {
		mockEndpoint("GET", "/api/data-sources", () => dataSources())
		const wrapper = await mountSelect({
			modelValue: configWith(null),
			types: [DataSourceType.MySQL],
		})
		await vi.waitFor(() => {
			expect(wrapper.text()).toContain(
				t("editor.config-modal-utils.data-source-label-missing"),
			)
		}, WAIT_FOR_OPTIONS)

		await openMenu(wrapper)

		expect(document.body.textContent).toContain(
			t("editor.config-modal-utils.data-source-no-options.placeholder"),
		)
		expect(document.body.textContent).not.toContain("Prod Prometheus")
	})

	it("stores the data source the reader picked", async ({ expect }) => {
		mockEndpoint("GET", "/api/data-sources", () => dataSources())
		const config = configWith(null)
		const wrapper = await mountSelect({ modelValue: config })
		await vi.waitFor(() => {
			expect(wrapper.text()).toContain(
				t("editor.config-modal-utils.data-source-label-missing"),
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
				t("editor.config-modal-utils.data-source-no-options.placeholder"),
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
