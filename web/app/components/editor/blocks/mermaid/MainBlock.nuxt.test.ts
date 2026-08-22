import type { VueWrapper } from "@vue/test-utils"
import { beforeEach, describe, it, vi } from "vitest"
import MainBlock from "./MainBlock.vue"
import MermaidPreview from "./MermaidPreview.vue"
import {
	makeEditor,
	makeNode,
	mountNodeView,
} from "../../test-helpers/node-view"
import { MERMAID_BLOCK_NAME } from "../node-names"
import { DiffStatus } from "../../diff/position-map"
import { t } from "~/components/test-helpers"

vi.mock("./useMermaid", async () => {
	const { ref } = await import("vue")

	return {
		useMermaid: () => ({
			render: vi.fn().mockResolvedValue({ svg: "<svg></svg>" }),
			isLoading: ref(false),
			loadError: ref<string | null>(null),
		}),
	}
})

let uidCounter = 0

// every block keeps its open/closed state in the app-wide editor store,
// keyed by uid — a fresh uid per test keeps them from sharing it
function nextUid(): string {
	uidCounter++

	return `mermaid-${uidCounter}`
}

const refreshTextCommentIndicators = vi.fn()

// the awareness the collaboration composable reads: the composable takes
// anything that is not a real Editor instance as the provider itself, so
// the editor stand-in carries the awareness directly
function makeCollaborationEditor(
	editing: { uid: string; name: string; color: string } | null,
) {
	const states = new Map<number, Record<string, unknown>>([[1, {}]])

	if (editing) {
		states.set(2, {
			editingNodeUid: { uid: editing.uid },
			user: { name: editing.name, color: editing.color },
		})
	}

	return makeEditor({
		commands: { refreshTextCommentIndicators: refreshTextCommentIndicators },
		awareness: {
			clientID: 1,
			getStates: () => states,
			on: vi.fn(),
			off: vi.fn(),
		},
	})
}

function mountMermaid(
	options: {
		uid?: string
		attrs?: Record<string, unknown>
		textContent?: string
		editing?: { uid: string; name: string; color: string } | null
	} = {},
) {
	const uid = options.uid ?? nextUid()

	return mountNodeView(MainBlock, {
		node: makeNode(
			{ uid: uid, ...options.attrs },
			{
				typeName: MERMAID_BLOCK_NAME,
				textContent: options.textContent ?? "",
			},
		),
		editor: makeCollaborationEditor(options.editing ?? null).editor,
	})
}

function toggleButton(wrapper: VueWrapper) {
	return wrapper.get("button")
}

// the code pane sits in the dom either way and is hidden with v-show
function codeVisible(wrapper: VueWrapper): boolean {
	const pane = wrapper.get(".overflow-x-auto").element as HTMLElement

	return pane.style.display !== "none"
}

// the editable flag is a shared cookie state and the editor store is
// app-wide, so these tests cannot interleave
describe("<MermaidMainBlock>", { concurrent: false }, () => {
	beforeEach(() => {
		refreshTextCommentIndicators.mockReset()
		useEditorMeta().setEditable(true)
		useEditorStore().setReviewableDiffActive(false)
	})

	it("identifies the wrapper by the node's uid", async ({ expect }) => {
		const uid = nextUid()

		const wrapper = await mountMermaid({ uid: uid })

		const root = wrapper.get("[data-node-view-wrapper]")

		expect(root.attributes("id")).toBe(uid)
		expect(root.attributes("data-uid")).toBe(uid)
	})

	it("exposes the node's comment id and diff status on the wrapper", async ({
		expect,
	}) => {
		const wrapper = await mountMermaid({
			attrs: { nodeCommentId: "comment-1", diffStatus: DiffStatus.Added },
		})

		const root = wrapper.get("[data-node-view-wrapper]")

		expect(root.attributes("data-node-comment-id")).toBe("comment-1")
		expect(root.attributes("data-diff-status")).toBe("added")
	})

	it("opens the code pane for a block with no diagram yet", async ({
		expect,
	}) => {
		const wrapper = await mountMermaid()

		expect(codeVisible(wrapper)).toBe(true)
	})

	it("keeps the code pane closed for a block that has a diagram", async ({
		expect,
	}) => {
		const wrapper = await mountMermaid({ textContent: "graph TD; A-->B;" })

		expect(codeVisible(wrapper)).toBe(false)
	})

	it("leaves an already tracked block's code pane as the reader left it", async ({
		expect,
	}) => {
		const uid = nextUid()
		useEditorStore().setMermaidBlockShowCode(uid, false)

		const wrapper = await mountMermaid({ uid: uid })

		expect(codeVisible(wrapper)).toBe(false)
	})

	it("closes the code pane when the reader hides it", async ({ expect }) => {
		const wrapper = await mountMermaid()

		await toggleButton(wrapper).trigger("click")

		expect(codeVisible(wrapper)).toBe(false)
		expect(toggleButton(wrapper).attributes("aria-pressed")).toBe("false")
	})

	it("reopens the code pane when the reader shows it again", async ({
		expect,
	}) => {
		const wrapper = await mountMermaid({ textContent: "graph TD; A-->B;" })

		await toggleButton(wrapper).trigger("click")

		expect(codeVisible(wrapper)).toBe(true)
		expect(toggleButton(wrapper).attributes("aria-pressed")).toBe("true")
	})

	it("remembers the code pane state across blocks with the same uid", async ({
		expect,
	}) => {
		const uid = nextUid()
		const wrapper = await mountMermaid({ uid: uid })
		await toggleButton(wrapper).trigger("click")

		const other = await mountMermaid({ uid: uid })

		expect(codeVisible(other)).toBe(false)
	})

	it("refreshes the comment indicators after the pane is toggled", async ({
		expect,
	}) => {
		const wrapper = await mountMermaid()

		await toggleButton(wrapper).trigger("click")
		await nextTick()

		expect(refreshTextCommentIndicators).toHaveBeenCalledTimes(1)
	})

	it("prompts for code while the block is empty and editable", async ({
		expect,
	}) => {
		const wrapper = await mountMermaid()

		expect(wrapper.text()).toContain(
			t("editor.placeholders.content.mermaid.content"),
		)
	})

	it("marks an empty block as unfilled in read mode", async ({ expect }) => {
		useEditorMeta().setEditable(false)

		const wrapper = await mountMermaid()

		expect(wrapper.text()).toContain(
			t("editor.placeholders.content.mermaid.content-empty"),
		)
	})

	it("shows no placeholder once the block has code", async ({ expect }) => {
		const wrapper = await mountMermaid({ textContent: "graph TD; A-->B;" })

		expect(wrapper.text()).not.toContain(
			t("editor.placeholders.content.mermaid.content"),
		)
	})

	it("previews the code the block holds", async ({ expect }) => {
		const wrapper = await mountMermaid({ textContent: "graph TD; A-->B;" })

		expect(wrapper.findComponent(MermaidPreview).props()).toEqual(
			expect.objectContaining({ source: "graph TD; A-->B;" }),
		)
	})

	it("previews only the modified side of a diffed block", async ({
		expect,
	}) => {
		const wrapper = await mountMermaid({
			textContent: "graph TD; A-->B;graph TD; A-->C;",
			attrs: { modifiedTextContent: "graph TD; A-->C;" },
		})

		expect(wrapper.findComponent(MermaidPreview).props()).toEqual(
			expect.objectContaining({ source: "graph TD; A-->C;" }),
		)
	})

	it.for([
		{ status: DiffStatus.Added, expected: "diff-added" },
		{ status: DiffStatus.Removed, expected: "diff-removed" },
	])(
		"marks a $status block with its diff colour whatever the pane shows",
		async ({ status, expected }, { expect }) => {
			const wrapper = await mountMermaid({ attrs: { diffStatus: status } })

			expect(wrapper.get("[data-node-view-wrapper]").classes()).toContain(
				expected,
			)
		},
	)

	it("marks a modified block only while its code is hidden", async ({
		expect,
	}) => {
		const wrapper = await mountMermaid({
			textContent: "graph TD; A-->B;",
			attrs: { diffStatus: DiffStatus.Modified },
		})

		expect(wrapper.get("[data-node-view-wrapper]").classes()).toContain(
			"diff-modified",
		)
	})

	it("leaves a modified block's colour to the inline diff while its code shows", async ({
		expect,
	}) => {
		const wrapper = await mountMermaid({
			attrs: { diffStatus: DiffStatus.Modified },
		})

		expect(wrapper.get("[data-node-view-wrapper]").classes()).not.toContain(
			"diff-modified",
		)
	})

	it("shows no diff colour on an unchanged block", async ({ expect }) => {
		const wrapper = await mountMermaid({
			attrs: { diffStatus: DiffStatus.Unchanged },
		})

		expect(wrapper.get("[data-node-view-wrapper]").classes()).not.toContain(
			"diff-modified",
		)
	})

	it("names the collaborator editing the block", async ({ expect }) => {
		const uid = nextUid()

		const wrapper = await mountMermaid({
			uid: uid,
			textContent: "graph TD; A-->B;",
			editing: { uid: uid, name: "Ada", color: "#ff0000" },
		})

		expect(wrapper.text()).toContain("Ada")
		expect(
			wrapper.get("[data-node-view-wrapper]").attributes("style"),
		).toContain("border-color: #ff0000")
	})

	it("names nobody while the block's code pane is open", async ({ expect }) => {
		const uid = nextUid()

		const wrapper = await mountMermaid({
			uid: uid,
			editing: { uid: uid, name: "Ada", color: "#ff0000" },
		})

		expect(wrapper.text()).not.toContain("Ada")
	})

	it("names nobody in read mode", async ({ expect }) => {
		useEditorMeta().setEditable(false)
		const uid = nextUid()

		const wrapper = await mountMermaid({
			uid: uid,
			textContent: "graph TD; A-->B;",
			editing: { uid: uid, name: "Ada", color: "#ff0000" },
		})

		expect(wrapper.text()).not.toContain("Ada")
	})

	it("names nobody when the collaborator is editing another block", async ({
		expect,
	}) => {
		const wrapper = await mountMermaid({
			textContent: "graph TD; A-->B;",
			editing: { uid: "elsewhere", name: "Ada", color: "#ff0000" },
		})

		expect(wrapper.text()).not.toContain("Ada")
	})
})
