<script lang="ts" setup>
import type { Node as PMNode } from "@tiptap/pm/model"
import {
	collectAttributeChanges,
	displayValue,
	formatChange,
	type AttributeChange,
	type FieldChange,
} from "~/components/editor/diff/change-count"
import DiffFieldChanges from "~/components/editor/diff/DiffFieldChanges.vue"
import {
	METRIC_REFRESH_INTERVAL_REFERENCE,
	METRIC_TIME_RANGE_REFERENCE,
} from "~/utils/variable"
import {
	VisualizationDataUnit,
	VisualizationMiscUnit,
	VisualizationTimeUnit,
} from "./utils"

// matches the per-field units of queries, such as "queries.1.legendFormat"
const QUERY_FIELD_RE = /^queries\.(\d+)\.(query|legendFormat)$/

const UNIT_CATEGORIES: Record<string, string> = Object.fromEntries([
	...Object.values(VisualizationTimeUnit).map(unitCategory("time")),
	...Object.values(VisualizationDataUnit).map(unitCategory("data")),
	...Object.values(VisualizationMiscUnit).map(unitCategory("misc")),
])

const props = defineProps<{
	node: PMNode
}>()

const { t } = useI18n({ useScope: "global" })
const { fetchDataSources } = useDataSourceAPI()

const rows = computed<FieldChange[]>(() =>
	collectAttributeChanges(props.node).map(toRow),
)

function toRow(change: AttributeChange): FieldChange {
	const queryField = QUERY_FIELD_RE.exec(change.name)
	if (queryField) {
		const index = Number(queryField[1]) + 1
		const label =
			queryField[2] === "query"
				? t("editor.metrics.config.query-name-default-format", { index })
				: t("editor.metrics.diff.legend", { index })

		// an empty legend falls back to the series labels, which the config
		// modal calls auto
		const format = (value: unknown) =>
			value === "" && queryField[2] === "legendFormat"
				? t("editor.diff-change-marker.auto")
				: displayValue(value)

		return { label: label, ...formatChange(change, format), code: true }
	}

	switch (change.name) {
		case "title":
			return row(t("editor.metrics.config.title-label"), change)
		case "dataSourceId":
			return row(
				t("editor.config-modal-utils.data-source-label"),
				change,
				dataSourceName,
			)
		case "visualizationType":
			return row(t("editor.metrics.diff.visualization"), change, (value) =>
				t(
					`editor.metrics.config.type-options.${displayValue(value).replace("_", "-")}.title`,
				),
			)
		case "timeRange":
			return row(
				t("editor.config-modal-utils.time-range-label"),
				change,
				(value) =>
					value === METRIC_TIME_RANGE_REFERENCE
						? t("editor.config-modal-utils.variable-time-range")
						: t(`general.time-range-options.${displayValue(value)}`),
			)
		case "refreshInterval":
			return row(
				t("editor.config-modal-utils.refresh-interval-label"),
				change,
				(value) =>
					value === METRIC_REFRESH_INTERVAL_REFERENCE
						? t("editor.config-modal-utils.variable-refresh-interval")
						: t(`general.refresh-interval-options.${displayValue(value)}`),
			)
		case "thresholds":
			return row(t("editor.metrics.config.thresholds-label"), change, (value) =>
				String((value as unknown[]).length),
			)
		case "baseThresholdColor":
			return row(t("editor.metrics.diff.base-threshold-color"), change)
		case "decimals":
			return autoRow(t("editor.metrics.config.decimals-label"), change)
		case "unitType":
			return row(t("editor.metrics.config.unit-label"), change, unitName)
		case "unitCustom":
			return row(t("editor.metrics.diff.custom-unit"), change)
		case "axisBoundsMin":
			return autoRow(t("editor.metrics.config.bounds-min-label"), change)
		case "axisBoundsMax":
			return autoRow(t("editor.metrics.config.bounds-max-label"), change)
		case "simulationPreset":
			return row(t("editor.metrics.simulation.preset-label"), change, (value) =>
				t(`editor.metrics.simulation.preset-options.${displayValue(value)}`),
			)
		case "width":
			return row(t("editor.metrics.diff.size"), change, (value) =>
				t(
					`editor.drag-handle.options.grid-item.width-options.${displayValue(value)}`,
				),
			)
		default:
			// an attribute this card has no label for still shows, so the
			// card lists every change the marker counts
			return row(change.name, change)
	}
}

// a setting always has a value, so its unset side shows as empty
function row(
	label: string,
	change: AttributeChange,
	format?: (value: unknown) => string,
): FieldChange {
	return { label: label, ...formatChange(change, format, "") }
}

// a setting whose unset value lets the chart pick one, as its placeholder
// in the config modal says
function autoRow(label: string, change: AttributeChange): FieldChange {
	return {
		label: label,
		...formatChange(change, displayValue, t("editor.diff-change-marker.auto")),
	}
}

function unitCategory(category: string) {
	return (unit: string): [string, string] => [unit, category]
}

function dataSourceName(id: unknown): string {
	const dataSource = fetchDataSources.state.value.data?.find(
		(source) => source.id === id,
	)

	return (
		dataSource?.name ?? t("editor.config-modal-utils.data-source-label-deleted")
	)
}

function unitName(value: unknown): string {
	const unit = displayValue(value)
	const category = UNIT_CATEGORIES[unit]

	return category
		? t(`editor.metrics.config.unit-options.${category}.options.${unit}`)
		: t(`editor.metrics.config.unit-options.${unit}`)
}
</script>

<template>
	<DiffFieldChanges class="w-80" :rows="rows" />
</template>
