import * as Y from "yjs"
import { TiptapTransformer } from "@hocuspocus/transformer"
import { getEditorExtensions } from "./schema/index.js"

export interface DocumentData {
	name: string
	content: any
	icon: string
}

// Create a TiptapTransformer instance with the bifrost schema
const extensions = getEditorExtensions()
const transformer = TiptapTransformer.extensions(extensions)

/**
 * Deep-clones a Y.XmlElement, preserving all attribute types (including arrays
 * and objects). Y.XmlElement.clone() only copies string attributes, silently
 * dropping complex attributes like the `queries` array on metricBlock.
 */
export function cloneXmlElement(source: Y.XmlElement): Y.XmlElement {
	const el = new Y.XmlElement(source.nodeName)
	// Y.XmlElement.setAttribute is declared as accepting `string`,
	// but the runtime stores arbitrary values which we rely on for
	// complex attributes (e.g. the queries array on metricBlock).
	// Cast through unknown to bypass the type declaration without
	// dropping non-string values.
	const setAttr = (
		el as unknown as { setAttribute(key: string, value: unknown): void }
	).setAttribute.bind(el)

	const attrs = source.getAttributes()
	for (const [key, value] of Object.entries(attrs)) {
		if (value === undefined) {
			continue
		}

		setAttr(key, value)
	}
	const children = source.toArray()
	if (children.length > 0) {
		el.insert(0, children.map(child => {
			if (child instanceof Y.XmlElement) {
				return cloneXmlElement(child)
			}
			const newText = new Y.XmlText()
			newText.applyDelta((child as Y.XmlText).toDelta())
			return newText
		}))
	}
	return el
}

function cloneXmlFragment(source: Y.XmlFragment, target: Y.XmlFragment): void {
	target.delete(0, target.length)
	for (let i = 0; i < source.length; i++) {
		const item = source.get(i)
		if (item instanceof Y.XmlElement) {
			target.insert(i, [cloneXmlElement(item)])
		} else if (item instanceof Y.XmlText) {
			const newText = new Y.XmlText()
			newText.applyDelta(item.toDelta())
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

/**
 * Replaces document data in a Y.Doc, fully overwriting existing content.
 * Uses direct cloning to avoid applyUpdate merge semantics that can cause duplication.
 */
export function replaceYdocContent(ydoc: Y.Doc, data: DocumentData): void {
	ydoc.transact(() => {
		const nameDoc = transformer.toYdoc(toNameContent(data.name), "name")
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

export { transformer }
