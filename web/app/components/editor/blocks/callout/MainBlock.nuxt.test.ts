import { beforeEach, describe, it, vi } from "vitest"
import MainBlock from "./MainBlock.vue"
import IconPicker from "../../IconPicker.vue"
import { makeNode, mountNodeView } from "../../test-helpers/node-view"
import { emitFrom, renderedIconNames } from "~/components/test-helpers"

function mountCallout(overrides: Record<string, unknown> = {}) {
	return mountNodeView(MainBlock, {
		node: makeNode({ uid: "callout-1", icon: "lucide:info" }),
		...overrides,
	})
}

// the icon-picker button carries the diff highlight when the icon changed
function iconPickerHighlighted(wrapper: {
	get: (selector: string) => { classes: () => string[] }
}): boolean {
	return wrapper.get("button").classes().includes("bg-diff-added")
}

// the editor store is app-wide, so these tests cannot interleave
describe("<CalloutMainBlock>", { concurrent: false }, () => {
	beforeEach(() => {
		useEditorStore().setReviewableDiffActive(false)
	})

	it("identifies the wrapper by the node's uid", async ({ expect }) => {
		const wrapper = await mountCallout()

		const root = wrapper.get("[data-node-view-wrapper]")

		expect(root.attributes("id")).toBe("callout-1")
		expect(root.attributes("data-uid")).toBe("callout-1")
	})

	it("exposes the node's comment id and diff status on the wrapper", async ({
		expect,
	}) => {
		const wrapper = await mountCallout({
			node: makeNode({
				uid: "callout-1",
				icon: "lucide:info",
				nodeCommentId: "comment-1",
				diffStatus: "added",
			}),
		})

		const root = wrapper.get("[data-node-view-wrapper]")

		expect(root.attributes("data-node-comment-id")).toBe("comment-1")
		expect(root.attributes("data-diff-status")).toBe("added")
	})

	it("shows the icon the node carries", async ({ expect }) => {
		const wrapper = await mountCallout()

		expect(renderedIconNames(wrapper)).toEqual(["lucide:info"])
	})

	it("stores a newly picked icon on the node", async ({ expect }) => {
		const updateAttributes = vi.fn()
		const wrapper = await mountCallout({ updateAttributes: updateAttributes })

		emitFrom(wrapper, IconPicker, "select", "lucide:triangle-alert")

		expect(updateAttributes).toHaveBeenCalledTimes(1)
		expect(updateAttributes).toHaveBeenCalledWith({
			icon: "lucide:triangle-alert",
		})
	})

	it("leaves the icon unhighlighted outside a reviewable diff", async ({
		expect,
	}) => {
		const wrapper = await mountCallout({
			node: makeNode({
				uid: "callout-1",
				icon: "lucide:info",
				oldNode: { attrs: { icon: "lucide:star" } },
			}),
		})

		expect(iconPickerHighlighted(wrapper)).toBe(false)
	})

	it("highlights the icon when the diff changed it", async ({ expect }) => {
		useEditorStore().setReviewableDiffActive(true)

		const wrapper = await mountCallout({
			node: makeNode({
				uid: "callout-1",
				icon: "lucide:info",
				oldNode: { attrs: { icon: "lucide:star" } },
			}),
		})

		expect(iconPickerHighlighted(wrapper)).toBe(true)
	})

	it("reads the previous icon out of a serialized old node", async ({
		expect,
	}) => {
		useEditorStore().setReviewableDiffActive(true)

		const wrapper = await mountCallout({
			node: makeNode({
				uid: "callout-1",
				icon: "lucide:info",
				oldNode: JSON.stringify({ attrs: { icon: "lucide:star" } }),
			}),
		})

		expect(iconPickerHighlighted(wrapper)).toBe(true)
	})

	it("leaves the icon unhighlighted when the diff kept it", async ({
		expect,
	}) => {
		useEditorStore().setReviewableDiffActive(true)

		const wrapper = await mountCallout({
			node: makeNode({
				uid: "callout-1",
				icon: "lucide:info",
				oldNode: { attrs: { icon: "lucide:info" } },
			}),
		})

		expect(iconPickerHighlighted(wrapper)).toBe(false)
	})

	it("leaves the icon unhighlighted when the node has no previous version", async ({
		expect,
	}) => {
		useEditorStore().setReviewableDiffActive(true)

		const wrapper = await mountCallout({
			node: makeNode({
				uid: "callout-1",
				icon: "lucide:info",
				oldNode: null,
			}),
		})

		expect(iconPickerHighlighted(wrapper)).toBe(false)
	})
})
