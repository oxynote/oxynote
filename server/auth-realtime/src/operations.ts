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
import { isDeepStrictEqual } from "node:util"
import * as Y from "yjs"
import { nanoid } from "nanoid"
import {
	cellChanges,
	dropInGrid,
	GRID_PLACE_ATTRS,
	type GridDropTarget,
	gridItemOf,
	type GridRect,
	layoutGrid,
	resizeInGrid,
	stackOrder,
	stackPlaces,
	stackPosOf,
	withoutPlaces,
} from "@oxynote/common/grid"
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
const INSERT_POSITIONS: string[] = ["before", "after", "above", "below"]

// the sides only a block in a grid has
const GRID_POSITIONS: string[] = ["above", "below"]

// the layouts of a grid a move can change. The wide layout places blocks
// on columns. The stacked layout shows them one per row.
const MOVE_LAYOUTS: string[] = ["wide", "stacked"]

// the nodes that hold grid blocks. metricGrid is the legacy name.
const GRID_NAMES = new Set(["blockGrid", "metricGrid"])

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

// where an insert or move lands next to its reference block. In a grid,
// before and after drop on the reference block's left and right edges,
// and above and below drop on its top and bottom edges. Outside a grid
// only before and after exist.
export type Position = "before" | "after" | "above" | "below"

/** Inserts a new block immediately before/after a referenced block. */
export interface InsertOp {
	kind: "insert"
	/** Insertion side relative to the reference block. */
	position: Position
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
	position: Position
	/** Reference block's uid in the document. */
	reference_uid: string
	// the layout of a grid the move changes. It is wide when left out.
	// stacked moves the block in the order narrow pages show, and needs
	// both blocks in the same grid.
	layout?: "wide" | "stacked"
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
	// the position is whatever the JSON payload carried, so it may name
	// no side at all
	checkPosition("insert", op.position)

	const found = blockByUid(doc, op.reference_uid, "reference_uid")
	const grid = gridOf(found.parent)
	const insertAt = landsAfter(op.position) ? found.index + 1 : found.index

	if (!grid) {
		checkOutsideGrid(op.position)
		insertBlock(found.parent, insertAt, op.block, PLACE_ADVICE)

		return
	}

	const layout = gridLayout(grid)

	insertBlock(
		found.parent,
		insertAt,
		{ ...op.block, attrs: withoutPlaces(op.block.attrs ?? {}) },
		PLACE_ADVICE,
	)
	placeInGrid(
		layout,
		found.parent.get(insertAt) as Y.XmlElement,
		null,
		targetBeside(found.index, op.position),
	)
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
	const grid = gridOf(found.parent)

	if (!grid) {
		found.parent.delete(found.index, 1)
		insertBlock(found.parent, found.index, op.block)

		return
	}

	const layout = gridLayout(grid)

	// the same block changes in place. So a user's concurrent edit
	// survives on every attribute that the replacement leaves unchanged.
	if (
		op.block.attrs?.[UID_ATTR] === op.block_uid &&
		op.block.type === found.element.nodeName
	) {
		checkBlock(op.block)
		updateInPlace(found.element, op.block.attrs)
		resizeIfSized(layout, found.index)

		return
	}

	const stackPos = stackPosOf(attrsOf(found.element))

	found.parent.delete(found.index, 1)
	insertBlock(found.parent, found.index, {
		...op.block,
		attrs: withoutPlaces(op.block.attrs ?? {}),
	})

	// another block takes the replaced one's places
	const added = found.parent.get(found.index) as Y.XmlElement
	layout.blocks[found.index] = added

	if (stackPos !== null) {
		setAttr(added, "stackPos", stackPos)
	}

	writeCells(
		layout.blocks,
		resizedLayout(layout, found.index) ?? layout.rects,
	)
}

function opUpdateAttrs(doc: Y.Doc, op: UpdateAttrsOp): void {
	const found = blockByUid(doc, op.block_uid, "block_uid")
	const grid = gridOf(found.parent)
	// only a new size moves the other blocks of a grid. The layout is
	// read before the update, so it still holds the old size.
	const layout =
		grid &&
		(Object.hasOwn(op.attrs, "width") ||
			Object.hasOwn(op.attrs, "height"))
			? gridLayout(grid)
			: null
	const current = attrsOf(found.element)

	for (const [key, value] of Object.entries(op.attrs)) {
		if (key === UID_ATTR) {
			// The uid is the block's identity; never let an
			// update_attrs op clobber it.
			continue
		}

		// writing a value the block already holds would still overwrite
		// a user's concurrent edit to it
		if (isDeepStrictEqual(value, current[key])) {
			continue
		}

		setAttr(found.element, key, value)
	}

	if (layout) {
		resizeIfSized(layout, found.index)
	}
}

function opDelete(doc: Y.Doc, op: DeleteOp): void {
	const found = blockByUid(doc, op.block_uid, "block_uid")

	found.parent.delete(found.index, 1)
	checkChildrenOf(found.parent)
	deleteEmptyGrid(found.parent)
}

function opMove(doc: Y.Doc, op: MoveOp): void {
	// the position is unvalidated JSON, same as opInsert's.
	checkPosition("move", op.position)

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

	// the layout is unvalidated JSON too
	if (op.layout !== undefined && !MOVE_LAYOUTS.includes(op.layout)) {
		throw new Error(
			`move layout must be "wide" or "stacked", got: ${op.layout}`,
		)
	}

	const grid = gridOf(reference.parent)

	if (op.layout === "stacked") {
		moveInStack(grid, found, reference, op.position)

		return
	}

	if (grid) {
		moveInGrid(grid, found, reference, op.position)

		return
	}

	checkOutsideGrid(op.position)

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

	if (landsAfter(op.position)) {
		insertAt += 1
	}

	found.parent.delete(found.index, 1)
	reference.parent.insert(insertAt, [clone])

	if (reference.parent !== found.parent) {
		checkChildrenOf(found.parent)
	}

	checkChildrenOf(reference.parent, PLACE_ADVICE)
	deleteEmptyGrid(found.parent)
}

// moveInGrid moves a block next to a block of grid. Within the grid only
// the cells change, so the block stays the same element. A block from
// elsewhere joins the grid without the places it had there.
function moveInGrid(
	grid: Y.XmlElement,
	found: Found,
	reference: Found,
	position: Position,
): void {
	const layout = gridLayout(grid)
	const target = targetBeside(reference.index, position)

	if (found.parent === grid) {
		placeInGrid(layout, found.element, found.index, target)

		return
	}

	const clone = cloneXmlElement(found.element)

	for (const key of GRID_PLACE_ATTRS) {
		clone.removeAttribute(key)
	}

	found.parent.delete(found.index, 1)
	grid.insert(
		landsAfter(position) ? reference.index + 1 : reference.index,
		[clone],
	)
	checkChildrenOf(found.parent)
	checkChildrenOf(grid, PLACE_ADVICE)
	placeInGrid(layout, clone, null, target)
	deleteEmptyGrid(found.parent)
}

// moveInStack moves a block of grid in the stacked layout, as a drag on a
// narrow page does. The cells of the wide layout stay. Every block gets
// its place in the new order, but only a changed place is written.
function moveInStack(
	grid: Y.XmlElement | null,
	found: Found,
	reference: Found,
	position: Position,
): void {
	if (!grid || found.parent !== grid) {
		throw new Error(
			"a stacked move needs both blocks in the same grid; move the block into the grid first",
		)
	}

	const { blocks, rects } = gridLayout(grid)
	const order = stackOrder(
		rects,
		blocks.map((block) => stackPosOf(attrsOf(block))),
	).filter((index) => index !== found.index)
	const at =
		order.indexOf(reference.index) + (landsAfter(position) ? 1 : 0)

	order.splice(at, 0, found.index)

	stackPlaces(order).forEach((place, index) => {
		const block = blocks[index]

		if (block && stackPosOf(attrsOf(block)) !== place) {
			setAttr(block, "stackPos", place)
		}
	})
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

type Found = ReturnType<typeof blockByUid>

// a grid's blocks and the cells where the editor puts them. rects[i]
// belongs to blocks[i].
interface GridLayout {
	blocks: Y.XmlElement[]
	rects: GridRect[]
}

// checkPosition refuses a position outside the four sides. kind names the
// operation for the error. The position is checked against a list of
// strings, because comparing the declared union against its own members
// reads as dead code to the type checker.
function checkPosition(kind: string, position: string): void {
	if (!INSERT_POSITIONS.includes(position)) {
		throw new Error(
			`${kind} position must be "before", "after", "above" or "below", got: ${position}`,
		)
	}
}

// landsAfter reports whether a position puts the block after its
// reference block
function landsAfter(position: string): boolean {
	return position === "after" || position === "below"
}

// gridOf returns parent when it is a grid
function gridOf(parent: Y.XmlFragment): Y.XmlElement | null {
	return parent instanceof Y.XmlElement && GRID_NAMES.has(parent.nodeName)
		? parent
		: null
}

// deleteEmptyGrid deletes parent when it is a grid with no blocks left, as
// the editor does. But a grid that is the only block of its parent stays,
// because deleting it would leave the parent empty.
function deleteEmptyGrid(parent: Y.XmlFragment): void {
	const grid = gridOf(parent)
	const holder = grid?.parent

	if (
		!grid ||
		grid.length > 0 ||
		!(holder instanceof Y.XmlFragment) ||
		holder.length < 2
	) {
		return
	}

	holder.delete(holder.toArray().indexOf(grid), 1)
}

// checkOutsideGrid refuses a position only a block in a grid has
function checkOutsideGrid(position: string): void {
	if (GRID_POSITIONS.includes(position)) {
		throw new Error(
			`position ${position} only places a block next to a block in a grid; use before or after here`,
		)
	}
}

// gridLayout returns a grid's blocks and the cells where the editor puts
// them
function gridLayout(grid: Y.XmlElement): GridLayout {
	const blocks = grid
		.toArray()
		.filter(
			(child): child is Y.XmlElement =>
				child instanceof Y.XmlElement,
		)

	return {
		blocks: blocks,
		rects: layoutGrid(
			blocks.map((block) => gridItemOf(attrsOf(block))),
		),
	}
}

// targetBeside returns the drop target beside the block at index. position
// names the side of the block: its left or right side, or its top or
// bottom side.
function targetBeside(index: number, position: string): GridDropTarget {
	switch (position) {
		case "before":
			return {
				kind: "beside",
				index: index,
				side: "left",
			}
		case "after":
			return {
				kind: "beside",
				index: index,
				side: "right",
			}
		case "above":
			return { kind: "above", index: index }
		default:
			return { kind: "below", index: index }
	}
}

// placeInGrid drops a block into a grid the way a drag does, and writes
// the cells of every block that moves. draggedIndex is the block's index
// in the layout, or null for a block that joins the grid.
function placeInGrid(
	layout: GridLayout,
	dragged: Y.XmlElement,
	draggedIndex: number | null,
	target: GridDropTarget,
): void {
	const size = gridItemOf(attrsOf(dragged))
	const rects = dropInGrid(
		layout.rects,
		{ index: draggedIndex, w: size.w, h: size.h },
		target,
	)

	if (!rects) {
		// NOCOV: only a free target can lack room, and targetBeside
		// never makes one. Its block is never the moved block, since a
		// move relative to itself is refused.
		return
	}

	writeCells(
		draggedIndex === null
			? [...layout.blocks, dragged]
			: layout.blocks,
		rects,
	)
}

// resizeIfSized lays the grid out again when the size that the block at
// index stores differs from its cells in the layout
function resizeIfSized(layout: GridLayout, index: number): void {
	const rects = resizedLayout(layout, index)
	if (rects) {
		writeCells(layout.blocks, rects)
	}
}

// resizedLayout returns the cells of every block after the block at index
// takes the size it stores, as the editor's resize does. It returns null
// when the stored size already matches the layout.
function resizedLayout(
	layout: GridLayout,
	index: number,
): readonly GridRect[] | null {
	const block = layout.blocks[index]
	const rect = layout.rects[index]

	if (!block || !rect) {
		// NOCOV: index is the block's own index in its grid, and the
		// layout holds every block of the grid.
		return null
	}

	const size = gridItemOf(attrsOf(block))

	return size.w === rect.w && size.h === rect.h
		? null
		: resizeInGrid(layout.rects, index, size.w, size.h)
}

// writeCells writes each block's cells from rects. Only the attributes
// whose value changes are written, so a user's concurrent edit to any
// other attribute survives.
function writeCells(blocks: Y.XmlElement[], rects: readonly GridRect[]): void {
	blocks.forEach((block, i) => {
		const rect = rects[i]
		if (!rect) {
			// NOCOV: a layout holds a rect for each of its blocks.
			return
		}

		for (const [key, value] of Object.entries(
			cellChanges(attrsOf(block), rect),
		)) {
			setAttr(block, key, value)
		}
	})
}

// updateInPlace writes the attributes of next that differ from the
// element's. It removes the ones that next leaves out. It never writes the
// block's places, so the block keeps its place in the live grid.
function updateInPlace(
	element: Y.XmlElement,
	next: Record<string, unknown>,
): void {
	const current = attrsOf(element)

	for (const key of new Set([
		...Object.keys(current),
		...Object.keys(next),
	])) {
		if (
			key === UID_ATTR ||
			GRID_PLACE_ATTRS.some((place) => place === key) ||
			isDeepStrictEqual(next[key], current[key])
		) {
			continue
		}

		if (next[key] === undefined) {
			element.removeAttribute(key)
		} else {
			setAttr(element, key, next[key])
		}
	}
}

// getAttributes is declared as a string map, but attrs hold any JSON
function attrsOf(element: Y.XmlElement): Record<string, unknown> {
	return element.getAttributes()
}

function setAttr(element: Y.XmlElement, key: string, value: unknown): void {
	;(element as Y.XmlElement<Record<string, any>>).setAttribute(key, value)
}

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

	// a grid without blocks shows nothing, so a new grid must hold one
	if (GRID_NAMES.has(block.type) && !block.content?.length) {
		throw new Error(`${block.type} must hold at least one block`)
	}

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
