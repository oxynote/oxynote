import {
	BLOCK_GRID_NAME,
	LEGACY_METRIC_GRID_NAME,
	LOG_BLOCK_NAME,
	METRIC_BLOCK_NAME,
} from "../node-names"
import {
	GRID_DEFAULT_SIZE,
	gridItemOf,
	heightName,
	widthName,
} from "@oxynote/common/grid"

// a translate function, such as vue-i18n's t
export type Translate = (
	key: string,
	params?: Record<string, unknown>,
	plural?: number,
) => string

export const BLOCK_GRID_NAMES: ReadonlySet<string> = new Set([
	BLOCK_GRID_NAME,
	// legacy: documents may still hold metricGrid
	LEGACY_METRIC_GRID_NAME,
])

export const GRID_ITEM_NAMES: ReadonlySet<string> = new Set([
	METRIC_BLOCK_NAME,
	LOG_BLOCK_NAME,
])

// the width attribute every grid item declares. It holds the number of
// columns the block spans in the wide layout. An older block stores a name
// such as "compact" instead, and gridItemOf reads that name too.
export function gridItemWidthAttribute() {
	return {
		default: GRID_DEFAULT_SIZE.w,
		parseHTML: (element: HTMLElement) =>
			gridItemOf({ width: parsedWidth(element.getAttribute("data-width")) }).w,
		renderHTML: (attributes: Record<string, any>) => ({
			"data-width": attributes.width as number,
		}),
	}
}

// the height attribute every grid item declares. It holds the number of
// rows the block spans.
export function gridItemHeightAttribute() {
	return {
		default: GRID_DEFAULT_SIZE.h,
		parseHTML: (element: HTMLElement) =>
			gridItemOf({ height: Number(element.getAttribute("data-height")) }).h,
		renderHTML: (attributes: Record<string, any>) => ({
			"data-height": attributes.height as number,
		}),
	}
}

// the attributes that place a grid item. widePosX and widePosY hold its
// column and row in the wide layout, and both count from zero. stackPos
// holds its place in the stacked layout. A block saved before these
// attributes existed has none of them, so the grid lays it out by its
// width.
export function gridItemPlaceAttributes() {
	return {
		widePosX: gridCountAttribute("widePosX", "data-wide-pos-x"),
		widePosY: gridCountAttribute("widePosY", "data-wide-pos-y"),
		stackPos: gridCountAttribute("stackPos", "data-stack-pos"),
	}
}

// gridSizeLabel names a width in columns or a height in rows. When the
// menu names a size, the label uses that name. Any other size shows its
// count.
export function gridSizeLabel(
	t: Translate,
	side: "width" | "height",
	count: number,
): string {
	const name = side === "width" ? widthName(count) : heightName(count)
	if (name) {
		return t(`editor.grid-item.${side}-options.${name}`)
	}

	const unit = side === "width" ? "columns" : "rows"

	return t(`editor.grid-item.${side}-options.${unit}`, { count: count }, count)
}

function gridCountAttribute(name: string, dataAttribute: string) {
	return {
		default: null,
		parseHTML: (element: HTMLElement) => {
			const raw = element.getAttribute(dataAttribute)
			const value = raw ? Number(raw) : NaN

			return Number.isInteger(value) && value >= 0 ? value : null
		},
		renderHTML: (attributes: Record<string, any>) =>
			attributes[name] == null
				? {}
				: { [dataAttribute]: attributes[name] as number },
	}
}

// a width from HTML: a number of columns, or the name an older block wrote
function parsedWidth(raw: string | null): unknown {
	return raw !== null && /^\d+$/.test(raw) ? Number(raw) : raw
}
