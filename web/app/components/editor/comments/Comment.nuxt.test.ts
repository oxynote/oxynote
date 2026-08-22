import { mountSuspended } from "@nuxt/test-utils/runtime"
import { describe, it, vi } from "vitest"
import Comment from "./Comment.vue"
import { WAIT_FOR_OPTIONS } from "~/components/test-helpers"

function paragraphDoc(text: string) {
	return {
		type: "doc",
		content: [{ type: "paragraph", content: [{ type: "text", text: text }] }],
	}
}

function mountComment(content: Record<string, unknown>) {
	return mountSuspended(Comment, { props: { content: content } })
}

// the editor is created in onMounted, so every assertion waits for the
// first render it produces
describe("<Comment>", () => {
	it("renders the comment's text", async ({ expect }) => {
		const wrapper = await mountComment(paragraphDoc("Looks good to me"))

		await vi.waitFor(() => {
			expect(wrapper.text()).toContain("Looks good to me")
		}, WAIT_FOR_OPTIONS)
	})

	it("renders the comment read-only", async ({ expect }) => {
		const wrapper = await mountComment(paragraphDoc("Looks good to me"))

		await vi.waitFor(() => {
			expect(wrapper.find(".ProseMirror").exists()).toBe(true)
		}, WAIT_FOR_OPTIONS)
		expect(wrapper.get(".ProseMirror").attributes("contenteditable")).toBe(
			"false",
		)
	})

	it("turns spellchecking off", async ({ expect }) => {
		const wrapper = await mountComment(paragraphDoc("Teh comment"))

		await vi.waitFor(() => {
			expect(wrapper.find(".ProseMirror").exists()).toBe(true)
		}, WAIT_FOR_OPTIONS)
		expect(wrapper.get(".ProseMirror").attributes("spellcheck")).toBe("false")
	})

	it("renders an empty comment as an empty paragraph", async ({ expect }) => {
		const wrapper = await mountComment({
			type: "doc",
			content: [{ type: "paragraph" }],
		})

		await vi.waitFor(() => {
			expect(wrapper.find(".ProseMirror").exists()).toBe(true)
		}, WAIT_FOR_OPTIONS)
		expect(wrapper.text()).toBe("")
	})

	it("re-renders when it is handed another comment", async ({ expect }) => {
		const wrapper = await mountComment(paragraphDoc("First"))
		await vi.waitFor(() => {
			expect(wrapper.text()).toContain("First")
		}, WAIT_FOR_OPTIONS)

		await wrapper.setProps({ content: paragraphDoc("Second") })

		await vi.waitFor(() => {
			expect(wrapper.text()).toContain("Second")
		}, WAIT_FOR_OPTIONS)
		expect(wrapper.text()).not.toContain("First")
	})

	it("keeps the formatting a comment carries", async ({ expect }) => {
		const wrapper = await mountComment({
			type: "doc",
			content: [
				{
					type: "paragraph",
					content: [{ type: "text", text: "bold", marks: [{ type: "bold" }] }],
				},
			],
		})

		await vi.waitFor(() => {
			expect(wrapper.find("strong").exists()).toBe(true)
		}, WAIT_FOR_OPTIONS)
		expect(wrapper.get("strong").text()).toBe("bold")
	})
})
