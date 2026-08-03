import { Extension } from "@tiptap/core"
import { Plugin, PluginKey } from "@tiptap/pm/state"

/**
 * meta key used to mark transactions that should bypass the content
 * lock (e.g. adding/removing comment marks in the diff editor).
 */
export const DIFF_COMMENT_TX_META = "diffCommentTransaction"

/**
 * meta key used to mark recompute transactions that replace the entire
 * diff document content.
 */
export const DIFF_RECOMPUTE_TX_META = "diffRecomputeTransaction"

/**
 * prevents all content modifications in the diff editor while still
 * allowing text selection, comment-related transactions, and recompute
 * transactions. bypassed via DIFF_COMMENT_TX_META or DIFF_RECOMPUTE_TX_META.
 */
export const DiffContentLock = Extension.create({
	name: "diffContentLock",

	addProseMirrorPlugins() {
		return [
			new Plugin({
				key: new PluginKey("diffContentLock"),
				filterTransaction(tr) {
					if (!tr.docChanged) {
						return true
					}

					if (tr.getMeta(DIFF_COMMENT_TX_META)) {
						return true
					}

					if (tr.getMeta(DIFF_RECOMPUTE_TX_META)) {
						return true
					}

					return false
				},
			}),
		]
	},
})
