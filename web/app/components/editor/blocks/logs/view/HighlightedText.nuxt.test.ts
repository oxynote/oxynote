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

	it("colours the keys and equals signs of a logfmt line", async ({
		expect,
	}) => {
		const wrapper = await mountInParagraph(
			'level=error msg="value out of range" value=+Inf',
			"",
			true,
		)

		expect(wrapper.text()).toBe(
			'level=error msg="value out of range" value=+Inf',
		)
		expect(
			wrapper.findAll(".text-muted-foreground").map((s) => s.text()),
		).toEqual(["level", "msg", "value"])
		expect(
			wrapper.findAll(".text-muted-foreground\\/70").map((s) => s.text()),
		).toEqual(["=", "=", "="])
	})

	it("marks the search inside a coloured key", async ({ expect }) => {
		const wrapper = await mountInParagraph("level=error", "lev", true)

		expect(wrapper.findAll("mark").map((m) => m.text())).toEqual(["lev"])
		expect(
			wrapper.findAll(".text-muted-foreground").map((s) => s.text()),
		).toEqual(["lev", "el"])
	})

	it("marks a search spanning a key, its equals sign and its value", async ({
		expect,
	}) => {
		const wrapper = await mountInParagraph("user=ada", "user=ada", true)

		expect(wrapper.text()).toBe("user=ada")
		expect(wrapper.findAll("mark").map((m) => m.text())).toEqual([
			"user",
			"=",
			"ada",
		])
	})

	it("colours nothing without the key colouring", async ({ expect }) => {
		const wrapper = await mountInParagraph("level=error", "")

		expect(wrapper.findAll("span")).toHaveLength(0)
	})
})

// the component renders bare text and marks without a root element, and a
// wrapper reads such a fragment's text node by node, trimming each. A
// host element reads it as one text.
function mountInParagraph(text: string, search: string, colourKeys = false) {
	return mountSuspended({
		render: () => h("p", [h(HighlightedText, { text, search, colourKeys })]),
	})
}
