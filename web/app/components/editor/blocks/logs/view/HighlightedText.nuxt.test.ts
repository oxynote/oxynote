import { mountSuspended } from "@nuxt/test-utils/runtime"
import { describe, it } from "vitest"
import HighlightedText from "./HighlightedText.vue"

describe("<HighlightedText>", () => {
	it("marks every match of the search in any case", async ({ expect }) => {
		const wrapper = await mountInParagraph("Timeout after timeout", "TIMEOUT")

		expect(wrapper.text()).toBe("Timeout after timeout")
		expect(wrapper.findAll("mark").map((m) => m.text())).toEqual([
			"Timeout",
			"timeout",
		])
	})

	it("marks nothing without a search", async ({ expect }) => {
		const wrapper = await mountInParagraph("Timeout after timeout", "")

		expect(wrapper.text()).toBe("Timeout after timeout")
		expect(wrapper.findAll("mark")).toHaveLength(0)
	})
})

// the component renders bare text and marks without a root element, and a
// wrapper reads such a fragment's text node by node, trimming each. A
// host element reads it as one text.
function mountInParagraph(text: string, search: string) {
	return mountSuspended({
		render: () => h("p", [h(HighlightedText, { text, search })]),
	})
}
