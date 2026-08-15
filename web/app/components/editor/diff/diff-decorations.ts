import { Extension, type JSONContent } from "@tiptap/core"
import { Plugin, PluginKey } from "@tiptap/pm/state"
import { Decoration, DecorationSet } from "@tiptap/pm/view"
import type { Node as PMNode } from "@tiptap/pm/model"
import { extractTokensFromJSON, extractTokensFromPMNode } from "./diff-tokens"
import { computeTokenDiff } from "./diff-ops"
import type { DiffOp } from "./diff-ops"
import type { DiffToken } from "./diff-tokens"
import { DiffStatus } from "./position-map"
import Bold from "@tiptap/extension-bold"
import Code from "@tiptap/extension-code"
import Italic from "@tiptap/extension-italic"
import Strike from "@tiptap/extension-strike"
import Underline from "@tiptap/extension-underline"
import { TaskItem } from "@tiptap/extension-list"
import {
	DEFAULT_OPAQUE_TYPES,
	OVERLAY_NODES,
	SELF_DECORATED_TYPES,
} from "./config"

export interface DiffDecorationsOptions {
	/**
	 * node types treated as opaque — when modified, the whole node gets
	 * a diff-modified class instead of character-level inline diffs.
	 */
	opaqueTypes: string[]
}

const pluginKey = new PluginKey("diffDecorations")

/**
 * per-side padding that extends the overlay beyond the node's box.
 * all values are CSS length strings.
 */
export interface OverlayPadding {
	top?: string
	right?: string
	bottom?: string
	left?: string
}

export const DiffDecorations = Extension.create<DiffDecorationsOptions>({
	name: "diffDecorations",

	addOptions() {
		return {
			opaqueTypes: DEFAULT_OPAQUE_TYPES,
		}
	},

	addProseMirrorPlugins() {
		const opaqueTypes = new Set(this.options.opaqueTypes)

		return [
			new Plugin({
				key: pluginKey,

				state: {
					init() {
						return DecorationSet.empty
					},
					apply(tr, oldSet) {
						if (!tr.docChanged) {
							return oldSet
						}

						// always rebuild: comment transactions use setNodeMarkup
						// which replaces nodes, causing DecorationSet.map() to
						// drop decorations spanning the replaced range
						return buildDiffDecorations(tr.doc, opaqueTypes)
					},
				},

				props: {
					decorations(state) {
						return this.getState(state) ?? DecorationSet.empty
					},
				},
			}),
		]
	},
})

function buildDiffDecorations(
	doc: PMNode,
	opaqueTypes: Set<string>,
): DecorationSet {
	const decorations: Decoration[] = []

	doc.descendants((node, pos) => {
		const status = node.attrs.diffStatus
		if (!status || status === DiffStatus.Unchanged) {
			return true
		}

		// node-view components that apply their own diff overlay
		if (SELF_DECORATED_TYPES.has(node.type.name)) {
			return false
		}

		if (status === DiffStatus.Added) {
			pushDiffDecoration(decorations, node, pos, "diff-added", true)
			return false
		}

		if (status === DiffStatus.Removed) {
			pushDiffDecoration(decorations, node, pos, "diff-removed", true)
			return false
		}

		// status === DiffStatus.Modified
		if (opaqueTypes.has(node.type.name)) {
			pushDiffDecoration(decorations, node, pos, "diff-modified", false)
			return false
		}

		// task items with a changed checked status get whole-item highlight
		if (
			node.type.name === TaskItem.name &&
			hasTaskListItemCheckedChanged(node)
		) {
			pushDiffDecoration(decorations, node, pos, "diff-modified", true)
			return true
		}

		if (node.isTextblock) {
			const oldNodeRaw = node.attrs.oldNode
			if (!oldNodeRaw) {
				return false
			}

			const oldTokens = extractTokensFromJSON(oldNodeRaw)
			const newTokens = extractTokensFromPMNode(node)
			const ops = computeTokenDiff(oldTokens, newTokens)

			// content starts at pos + 1 (inside the node)
			let cursor = pos + 1
			for (const op of ops) {
				if (op.type === "equal") {
					cursor += opTextLength(op)
				} else if (op.type === "insert") {
					const len = opTextLength(op)
					decorations.push(
						Decoration.inline(cursor, cursor + len, {
							class: "diff-text-added",
						}),
					)
					cursor += len
				} else {
					// delete — widget at current position
					decorations.push(
						Decoration.widget(cursor, renderDeletedTokens(op.tokens), {
							side: -1,
						}),
					)
				}
			}

			return false
		}

		// modified block with block content — recurse into children
		return true
	})

	return DecorationSet.create(doc, decorations)
}

/**
 * push either a simple class-based node decoration or, for node types
 * in OVERLAY_NODES, a widget overlay that covers indicators (bullets,
 * numbers, checkboxes) sitting outside the padding box. overlays are
 * only used for fully added/removed nodes — modified nodes with
 * internal text diffs use the standard class decoration only.
 */
function pushDiffDecoration(
	decorations: Decoration[],
	node: PMNode,
	pos: number,
	diffClass: string,
	useOverlay: boolean,
): void {
	const overlayPadding = useOverlay
		? OVERLAY_NODES.get(node.type.name)
		: undefined

	if (overlayPadding) {
		decorations.push(
			Decoration.node(pos, pos + node.nodeSize, {
				class: "diff-overlay-anchor",
			}),
		)
		decorations.push(
			Decoration.widget(
				pos + 1,
				() => createDiffOverlayElement(diffClass, overlayPadding),
				{ side: -1, key: `diff-overlay-${pos}` },
			),
		)
	} else {
		decorations.push(
			Decoration.node(pos, pos + node.nodeSize, {
				class: diffClass,
			}),
		)
	}
}

function createDiffOverlayElement(
	diffClass: string,
	padding: OverlayPadding,
): HTMLElement {
	const el = document.createElement("span")
	el.contentEditable = "false"
	el.className = `diff-overlay ${diffClass}`
	el.style.top = `-${padding.top ?? "0px"}`
	el.style.right = `-${padding.right ?? "0px"}`
	el.style.bottom = `-${padding.bottom ?? "0px"}`
	el.style.left = `-${padding.left ?? "0px"}`

	return el
}

/** check if a task item's checked attribute differs from its old version */
function hasTaskListItemCheckedChanged(node: PMNode): boolean {
	const raw = node.attrs.oldNode
	if (!raw) {
		return false
	}

	return raw.attrs?.checked !== node.attrs.checked
}

function opTextLength(op: DiffOp): number {
	let len = 0
	for (const t of op.tokens) {
		len += t.text.length
	}

	return len
}

/** render deleted tokens as a widget DOM element */
function renderDeletedTokens(tokens: DiffToken[]): HTMLElement {
	const wrapper = document.createElement("span")
	wrapper.className = "diff-text-removed"

	let currentMarksKey: string | null = null
	// consecutive tokens from the same source text node share the same
	// marks array reference. track the last reference so we can skip
	// jsonStableStringify when it hasn't changed — for a 500-char
	// deletion from 3 text nodes this reduces 500 calls to 3.
	let currentMarksRef: JSONContent[] | null = null
	let currentSpan: HTMLSpanElement | null = null

	for (const token of tokens) {
		let marksKey: string
		if (token.marks === currentMarksRef) {
			marksKey = currentMarksKey!
		} else {
			marksKey = token.marks.length > 0 ? jsonStableStringify(token.marks) : ""
			currentMarksRef = token.marks
		}

		// group consecutive tokens with the same marks into one span
		if (marksKey !== currentMarksKey) {
			currentSpan = document.createElement("span")
			applyMarkStyles(currentSpan, token.marks as JSONMark[])
			wrapper.appendChild(currentSpan)
			currentMarksKey = marksKey
		}

		currentSpan!.textContent += token.text
	}

	return wrapper
}

/** apply basic mark styles to a span element */
function applyMarkStyles(span: HTMLSpanElement, marks: JSONMark[]) {
	for (const mark of marks) {
		switch (mark.type) {
			case Bold.name:
				span.style.fontWeight = "bold"
				break
			case Italic.name:
				span.style.fontStyle = "italic"
				break
			case Underline.name:
				span.style.textDecoration = "underline"
				break
			case Strike.name:
				span.style.textDecoration = "line-through"
				break
			case Code.name:
				span.style.fontFamily = "monospace"
				span.style.fontSize = "0.9em"
				break
		}
	}
}

interface JSONMark {
	type: string
	attrs?: Record<string, unknown>
}
