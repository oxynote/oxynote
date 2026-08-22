import { mountSuspended } from "@nuxt/test-utils/runtime"
import { describe, it } from "vitest"
import HelpPopoverContent from "./HelpPopoverContent.vue"
import { t } from "~/components/test-helpers"

function mountHelp(dataSourceType: DataSourceType) {
	return mountSuspended(HelpPopoverContent, {
		props: { dataSourceType: dataSourceType },
	})
}

describe("<HelpPopoverContent>", () => {
	it.for([
		{
			type: DataSourceType.Prometheus,
			expectedKey:
				"editor.metrics.config.query-explanations.prometheus.main-placeholders.prometheus-queries",
		},
		{
			type: DataSourceType.PostgreSQL,
			expectedKey:
				"editor.metrics.config.query-explanations.postgresql.main-placeholders.postgresql-queries",
		},
		{
			type: DataSourceType.MySQL,
			expectedKey:
				"editor.metrics.config.query-explanations.mysql.main-placeholders.mysql-queries",
		},
		{
			type: DataSourceType.MariaDB,
			expectedKey:
				"editor.metrics.config.query-explanations.mariadb.main-placeholders.mariadb-queries",
		},
	])("explains $type queries", async ({ type, expectedKey }, { expect }) => {
		const wrapper = await mountHelp(type)

		expect(wrapper.text()).toContain(t(expectedKey))
	})

	it("explains only the data source it was given", async ({ expect }) => {
		const wrapper = await mountHelp(DataSourceType.MySQL)

		expect(wrapper.text()).not.toContain(
			t(
				"editor.metrics.config.query-explanations.prometheus.main-placeholders.prometheus-queries",
			),
		)
		expect(wrapper.text()).not.toContain(
			t(
				"editor.metrics.config.query-explanations.mariadb.main-placeholders.mariadb-queries",
			),
		)
	})
})
