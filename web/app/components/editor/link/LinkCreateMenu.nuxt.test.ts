import { mountSuspended } from "@nuxt/test-utils/runtime"
import type { VueWrapper } from "@vue/test-utils"
import { describe, it } from "vitest"
import LinkCreateMenu from "./LinkCreateMenu.vue"
import {
	commandArgs,
	commandNames,
	makeEditor,
} from "../test-helpers/node-view"
import { t } from "~/components/test-helpers"

function mountMenu() {
	const { editor, commands } = makeEditor()

	return mountSuspended(LinkCreateMenu, { props: { editor: editor } }).then(
		(wrapper) => ({ wrapper, commands }),
	)
}

async function typeUrl(wrapper: VueWrapper, url: string) {
	await wrapper.get("input").setValue(url)
}

async function pressKey(wrapper: VueWrapper, key: string) {
	await wrapper.get("input").trigger("keydown", { key: key })
}

describe("<LinkCreateMenu>", () => {
	it("prompts for a page or url", async ({ expect }) => {
		const { wrapper } = await mountMenu()

		expect(wrapper.get("input").attributes("placeholder")).toBe(
			t("editor.link.page-or-url-placeholder"),
		)
	})

	it("creates a link for the address the reader typed", async ({ expect }) => {
		const { wrapper, commands } = await mountMenu()
		await typeUrl(wrapper, "oxynote.test/docs")

		await pressKey(wrapper, "Enter")

		expect(commandNames(commands)).toEqual(["focus", "setLink", "run"])
		expect(commandArgs(commands, "setLink")).toEqual([
			{ href: "https://oxynote.test/docs" },
		])
	})

	it("keeps the address the reader typed when it already has a scheme", async ({
		expect,
	}) => {
		const { wrapper, commands } = await mountMenu()
		await typeUrl(wrapper, "http://oxynote.test")

		await pressKey(wrapper, "Enter")

		expect(commandArgs(commands, "setLink")).toEqual([
			{ href: "http://oxynote.test" },
		])
	})

	it("closes itself once the link is made", async ({ expect }) => {
		const { wrapper } = await mountMenu()
		await typeUrl(wrapper, "oxynote.test")

		await pressKey(wrapper, "Enter")

		expect(wrapper.emitted("close")).toHaveLength(1)
		expect((wrapper.get("input").element as HTMLInputElement).value).toBe("")
	})

	it("creates nothing for a blank address", async ({ expect }) => {
		const { wrapper, commands } = await mountMenu()
		await typeUrl(wrapper, "   ")

		await pressKey(wrapper, "Enter")

		expect(commands).toEqual([])
		expect(wrapper.emitted("close")).toBeUndefined()
	})

	it("abandons the link on escape", async ({ expect }) => {
		const { wrapper, commands } = await mountMenu()
		await typeUrl(wrapper, "oxynote.test")

		await pressKey(wrapper, "Escape")

		expect(commands).toEqual([])
		expect(wrapper.emitted("close")).toHaveLength(1)
	})

	it("stays open on any other key", async ({ expect }) => {
		const { wrapper } = await mountMenu()

		await pressKey(wrapper, "a")

		expect(wrapper.emitted("close")).toBeUndefined()
	})

	it("abandons the link from its close button", async ({ expect }) => {
		const { wrapper, commands } = await mountMenu()
		await typeUrl(wrapper, "oxynote.test")

		await wrapper.get("button").trigger("click")

		expect(commands).toEqual([])
		expect(wrapper.emitted("close")).toHaveLength(1)
	})
})
