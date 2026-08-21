import Paragraph from "@tiptap/extension-paragraph"
import Heading from "@tiptap/extension-heading"
import { Extension, isNodeEmpty } from "@tiptap/core"
import type { Editor } from "@tiptap/core"
import type { Node as ProsemirrorNode } from "@tiptap/pm/model"
import { cn } from "~/lib/utils"
import { Plugin, PluginKey } from "@tiptap/pm/state"
import { Decoration, DecorationSet } from "@tiptap/pm/view"
import {
	SplitDocumentationLeftSide,
	SplitDocumentationPlaceholderParents,
} from "./blocks/split-documentation"
import {
	ParameterListItemHeaderTitle,
	ParameterListHeader,
	ParameterListItemHeaderType,
	ParameterListItem,
} from "./blocks/split-documentation/parameter-list"
import { CalloutBlock } from "./blocks/callout"
import {
	CODE_BLOCK_NAME,
	CODE_BLOCK_TITLE_NAME,
	MERMAID_BLOCK_NAME,
} from "./blocks/node-names"

export const placeholderEmptyNodeClass = cn(
	"before:content-[attr(data-placeholder)] before:text-muted-foreground/60 before:float-left before:h-0 before:pointer-events-none",
)

export function defaultNamePlaceholder(t: (i18nPath: string) => string) {
	return DynamicPlaceholder.extend({
		name: "defaultNamePlaceholder",
		addOptions(): DynamicPlaceholderOptions {
			return {
				// this.options inside addOptions resolves back into this very
				// override, so the base defaults are read off the parent
				// extension directly
				...DynamicPlaceholder.options,
				placeholder: () => {
					return t("editor.placeholders.name-default")
				},
				emptyNodeClass: cn(placeholderEmptyNodeClass),
				showOnlyNodes: "any",
			}
		},
	})
}

export function defaultContentPlaceholder(
	t: (i18nPath: string) => string,
	isEditingDisabled: MaybeRef<boolean>,
	location: "document" | "comment" = "document",
) {
	return DynamicPlaceholder.extend({
		name: "defaultContentPlaceholder",
		addOptions(): DynamicPlaceholderOptions {
			return {
				// this.options inside addOptions resolves back into this very
				// override, so the base defaults are read off the parent
				// extension directly
				...DynamicPlaceholder.options,
				placeholder: ({ node, pos, editor }) => {
					switch (node.type.name) {
						case Heading.name:
							if (
								isNodeInsideTiptapNode(editor.state, pos, [
									SplitDocumentationLeftSide.name,
								])
							) {
								if (toValue(isEditingDisabled)) {
									return t(
										"editor.placeholders.content.split-documentation.left-side-heading-empty",
									)
								}

								return t(
									"editor.placeholders.content.split-documentation.left-side-heading",
								)
							}

							return t("editor.placeholders.content.heading")
						case Paragraph.name:
							if (
								isNodeInsideTiptapNode(editor.state, pos, [
									ParameterListItem.name,
								])
							) {
								if (toValue(isEditingDisabled)) {
									return t(
										"editor.placeholders.content.split-documentation.parameter-list.item.paragraph-empty",
									)
								}

								return t(
									"editor.placeholders.content.split-documentation.parameter-list.item.paragraph",
								)
							} else if (
								isNodeInsideTiptapNode(editor.state, pos, [
									SplitDocumentationLeftSide.name,
								])
							) {
								if (toValue(isEditingDisabled)) {
									return t(
										"editor.placeholders.content.split-documentation.left-side-paragraph-empty",
									)
								}

								return t(
									"editor.placeholders.content.split-documentation.left-side-paragraph",
								)
							} else if (
								isNodeInsideTiptapNode(editor.state, pos, [CalloutBlock.name])
							) {
								return t("editor.placeholders.content.callout.content")
							}

							if (location === "document") {
								return t("editor.placeholders.content.paragraph")
							} else if (editor.isEmpty) {
								return t("editor.placeholders.content.comment")
							}
					}

					return ""
				},
				emptyNodeClass: cn(placeholderEmptyNodeClass),
				includeChildren: true,
				// includes "current" active empty node
				showOnlyNodes: [
					{
						parent: true,
						name: CalloutBlock.name,
					},
					...SplitDocumentationPlaceholderParents,
				],
			}
		},
	})
}

// this function should be rarely used. The Placeholder extension should
// handle most of the cases.
export function explicitContentPlaceholder(
	t: (i18nPath: string) => string,
	nodeType: string,
	isEditingDisabled: MaybeRef<boolean>,
): string {
	switch (nodeType) {
		case CODE_BLOCK_NAME:
			if (toValue(isEditingDisabled)) {
				return t(
					"editor.placeholders.content.extended-code-block.content-empty",
				)
			}

			return t("editor.placeholders.content.extended-code-block.content")
		case CODE_BLOCK_TITLE_NAME:
			if (toValue(isEditingDisabled)) {
				return t(
					"editor.placeholders.content.extended-code-block.header-title-empty",
				)
			}

			return t("editor.placeholders.content.extended-code-block.header-title")
		case ParameterListHeader.name:
			if (toValue(isEditingDisabled)) {
				return t(
					"editor.placeholders.content.split-documentation.parameter-list.header-name-empty",
				)
			}

			return t(
				"editor.placeholders.content.split-documentation.parameter-list.header-name",
			)
		case ParameterListItemHeaderType.name:
			if (toValue(isEditingDisabled)) {
				return t(
					"editor.placeholders.content.split-documentation.parameter-list.item.header-type-empty",
				)
			}

			return t(
				"editor.placeholders.content.split-documentation.parameter-list.item.header-type",
			)
		case ParameterListItemHeaderTitle.name:
			if (toValue(isEditingDisabled)) {
				return t(
					"editor.placeholders.content.split-documentation.parameter-list.item.header-name-empty",
				)
			}

			return t(
				"editor.placeholders.content.split-documentation.parameter-list.item.header-name",
			)
		case MERMAID_BLOCK_NAME:
			if (toValue(isEditingDisabled)) {
				return t("editor.placeholders.content.mermaid.content-empty")
			}

			return t("editor.placeholders.content.mermaid.content")
	}

	return ""
}

// Based on: https://github.com/ueberdosis/tiptap/blob/develop/packages/extensions/src/placeholder/placeholder.ts#L7
export interface DynamicPlaceholderOptions {
	/**
	 * **The class name for the empty editor**
	 * @default 'is-editor-empty'
	 */
	emptyEditorClass: string

	/**
	 * **The class name for empty nodes**
	 * @default 'is-empty'
	 */
	emptyNodeClass: string

	/**
	 * **The placeholder content**
	 *
	 * You can use a function to return a dynamic placeholder or a string.
	 * @default 'Write something …'
	 */
	placeholder:
		| ((PlaceholderProps: {
				editor: Editor
				node: ProsemirrorNode
				pos: number
				hasAnchor: boolean
		  }) => string)
		| string

	/**
	 * **Checks if the placeholder should be only shown when the editor is editable.**
	 *
	 * If true, the placeholder will only be shown when the editor is editable.
	 * If false, the placeholder will always be shown.
	 * @default true
	 */
	showOnlyWhenEditable: boolean

	/**
	 * **Checks if the placeholder should be only shown when the target nodes are empty.**
	 *
	 * If "current", the placeholder will only be shown when the current node is empty.
	 * If "any", the placeholder will be shown when any node is empty.
	 * If an array of strings/names, the placeholder will be shown when the nodes
	 * with the specified names are empty. This applies to the "current" node
	 * too.
	 * @default true
	 */
	showOnlyNodes:
		| "current"
		| "any"
		| {
				parent: boolean // indicates if the name filter should apply to the node's parent
				name: string
		  }[]

	/**
	 * **Controls if the placeholder should be shown for all descendents.**
	 *
	 * If true, the placeholder will be shown for all descendents.
	 * If false, the placeholder will only be shown for the current node.
	 * @default false
	 */
	includeChildren: boolean
}

// Based on: https://github.com/ueberdosis/tiptap/blob/develop/packages/extensions/src/placeholder/placeholder.ts#L63
// The official Placeholder was copied/forked because we need to support
// a mix of permament and temporary placeholders in different nodes.
const DynamicPlaceholder = Extension.create<DynamicPlaceholderOptions>({
	name: "dynamic-placeholder",
	addOptions() {
		return {
			emptyEditorClass: "is-editor-empty",
			emptyNodeClass: "is-empty",
			placeholder: "Write something…",
			showOnlyWhenEditable: true,
			showOnlyNodes: "current",
			includeChildren: false,
		}
	},
	addProseMirrorPlugins() {
		return [
			new Plugin({
				key: new PluginKey("dynamic-placeholder"),
				props: {
					decorations: ({ doc, selection }) => {
						const active =
							this.editor.isEditable || !this.options.showOnlyWhenEditable
						const { anchor } = selection
						const decorations: Decoration[] = []

						if (!active) {
							return null
						}

						const isEmptyDoc = this.editor.isEmpty

						doc.descendants((node, pos) => {
							const nodeParent = doc.resolve(pos).parent
							const hasAnchor = anchor >= pos && anchor <= pos + node.nodeSize
							const isEmpty = !node.isLeaf && isNodeEmpty(node)

							const showAny = this.options.showOnlyNodes === "any"
							const showSelectedNodes =
								Array.isArray(this.options.showOnlyNodes) &&
								this.options.showOnlyNodes.some((nodeFilter) => {
									if (nodeFilter.parent) {
										return nodeFilter.name === nodeParent.type.name
									}

									return nodeFilter.name === node.type.name
								})

							if ((hasAnchor || showAny || showSelectedNodes) && isEmpty) {
								const classes = [this.options.emptyNodeClass]

								if (isEmptyDoc) {
									classes.push(this.options.emptyEditorClass)
								}

								const decoration = Decoration.node(pos, pos + node.nodeSize, {
									class: classes.join(" "),
									"data-placeholder":
										typeof this.options.placeholder === "function"
											? this.options.placeholder({
													editor: this.editor,
													node,
													pos,
													hasAnchor,
												})
											: this.options.placeholder,
								})

								decorations.push(decoration)
							}

							return this.options.includeChildren
						})

						return DecorationSet.create(doc, decorations)
					},
				},
			}),
		]
	},
})
