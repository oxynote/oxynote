import { describe, it } from "vitest"
import {
	DEFAULT_SIMULATION_PRESET,
	generateSimulatedQueryResults,
	generateSimulatedResult,
	isSimulationPreset,
	simulationStepSeconds,
} from "./simulation"
import { MetricSimulationPreset, TimeRangePreset } from "./utils"
import { at } from "~/components/test-helpers"

// a fixed instant so every expectation is reproducible; the generator is
// a pure function of the window it is given
const NOW = new Date(2026, 7, 19, 15, 30, 45, 123)

function windowOf(minutes: number, endingAt: Date = NOW) {
	return {
		from: new Date(endingAt.getTime() - minutes * 60 * 1000),
		to: endingAt,
	}
}

describe("simulationStepSeconds", () => {
	it("aims for roughly a hundred points and never buckets below 15s", ({
		expect,
	}) => {
		expect(simulationStepSeconds(5 * 60)).toBe(15)
		expect(simulationStepSeconds(60 * 60)).toBe(36)
		expect(simulationStepSeconds(24 * 60 * 60)).toBe(864)
	})
})

describe("isSimulationPreset", () => {
	it("accepts every preset that ships", ({ expect }) => {
		for (const preset of Object.values(MetricSimulationPreset)) {
			expect(isSimulationPreset(preset)).toBe(true)
		}
	})

	it.for([null, undefined, 42, "", "pie_of_the_day", {}])(
		"rejects %s",
		(value, { expect }) => {
			expect(isSimulationPreset(value)).toBe(false)
		},
	)
})

describe("generateSimulatedResult", () => {
	it("gives every preset its own fixed series set", ({ expect }) => {
		const { from, to } = windowOf(15)
		const seriesOf = (preset: MetricSimulationPreset) =>
			generateSimulatedResult(preset, from, to).data.map(
				(series) => series.labels.series,
			)

		expect(seriesOf(MetricSimulationPreset.CPUUsage)).toEqual(["usage"])
		expect(seriesOf(MetricSimulationPreset.MemoryUsage)).toEqual(["used"])
		expect(seriesOf(MetricSimulationPreset.DiskUsage)).toEqual(["used"])
		expect(seriesOf(MetricSimulationPreset.HTTPRequests)).toEqual([
			"2xx",
			"4xx",
			"5xx",
		])
		expect(seriesOf(MetricSimulationPreset.HTTPLatency)).toEqual([
			"p50",
			"p95",
			"p99",
		])
		expect(seriesOf(MetricSimulationPreset.ErrorRate)).toEqual(["errors"])
	})

	it("returns an ok result with one labelled series per preset series", ({
		expect,
	}) => {
		const { from, to } = windowOf(15)
		const res = generateSimulatedResult(
			MetricSimulationPreset.HTTPLatency,
			from,
			to,
		)

		expect(res.status).toBe(GenericQueryResultStatus.Ok)
		expect(res.data.map((s) => s.labels)).toEqual([
			{ series: "p50" },
			{ series: "p95" },
			{ series: "p99" },
		])
		expect(at(res.data, 0).metrics.length).toBeGreaterThan(50)
	})

	it("is deterministic for the same preset and window", ({ expect }) => {
		const { from, to } = windowOf(30)

		expect(
			generateSimulatedResult(MetricSimulationPreset.CPUUsage, from, to),
		).toEqual(
			generateSimulatedResult(MetricSimulationPreset.CPUUsage, from, to),
		)
	})

	it("keeps overlapping points identical when the window slides", ({
		expect,
	}) => {
		const first = windowOf(15)
		const second = windowOf(15, new Date(NOW.getTime() + 60 * 1000))

		const before = generateSimulatedResult(
			MetricSimulationPreset.HTTPRequests,
			first.from,
			first.to,
		)
		const after = generateSimulatedResult(
			MetricSimulationPreset.HTTPRequests,
			second.from,
			second.to,
		)

		const afterByTimestamp = new Map(
			at(after.data, 0).metrics.map(([ts, value]) => [ts, value]),
		)
		const overlapping = at(before.data, 0).metrics.filter(([ts]) =>
			afterByTimestamp.has(ts),
		)

		expect(overlapping.length).toBeGreaterThan(10)
		for (const [ts, value] of overlapping) {
			expect(afterByTimestamp.get(ts)).toBe(value)
		}
	})

	it("aligns buckets to the step so the same window always samples the same instants", ({
		expect,
	}) => {
		const { from, to } = windowOf(60)
		const step = simulationStepSeconds(60 * 60)
		const res = generateSimulatedResult(
			MetricSimulationPreset.ErrorRate,
			from,
			to,
		)

		for (const [ts] of at(res.data, 0).metrics) {
			expect(ts % step).toBe(0)
		}
	})

	it("keeps percentage presets inside 0..100", ({ expect }) => {
		const { from, to } = windowOf(24 * 60)

		for (const preset of [
			MetricSimulationPreset.CPUUsage,
			MetricSimulationPreset.DiskUsage,
			MetricSimulationPreset.ErrorRate,
		]) {
			const res = generateSimulatedResult(preset, from, to)

			for (const [, value] of at(res.data, 0).metrics) {
				expect(value).toBeGreaterThanOrEqual(0)
				expect(value).toBeLessThanOrEqual(100)
			}
		}
	})

	it("gives disk usage a monotonically growing shape", ({ expect }) => {
		const { from, to } = windowOf(7 * 24 * 60)
		const res = generateSimulatedResult(
			MetricSimulationPreset.DiskUsage,
			from,
			to,
		)
		const metrics = at(res.data, 0).metrics

		expect(at(metrics, metrics.length - 1)[1]).toBeGreaterThan(
			at(metrics, 0)[1],
		)
	})

	it("gives each preset a distinct shape", ({ expect }) => {
		const { from, to } = windowOf(60)
		const firstValues = Object.values(MetricSimulationPreset).map(
			(preset) =>
				at(at(generateSimulatedResult(preset, from, to).data, 0).metrics, 0)[1],
		)

		expect(new Set(firstValues).size).toBe(firstValues.length)
	})

	it("returns empty series for an inverted window", ({ expect }) => {
		const res = generateSimulatedResult(
			MetricSimulationPreset.CPUUsage,
			NOW,
			new Date(NOW.getTime() - 60 * 1000),
		)

		expect(at(res.data, 0).metrics).toEqual([])
	})
})

describe("generateSimulatedQueryResults", () => {
	it("wraps the result as one virtual query naming series from labels", ({
		expect,
	}) => {
		const res = generateSimulatedQueryResults(
			MetricSimulationPreset.HTTPLatency,
			TimeRangePreset.Last15Minutes,
			NOW,
		)

		expect(res).toHaveLength(1)
		expect(at(res, 0).name).toBe(MetricSimulationPreset.HTTPLatency)
		expect(at(res, 0).legendFormat).toBe("{{series}}")
		expect(at(res, 0).result.data).toHaveLength(3)
	})

	it("falls back to the default time range when the block has none", ({
		expect,
	}) => {
		const res = generateSimulatedQueryResults(
			DEFAULT_SIMULATION_PRESET,
			null,
			NOW,
		)
		const metrics = at(at(res, 0).result.data, 0).metrics
		const spanSeconds = at(metrics, metrics.length - 1)[0] - at(metrics, 0)[0]

		expect(spanSeconds).toBeLessThanOrEqual(5 * 60)
		expect(spanSeconds).toBeGreaterThan(4 * 60)
	})
})
