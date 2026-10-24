// shared helpers for the metric block component suites. Test-only: the
// app/**/test-helpers.ts coverage exclude keeps this out of the
// denominator, and nothing here is imported by app code.
import { TimeRangePreset } from "~/utils/time-range"

export { stubThemeColorContext as stubChartColorContext } from "../../test-helpers/theme"

// the built-in group, with the user's own time range and the default
// refresh interval, beside a text variable on its default
export function variableTree(): VariableTreeResponse {
	const variable = {
		organizationId: "org-1",
		groupId: "group-1",
		description: null,
		sortIndex: 0,
		createdAt: "2026-01-01T00:00:00Z",
		createdBy: null,
	}

	return [
		{
			id: "group-1",
			organizationId: "org-1",
			groupName: "Metrics",
			color: "#000000",
			sortIndex: 0,
			builtin: true,
			createdAt: "2026-01-01T00:00:00Z",
			createdBy: null,
			variables: [
				{
					...variable,
					id: "var-range",
					key: "metric_range",
					name: "Metric time range",
					type: "time-range",
					defaultValue: TimeRangePreset.Last24Hours,
					builtin: true,
					value: TimeRangePreset.Last7Days,
					options: [],
				},
				{
					...variable,
					id: "var-refresh",
					key: "metric_refresh",
					name: "Metric refresh interval",
					type: "list",
					defaultValue: "opt-5m",
					builtin: true,
					value: null,
					options: [
						{
							id: "opt-5m",
							variableId: "var-refresh",
							value: "5m",
							sortIndex: 0,
						},
						{
							id: "opt-1m",
							variableId: "var-refresh",
							value: "1m",
							sortIndex: 1,
						},
					],
				},
				{
					...variable,
					id: "var-env",
					key: "env",
					name: "Environment",
					type: "text",
					defaultValue: "prod",
					builtin: false,
					value: null,
					options: [],
				},
			],
		},
	]
}
