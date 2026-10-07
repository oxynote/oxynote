import { getSchema } from "@tiptap/core"
import Document from "@tiptap/extension-document"
import Paragraph from "@tiptap/extension-paragraph"
import Text from "@tiptap/extension-text"
import { describe, it } from "vitest"
import { markType } from "../test-helpers"
import { LinkMark } from "./link-mark"

const link = markType(getSchema([Document, Paragraph, Text, LinkMark]), "link")

describe("LinkMark", () => {
	it("does not extend over text typed at its end", ({ expect }) => {
		expect(link.spec.inclusive).toBe(false)
	})

	it("renders an anchor in the link colour", ({ expect }) => {
		const rendered = render({ href: "https://example.com" })

		expect(rendered.tag).toBe("a")
		expect(rendered.attributes.href).toBe("https://example.com")
		expect(rendered.attributes.class.split(" ")).toEqual(
			expect.arrayContaining(["text-link", "underline", "cursor-pointer"]),
		)
	})

	it("keeps the classes a saved link carries without rendering them", ({
		expect,
	}) => {
		const rendered = render({
			href: "https://example.com",
			class: "text-primary saved-class",
		})

		expect(rendered.mark.attrs.class).toBe("text-primary saved-class")
		expect(rendered.attributes.class).toBe(
			render({ href: "" }).attributes.class,
		)
	})
})

function render(attrs: Record<string, string>) {
	const mark = link.create(attrs)
	const [tag, attributes] = link.spec.toDOM?.(mark, false) as [
		string,
		{ href: string; class: string },
	]

	return { mark: mark, tag: tag, attributes: attributes }
}
