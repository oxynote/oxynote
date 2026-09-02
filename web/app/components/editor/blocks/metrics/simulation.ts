import {
	MetricSimulationPreset,
	resolveTimeRange,
	TimeRangePreset,
	type MetricConfig,
} from "./utils"

// the preset a block starts simulating with when the user presses the
// "simulate data" button
export const DEFAULT_SIMULATION_PRESET = MetricSimulationPreset.CPUUsage

// how often a simulated block asks core whether its real data has
// arrived. It is deliberately not the block's refresh interval: a chart
// that redraws once a day would keep simulating for a day after its
// metric went live.
export const SIMULATION_CHECK_INTERVAL_MS = 10_000

// how many points a simulated series aims for across its time range. The
// step derived from it is what buckets are aligned to, so a window that
// slides forward keeps the points it already showed.
const TARGET_POINTS = 100

// the smallest bucket size, mirroring the query processor's floor so a
// short range does not produce a point per second
const MIN_STEP_SECONDS = 15

const SECONDS_PER_DAY = 24 * 60 * 60

// a bucket of one series: the aligned timestamp plus the deterministic
// draws a waveform may take at it. salt separates independent draws
// within the same bucket (e.g. "is this a spike" from "how big").
interface Sample {
	timestamp: number
	rand: (salt: number) => number
}

interface PresetSeries {
	name: string
	value: (s: Sample) => number
}

// deterministic 32-bit mix of the caller's inputs. Values are a pure
// function of (preset, series, bucket, salt), so re-rendering the same
// window — or sliding it forward — never moves a point that already
// existed.
function hash(...parts: number[]): number {
	let h = 0x811c9dc5

	for (const part of parts) {
		h ^= part | 0
		h = Math.imul(h, 0x01000193)
		h ^= h >>> 15
	}

	return h >>> 0
}

function unitRand(...parts: number[]): number {
	return hash(...parts) / 0x100000000
}

// position within the day as a 0..1 fraction, the base of every diurnal
// waveform
function dayFraction(timestamp: number): number {
	return (timestamp % SECONDS_PER_DAY) / SECONDS_PER_DAY
}

// a smooth daily cycle peaking in the afternoon
function diurnal(timestamp: number): number {
	return (Math.sin(2 * Math.PI * (dayFraction(timestamp) - 0.25)) + 1) / 2
}

// a 0..1 ramp that repeats every periodSeconds, used by sawtooth shapes
function sawtooth(timestamp: number, periodSeconds: number): number {
	return (timestamp % periodSeconds) / periodSeconds
}

function clamp(v: number, min: number, max: number): number {
	return Math.min(max, Math.max(min, v))
}

// each preset owns its series set and their shapes; magnitudes are unit
// agnostic since the block applies its own configured unit.
const PRESETS: Record<MetricSimulationPreset, PresetSeries[]> = {
	[MetricSimulationPreset.CPUUsage]: [
		{
			name: "usage",
			value: (s) =>
				clamp(30 + 30 * diurnal(s.timestamp) + (s.rand(1) - 0.5) * 12, 0, 100),
		},
	],
	[MetricSimulationPreset.MemoryUsage]: [
		{
			// climbs steadily and drops back on restarts, the classic
			// heap sawtooth
			name: "used",
			value: (s) =>
				clamp(
					2.4 + 4.6 * sawtooth(s.timestamp, 45 * 60) + s.rand(1) * 0.25,
					0,
					8,
				),
		},
	],
	[MetricSimulationPreset.DiskUsage]: [
		{
			// grows slowly and monotonically over a 90-day cycle, so any
			// window shorter than that only ever climbs
			name: "used",
			value: (s) =>
				clamp(
					35 +
						55 * sawtooth(s.timestamp, 90 * SECONDS_PER_DAY) +
						s.rand(1) * 0.4,
					0,
					100,
				),
		},
	],
	[MetricSimulationPreset.HTTPRequests]: [
		{
			name: "2xx",
			value: (s) =>
				Math.round(900 + 700 * diurnal(s.timestamp) + (s.rand(1) - 0.5) * 220),
		},
		{
			name: "4xx",
			value: (s) =>
				Math.round(
					40 +
						30 * diurnal(s.timestamp) +
						s.rand(2) * 25 +
						(s.rand(3) < 0.05 ? s.rand(4) * 160 : 0),
				),
		},
		{
			name: "5xx",
			value: (s) =>
				Math.round(s.rand(5) * 6 + (s.rand(6) < 0.03 ? s.rand(7) * 70 : 0)),
		},
	],
	[MetricSimulationPreset.HTTPLatency]: [
		{
			name: "p50",
			value: (s) => 28 + s.rand(1) * 10,
		},
		{
			// a calm baseline broken by short bursts, the shape a latency
			// chart is usually read for
			name: "p95",
			value: (s) =>
				68 + s.rand(2) * 12 + (s.rand(3) < 0.12 ? s.rand(4) * 380 : 0),
		},
		{
			name: "p99",
			value: (s) =>
				120 + s.rand(5) * 40 + (s.rand(6) < 0.18 ? s.rand(7) * 620 : 0),
		},
	],
	[MetricSimulationPreset.ErrorRate]: [
		{
			// mostly flat near zero, with rare incident bursts
			name: "errors",
			value: (s) =>
				clamp(
					0.2 + s.rand(1) * 0.4 + (s.rand(2) < 0.04 ? s.rand(3) * 9 : 0),
					0,
					100,
				),
		},
	],
}

// stepSeconds is the bucket size a range of the given length is sampled
// at. It depends only on the range length, so the buckets of a window
// that slides forward stay aligned with the ones before it.
export function simulationStepSeconds(rangeSeconds: number): number {
	return Math.max(MIN_STEP_SECONDS, Math.round(rangeSeconds / TARGET_POINTS))
}

// isSimulationPreset reports whether a value stored on a block names a
// preset this build knows. The attribute reaches the editor as whatever
// the document holds, so an unknown one has to read as "not simulating"
// rather than reaching a lookup that has nothing to answer with.
export function isSimulationPreset(
	value: unknown,
): value is MetricSimulationPreset {
	return (
		typeof value === "string" &&
		Object.values(MetricSimulationPreset).includes(
			value as MetricSimulationPreset,
		)
	)
}

// generateSimulatedResult builds the query result a simulated block
// renders. It is shaped like a real data source response so the block's
// normal pipeline (units, decimals, thresholds, every chart type) applies
// to it unchanged.
export function generateSimulatedResult(
	preset: MetricSimulationPreset,
	from: Date,
	to: Date,
): GenericQueryResult {
	const fromSeconds = Math.floor(from.getTime() / 1000)
	const toSeconds = Math.floor(to.getTime() / 1000)
	const step = simulationStepSeconds(Math.max(0, toSeconds - fromSeconds))

	const firstBucket = Math.ceil(fromSeconds / step) * step
	const lastBucket = Math.floor(toSeconds / step) * step

	const presetSeed = Object.values(MetricSimulationPreset).indexOf(preset) + 1

	const data = PRESETS[preset].map((series, seriesIndex) => {
		const metrics: [number, number][] = []

		for (let ts = firstBucket; ts <= lastBucket; ts += step) {
			const sample: Sample = {
				timestamp: ts,
				rand: (salt) => unitRand(presetSeed, seriesIndex, ts, salt),
			}

			metrics.push([ts, series.value(sample)])
		}

		return {
			labels: { series: series.name },
			metrics,
		}
	})

	return {
		status: GenericQueryResultStatus.Ok,
		data,
	}
}

// generateSimulatedQueryResults wraps the generated result the way
// VisualizationContainer consumes real query results. The legend format
// pins series names to the preset's own, since a simulated block has no
// real labels to interpolate.
export function generateSimulatedQueryResults(
	preset: MetricSimulationPreset,
	timeRange: MetricConfig["timeRange"],
	now: Date = new Date(),
): { name: string; legendFormat: string; result: GenericQueryResult }[] {
	const { from, to } = resolveTimeRange(
		timeRange ?? TimeRangePreset.Last5Minutes,
		now,
	)

	return [
		{
			name: preset,
			legendFormat: "{{series}}",
			result: generateSimulatedResult(preset, from, to),
		},
	]
}
