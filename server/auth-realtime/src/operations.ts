/**
 * Operation interpreter for the Go-driven edit RPC.
 *
 * The Go assistant builds operations against the canonical block
 * model, expands them into ProseMirror JSON, and posts them to the
 * /api/x/documents/:docId/branches/:branchId/operations route. This
 * module owns the Y.XmlFragment mutations that apply those
 * operations to a live Y.Doc. Every applied batch runs inside a
 * single `Y.Doc.transact` so subscribers see one update.
 *
 * The operations themselves are intentionally small (single-block
 * atomic) because the AI fires them in parallel within one turn —
 * see docs/assistant-rework.md §2.1 for the schema.
 */
import * as Y from "yjs"
import { nanoid } from "nanoid"
import { transformer, cloneXmlElement } from "./ydocument.js"

/** Canonical attribute name carried on every editor block. */
const UID_ATTR = "uid"

/**
 * One batched operation against a document. Shapes mirror the Go
 * canonical/op definitions; the Node side only validates the
 * minimum required to apply each op.
 */
export type Operation =
	| InsertOp
	| AppendOp
	| PrependOp
	| ReplaceOp
	| UpdateTextOp
	| UpdateAttrsOp
	| DeleteOp
	| SetNameOp
	| SetIconOp

/** Inserts a new block immediately before/after a referenced block. */
export interface InsertOp {
	kind: "insert"
	/** Insertion side relative to the reference block. */
	position: "before" | "after"
	/** Reference block's uid in the document. */
	reference_uid: string
	/** New block as ProseMirror JSON, including a uid attribute. */
	block: PMNode
}

/** Appends a new block at the end of the document root. */
export interface AppendOp {
	kind: "append"
	block: PMNode
}

/** Prepends a new block at the start of the document root. */
export interface PrependOp {
	kind: "prepend"
	block: PMNode
}

/** Replaces an existing block by uid with a new block. */
export interface ReplaceOp {
	kind: "replace"
	block_uid: string
	block: PMNode
}

/**
 * Replaces the inline content of a text-bearing block in place,
 * preserving the block's type, attrs, and uid. Children that are
 * themselves blocks (e.g. paragraphs inside a callout) are not
 * affected.
 */
export interface UpdateTextOp {
	kind: "update_text"
	block_uid: string
	/** ProseMirror inline content (text nodes with marks). */
	content: PMInline[]
}

/**
 * Sets/overrides the named attributes on an existing block. Other
 * attributes are preserved. The uid attribute cannot be changed.
 */
export interface UpdateAttrsOp {
	kind: "update_attrs"
	block_uid: string
	attrs: Record<string, unknown>
}

/** Removes a block from the document. */
export interface DeleteOp {
	kind: "delete"
	block_uid: string
}

/** Updates the document's display name. */
export interface SetNameOp {
	kind: "set_name"
	name: string
}

/** Updates the document's icon identifier. */
export interface SetIconOp {
	kind: "set_icon"
	icon: string
}

/** A ProseMirror JSON node. Loose shape; transformer validates. */
export interface PMNode {
	type: string
	attrs?: Record<string, unknown>
	content?: (PMNode | PMInline)[]
	text?: string
	marks?: PMMark[]
}

/** A ProseMirror inline text node fragment. */
export interface PMInline {
	type: "text"
	text: string
	marks?: PMMark[]
}

/** A ProseMirror mark (bold, italic, link, …). */
export interface PMMark {
	type: string
	attrs?: Record<string, unknown>
}

/** One operation's outcome in the batch response. */
export interface OperationError {
	/** Index of the failing operation in the request payload. */
	index: number
	/** Short, machine-readable reason. */
	message: string
}

/** Result of applying a batch of operations. */
export interface ApplyResult {
	/** Number of operations that were applied successfully. */
	applied: number
	/** Per-op errors, one entry per failed op (in input order). */
	errors: OperationError[]
}

/**
 * Applies a batch of operations to the given Y.Doc inside a single
 * transaction. Failures on individual operations are collected but
 * do not abort the batch — applied state is committed for the ops
 * that succeeded. Subscribers see one consolidated update.
 */
export function applyOperations(doc: Y.Doc, ops: Operation[]): ApplyResult {
	const result: ApplyResult = { applied: 0, errors: [] }

	doc.transact(() => {
		ops.forEach((op, index) => {
			try {
				applyOperation(doc, op)
				result.applied++
			} catch (err) {
				result.errors.push({
					index,
					message:
						err instanceof Error
							? err.message
							: String(err),
				})
			}
		})
	})

	return result
}

/** Dispatches one operation to its concrete handler. */
function applyOperation(doc: Y.Doc, op: Operation): void {
	switch (op.kind) {
		case "insert":
			return opInsert(doc, op)
		case "append":
			return opAppend(doc, op)
		case "prepend":
			return opPrepend(doc, op)
		case "replace":
			return opReplace(doc, op)
		case "update_text":
			return opUpdateText(doc, op)
		case "update_attrs":
			return opUpdateAttrs(doc, op)
		case "delete":
			return opDelete(doc, op)
		case "set_name":
			return opSetName(doc, op)
		case "set_icon":
			return opSetIcon(doc, op)
	}
}

/* ---------------- per-op handlers ---------------- */

function opInsert(doc: Y.Doc, op: InsertOp): void {
	const found = findByUid(doc.getXmlFragment("content"), op.reference_uid)
	if (!found) {
		throw new Error(`reference_uid not found: ${op.reference_uid}`)
	}

	const xml = pmBlockToY(op.block)
	const insertAt =
		op.position === "before" ? found.index : found.index + 1
	found.parent.insert(insertAt, [xml])
}

function opAppend(doc: Y.Doc, op: AppendOp): void {
	const frag = doc.getXmlFragment("content")

	// TipTap's trailingNode extension keeps an empty paragraph at the
	// end of the document as a click target. Inserting after it
	// produces a visible gap because the extension immediately
	// appends a fresh trailing paragraph, turning the old one into a
	// stray empty block. Insert before it instead so the trailing
	// paragraph stays last.
	let insertAt = frag.length
	if (insertAt > 0) {
		const last = frag.get(insertAt - 1)
		if (last instanceof Y.XmlElement && isEmptyParagraph(last)) {
			insertAt -= 1
		}
	}

	frag.insert(insertAt, [pmBlockToY(op.block)])
}

function opPrepend(doc: Y.Doc, op: PrependOp): void {
	const frag = doc.getXmlFragment("content")
	frag.insert(0, [pmBlockToY(op.block)])
}

function opReplace(doc: Y.Doc, op: ReplaceOp): void {
	const found = findByUid(doc.getXmlFragment("content"), op.block_uid)
	if (!found) {
		throw new Error(`block_uid not found: ${op.block_uid}`)
	}

	const xml = pmBlockToY(op.block)
	found.parent.delete(found.index, 1)
	found.parent.insert(found.index, [xml])
}

function opUpdateText(doc: Y.Doc, op: UpdateTextOp): void {
	const found = findByUid(doc.getXmlFragment("content"), op.block_uid)
	if (!found) {
		throw new Error(`block_uid not found: ${op.block_uid}`)
	}

	// Replace the block's children with a fresh Y.XmlText carrying
	// the new inline content. Block-level children (e.g. paragraphs
	// nested inside a list item) would also be wiped — update_text
	// is only meant for text-bearing leaf blocks (paragraph, heading,
	// codeBlock, mermaidBlock, …) so that's intended.
	found.element.delete(0, found.element.length)

	const text = buildInlineText(op.content)
	if (text) {
		found.element.insert(0, [text])
	}
}

function opUpdateAttrs(doc: Y.Doc, op: UpdateAttrsOp): void {
	const found = findByUid(doc.getXmlFragment("content"), op.block_uid)
	if (!found) {
		throw new Error(`block_uid not found: ${op.block_uid}`)
	}

	for (const [key, value] of Object.entries(op.attrs)) {
		if (key === UID_ATTR) {
			// The uid is the block's identity; never let an
			// update_attrs op clobber it.
			continue
		}

		// Y.XmlElement.setAttribute is typed as accepting strings
		// only, but the runtime stores arbitrary values, which is
		// what the editor schema relies on for nested metric
		// configuration. Cast through unknown to bypass the
		// declaration's stricter type.
		;(
			found.element as unknown as {
				setAttribute(key: string, value: unknown): void
			}
		).setAttribute(key, value)
	}
}

function opDelete(doc: Y.Doc, op: DeleteOp): void {
	const found = findByUid(doc.getXmlFragment("content"), op.block_uid)
	if (!found) {
		throw new Error(`block_uid not found: ${op.block_uid}`)
	}

	found.parent.delete(found.index, 1)
}

function opSetName(doc: Y.Doc, op: SetNameOp): void {
	const nameFrag = doc.getXmlFragment("name")
	nameFrag.delete(0, nameFrag.length)

	const para = new Y.XmlElement("paragraph")
	para.setAttribute(UID_ATTR, nanoid())

	if (op.name) {
		const text = new Y.XmlText()
		text.insert(0, op.name)
		para.insert(0, [text])
	}

	nameFrag.insert(0, [para])
}

function opSetIcon(doc: Y.Doc, op: SetIconOp): void {
	const iconText = doc.getText("icon")
	iconText.delete(0, iconText.length)
	if (op.icon) {
		iconText.insert(0, op.icon)
	}
}

/* ---------------- helpers ---------------- */

/**
 * Locates the first descendant XmlElement whose uid attribute
 * matches the target, returning the element along with its
 * immediate parent fragment and index in that parent. Used by
 * operations that need to mutate a block in place or insert
 * adjacent to it.
 */
export function findByUid(
	fragment: Y.XmlFragment,
	uid: string,
): {
	parent: Y.XmlFragment | Y.XmlElement
	index: number
	element: Y.XmlElement
} | null {
	for (let i = 0; i < fragment.length; i++) {
		const child = fragment.get(i)
		if (!(child instanceof Y.XmlElement)) {
			continue
		}

		if (child.getAttribute(UID_ATTR) === uid) {
			return { parent: fragment, index: i, element: child }
		}

		const inside = findByUid(child, uid)
		if (inside) {
			return inside
		}
	}

	return null
}

/**
 * Reports whether el is a paragraph node with no visible inline
 * content. Used by opAppend to detect TipTap's trailing-paragraph
 * affordance so the new block is inserted before it instead of
 * after.
 */
function isEmptyParagraph(el: Y.XmlElement): boolean {
	if (el.nodeName !== "paragraph") {
		return false
	}

	for (let i = 0; i < el.length; i++) {
		const child = el.get(i)
		if (child instanceof Y.XmlText && child.length > 0) {
			return false
		}
		if (child instanceof Y.XmlElement) {
			return false
		}
	}

	return true
}

/**
 * Converts a single ProseMirror block (as JSON) into a Y.XmlElement
 * detached from any document so the caller can insert it into a
 * live fragment. Uses the existing TiptapTransformer for the
 * schema-aware conversion and clones the result so we don't leak
 * the temporary Y.Doc.
 */
export function pmBlockToY(block: PMNode): Y.XmlElement {
	const tempDoc = transformer.toYdoc(
		{ type: "doc", content: [block] },
		"content",
	)

	const frag = tempDoc.getXmlFragment("content")
	const first = frag.get(0)
	if (!(first instanceof Y.XmlElement)) {
		throw new Error(
			"pmBlockToY: transformer produced no XmlElement",
		)
	}

	return cloneXmlElement(first)
}

/**
 * Builds a Y.XmlText carrying the supplied ProseMirror inline
 * content (text nodes with optional marks). Returns null when the
 * content is empty so callers can skip the insert.
 */
function buildInlineText(content: PMInline[]): Y.XmlText | null {
	if (!content || content.length === 0) {
		return null
	}

	const text = new Y.XmlText()
	let cursor = 0

	for (const node of content) {
		if (node.type !== "text" || !node.text) {
			continue
		}

		const attrs = marksToAttrs(node.marks)
		text.insert(cursor, node.text, attrs)
		cursor += node.text.length
	}

	if (cursor === 0) {
		return null
	}

	return text
}

/**
 * Translates a ProseMirror mark array into the format-attribute
 * shape Y.XmlText expects: { [markType]: markAttrs | true }.
 */
function marksToAttrs(
	marks: PMMark[] | undefined,
): Record<string, unknown> | undefined {
	if (!marks || marks.length === 0) {
		return undefined
	}

	const out: Record<string, unknown> = {}
	for (const mark of marks) {
		out[mark.type] = mark.attrs ?? true
	}

	return out
}
