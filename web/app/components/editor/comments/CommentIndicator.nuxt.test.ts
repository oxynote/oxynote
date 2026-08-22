import { mountSuspended } from "@nuxt/test-utils/runtime"
import type { VueWrapper } from "@vue/test-utils"
import { describe, it } from "vitest"
import CommentIndicator from "./CommentIndicator.vue"
import { renderedIconNames } from "~/components/test-helpers"

function mountIndicator(props: Record<string, unknown> = {}) {
	return mountSuspended(CommentIndicator, {
		props: {
			commentType: "text",
			top: 100,
			left: 50,
			forcedHighlight: false,
			hovered: false,
			...props,
		},
	})
}

function position(wrapper: VueWrapper): string {
	return wrapper.get("div").attributes("style") ?? ""
}

describe("<CommentIndicator>", () => {
	it("shows a comment bubble", async ({ expect }) => {
		const wrapper = await mountIndicator()

		expect(renderedIconNames(wrapper)).toEqual(["mingcute:message-4-fill"])
	})

	it("sits just above and left of a text comment", async ({ expect }) => {
		const wrapper = await mountIndicator()

		expect(position(wrapper)).toContain("top: 88px")
		expect(position(wrapper)).toContain("left: 44px")
	})

	it("sits closer to a node comment", async ({ expect }) => {
		const wrapper = await mountIndicator({ commentType: "node" })

		expect(position(wrapper)).toContain("top: 92px")
		expect(position(wrapper)).toContain("left: 44px")
	})

	it("lifts a hovered text comment further up", async ({ expect }) => {
		const wrapper = await mountIndicator({ hovered: true })

		expect(position(wrapper)).toContain("top: 84px")
	})

	it("lifts a hovered node comment further up", async ({ expect }) => {
		const wrapper = await mountIndicator({
			commentType: "node",
			hovered: true,
		})

		expect(position(wrapper)).toContain("top: 86px")
	})

	it("lifts a forcibly highlighted comment too", async ({ expect }) => {
		const wrapper = await mountIndicator({ forcedHighlight: true })

		expect(position(wrapper)).toContain("top: 84px")
	})

	it("stays faded while nothing points at it", async ({ expect }) => {
		const wrapper = await mountIndicator()

		expect(wrapper.get("div").attributes("data-hovered")).toBeUndefined()
	})

	it("becomes solid once the pointer is on it", async ({ expect }) => {
		const wrapper = await mountIndicator({ hovered: true })

		expect(wrapper.get("div").attributes("data-hovered")).toBe("")
	})

	it("becomes solid while it is forcibly highlighted", async ({ expect }) => {
		const wrapper = await mountIndicator({ forcedHighlight: true })

		expect(wrapper.get("div").attributes("data-hovered")).toBe("")
	})

	it("animates its moves while the page is still", async ({ expect }) => {
		const wrapper = await mountIndicator()

		expect(wrapper.get("div").classes()).toContain(
			"transition-[top,left,opacity]",
		)
	})
})
