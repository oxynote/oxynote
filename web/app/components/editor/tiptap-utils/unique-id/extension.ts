import {
	combineTransactionSteps,
	Extension,
	findChildren,
	findChildrenInRange,
	getChangedRanges,
} from "@tiptap/core"
import Paragraph from "@tiptap/extension-paragraph"
import type { Node as ProseMirrorNode } from "@tiptap/pm/model"
import { Fragment, Slice } from "@tiptap/pm/model"
import type { Transaction } from "@tiptap/pm/state"
import { Plugin, PluginKey } from "@tiptap/pm/state"
import { v4 as uuidv4 } from "uuid"

import { findDuplicates } from "./helpers/findDuplicates.js"

export interface UniqueIDGenerationContext {
	node: ProseMirrorNode
	pos: number
}

export interface UniqueIDOptions {
	/**
	 * The name of the attribute to add the unique ID to.
	 * @default "id"
	 */
	attributeName: string
	/**
	 * The types of nodes to add unique IDs to.
	 * @default []
	 */
	types: string[]
	/**
	 * The function that generates the unique ID. By default, a UUID v4 is
	 * generated. However, you can provide your own function to generate the
	 * unique ID based on the node type and the position.
	 */
	generateID: (ctx: UniqueIDGenerationContext) => any
	/**
	 * Ignore some mutations, for example applied from other users through the collaboration plugin.
	 *
	 * @default null
	 */
	filterTransaction: ((transaction: Transaction) => boolean) | null
	/**
	 * Whether to update the document by adding unique IDs to the nodes. Set this
	 * property to `false` if the document is in `readonly` mode, is immutable, or
	 * you don't want it to be modified.
	 *
	 * @default true
	 */
	updateDocument: boolean
}

export const UniqueID = Extension.create<UniqueIDOptions>({
	name: "uniqueID",

	// we’ll set a very high priority to make sure this runs first
	// and is compatible with `appendTransaction` hooks of other extensions
	priority: 10000,

	addOptions() {
		return {
			attributeName: "id",
			types: [],
			generateID: () => uuidv4(),
			filterTransaction: null,
			updateDocument: true,
		}
	},

	addGlobalAttributes() {
		return [
			{
				types: this.options.types,
				attributes: {
					[this.options.attributeName]: {
						default: null,
						parseHTML: (element) =>
							element.getAttribute(`data-${this.options.attributeName}`),
						renderHTML: (attributes) => {
							if (!attributes[this.options.attributeName]) {
								return {}
							}

							return {
								[`data-${this.options.attributeName}`]: attributes[
									this.options.attributeName
								] as string,
							}
						},
					},
				},
			},
		]
	},

	// check initial content for missing ids
	onCreate() {
		if (!this.options.updateDocument) {
			return
		}

		const collaboration = this.editor.extensionManager.extensions.find(
			(ext) => ext.name === "collaboration",
		)
		const collaborationCaret = this.editor.extensionManager.extensions.find(
			(ext) => ext.name === "collaborationCaret",
		)

		// the provider is only used for its event emitter interface, so a
		// minimal structural type is enough here.
		interface SyncedEventProvider {
			on: (event: "synced", handler: () => void) => void
			off: (event: "synced", handler: () => void) => void
		}

		const collabExtensions = [collaboration, collaborationCaret].filter(Boolean)

		const providerOf = (ext: (typeof collabExtensions)[number]) =>
			(ext?.options as { provider?: SyncedEventProvider } | undefined)?.provider

		const collab = collabExtensions.find((ext) => providerOf(ext))
		const provider = providerOf(collab)

		const createIds = () => {
			const { view, state } = this.editor
			const { tr, doc } = state
			const { types, attributeName, generateID } = this.options
			const nodesWithoutId = findChildren(doc, (node) => {
				return (
					types.includes(node.type.name) && node.attrs[attributeName] === null
				)
			})

			nodesWithoutId.forEach(({ node, pos }) => {
				tr.setNodeMarkup(pos, undefined, {
					...node.attrs,
					[attributeName]: generateID({ node, pos }) as string,
				})
			})

			tr.setMeta("addToHistory", false)

			view.dispatch(tr)

			if (provider) {
				provider.off("synced", createIds)
			}
		}

		/**
		 * We need to handle collaboration a bit different here
		 * because we can't automatically add IDs when the provider is not yet synced
		 * otherwise we end up with empty paragraphs
		 */
		if (collab) {
			if (!provider) {
				createIds()
				return
			}

			provider.on("synced", createIds)
		} else {
			createIds()
			return
		}
	},

	addProseMirrorPlugins() {
		if (!this.options.updateDocument) {
			return []
		}

		let dragSourceElement: Element | null = null
		let transformPasted = false

		return [
			new Plugin({
				key: new PluginKey("uniqueID"),

				appendTransaction: (transactions, oldState, newState) => {
					const hasDocChanges =
						transactions.some((transaction) => transaction.docChanged) &&
						!oldState.doc.eq(newState.doc)
					const filterTransactions =
						this.options.filterTransaction &&
						transactions.some((tr) => !this.options.filterTransaction?.(tr))

					const isCollabTransaction = transactions.find((tr) =>
						tr.getMeta("y-sync$"),
					)

					if (isCollabTransaction) {
						return
					}

					if (!hasDocChanges || filterTransactions) {
						return
					}

					const { tr } = newState

					const { types, attributeName, generateID } = this.options
					const transform = combineTransactionSteps(
						oldState.doc,
						transactions as Transaction[],
					)
					const { mapping } = transform

					// get changed ranges based on the old state
					const changes = getChangedRanges(transform)

					changes.forEach(({ newRange }) => {
						const newNodes = findChildrenInRange(
							newState.doc,
							newRange,
							(node) => {
								return types.includes(node.type.name)
							},
						)

						const newIds = newNodes
							.map(({ node }) => node.attrs[attributeName] as string | null)
							.filter((id) => id !== null)

						newNodes.forEach(({ node, pos }, i) => {
							// instead of checking `node.attrs[attributeName]` directly
							// we look at the current state of the node within `tr.doc`.
							// this helps to prevent adding new ids to the same node
							// if the node changed multiple times within one transaction
							const id = tr.doc.nodeAt(pos)?.attrs[attributeName] as
								| string
								| null

							if (id === null) {
								tr.setNodeMarkup(pos, undefined, {
									...node.attrs,
									[attributeName]: generateID({ node, pos }) as string,
								})

								return
							}

							const nextNode = newNodes[i + 1]

							// ID Swap Logic - Only for paragraph nodes
							//
							// This handles a specific editing scenario: pressing Enter at the
							// START of a paragraph splits it into two nodes that both inherit
							// the same ID:
							//
							//   Before: <p id="abc">Hello world</p>
							//   After:  <p id="abc"></p>           (empty, cursor here)
							//           <p id="abc">Hello world</p> (content moved here)
							//
							// The intent is that the ID should "follow the content" - if someone
							// has a bookmark to #abc, they want the paragraph with "Hello world",
							// not the newly created empty one. So we swap IDs:
							//
							//   Result: <p id="NEW"></p>           (new ID for empty node)
							//           <p id="abc">Hello world</p> (keeps original ID)
							//
							// IMPORTANT: This ONLY applies to paragraph nodes. Other nodes like
							// atom/leaf nodes (e.g., MetricBlock) always have content.size === 0
							// but should NOT have their IDs swapped - they use IDs as keys into
							// external stores and swapping causes data corruption.
							if (
								nextNode &&
								node.content.size === 0 &&
								node.type.name === Paragraph.name
							) {
								const nextNodeInTr = tr.doc.nodeAt(nextNode.pos)
								if (
									nextNodeInTr?.attrs[attributeName] &&
									nextNodeInTr.attrs[attributeName] !== id
								) {
									return
								}

								tr.setNodeMarkup(nextNode.pos, undefined, {
									...nextNode.node.attrs,
									[attributeName]: id,
								})
								newIds[i + 1] = id

								const generatedId = generateID({ node, pos }) as string

								tr.setNodeMarkup(pos, undefined, {
									...node.attrs,
									[attributeName]: generatedId,
								})
								newIds[i] = generatedId

								return tr
							}

							const duplicatedNewIds = findDuplicates(newIds)

							// check if the node doesn’t exist in the old state
							const { deleted } = mapping.invert().mapResult(pos)

							const newNode = deleted && duplicatedNewIds.includes(id)

							if (newNode) {
								tr.setNodeMarkup(pos, undefined, {
									...node.attrs,
									[attributeName]: generateID({ node, pos }) as string,
								})
							}
						})
					})

					if (!tr.steps.length) {
						return
					}

					// `tr.setNodeMarkup` resets the stored marks
					// so we'll restore them if they exist
					tr.setStoredMarks(newState.tr.storedMarks)

					// Mark this transaction as coming from UniqueID
					// to prevent infinite loops with other extensions (e.g., TrailingNode)
					tr.setMeta("__uniqueIDTransaction", true)

					return tr
				},

				// we register a global drag handler to track the current drag source element
				view(view) {
					const handleDragstart = (event: DragEvent) => {
						dragSourceElement = view.dom.parentElement?.contains(
							event.target as Element,
						)
							? view.dom.parentElement
							: null
					}

					window.addEventListener("dragstart", handleDragstart)

					return {
						destroy() {
							window.removeEventListener("dragstart", handleDragstart)
						},
					}
				},

				props: {
					// `handleDOMEvents` is called before `transformPasted`
					// so we can do some checks before
					handleDOMEvents: {
						// only create new ids for dropped content
						// or dropped content while holding `alt`
						// or content is dragged from another editor
						drop: (view, event) => {
							if (
								dragSourceElement !== view.dom.parentElement ||
								event.dataTransfer?.effectAllowed === "copyMove" ||
								event.dataTransfer?.effectAllowed === "copy"
							) {
								dragSourceElement = null
								transformPasted = true
							}

							return false
						},
						// always create new ids on pasted content
						paste: () => {
							transformPasted = true

							return false
						},
					},

					// we’ll remove ids for every pasted node
					// so we can create a new one within `appendTransaction`
					transformPasted: (slice) => {
						if (!transformPasted) {
							return slice
						}

						const { types, attributeName } = this.options
						const removeId = (fragment: Fragment): Fragment => {
							const list: ProseMirrorNode[] = []

							fragment.forEach((node) => {
								// don’t touch text nodes
								if (node.isText) {
									list.push(node)

									return
								}

								// check for any other child nodes
								if (!types.includes(node.type.name)) {
									list.push(node.copy(removeId(node.content)))

									return
								}

								// remove id
								const nodeWithoutId = node.type.create(
									{
										...node.attrs,
										[attributeName]: null,
									},
									removeId(node.content),
									node.marks,
								)

								list.push(nodeWithoutId)
							})

							return Fragment.from(list)
						}

						// reset check
						transformPasted = false

						return new Slice(
							removeId(slice.content),
							slice.openStart,
							slice.openEnd,
						)
					},
				},
			}),
		]
	},
})
