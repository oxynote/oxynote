import { Node as TiptapNode } from "@tiptap/core"
import type { Attrs, MarkType, NodeType, Schema } from "@tiptap/pm/model"
import { METRIC_BLOCK_NAME, METRIC_GRID_NAME } from "../blocks/node-names"

// the suites build minimal schemas whose node names come from the real
// block extensions. Computed keys widen the schema's node map to a
// string index, so lookups need a throwing guard to stay type-safe
// under noUncheckedIndexedAccess.
export function nodeType(schema: Schema, name: string): NodeType {
	const type = schema.nodes[name]

	if (!type) {
		throw new Error(`node type "${name}" is missing from the test schema`)
	}

	return type
}

export function markType(schema: Schema, name: string): MarkType {
	const type = schema.marks[name]

	if (!type) {
		throw new Error(`mark type "${name}" is missing from the test schema`)
	}

	return type
}

// the attribute parsers only ever call getAttribute, so a plain lookup
// object stands in for the parsed element
export function parseAttributes(
	type: NodeType,
	attributes: Record<string, string>,
): Attrs | false | null | undefined {
	const rule = type.spec.parseDOM?.find((candidate) => candidate.getAttrs)

	if (!rule?.getAttrs) {
		throw new Error(`${type.name} declares no attribute parser`)
	}

	const element = {
		getAttribute: (name: string) => attributes[name] ?? null,
	} as unknown as HTMLElement

	return rule.getAttrs(element)
}

// the real metric extensions read theme colors off a canvas while
// building their attribute defaults, which no node environment can
// provide. The stand-ins carry the names, groups, and nesting rules the
// surrounding schema needs; a suite that also renders them extends
// these with a renderHTML of its own.
export const MetricGridStub = TiptapNode.create({
	name: METRIC_GRID_NAME,
	group: "block",
	content: `${METRIC_BLOCK_NAME}*`,
	isolating: true,
})

export const MetricBlockStub = TiptapNode.create({
	name: METRIC_BLOCK_NAME,
	group: METRIC_BLOCK_NAME,
	atom: true,
	isolating: true,
})
