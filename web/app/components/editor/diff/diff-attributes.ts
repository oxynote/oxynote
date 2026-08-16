import { Extension } from "@tiptap/core"

export interface DiffAttributeOptions {
	/** node type names to attach diff attributes to */
	types: string[]
}

/**
 * adds diff metadata attributes to the specified node types. these are
 * used by the merged diff editor to track which blocks are added,
 * removed, modified, or unchanged relative to the original version.
 *
 * custom node components read diffStatus from node.attrs and render
 * data-diff-status on their NodeViewWrapper.
 */
export const DiffAttributes = Extension.create<DiffAttributeOptions>({
	name: "diffAttributes",

	addOptions() {
		return { types: [] }
	},

	addGlobalAttributes() {
		return [
			{
				types: this.options.types,
				attributes: {
					diffStatus: {
						default: null,
						renderHTML: (attributes) => {
							if (!attributes.diffStatus) {
								return {}
							}

							return {
								"data-diff-status": attributes.diffStatus as string,
							}
						},
						parseHTML: (element) =>
							element.getAttribute("data-diff-status") ?? null,
					},
					modifiedIndex: {
						default: null,
						rendered: false,
					},
					originalIndex: {
						default: null,
						rendered: false,
					},
					oldNode: {
						default: null,
						rendered: false,
					},
					modifiedTextContent: {
						default: null,
						rendered: false,
					},
				},
			},
		]
	},
})
