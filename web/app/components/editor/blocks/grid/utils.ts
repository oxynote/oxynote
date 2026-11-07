import {
	BLOCK_GRID_NAME,
	LEGACY_METRIC_GRID_NAME,
	LOG_BLOCK_NAME,
	METRIC_BLOCK_NAME,
} from "../node-names"

export enum GridItemWidth {
	Compact = "compact",
	Standard = "standard",
	Wide = "wide",
}

export const BLOCK_GRID_NAMES: ReadonlySet<string> = new Set([
	BLOCK_GRID_NAME,
	// legacy: documents may still hold metricGrid
	LEGACY_METRIC_GRID_NAME,
])

export const GRID_ITEM_NAMES: ReadonlySet<string> = new Set([
	METRIC_BLOCK_NAME,
	LOG_BLOCK_NAME,
])

// the width attribute every grid item declares
export function gridItemWidthAttribute() {
	return {
		default: GridItemWidth.Standard,
		parseHTML: (element: HTMLElement) =>
			(element.getAttribute("data-width") as GridItemWidth | null) ||
			GridItemWidth.Standard,
		renderHTML: (attributes: Record<string, any>) => ({
			"data-width": attributes.width as GridItemWidth,
		}),
	}
}
