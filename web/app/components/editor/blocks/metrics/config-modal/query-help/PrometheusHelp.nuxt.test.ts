import { mountSuspended } from "@nuxt/test-utils/runtime"
import { describe, it } from "vitest"
import PrometheusHelp from "./PrometheusHelp.vue"
import { t } from "~/components/test-helpers"

describe("<PrometheusHelp>", () => {
	it("explains what a prometheus query is", async ({ expect }) => {
		const wrapper = await mountSuspended(PrometheusHelp)

		expect(wrapper.text()).toContain(
			t(
				"editor.metrics.config.query-explanations.prometheus.main-placeholders.prometheus-queries",
			),
		)
	})

	it("shows the query syntax pieces as code", async ({ expect }) => {
		const wrapper = await mountSuspended(PrometheusHelp)

		expect(wrapper.findAll("code").map((c) => c.text())).toContain(
			t(
				"editor.metrics.config.query-explanations.prometheus.main-placeholders.rate-function",
			),
		)
	})

	it("links to the official documentation", async ({ expect }) => {
		const wrapper = await mountSuspended(PrometheusHelp)

		const link = wrapper.get("a")

		expect(link.text()).toBe(
			t(
				"editor.metrics.config.query-explanations.prometheus.main-placeholders.official-prometheus-docs",
			),
		)
		expect(link.attributes("href")).toBe(
			useRuntimeConfig().public.prometheusQueryGuideURL,
		)
		expect(link.attributes("rel")).toBe("noopener noreferrer")
	})

	it("lists both worked examples", async ({ expect }) => {
		const wrapper = await mountSuspended(PrometheusHelp)

		expect(wrapper.text()).toContain(
			t("editor.metrics.config.query-explanations.prometheus.examples-title"),
		)
		expect(wrapper.findAll("li")).toHaveLength(2)
	})
})
