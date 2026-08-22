import { mountSuspended } from "@nuxt/test-utils/runtime"
import { describe, it } from "vitest"
import MariaDBHelp from "./MariaDBHelp.vue"
import { t } from "~/components/test-helpers"

describe("<MariaDBHelp>", () => {
	it("explains what the query should return", async ({ expect }) => {
		const wrapper = await mountSuspended(MariaDBHelp)

		expect(wrapper.text()).toContain(
			t(
				"editor.metrics.config.query-explanations.mariadb.main-placeholders.mariadb-queries",
			),
		)
	})

	it("shows the expected column names as code", async ({ expect }) => {
		const wrapper = await mountSuspended(MariaDBHelp)

		expect(wrapper.findAll("code").map((c) => c.text())).toContain(
			t(
				"editor.metrics.config.query-explanations.mariadb.main-placeholders.time-column",
			),
		)
	})

	it("links to the official documentation", async ({ expect }) => {
		const wrapper = await mountSuspended(MariaDBHelp)

		const link = wrapper.get("a")

		expect(link.text()).toBe(
			t(
				"editor.metrics.config.query-explanations.mariadb.main-placeholders.docs-link",
			),
		)
		expect(link.attributes("href")).toBe(
			useRuntimeConfig().public.mariadbQueryGuideURL,
		)
		expect(link.attributes("rel")).toBe("noopener noreferrer")
	})

	it("lists a single-series and a multi-series example", async ({ expect }) => {
		const wrapper = await mountSuspended(MariaDBHelp)

		expect(wrapper.text()).toContain(
			t("editor.metrics.config.query-explanations.mariadb.examples-title"),
		)
		expect(wrapper.findAll("li")).toHaveLength(2)
		expect(wrapper.text()).toContain(
			t(
				"editor.metrics.config.query-explanations.mariadb.examples.example-1.title",
			),
		)
		expect(wrapper.text()).toContain(
			t(
				"editor.metrics.config.query-explanations.mariadb.examples.example-2.title",
			),
		)
	})
})
