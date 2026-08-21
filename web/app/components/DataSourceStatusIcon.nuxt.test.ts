import { mountSuspended } from "@nuxt/test-utils/runtime"
import { describe, it } from "vitest"
import DataSourceStatusIcon from "./DataSourceStatusIcon.vue"
import { renderedIconNames } from "./test-helpers"

function makeDataSource(
	type: DataSourceType,
	status: DataSourceStatus = DataSourceStatus.Success,
): DataSource {
	return {
		id: "ds-1",
		name: "Metrics",
		type: type,
		url: "http://localhost:9090",
		status: status,
		createdAt: "2026-01-01T00:00:00Z",
		updatedAt: null,
	}
}

describe("<DataSourceStatusIcon>", () => {
	it("renders a dashed placeholder with no icons when there is no data source", async ({
		expect,
	}) => {
		const wrapper = await mountSuspended(DataSourceStatusIcon, {
			props: { dataSource: null },
		})

		expect(wrapper.classes()).toContain("border-dashed")
		expect(renderedIconNames(wrapper)).toEqual([])
	})

	it.for([
		{ type: DataSourceType.Prometheus, expected: "devicon:prometheus" },
		{ type: DataSourceType.PostgreSQL, expected: "devicon:postgresql" },
		{ type: DataSourceType.MySQL, expected: "devicon:mysql" },
		{ type: DataSourceType.MariaDB, expected: "devicon:mariadb" },
	])("shows the $type logo", async ({ type, expected }, { expect }) => {
		const wrapper = await mountSuspended(DataSourceStatusIcon, {
			props: { dataSource: makeDataSource(type) },
		})

		expect(renderedIconNames(wrapper)[0]).toBe(expected)
	})

	it("falls back to the prometheus logo for a type this build does not know", async ({
		expect,
	}) => {
		const wrapper = await mountSuspended(DataSourceStatusIcon, {
			props: {
				dataSource: makeDataSource("clickhouse" as DataSourceType),
			},
		})

		expect(renderedIconNames(wrapper)[0]).toBe("devicon:prometheus")
	})

	it("marks a reachable data source with a green check badge", async ({
		expect,
	}) => {
		const wrapper = await mountSuspended(DataSourceStatusIcon, {
			props: { dataSource: makeDataSource(DataSourceType.Prometheus) },
		})

		expect(wrapper.classes()).toContain("border-status-success")
		expect(wrapper.html()).toContain("bg-status-success")
		expect(renderedIconNames(wrapper)).toContain("lucide:check")
	})

	it.for([
		DataSourceStatus.Unauthorized,
		DataSourceStatus.Unreachable,
		DataSourceStatus.VersionNotSupported,
		DataSourceStatus.NotReadOnly,
	])(
		"marks a data source in status %s with a red cross badge",
		async (status, { expect }) => {
			const wrapper = await mountSuspended(DataSourceStatusIcon, {
				props: {
					dataSource: makeDataSource(DataSourceType.Prometheus, status),
				},
			})

			expect(wrapper.classes()).toContain("border-status-error")
			expect(wrapper.html()).toContain("bg-status-error")
			expect(renderedIconNames(wrapper)).toContain("lucide:x")
		},
	)

	it("defaults to the large size", async ({ expect }) => {
		const wrapper = await mountSuspended(DataSourceStatusIcon, {
			props: { dataSource: makeDataSource(DataSourceType.Prometheus) },
		})

		expect(wrapper.classes()).toContain("size-8.5")
	})

	it("renders at the small size when asked", async ({ expect }) => {
		const wrapper = await mountSuspended(DataSourceStatusIcon, {
			props: {
				dataSource: makeDataSource(DataSourceType.Prometheus),
				size: "6.5",
			},
		})

		expect(wrapper.classes()).toContain("size-6.5")
	})
})
