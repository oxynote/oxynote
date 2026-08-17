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
})
