import { mountSuspended } from "@nuxt/test-utils/runtime"
import type { VueWrapper } from "@vue/test-utils"
import { afterEach, beforeEach, describe, it } from "vitest"
import CommentIndicatorContainer from "./CommentIndicatorContainer.vue"
import CommentIndicator from "./CommentIndicator.vue"
import { at } from "~/components/test-helpers"

let container: HTMLElement | null = null

function overlay(nodeCommentId: string, forcedHighlight = false) {
	return {
		nodeCommentId: nodeCommentId,
		top: 10,
		left: 20,
		forcedHighlight: forcedHighlight,
	}
}

function indicator(commentId: string, forcedHighlight = false) {
	return {
		commentId: commentId,
		top: 30,
		left: 40,
		forcedHighlight: forcedHighlight,
	}
}

function mountContainer(props: Record<string, unknown> = {}) {
	return mountSuspended(CommentIndicatorContainer, {
		props: { nodeCommentState: null, textCommentState: null, ...props },
	})
}

function indicators(wrapper: VueWrapper) {
	return wrapper.findAllComponents(CommentIndicator)
}

// the indicators are teleported into an element the editor owns, so each
// test needs one in the document to teleport into
describe("<CommentIndicatorContainer>", { concurrent: false }, () => {
	beforeEach(() => {
		container = document.createElement("div")
		document.body.appendChild(container)
	})

	afterEach(() => {
		container?.remove()
		container = null
	})

	it("renders nothing while there are no comments", async ({ expect }) => {
		const wrapper = await mountContainer()

		expect(indicators(wrapper)).toHaveLength(0)
	})

	it("renders nothing while the editor has no overlay container", async ({
		expect,
	}) => {
		const wrapper = await mountContainer({
			nodeCommentState: {
				container: null,
				overlays: [overlay("node-1")],
				hoveredNodeCommentId: null,
			},
		})

		expect(indicators(wrapper)).toHaveLength(0)
	})

	it("places one indicator per commented node", async ({ expect }) => {
		const wrapper = await mountContainer({
			nodeCommentState: {
				container: container,
				overlays: [overlay("node-1"), overlay("node-2")],
				hoveredNodeCommentId: null,
			},
		})

		expect(indicators(wrapper)).toHaveLength(2)
		expect(at(indicators(wrapper), 0).props("commentType")).toBe("node")
		expect(container?.querySelectorAll(".absolute")).toHaveLength(2)
	})

	it("places one indicator per commented text range", async ({ expect }) => {
		const wrapper = await mountContainer({
			textCommentState: {
				container: container,
				indicators: [indicator("text-1")],
				hoveredCommentId: null,
			},
		})

		expect(indicators(wrapper)).toHaveLength(1)
		expect(at(indicators(wrapper), 0).props("commentType")).toBe("text")
	})

	it("marks the node indicator the pointer is on", async ({ expect }) => {
		const wrapper = await mountContainer({
			nodeCommentState: {
				container: container,
				overlays: [overlay("node-1"), overlay("node-2")],
				hoveredNodeCommentId: "node-2",
			},
		})

		expect(at(indicators(wrapper), 0).props("hovered")).toBe(false)
		expect(at(indicators(wrapper), 1).props("hovered")).toBe(true)
	})

	it("marks the text indicator the pointer is on", async ({ expect }) => {
		const wrapper = await mountContainer({
			textCommentState: {
				container: container,
				indicators: [indicator("text-1"), indicator("text-2")],
				hoveredCommentId: "text-1",
			},
		})

		expect(at(indicators(wrapper), 0).props("hovered")).toBe(true)
		expect(at(indicators(wrapper), 1).props("hovered")).toBe(false)
	})

	it("passes a forced highlight through", async ({ expect }) => {
		const wrapper = await mountContainer({
			nodeCommentState: {
				container: container,
				overlays: [overlay("node-1", true)],
				hoveredNodeCommentId: null,
			},
		})

		expect(at(indicators(wrapper), 0).props("forcedHighlight")).toBe(true)
	})

	it("asks to open the node comment that was clicked", async ({ expect }) => {
		const wrapper = await mountContainer({
			nodeCommentState: {
				container: container,
				overlays: [overlay("node-1")],
				hoveredNodeCommentId: null,
			},
		})

		await at(indicators(wrapper), 0).trigger("click")

		expect(wrapper.emitted("open-comment")).toEqual([["node", "node-1"]])
	})

	it("asks to open the text comment that was clicked", async ({ expect }) => {
		const wrapper = await mountContainer({
			textCommentState: {
				container: container,
				indicators: [indicator("text-1")],
				hoveredCommentId: null,
			},
		})

		await at(indicators(wrapper), 0).trigger("click")

		expect(wrapper.emitted("open-comment")).toEqual([["text", "text-1"]])
	})

	it("reports the pointer entering and leaving a node indicator", async ({
		expect,
	}) => {
		const wrapper = await mountContainer({
			nodeCommentState: {
				container: container,
				overlays: [overlay("node-1")],
				hoveredNodeCommentId: null,
			},
		})

		await at(indicators(wrapper), 0).trigger("mouseenter")
		await at(indicators(wrapper), 0).trigger("mouseleave")

		expect(wrapper.emitted("comment-hover-change")).toEqual([
			["node", "node-1", true],
			["node", "node-1", false],
		])
	})

	it("reports the pointer entering and leaving a text indicator", async ({
		expect,
	}) => {
		const wrapper = await mountContainer({
			textCommentState: {
				container: container,
				indicators: [indicator("text-1")],
				hoveredCommentId: null,
			},
		})

		await at(indicators(wrapper), 0).trigger("mouseenter")
		await at(indicators(wrapper), 0).trigger("mouseleave")

		expect(wrapper.emitted("comment-hover-change")).toEqual([
			["text", "text-1", true],
			["text", "text-1", false],
		])
	})
})
