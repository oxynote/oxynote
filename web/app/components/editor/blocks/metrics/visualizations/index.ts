import type VChart from "vue-echarts"
import {
	VISUALIZATION_MAX_CUSTOM_UNIT_LENGTH,
	VisualizationCoreUnit,
	VisualizationDataUnit,
	VisualizationMiscUnit,
	VisualizationTimeUnit,
	type VisualizationUnit,
} from "../utils"

const MAX_AUTO_DECIMALS_AFTER_LAST_ZERO = 3

export interface BarChartData {
	name: string // shown in legend (can be label)
	data: [Date, number][]
}

export type MultipleBarChartData = BarChartData[]

export interface BarChartThreshold {
	value: number
	color: string
	label?: string
}

export interface LineChartData {
	name: string // shown in legend (can be label)
	data: [Date, number][]
}

export type MultipleLineChartData = LineChartData[]

export interface LineChartThreshold {
	value: number
	color: string
	label?: string
}

export interface GaugeChartData {
	name: string // shown in legend (can be label)
	value: number
}

export type MultipleGaugeChartData = GaugeChartData[]

export interface GaugeChartThreshold {
	value: number
	color: string // color starting at this threshold until the next one
}

export interface LegendSelectChangedPayload {
	name: string
	selected: Record<string, boolean>
}

export function installSoloLegendBehavior(chart: InstanceType<typeof VChart>) {
	let soloName: string | null = null

	return function (params: LegendSelectChangedPayload) {
		const clicked = params.name

		// restore all if clicking the soloed series again
		if (soloName === clicked) {
			soloName = null
			chart.dispatchAction({ type: "legendAllSelect" })
			return
		}

		// otherwise solo clicked
		soloName = clicked
		chart.dispatchAction({ type: "legendAllSelect" })
		for (const name of Object.keys(params.selected)) {
			if (name !== clicked) {
				chart.dispatchAction({ type: "legendUnSelect", name })
			}
		}
		chart.dispatchAction({ type: "legendSelect", name: clicked })
	}
}

function truncToDecimals(num: number, decimals: number | null) {
	if (decimals === null) {
		// Find position of last leading zero after decimal point
		// e.g., 0.00123 -> last zero is at position 2, so we truncate to
		// 2 + MAX_AUTO_DECIMALS_AFTER_LAST_ZERO
		const absNum = Math.abs(num)
		if (absNum === 0) {
			return 0
		}

		if (absNum >= 1) {
			decimals = MAX_AUTO_DECIMALS_AFTER_LAST_ZERO
		} else {
			// Count leading zeros after decimal point
			// -log10(0.001) = 3, floor gives us 2 for 0.001-0.009 range
			const leadingZeros = Math.floor(-Math.log10(absNum))
			decimals = leadingZeros + MAX_AUTO_DECIMALS_AFTER_LAST_ZERO
		}
	}

	return parseFloat(num.toFixed(decimals))
}

function extractVisualizationData(
	visualizationType: GenericQueryChartType | null | undefined,
	input: GenericQueryResult | null | undefined,
	legendLabelFormat: string,
	decimals: number | null,
	orderedLegendLabelFn: (type: GenericQueryChartType) => string,
): {
	status: GenericQueryResultStatus
	queryErrorMessage?: string
	data?: MultipleLineChartData | MultipleBarChartData | MultipleGaugeChartData
} {
	if (!visualizationType) {
		return { status: GenericQueryResultStatus.TypeNotSelected }
	}

	if (!input) {
		return { status: GenericQueryResultStatus.NoData }
	}

	if (input.status !== GenericQueryResultStatus.Ok) {
		return {
			status: input.status,
			queryErrorMessage: input.queryErrorMessage,
		}
	}

	const cleanNum = (v: number) => {
		return truncToDecimals(v, decimals)
	}

	switch (visualizationType) {
		case GenericQueryChartType.Line: // fallthrough
		case GenericQueryChartType.Bar: {
			const res: MultipleBarChartData = []

			for (const series of input.data) {
				if (!series.metrics.length) {
					continue
				}

				const points: [Date, number][] = []
				for (const [timeUnix, val] of series.metrics) {
					points.push([new Date(timeUnix * 1000), cleanNum(val)])
				}

				res.push({
					name: nameFromMetric(
						visualizationType,
						legendLabelFormat,
						orderedLegendLabelFn,
						series.labels,
					),
					data: points,
				})
			}

			return {
				status: input.status,
				data: res,
			}
		}
		case GenericQueryChartType.Gauge: {
			const res: MultipleGaugeChartData = []

			for (const series of input.data) {
				const lastMetric = series.metrics[series.metrics.length - 1]
				if (!series.metrics.length || lastMetric?.length !== 2) {
					continue
				}

				res.push({
					name: nameFromMetric(
						visualizationType,
						legendLabelFormat,
						orderedLegendLabelFn,
						series.labels,
					),
					value: cleanNum(lastMetric[1]),
				})
			}

			return {
				status: input.status,
				data: res,
			}
		}
	}
}

function nameFromMetric(
	type: GenericQueryChartType,
	legendLabelFormat: string, // contains template variables (e.g. {{instance}})
	orderedLegendLabelFn: (type: GenericQueryChartType) => string,
	labels: Record<string, string>,
) {
	if (legendLabelFormat) {
		return legendLabelFormat.replace(
			/\{\{([a-zA-Z_][\w:]*)\}\}/g,
			(match: string, variable: string) => {
				return labels[variable] ?? match
			},
		)
	}

	return orderedLegendLabelFn(type)
}

export function indexToAlphabeticLabel(index: number): string {
	let result = ""
	let n = index

	do {
		result = String.fromCharCode(65 + (n % 26)) + result
		n = Math.floor(n / 26) - 1
	} while (n >= 0)

	return result
}

export interface QueryResultItem {
	name: string
	legendFormat: string
	result: GenericQueryResult | null | undefined
}

export function mergeVisualizationResults(
	visualizationType: GenericQueryChartType | null | undefined,
	queryResults: QueryResultItem[] | null | undefined,
	decimals: number | null,
	orderedLegendLabelFn: (type: GenericQueryChartType) => string,
): {
	status: GenericQueryResultStatus
	queryErrorMessage?: string
	data?: MultipleLineChartData | MultipleBarChartData | MultipleGaugeChartData
} {
	if (!visualizationType) {
		return { status: GenericQueryResultStatus.TypeNotSelected }
	}

	if (!queryResults?.length) {
		return { status: GenericQueryResultStatus.NoData }
	}

	const allData: (LineChartData | BarChartData | GaugeChartData)[] = []

	for (const { result, legendFormat } of queryResults) {
		const extracted = extractVisualizationData(
			visualizationType,
			result,
			legendFormat,
			decimals,
			orderedLegendLabelFn,
		)

		if (extracted.status === GenericQueryResultStatus.NoData) {
			return { status: GenericQueryResultStatus.NoData }
		} else if (
			extracted.status === GenericQueryResultStatus.ChartAndDataMismatch
		) {
			return { status: GenericQueryResultStatus.ChartAndDataMismatch }
		} else if (extracted.status === GenericQueryResultStatus.Invalid) {
			return { status: GenericQueryResultStatus.Invalid }
		} else if (extracted.status === GenericQueryResultStatus.QueryError) {
			return {
				status: GenericQueryResultStatus.QueryError,
				queryErrorMessage: extracted.queryErrorMessage,
			}
		} else if (!extracted.data) {
			return { status: GenericQueryResultStatus.Invalid }
		}

		allData.push(...extracted.data)
	}

	return {
		status: GenericQueryResultStatus.Ok,
		data: allData as
			| MultipleLineChartData
			| MultipleBarChartData
			| MultipleGaugeChartData,
	}
}

const ONE_SECOND = 1000
const ONE_MINUTE = 60 * ONE_SECOND
const ONE_HOUR = 60 * ONE_MINUTE
const ONE_DAY = 24 * ONE_HOUR
const ONE_YEAR = 365 * ONE_DAY

/**
 * Computes the time range of the series data and returns a consistent
 * formatter function for x-axis labels. All ticks use the same format
 * so labels never mix e.g. "16:34" with "16:34:40".
 */
export function xAxisLabelFormatter(
	data: MultipleLineChartData | MultipleBarChartData,
): (value: number) => string {
	let minTime = Infinity
	let maxTime = -Infinity

	for (const series of data) {
		for (const [date] of series.data) {
			const t = date.getTime()
			if (t < minTime) minTime = t
			if (t > maxTime) maxTime = t
		}
	}

	const range = maxTime - minTime

	const pad = (n: number) => String(n).padStart(2, "0")

	return (value: number) => {
		const d = new Date(value)

		if (range < ONE_MINUTE) {
			// sub-hour: always show HH:mm:ss
			return `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
		}

		if (range < ONE_DAY) {
			// sub-day: show HH:mm
			return `${pad(d.getHours())}:${pad(d.getMinutes())}`
		}

		if (range < ONE_YEAR) {
			// sub-year: show short month + day
			const month = d.toLocaleString("en", { month: "short" })
			return `${month} ${d.getDate()}`
		}

		// multi-year: show year
		return `${d.getFullYear()}`
	}
}

export function yAxisLabelFormatter(
	unit: {
		type?: VisualizationUnit | null
		custom?: string | null
	},
	value: number,
	decimals: number | null = null,
): string {
	if (!Number.isFinite(value)) {
		return `${value}`
	}

	if (unit.type === VisualizationCoreUnit.Custom && unit.custom) {
		return `${value} ${unit.custom.slice(0, VISUALIZATION_MAX_CUSTOM_UNIT_LENGTH)}`
	}

	const formatScaledNumber = (num: number) =>
		`${truncToDecimals(num, decimals)}`

	type ScalableUnit = VisualizationTimeUnit | VisualizationDataUnit
	interface UnitScale {
		unit: ScalableUnit
		label: string
		factorToNext: number | null
	}

	const scaleUnits = (
		baseUnit: ScalableUnit,
		units: readonly UnitScale[],
	): { value: number; label: string } => {
		const startIndex = units.findIndex((entry) => entry.unit === baseUnit)
		if (startIndex === -1) {
			return { value, label: "" }
		}

		if (value === 0) {
			return { value: 0, label: units[startIndex]?.label ?? "" }
		}

		let scaled = value
		let index = startIndex
		let absValue = Math.abs(scaled)

		while (index < units.length - 1) {
			const factor = units[index]?.factorToNext
			if (!factor || absValue < factor) {
				break
			}

			scaled /= factor
			index += 1
			absValue = Math.abs(scaled)
		}

		while (index > 0 && absValue > 0 && absValue < 1) {
			const factor = units[index - 1]?.factorToNext
			if (!factor) {
				break
			}

			scaled *= factor
			index -= 1
			absValue = Math.abs(scaled)
		}

		return { value: scaled, label: units[index]?.label ?? "" }
	}

	const timeUnits: UnitScale[] = [
		{
			unit: VisualizationTimeUnit.Nanoseconds,
			label: "ns",
			factorToNext: 1000,
		},
		{
			unit: VisualizationTimeUnit.Microseconds,
			label: "µs",
			factorToNext: 1000,
		},
		{
			unit: VisualizationTimeUnit.Milliseconds,
			label: "ms",
			factorToNext: 1000,
		},
		{ unit: VisualizationTimeUnit.Seconds, label: "s", factorToNext: 60 },
		{ unit: VisualizationTimeUnit.Minutes, label: "m", factorToNext: 60 },
		{ unit: VisualizationTimeUnit.Hours, label: "h", factorToNext: 24 },
		{ unit: VisualizationTimeUnit.Days, label: "d", factorToNext: null },
	]

	const byteUnits: UnitScale[] = [
		{ unit: VisualizationDataUnit.Bytes, label: "B", factorToNext: 1000 },
		{ unit: VisualizationDataUnit.Kilobytes, label: "KB", factorToNext: 1000 },
		{ unit: VisualizationDataUnit.Megabytes, label: "MB", factorToNext: 1000 },
		{ unit: VisualizationDataUnit.Gigabytes, label: "GB", factorToNext: 1000 },
		{ unit: VisualizationDataUnit.Terabytes, label: "TB", factorToNext: null },
	]

	const bitUnits: UnitScale[] = [
		{ unit: VisualizationDataUnit.Bits, label: "b", factorToNext: 1000 },
		{ unit: VisualizationDataUnit.Kilobits, label: "Kb", factorToNext: 1000 },
		{ unit: VisualizationDataUnit.Megabits, label: "Mb", factorToNext: 1000 },
		{ unit: VisualizationDataUnit.Gigabits, label: "Gb", factorToNext: 1000 },
		{ unit: VisualizationDataUnit.Terabits, label: "Tb", factorToNext: null },
	]

	switch (unit.type) {
		// time
		case VisualizationTimeUnit.Nanoseconds:
		case VisualizationTimeUnit.Microseconds:
		case VisualizationTimeUnit.Milliseconds:
		case VisualizationTimeUnit.Seconds:
		case VisualizationTimeUnit.Minutes:
		case VisualizationTimeUnit.Hours:
		case VisualizationTimeUnit.Days: {
			const scaled = scaleUnits(unit.type, timeUnits)
			return `${formatScaledNumber(scaled.value)} ${scaled.label}`
		}

		// data
		case VisualizationDataUnit.Bytes:
		case VisualizationDataUnit.Kilobytes:
		case VisualizationDataUnit.Megabytes:
		case VisualizationDataUnit.Gigabytes:
		case VisualizationDataUnit.Terabytes: {
			const scaled = scaleUnits(unit.type, byteUnits)
			return `${formatScaledNumber(scaled.value)} ${scaled.label}`
		}
		case VisualizationDataUnit.Bits:
		case VisualizationDataUnit.Kilobits:
		case VisualizationDataUnit.Megabits:
		case VisualizationDataUnit.Gigabits:
		case VisualizationDataUnit.Terabits: {
			const scaled = scaleUnits(unit.type, bitUnits)
			return `${formatScaledNumber(scaled.value)} ${scaled.label}`
		}

		// misc
		case VisualizationMiscUnit.Percent0to100:
			return `${value}%`
		case VisualizationMiscUnit.Percent0to1:
			return `${value * 100}%`
	}

	return `${value}`
}

/**
 * Calculate a nice step size based on the range magnitude.
 * Returns steps like 0.5, 5, 50, 500, etc.
 */
function calculateNiceStep(range: number): number {
	if (range <= 0) {
		return 0.5
	}

	const magnitude = Math.pow(10, Math.floor(Math.log10(range)))
	const normalized = range / magnitude

	// Choose step to produce ~4-8 ticks
	if (normalized <= 2) {
		return magnitude * 0.5
	} else if (normalized <= 5) {
		return magnitude
	} else {
		return magnitude * 2
	}
}

/**
 * Rounds a value up to the nearest step.
 */
function roundUpToStep(value: number, step: number): number {
	const result = Math.ceil(value / step) * step
	// round to avoid floating-point precision issues (e.g., 1.4000000000000001)
	const decimals = countDecimals(step)
	return Number(result.toFixed(decimals))
}

/**
 * Rounds a value down to the nearest step.
 */
function roundDownToStep(value: number, step: number): number {
	const result = Math.floor(value / step) * step
	// round to avoid floating-point precision issues
	const decimals = countDecimals(step)
	return Number(result.toFixed(decimals))
}

/**
 * Count the number of decimal places in a number.
 */
function countDecimals(value: number): number {
	if (Math.floor(value) === value) {
		return 0
	}

	const str = value.toString()
	const decimalIndex = str.indexOf(".")

	return decimalIndex === -1 ? 0 : str.length - decimalIndex - 1
}

/**
 * Calculate optimal Y-axis bounds for bar and line charts.
 *
 * - Considers both data values and threshold values
 * - Uses range-based margins (not absolute value percentages)
 * - Rounds min down and max up to consistent step increments
 * - If all values are non-negative, min is clamped at 0
 * - Returns { min: 0, max: 100 } for empty data
 */
export function calculateYAxisBounds(
	data: (BarChartData | LineChartData)[],
	thresholds?: { value: number }[],
	margin = 0.1,
	bounds?: { min?: number | null; max?: number | null },
): { min: number; max: number } {
	// Collect all values from data points
	const allValues: number[] = []

	for (const series of data) {
		for (const [, value] of series.data) {
			allValues.push(value)
		}
	}

	// Include threshold values
	if (thresholds) {
		for (const t of thresholds) {
			allValues.push(t.value)
		}
	}

	// Return defaults for empty data
	if (allValues.length === 0) {
		return { min: 0, max: 100 }
	}

	const dataMin = Math.min(...allValues)
	const dataMax = Math.max(...allValues)
	const range = dataMax - dataMin

	// Handle zero range (all values are the same)
	if (range === 0) {
		const step = calculateNiceStep(Math.abs(dataMax) || 1)
		const max = roundUpToStep(dataMax + step, step)
		const min =
			dataMin >= 0
				? Math.max(0, roundDownToStep(dataMin - step, step))
				: roundDownToStep(dataMin - step, step)
		return {
			min: bounds?.min ?? min,
			max: bounds?.max ?? max,
		}
	}

	// Calculate a consistent step size based on the range
	const step = calculateNiceStep(range)

	// Use range-based margins for consistent spacing
	const rangeMargin = range * margin

	// Max: add margin based on range and round up to step
	const maxWithMargin = dataMax + rangeMargin
	const max = roundUpToStep(maxWithMargin, step)

	// Min: subtract margin based on range, round down to step
	// Only clamp at 0 if all values are non-negative
	const minWithMargin = dataMin - rangeMargin
	const minRounded = roundDownToStep(minWithMargin, step)
	const min = dataMin >= 0 ? Math.max(0, minRounded) : minRounded

	return {
		min: bounds?.min ?? min,
		max: bounds?.max ?? max,
	}
}

/**
 * Calculate optimal axis bounds for gauge charts with shared scale.
 *
 * - Considers all gauge values and threshold values together
 * - Uses range-based margins for consistent spacing
 * - Rounds min down and max up to consistent step increments
 * - If all values are non-negative, min is clamped at 0
 * - Returns { min: 0, max: 100 } for empty data
 */
export function calculateGaugeAxisBounds(
	gauges: MultipleGaugeChartData,
	thresholds?: { value: number }[],
	margin = 0.1,
	bounds?: { min?: number | null; max?: number | null },
): { min: number; max: number } {
	// Collect all values from gauges
	const allValues: number[] = gauges.map((g) => g.value)

	// Include threshold values
	if (thresholds) {
		for (const t of thresholds) {
			allValues.push(t.value)
		}
	}

	// Return defaults for empty data
	if (allValues.length === 0) {
		return { min: 0, max: 100 }
	}

	const dataMin = Math.min(...allValues)
	const dataMax = Math.max(...allValues)
	const range = dataMax - dataMin

	// Handle zero range (all values are the same)
	if (range === 0) {
		const step = calculateNiceStep(Math.abs(dataMax) || 1)
		const max = roundUpToStep(dataMax + step, step)
		const min =
			dataMin >= 0
				? Math.max(0, roundDownToStep(dataMin - step, step))
				: roundDownToStep(dataMin - step, step)
		return {
			min: bounds?.min ?? min,
			max: bounds?.max ?? max,
		}
	}

	// Calculate a consistent step size based on the range
	const step = calculateNiceStep(range)

	// Use range-based margins for consistent spacing
	const rangeMargin = range * margin

	// Max: add margin based on range and round up to step
	const maxWithMargin = dataMax + rangeMargin
	const max = roundUpToStep(maxWithMargin, step)

	// Min: subtract margin based on range, round down to step
	// Only clamp at 0 if all values are non-negative
	const minWithMargin = dataMin - rangeMargin
	const minRounded = roundDownToStep(minWithMargin, step)
	const min = dataMin >= 0 ? Math.max(0, minRounded) : minRounded

	return {
		min: bounds?.min ?? min,
		max: bounds?.max ?? max,
	}
}
