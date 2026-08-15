export enum PrometheusValueType {
	Scalar = "scalar",
	Vector = "vector",
	Matrix = "matrix",
	String = "string",
}

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

export interface PrometheusQueryParams extends PrometheusTimeRange {
	q: string
}

export interface PrometheusLabelParams extends PrometheusTimeRange {
	matchers?: string[]
}

export interface PrometheusLabelValuesParams extends PrometheusLabelParams {
	label: string
}

export interface PrometheusQueryResult {
	type: PrometheusValueType
	warnings?: string[]
	result:
		| PrometheusQueryScalarResult
		| PrometheusQueryStringResult
		| PrometheusQueryVectorResult
		| PrometheusQueryMatrixResult
}

// https://prometheus.io/docs/prometheus/latest/querying/api/#scalars
export type PrometheusQueryScalarResult = [number, string] // unix seconds

// https://prometheus.io/docs/prometheus/latest/querying/api/#strings
export type PrometheusQueryStringResult = [number, string] // unix seconds

// https://prometheus.io/docs/prometheus/latest/querying/api/#instant-vectors
// Each series could have the "value" key, or the "histogram" key, but not both.
export type PrometheusQueryVectorResult = {
	metric: Record<string, string> // label name: label value
	value?: [number, string] // unix seconds
	histogram?: [number, PrometheusQueryHistogramResultValue] // unix seconds
}[]

// https://prometheus.io/docs/prometheus/latest/querying/api/#range-vectors
// Each series could have the "values" key, or the "histograms" key, or both.
// For a given timestamp, there will only be one sample of either float or
// histogram type.
export type PrometheusQueryMatrixResult = {
	metric: Record<string, string> // label name: label value
	values?: [number, string][] // unix seconds
	histograms?: [number, PrometheusQueryHistogramResultValue][] // unix seconds
}[]

// https://prometheus.io/docs/prometheus/latest/querying/api/#native-histograms
export interface PrometheusQueryHistogramResultValue {
	count: string
	sum: string
	buckets: [
		0 | 1 | 2 | 3, // boundary_rule
		string, // left_boundary
		string, // right_boundary
		string, // count_in_bucket
	][]
}

export interface PrometheusMultipleQueriesParams extends PrometheusTimeRange {
	queries: string[]
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
