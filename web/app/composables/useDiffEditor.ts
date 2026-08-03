import type { ShallowRef, Ref } from "vue"
import type { Extensions, JSONContent } from "@tiptap/core"
import type * as Y from "yjs"
import { Editor } from "@tiptap/vue-3"
import Document from "@tiptap/extension-document"
import Heading from "@tiptap/extension-heading"
import Paragraph from "@tiptap/extension-paragraph"
import Text from "@tiptap/extension-text"
import { yXmlFragmentToProseMirrorRootNode } from "y-prosemirror"
import { DiffAttributes } from "~/components/editor/diff/diff-attributes"
import { DiffDecorations } from "~/components/editor/diff/diff-decorations"
import {
	DiffTextAddedMark,
	DiffTextRemovedMark,
} from "~/components/editor/diff/diff-text-marks"
import {
	DiffContentLock,
	DIFF_RECOMPUTE_TX_META,
} from "~/components/editor/diff/diff-content-lock"
import {
	NodeComment,
	type NodeCommentOverlayState,
} from "~/components/editor/comments/node-comment-extension"
import { editorProseClass } from "~/components/editor/schema-extensions"
import {
	computeMergedDocument,
	type MergeOptions,
} from "~/components/editor/diff/compute-merged-document"
import type { PositionMap } from "~/components/editor/diff/position-map"
import { buildPositionMapFromDoc } from "~/components/editor/diff/position-map"
import {
	CODE_BLOCK_NAME,
	CODE_BLOCK_TITLE_NAME,
	MERMAID_BLOCK_NAME,
	SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_HEADER_TITLE_NAME,
	SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_HEADER_TYPE_NAME,
	SPLIT_DOCUMENTATION_PARAMETER_LIST_HEADER_NAME,
} from "~/components/editor/blocks/node-names"
import {
	COMMENT_MARK_NAME,
	DIFF_TEXT_ADDED_MARK_NAME,
	DIFF_TEXT_REMOVED_MARK_NAME,
} from "~/components/editor/mark-names"
import { cn } from "~/lib/utils"

export interface UseDiffEditorOptions {
	/** debounce interval in ms (default: 250) */
	debounceMs?: number
	/** maximum time to wait before forcing a recompute during continuous edits (default: 1500) */
	maxWaitMs?: number
	/** options for the merge algorithm */
	mergeOptions?: MergeOptions
	/** called when the node comment icon overlay state changes */
	onNodeCommentOverlayStateChange?: (state: NodeCommentOverlayState) => void
	/** called when a node comment overlay/highlight is clicked */
	onNodeCommentClick?: (nodeCommentId: string) => void
}

export interface UseDiffEditorReturn {
	editor: ShallowRef<Editor | null>
	positionMap: Ref<PositionMap>
	destroy: () => void
	/** skip the next scheduled recompute (used when a comment-only change should not trigger diff refresh) */
	suppressNextRecompute: () => void
}

/**
 * creates a read-only merged diff editor that reactively compares two
 * yjs documents (target and active). subscribes to yjs update events on
 * both documents and recomputes the diff on changes.
 */
export function useDiffEditor(
	targetYdoc: () => Y.Doc,
	activeYdoc: () => Y.Doc,
	extensions: Extensions,
	options: UseDiffEditorOptions = {},
): UseDiffEditorReturn {
	const debounceMs = options.debounceMs ?? 250
	const maxWaitMs = options.maxWaitMs ?? 1500
	const mergeOptions = options.mergeOptions
	const onNodeCommentOverlayStateChange =
		options.onNodeCommentOverlayStateChange
	const onNodeCommentClick = options.onNodeCommentClick

	const editor = shallowRef<Editor | null>(null)
	const positionMap = ref<PositionMap>([])

	let destroyed = false

	// tracks how many pending comment-only operations should suppress a
	// recompute. a counter (not a boolean) so multiple calls before the
	// debounce fires are handled correctly. the expiry timestamp ensures
	// stale suppresses don't accidentally swallow a real recompute if a
	// collab edit arrives much later.
	let suppressCount = 0
	let suppressExpiry = 0

	// serialized snapshot of the last merged doc. used to skip the
	// expensive downstream work (doc replacement, DOM reconciliation,
	// decoration rebuild, position map rebuild) when the merge result
	// hasn't actually changed — common during collab when peers move
	// cursors or comment-only changes propagate.
	let previousMergedSnapshot: string | null = null

	const scheduleRecompute = useDebounceFn(recompute, debounceMs, {
		maxWait: maxWaitMs,
	})

	// collect all node type names so DiffAttributes registers on them.
	// text nodes cannot have attributes in ProseMirror and doc-level
	// attrs are not useful for diffing, so both are excluded.
	const allNodeTypes = extensions
		.filter((ext) => ext.type === "node")
		.map((ext) => ext.name)

	// extend code block nodes to allow diff text marks so that inline
	// diff additions/deletions inside code blocks can coexist with
	// comment marks without violating the ProseMirror schema.
	const diffCodeBlockMarks = `${COMMENT_MARK_NAME} ${DIFF_TEXT_ADDED_MARK_NAME} ${DIFF_TEXT_REMOVED_MARK_NAME}`
	const restrictedMarkNodes = new Set([
		CODE_BLOCK_NAME,
		CODE_BLOCK_TITLE_NAME,
		MERMAID_BLOCK_NAME,
		Heading.name,
		SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_HEADER_TITLE_NAME,
		SPLIT_DOCUMENTATION_PARAMETER_LIST_ITEM_HEADER_TYPE_NAME,
		SPLIT_DOCUMENTATION_PARAMETER_LIST_HEADER_NAME,
	])
	const patchedExtensions = extensions.map((ext) =>
		restrictedMarkNodes.has(ext.name)
			? ext.extend({ marks: diffCodeBlockMarks })
			: ext,
	)

	// create the merged editor instance
	const editorInstance = new Editor({
		editable: true,
		extensions: [
			Document,
			Text,
			...patchedExtensions,
			DiffTextAddedMark,
			DiffTextRemovedMark,
			DiffAttributes.configure({ types: allNodeTypes }),
			DiffDecorations,
			DiffContentLock,
			NodeComment.configure({
				types: allNodeTypes,
				onOverlayStateChange: onNodeCommentOverlayStateChange,
				onNodeCommentClick: onNodeCommentClick,
			}),
		],
		editorProps: {
			attributes: {
				class: cn(editorProseClass, "caret-transparent"),
				spellcheck: "false",
			},
		},
	})
	editor.value = editorInstance

	// subscribe to yjs updates on both documents
	const onTargetUpdate = () => scheduleRecompute()
	const onActiveUpdate = () => scheduleRecompute()

	let currentTargetYdoc = targetYdoc()
	let currentActiveYdoc = activeYdoc()

	currentTargetYdoc.on("update", onTargetUpdate)
	currentActiveYdoc.on("update", onActiveUpdate)

	// watch for ydoc reference changes and re-subscribe
	watch([targetYdoc, activeYdoc], ([newTarget, newActive]) => {
		let changed = false

		if (newTarget !== currentTargetYdoc) {
			currentTargetYdoc.off("update", onTargetUpdate)
			currentTargetYdoc = newTarget
			currentTargetYdoc.on("update", onTargetUpdate)
			changed = true
		}

		if (newActive !== currentActiveYdoc) {
			currentActiveYdoc.off("update", onActiveUpdate)
			currentActiveYdoc = newActive
			currentActiveYdoc.on("update", onActiveUpdate)
			changed = true
		}

		if (changed) {
			previousMergedSnapshot = null
			recompute()
		}
	})

	// initial computation — run synchronously so the editor starts with
	// the merged content and avoids an empty→full doc transition
	recompute()

	function extractJSON(ydoc: Y.Doc) {
		try {
			const fragment = ydoc.getXmlFragment("content")
			// check if the fragment has content
			if (fragment.length === 0) {
				return null
			}
			return yXmlFragmentToProseMirrorRootNode(
				fragment,
				editor.value!.schema,
			).toJSON()
		} catch {
			return null
		}
	}

	/** strip trailing empty paragraphs added by the TrailingNode extension */
	function stripTrailingEmptyParagraphs(doc: JSONContent): JSONContent {
		if (!doc.content) {
			return doc
		}

		let end = doc.content.length
		while (end > 0) {
			const node = doc.content[end - 1]!
			if (
				node.type !== Paragraph.name ||
				(node.content && node.content.length > 0)
			) {
				break
			}
			end--
		}

		return { ...doc, content: doc.content.slice(0, end) }
	}

	function recompute() {
		if (destroyed || !editor.value) {
			return
		}

		if (suppressCount > 0 && Date.now() < suppressExpiry) {
			suppressCount--
			return
		}
		suppressCount = 0

		const rawTarget = extractJSON(currentTargetYdoc)
		const rawActive = extractJSON(currentActiveYdoc)

		if (!rawTarget && !rawActive) {
			return
		}

		const emptyDoc: JSONContent = { type: "doc", content: [] }

		const targetJSON = rawTarget
			? stripTrailingEmptyParagraphs(rawTarget)
			: emptyDoc
		const activeJSON = rawActive
			? stripTrailingEmptyParagraphs(rawActive)
			: emptyDoc

		if (!targetJSON.content?.length && !activeJSON.content?.length) {
			return
		}

		const result = computeMergedDocument(targetJSON, activeJSON, mergeOptions)

		// skip the expensive downstream work if the merged doc is
		// identical to the previous result
		const snapshot = jsonStableStringify(result.doc)
		if (snapshot === previousMergedSnapshot) {
			return
		}

		previousMergedSnapshot = snapshot

		// dispatch setContent with recompute meta to bypass DiffContentLock
		const { tr } = editor.value.state
		const newDoc = editor.value.schema.nodeFromJSON(result.doc)
		tr.replaceWith(0, editor.value.state.doc.content.size, newDoc.content)
		tr.setMeta(DIFF_RECOMPUTE_TX_META, true)
		editor.value.view.dispatch(tr)

		// rebuild position map from the actual ProseMirror doc for accurate
		// positions instead of the JSON-based estimates
		positionMap.value = buildPositionMapFromDoc(editor.value.state.doc)
	}

	function destroy() {
		destroyed = true
		currentTargetYdoc.off("update", onTargetUpdate)
		currentActiveYdoc.off("update", onActiveUpdate)
		editorInstance.destroy()
		editor.value = null
	}

	function suppressNextRecompute() {
		suppressCount++
		// expire the suppress after one full debounce + maxWait cycle so
		// stale suppresses don't swallow unrelated recomputes
		suppressExpiry = Date.now() + debounceMs + maxWaitMs
	}

	return { editor, positionMap, destroy, suppressNextRecompute }
}
