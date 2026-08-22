import type { VueWrapper } from "@vue/test-utils"
import { beforeEach, describe, it } from "vitest"
import LeftSideBlock from "./LeftSideBlock.vue"
import {
	commandArgs,
	commandNames,
	makeEditor,
	makeNode,
	mountNodeViewUnderTooltipProvider,
} from "../../test-helpers/node-view"
import { at, findButtonByText, t } from "~/components/test-helpers"

const ADD_BUTTON = "editor.split-documentation.left-side-bottom-action-button"

function mountLeftSide(overrides: Record<string, unknown> = {}) {
	return mountNodeViewUnderTooltipProvider(LeftSideBlock, {
		node: makeNode({ uid: "left-1" }),
		...overrides,
	})
}

// the bottom action row is the wrapper's second child, after the node
// content, and is hidden with v-show — which leaves it in the dom with
// an inline display:none
function actionsHidden(wrapper: VueWrapper): boolean {
	const row = at(wrapper.findAll("[data-node-view-wrapper] > div"), 1)

	return (row.element as HTMLElement).style.display === "none"
}

// the editable flag is a shared cookie state and the editor store is
// app-wide, so these tests cannot interleave
describe("<SplitDocumentationLeftSideBlock>", { concurrent: false }, () => {
	beforeEach(() => {
		useEditorMeta().setEditable(true)
		useEditorMeta().updateLock(false)
		useEditorStore().setReviewableDiffActive(false)
	})

	it("identifies the wrapper by the node's uid", async ({ expect }) => {
		const wrapper = await mountLeftSide()

		const root = wrapper.get("[data-node-view-wrapper]")

		expect(root.attributes("id")).toBe("left-1")
		expect(root.attributes("data-uid")).toBe("left-1")
	})

	it("exposes the node's comment id and diff status on the wrapper", async ({
		expect,
	}) => {
		const wrapper = await mountLeftSide({
			node: makeNode({
				uid: "left-1",
				nodeCommentId: "comment-1",
				diffStatus: "modified",
			}),
		})

		const root = wrapper.get("[data-node-view-wrapper]")

		expect(root.attributes("data-node-comment-id")).toBe("comment-1")
		expect(root.attributes("data-diff-status")).toBe("modified")
	})

	it("offers the add-parameters action while editing is possible", async ({
		expect,
	}) => {
		const wrapper = await mountLeftSide()

		expect(wrapper.text()).toContain(t(ADD_BUTTON))
		expect(actionsHidden(wrapper)).toBe(false)
	})

	it("hides the add-parameters action in read mode", async ({ expect }) => {
		useEditorMeta().setEditable(false)

		const wrapper = await mountLeftSide()

		expect(actionsHidden(wrapper)).toBe(true)
	})

	it("hides the add-parameters action while the editor is locked", async ({
		expect,
	}) => {
		useEditorMeta().updateLock(true)

		const wrapper = await mountLeftSide()

		expect(actionsHidden(wrapper)).toBe(true)
	})

	it("hides the add-parameters action while a reviewable diff is shown", async ({
		expect,
	}) => {
		useEditorStore().setReviewableDiffActive(true)

		const wrapper = await mountLeftSide()

		expect(actionsHidden(wrapper)).toBe(true)
	})

	it("appends a parameter list at the node's position", async ({ expect }) => {
		const { editor, commands } = makeEditor()
		const wrapper = await mountLeftSide({ editor: editor, getPos: () => 7 })

		await findButtonByText(wrapper, t(ADD_BUTTON)).trigger("click")

		expect(commandNames(commands)).toEqual([
			"focus",
			"appendParameterListOnLeftSide",
			"run",
		])
		expect(commandArgs(commands, "appendParameterListOnLeftSide")).toEqual([7])
	})

	it("runs no command when the node has no resolvable position", async ({
		expect,
	}) => {
		const { editor, commands } = makeEditor()
		const wrapper = await mountLeftSide({
			editor: editor,
			getPos: () => undefined,
		})

		await findButtonByText(wrapper, t(ADD_BUTTON)).trigger("click")

		expect(commands).toEqual([])
	})
})
