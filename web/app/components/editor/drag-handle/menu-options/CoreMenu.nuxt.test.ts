import { afterEach, beforeEach, describe, it, vi } from "vitest"
import { toast } from "vue-sonner"
import CoreMenu from "./CoreMenu.vue"
import {
	hoveredBlock,
	mountMenuOptions,
	type HoveredBlock,
} from "./test-helpers"
import {
	commandArgs,
	commandNames,
	makeEditor,
} from "../../test-helpers/node-view"
import { METRIC_BLOCK_NAME } from "../../blocks/node-names"
import { SUPPRESS_SCROLL_TO_SELECTION_META } from "../../scroll-control"
import { clearTeleportedOverlays, menuItem, t } from "~/components/test-helpers"

vi.mock("vue-sonner", () => ({
	toast: {
		custom: vi.fn(),
		dismiss: vi.fn(),
	},
}))

const COPY_LINK = "editor.drag-handle.options.copy-link"
const DUPLICATE = "editor.drag-handle.options.default.duplicate-block"
const ADD_COMMENT = "editor.drag-handle.options.add-node-comment"
const OPEN_COMMENT = "editor.drag-handle.options.open-node-comment"
const DELETE = "editor.drag-handle.options.delete-block"

// the editor the menu reads the document through, plus the transaction
// and view calls a deletion goes through
function makeMenuEditor(
	options: {
		nodeAtPos?: {
			typeName?: string
			nodeSize?: number
			attrs?: Record<string, unknown>
			json?: Record<string, unknown>
		} | null
		ancestorTypeNames?: string[]
		hasNodeComment?: boolean
	} = {},
) {
	const node =
		options.nodeAtPos === null
			? null
			: {
					type: { name: options.nodeAtPos?.typeName ?? "paragraph" },
					nodeSize: options.nodeAtPos?.nodeSize ?? 6,
					attrs: options.nodeAtPos?.attrs ?? { uid: "block-1" },
					toJSON: () =>
						options.nodeAtPos?.json ?? {
							type: options.nodeAtPos?.typeName ?? "paragraph",
							attrs: options.nodeAtPos?.attrs ?? { uid: "block-1" },
						},
				}
	const ancestors = options.ancestorTypeNames ?? []
	const deletedRanges: [number, number][] = []
	const transaction = {
		delete: (from: number, to: number) => {
			deletedRanges.push([from, to])

			return transaction
		},
		doc: { forEach: () => undefined },
	}
	const dispatch = vi.fn()
	const focus = vi.fn()
	const hasNodeComment = vi.fn(() => options.hasNodeComment ?? false)

	const { editor, commands } = makeEditor({
		state: {
			doc: {
				nodeAt: () => node,
				resolve: () => ({
					depth: ancestors.length,
					node: (depth: number) => ({
						type: { name: ancestors[depth - 1] ?? "doc" },
					}),
				}),
				nodesBetween: () => undefined,
			},
			tr: transaction,
			schema: { nodes: {} },
		},
		view: { dispatch: dispatch },
		commands: { hasNodeComment: hasNodeComment, focus: focus },
	})

	return { editor, commands, dispatch, focus, deletedRanges, hasNodeComment }
}

function mountCoreMenu(
	editor: ReturnType<typeof makeMenuEditor>["editor"],
	hovered: HoveredBlock | null,
	dataSyncProvider: unknown = null,
) {
	return mountMenuOptions(CoreMenu, {
		editor: editor,
		hovered: hovered,
		dataSyncProvider: dataSyncProvider,
	})
}

function block(typeName: string, attrs: Record<string, unknown> = {}) {
	return hoveredBlock(10, { typeName: typeName, attrs: attrs })
}

function menuLabels(): string[] {
	return Array.from(
		document.body.querySelectorAll<HTMLElement>("[role^='menuitem']"),
	).map((item) => item.textContent.trim())
}

// the editable flag is a shared cookie state, the editor store and the
// mocked toast module are app-wide, and the menu body is teleported into
// a shared <body>, so these tests cannot interleave
describe("<CoreMenu>", { concurrent: false }, () => {
	beforeEach(() => {
		vi.mocked(toast.custom).mockReset()
		useEditorMeta().setEditable(true)
		useEditorStore().setReviewableDiffActive(false)
		useEditorStore().setBranchReviewableActionsActive(false)
	})

	afterEach(clearTeleportedOverlays)

	it("offers the block actions for an ordinary block", async ({ expect }) => {
		const { editor } = makeMenuEditor()

		await mountCoreMenu(editor, block("paragraph", { uid: "block-1" }))

		expect(menuLabels()).toEqual([
			t("editor.drag-handle.options.default.add-above-block"),
			t("editor.drag-handle.options.default.add-below-block"),
			t(COPY_LINK),
			t(DUPLICATE),
			t(ADD_COMMENT),
			t(DELETE),
		])
	})

	it("offers only the comment action in read mode", async ({ expect }) => {
		useEditorMeta().setEditable(false)
		useEditorStore().setBranchReviewableActionsActive(true)
		const { editor } = makeMenuEditor()

		await mountCoreMenu(editor, block("paragraph", { uid: "block-1" }))

		expect(menuLabels()).toEqual([t(COPY_LINK), t(ADD_COMMENT)])
	})

	it("offers no comment action while comments are not allowed", async ({
		expect,
	}) => {
		useEditorMeta().setEditable(false)
		const { editor } = makeMenuEditor()

		await mountCoreMenu(editor, block("paragraph", { uid: "block-1" }))

		expect(menuLabels()).toEqual([t(COPY_LINK)])
	})

	it("offers the split documentation actions for one of those blocks", async ({
		expect,
	}) => {
		const { editor } = makeMenuEditor()

		await mountCoreMenu(editor, block("splitDocumentation"))

		expect(menuLabels()).toContain(
			t("editor.drag-handle.options.split-doc.add-parameter-list"),
		)
		expect(menuLabels()).toContain(
			t("editor.drag-handle.options.split-doc.invert-sides"),
		)
		expect(menuLabels()).toContain(
			t("editor.drag-handle.options.default.add-above-block"),
		)
	})

	it("offers the right-side actions for a block inside one", async ({
		expect,
	}) => {
		const { editor } = makeMenuEditor({
			ancestorTypeNames: ["splitDocumentationRightSide"],
		})

		await mountCoreMenu(editor, block("codeBlock"))

		expect(menuLabels()).toContain(
			t(
				"editor.drag-handle.options.split-doc-right-side.add-code-block-above-block",
			),
		)
		expect(menuLabels()).not.toContain(
			t("editor.drag-handle.options.default.add-above-block"),
		)
	})

	it("offers the parameter list actions for a parameter list", async ({
		expect,
	}) => {
		const { editor } = makeMenuEditor()

		await mountCoreMenu(editor, block("splitDocumentationParameterList"))

		expect(menuLabels()).toContain(
			t("editor.drag-handle.options.split-doc-parameter-list.add-above-block"),
		)
	})

	it("offers the parameter actions for a parameter", async ({ expect }) => {
		const { editor } = makeMenuEditor()

		await mountCoreMenu(editor, block("splitDocumentationParameterListItem"))

		expect(menuLabels()).toContain(
			t(
				"editor.drag-handle.options.split-doc-parameter-list-item.add-above-block",
			),
		)
	})

	it("offers the resize action for a metric block", async ({ expect }) => {
		const { editor } = makeMenuEditor()

		await mountCoreMenu(editor, block(METRIC_BLOCK_NAME))

		expect(menuLabels()).toContain(
			t("editor.drag-handle.options.metric-block.width"),
		)
		expect(menuLabels()).toContain(
			t("editor.drag-handle.options.default.add-above-block"),
		)
	})

	it("offers no link for a block without an id", async ({ expect }) => {
		const { editor } = makeMenuEditor({ nodeAtPos: { attrs: {} } })

		await mountCoreMenu(editor, block("paragraph"))

		expect(menuLabels()).not.toContain(t(COPY_LINK))
	})

	it("copies a link to the hovered block", async ({ expect }) => {
		const writeText = vi
			.spyOn(navigator.clipboard, "writeText")
			.mockResolvedValue(undefined)
		const { editor } = makeMenuEditor()
		await mountCoreMenu(editor, block("paragraph", { uid: "block-1" }))

		menuItem(t(COPY_LINK)).click()

		expect(writeText).toHaveBeenCalledTimes(1)
		expect(writeText.mock.calls[0]?.[0]).toContain("#block-1")
	})

	it("duplicates the hovered block right after it", async ({ expect }) => {
		const { editor, commands } = makeMenuEditor()
		await mountCoreMenu(editor, block("paragraph", { uid: "block-1" }))

		menuItem(t(DUPLICATE)).click()

		expect(commandNames(commands)).toEqual(["focus", "insertContentAt", "run"])
		expect(commandArgs(commands, "insertContentAt")).toEqual([
			16,
			{ type: "paragraph", attrs: { uid: null } },
		])
	})

	it("strips the comment marks off a duplicated block", async ({ expect }) => {
		const { editor, commands } = makeMenuEditor({
			nodeAtPos: {
				json: {
					type: "paragraph",
					attrs: { uid: "block-1", nodeCommentId: "c-1" },
					content: [
						{
							type: "text",
							text: "hi",
							marks: [{ type: "comment" }, { type: "bold" }],
						},
					],
				},
			},
		})
		await mountCoreMenu(editor, block("paragraph", { uid: "block-1" }))

		menuItem(t(DUPLICATE)).click()

		expect(commandArgs(commands, "insertContentAt")?.[1]).toEqual({
			type: "paragraph",
			attrs: { uid: null, nodeCommentId: null },
			content: [{ type: "text", text: "hi", marks: [{ type: "bold" }] }],
		})
	})

	it("duplicates a metric block without scrolling the selection", async ({
		expect,
	}) => {
		const { editor, commands } = makeMenuEditor({
			nodeAtPos: { typeName: METRIC_BLOCK_NAME, attrs: { uid: "block-1" } },
		})
		await mountCoreMenu(editor, block(METRIC_BLOCK_NAME, { uid: "block-1" }))

		menuItem(t(DUPLICATE)).click()

		expect(commandNames(commands)).toEqual([
			"setMeta",
			"focus",
			"insertContentAt",
			"run",
		])
		expect(commandArgs(commands, "setMeta")).toEqual([
			SUPPRESS_SCROLL_TO_SELECTION_META,
			true,
		])
	})

	it("duplicates nothing when the block is no longer there", async ({
		expect,
	}) => {
		const { editor, commands } = makeMenuEditor({ nodeAtPos: null })
		await mountCoreMenu(editor, block("paragraph", { uid: "block-1" }))

		menuItem(t(DUPLICATE)).click()

		expect(commands).toEqual([])
	})

	it("asks to add a comment on the hovered block", async ({ expect }) => {
		const { editor } = makeMenuEditor()
		const wrapper = await mountCoreMenu(
			editor,
			block("paragraph", { uid: "block-1" }),
		)

		menuItem(t(ADD_COMMENT)).click()
		await nextTick()

		expect(wrapper.findComponent(CoreMenu).emitted("add-node-comment")).toEqual(
			[[10]],
		)
	})

	it("asks to open the comment a block already carries", async ({ expect }) => {
		const { editor } = makeMenuEditor({ hasNodeComment: true })
		const wrapper = await mountCoreMenu(
			editor,
			block("paragraph", { uid: "block-1" }),
		)

		expect(menuLabels()).toContain(t(OPEN_COMMENT))
		expect(menuLabels()).not.toContain(t(ADD_COMMENT))

		menuItem(t(OPEN_COMMENT)).click()
		await nextTick()

		expect(
			wrapper.findComponent(CoreMenu).emitted("open-node-comment"),
		).toEqual([[10]])
	})

	it("deletes the hovered block", async ({ expect }) => {
		const { editor, dispatch, focus, deletedRanges } = makeMenuEditor()
		await mountCoreMenu(editor, block("paragraph", { uid: "block-1" }))

		menuItem(t(DELETE)).click()
		await nextTick()

		expect(deletedRanges).toEqual([[10, 16]])
		expect(dispatch).toHaveBeenCalledTimes(1)
		expect(focus).toHaveBeenCalledWith(undefined, { scrollIntoView: false })
		expect(toast.custom).toHaveBeenCalledTimes(0)
	})

	it("deletes nothing when the block is no longer there", async ({
		expect,
	}) => {
		const { editor, dispatch, deletedRanges } = makeMenuEditor({
			nodeAtPos: null,
		})
		await mountCoreMenu(editor, block("paragraph", { uid: "block-1" }))

		menuItem(t(DELETE)).click()
		await nextTick()

		expect(deletedRanges).toEqual([])
		expect(dispatch).toHaveBeenCalledTimes(0)
	})

	it("refuses to delete a block someone else is editing", async ({
		expect,
	}) => {
		const { editor, dispatch, deletedRanges } = makeMenuEditor()
		const provider = {
			awareness: {
				clientID: 1,
				getStates: () =>
					new Map<number, Record<string, unknown>>([
						[2, { editingNodeUid: { uid: "block-1" } }],
					]),
			},
		}
		await mountCoreMenu(
			editor,
			block("paragraph", { uid: "block-1" }),
			provider,
		)

		menuItem(t(DELETE)).click()
		await nextTick()

		expect(deletedRanges).toEqual([])
		expect(dispatch).toHaveBeenCalledTimes(0)
		expect(toast.custom).toHaveBeenCalledTimes(1)
	})

	it("keeps the last hovered block's actions while the menu closes", async ({
		expect,
	}) => {
		const { editor } = makeMenuEditor()
		const hovered = shallowRef<HoveredBlock | null>(block("splitDocumentation"))
		await mountMenuOptions(CoreMenu, () => ({
			editor: editor,
			hovered: hovered.value,
			dataSyncProvider: null,
		}))

		hovered.value = null
		await nextTick()

		expect(menuLabels()).toContain(
			t("editor.drag-handle.options.split-doc.invert-sides"),
		)
	})

	it("shows only the block-agnostic actions before anything is hovered", async ({
		expect,
	}) => {
		const { editor } = makeMenuEditor()

		await mountCoreMenu(editor, null)

		expect(menuLabels()).toEqual([t(DUPLICATE), t(ADD_COMMENT), t(DELETE)])
	})
})
