import * as Y from "yjs"
import { ProsemirrorTransformer } from "@hocuspocus/transformer"
import { getSchema } from "@tiptap/core"
import { getEditorExtensions } from "./schema/index.js"

export interface DocumentData {
	name: string
	content: any
	icon: string
}

const schema = getSchema(getEditorExtensions())
const transformer = ProsemirrorTransformer.schema(schema)

/**
 * Deep-clones a Y.XmlElement, preserving all attribute types (including arrays
 * and objects). Y.XmlElement.clone() only copies string attributes, silently
 * dropping complex attributes like the `queries` array on metricBlock.
 */
export function cloneXmlElement(source: Y.XmlElement): Y.XmlElement {
	const el = new Y.XmlElement(source.nodeName)
	const attrs = source.getAttributes()
	for (const [key, value] of Object.entries(attrs)) {
		if (value === undefined) {
			// NOCOV: getAttributes never yields an undefined value for a
			// key it reports, so this only guards a future yjs change.
			continue
		}

		// the default attribute type is string, but attrs hold any JSON.
		;(el as Y.XmlElement<Record<string, any>>).setAttribute(
			key,
			value,
		)
	}

	const children = source.toArray()
	if (children.length > 0) {
		el.insert(
			0,
			children.map((child) => {
				if (child instanceof Y.XmlElement) {
					return cloneXmlElement(child)
				}
				const newText = new Y.XmlText()
				// Y.XmlText.toDelta is declared as returning
				// `any`, while applyDelta wants a delta list.
				newText.applyDelta(
					(child as Y.XmlText).toDelta() as any[],
				)
				return newText
			}),
		)
	}
	return el
}

// cloneXmlFragment replaces target's children with deep clones of
// source's.
export function cloneXmlFragment(
	source: Y.XmlFragment,
	target: Y.XmlFragment,
): void {
	target.delete(0, target.length)
	for (let i = 0; i < source.length; i++) {
		const item = source.get(i)
		if (item instanceof Y.XmlElement) {
			target.insert(i, [cloneXmlElement(item)])
		} else if (item instanceof Y.XmlText) {
			const newText = new Y.XmlText()
			newText.applyDelta(item.toDelta() as any[])
			target.insert(i, [newText])
		}
	}
}

function toNameContent(name: string) {
	return {
		type: "doc",
		content: [
			{
				type: "paragraph",
				content: [{ type: "text", text: name }],
			},
		],
	}
}

// systemOrigin marks a transaction as core's own rather than a person's.
// Hocuspocus hands the origin's context to onChange and onStoreDocument,
// which is how a persist knows whether it may touch a protected branch.
export const systemOrigin = { source: "local", context: { system: true } }

// isSystemContext reads that mark back off whatever context a hook was
// handed. A client connection's context carries a session instead, and
// anything else is a change core did not make.
export function isSystemContext(context: unknown): boolean {
	return (
		!!context &&
		typeof context === "object" &&
		(context as { system?: unknown }).system === true
	)
}

/**
 * Replaces document data in a Y.Doc, fully overwriting existing content.
 * Uses direct cloning to avoid applyUpdate merge semantics that can cause duplication.
 */
export function replaceYdocContent(ydoc: Y.Doc, data: DocumentData): void {
	ydoc.transact(() => {
		const nameDoc = transformer.toYdoc(
			toNameContent(data.name),
			"name",
		)
		cloneXmlFragment(
			nameDoc.getXmlFragment("name"),
			ydoc.getXmlFragment("name"),
		)

		const contentDoc = transformer.toYdoc(data.content, "content")
		cloneXmlFragment(
			contentDoc.getXmlFragment("content"),
			ydoc.getXmlFragment("content"),
		)

		const iconText = ydoc.getText("icon")
		iconText.delete(0, iconText.length)
		iconText.insert(0, data.icon)
	})
}

export { schema, transformer }
