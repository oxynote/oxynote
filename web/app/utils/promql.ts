import { parser, VectorSelector } from "@prometheus-io/lezer-promql"

/**
 * extracts all vector selectors (series selectors) from a PromQL expression.
 * e.g. `rate(http_requests_total{job="api"}[5m])` → `["http_requests_total{job=\"api\"}"]`
 *
 * these selectors are valid for use as Prometheus `match[]` parameters.
 */
export function extractPromQLSelectors(query: string): string[] {
	const tree = parser.parse(query)
	const selectors: string[] = []

	tree.cursor().iterate((node) => {
		if (node.type.id === VectorSelector) {
			selectors.push(query.slice(node.from, node.to))

			return false // don't descend into children
		}
	})

	return selectors
}
