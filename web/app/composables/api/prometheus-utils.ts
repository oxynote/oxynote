import type { PrometheusClient } from "@prometheus-io/codemirror-promql"
import type { MetricMetadata } from "@prometheus-io/codemirror-promql/dist/esm/client/prometheus"
import type { Matcher } from "@prometheus-io/codemirror-promql/dist/esm/types"
import { EqlRegex, EqlSingle, Neq, NeqRegex } from "@prometheus-io/lezer-promql"
import {
	resolveTimeRange,
	type TimeRangePreset,
} from "~/components/editor/blocks/metrics/utils"

// based on: https://github.com/prometheus/prometheus/blob/main/web/ui/module/codemirror-promql/src/client/prometheus.ts#L83
export class PrometheusDataSourceClient implements PrometheusClient {
	private readonly timeRangeFn: () => Promise<TimeRangePreset> // in milliseconds
	private readonly metricMetadataFn: () => Promise<PrometheusMetadataResult>
	private readonly labelNamesFn: (
		params: PrometheusLabelParams,
	) => Promise<PrometheusLabelNamesResult>
	private readonly labelValuesFn: (
		params: PrometheusLabelValuesParams,
	) => Promise<PrometheusLabelValuesResult>
	private readonly seriesFn: (
		params: PrometheusSeriesParams,
	) => Promise<PrometheusSeriesResult>

	constructor(options: {
		timeRangeFn: () => Promise<TimeRangePreset>
		metricMetadataFn: () => Promise<PrometheusMetadataResult>
		labelNamesFn: (
			params: PrometheusLabelParams,
		) => Promise<PrometheusLabelNamesResult>
		labelValuesFn: (
			params: PrometheusLabelValuesParams,
		) => Promise<PrometheusLabelValuesResult>
		seriesFn: (
			params: PrometheusSeriesParams,
		) => Promise<PrometheusSeriesResult>
	}) {
		this.timeRangeFn = options.timeRangeFn
		this.metricMetadataFn = options.metricMetadataFn
		this.labelNamesFn = options.labelNamesFn
		this.labelValuesFn = options.labelValuesFn
		this.seriesFn = options.seriesFn
	}

	// https://prometheus.io/docs/prometheus/latest/querying/api/#getting-label-names
	async labelNames(metricName?: string): Promise<string[]> {
		const timeRangeKey = await this.timeRangeFn()
		const timeRange = resolveTimeRange(timeRangeKey)
		const params: PrometheusLabelParams = {
			from: timeRange.from,
			to: timeRange.to,
			timeRangeKey: timeRangeKey,
		}

		if (metricName) {
			params.matchers = [metricName]
		}

		return (await this.labelNamesFn(params)).result
	}

	// labelValues return a list of the value associated to the given labelName.
	// In case a metric is provided, then the list of values is then associated to the couple <MetricName, LabelName>
	// https://prometheus.io/docs/prometheus/latest/querying/api/#querying-label-values
	async labelValues(
		labelName: string,
		metricName?: string,
		matchers?: Matcher[],
	): Promise<string[]> {
		const timeRangeKey = await this.timeRangeFn()
		const timeRange = resolveTimeRange(timeRangeKey)
		const params: PrometheusLabelValuesParams = {
			from: timeRange.from,
			to: timeRange.to,
			label: labelName,
			timeRangeKey: timeRangeKey,
		}

		if (metricName) {
			params.matchers = [labelMatchersToString(metricName, matchers, labelName)]
		}

		return (await this.labelValuesFn(params)).result
	}

	async metricMetadata(): Promise<Record<string, MetricMetadata[]>> {
		return (await this.metricMetadataFn()).result
	}

	async series(
		metricName: string,
		matchers?: Matcher[],
		labelName?: string,
	): Promise<Map<string, string>[]> {
		const timeRangeKey = await this.timeRangeFn()
		const timeRange = resolveTimeRange(timeRangeKey)
		const params: PrometheusSeriesParams = {
			from: timeRange.from,
			to: timeRange.to,
			timeRangeKey: timeRangeKey,
			matchers: [labelMatchersToString(metricName, matchers, labelName)],
		}

		return (await this.seriesFn(params)).result.map(
			(m) => new Map(Object.entries(m)),
		)
	}

	metricNames(): Promise<string[]> {
		return this.labelValues("__name__")
	}

	async flags(): Promise<Record<string, string>> {
		// no-op for now
		// existing implementation: https://github.com/prometheus/prometheus/blob/main/web/ui/module/codemirror-promql/src/client/prometheus.ts#L197
		return {}
	}

	destroy(): void {
		// no-op for now
	}
}

// based on: https://github.com/prometheus/prometheus/blob/main/web/ui/module/codemirror-promql/src/parser/matcher.ts#L97
function labelMatchersToString(
	metricName: string,
	matchers?: Matcher[],
	labelName?: string,
): string {
	if (!matchers || matchers.length === 0) {
		return metricName
	}

	let matchersAsString = ""
	for (const matcher of matchers) {
		if (matcher.name === labelName || matcher.value === "") {
			continue
		}
		let type = ""
		switch (matcher.type) {
			case EqlSingle:
				type = "="
				break
			case Neq:
				type = "!="
				break
			case NeqRegex:
				type = "!~"
				break
			case EqlRegex:
				type = "=~"
				break
			default:
				type = "="
		}

		const m = `${matcher.name}${type}"${matcher.value}"`
		if (matchersAsString === "") {
			matchersAsString = m
		} else {
			matchersAsString = `${matchersAsString},${m}`
		}
	}

	return `${metricName}{${matchersAsString}}`
}
