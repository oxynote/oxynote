import type { AnyExtension, Editor, Node as TiptapNode } from "@tiptap/core"
import { getExtensionField } from "@tiptap/core"
import type { Node as PMNode, NodeType, Schema } from "@tiptap/pm/model"
import type { Plugin, Transaction } from "@tiptap/pm/state"
import { EditorState, TextSelection } from "@tiptap/pm/state"
import type { DecorationSet } from "@tiptap/pm/view"
import { vi } from "vitest"
import { nodeType } from "./schema"

type ShortcutMap = Record<string, (props: { editor: Editor }) => boolean>

// a fake editor whose view applies every dispatched transaction to a
// plain EditorState, so handlers written against Editor run without a
// DOM-backed EditorView. Dispatched transactions are recorded and
// applied, so state() tracks the result.
export function makeDispatchEditor(initial: EditorState): {
	editor: Editor
	dispatched: Transaction[]
	state: () => EditorState
} {
	let current = initial
	const dispatched: Transaction[] = []

	const editor = {
		get state() {
			return current
		},
		view: {
			dispatch(tr: Transaction) {
				dispatched.push(tr)
				current = current.apply(tr)
			},
		},
	} as unknown as Editor

	return { editor, dispatched, state: () => current }
}

// the same stand-in with the caret placed up front, for the keyboard
// handlers that act on the selection. Its dispatch is a spy so a suite
// can assert that a handler declined to touch the document.
export function makeCursorEditor(
	doc: PMNode,
	cursorPos: number,
): {
	editor: Editor
	dispatch: ReturnType<typeof vi.fn>
	state: () => EditorState
} {
	let state = EditorState.create({
		doc,
		selection: TextSelection.create(doc, cursorPos),
	})

	const dispatch = vi.fn((tr: Transaction) => {
		state = state.apply(tr)
	})

	const editor = {
		get state() {
			return state
		},
		view: { dispatch },
	} as unknown as Editor

	return { editor, dispatch, state: () => state }
}

export function stateAt(doc: PMNode, from: number, to = from): EditorState {
	return EditorState.create({
		doc,
		selection: TextSelection.create(doc, from, to),
	})
}

// returns the absolute position right before the text node with the
// given text — inside its parent, at parent offset zero
export function startOfText(doc: PMNode, text: string): number {
	let found = -1

	doc.descendants((node, pos) => {
		if (found !== -1) {
			return false
		}

		if (node.isText && node.text === text) {
			found = pos
			return false
		}

		return true
	})

	if (found === -1) {
		throw new Error(`text "${text}" not found in the test document`)
	}

	return found
}

// invokes a node extension's keyboard shortcut factory with just enough
// context for handlers that read only their own name, node type, and
// editor, and returns the requested handler
export function nodeKeyboardShortcut(
	extension: TiptapNode,
	editor: Editor,
	key: string,
	schema: Schema,
): (props: { editor: Editor }) => boolean {
	const addKeyboardShortcuts = extension.config
		.addKeyboardShortcuts as unknown as
		| ((this: { name: string; type: NodeType; editor: Editor }) => ShortcutMap)
		| undefined

	if (!addKeyboardShortcuts) {
		throw new Error(`${extension.name} defines no keyboard shortcuts`)
	}

	const shortcuts = addKeyboardShortcuts.call({
		name: extension.name,
		type: nodeType(schema, extension.name),
		editor,
	})

	const handler = shortcuts[key]

	if (!handler) {
		throw new Error(`${extension.name} defines no ${key} shortcut`)
	}

	return handler
}

// the context tiptap binds an extension's config callbacks to, reduced
// to the fields those callbacks read outside a live editor
export function extensionContext(
	extension: AnyExtension,
	name: string,
	editor?: Editor,
) {
	return {
		name,
		options: extension.options as Record<string, unknown>,
		storage: extension.storage as Record<string, unknown>,
		editor,
	}
}

// builds an extension's whole shortcut map bound to a stub editor
// holding a document with the caret at `anchor` (and optionally
// extended to `head`), for handlers that read the extension's options
// and storage rather than its node type
export function shortcutsAt(
	extension: AnyExtension,
	name: string,
	docNode: PMNode,
	anchor: number,
	head?: number,
): {
	shortcuts: ShortcutMap
	editor: Editor
	dispatched: Transaction[]
	state: () => EditorState
} {
	const target = makeDispatchEditor(stateAt(docNode, anchor, head ?? anchor))
	const build = getExtensionField<() => ShortcutMap>(
		extension,
		"addKeyboardShortcuts",
		extensionContext(extension, name, target.editor),
	)

	return { ...target, shortcuts: build() }
}

// compresses a decoration-set plugin's output into [class, text] pairs
export function decorationClassShape(
	state: EditorState,
	plugin: Plugin,
): [string, string][] {
	const set = plugin.getState(state) as DecorationSet

	return set
		.find()
		.map((deco) => [
			(deco as unknown as { type: { attrs: { class: string } } }).type.attrs
				.class,
			state.doc.textBetween(deco.from, deco.to),
		])
}
