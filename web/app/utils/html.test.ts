import { describe, it } from "vitest"
import { waitForHtmlElementById } from "./html"

describe("waitForHtmlElementById", () => {
	// node has no document, so this exercises the SSR guard directly
	it("resolves null when no document exists", async ({ expect }) => {
		await expect(waitForHtmlElementById("any", 1000)).resolves.toBeNull()
	})
})
