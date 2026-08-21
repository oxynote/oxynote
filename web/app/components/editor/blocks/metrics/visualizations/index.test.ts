import type VChart from "vue-echarts"
import { describe, it, vi } from "vitest"
import {
	VisualizationCoreUnit,
	VisualizationDataUnit,
	VisualizationMiscUnit,
	VisualizationTimeUnit,
} from "../utils"
import {
	calculateGaugeAxisBounds,
	calculateYAxisBounds,
	indexToAlphabeticLabel,
	installSoloLegendBehavior,
	mergeVisualizationResults,
	type BarChartData,
	type MultipleGaugeChartData,
	type QueryResultItem,
	xAxisLabelFormatter,
	yAxisLabelFormatter,
} from "."

// the solo handler only ever calls dispatchAction, so a bare spy stands
// in for the whole echarts component
function stubChart() {
	const dispatchAction =
		vi.fn<(action: { type: string; name?: string }) => void>()

	return {
		dispatchAction,
		chart: { dispatchAction } as unknown as InstanceType<typeof VChart>,
	}
}

// the legend label fallback is injected, so it can report the chart type
// it was asked about
const orderedLabel = (type: GenericQueryChartType) => `auto-${type}`

function okResult(
	data: { labels: Record<string, string>; metrics: [number, number][] }[],
): GenericQueryResult {
	return { status: GenericQueryResultStatus.Ok, data }
}

function queryItem(
	result: GenericQueryResult | null | undefined,
	legendFormat = "",
): QueryResultItem {
	return { name: "Query 1", legendFormat, result }
}

// local-time components keep the x-axis expectations timezone-independent
function at(
	hours: number,
	minutes: number,
	seconds: number,
	day = 19,
	month = 7,
	year = 2026,
): Date {
	return new Date(year, month, day, hours, minutes, seconds)
}

function series(...dates: Date[]): BarChartData[] {
	return [{ name: "s", data: dates.map((d) => [d, 1]) }]
}

function bars(...values: number[]): BarChartData[] {
	return [{ name: "s", data: values.map((v) => [at(0, 0, 0), v]) }]
}

function gauges(...values: number[]): MultipleGaugeChartData {
	return values.map((value, i) => ({ name: `g${i}`, value }))
}

describe("installSoloLegendBehavior", () => {
	it("solos the clicked series by unselecting every other one", ({
		expect,
	}) => {
		const { chart, dispatchAction } = stubChart()
		const onLegendSelectChanged = installSoloLegendBehavior(chart)

		onLegendSelectChanged({
			name: "a",
			selected: { a: true, b: true, c: true },
		})

		expect(dispatchAction.mock.calls.map(([call]) => call)).toEqual([
			{ type: "legendAllSelect" },
			{ type: "legendUnSelect", name: "b" },
			{ type: "legendUnSelect", name: "c" },
			{ type: "legendSelect", name: "a" },
		])
	})

	it("restores every series when the soloed one is clicked again", ({
		expect,
	}) => {
		const { chart, dispatchAction } = stubChart()
		const onLegendSelectChanged = installSoloLegendBehavior(chart)
		const payload = { name: "a", selected: { a: true, b: true } }

		onLegendSelectChanged(payload)
		dispatchAction.mockClear()
		onLegendSelectChanged(payload)

		expect(dispatchAction.mock.calls.map(([call]) => call)).toEqual([
			{ type: "legendAllSelect" },
		])
	})

	it("solos the new series when a different one is clicked while soloed", ({
		expect,
	}) => {
		const { chart, dispatchAction } = stubChart()
		const onLegendSelectChanged = installSoloLegendBehavior(chart)

		onLegendSelectChanged({ name: "a", selected: { a: true, b: true } })
		dispatchAction.mockClear()
		onLegendSelectChanged({ name: "b", selected: { a: true, b: true } })

		expect(dispatchAction.mock.calls.map(([call]) => call)).toEqual([
			{ type: "legendAllSelect" },
			{ type: "legendUnSelect", name: "a" },
			{ type: "legendSelect", name: "b" },
		])
	})

	it("solos again after a restore", ({ expect }) => {
		const { chart, dispatchAction } = stubChart()
		const onLegendSelectChanged = installSoloLegendBehavior(chart)
		const payload = { name: "a", selected: { a: true, b: true } }

		onLegendSelectChanged(payload)
		onLegendSelectChanged(payload)
		dispatchAction.mockClear()
		onLegendSelectChanged(payload)

		expect(dispatchAction.mock.calls.map(([call]) => call)).toEqual([
			{ type: "legendAllSelect" },
			{ type: "legendUnSelect", name: "b" },
			{ type: "legendSelect", name: "a" },
		])
	})

	it("emits only the restore action for a single-series legend", ({
		expect,
	}) => {
		const { chart, dispatchAction } = stubChart()
		const onLegendSelectChanged = installSoloLegendBehavior(chart)

		onLegendSelectChanged({ name: "a", selected: { a: true } })

		expect(dispatchAction.mock.calls.map(([call]) => call)).toEqual([
			{ type: "legendAllSelect" },
			{ type: "legendSelect", name: "a" },
		])
	})
})

describe("indexToAlphabeticLabel", () => {
	it.for([
		{ index: 0, expected: "A" },
		{ index: 1, expected: "B" },
		{ index: 25, expected: "Z" },
		{ index: 26, expected: "AA" },
		{ index: 27, expected: "AB" },
		{ index: 51, expected: "AZ" },
		{ index: 52, expected: "BA" },
		{ index: 701, expected: "ZZ" },
		{ index: 702, expected: "AAA" },
	])("labels index $index as $expected", ({ index, expected }, { expect }) => {
		expect(indexToAlphabeticLabel(index)).toBe(expected)
	})
})

describe("mergeVisualizationResults", () => {
	it.for([
		{ name: "null", type: null },
		{ name: "undefined", type: undefined },
	])(
		"reports type-not-selected when the chart type is $name",
		({ type }, { expect }) => {
			expect(
				mergeVisualizationResults(
					type,
					[queryItem(okResult([]))],
					null,
					orderedLabel,
				),
			).toEqual({ status: GenericQueryResultStatus.TypeNotSelected })
		},
	)

	it.for([
		{ name: "null", results: null },
		{ name: "undefined", results: undefined },
		{ name: "an empty list", results: [] },
	])(
		"reports no-data when the query results are $name",
		({ results }, { expect }) => {
			expect(
				mergeVisualizationResults(
					GenericQueryChartType.Line,
					results,
					null,
					orderedLabel,
				),
			).toEqual({ status: GenericQueryResultStatus.NoData })
		},
	)

	it("converts unix seconds to dates for a line chart", ({ expect }) => {
		const merged = mergeVisualizationResults(
			GenericQueryChartType.Line,
			[
				queryItem(
					okResult([
						{
							labels: { instance: "node-1" },
							metrics: [
								[1700000000, 1],
								[1700000060, 2],
							],
						},
					]),
				),
			],
			null,
			orderedLabel,
		)

		expect(merged).toEqual({
			status: GenericQueryResultStatus.Ok,
			data: [
				{
					name: "auto-line_chart",
					data: [
						[new Date(1700000000000), 1],
						[new Date(1700000060000), 2],
					],
				},
			],
		})
	})

	it("concatenates the series of every query into one data set", ({
		expect,
	}) => {
		const merged = mergeVisualizationResults(
			GenericQueryChartType.Bar,
			[
				queryItem(
					okResult([{ labels: { job: "a" }, metrics: [[1700000000, 1]] }]),
					"{{job}}",
				),
				queryItem(
					okResult([
						{ labels: { job: "b" }, metrics: [[1700000000, 2]] },
						{ labels: { job: "c" }, metrics: [[1700000000, 3]] },
					]),
					"{{job}}",
				),
			],
			null,
			orderedLabel,
		)

		expect(merged.data?.map((s) => s.name)).toEqual(["a", "b", "c"])
	})

	it("skips series that carry no metrics in a line chart", ({ expect }) => {
		const merged = mergeVisualizationResults(
			GenericQueryChartType.Line,
			[
				queryItem(
					okResult([
						{ labels: { job: "empty" }, metrics: [] },
						{ labels: { job: "filled" }, metrics: [[1700000000, 1]] },
					]),
					"{{job}}",
				),
			],
			null,
			orderedLabel,
		)

		expect(merged.data?.map((s) => s.name)).toEqual(["filled"])
	})

	it("returns an ok status with no series when every series is empty", ({
		expect,
	}) => {
		expect(
			mergeVisualizationResults(
				GenericQueryChartType.Line,
				[queryItem(okResult([{ labels: {}, metrics: [] }]))],
				null,
				orderedLabel,
			),
		).toEqual({ status: GenericQueryResultStatus.Ok, data: [] })
	})

	it("keeps only the last sample of each series for a gauge chart", ({
		expect,
	}) => {
		const merged = mergeVisualizationResults(
			GenericQueryChartType.Gauge,
			[
				queryItem(
					okResult([
						{
							labels: { job: "a" },
							metrics: [
								[1700000000, 1],
								[1700000060, 7],
							],
						},
					]),
					"{{job}}",
				),
			],
			null,
			orderedLabel,
		)

		expect(merged).toEqual({
			status: GenericQueryResultStatus.Ok,
			data: [{ name: "a", value: 7 }],
		})
	})

	it("skips gauge series whose last sample is not a time-value pair", ({
		expect,
	}) => {
		const merged = mergeVisualizationResults(
			GenericQueryChartType.Gauge,
			[
				queryItem(
					okResult([
						{
							labels: { job: "broken" },
							metrics: [[1700000000] as unknown as [number, number]],
						},
						{ labels: { job: "empty" }, metrics: [] },
						{ labels: { job: "good" }, metrics: [[1700000000, 5]] },
					]),
					"{{job}}",
				),
			],
			null,
			orderedLabel,
		)

		expect(merged.data).toEqual([{ name: "good", value: 5 }])
	})

	it.for([
		{ name: "null", result: null },
		{ name: "undefined", result: undefined },
	])(
		"reports no-data when a query result is $name",
		({ result }, { expect }) => {
			expect(
				mergeVisualizationResults(
					GenericQueryChartType.Line,
					[queryItem(okResult([])), queryItem(result)],
					null,
					orderedLabel,
				),
			).toEqual({ status: GenericQueryResultStatus.NoData })
		},
	)

	it.for([
		{ status: GenericQueryResultStatus.NoData },
		{ status: GenericQueryResultStatus.ChartAndDataMismatch },
		{ status: GenericQueryResultStatus.Invalid },
	])(
		"propagates the $status status of a failed query",
		({ status }, { expect }) => {
			expect(
				mergeVisualizationResults(
					GenericQueryChartType.Line,
					[queryItem({ status, data: [] })],
					null,
					orderedLabel,
				),
			).toEqual({ status })
		},
	)

	it("propagates the error and its message for a query that errored", ({
		expect,
	}) => {
		expect(
			mergeVisualizationResults(
				GenericQueryChartType.Line,
				[
					queryItem(okResult([{ labels: {}, metrics: [[1700000000, 1]] }])),
					queryItem({
						status: GenericQueryResultStatus.QueryError,
						data: [],
						queryErrorMessage: "boom",
					}),
				],
				null,
				orderedLabel,
			),
		).toEqual({
			status: GenericQueryResultStatus.QueryError,
			queryErrorMessage: "boom",
		})
	})

	it("propagates the error of a query that errored without a message", ({
		expect,
	}) => {
		expect(
			mergeVisualizationResults(
				GenericQueryChartType.Line,
				[queryItem({ status: GenericQueryResultStatus.QueryError, data: [] })],
				null,
				orderedLabel,
			),
		).toEqual({
			status: GenericQueryResultStatus.QueryError,
			queryErrorMessage: undefined,
		})
	})

	it.for<{
		name: string
		format: string
		labels: Record<string, string>
		expected: string
	}>([
		{
			name: "substitutes every template variable from the labels",
			format: "{{job}} on {{instance}}",
			labels: { job: "api", instance: "node-1" },
			expected: "api on node-1",
		},
		{
			name: "leaves an unknown template variable untouched",
			format: "{{job}}/{{missing}}",
			labels: { job: "api" },
			expected: "api/{{missing}}",
		},
		{
			name: "substitutes a variable containing a colon",
			format: "{{job:name}}",
			labels: { "job:name": "api" },
			expected: "api",
		},
		{
			name: "ignores a placeholder starting with a digit",
			format: "{{1job}}",
			labels: { "1job": "api" },
			expected: "{{1job}}",
		},
		{
			name: "keeps a format without any template variable verbatim",
			format: "static",
			labels: { job: "api" },
			expected: "static",
		},
	])("$name", ({ format, labels, expected }, { expect }) => {
		const merged = mergeVisualizationResults(
			GenericQueryChartType.Line,
			[queryItem(okResult([{ labels, metrics: [[1700000000, 1]] }]), format)],
			null,
			orderedLabel,
		)

		expect(merged.data?.[0]?.name).toBe(expected)
	})

	it("falls back to the ordered legend label for a gauge without a format", ({
		expect,
	}) => {
		const merged = mergeVisualizationResults(
			GenericQueryChartType.Gauge,
			[queryItem(okResult([{ labels: {}, metrics: [[1700000000, 1]] }]))],
			null,
			orderedLabel,
		)

		expect(merged.data?.[0]?.name).toBe("auto-gauge_chart")
	})

	it.for([
		{ name: "keeps an integer untouched", value: 42, expected: 42 },
		{ name: "keeps zero at zero", value: 0, expected: 0 },
		{
			name: "truncates a value above one to three decimals",
			value: 1.23456789,
			expected: 1.235,
		},
		{
			name: "truncates a negative value to three decimals",
			value: -1.23456789,
			expected: -1.235,
		},
		{
			name: "keeps three decimals for a value just under one",
			value: 0.987654,
			expected: 0.988,
		},
		{
			name: "keeps five decimals past two leading zeros",
			value: 0.00123456,
			expected: 0.00123,
		},
		{
			name: "keeps six decimals past three leading zeros",
			value: 0.0001234567,
			expected: 0.000123,
		},
		{
			name: "keeps five decimals for an exact power of ten",
			value: 0.01,
			expected: 0.01,
		},
	])(
		"$name when no decimal count is configured",
		({ value, expected }, { expect }) => {
			const merged = mergeVisualizationResults(
				GenericQueryChartType.Gauge,
				[queryItem(okResult([{ labels: {}, metrics: [[1700000000, value]] }]))],
				null,
				orderedLabel,
			)

			expect(merged.data?.[0]).toEqual({
				name: "auto-gauge_chart",
				value: expected,
			})
		},
	)

	it.for([
		{ decimals: 0, expected: 1 },
		{ decimals: 1, expected: 1.2 },
		{ decimals: 4, expected: 1.2346 },
	])(
		"rounds to $decimals decimals when configured",
		({ decimals, expected }, { expect }) => {
			const merged = mergeVisualizationResults(
				GenericQueryChartType.Gauge,
				[
					queryItem(
						okResult([{ labels: {}, metrics: [[1700000000, 1.23456789]] }]),
					),
				],
				decimals,
				orderedLabel,
			)

			expect(merged.data?.[0]).toEqual({
				name: "auto-gauge_chart",
				value: expected,
			})
		},
	)
})

describe("xAxisLabelFormatter", () => {
	it.for([
		{
			name: "formats seconds when the range is under a minute",
			points: [at(15, 30, 45), at(15, 31, 14)],
			value: at(15, 30, 45),
			expected: "15:30:45",
		},
		{
			name: "formats seconds for a single data point",
			points: [at(9, 5, 3)],
			value: at(9, 5, 3),
			expected: "09:05:03",
		},
		{
			name: "drops the seconds at exactly one minute of range",
			points: [at(15, 30, 0), at(15, 31, 0)],
			value: at(15, 30, 0),
			expected: "15:30",
		},
		{
			name: "formats hours and minutes for a sub-day range",
			points: [at(1, 0, 0), at(23, 0, 0)],
			value: at(7, 8, 9),
			expected: "07:08",
		},
		{
			name: "switches to month and day at exactly one day of range",
			points: [at(0, 0, 0, 19), at(0, 0, 0, 20)],
			value: at(0, 0, 0, 19),
			expected: "Aug 19",
		},
		{
			name: "formats month and day for a multi-week range",
			points: [at(0, 0, 0, 1, 6), at(0, 0, 0, 19, 7)],
			value: at(0, 0, 0, 3, 6),
			expected: "Jul 3",
		},
		{
			name: "switches to the year at exactly one year of range",
			points: [at(0, 0, 0, 19, 7, 2025), at(0, 0, 0, 19, 7, 2026)],
			value: at(0, 0, 0, 19, 7, 2026),
			expected: "2026",
		},
	])("$name", ({ points, value, expected }, { expect }) => {
		const format = xAxisLabelFormatter(series(...points))

		expect(format(value.getTime())).toBe(expected)
	})

	it("formats seconds for an empty data set", ({ expect }) => {
		expect(xAxisLabelFormatter([])(at(15, 30, 45).getTime())).toBe("15:30:45")
	})

	it("spans the range across every series", ({ expect }) => {
		const format = xAxisLabelFormatter([
			{ name: "a", data: [[at(15, 30, 0), 1]] },
			{ name: "b", data: [[at(17, 30, 0), 1]] },
		])

		expect(format(at(15, 30, 0).getTime())).toBe("15:30")
	})
})

describe("yAxisLabelFormatter", () => {
	it.for([
		{ name: "NaN", value: Number.NaN, expected: "NaN" },
		{ name: "Infinity", value: Number.POSITIVE_INFINITY, expected: "Infinity" },
		{
			name: "-Infinity",
			value: Number.NEGATIVE_INFINITY,
			expected: "-Infinity",
		},
	])("renders $name verbatim", ({ value, expected }, { expect }) => {
		expect(
			yAxisLabelFormatter({ type: VisualizationTimeUnit.Seconds }, value),
		).toBe(expected)
	})

	it("appends a custom unit", ({ expect }) => {
		expect(
			yAxisLabelFormatter(
				{ type: VisualizationCoreUnit.Custom, custom: "widgets" },
				12,
			),
		).toBe("12 widgets")
	})

	it("truncates a custom unit longer than the allowed length", ({ expect }) => {
		expect(
			yAxisLabelFormatter(
				{ type: VisualizationCoreUnit.Custom, custom: "abcdefghij" },
				12,
			),
		).toBe("12 abcdefgh")
	})

	it.for([
		{
			name: "an empty custom unit",
			unit: { type: VisualizationCoreUnit.Custom, custom: "" },
		},
		{
			name: "a missing custom unit",
			unit: { type: VisualizationCoreUnit.Custom },
		},
		{ name: "no unit type", unit: {} },
		{ name: "a null unit type", unit: { type: null } },
	])("renders the bare value for $name", ({ unit }, { expect }) => {
		expect(yAxisLabelFormatter(unit, 12)).toBe("12")
	})

	it.for([
		{ unit: VisualizationTimeUnit.Nanoseconds, value: 1, expected: "1 ns" },
		{
			unit: VisualizationTimeUnit.Nanoseconds,
			value: 1500,
			expected: "1.5 µs",
		},
		{
			unit: VisualizationTimeUnit.Nanoseconds,
			value: 0.5,
			expected: "0.5 ns",
		},
		{
			unit: VisualizationTimeUnit.Microseconds,
			value: 2500,
			expected: "2.5 ms",
		},
		{
			unit: VisualizationTimeUnit.Milliseconds,
			value: 1500,
			expected: "1.5 s",
		},
		{ unit: VisualizationTimeUnit.Seconds, value: 0.5, expected: "500 ms" },
		{ unit: VisualizationTimeUnit.Seconds, value: 90, expected: "1.5 m" },
		{ unit: VisualizationTimeUnit.Seconds, value: 0, expected: "0 s" },
		{ unit: VisualizationTimeUnit.Seconds, value: -1500, expected: "-25 m" },
		{ unit: VisualizationTimeUnit.Minutes, value: 90, expected: "1.5 h" },
		{ unit: VisualizationTimeUnit.Hours, value: 36, expected: "1.5 d" },
		{ unit: VisualizationTimeUnit.Days, value: 400, expected: "400 d" },
		{
			unit: VisualizationTimeUnit.Nanoseconds,
			value: 86400e9,
			expected: "1 d",
		},
	])(
		"scales $value $unit to $expected",
		({ unit, value, expected }, { expect }) => {
			expect(yAxisLabelFormatter({ type: unit }, value)).toBe(expected)
		},
	)

	it.for([
		{ unit: VisualizationDataUnit.Bytes, value: 1500, expected: "1.5 KB" },
		{ unit: VisualizationDataUnit.Bytes, value: 0, expected: "0 B" },
		{ unit: VisualizationDataUnit.Bytes, value: 999, expected: "999 B" },
		{ unit: VisualizationDataUnit.Bytes, value: 1e15, expected: "1000 TB" },
		{ unit: VisualizationDataUnit.Kilobytes, value: 0.5, expected: "500 B" },
		{
			unit: VisualizationDataUnit.Megabytes,
			value: 2048,
			expected: "2.048 GB",
		},
		{ unit: VisualizationDataUnit.Gigabytes, value: 1500, expected: "1.5 TB" },
		{ unit: VisualizationDataUnit.Terabytes, value: 3, expected: "3 TB" },
		{ unit: VisualizationDataUnit.Bits, value: 2000, expected: "2 Kb" },
		{ unit: VisualizationDataUnit.Kilobits, value: 1e6, expected: "1 Gb" },
		{ unit: VisualizationDataUnit.Megabits, value: 0.001, expected: "1 Kb" },
		{ unit: VisualizationDataUnit.Gigabits, value: 1500, expected: "1.5 Tb" },
		{ unit: VisualizationDataUnit.Terabits, value: 0, expected: "0 Tb" },
	])(
		"scales $value $unit to $expected",
		({ unit, value, expected }, { expect }) => {
			expect(yAxisLabelFormatter({ type: unit }, value)).toBe(expected)
		},
	)

	it.for([
		{ unit: VisualizationMiscUnit.Percent0to100, value: 42, expected: "42%" },
		{ unit: VisualizationMiscUnit.Percent0to1, value: 0.5, expected: "50%" },
	])(
		"renders $value $unit as $expected",
		({ unit, value, expected }, { expect }) => {
			expect(yAxisLabelFormatter({ type: unit }, value)).toBe(expected)
		},
	)

	it("applies the configured decimal count to a scaled value", ({ expect }) => {
		expect(
			yAxisLabelFormatter(
				{ type: VisualizationTimeUnit.Milliseconds },
				1234.5678,
				2,
			),
		).toBe("1.23 s")
	})

	it("keeps the automatic decimal count when none is configured", ({
		expect,
	}) => {
		expect(
			yAxisLabelFormatter(
				{ type: VisualizationTimeUnit.Milliseconds },
				1234.5678,
			),
		).toBe("1.235 s")
	})
})

describe("calculateYAxisBounds", () => {
	it("returns the default bounds for an empty data set", ({ expect }) => {
		expect(calculateYAxisBounds([])).toEqual({ min: 0, max: 100 })
	})

	it("returns the default bounds when every series is empty", ({ expect }) => {
		expect(calculateYAxisBounds([{ name: "s", data: [] }])).toEqual({
			min: 0,
			max: 100,
		})
	})

	it.for([
		{
			name: "rounds to half-magnitude steps for a range at the low ratio",
			values: [10, 30],
			expected: { min: 5, max: 35 },
		},
		{
			name: "rounds to full-magnitude steps for a mid-ratio range",
			values: [0, 40],
			expected: { min: 0, max: 50 },
		},
		{
			name: "rounds to double-magnitude steps for a high-ratio range",
			values: [0, 80],
			expected: { min: 0, max: 100 },
		},
		{
			name: "clamps the minimum at zero for non-negative data",
			values: [1, 40],
			expected: { min: 0, max: 50 },
		},
		{
			name: "keeps a negative minimum below zero",
			values: [-30, -10],
			expected: { min: -35, max: -5 },
		},
		{
			name: "spans zero for data on both sides",
			values: [-20, 20],
			expected: { min: -30, max: 30 },
		},
		{
			name: "keeps fractional steps free of floating point noise",
			values: [0.1, 0.3],
			expected: { min: 0.05, max: 0.35 },
		},
		{
			name: "pads a single value by one step",
			values: [42],
			expected: { min: 30, max: 60 },
		},
		{
			name: "pads a repeated value by one step",
			values: [42, 42],
			expected: { min: 30, max: 60 },
		},
		{
			name: "pads a single zero with the fallback magnitude",
			values: [0],
			expected: { min: 0, max: 0.5 },
		},
		{
			name: "pads a single negative value below zero",
			values: [-5],
			expected: { min: -6, max: -4 },
		},
	])("$name", ({ values, expected }, { expect }) => {
		expect(calculateYAxisBounds(bars(...values))).toEqual(expected)
	})

	it("collects the values of every series", ({ expect }) => {
		expect(
			calculateYAxisBounds([
				{ name: "a", data: [[at(0, 0, 0), 10]] },
				{ name: "b", data: [[at(0, 0, 0), 30]] },
			]),
		).toEqual({ min: 5, max: 35 })
	})

	it("widens the bounds to include the thresholds", ({ expect }) => {
		expect(calculateYAxisBounds(bars(10, 30), [{ value: 90 }])).toEqual({
			min: 0,
			max: 100,
		})
	})

	it("derives the bounds from the thresholds alone when there is no data", ({
		expect,
	}) => {
		expect(calculateYAxisBounds([], [{ value: 10 }, { value: 30 }])).toEqual({
			min: 5,
			max: 35,
		})
	})

	it("drops the margin when it is zero", ({ expect }) => {
		expect(calculateYAxisBounds(bars(10, 30), undefined, 0)).toEqual({
			min: 10,
			max: 30,
		})
	})

	it("widens the bounds for a larger margin", ({ expect }) => {
		expect(calculateYAxisBounds(bars(10, 30), undefined, 0.5)).toEqual({
			min: 0,
			max: 40,
		})
	})

	it.for([
		{
			name: "overrides both bounds",
			bounds: { min: -5, max: 55 },
			expected: { min: -5, max: 55 },
		},
		{
			name: "overrides only the minimum",
			bounds: { min: -5 },
			expected: { min: -5, max: 35 },
		},
		{
			name: "overrides a bound with zero",
			bounds: { min: 0, max: 0 },
			expected: { min: 0, max: 0 },
		},
		{
			name: "falls back to the computed bounds for null overrides",
			bounds: { min: null, max: null },
			expected: { min: 5, max: 35 },
		},
		{
			name: "falls back to the computed bounds for an empty override",
			bounds: {},
			expected: { min: 5, max: 35 },
		},
	])("$name", ({ bounds, expected }, { expect }) => {
		expect(calculateYAxisBounds(bars(10, 30), undefined, 0.1, bounds)).toEqual(
			expected,
		)
	})
})

describe("calculateGaugeAxisBounds", () => {
	it("returns the default bounds for an empty gauge list", ({ expect }) => {
		expect(calculateGaugeAxisBounds([])).toEqual({ min: 0, max: 100 })
	})

	it.for([
		{
			name: "pads a single positive gauge by one step",
			values: [50],
			expected: { min: 40, max: 60 },
		},
		{
			name: "pads a zero gauge with the fallback magnitude",
			values: [0],
			expected: { min: 0, max: 0.5 },
		},
		{
			name: "pads a single negative gauge below zero",
			values: [-5],
			expected: { min: -6, max: -4 },
		},
		{
			name: "pads identical gauges by one step",
			values: [50, 50],
			expected: { min: 40, max: 60 },
		},
	])("$name", ({ values, expected }, { expect }) => {
		expect(calculateGaugeAxisBounds(gauges(...values))).toEqual(expected)
	})

	it.for([
		{
			name: "rounds a spread of gauges to consistent steps",
			values: [10, 30],
			expected: { min: 5, max: 35 },
		},
		{
			name: "clamps the minimum at zero for non-negative gauges",
			values: [1, 40],
			expected: { min: 0, max: 50 },
		},
		{
			name: "keeps a negative minimum below zero",
			values: [-30, -10],
			expected: { min: -35, max: -5 },
		},
	])("$name", ({ values, expected }, { expect }) => {
		expect(calculateGaugeAxisBounds(gauges(...values))).toEqual(expected)
	})

	it("widens the bounds to include the thresholds", ({ expect }) => {
		expect(calculateGaugeAxisBounds(gauges(50), [{ value: 90 }])).toEqual({
			min: 40,
			max: 100,
		})
	})

	it("derives the bounds from the thresholds alone when there are no gauges", ({
		expect,
	}) => {
		expect(
			calculateGaugeAxisBounds([], [{ value: 10 }, { value: 30 }]),
		).toEqual({ min: 5, max: 35 })
	})

	it("drops the margin when it is zero", ({ expect }) => {
		expect(calculateGaugeAxisBounds(gauges(10, 30), undefined, 0)).toEqual({
			min: 10,
			max: 30,
		})
	})

	it.for([
		{
			name: "overrides both bounds for a spread of gauges",
			values: [10, 30],
			bounds: { min: -5, max: 55 },
			expected: { min: -5, max: 55 },
		},
		{
			name: "overrides both bounds for identical gauges",
			values: [50],
			bounds: { min: 1, max: 2 },
			expected: { min: 1, max: 2 },
		},
		{
			name: "falls back to the computed bounds for null overrides",
			values: [50],
			bounds: { min: null, max: null },
			expected: { min: 40, max: 60 },
		},
	])("$name", ({ values, bounds, expected }, { expect }) => {
		expect(
			calculateGaugeAxisBounds(gauges(...values), undefined, 0.1, bounds),
		).toEqual(expected)
	})
})
