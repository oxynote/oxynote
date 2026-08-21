import { EditorView } from "codemirror"
import { EditorState } from "@codemirror/state"
import { usePromQLLegendExtension, usePromQLQueryExtension } from "./promql"
import type { TimeRangePreset } from "../../utils"
import { useSQLLegendExtension, useSQLQueryExtension } from "./sql"

export const codemirrorTheme = EditorView.theme({
	"&": {
		zIndex: "100 !important",
		fontSize: "0.8125rem !important", // text-2sm
		borderRadius: "0.1875rem !important", // needs to +- match the container
		backgroundColor: "transparent !important",
	},
	"&.cm-focused": {
		outline: "none !important",
	},
	"&.cm-focused .cm-scroller": {
		outline: "none !important",
	},
	"&.cm-focused .cm-content": {
		outline: "none !important",
	},
	"&.cm-focused .cm-selectionLayer": {
		outline: "none !important",
	},
	".cm-tooltip.cm-tooltip-autocomplete": {
		pointerEvents: "auto !important",
		border: "0.0625rem solid var(--border) !important",
		borderRadius: "0.3125rem !important",
		backgroundColor: "var(--popover) !important",
		boxShadow:
			"0 10px 15px -3px rgb(0 0 0 / 0.1), 0 4px 6px -4px rgb(0 0 0 / 0.1) !important",
		padding: "0.25rem !important",
		fontSize: "0.8125rem !important", // text-2sm
		color: "var(--popover-foreground) !important",
		outline: "none !important",
	},
	// option list
	".cm-tooltip-autocomplete ul": {
		padding: "0 !important",
		margin: "0 !important",
		maxHeight: "15rem !important",
		overflowY: "auto !important",
		color: "var(--popover-foreground) !important",
		outline: "none !important",
	},
	// each option
	".cm-tooltip-autocomplete li": {
		padding: "0.25rem 0.25rem !important",
		borderRadius: "0.25rem !important",
		cursor: "pointer !important",
		color: "var(--popover-foreground) !important",
		outline: "none !important",
	},
	// active / hovered option
	".cm-tooltip-autocomplete li[aria-selected]": {
		backgroundColor:
			"color-mix(in oklab, var(--accent) 50%, transparent) !important",
		color: "var(--accent-foreground) !important",
	},
	// secondary info (labels / types)
	".cm-tooltip-autocomplete li .cm-completionDetail": {
		fontSize: "0.8125rem !important", // text-2sm
		color: "var(--popover-foreground) !important",
	},
	".cm-tooltip-autocomplete li .cm-completionMatchedText": {
		textDecoration: "none !important",
		fontWeight: "600 !important",
	},
	// description box of the selected option
	".cm-tooltip-autocomplete .cm-completionInfo": {
		padding: "0.125rem 0.25rem !important",
		fontSize: "0.8125rem !important", // text-2sm
		color: "var(--popover-foreground) !important",
		backgroundColor: "var(--popover) !important",
		border: "0.0625rem solid var(--border) !important",
		borderRadius: "0.3125rem !important",
		boxShadow:
			"0 10px 15px -3px rgb(0 0 0 / 0.1), 0 4px 6px -4px rgb(0 0 0 / 0.1) !important",
	},
	".cm-tooltip-autocomplete .cm-completionDetail": {
		opacity: "0.6 !important",
	},
	".cm-tooltip-autocomplete .cm-completionIcon": {
		width: "1em !important",
		paddingRight: "0.4em !important",
		display: "inline-block !important",
		textAlign: "center !important",
		opacity: "0.6 !important",
		fontSize: "0.9em !important",
		lineHeight: "1 !important",
		color: "var(--popover-foreground) !important",
	},
	".cm-tooltip-autocomplete .cm-completionInfo-left": {
		marginRight: "0.25rem !important",
	},
	".cm-tooltip-autocomplete .cm-completionInfo-right": {
		marginLeft: "0.25rem !important",
	},
	".cm-completionIcon-function:after, .cm-completionIcon-method:after": {
		content: "'ƒ' !important",
	},
	".cm-completionIcon-class:after": {
		content: "'○' !important",
	},
	".cm-completionIcon-interface:after": {
		content: "'◌' !important",
	},
	".cm-completionIcon-variable:after": {
		content: "'x' !important",
	},
	".cm-completionIcon-constant:after": {
		content: "'π' !important",
	},
	".cm-completionIcon-type:after": {
		content: "'T' !important",
	},
	".cm-completionIcon-enum:after": {
		content: "'∑' !important",
	},
	".cm-completionIcon-property:after": {
		content: "'•' !important",
	},
	".cm-completionIcon-keyword:after": {
		content: "'#' !important",
	},
	".cm-completionIcon-namespace:after": {
		content: "'⌂' !important",
	},
	".cm-completionIcon-text:after": {
		content: "'abc' !important",
		fontSize: "0.6em !important",
		verticalAlign: "middle !important",
	},
})

// this extension forces an editor to be single-line.
export function singleLineExtension() {
	return EditorState.transactionFilter.of((tr) => {
		if (!tr.docChanged) {
			return tr
		}

		if (tr.newDoc.lines <= 1) {
			return tr
		}

		const lines: string[] = []
		for (let i = 1; i <= tr.newDoc.lines; i++) {
			lines.push(tr.newDoc.line(i).text)
		}

		const newText = lines.join(" ")

		// the replacement has to follow the original transaction: codemirror
		// resolves the first spec against the start document, and only honors
		// `sequential` from the second spec onwards
		return [
			tr,
			{
				changes: { from: 0, to: tr.newDoc.length, insert: newText },
				sequential: true,
			},
		]
	})
}

export function trimEditorWhitespace(view: EditorView) {
	const doc = view.state.doc.toString()
	const trimmed = doc.trim()

	if (trimmed !== doc) {
		view.dispatch({
			changes: { from: 0, to: doc.length, insert: trimmed },
		})
	}
}

export function useQueryEditorExtension(
	t: (key: string) => string,
	dataSourceId: MaybeRefOrGetter<string | null | undefined>,
	timeRange: MaybeRefOrGetter<TimeRangePreset | null | undefined>,
	isEditingEnabled: MaybeRefOrGetter<boolean | null | undefined>,
) {
	const { fetchDataSources } = useDataSourceAPI()
	const selectedDataSourceType = computed(() => {
		const id = toValue(dataSourceId)
		return (
			fetchDataSources.state.value.data?.find((v) => v.id === id)?.type || null
		)
	})
	const promqlExtension = usePromQLQueryExtension(
		t,
		dataSourceId,
		timeRange,
		() => selectedDataSourceType.value === DataSourceType.Prometheus,
	)
	const sqlExtension = useSQLQueryExtension(
		t,
		dataSourceId,
		selectedDataSourceType,
		timeRange,
		() => isDataSourceSQLBased(selectedDataSourceType.value),
	)

	return {
		extensions: computed(() => {
			switch (selectedDataSourceType.value) {
				case DataSourceType.Prometheus:
					return promqlExtension.extensions.value
				default:
					if (isDataSourceSQLBased(selectedDataSourceType.value)) {
						return sqlExtension.extensions.value
					}
			}

			return undefined
		}),
		placeholder: computed(() => {
			if (!toValue(isEditingEnabled)) {
				return t(`editor.metrics.config.query-placeholder.empty`)
			}

			return t(
				`editor.metrics.config.query-placeholder.${selectedDataSourceType.value || "default"}`,
			)
		}),
	}
}

export function useLegendEditorExtension(
	query: MaybeRefOrGetter<string | null | undefined>,
	dataSourceId: MaybeRefOrGetter<string | null | undefined>,
	timeRange: MaybeRefOrGetter<TimeRangePreset | null | undefined>,
) {
	const { fetchDataSources } = useDataSourceAPI()
	const selectedDataSourceType = computed(() => {
		const id = toValue(dataSourceId)
		return (
			fetchDataSources.state.value.data?.find((v) => v.id === id)?.type || null
		)
	})
	const promqlExtension = usePromQLLegendExtension(
		query,
		dataSourceId,
		timeRange,
		() => selectedDataSourceType.value === DataSourceType.Prometheus,
	)
	const sqlExtension = useSQLLegendExtension(
		query,
		dataSourceId,
		timeRange,
		() => isDataSourceSQLBased(selectedDataSourceType.value),
	)

	async function fetchAllLabelNames() {
		switch (selectedDataSourceType.value) {
			case DataSourceType.Prometheus:
				return await promqlExtension.fetchAllLabelNames()
			default:
				if (isDataSourceSQLBased(selectedDataSourceType.value)) {
					return await sqlExtension.fetchAllLabelNames()
				}

				return []
		}
	}

	async function fetchExampleLabelValues(): Promise<Record<string, string>> {
		switch (selectedDataSourceType.value) {
			case DataSourceType.Prometheus:
				return await promqlExtension.fetchExampleLabelValues()
			default:
				if (isDataSourceSQLBased(selectedDataSourceType.value)) {
					return await sqlExtension.fetchExampleLabelValues()
				}

				return {}
		}
	}

	return {
		fetchAllLabelNames,
		fetchExampleLabelValues,
	}
}
