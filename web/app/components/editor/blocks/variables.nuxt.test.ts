import { afterEach, beforeEach, describe, it } from "vitest"
import { effectScope, type EffectScope } from "vue"
import {
	clearQueryCache,
	disposeMockEndpoints,
	mockDeferredEndpoint,
	runInApp,
	seedQueryData,
} from "~/composables/api/test-helpers"
import { VARIABLE_QUERY_KEYS } from "~/composables/api/useVariableAPI"
import { RefreshInterval, TimeRangePreset } from "~/utils/time-range"
import {
	METRIC_REFRESH_INTERVAL_REFERENCE,
	METRIC_TIME_RANGE_REFERENCE,
} from "~/utils/variable"
import { variableTree } from "./metrics/test-helpers"
import { useBlockVariables } from "./variables"

const TREE_URL = "/api/variables/tree"

let scope: EffectScope | null = null

// the tree is one app-wide cache entry, so the tests cannot interleave
describe("useBlockVariables", { concurrent: false }, () => {
	beforeEach(() => {
		clearQueryCache()
	})
	afterEach(() => {
		scope?.stop()
		scope = null
		disposeMockEndpoints()
	})

	describe("when the variables have loaded", () => {
		beforeEach(() => {
			seedQueryData(VARIABLE_QUERY_KEYS.tree, variableTree())
		})

		it("passes a fixed time range and refresh interval through", ({
			expect,
		}) => {
			const metric = use()

			expect(metric.timeRange(TimeRangePreset.Last1Hour)).toBe(
				TimeRangePreset.Last1Hour,
			)
			expect(metric.refreshInterval(RefreshInterval.S30)).toBe(
				RefreshInterval.S30,
			)
			expect(metric.timeRange(null)).toBeNull()
		})

		it("resolves the built-in time range to the user's value", ({ expect }) => {
			const metric = use()

			expect(metric.timeRange(METRIC_TIME_RANGE_REFERENCE)).toBe(
				TimeRangePreset.Last7Days,
			)
		})

		it("resolves the built-in refresh interval to its default option", ({
			expect,
		}) => {
			const metric = use()

			expect(metric.refreshInterval(METRIC_REFRESH_INTERVAL_REFERENCE)).toBe(
				RefreshInterval.M5,
			)
		})

		it("maps the values the queries reference", ({ expect }) => {
			const metric = use()

			expect(metric.queryValues(['up{env="$env"}', "$missing"])).toEqual({
				env: "prod",
			})
		})
	})

	describe("while the variables load", () => {
		beforeEach(() => {
			mockDeferredEndpoint("GET", TREE_URL)
		})

		it("resolves no built-in yet", ({ expect }) => {
			const metric = use()

			expect(metric.timeRange(METRIC_TIME_RANGE_REFERENCE)).toBeNull()
			expect(
				metric.refreshInterval(METRIC_REFRESH_INTERVAL_REFERENCE),
			).toBeNull()
		})

		it("holds back the values of queries with a reference", ({ expect }) => {
			const metric = use()

			expect(metric.queryValues(['up{env="$env"}'])).toBeNull()
		})

		it("maps no values for queries without a reference", ({ expect }) => {
			const metric = use()

			expect(metric.queryValues(["up"])).toEqual({})
		})
	})

	it("falls back to the seeded defaults without the built-ins", ({
		expect,
	}) => {
		seedQueryData(VARIABLE_QUERY_KEYS.tree, [])
		const metric = use()

		expect(metric.timeRange(METRIC_TIME_RANGE_REFERENCE)).toBe(
			TimeRangePreset.Last24Hours,
		)
		expect(metric.refreshInterval(METRIC_REFRESH_INTERVAL_REFERENCE)).toBe(
			RefreshInterval.M5,
		)
	})
})

function use() {
	scope = effectScope()

	const metric = scope.run(() => runInApp(() => useBlockVariables()))
	if (!metric) {
		throw new Error("the composable did not run")
	}

	return metric
}
