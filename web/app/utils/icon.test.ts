import { describe, it } from "vitest"
import { selectableIconList, selectableIcons } from "./icon"

describe("selectableIconList", () => {
	it("returns the id of every selectable icon in order", ({ expect }) => {
		const list = selectableIconList()

		expect(list).toEqual(selectableIcons.map((v) => v.id))
		expect(list.length).toBeGreaterThan(0)
	})

	it("returns only prefixed icon ids", ({ expect }) => {
		for (const id of selectableIconList()) {
			expect(id).toMatch(/^mingcute:/)
		}
	})
})
