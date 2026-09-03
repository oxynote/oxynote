import { describe, it } from "vitest"
import { mountSuspended } from "@nuxt/test-utils/runtime"
import TagPill from "./TagPill.vue"

function mountPill(name: string, color?: string) {
	return mountSuspended(TagPill, { props: { name: name, color: color } })
}

// the two modes are pure css, which happy-dom loads none of, so the custom
// properties the variants read are the only trace a test can follow
describe("<TagPill>", () => {
	it("shows the tag's name", async ({ expect }) => {
		const wrapper = await mountPill("Production", "#1a9e4a")

		expect(wrapper.text()).toBe("Production")
	})

	it("carries a light and a dark treatment of its colour", async ({
		expect,
	}) => {
		const wrapper = await mountPill("Production", "#1a9e4a")
		const style = wrapper.get("span").attributes("style") ?? ""

		expect(style).toContain(
			"--tag-bg: color-mix(in srgb, #1a9e4a 13%, transparent)",
		)
		expect(style).toContain("--tag-fg: color-mix(in srgb, #1a9e4a 80%, black)")
		expect(style).toContain(
			"--tag-dark-bg: color-mix(in srgb, #1a9e4a 18%, transparent)",
		)
		expect(style).toContain(
			"--tag-dark-border: color-mix(in srgb, #1a9e4a 40%, transparent)",
		)
		expect(style).toContain(
			"--tag-dark-fg: color-mix(in srgb, #1a9e4a 60%, white)",
		)
	})

	it("falls back to the neutral treatment without a colour", async ({
		expect,
	}) => {
		const wrapper = await mountPill("+2")
		const pill = wrapper.get("span")

		expect(pill.attributes("style")).toBeUndefined()
		expect(pill.classes()).toContain("bg-muted")
	})
})
