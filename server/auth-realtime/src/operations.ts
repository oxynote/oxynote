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

/** The sides an insert or move may name, as a widened list for validation. */
const INSERT_POSITIONS: string[] = ["before", "after"]

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
	| MoveOp
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
 * preserving the block's type, attrs, and uid. A callout or blockquote
 * is written through to its first paragraph, since their schemas hold
 * blocks rather than text; any other block-carrying target is refused
 * rather than flattened.
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

/**
 * Moves an existing block immediately before/after a referenced
 * block, keeping the block's uid, attrs, and nested content intact.
 */
export interface MoveOp {
	kind: "move"
	/** Moved block's uid in the document. */
	block_uid: string
	/** Landing side relative to the reference block. */
	position: "before" | "after"
	/** Reference block's uid in the document. */
	reference_uid: string
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
			opInsert(doc, op)
			return
		case "append":
			opAppend(doc, op)
			return
		case "prepend":
			opPrepend(doc, op)
			return
		case "replace":
			opReplace(doc, op)
			return
		case "update_text":
			opUpdateText(doc, op)
			return
		case "update_attrs":
			opUpdateAttrs(doc, op)
			return
		case "delete":
			opDelete(doc, op)
			return
		case "move":
			opMove(doc, op)
			return
		case "set_name":
			opSetName(doc, op)
			return
		case "set_icon":
			opSetIcon(doc, op)
			return
		default: {
			// operations arrive as unvalidated JSON, so the switch
			// can be handed a kind this service does not implement
			// — which means the two sides have drifted apart.
			// Falling through would report the no-op as applied.
			const { kind } = op as { kind?: unknown }

			throw new Error(
				`unknown operation kind: ${String(kind)}`,
			)
		}
	}
}

/* ---------------- per-op handlers ---------------- */

function opInsert(doc: Y.Doc, op: InsertOp): void {
	// the position is whatever the JSON payload carried, so anything but
	// the two sides is rejected rather than silently taken as "after".
	// Checked against a list because comparing the declared union against
	// its own members reads as dead code to the type checker.
	if (!INSERT_POSITIONS.includes(op.position)) {
		throw new Error(
			`insert position must be "before" or "after", got: ${op.position}`,
		)
	}

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

// the blocks whose children are inline content, so replacing them
// wholesale with a text run is the whole edit.
const TEXT_LEAF_NODES = new Set([
	"paragraph",
	"heading",
	"codeBlock",
	"mermaidBlock",
])

// the blocks that hold their text one level down, in a first child
// paragraph. Their schemas admit block children only, so writing a text
// run into them directly produces a node the editor cannot parse and
// the canonical model cannot read back.
const TEXT_WRAPPER_NODES = new Set(["calloutBlock", "blockquote"])

function opUpdateText(doc: Y.Doc, op: UpdateTextOp): void {
	const found = findByUid(doc.getXmlFragment("content"), op.block_uid)
	if (!found) {
		throw new Error(`block_uid not found: ${op.block_uid}`)
	}

	const nodeName = found.element.nodeName

	// the edit replaces everything under the target, so a block holding
	// other blocks would lose them — and lose the uids comments and
	// hooks are anchored to — while reporting success. Only the caller
	// knows that was not meant, so refuse instead of guessing.
	if (
		!TEXT_LEAF_NODES.has(nodeName) &&
		!TEXT_WRAPPER_NODES.has(nodeName)
	) {
		throw new Error(
			`update_text does not apply to ${nodeName}: it carries ` +
				`blocks rather than text, and the edit would discard ` +
				`them. Use replace_block to rewrite it whole, or ` +
				`update_text on the block holding the text.`,
		)
	}

	const target = TEXT_WRAPPER_NODES.has(nodeName)
		? firstParagraph(found.element)
		: found.element

	if (!target) {
		throw new Error(
			`update_text found no paragraph to write in ${nodeName}`,
		)
	}

	target.delete(0, target.length)

	const text = buildInlineText(op.content)
	if (text) {
		target.insert(0, [text])
	}
}

// firstParagraph returns the wrapper's first paragraph child, which is
// where its text lives.
function firstParagraph(el: Y.XmlElement): Y.XmlElement | null {
	for (let i = 0; i < el.length; i++) {
		const child = el.get(i)
		if (
			child instanceof Y.XmlElement &&
			child.nodeName === "paragraph"
		) {
			return child
		}
	}

	return null
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

function opMove(doc: Y.Doc, op: MoveOp): void {
	// the position is unvalidated JSON, same as opInsert's.
	if (!INSERT_POSITIONS.includes(op.position)) {
		throw new Error(
			`move position must be "before" or "after", got: ${op.position}`,
		)
	}

	if (op.block_uid === op.reference_uid) {
		throw new Error(
			`cannot move a block relative to itself: ${op.block_uid}`,
		)
	}

	const frag = doc.getXmlFragment("content")

	const found = findByUid(frag, op.block_uid)
	if (!found) {
		throw new Error(`block_uid not found: ${op.block_uid}`)
	}

	const reference = findByUid(frag, op.reference_uid)
	if (!reference) {
		throw new Error(`reference_uid not found: ${op.reference_uid}`)
	}

	// a reference nested inside the moved block is destroyed by the
	// removal below, leaving the move nowhere to land.
	if (isInside(reference.element, found.element)) {
		throw new Error(
			`reference_uid is inside the moved block: ${op.reference_uid}`,
		)
	}

	// a removed Y.XmlElement cannot be reinserted, so the block is
	// cloned first. CloneXmlElement keeps the uid and every non-string
	// attribute, which is what keeps comments, hooks, and files
	// attached across the move.
	const clone = cloneXmlElement(found.element)

	// removing the block shifts its later siblings down by one, so a
	// reference behind it in the same parent is re-indexed before the
	// removal invalidates the index findByUid reported.
	let insertAt = reference.index
	if (reference.parent === found.parent && found.index < insertAt) {
		insertAt -= 1
	}

	if (op.position === "after") {
		insertAt += 1
	}

	found.parent.delete(found.index, 1)
	reference.parent.insert(insertAt, [clone])
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
 * Reports whether el sits anywhere inside ancestor's subtree. Used by
 * opMove to refuse a reference the removal of the moved block would
 * destroy.
 */
function isInside(el: Y.XmlElement, ancestor: Y.XmlElement): boolean {
	let parent = el.parent

	while (parent !== null) {
		if (parent === ancestor) {
			return true
		}

		parent = parent.parent
	}

	return false
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
	// eslint-disable-next-line @typescript-eslint/no-unnecessary-condition -- operations arrive as unvalidated JSON, so the declared type is a claim about the caller rather than a guarantee about the value
	if (!content || content.length === 0) {
		return null
	}

	const text = new Y.XmlText()
	let cursor = 0

	for (const node of content) {
		// eslint-disable-next-line @typescript-eslint/no-unnecessary-condition -- same as above: the node kind is whatever the JSON payload carried, not what the type says
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
