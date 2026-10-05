import { describe, it } from "vitest"
import { defaultAvatar } from "./avatar"

describe("defaultAvatar", () => {
	it("draws an svg data uri", ({ expect }) => {
		expect(defaultAvatar("user", "ada")).toMatch(/^data:image\/svg\+xml/)
	})

	it("draws the same picture for the same seed", ({ expect }) => {
		expect(defaultAvatar("user", "ada")).toBe(defaultAvatar("user", "ada"))
	})

	it("draws a different picture for another seed", ({ expect }) => {
		expect(defaultAvatar("user", "ada")).not.toBe(
			defaultAvatar("user", "grace"),
		)
	})

	it("draws a user and an organization differently for one seed", ({
		expect,
	}) => {
		expect(defaultAvatar("user", "ada")).not.toBe(
			defaultAvatar("organization", "ada"),
		)
	})

	it("draws every deleted user the same", ({ expect }) => {
		expect(defaultAvatar("deleted-user", "ada")).toBe(
			defaultAvatar("deleted-user"),
		)
	})

	it("draws every invited user the same", ({ expect }) => {
		expect(defaultAvatar("invited-user", "ada")).toBe(
			defaultAvatar("invited-user"),
		)
	})

	it("draws an invited user unlike a deleted one", ({ expect }) => {
		expect(defaultAvatar("invited-user")).not.toBe(
			defaultAvatar("deleted-user"),
		)
	})

	it("draws a deleted user in grey", ({ expect }) => {
		const colors = svg(defaultAvatar("deleted-user")).match(/#[0-9a-f]{6}/gi)

		expect(new Set(colors)).toEqual(new Set(["#e5e5e5", "#a3a3a3"]))
	})

	it("hides the picture itself from screen readers", ({ expect }) => {
		expect(svg(defaultAvatar("user", "ada"))).toContain('aria-hidden="true"')
	})

	it("never animates a user", ({ expect }) => {
		expect(svg(defaultAvatar("user", "ada"))).not.toContain("@keyframes")
	})

	it("draws every organization's planet in a warm colour", ({ expect }) => {
		const warm = ["#e27a8c", "#e37f64", "#d88a40", "#c1982a", "#d67cb2"]
		const pictures = Array.from({ length: 50 }, (_, i) =>
			svg(defaultAvatar("organization", `org-${i}`)),
		)

		expect(
			pictures.every((picture) => warm.some((c) => picture.includes(c))),
		).toBe(true)
	})

	it("draws an organization with few stars", ({ expect }) => {
		// the style fills most of its twelve star slots by default, too many
		// to read on a small tile
		const stars = Array.from(
			{ length: 50 },
			(_, i) =>
				svg(defaultAvatar("organization", `org-${i}`)).match(/href="#star-/g)
					?.length ?? 0,
		)

		expect(Math.max(...stars)).toBeLessThanOrEqual(8)
	})

	it("draws an organization's stars three times the style's size", ({
		expect,
	}) => {
		const pictures = Array.from({ length: 50 }, (_, i) =>
			svg(defaultAvatar("organization", `org-${i}`)),
		).join("")

		// the style's large star has a radius of 1.4
		expect(pictures).toMatch(
			/id="star-large-[^"]+"><g[^>]*><circle[^>]*r="4\.2"/,
		)
	})

	it("animates some organizations and leaves others still", ({ expect }) => {
		const animated = Array.from({ length: 50 }, (_, i) =>
			svg(defaultAvatar("organization", `org-${i}`)).includes("@keyframes"),
		)

		expect(new Set(animated)).toEqual(new Set([true, false]))
	})
})

function svg(uri: string): string {
	return decodeURIComponent(uri.slice(uri.indexOf(",") + 1))
}
