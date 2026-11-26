import { describe, it } from "vitest"
import { extractPromQLSelectors } from "./promql"

describe("extractPromQLSelectors", () => {
	it.for([
		{
			name: "extracts a bare metric name",
			query: "up",
			expected: ["up"],
		},
		{
			name: "extracts a selector with label matchers from a function call",
			query: 'rate(http_requests_total{job="api"}[5m])',
			expected: ['http_requests_total{job="api"}'],
		},
		{
			name: "extracts every selector of a binary expression",
			query: 'a{env="prod"} + b',
			expected: ['a{env="prod"}', "b"],
		},
		{
			name: "extracts a selector with a dynamic duration placeholder",
			query: "rate(http_requests_total[$__interval])",
			expected: ["http_requests_total"],
		},
		{
			name: "returns no selectors for an empty query",
			query: "",
			expected: [],
		},
	])("$name", ({ query, expected }, { expect }) => {
		expect(extractPromQLSelectors(query)).toEqual(expected)
	})

	it.for([
		{
			name: "extracts selectors that only MetricsQL parses",
			query: 'rate(foo{a="b" or c="d"}) + foo.bar',
			expected: ['foo{a="b" or c="d"}', "foo.bar"],
		},
		{
			name: "leaves out a WITH template used as a metric",
			query: 'WITH (m = up{job="api"}, f(x) = rate(x[5m])) m + f(foo)',
			expected: ['up{job="api"}', "foo"],
		},
		{
			name: "keeps a metric named like a template parameter outside its template",
			query: "WITH (f(up) = rate(up[5m])) f(foo) + up",
			expected: ["foo", "up"],
		},
		{
			name: "leaves out a selector that uses a WITH template",
			query: 'WITH (f = {job="api"}) rate(foo{f}[5m])',
			expected: ['{job="api"}'],
		},
	])("$name with the metricsql dialect", ({ query, expected }, { expect }) => {
		expect(extractPromQLSelectors(query, "metricsql")).toEqual(expected)
	})
})
