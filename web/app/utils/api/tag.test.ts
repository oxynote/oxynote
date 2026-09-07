import { describe, it } from "vitest"
import { makeWsBranchTagsChangeTopic } from "./tag"

describe("makeWsBranchTagsChangeTopic", () => {
	it("builds the branch tags change topic for the document", ({ expect }) => {
		expect(makeWsBranchTagsChangeTopic("d1")).toBe("change@documents.d1.tags")
	})
})
