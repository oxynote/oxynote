import { afterEach, beforeEach, describe, it, vi } from "vitest"
import { Matcher } from "@prometheus-io/codemirror-promql/dist/esm/types"
import { EqlRegex, EqlSingle, Neq, NeqRegex } from "@prometheus-io/lezer-promql"
import { TimeRangePreset } from "~/components/editor/blocks/metrics/utils"
import { PrometheusDataSourceClient } from "./prometheus-utils"

const NOW = new Date("2026-01-02T03:00:00.000Z")
const FROM = new Date("2026-01-02T02:55:00.000Z") // NOW minus 5 minutes

const TIME_PARAMS = {
	from: FROM,
	to: NOW,
	timeRangeKey: TimeRangePreset.Last5Minutes,
}

function makeMatcher(name: string, value: string, type: number) {
	return new Matcher(type, name, value)
}

function makeClient(
	overrides: Partial<
		ConstructorParameters<typeof PrometheusDataSourceClient>[0]
	> = {},
) {
	const fns = {
		timeRangeFn: vi.fn().mockResolvedValue(TimeRangePreset.Last5Minutes),
		metricMetadataFn: vi.fn().mockResolvedValue({ result: {} }),
		labelNamesFn: vi.fn().mockResolvedValue({ result: [] }),
		labelValuesFn: vi.fn().mockResolvedValue({ result: [] }),
		seriesFn: vi.fn().mockResolvedValue({ result: [] }),
	}

	return {
		client: new PrometheusDataSourceClient({ ...fns, ...overrides }),
		fns,
	}
}

// the tests pin the global clock with fake timers — shared mutable state
// that per-test hooks would corrupt mid-flight — so they cannot interleave
describe("PrometheusDataSourceClient", { concurrent: false }, () => {
	// the client resolves its query window from the wall clock, so the
	// tests pin it to a fixed instant
	beforeEach(() => {
		vi.useFakeTimers()
		vi.setSystemTime(NOW)
	})

	afterEach(() => {
		vi.useRealTimers()
	})

	describe("labelNames", () => {
		it("queries label names for the resolved time range", async ({
			expect,
		}) => {
			const labelNamesFn = vi
				.fn()
				.mockResolvedValue({ result: ["job", "instance"] })
			const { client, fns } = makeClient({ labelNamesFn })

			const result = await client.labelNames()

			expect(result).toEqual(["job", "instance"])
			expect(fns.timeRangeFn).toHaveBeenCalledTimes(1)
			expect(labelNamesFn).toHaveBeenCalledExactlyOnceWith(TIME_PARAMS)
		})

		it("restricts label names to the given metric", async ({ expect }) => {
			const { client, fns } = makeClient()

			await client.labelNames("up")

			expect(fns.labelNamesFn).toHaveBeenCalledExactlyOnceWith({
				...TIME_PARAMS,
				matchers: ["up"],
			})
		})
	})

	describe("labelValues", () => {
		it("queries the values of a label without a metric", async ({ expect }) => {
			const labelValuesFn = vi.fn().mockResolvedValue({ result: ["a", "b"] })
			const { client, fns } = makeClient({ labelValuesFn })

			const result = await client.labelValues("job")

			expect(result).toEqual(["a", "b"])
			expect(fns.timeRangeFn).toHaveBeenCalledTimes(1)
			expect(labelValuesFn).toHaveBeenCalledExactlyOnceWith({
				...TIME_PARAMS,
				label: "job",
			})
		})

		it("matches on the bare metric when there are no matchers", async ({
			expect,
		}) => {
			const { client, fns } = makeClient()

			await client.labelValues("job", "up")

			expect(fns.labelValuesFn).toHaveBeenCalledExactlyOnceWith({
				...TIME_PARAMS,
				label: "job",
				matchers: ["up"],
			})
		})

		it("builds a selector from the matchers, skipping the queried label and empty values", async ({
			expect,
		}) => {
			const { client, fns } = makeClient()

			await client.labelValues("instance", "up", [
				makeMatcher("job", "api", EqlSingle),
				makeMatcher("env", "prod", Neq),
				makeMatcher("pod", "a.*", EqlRegex),
				makeMatcher("node", "b.*", NeqRegex),
				makeMatcher("empty", "", EqlSingle),
				makeMatcher("instance", "x", EqlSingle),
			])

			expect(fns.labelValuesFn).toHaveBeenCalledExactlyOnceWith({
				...TIME_PARAMS,
				label: "instance",
				matchers: ['up{job="api",env!="prod",pod=~"a.*",node!~"b.*"}'],
			})
		})

		it("falls back to equality for an unknown matcher type", async ({
			expect,
		}) => {
			const { client, fns } = makeClient()

			await client.labelValues("instance", "up", [
				makeMatcher("job", "api", -1),
			])

			expect(fns.labelValuesFn).toHaveBeenCalledExactlyOnceWith({
				...TIME_PARAMS,
				label: "instance",
				matchers: ['up{job="api"}'],
			})
		})
	})

	describe("metricMetadata", () => {
		it("returns the metadata result", async ({ expect }) => {
			const metadata = { up: [{ type: "gauge", help: "", unit: "" }] }
			const metricMetadataFn = vi.fn().mockResolvedValue({ result: metadata })
			const { client } = makeClient({ metricMetadataFn })

			const result = await client.metricMetadata()

			expect(result).toBe(metadata)
			expect(metricMetadataFn).toHaveBeenCalledTimes(1)
		})
	})

	describe("series", () => {
		it("queries the series and maps each result into label maps", async ({
			expect,
		}) => {
			const seriesFn = vi.fn().mockResolvedValue({
				result: [{ __name__: "up", job: "api" }, { __name__: "up" }],
			})
			const { client, fns } = makeClient({ seriesFn })

			const result = await client.series("up", [
				makeMatcher("job", "api", EqlSingle),
			])

			expect(result).toEqual([
				new Map([
					["__name__", "up"],
					["job", "api"],
				]),
				new Map([["__name__", "up"]]),
			])
			expect(fns.timeRangeFn).toHaveBeenCalledTimes(1)
			expect(seriesFn).toHaveBeenCalledExactlyOnceWith({
				...TIME_PARAMS,
				matchers: ['up{job="api"}'],
			})
		})
	})

	describe("metricNames", () => {
		it("queries the values of the __name__ label", async ({ expect }) => {
			const labelValuesFn = vi.fn().mockResolvedValue({ result: ["up"] })
			const { client } = makeClient({ labelValuesFn })

			const result = await client.metricNames()

			expect(result).toEqual(["up"])
			expect(labelValuesFn).toHaveBeenCalledExactlyOnceWith({
				...TIME_PARAMS,
				label: "__name__",
			})
		})
	})

	describe("flags", () => {
		it("resolves an empty flag set", async ({ expect }) => {
			const { client, fns } = makeClient()

			const result = await client.flags()

			expect(result).toEqual({})
			expect(fns.timeRangeFn).toHaveBeenCalledTimes(0)
		})
	})

	describe("destroy", () => {
		it("is a no-op", ({ expect }) => {
			const { client, fns } = makeClient()

			client.destroy()

			expect(fns.timeRangeFn).toHaveBeenCalledTimes(0)
		})
	})
})
