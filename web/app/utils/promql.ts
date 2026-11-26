import { parser, VectorSelector } from "@prometheus-io/lezer-promql"
import type { SyntaxNode } from "@lezer/common"

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
	const selectors: string[] = []

	tree.cursor().iterate((node) => {
		if (node.type.id !== VectorSelector) {
			return
		}

		const name = node.node.firstChild
		const isTemplate =
			!!name &&
			templatesInScope(node.node, query).has(query.slice(name.from, name.to))
		if (!isTemplate && !usesTemplate(node.node)) {
			selectors.push(query.slice(node.from, node.to))
		}

		return false // don't descend into children
	})

	return selectors
}

// templatesInScope returns the WITH names a node can see: the bindings of
// every WITH around it, and the parameters of the binding it is in. A
// parameter means nothing outside its binding, so the same name is a
// metric there.
function templatesInScope(node: SyntaxNode, query: string): Set<string> {
	const names = new Set<string>()

	for (let parent = node.parent; parent; parent = parent.parent) {
		if (parent.name === "WithExpr") {
			for (const binding of parent.getChildren("WithBinding")) {
				const name = binding.firstChild
				if (name) {
					names.add(query.slice(name.from, name.to))
				}
			}
		}

		const params =
			parent.name === "WithBinding" && parent.getChild("WithParams")
		for (
			let param = params ? params.firstChild : null;
			param;
			param = param.nextSibling
		) {
			names.add(query.slice(param.from, param.to))
		}
	}

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
