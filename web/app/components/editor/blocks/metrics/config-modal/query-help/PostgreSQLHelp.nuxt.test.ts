import { mountSuspended } from "@nuxt/test-utils/runtime"
import { describe, it } from "vitest"
import PostgreSQLHelp from "./PostgreSQLHelp.vue"
import { t } from "~/components/test-helpers"

describe("<PostgreSQLHelp>", () => {
	it("explains what the query should return", async ({ expect }) => {
		const wrapper = await mountSuspended(PostgreSQLHelp)

		expect(wrapper.text()).toContain(
			t(
				"editor.metrics.config.query-explanations.postgresql.main-placeholders.postgresql-queries",
			),
		)
	})

	it("shows the expected column names as code", async ({ expect }) => {
		const wrapper = await mountSuspended(PostgreSQLHelp)

		expect(wrapper.findAll("code").map((c) => c.text())).toContain(
			t(
				"editor.metrics.config.query-explanations.postgresql.main-placeholders.time-column",
			),
		)
	})

	it("links to the official documentation", async ({ expect }) => {
		const wrapper = await mountSuspended(PostgreSQLHelp)

		const link = wrapper.get("a")

		expect(link.text()).toBe(
			t(
				"editor.metrics.config.query-explanations.postgresql.main-placeholders.docs-link",
			),
		)
		expect(link.attributes("href")).toBe(
			useRuntimeConfig().public.postgresqlQueryGuideURL,
		)
		expect(link.attributes("rel")).toBe("noopener noreferrer")
	})

	it("lists a single-series and a multi-series example", async ({ expect }) => {
		const wrapper = await mountSuspended(PostgreSQLHelp)

		expect(wrapper.text()).toContain(
			t("editor.metrics.config.query-explanations.postgresql.examples-title"),
		)
		expect(wrapper.findAll("li")).toHaveLength(2)
		expect(wrapper.text()).toContain(
			t(
				"editor.metrics.config.query-explanations.postgresql.examples.example-1.title",
			),
		)
		expect(wrapper.text()).toContain(
			t(
				"editor.metrics.config.query-explanations.postgresql.examples.example-2.title",
			),
		)
	})
})
