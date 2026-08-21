// shared helpers for the query editor test suites in this directory.
// The suites run in the node vitest project, which resolves only
// app/utils and app/composables auto-imports — everything else nuxt
// would normally inject is provided here.
import { vi, type Mock } from "vitest"
import { computed, ref, toRef, toValue } from "vue"
import { refDebounced } from "@vueuse/core"

// nuxt auto-imports vue reactivity and vueuse helpers into app code; the
// node vitest project leaves them as bare globals in the modules under
// test. Per-file module isolation keeps the assignment from leaking into
// other suites.
export function installAutoImportGlobals() {
	Object.assign(globalThis, { computed, ref, toRef, toValue, refDebounced })
}

function makeQueryStub(data: unknown) {
	return { refresh: vi.fn().mockResolvedValue({ data }) }
}

export interface PrometheusStubData {
	metadata?: PrometheusMetadataResult | null
	labels?: PrometheusLabelNamesResult | null
	labelValues?: PrometheusLabelValuesResult | null
	series?: PrometheusSeriesResult | null
}

// wires a fresh fake prometheus API into the mock behind the
// auto-imported composable. The mock return value is read synchronously
// by the composable call that immediately follows, so concurrent tests
// cannot observe each other's stub.
export function stubPrometheusAPI(mock: Mock, data: PrometheusStubData = {}) {
	const api = {
		metadata: makeQueryStub(data.metadata ?? { result: {} }),
		labels: makeQueryStub(data.labels ?? { result: [] }),
		labelValues: makeQueryStub(data.labelValues ?? { result: [] }),
		series: makeQueryStub(data.series ?? { result: [] }),
	}

	const captured: {
		labelValuesParams?: Ref<PrometheusLabelValuesParams | null | undefined>
		labelsParams?: MaybeRefOrGetter<PrometheusLabelParams | null | undefined>
	} = {}

	mock.mockReturnValue({
		usePrometheusMetadata: () => api.metadata,
		usePrometheusLabels: (
			_id: unknown,
			params: MaybeRefOrGetter<PrometheusLabelParams | null | undefined>,
		) => {
			captured.labelsParams = params

			return api.labels
		},
		usePrometheusLabelValues: (
			_id: unknown,
			params: Ref<PrometheusLabelValuesParams | null | undefined>,
		) => {
			captured.labelValuesParams = params

			return api.labelValues
		},
		usePrometheusSeries: () => api.series,
	})

	return { api, captured }
}

export interface SQLStubData {
	metadata?: SQLMetadataResult | null
	labels?: SQLLabelsResult | null
}

// same contract as stubPrometheusAPI, for the SQL data source API
export function stubSQLAPI(mock: Mock, data: SQLStubData = {}) {
	const api = {
		metadata: {
			data: ref<SQLMetadataResult | null>(data.metadata ?? null),
			refresh: vi.fn(),
		},
		labels: { refresh: vi.fn().mockResolvedValue({ data: data.labels }) },
	}

	const captured: {
		labelsParams?: MaybeRefOrGetter<SQLLabelsParams | null | undefined>
	} = {}

	mock.mockReturnValue({
		useSQLMetadata: () => api.metadata,
		useSQLLabels: (
			_id: unknown,
			params: MaybeRefOrGetter<SQLLabelsParams | null | undefined>,
		) => {
			captured.labelsParams = params

			return api.labels
		},
	})

	return { api, captured }
}
