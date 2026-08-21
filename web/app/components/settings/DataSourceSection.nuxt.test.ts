import { afterEach, beforeEach, describe, it } from "vitest"
import {
	clearQueryCache,
	disposeMockEndpoints,
	seedQueryData,
} from "~/composables/api/test-helpers"
import DataSourceSection from "./DataSourceSection.vue"
import { at, menuItem, mountUnderTooltipProvider } from "../test-helpers"

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

	it("offers all four data source types", async ({ expect }) => {
		seedDataSources([])

		const { wrapper } = await mountSection()

		expect(wrapper.text()).toContain("Prometheus")
		expect(wrapper.text()).toContain("PostgreSQL")
		expect(wrapper.text()).toContain("MySQL")
		expect(wrapper.text()).toContain("MariaDB")
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
		{ index: 1, type: DataSourceType.PostgreSQL },
		{ index: 2, type: DataSourceType.MySQL },
		{ index: 3, type: DataSourceType.MariaDB },
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
		DataSourceType.PostgreSQL,
		DataSourceType.MySQL,
		DataSourceType.MariaDB,
	])(
		"asks to update a %s data source picked from its options menu",
		async (type, { expect }) => {
			const dataSource = makeDataSource({ type: type })
			seedDataSources([dataSource])
			const { wrapper, section } = await mountSection()
			await wrapper.get("[data-slot='dropdown-menu-trigger']").trigger("click")

			menuItem("Update").click()
			await nextTick()

			expect(section.emitted("data-source-update")).toEqual([[dataSource]])
		},
	)

	it.for([
		DataSourceType.PostgreSQL,
		DataSourceType.MySQL,
		DataSourceType.MariaDB,
	])(
		"asks to remove a %s data source picked from its options menu",
		async (type, { expect }) => {
			const dataSource = makeDataSource({ type: type })
			seedDataSources([dataSource])
			const { wrapper, section } = await mountSection()
			await wrapper.get("[data-slot='dropdown-menu-trigger']").trigger("click")

			menuItem("Remove").click()
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

		menuItem("Update").click()
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

		menuItem("Remove").click()
		await nextTick()

		expect(section.emitted("data-source-removal")).toEqual([[dataSource]])
	})

	it("shows when the data source was connected", async ({ expect }) => {
		seedDataSources([makeDataSource()])

		const { wrapper } = await mountSection()

		expect(wrapper.text()).toContain("Jan 1, 2026")
	})
})
