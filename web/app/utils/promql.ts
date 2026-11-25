import { parser, VectorSelector } from "@prometheus-io/lezer-promql"
import type { SyntaxNode } from "@lezer/common"

export type PromQLDialect = "promql" | "metricsql"

export const metricsqlParser = parser.configure({ dialect: "metricsql" })

/**
 * extracts all vector selectors (series selectors) from a PromQL expression.
 * e.g. `rate(http_requests_total{job="api"}[5m])` → `["http_requests_total{job=\"api\"}"]`
 *
 * these selectors are valid for use as Prometheus `match[]` parameters. A
 * MetricsQL selector that uses a WITH template is left out, because only
 * the whole query gives the template its meaning.
 */
export function extractPromQLSelectors(
	query: string,
	dialect: PromQLDialect = "promql",
): string[] {
	const tree = (dialect === "metricsql" ? metricsqlParser : parser).parse(query)
	const selectors: string[] = []

	tree.cursor().iterate((node) => {
		if (node.type.id !== VectorSelector) {
			return
		}

		if (!usesTemplate(node.node)) {
			selectors.push(query.slice(node.from, node.to))
		}

		return false // don't descend into children
	})

	return selectors
}

function usesTemplate(node: SyntaxNode): boolean {
	for (let child = node.firstChild; child; child = child.nextSibling) {
		if (child.name === "TemplateReference" || usesTemplate(child)) {
			return true
		}
	}

	return false
}
