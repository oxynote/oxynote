import { flushPromises } from "@vue/test-utils"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import { toast } from "vue-sonner"
import {
	clearQueryCache,
	disposeMockEndpoints,
	seedQueryData,
} from "~/composables/api/test-helpers"
import DataSourceSection from "./DataSourceSection.vue"
import {
	at,
	clearTeleportedOverlays,
	menuItem,
	mountUnderTooltipProvider,
	raisedToasts,
	seedAuthOrganization,
	seedAuthSession,
	t,
} from "../test-helpers"

vi.mock("vue-sonner", () => ({
	toast: { custom: vi.fn(), dismiss: vi.fn() },
}))

function makeDataSource(overrides: Partial<DataSource> = {}): DataSource {
	return {
		id: "ds1".padEnd(20, "0"),
		name: "Prod metrics",
		type: DataSourceType.Prometheus,
		url: "http://prometheus.test",
		status: DataSourceStatus.Success,
		createdAt: "2026-01-01T00:00:00Z",
		updatedAt: null,
		...overrides,
	}
}

function seedDataSources(dataSources: DataSource[]) {
	seedQueryData(["data-sources", "list"], dataSources)
}

async function mountSection() {
	const wrapper = await mountUnderTooltipProvider(DataSourceSection, {})

	return { wrapper, section: wrapper.findComponent(DataSourceSection) }
}

// the query cache is app-wide and the options menus are teleported into
// the shared <body>
describe("<DataSourceSection>", { concurrent: false }, () => {
	beforeEach(clearQueryCache)

	afterEach(disposeMockEndpoints)

	it("offers all seven data source types", async ({ expect }) => {
		seedDataSources([])

		const { wrapper } = await mountSection()

		expect(wrapper.text()).toContain(
			t("settings.data-sources.prometheus.title"),
		)
		expect(wrapper.text()).toContain(t("settings.data-sources.loki.title"))
		expect(wrapper.text()).toContain(
			t("settings.data-sources.victorialogs.title"),
		)
		expect(wrapper.text()).toContain(
			t("settings.data-sources.postgresql.title"),
		)
		expect(wrapper.text()).toContain(t("settings.data-sources.supabase.title"))
		expect(wrapper.text()).toContain(t("settings.data-sources.mysql.title"))
		expect(wrapper.text()).toContain(t("settings.data-sources.mariadb.title"))
	})

	it("describes a type that has nothing connected yet", async ({ expect }) => {
		seedDataSources([])

		const { wrapper } = await mountSection()

		expect(wrapper.text()).toContain("Connect your Prometheus server")
	})

	it("lists the connected data sources of a type", async ({ expect }) => {
		seedDataSources([makeDataSource()])

		const { wrapper } = await mountSection()

		expect(wrapper.text()).toContain("Prod metrics")
		expect(wrapper.text()).toContain("http://prometheus.test")
	})

	it("keeps a data source out of another type's list", async ({ expect }) => {
		seedDataSources([
			makeDataSource({ name: "Analytics", type: DataSourceType.PostgreSQL }),
		])

		const { wrapper } = await mountSection()

		expect(wrapper.text()).toContain("Analytics")
		expect(wrapper.text()).toContain("Connect your Prometheus server")
	})

	it("hides a data source that is still being inserted", async ({ expect }) => {
		seedDataSources([
			makeDataSource({
				name: "Pending",
				status: DataSourceStatus.LocalOptimisticInsert,
			}),
		])

		const { wrapper } = await mountSection()

		expect(wrapper.text()).not.toContain("Pending")
	})

	it.for([
		{ index: 0, type: DataSourceType.Prometheus },
		{ index: 1, type: DataSourceType.Loki },
		{ index: 2, type: DataSourceType.VictoriaLogs },
		{ index: 3, type: DataSourceType.PostgreSQL },
		{ index: 4, type: DataSourceType.Supabase },
		{ index: 5, type: DataSourceType.MySQL },
		{ index: 6, type: DataSourceType.MariaDB },
	])(
		"asks to connect a new $type data source",
		async ({ index, type }, { expect }) => {
			seedDataSources([])
			const { wrapper, section } = await mountSection()

			await at(wrapper.findAll("button"), index).trigger("click")

			expect(section.emitted("data-source-creation")).toEqual([[type]])
		},
	)

	it.for([
		DataSourceType.Loki,
		DataSourceType.VictoriaLogs,
		DataSourceType.PostgreSQL,
		DataSourceType.Supabase,
		DataSourceType.MySQL,
		DataSourceType.MariaDB,
	])(
		"asks to update a %s data source picked from its options menu",
		async (type, { expect }) => {
			const dataSource = makeDataSource({ type: type })
			seedDataSources([dataSource])
			const { wrapper, section } = await mountSection()
			await wrapper.get("[data-slot='dropdown-menu-trigger']").trigger("click")

			menuItem(t(`settings.data-sources.${type}.options.update.title`)).click()
			await nextTick()

			expect(section.emitted("data-source-update")).toEqual([[dataSource]])
		},
	)

	it.for([
		DataSourceType.Loki,
		DataSourceType.VictoriaLogs,
		DataSourceType.PostgreSQL,
		DataSourceType.Supabase,
		DataSourceType.MySQL,
		DataSourceType.MariaDB,
	])(
		"asks to remove a %s data source picked from its options menu",
		async (type, { expect }) => {
			const dataSource = makeDataSource({ type: type })
			seedDataSources([dataSource])
			const { wrapper, section } = await mountSection()
			await wrapper.get("[data-slot='dropdown-menu-trigger']").trigger("click")

			menuItem(t(`settings.data-sources.${type}.options.delete.title`)).click()
			await nextTick()

			expect(section.emitted("data-source-removal")).toEqual([[dataSource]])
		},
	)

	it("asks to update the data source picked from its options menu", async ({
		expect,
	}) => {
		const dataSource = makeDataSource()
		seedDataSources([dataSource])
		const { wrapper, section } = await mountSection()
		await wrapper.get("[data-slot='dropdown-menu-trigger']").trigger("click")

		menuItem(
			t(
				`settings.data-sources.${DataSourceType.Prometheus}.options.update.title`,
			),
		).click()
		await nextTick()

		expect(section.emitted("data-source-update")).toEqual([[dataSource]])
	})

	it("asks to remove the data source picked from its options menu", async ({
		expect,
	}) => {
		const dataSource = makeDataSource()
		seedDataSources([dataSource])
		const { wrapper, section } = await mountSection()
		await wrapper.get("[data-slot='dropdown-menu-trigger']").trigger("click")

		menuItem(
			t(
				`settings.data-sources.${DataSourceType.Prometheus}.options.delete.title`,
			),
		).click()
		await nextTick()

		expect(section.emitted("data-source-removal")).toEqual([[dataSource]])
	})

	it("copies a data source's URL from its options menu", async ({ expect }) => {
		vi.mocked(toast.custom).mockReset()
		const writeText = vi
			.spyOn(navigator.clipboard, "writeText")
			.mockResolvedValue(undefined)
		seedDataSources([makeDataSource()])
		const { wrapper } = await mountSection()
		await wrapper.get("[data-slot='dropdown-menu-trigger']").trigger("click")

		menuItem(t("settings.data-sources.copy-url")).click()
		await flushPromises()

		expect(writeText).toHaveBeenCalledExactlyOnceWith("http://prometheus.test")
		expect(raisedToasts()).toMatchObject([
			{ type: "success", title: t("settings.data-sources.url-copied") },
		])
	})

	it("shows when the data source was connected", async ({ expect }) => {
		seedDataSources([makeDataSource()])

		const { wrapper } = await mountSection()

		expect(wrapper.text()).toContain("Jan 1, 2026")
	})

	describe("for a member who is not an admin", { concurrent: false }, () => {
		beforeEach(() => {
			clearTeleportedOverlays()
			seedAuthSession({ id: "u1" })
			seedAuthOrganization({ members: [{ userId: "u1", role: "editor" }] })
		})

		it("lists the connections without the controls that change them", async ({
			expect,
		}) => {
			seedDataSources([makeDataSource()])

			const { wrapper } = await mountSection()

			expect.soft(wrapper.text()).toContain("Prod metrics")
			// only the options menu, which holds nothing that changes the source
			expect.soft(wrapper.findAll("button")).toHaveLength(1)
			// the six types other than prometheus have nothing connected
			expect
				.soft(wrapper.text().split(t("settings.data-sources.not-connected")))
				.toHaveLength(7)
		})

		it("offers only the URL copy in a data source's options menu", async ({
			expect,
		}) => {
			seedDataSources([makeDataSource()])
			const { wrapper } = await mountSection()

			await wrapper.get("[data-slot='dropdown-menu-trigger']").trigger("click")

			expect(
				Array.from(
					document.body.querySelectorAll("[role='menu'] [role^='menuitem']"),
				).map((el) => el.textContent.trim()),
			).toEqual([t("settings.data-sources.copy-url")])
		})
	})
})
