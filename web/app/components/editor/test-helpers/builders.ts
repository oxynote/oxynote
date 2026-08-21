import type { JSONContent } from "@tiptap/core"
import type { Attrs, Node as PMNode, Schema } from "@tiptap/pm/model"
import { nodeType } from "./schema"

export function paragraph(
	text?: string,
	attrs?: JSONContent["attrs"],
): JSONContent {
	return {
		type: "paragraph",
		attrs,
		content: text ? [{ type: "text", text }] : undefined,
	}
}

export function text(
	value: string,
	marks?: NonNullable<JSONContent["marks"]>,
): JSONContent {
	return marks === undefined
		? { type: "text", text: value }
		: { type: "text", text: value, marks }
}

export function doc(...content: JSONContent[]): JSONContent {
	return { type: "doc", content }
}

// builds nodes tersely: string children become text nodes
export function attrBlockBuilder(schema: Schema) {
	return (
		name: string,
		attrs: Attrs | null,
		...children: (PMNode | string)[]
	): PMNode =>
		nodeType(schema, name).create(
			attrs,
			children.map((child) =>
				typeof child === "string" ? schema.text(child) : child,
			),
		)
}

// the same, for nodes whose attributes never matter to the suite
export function blockBuilder(schema: Schema) {
	const build = attrBlockBuilder(schema)

	return (name: string, ...children: (PMNode | string)[]): PMNode =>
		build(name, null, ...children)
}

// creates the doc node the given blocks belong to, so a suite holding
// documents from more than one schema gets the right one per document.
// The argument is only the fallback for an empty document.
export function docBuilder(fallback: Schema) {
	return (...blocks: PMNode[]): PMNode =>
		nodeType(blocks[0]?.type.schema ?? fallback, "doc").create(null, blocks)
}

export function jsonDocBuilder(schema: Schema) {
	return (...content: JSONContent[]): PMNode =>
		schema.nodeFromJSON({ type: "doc", content })
}
