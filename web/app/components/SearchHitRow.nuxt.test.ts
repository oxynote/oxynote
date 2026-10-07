import { mountSuspended } from "@nuxt/test-utils/runtime"
import { afterEach, beforeEach, describe, it } from "vitest"
import {
	clearQueryCache,
	disposeMockEndpoints,
	mockEndpoint,
	seedQueryData,
} from "~/composables/api/test-helpers"
import type { DocumentSearchHitAttrs } from "~/utils"
import SearchHitRow from "./SearchHitRow.vue"
import { renderedIconNames, t } from "./test-helpers"

const HREF = "/Acme/Runbook-doc1#block-7"
const DATA_SOURCE = { id: "ds1", name: "prod-prom" }

// the data sources live in the app-wide query cache and the endpoint
// registry is shared, so the tests cannot interleave
describe("<SearchHitRow>", { concurrent: false }, () => {
	beforeEach(clearQueryCache)

	afterEach(disposeMockEndpoints)

	it("links to the block the hit is in", async ({ expect }) => {
		const wrapper = await mountRow({ type: "paragraph" })

		expect(wrapper.find("a").attributes("href")).toBe(HREF)
	})

	// what the sanitizer keeps of a highlight is covered by its own
	// browser tests, so the text here carries none
	it("shows the text of the block", async ({ expect }) => {
		const wrapper = await mountRow({
			type: "paragraph",
			text: "every shipment has a label",
		})

		expect(wrapper.text()).toBe("every shipment has a label")
	})

	it.for([
		{ input: "heading", expected: "lucide:heading" },
		{ input: "paragraph", expected: "lucide:text" },
		{ input: "codeBlock", expected: "lucide:square-code" },
		{ input: "codeBlockTitle", expected: "lucide:square-code" },
		{ input: "mermaidBlock", expected: "lucide:network" },
		{ input: "imageBlock", expected: "lucide:image" },
		{ input: "fileBlock", expected: "mingcute:attachment-line" },
	])(
		"marks a $input hit with its block's icon",
		async ({ input, expected }, { expect }) => {
			const wrapper = await mountRow({ type: input })

			expect(renderedIconNames(wrapper)).toEqual([expected])
		},
	)

	describe("a code hit", { concurrent: false }, () => {
		it("names the language of the block", async ({ expect }) => {
			const wrapper = await mountRow({
				type: "codeBlock",
				attrs: { language: "python" },
			})

			expect(wrapper.text()).toContain("Python")
		})

		it("names no language when the block has none", async ({ expect }) => {
			const wrapper = await mountRow({ type: "codeBlock", text: "POST /v1" })

			expect(wrapper.text()).toBe("POST /v1")
		})

		it("is set in a monospace face", async ({ expect }) => {
			const wrapper = await mountRow({ type: "codeBlock" })

			expect(wrapper.find(".font-mono").exists()).toBe(true)
		})
	})

	describe("a metric hit", { concurrent: false }, () => {
		it.for([
			{
				input: GenericQueryChartType.Line,
				icon: "lucide:chart-line",
				title: "editor.metrics.config.type-options.line-chart.title",
			},
			{
				input: GenericQueryChartType.Bar,
				icon: "lucide:bar-chart-3",
				title: "editor.metrics.config.type-options.bar-chart.title",
			},
			{
				input: GenericQueryChartType.Gauge,
				icon: "lucide:gauge",
				title: "editor.metrics.config.type-options.gauge-chart.title",
			},
		])(
			"names and draws the $input type",
			async ({ input, icon, title }, { expect }) => {
				seedQueryData(["data-sources", "list"], [])

				const wrapper = await mountRow({
					type: "metricBlock",
					attrs: { visualizationType: input },
				})

				expect(renderedIconNames(wrapper)).toEqual([icon])
				expect(wrapper.text()).toContain(t(title))
			},
		)

		it("counts as a line chart when the block names no type", async ({
			expect,
		}) => {
			seedQueryData(["data-sources", "list"], [])

			const wrapper = await mountRow({ type: "metricBlock" })

			expect(renderedIconNames(wrapper)).toEqual(["lucide:chart-line"])
			expect(wrapper.text()).toContain(
				t("editor.metrics.config.type-options.line-chart.title"),
			)
		})

		it("names the data source the block reads", async ({ expect }) => {
			seedQueryData(["data-sources", "list"], [DATA_SOURCE])

			const wrapper = await mountRow({
				type: "metricBlock",
				attrs: { dataSourceId: "ds1" },
			})

			expect(wrapper.text()).toContain(
				t("sidebar.search.metric-source", { source: "prod-prom" }),
			)
		})

		it("names no data source when the block's one is gone", async ({
			expect,
		}) => {
			seedQueryData(["data-sources", "list"], [DATA_SOURCE])

			const wrapper = await mountRow({
				type: "metricBlock",
				attrs: { dataSourceId: "ds2" },
			})

			expect(wrapper.text()).not.toContain("prod-prom")
		})
	})

	it("loads the data sources for a metric hit only", async ({ expect }) => {
		const listCalls = mockEndpoint("GET", "/api/data-sources", () => [])

		await mountRow({ type: "paragraph" })
		await mountRow({ type: "codeBlock" })
		expect(listCalls).toHaveLength(0)

		await mountRow({ type: "metricBlock" })

		expect(listCalls).toHaveLength(1)
	})
})

function mountRow(hit: {
	type: string
	text?: string
	attrs?: DocumentSearchHitAttrs
}) {
	return mountSuspended(SearchHitRow, {
		props: {
			hit: { id: "block-7", text: "a hit", ...hit },
			href: HREF,
		},
	})
}
