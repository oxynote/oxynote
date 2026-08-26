export enum PrometheusMetricType {
	Counter = "counter",
	Gauge = "gauge",
	Histogram = "histogram",
	Summary = "summary",
}

export interface PrometheusTimeRange {
	from?: Date | string
	to?: Date | string
	timeRangeKey?: string // key representing a preset time range (for caching; local)
}

export interface PrometheusLabelParams extends PrometheusTimeRange {
	matchers?: string[]
}

export interface PrometheusLabelValuesParams extends PrometheusLabelParams {
	label: string
}

export interface PrometheusMetricMetadata {
	type: PrometheusMetricType
	help: string
	unit: string
}

export interface PrometheusMetadataResult {
	result: Record<string, PrometheusMetricMetadata[]>
}

export interface PrometheusLabelNamesResult {
	warnings?: string[]
	result: string[]
}

export interface PrometheusLabelValuesResult {
	warnings?: string[]
	result: string[]
}

export interface PrometheusSeriesParams extends PrometheusTimeRange {
	matchers: string[]
}

export interface PrometheusSeriesResult {
	warnings?: string[]
	result: Record<string, string>[]
}
