import { describe, it } from "vitest"
import { renderCollaborationCaret } from "./collaboration"

describe("renderCollaborationCaret", () => {
	it("renders a caret in the user color labeled with the user name", ({
		expect,
	}) => {
		const caret = renderCollaborationCaret({
			name: "Ada",
			color: "rgb(10, 20, 30)",
		})

		expect(caret.tagName).toBe("SPAN")
		expect(caret.style.borderColor).toBe("rgb(10, 20, 30)")

		const label = caret.querySelector("div")
		expect(label?.textContent).toBe("Ada")
		expect(label?.style.backgroundColor).toBe("rgb(10, 20, 30)")
	})
})
