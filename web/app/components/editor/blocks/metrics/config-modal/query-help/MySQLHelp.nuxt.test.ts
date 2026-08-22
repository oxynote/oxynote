import { mountSuspended } from "@nuxt/test-utils/runtime"
import { describe, it } from "vitest"
import MySQLHelp from "./MySQLHelp.vue"
import { t } from "~/components/test-helpers"

describe("<MySQLHelp>", () => {
	it("explains what the query should return", async ({ expect }) => {
		const wrapper = await mountSuspended(MySQLHelp)

		expect(wrapper.text()).toContain(
			t(
				"editor.metrics.config.query-explanations.mysql.main-placeholders.mysql-queries",
			),
		)
	})

	it("shows the expected column names as code", async ({ expect }) => {
		const wrapper = await mountSuspended(MySQLHelp)

		expect(wrapper.findAll("code").map((c) => c.text())).toContain(
			t(
				"editor.metrics.config.query-explanations.mysql.main-placeholders.time-column",
			),
		)
	})

	it("links to the official documentation", async ({ expect }) => {
		const wrapper = await mountSuspended(MySQLHelp)

		const link = wrapper.get("a")

		expect(link.text()).toBe(
			t(
				"editor.metrics.config.query-explanations.mysql.main-placeholders.docs-link",
			),
		)
		expect(link.attributes("href")).toBe(
			useRuntimeConfig().public.mysqlQueryGuideURL,
		)
		expect(link.attributes("rel")).toBe("noopener noreferrer")
	})

	it("lists a single-series and a multi-series example", async ({ expect }) => {
		const wrapper = await mountSuspended(MySQLHelp)

		expect(wrapper.text()).toContain(
			t("editor.metrics.config.query-explanations.mysql.examples-title"),
		)
		expect(wrapper.findAll("li")).toHaveLength(2)
		expect(wrapper.text()).toContain(
			t(
				"editor.metrics.config.query-explanations.mysql.examples.example-1.title",
			),
		)
		expect(wrapper.text()).toContain(
			t(
				"editor.metrics.config.query-explanations.mysql.examples.example-2.title",
			),
		)
	})
})
