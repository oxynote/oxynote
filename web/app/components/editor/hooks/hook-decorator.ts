import { Extension } from "@tiptap/core"
import { Plugin, PluginKey, type Transaction } from "prosemirror-state"
import { Decoration, DecorationSet } from "prosemirror-view"
import { cn } from "@/lib/utils"
import type { Node as PMNode } from "prosemirror-model"
import type { EditorView } from "prosemirror-view"
import {
	CODE_BLOCK_NAME,
	FILE_BLOCK_NAME,
	IMAGE_BLOCK_NAME,
	METRIC_BLOCK_NAME,
} from "../blocks/node-names"
import {
	HOOK_BAR_STATUS_CLASS,
	hookGroupStatus,
	type HookGroupStatus,
} from "./hook-status"

/** block types that use node views and need the decoration widget placed
 * outside the node (at `pos`) rather than inside (at `pos + 1`) */
const NODE_VIEW_BLOCK_NAMES: ReadonlySet<string> = new Set([
	CODE_BLOCK_NAME,
	IMAGE_BLOCK_NAME,
	FILE_BLOCK_NAME,
])

declare module "@tiptap/core" {
	interface Commands<ReturnType> {
		/** Command group for HookDecorator */
		hookDecorator: {
			/** Recompute decorations when hooks change */
			refreshHookDecorations: () => ReturnType
		}
	}
}

interface Options {
	attributeName: string
	getHooks: () => DocumentHook[]
}

type BarStatus = Exclude<HookGroupStatus, "fresh">

const key = new PluginKey("hook-decorator")

// barStatusByBlock sums up the hooks of each block into what its bar
// shows. A block whose hooks are all fresh is left out.
function barStatusByBlock(hooks: DocumentHook[]): Map<string, BarStatus> {
	const hooksByBlock = new Map<string, DocumentHook[]>()

	for (const hook of hooks) {
		if (hook.blockId === null) {
			continue
		}

		hooksByBlock.set(hook.blockId, [
			...(hooksByBlock.get(hook.blockId) ?? []),
			hook,
		])
	}

	const statuses = new Map<string, BarStatus>()

	for (const [blockId, blockHooks] of hooksByBlock) {
		const status = hookGroupStatus(blockHooks)
		if (status && status !== "fresh") {
			statuses.set(blockId, status)
		}
	}

	return statuses
}

const HOOK_DECORATION_ATTR = "data-hook-decoration"

// extra properties the widget element carries so its position can be
// recomputed from outside the decoration.
interface HookDecorationElement extends HTMLElement {
	__updatePosition?: () => void
	ignoreMutation?: boolean
}

function buildDecorations(
	doc: PMNode,
	opt: Options,
	statuses: Map<string, BarStatus>,
): DecorationSet {
	const decorations: Decoration[] = []

	doc.descendants((node, pos) => {
		if (!node.isBlock) {
			return false
		}

		const val = node.attrs[opt.attributeName] as string | null | undefined
		const hasAttr = val != null && val !== ""
		const status = hasAttr ? statuses.get(val) : undefined
		if (!status) {
			return true
		}

		decorations.push(
			Decoration.node(pos, pos + node.nodeSize, {
				class: `relative`,
			}),
		)

		// for atom nodes and code blocks, widget must be placed at pos
		// (not pos + 1) so it renders outside the node view wrapper.
		// for metric blocks, place the widget outside the parent metric grid
		// entirely to avoid interfering with the grid's CSS layout and gap zones.
		const placeOutside =
			node.isAtom || NODE_VIEW_BLOCK_NAMES.has(node.type.name)

		let widgetPos: number
		let targetNodePos: number // the node whose DOM rect we align to

		if (node.type.name === METRIC_BLOCK_NAME) {
			// place before the parent metric grid
			const $pos = doc.resolve(pos)
			widgetPos = $pos.before($pos.depth)
			targetNodePos = pos
		} else if (placeOutside) {
			widgetPos = pos
			targetNodePos = pos
		} else {
			widgetPos = pos + 1
			targetNodePos = pos
		}

		decorations.push(
			Decoration.widget(
				widgetPos,
				(view) => {
					const el: HookDecorationElement = document.createElement("span")

					el.setAttribute("aria-hidden", "true")
					el.setAttribute(CLONE_IGNORE_ATTR, "true")
					el.setAttribute(HOOK_DECORATION_ATTR, "true")

					const editorContainer = view.dom.closest(".content-editor")

					const updatePosition = () => {
						const nodeDOM = view.nodeDOM(targetNodePos) as HTMLElement | null
						if (!nodeDOM || !editorContainer) {
							return
						}

						const containerRect = editorContainer.getBoundingClientRect()
						const nodeRect = nodeDOM.getBoundingClientRect()
						const leftOffset = containerRect.left - nodeRect.left

						// when placed outside the node, we position relative to the
						// widget's offset parent instead of the node itself
						if (placeOutside) {
							const offsetParent = el.offsetParent as HTMLElement | null
							if (offsetParent) {
								const parentRect = offsetParent.getBoundingClientRect()
								el.style.top = `${nodeRect.top - parentRect.top}px`
								el.style.left = `${containerRect.left - parentRect.left}px`
							}
						} else {
							el.style.left = `${leftOffset}px`
						}
						el.style.height = `${nodeRect.height}px`
					}

					// Store update function on the element itself
					el.__updatePosition = updatePosition

					requestAnimationFrame(updatePosition)

					el.className = cn(
						"pointer-events-none absolute top-0",
						"block w-1.25 rounded-r-lg",
						HOOK_BAR_STATUS_CLASS[status],
						// the bar hugs the page edge in full view; in compact view the
						// column floats, so it becomes a pill
						"in-data-compact-view:rounded-full",
						placeOutside && "m-0!",
					)
					el.ignoreMutation = true
					return el
				},
				{
					side: -1,
					// the key carries the status, so a bar whose status changes
					// is drawn again
					key: `hook-decoration-${pos}-${status}`,
					ignoreSelection: true,
				},
			),
		)

		return true
	})

	return DecorationSet.create(doc, decorations)
}

export const HookDecorator = Extension.create<Partial<Options>>({
	name: "hookDecorator",
	addOptions() {
		return {
			attributeName: "uid",
			getHooks: () => [],
		} satisfies Options
	},
	addCommands() {
		return {
			refreshHookDecorations:
				() =>
				({
					tr,
					dispatch,
				}: {
					tr: Transaction
					dispatch?: (tr: Transaction) => void
				}) => {
					dispatch?.(tr.setMeta(key, { hooksChanged: true }))
					return true
				},
		}
	},
	addProseMirrorPlugins() {
		const opt = this.options as Options

		return [
			new Plugin({
				key,
				state: {
					init: (_cfg, { doc }) => {
						const statuses = barStatusByBlock(opt.getHooks())
						return {
							statuses,
							decos: buildDecorations(doc, opt, statuses),
						}
					},
					apply(tr, old, _oldState, newState) {
						const meta = tr.getMeta(key) as
							{ hooksChanged?: boolean } | undefined

						if (tr.docChanged || meta?.hooksChanged) {
							const statuses = barStatusByBlock(opt.getHooks())
							return {
								statuses,
								decos: buildDecorations(newState.doc, opt, statuses),
							}
						}

						return {
							statuses: old.statuses,
							decos: old.decos.map(tr.mapping, tr.doc),
						}
					},
				},
				props: {
					decorations(state) {
						return this.getState(state)?.decos
					},
				},
				view(editorView: EditorView) {
					const updateAllDecorations = () => {
						const decorations = editorView.dom.querySelectorAll(
							`[${HOOK_DECORATION_ATTR}]`,
						)
						decorations.forEach((el) => {
							;(el as HookDecorationElement).__updatePosition?.()
						})
					}

					// observe the editor element to catch all layout changes
					// (sidebar toggle, notification sidebar, split view, window resize, etc.)
					const resizeObserver = new ResizeObserver(() => {
						requestAnimationFrame(updateAllDecorations)
					})
					resizeObserver.observe(editorView.dom)

					return {
						destroy() {
							resizeObserver.disconnect()
						},
					}
				},
			}),
		]
	},
})
