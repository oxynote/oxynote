import { Extension } from "@tiptap/core"
import { Plugin, PluginKey } from "@tiptap/pm/state"
import { canJoin } from "prosemirror-transform"
import type { NodeType } from "@tiptap/pm/model"
import type { Transaction } from "@tiptap/pm/state"

// Based on: https://github.com/NiclasDev63/tiptap-extension-auto-joiner
// This extension provides the expected behavior for lists, e.g. joining
// two adjacent lists to one. It can be extended to join not only lists, but
// all types of nodes that are next to each other, such as paragraphs or
// custom nodes.
// By default it joins bullet lists, ordered lists and task lists.

// https://discuss.prosemirror.net/t/how-to-autojoin-all-the-time/2957/4
// Ripped out from prosemirror-commands wrapDispatchForJoin
function autoJoin(
	tr: Transaction, // An old transaction
	newTr: Transaction, // The latest state
	nodeTypes: NodeType[], // The node type to join
) {
	// Find all ranges where we might want to join.
	let ranges: number[] = []
	for (const map of tr.mapping.maps) {
		ranges = ranges.map((range) => map.map(range))
		map.forEach((_s, _e, from, to) => ranges.push(from, to))
	}

	// Figure out which joinable points exist inside those ranges,
	// by checking all node boundaries in their parent nodes.
	const joinable: number[] = []
	for (let i = 0; i < ranges.length; i += 2) {
		const from = ranges[i],
			to = ranges[i + 1]
		if (from === undefined || to === undefined) continue
		const $from = tr.doc.resolve(from),
			depth = $from.sharedDepth(to),
			parent = $from.node(depth)
		for (
			let index = $from.indexAfter(depth), pos = $from.after(depth + 1);
			pos <= to;
			++index
		) {
			const after = parent.maybeChild(index)
			if (!after) break
			if (index && !joinable.includes(pos)) {
				const before = parent.child(index - 1)
				if (before.type == after.type && nodeTypes.includes(before.type))
					joinable.push(pos)
			}
			pos += after.nodeSize
		}
	}

	let joined = false

	// Join the joinable points
	joinable.sort((a, b) => a - b)
	for (let i = joinable.length - 1; i >= 0; i--) {
		const joinPos = joinable[i]
		if (joinPos === undefined) continue
		// Check canJoin against the newTr's current document state, not the old tr.doc
		// The positions were calculated from tr.doc but we need to verify they're still valid
		// in newTr.doc before attempting the join
		try {
			if (canJoin(newTr.doc, joinPos)) {
				newTr.join(joinPos)
				joined = true
			}
		} catch {
			// Position may be invalid in the new document, skip this join
		}
	}

	return joined
}

export interface AutoJoinerOptions {
	elementsToJoin: string[]
}

const AutoJoiner = Extension.create<AutoJoinerOptions>({
	name: "autoJoiner",

	addOptions() {
		return {
			elementsToJoin: [],
		}
	},

	addProseMirrorPlugins() {
		const plugin = new PluginKey(this.name)
		const joinableNodes = [
			this.editor.schema.nodes.bulletList,
			this.editor.schema.nodes.orderedList,
			this.editor.schema.nodes.taskList,
		]
		this.options.elementsToJoin.forEach((element) => {
			const nodeTyp = this.editor.schema.nodes[element]
			if (nodeTyp) {
				joinableNodes.push(nodeTyp)
			}
		})

		return [
			new Plugin({
				key: plugin,
				appendTransaction(transactions, _, newState) {
					// Create a new transaction.
					const newTr = newState.tr

					let joined = false
					for (const transaction of transactions) {
						// Skip transactions that don't modify the document
						if (!transaction.docChanged) {
							continue
						}

						const anotherJoin = autoJoin(
							transaction,
							newTr,
							joinableNodes as NodeType[],
						)
						joined = anotherJoin || joined
					}
					// Only return the transaction if we actually joined something
					// and the transaction has steps
					if (joined && newTr.steps.length > 0) {
						return newTr
					}
				},
			}),
		]
	},
})

export default AutoJoiner
