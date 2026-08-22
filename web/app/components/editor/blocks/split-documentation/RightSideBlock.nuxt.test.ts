import type { VueWrapper } from "@vue/test-utils"
import { beforeEach, describe, it } from "vitest"
import RightSideBlock from "./RightSideBlock.vue"
import {
	commandArgs,
	commandNames,
	makeEditor,
	makeNode,
	mountNodeViewUnderTooltipProvider,
} from "../../test-helpers/node-view"
import { at, findButtonByText, t } from "~/components/test-helpers"
import { SUPPRESS_SCROLL_TO_SELECTION_META } from "../../scroll-control"

const ADD_CODE_BUTTON =
	"editor.split-documentation.right-side-bottom-action-buttons.add-code"
const ADD_METRICS_BUTTON =
	"editor.split-documentation.right-side-bottom-action-buttons.add-metrics"

function mountRightSide(overrides: Record<string, unknown> = {}) {
	return mountNodeViewUnderTooltipProvider(RightSideBlock, {
		node: makeNode({ uid: "right-1" }),
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
describe("<SplitDocumentationRightSideBlock>", { concurrent: false }, () => {
	beforeEach(() => {
		useEditorMeta().setEditable(true)
		useEditorMeta().updateLock(false)
		useEditorStore().setReviewableDiffActive(false)
	})

	it("identifies the wrapper by the node's uid", async ({ expect }) => {
		const wrapper = await mountRightSide()

		const root = wrapper.get("[data-node-view-wrapper]")

		expect(root.attributes("id")).toBe("right-1")
		expect(root.attributes("data-uid")).toBe("right-1")
	})

	it("exposes the node's comment id and diff status on the wrapper", async ({
		expect,
	}) => {
		const wrapper = await mountRightSide({
			node: makeNode({
				uid: "right-1",
				nodeCommentId: "comment-2",
				diffStatus: "removed",
			}),
		})

		const root = wrapper.get("[data-node-view-wrapper]")

		expect(root.attributes("data-node-comment-id")).toBe("comment-2")
		expect(root.attributes("data-diff-status")).toBe("removed")
	})

	it("offers both add actions while editing is possible", async ({
		expect,
	}) => {
		const wrapper = await mountRightSide()

		expect(wrapper.text()).toContain(t(ADD_CODE_BUTTON))
		expect(wrapper.text()).toContain(t(ADD_METRICS_BUTTON))
		expect(actionsHidden(wrapper)).toBe(false)
	})

	it("hides the add actions in read mode", async ({ expect }) => {
		useEditorMeta().setEditable(false)

		const wrapper = await mountRightSide()

		expect(actionsHidden(wrapper)).toBe(true)
	})

	it("hides the add actions while the editor is locked", async ({ expect }) => {
		useEditorMeta().updateLock(true)

		const wrapper = await mountRightSide()

		expect(actionsHidden(wrapper)).toBe(true)
	})

	it("hides the add actions while a reviewable diff is shown", async ({
		expect,
	}) => {
		useEditorStore().setReviewableDiffActive(true)

		const wrapper = await mountRightSide()

		expect(actionsHidden(wrapper)).toBe(true)
	})

	it("appends a code block at the node's position", async ({ expect }) => {
		const { editor, commands } = makeEditor()
		const wrapper = await mountRightSide({ editor: editor, getPos: () => 4 })

		await findButtonByText(wrapper, t(ADD_CODE_BUTTON)).trigger("click")

		expect(commandNames(commands)).toEqual([
			"focus",
			"appendBlockOnRightSide",
			"run",
		])
		expect(commandArgs(commands, "appendBlockOnRightSide")).toEqual([4, "code"])
	})

	it("appends a metrics block without scrolling the selection into view", async ({
		expect,
	}) => {
		const { editor, commands } = makeEditor()
		const wrapper = await mountRightSide({ editor: editor, getPos: () => 4 })

		await findButtonByText(wrapper, t(ADD_METRICS_BUTTON)).trigger("click")

		expect(commandNames(commands)).toEqual([
			"setMeta",
			"focus",
			"appendBlockOnRightSide",
			"run",
		])
		expect(commandArgs(commands, "setMeta")).toEqual([
			SUPPRESS_SCROLL_TO_SELECTION_META,
			true,
		])
		expect(commandArgs(commands, "appendBlockOnRightSide")).toEqual([
			4,
			"metrics",
		])
	})

	it("appends at the very start of the document", async ({ expect }) => {
		const { editor, commands } = makeEditor()
		const wrapper = await mountRightSide({ editor: editor, getPos: () => 0 })

		await findButtonByText(wrapper, t(ADD_CODE_BUTTON)).trigger("click")

		expect(commandArgs(commands, "appendBlockOnRightSide")).toEqual([0, "code"])
	})

	it("runs no command when the node has no resolvable position", async ({
		expect,
	}) => {
		const { editor, commands } = makeEditor()
		const wrapper = await mountRightSide({
			editor: editor,
			getPos: () => undefined,
		})

		await findButtonByText(wrapper, t(ADD_CODE_BUTTON)).trigger("click")

		expect(commands).toEqual([])
	})

	it("runs no command when the position is negative", async ({ expect }) => {
		const { editor, commands } = makeEditor()
		const wrapper = await mountRightSide({ editor: editor, getPos: () => -1 })

		await findButtonByText(wrapper, t(ADD_CODE_BUTTON)).trigger("click")

		expect(commands).toEqual([])
	})
})
