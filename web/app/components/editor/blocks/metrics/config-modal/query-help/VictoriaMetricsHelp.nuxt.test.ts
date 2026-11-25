import { mountSuspended } from "@nuxt/test-utils/runtime"
import { describe, it } from "vitest"
import VictoriaMetricsHelp from "./VictoriaMetricsHelp.vue"
import { t } from "~/components/test-helpers"

describe("<VictoriaMetricsHelp>", () => {
	it("explains what a victoriametrics query is", async ({ expect }) => {
		const wrapper = await mountSuspended(VictoriaMetricsHelp)

		expect(wrapper.text()).toContain(
			t(
				"editor.metrics.config.query-explanations.victoriametrics.main-placeholders.metricsql-queries",
			),
		)
	})

	it("shows the query syntax pieces as code", async ({ expect }) => {
		const wrapper = await mountSuspended(VictoriaMetricsHelp)

		expect(wrapper.findAll("code").map((c) => c.text())).toContain(
			t(
				"editor.metrics.config.query-explanations.victoriametrics.main-placeholders.rate-function",
			),
		)
	})

	it("links to the official documentation", async ({ expect }) => {
		const wrapper = await mountSuspended(VictoriaMetricsHelp)

		const link = wrapper.get("a")

		expect(link.text()).toBe(
			t(
				"editor.metrics.config.query-explanations.victoriametrics.main-placeholders.official-docs",
			),
		)
		expect(link.attributes("href")).toBe(
			useRuntimeConfig().public.victoriametricsQueryGuideURL,
		)
		expect(link.attributes("rel")).toBe("noopener noreferrer")
	})

	it("lists both worked examples", async ({ expect }) => {
		const wrapper = await mountSuspended(VictoriaMetricsHelp)

		expect(wrapper.text()).toContain(
			t(
				"editor.metrics.config.query-explanations.victoriametrics.examples-title",
			),
		)
		expect(wrapper.findAll("li")).toHaveLength(2)
	})
})
