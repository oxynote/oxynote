import { GenericQueryChartType } from "./api/data-source/generic-query"

// the icon each kind of block is shown with, by its node type. The icon
// scanner reads .vue files only and so misses the names here, which is why
// nuxt.config.ts bundles the whole list.
export const BLOCK_ICONS = {
	paragraph: "lucide:text",
	heading: "lucide:heading",
	bulletList: "lucide:list",
	orderedList: "lucide:list-ordered",
	taskList: "lucide:list-checks",
	horizontalRule: "lucide:minus",
	codeBlock: "lucide:square-code",
	codeBlockTitle: "lucide:square-code",
	calloutBlock: "lucide:square-m",
	imageBlock: "lucide:image",
	fileBlock: "mingcute:attachment-line",
	figmaBlock: "simple-icons:figma",
	mermaidBlock: "lucide:network",
	splitDocumentation: "lucide:square-split-horizontal",
	metricBlock: "lucide:chart-line",
} as const

export const HEADING_LEVEL_ICONS = {
	1: "lucide:heading-1",
	2: "lucide:heading-2",
	3: "lucide:heading-3",
} as const

export const METRIC_CHART_ICONS: Record<GenericQueryChartType, string> = {
	[GenericQueryChartType.Line]: "lucide:chart-line",
	[GenericQueryChartType.Bar]: "lucide:bar-chart-3",
	[GenericQueryChartType.Gauge]: "lucide:gauge",
}

// blockIcon answers for a node type that is only known at run time. A type
// without an icon of its own is shown as text.
export function blockIcon(nodeType: string): string {
	return (
		(BLOCK_ICONS as Record<string, string>)[nodeType] ?? BLOCK_ICONS.paragraph
	)
}

export function blockIconList(): string[] {
	return [
		...new Set([
			...Object.values(BLOCK_ICONS),
			...Object.values(HEADING_LEVEL_ICONS),
			...Object.values(METRIC_CHART_ICONS),
		]),
	]
}
