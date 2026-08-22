import type { VueWrapper } from "@vue/test-utils"
import { beforeEach, describe, it } from "vitest"
import IconStack, { type IconMetadata } from "./IconStack.vue"
import {
	clearTeleportedOverlays,
	mountUnderTooltipProvider,
	renderedIconNames,
	t,
} from "~/components/test-helpers"

function person(
	name: string,
	overrides: Partial<IconMetadata> = {},
): IconMetadata {
	return { id: `id-${name}`, name: name, ...overrides }
}

function mountStack(props: Record<string, unknown>) {
	return mountUnderTooltipProvider(IconStack, {
		props: { title: t("editor.name-editor.maintainers"), ...props },
	})
}

// each avatar renders either an image, an icon or the person's initials
function avatarLabels(wrapper: VueWrapper): string[] {
	return wrapper.findAll("li").map((item) => item.text())
}

// the tooltip bodies are teleported into a shared <body>
describe("<IconStack>", { concurrent: false }, () => {
	beforeEach(clearTeleportedOverlays)

	it("names what the stack is showing", async ({ expect }) => {
		const wrapper = await mountStack({ icons: [] })

		expect(wrapper.get("span").text()).toBe(t("editor.name-editor.maintainers"))
	})

	it("shows a single placeholder while the stack is empty", async ({
		expect,
	}) => {
		const wrapper = await mountStack({ icons: [] })

		expect(wrapper.findAll("li")).toHaveLength(1)
		expect(renderedIconNames(wrapper)).toEqual(["lucide:plus"])
	})

	it("shows the placeholder icon the host asked for", async ({ expect }) => {
		const wrapper = await mountStack({ icons: [], default: "lucide:user" })

		expect(renderedIconNames(wrapper)).toEqual(["lucide:user"])
	})

	it("falls back to a person's initials", async ({ expect }) => {
		const wrapper = await mountStack({ icons: [person("Ada Lovelace")] })

		expect(avatarLabels(wrapper)).toEqual(["AL"])
	})

	it("shows a person's picture when they have one", async ({ expect }) => {
		const wrapper = await mountStack({
			icons: [person("Ada", { url: "https://cdn.test/ada.png" })],
		})

		expect(wrapper.get("img").attributes("src")).toBe(
			"https://cdn.test/ada.png",
		)
	})

	it("shows an entry's icon when it has one", async ({ expect }) => {
		const wrapper = await mountStack({
			icons: [person("Reminder", { icon: "lucide:bell" })],
		})

		expect(renderedIconNames(wrapper)).toEqual(["lucide:bell"])
	})

	it("shows at most three entries", async ({ expect }) => {
		const wrapper = await mountStack({
			icons: [person("A"), person("B"), person("C"), person("D")],
		})

		expect(avatarLabels(wrapper).slice(0, 3)).toEqual(["A", "B", "C"])
	})

	it("counts the entries it could not fit", async ({ expect }) => {
		const wrapper = await mountStack({
			icons: [person("A"), person("B"), person("C"), person("D"), person("E")],
		})

		expect(wrapper.text()).toContain("+2")
	})

	it("counts nothing when everything fits", async ({ expect }) => {
		const wrapper = await mountStack({ icons: [person("A"), person("B")] })

		expect(wrapper.text()).not.toContain("+")
	})

	it("marks an approved entry", async ({ expect }) => {
		const wrapper = await mountStack({
			icons: [person("Ada", { approved: true })],
		})

		expect(renderedIconNames(wrapper)).toEqual(["lucide:check"])
	})

	it("puts stale hooks first", async ({ expect }) => {
		const wrapper = await mountStack({
			icons: [
				person("Fresh", { icon: "lucide:bell" }),
				person("Stale", { icon: "lucide:bell", staleHook: true }),
			],
		})

		expect(avatarLabels(wrapper).at(0)).toBe("")
		expect(wrapper.findAll("[data-hook-stale]")).toHaveLength(1)
	})

	it("orders hooks of equal freshness by when they last ran", async ({
		expect,
	}) => {
		const wrapper = await mountStack({
			icons: [
				person("Newer", { hookUpdatedAt: new Date("2026-02-01T00:00:00Z") }),
				person("Older", { hookUpdatedAt: new Date("2026-01-01T00:00:00Z") }),
			],
		})

		expect(avatarLabels(wrapper)).toEqual(["O", "N"])
	})

	it("marks itself clickable when the host says so", async ({ expect }) => {
		const wrapper = await mountStack({ icons: [], clickable: true })

		expect(wrapper.get("div").classes()).toContain("cursor-pointer")
	})

	it("disables its avatars when the host says so", async ({ expect }) => {
		const wrapper = await mountStack({
			icons: [person("Ada")],
			disabled: true,
		})

		expect(
			wrapper.get("[data-slot='avatar']").attributes("data-disabled"),
		).toBe("")
	})
})
