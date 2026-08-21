import { Extension } from "@tiptap/core"
import { Selection, TextSelection } from "prosemirror-state"
import { Plugin, PluginKey } from "@tiptap/pm/state"
import { Slice, Fragment, type Node } from "@tiptap/pm/model"

function hasAnyTextblock(doc: Node): boolean {
	let found = false
	doc.descendants((node) => {
		if (node.isTextblock) {
			found = true
			return false
		}
		return !found
	})
	return found
}

function firstTextPos(doc: Node): number | null {
	const end = doc.content.size
	for (let pos = 1; pos < end; pos++) {
		const $pos = doc.resolve(pos)
		if ($pos.parent.isTextblock) return pos
	}
	return null
}

function lastTextPos(doc: Node): number | null {
	const end = doc.content.size
	for (let pos = end - 1; pos > 0; pos--) {
		const $pos = doc.resolve(pos)
		if ($pos.parent.isTextblock) return pos
	}
	return null
}

// Move a raw pos to a nearby valid *text* cursor position.
// bias: -1 for up/left, +1 for down/right.
function clampToText(doc: Node, pos: number, bias: -1 | 1): number | null {
	try {
		const sel = Selection.near(doc.resolve(pos), bias)
		if (sel instanceof TextSelection) return sel.head
		// If it's a NodeSelection, try one step further in the same direction.
		const nudged = Math.max(1, Math.min(doc.content.size - 1, pos + bias))
		const sel2 = Selection.near(doc.resolve(nudged), bias)
		if (sel2 instanceof TextSelection) return sel2.head
		return null
	} catch {
		return null
	}
}

export const TextSelectClipboard = Extension.create({
	name: "textSelectClipboard",
	addProseMirrorPlugins() {
		return [
			new Plugin({
				key: new PluginKey("textSelectClipboard"),
				props: {
					transformCopied: (slice) => {
						const schema = this.editor.schema

						const extractText = (fragment: Fragment): string => {
							let text = ""

							fragment.forEach((node) => {
								if (node.isText) {
									text += node.text ?? ""
								} else if (node.content.size > 0) {
									text += extractText(node.content)
								}

								if (node.isBlock && text && !text.endsWith("\n")) {
									text += "\n"
								}
							})

							return text
						}

						const plainText = extractText(slice.content)

						// Split on newlines, trim, and skip empty lines
						const lines = plainText
							.split(/\r?\n/)
							.map((line) => line.trim())
							.filter((line) => line.length > 0)

						const combinedText = lines.join(" ")

						if (!combinedText.length) {
							return new Slice(Fragment.empty, 0, 0)
						}

						const textNode = schema.text(combinedText)
						return new Slice(Fragment.from(textNode), 0, 0)
					},
				},
			}),
		]
	},
})

export const TextSelectShortcuts = Extension.create({
	addKeyboardShortcuts() {
		return {
			"Mod-a": ({ editor }) => {
				const { state, view } = editor
				const { $from } = state.selection
				const node = $from.node()

				if (node.isTextblock) {
					const pos = $from.start()
					const end = $from.end()

					const tr = state.tr.setSelection(
						TextSelection.create(state.doc, pos, end),
					)

					view.dispatch(tr)

					return true
				}

				return false
			},
			"Shift-ArrowUp": ({ editor }) => {
				const { state, view } = editor
				const { doc, selection } = state
				if (!hasAnyTextblock(doc)) return true // nothing to do in an empty schema/doc

				const first = firstTextPos(doc)
				const last = lastTextPos(doc)
				if (first == null || last == null) return true

				const anchorClamped = clampToText(doc, selection.anchor, -1) ?? first
				const headClamped = clampToText(doc, selection.head, -1) ?? first

				// already full-doc selection? do nothing
				if (
					Math.min(anchorClamped, headClamped) <= first &&
					Math.max(anchorClamped, headClamped) >= last
				) {
					return true
				}

				const $head = doc.resolve(headClamped)
				const parent = $head.parent
				if (!parent.isTextblock) return true

				const blockStart = $head.start()

				// extend within current block first
				const targetWithin = clampToText(doc, blockStart, -1) ?? first
				if (headClamped !== targetWithin) {
					const sel = TextSelection.between(
						doc.resolve(anchorClamped),
						doc.resolve(targetWithin),
						-1,
					)

					view.dispatch(state.tr.setSelection(sel).scrollIntoView())
					return true
				}

				// jump to previous non-empty block start (by scanning upward)
				let pos = blockStart - 1
				while (pos > first) {
					const $p = doc.resolve(pos)
					if ($p.parent.isTextblock && $p.parent.content.size > 0) {
						const target = clampToText(doc, $p.start(), -1) ?? first
						const sel = TextSelection.between(
							doc.resolve(anchorClamped),
							doc.resolve(target),
							-1,
						)

						view.dispatch(state.tr.setSelection(sel).scrollIntoView())
						return true
					}
					pos--
				}

				// otherwise clamp to first
				const sel = TextSelection.between(
					doc.resolve(anchorClamped),
					doc.resolve(first),
					-1,
				)

				view.dispatch(state.tr.setSelection(sel).scrollIntoView())

				return true
			},

			"Shift-ArrowDown": ({ editor }) => {
				const { state, view } = editor
				const { doc, selection } = state
				if (!hasAnyTextblock(doc)) return true

				const first = firstTextPos(doc)
				const last = lastTextPos(doc)
				if (first == null || last == null) return true

				const anchorClamped = clampToText(doc, selection.anchor, 1) ?? last
				const headClamped = clampToText(doc, selection.head, 1) ?? last

				// already full-doc selection? do nothing
				if (
					Math.min(anchorClamped, headClamped) <= first &&
					Math.max(anchorClamped, headClamped) >= last
				) {
					return true
				}

				const $head = doc.resolve(headClamped)
				const parent = $head.parent
				if (!parent.isTextblock) return true

				const blockEnd = $head.end()

				// extend within current block first
				const targetWithin = clampToText(doc, blockEnd, 1) ?? last
				if (headClamped !== targetWithin) {
					const sel = TextSelection.between(
						doc.resolve(anchorClamped),
						doc.resolve(targetWithin),
						1,
					)

					view.dispatch(state.tr.setSelection(sel).scrollIntoView())
					return true
				}

				// jump to next non-empty block end (by scanning downward)
				let pos = blockEnd + 1
				const docEnd = doc.content.size
				while (pos < docEnd) {
					const $p = doc.resolve(pos)
					if ($p.parent.isTextblock && $p.parent.content.size > 0) {
						const target = clampToText(doc, $p.end(), 1) ?? last
						const sel = TextSelection.between(
							doc.resolve(anchorClamped),
							doc.resolve(target),
							1,
						)

						view.dispatch(state.tr.setSelection(sel).scrollIntoView())
						return true
					}
					pos++
				}

				// otherwise clamp to last
				const sel = TextSelection.between(
					doc.resolve(anchorClamped),
					doc.resolve(last),
					1,
				)

				view.dispatch(state.tr.setSelection(sel).scrollIntoView())

				return true
			},
		}
	},
})
