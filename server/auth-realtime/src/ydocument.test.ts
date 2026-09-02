import { describe, it } from "vitest"
import * as Y from "yjs"
import { fragmentXml } from "./test-helpers.js"
import {
	cloneXmlElement,
	isSystemContext,
	replaceYdocContent,
	transformer,
	type DocumentData,
} from "./ydocument.js"

function paragraph(uid: string, text: string) {
	return {
		type: "paragraph",
		attrs: { uid },
		content: [{ type: "text", text }],
	}
}

function documentData(overrides: Partial<DocumentData> = {}): DocumentData {
	return {
		name: "Runbook",
		content: {
			type: "doc",
			content: [paragraph("p1", "Restart the service")],
		},
		icon: "lucide:file",
		...overrides,
	}
}

// the serialized content fragment, which is where duplication shows up
function contentXml(ydoc: Y.Doc): string {
	return fragmentXml(ydoc, "content")
}

// yjs refuses to read a type that is not integrated into a document, so
// both the elements under test and the clones taken from them are attached
// to a doc of their own before anything inspects them
function attached(element: Y.XmlElement): Y.XmlElement {
	new Y.Doc().getXmlFragment("content").insert(0, [element])

	return element
}

// Y.XmlElement.setAttribute is declared as accepting strings, but the
// runtime stores arbitrary values — which is exactly what the clone has to
// carry over
function setAnyAttribute(
	element: Y.XmlElement,
	key: string,
	value: unknown,
): void {
	;(
		element as unknown as {
			setAttribute(key: string, value: unknown): void
		}
	).setAttribute(key, value)
}

// an element carrying the kind of non-string attribute Y.XmlElement.clone()
// drops — the reason cloneXmlElement exists at all
function elementWithComplexAttributes(): Y.XmlElement {
	const element = attached(new Y.XmlElement("metricBlock"))

	setAnyAttribute(element, "uid", "m1")
	setAnyAttribute(element, "queries", [{ expr: "up", legend: "up" }])
	setAnyAttribute(element, "config", { unit: "short", refresh: 30 })

	return element
}

describe("cloneXmlElement", () => {
	it("copies the source's node name", ({ expect }) => {
		const source = attached(new Y.XmlElement("calloutBlock"))

		expect(attached(cloneXmlElement(source)).nodeName).toBe(
			"calloutBlock",
		)
	})

	it("copies string attributes", ({ expect }) => {
		const source = attached(new Y.XmlElement("paragraph"))
		source.setAttribute("uid", "p1")

		const clone = attached(cloneXmlElement(source))

		expect(clone.getAttribute("uid")).toBe("p1")
	})

	// Y.XmlElement.clone() copies only string attributes, silently
	// dropping arrays and objects — which is what would strip a metric
	// block's queries on every branch fork and merge
	it("preserves array and object attributes", ({ expect }) => {
		const clone = attached(
			cloneXmlElement(elementWithComplexAttributes()),
		)

		expect(clone.getAttribute("queries")).toEqual([
			{ expr: "up", legend: "up" },
		])
		expect(clone.getAttribute("config")).toEqual({
			unit: "short",
			refresh: 30,
		})
	})

	it("deep-clones nested elements", ({ expect }) => {
		const source = attached(new Y.XmlElement("calloutBlock"))
		const child = new Y.XmlElement("paragraph")
		source.insert(0, [child])
		child.setAttribute("uid", "p1")

		const clone = attached(cloneXmlElement(source))
		const clonedChild = clone.get(0)

		expect(clonedChild).toBeInstanceOf(Y.XmlElement)
		expect((clonedChild as Y.XmlElement).getAttribute("uid")).toBe(
			"p1",
		)
	})

	it("clones text children with their formatting", ({ expect }) => {
		const source = attached(new Y.XmlElement("paragraph"))
		const text = new Y.XmlText()
		source.insert(0, [text])
		text.insert(0, "bold", { bold: true })

		const clone = attached(cloneXmlElement(source))
		const clonedText = clone.get(0) as Y.XmlText

		expect(clonedText).toBeInstanceOf(Y.XmlText)
		expect(clonedText.toDelta()).toEqual([
			{ insert: "bold", attributes: { bold: true } },
		])
	})

	it("returns an element with no children when the source has none", ({
		expect,
	}) => {
		const source = attached(new Y.XmlElement("horizontalRule"))

		expect(attached(cloneXmlElement(source)).length).toBe(0)
	})

	it("detaches the clone so mutating it leaves the source alone", ({
		expect,
	}) => {
		const source = attached(new Y.XmlElement("paragraph"))
		source.setAttribute("uid", "p1")

		const clone = attached(cloneXmlElement(source))
		clone.setAttribute("uid", "p2")

		expect(source.getAttribute("uid")).toBe("p1")
	})
})

describe("isSystemContext", () => {
	it("reads the mark core puts on its own transactions", ({ expect }) => {
		expect(isSystemContext({ system: true })).toBe(true)
	})

	it.for([
		{
			name: "a client connection's context",
			input: { session: {} },
		},
		{ name: "the mark set to false", input: { system: false } },
		{
			name: "the mark spelled as a string",
			input: { system: "true" },
		},
		{ name: "an empty context", input: {} },
		{ name: "no context at all", input: null },
		{ name: "an undefined context", input: undefined },
		{ name: "a context that is not an object", input: "system" },
	])(
		"reads $name as a change core did not make",
		({ input }, { expect }) => {
			expect(isSystemContext(input)).toBe(false)
		},
	)
})

describe("replaceYdocContent", () => {
	it("writes the name into the name fragment", ({ expect }) => {
		const ydoc = new Y.Doc()

		replaceYdocContent(ydoc, documentData({ name: "Runbook" }))

		expect(fragmentXml(ydoc, "name")).toContain("Runbook")
	})

	it("writes the content into the content fragment", ({ expect }) => {
		const ydoc = new Y.Doc()

		replaceYdocContent(ydoc, documentData())

		expect(contentXml(ydoc)).toContain("Restart the service")
	})

	it("writes the icon into the icon text", ({ expect }) => {
		const ydoc = new Y.Doc()

		replaceYdocContent(ydoc, documentData({ icon: "lucide:siren" }))

		expect(ydoc.getText("icon").toJSON()).toBe("lucide:siren")
	})

	it("clears the icon when the replacement has none", ({ expect }) => {
		const ydoc = new Y.Doc()
		replaceYdocContent(ydoc, documentData({ icon: "lucide:siren" }))

		replaceYdocContent(ydoc, documentData({ icon: "" }))

		expect(ydoc.getText("icon").toJSON()).toBe("")
	})

	// the whole reason this function exists instead of Y.applyUpdate:
	// merging two states with different client origins keeps both copies
	it("leaves one copy when the same content is applied twice", ({
		expect,
	}) => {
		const ydoc = new Y.Doc()

		replaceYdocContent(ydoc, documentData())
		const afterFirst = contentXml(ydoc)
		replaceYdocContent(ydoc, documentData())

		expect(contentXml(ydoc)).toEqual(afterFirst)
	})

	it("removes the previous content rather than appending to it", ({
		expect,
	}) => {
		const ydoc = new Y.Doc()
		replaceYdocContent(ydoc, documentData())

		replaceYdocContent(
			ydoc,
			documentData({
				content: {
					type: "doc",
					content: [
						paragraph(
							"p2",
							"Page the on-call",
						),
					],
				},
			}),
		)

		const text = contentXml(ydoc)
		expect(text).toContain("Page the on-call")
		expect(text).not.toContain("Restart the service")
	})

	it("replaces the name rather than appending to it", ({ expect }) => {
		const ydoc = new Y.Doc()
		replaceYdocContent(ydoc, documentData({ name: "Runbook" }))

		replaceYdocContent(ydoc, documentData({ name: "Playbook" }))

		const name = fragmentXml(ydoc, "name")
		expect(name).toContain("Playbook")
		expect(name).not.toContain("Runbook")
	})

	// content written by a doc with a different clientID is what a branch
	// merge carries. Cloning it in must not CRDT-merge with what is
	// already there.
	it("takes over content from a document with a different client id", ({
		expect,
	}) => {
		const source = new Y.Doc()
		replaceYdocContent(
			source,
			documentData({
				content: {
					type: "doc",
					content: [
						paragraph(
							"p9",
							"Merged from a branch",
						),
					],
				},
			}),
		)

		const target = new Y.Doc()
		replaceYdocContent(target, documentData())
		expect(target.clientID).not.toBe(source.clientID)

		replaceYdocContent(target, {
			name: "Runbook",
			content: transformer.fromYdoc(
				source,
				"content",
			) as unknown,
			icon: "lucide:file",
		})

		const text = contentXml(target)
		expect(text).toContain("Merged from a branch")
		expect(text).not.toContain("Restart the service")
	})

	// the persisted rawContent has to reload into an identical document,
	// because that is what stops a restart from creating a second clientID
	// whose state merges with the reconnecting clients'
	it("produces a state that reloads into an identical document", ({
		expect,
	}) => {
		const ydoc = new Y.Doc()
		replaceYdocContent(ydoc, documentData())

		const reloaded = new Y.Doc()
		Y.applyUpdate(reloaded, Y.encodeStateAsUpdate(ydoc))

		expect(contentXml(reloaded)).toEqual(contentXml(ydoc))
		expect(reloaded.getText("icon").toJSON()).toBe(
			ydoc.getText("icon").toJSON(),
		)
	})

	it("emits a single update for the whole replacement", ({ expect }) => {
		const ydoc = new Y.Doc()
		let updates = 0
		ydoc.on("update", () => {
			updates++
		})

		replaceYdocContent(ydoc, documentData())

		expect(updates).toBe(1)
	})
})
