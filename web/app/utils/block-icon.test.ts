import { describe, it } from "vitest"
import {
	CALLOUT_BLOCK_NAME,
	CODE_BLOCK_NAME,
	CODE_BLOCK_TITLE_NAME,
	FIGMA_BLOCK_NAME,
	FILE_BLOCK_NAME,
	IMAGE_BLOCK_NAME,
	MERMAID_BLOCK_NAME,
	METRIC_BLOCK_NAME,
	SPLIT_DOCUMENTATION_NAME,
} from "~/components/editor/blocks/node-names"
import { GenericQueryChartType } from "./api/data-source/generic-query"
import {
	BLOCK_ICONS,
	blockIcon,
	blockIconList,
	HEADING_LEVEL_ICONS,
	METRIC_CHART_ICONS,
} from "./block-icon"

describe("BLOCK_ICONS", () => {
	it.for([
		CALLOUT_BLOCK_NAME,
		CODE_BLOCK_NAME,
		CODE_BLOCK_TITLE_NAME,
		FIGMA_BLOCK_NAME,
		FILE_BLOCK_NAME,
		IMAGE_BLOCK_NAME,
		MERMAID_BLOCK_NAME,
		METRIC_BLOCK_NAME,
		SPLIT_DOCUMENTATION_NAME,
	])(
		"is keyed by the editor's own name for the %s node",
		(name, { expect }) => {
			expect(BLOCK_ICONS).toHaveProperty(name)
		},
	)
})

describe("METRIC_CHART_ICONS", () => {
	it("has an icon for every chart type", ({ expect }) => {
		expect(Object.keys(METRIC_CHART_ICONS).sort()).toEqual(
			Object.values(GenericQueryChartType).sort(),
		)
	})
})

describe("blockIcon", () => {
	it("answers with the icon of a node type it knows", ({ expect }) => {
		expect(blockIcon(IMAGE_BLOCK_NAME)).toBe(BLOCK_ICONS.imageBlock)
	})

	it("shows a node type it does not know as text", ({ expect }) => {
		expect(blockIcon("somethingNew")).toBe(BLOCK_ICONS.paragraph)
	})
})

describe("blockIconList", () => {
	it("lists every icon a block can be shown with", ({ expect }) => {
		expect(blockIconList()).toEqual(
			expect.arrayContaining([
				...Object.values(BLOCK_ICONS),
				...Object.values(HEADING_LEVEL_ICONS),
				...Object.values(METRIC_CHART_ICONS),
			]),
		)
	})

	it("lists an icon two blocks share only once", ({ expect }) => {
		const icons = blockIconList()

		expect(icons).toHaveLength(new Set(icons).size)
	})
})
