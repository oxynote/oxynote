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
import { gridItemRow } from "~/components/editor/diff/grid-place"
import {
	METRIC_REFRESH_INTERVAL_REFERENCE,
	METRIC_TIME_RANGE_REFERENCE,
} from "~/utils/variable"

// matches the per-field units of queries, such as "queries.1.query"
const QUERY_FIELD_RE = /^queries\.(\d+)\.query$/

const props = defineProps<{
	node: PMNode
}>()

const { t, locale } = useI18n({ useScope: "global" })
const { fetchDataSources } = useDataSourceAPI()

const rows = computed<FieldChange[]>(() =>
	collectAttributeChanges(props.node).map(toRow),
)

function toRow(change: AttributeChange): FieldChange {
	const gridRow = gridItemRow(change, t, locale.value)
	if (gridRow) {
		return gridRow
	}

	const queryField = QUERY_FIELD_RE.exec(change.name)
	if (queryField) {
		const label = t("editor.logs.config.query-name-default-format", {
			index: Number(queryField[1]) + 1,
		})

		return { label: label, ...formatChange(change), code: true }
	}

	switch (change.name) {
		case "title":
			return row(t("editor.logs.config.title-label"), change)
		case "dataSourceId":
			return row(
				t("editor.config-modal-utils.data-source-label"),
				change,
				dataSourceName,
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
		case "timestampFormat":
			return row(
				t("editor.logs.config.timestamp-format-label"),
				change,
				(value) =>
					t(
						`editor.logs.config.timestamp-format-options.${displayValue(value)}.title`,
					),
			)
		case "order":
			return row(t("editor.logs.config.order-label"), change, (value) =>
				t(`editor.logs.config.order-options.${displayValue(value)}`),
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

function dataSourceName(id: unknown): string {
	const dataSource = fetchDataSources.state.value.data?.find(
		(source) => source.id === id,
	)

	return (
		dataSource?.name ?? t("editor.config-modal-utils.data-source-label-deleted")
	)
}
</script>

<template>
	<DiffFieldChanges class="w-80" :rows="rows" />
</template>
