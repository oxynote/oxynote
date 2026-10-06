/**
 * Operation interpreter for the Go-driven edit RPC.
 *
 * The Go assistant builds operations as ProseMirror JSON and posts
 * them to the
 * /api/x/documents/:docId/branches/:branchId/operations route. This
 * module owns the Y.XmlFragment mutations that apply those
 * operations to a live Y.Doc. A batch applies whole or not at all,
 * inside a single `Y.Doc.transact` so subscribers see one update.
 */
import * as Y from "yjs"
import { nanoid } from "nanoid"
import { transformer, cloneXmlElement, cloneXmlFragment } from "./ydocument.js"
import { checkBlock, checkChildren } from "./validate.js"

/** Canonical attribute name carried on every editor block. */
const UID_ATTR = "uid"

// what a refused insert or move should try instead.
const PLACE_ADVICE = "Choose a reference block in a container that takes it."

// what a refused append or prepend should try instead.
const END_ADVICE =
	"Insert it before or after a block in a container that takes it."

/** The sides an insert or move may name, as a widened list for validation. */
const INSERT_POSITIONS: string[] = ["before", "after"]

/**
 * One batched operation against a document. Shapes mirror the Go
 * edit operations. An op that adds, replaces, moves or deletes a block
 * is checked against the editor schema (validate.ts). applyOperations
 * runs the batch on a trial copy first, so a refused op never reaches
 * the document.
 */
export type Operation =
	| InsertOp
	| AppendOp
	| PrependOp
	| ReplaceOp
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

/** A ProseMirror JSON node. Loose shape; validate.ts checks it. */
export interface PMNode {
	type: string
	attrs?: Record<string, unknown>
	content?: PMNode[]
	text?: string
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
	/** The operation that failed, if any; a batch stops at the first. */
	errors: OperationError[]
}

/**
 * Applies a batch of operations to the given Y.Doc inside a single
 * transaction, all of them or none. Subscribers see one consolidated
 * update.
 */
export function applyOperations(doc: Y.Doc, ops: Operation[]): ApplyResult {
	// a Yjs transaction cannot be rolled back, so the batch first runs
	// on an exact copy that is then thrown away. Only a batch that
	// applies whole reaches doc.
	const trial = new Y.Doc()
	cloneXmlFragment(
		doc.getXmlFragment("content"),
		trial.getXmlFragment("content"),
	)

	const error = runOperations(trial, ops)
	if (error) {
		return { errors: [error] }
	}

	doc.transact(() => {
		runOperations(doc, ops)
	})

	return { errors: [] }
}

// runOperations applies each operation in turn and returns the first
// that fails. The ones after it build on blocks it never wrote, so
// their errors would only mislead.
function runOperations(
	doc: Y.Doc,
	ops: Operation[],
): OperationError | undefined {
	for (const [index, op] of ops.entries()) {
		try {
			applyOperation(doc, op)
		} catch (err) {
			return {
				index,
				message:
					err instanceof Error
						? err.message
						: String(err),
			}
		}
	}

	return undefined
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

	const found = blockByUid(doc, op.reference_uid, "reference_uid")
	const insertAt =
		op.position === "before" ? found.index : found.index + 1

	insertBlock(found.parent, insertAt, op.block, PLACE_ADVICE)
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
	const last = insertAt > 0 ? frag.get(insertAt - 1) : null
	if (
		last instanceof Y.XmlElement &&
		last.nodeName === "paragraph" &&
		last
			.toArray()
			.every(
				(child) =>
					child instanceof Y.XmlText &&
					child.length === 0,
			)
	) {
		insertAt -= 1
	}

	insertBlock(frag, insertAt, op.block, END_ADVICE)
}

function opPrepend(doc: Y.Doc, op: PrependOp): void {
	insertBlock(doc.getXmlFragment("content"), 0, op.block, END_ADVICE)
}

function opReplace(doc: Y.Doc, op: ReplaceOp): void {
	const found = blockByUid(doc, op.block_uid, "block_uid")

	found.parent.delete(found.index, 1)
	insertBlock(found.parent, found.index, op.block)
}

function opUpdateAttrs(doc: Y.Doc, op: UpdateAttrsOp): void {
	const found = blockByUid(doc, op.block_uid, "block_uid")

	for (const [key, value] of Object.entries(op.attrs)) {
		if (key === UID_ATTR) {
			// The uid is the block's identity; never let an
			// update_attrs op clobber it.
			continue
		}

		// the default attribute type is string, but attrs hold any JSON.
		;(
			found.element as Y.XmlElement<Record<string, any>>
		).setAttribute(key, value)
	}
}

function opDelete(doc: Y.Doc, op: DeleteOp): void {
	const found = blockByUid(doc, op.block_uid, "block_uid")

	found.parent.delete(found.index, 1)
	checkChildrenOf(found.parent)
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

	const found = blockByUid(doc, op.block_uid, "block_uid")
	const reference = blockByUid(doc, op.reference_uid, "reference_uid")

	// a reference nested inside the moved block is destroyed by the
	// removal below, leaving the move nowhere to land.
	if (Y.isParentOf(found.element, reference.element._item)) {
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
	// removal invalidates the index blockByUid reported.
	let insertAt = reference.index
	if (reference.parent === found.parent && found.index < insertAt) {
		insertAt -= 1
	}

	if (op.position === "after") {
		insertAt += 1
	}

	found.parent.delete(found.index, 1)
	reference.parent.insert(insertAt, [clone])

	if (reference.parent !== found.parent) {
		checkChildrenOf(found.parent)
	}

	checkChildrenOf(reference.parent, PLACE_ADVICE)
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

// blockByUid returns the block carrying uid, with its parent and its
// index there. field names the op field that held uid, for the error.
function blockByUid(doc: Y.Doc, uid: string, field: string) {
	const [element] = doc
		.getXmlFragment("content")
		.createTreeWalker(
			(node) =>
				node instanceof Y.XmlElement &&
				node.getAttribute(UID_ATTR) === uid,
		)

	if (!(element instanceof Y.XmlElement)) {
		throw new Error(`${field} not found: ${uid}`)
	}

	// the walker only yields nodes inside the fragment.
	const parent = element.parent as Y.XmlFragment
	return { parent, index: parent.toArray().indexOf(element), element }
}

// insertBlock inserts block into parent at index, after checking the
// block and then the parent's children against the schema. advice is
// passed to checkChildren.
function insertBlock(
	parent: Y.XmlFragment,
	index: number,
	block: PMNode,
	advice = "",
): void {
	checkBlock(block)

	const first = transformer
		.toYdoc({ type: "doc", content: [block] }, "content")
		.getXmlFragment("content")
		.get(0)

	// a bare text node passes checkBlock but becomes a Y.XmlText.
	if (!(first instanceof Y.XmlElement)) {
		throw new Error(`${block.type} is not a block`)
	}

	// the element belongs to the transformer's own Y.Doc.
	parent.insert(index, [cloneXmlElement(first)])
	checkChildrenOf(parent, advice)
}

// checkChildrenOf refuses parent holding children the schema does not
// accept. advice is passed to checkChildren.
function checkChildrenOf(
	parent: Y.XmlFragment | Y.XmlElement,
	advice = "",
): void {
	// the content fragment is the document root.
	checkChildren(
		parent instanceof Y.XmlElement ? parent.nodeName : "doc",
		parent
			.toArray()
			.flatMap((child) =>
				child instanceof Y.XmlElement
					? [child.nodeName]
					: [],
			),
		advice,
	)
}
