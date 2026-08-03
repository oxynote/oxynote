import { Extension } from "@tiptap/core"
import { Plugin, PluginKey } from "prosemirror-state"
import { Decoration, DecorationSet } from "prosemirror-view"

const parameterListSeparatorsKey = new PluginKey("parameterListSeparators")

export const ParameterListSeparators = Extension.create({
	name: "splitDocumentationParameterListSeparators",
	addProseMirrorPlugins() {
		return [
			new Plugin({
				key: parameterListSeparatorsKey,
				state: {
					init(_, { doc, schema }) {
						return createSeparatorDecorations(doc, schema)
					},
					apply(tr, oldState, _oldEditorState, newEditorState) {
						if (!tr.docChanged) {
							return oldState
						}
						// Rebuild decorations when document changes
						return createSeparatorDecorations(
							newEditorState.doc,
							newEditorState.schema,
						)
					},
				},
				props: {
					decorations(state) {
						return parameterListSeparatorsKey.getState(state)
					},
				},
			}),
		]
	},
})

// Counter to ensure unique widget keys
let separatorWidgetCounter = 0

function createSeparatorDecorations(
	doc: any,
	schema: any,
): DecorationSet | null {
	const decorations: Decoration[] = []

	const parameterListType = schema.nodes.splitDocumentationParameterList

	if (!parameterListType) {
		return null
	}

	doc.descendants((node: any, pos: number) => {
		if (node.type !== parameterListType) {
			return
		}

		let isFirstChild = true
		let childIndex = 0

		node.forEach((_child: any, offset: number) => {
			if (!isFirstChild) {
				const widgetPos = pos + offset + 1

				decorations.push(
					Decoration.widget(
						widgetPos,
						() => {
							const div = document.createElement("div")
							div.className =
								"mt-4 mb-3 w-full border-t border-border drag-handle-ignore"
							return div
						},
						{
							// Use unique key to prevent DOM reuse issues
							key: `param-sep-${pos}-${childIndex}-${separatorWidgetCounter++}`,
						},
					),
				)
			}

			isFirstChild = false
			childIndex++
		})
	})

	if (!decorations.length) {
		return null
	}

	return DecorationSet.create(doc, decorations)
}
