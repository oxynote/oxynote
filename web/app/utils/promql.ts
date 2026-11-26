import { parser, VectorSelector } from "@prometheus-io/lezer-promql"
import type { SyntaxNode, Tree } from "@lezer/common"

export type PromQLDialect = "promql" | "metricsql"

export const metricsqlParser = parser.configure({ dialect: "metricsql" })

/**
 * extracts all vector selectors (series selectors) from a PromQL expression.
 * e.g. `rate(http_requests_total{job="api"}[5m])` → `["http_requests_total{job=\"api\"}"]`
 *
 * these selectors are valid for use as Prometheus `match[]` parameters. A
 * MetricsQL selector that is or uses a WITH template is left out, because
 * only the whole query gives the template its meaning.
 */
export function extractPromQLSelectors(
	query: string,
	dialect: PromQLDialect = "promql",
): string[] {
	const tree = (dialect === "metricsql" ? metricsqlParser : parser).parse(query)
	const templates = templateNames(tree, query)
	const selectors: string[] = []

	tree.cursor().iterate((node) => {
		if (node.type.id !== VectorSelector) {
			return
		}

		const name = node.node.firstChild
		const isTemplate = !!name && templates.has(query.slice(name.from, name.to))
		if (!isTemplate && !usesTemplate(node.node)) {
			selectors.push(query.slice(node.from, node.to))
		}

		return false // don't descend into children
	})

	return selectors
}

// templateNames returns the names that the query's WITH bindings define,
// with the parameters of the templates that take arguments.
function templateNames(tree: Tree, query: string): Set<string> {
	const names = new Set<string>()

	tree.iterate({
		enter: (node) => {
			const isName =
				(node.node.parent?.name === "WithBinding" &&
					node.node.prevSibling === null) ||
				node.node.parent?.name === "WithParams"
			if (isName) {
				names.add(query.slice(node.from, node.to))
			}
		},
	})

	return names
}

function usesTemplate(node: SyntaxNode): boolean {
	for (let child = node.firstChild; child; child = child.nextSibling) {
		if (child.name === "TemplateReference" || usesTemplate(child)) {
			return true
		}
	}

	return false
}
