import { flushPromises, type VueWrapper } from "@vue/test-utils"
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

// each avatar renders either an image or an icon. The picture of an entry
// is its image source, and an entry showing an icon has none
function avatarPictures(wrapper: VueWrapper): (string | undefined)[] {
	return wrapper.findAll("li").map((item) => {
		const picture = item.find("img")

		return picture.exists() ? picture.attributes("src") : undefined
	})
}

function defaultPicture(name: string): string {
	return defaultAvatar("user", `id-${name}`)
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

	it("falls back to a person's default avatar", async ({ expect }) => {
		const wrapper = await mountStack({ icons: [person("Ada Lovelace")] })
		await flushPromises()

		expect(avatarPictures(wrapper)).toEqual([defaultPicture("Ada Lovelace")])
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
		await flushPromises()

		expect(avatarPictures(wrapper).slice(0, 3)).toEqual([
			defaultPicture("A"),
			defaultPicture("B"),
			defaultPicture("C"),
		])
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

	it("puts triggered hooks first", async ({ expect }) => {
		const wrapper = await mountStack({
			icons: [
				person("Fresh", { icon: "lucide:bell" }),
				person("Triggered", { icon: "lucide:bell", triggeredHook: true }),
			],
		})

		expect(avatarPictures(wrapper).at(0)).toBeUndefined()
		expect(wrapper.findAll("[data-hook-triggered]")).toHaveLength(1)
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

		await flushPromises()

		expect(avatarPictures(wrapper)).toEqual([
			defaultPicture("Older"),
			defaultPicture("Newer"),
		])
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
