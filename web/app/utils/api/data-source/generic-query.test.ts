import { describe, it } from "vitest"
import { formatQueryTimeRange } from "./generic-query"

describe("formatQueryTimeRange", () => {
	it.for([
		{
			name: "returns no params without a time range",
			timeRange: undefined,
			expected: {},
		},
		{
			name: "returns no params for an empty time range",
			timeRange: {},
			expected: {},
		},
		{
			name: "converts Date bounds to ISO strings",
			timeRange: {
				from: new Date("2024-06-15T12:00:00Z"),
				to: new Date("2024-06-15T13:00:00Z"),
			},
			expected: {
				from: "2024-06-15T12:00:00.000Z",
				to: "2024-06-15T13:00:00.000Z",
			},
		},
		{
			name: "passes string bounds through unchanged",
			timeRange: { from: "now-1h", to: "now" },
			expected: { from: "now-1h", to: "now" },
		},
		{
			name: "includes only the bound that is set",
			timeRange: { to: "now" },
			expected: { to: "now" },
		},
	])("$name", ({ timeRange, expected }, { expect }) => {
		expect(formatQueryTimeRange(timeRange)).toEqual(expected)
	})
})
