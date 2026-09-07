import { mountSuspended } from "@nuxt/test-utils/runtime"
import type { VueWrapper } from "@vue/test-utils"
import { describe, it, vi } from "vitest"
import CommandList from "./CommandList.vue"
import CommandButton from "./CommandButton.vue"
import { CommandGroup, type CommandData, type CommandItem } from "./items"
import { at, t } from "~/components/test-helpers"

// the list renders the translated title of each item, so fixtures name a
// real slash command and expectations resolve the same string
const title = (slug: string) => t(`editor.slash-commands.items.${slug}.title`)

function item(
	slug: string,
	group: CommandGroup,
	overrides: Partial<CommandItem> = {},
): CommandItem {
	return {
		titleI18nKey: `editor.slash-commands.items.${slug}.title`,
		descriptionI18nKey: `editor.slash-commands.items.${slug}.description`,
		nodeType: slug,
		icon: "lucide:type",
		group: group,
		command: () => undefined,
		...overrides,
	}
}

function mountList(
	items: CommandItem[],
	options: {
		query?: string
		command?: (item: {
			titleI18nKey: string
			command: (data: CommandData) => void
		}) => void
		initClose?: () => void
	} = {},
) {
	return mountSuspended(CommandList, {
		props: {
			query: options.query ?? "",
			items: items,
			command: options.command ?? (() => undefined),
			initClose: options.initClose ?? (() => undefined),
		},
	})
}

function titles(wrapper: VueWrapper): string[] {
	return wrapper
		.findAllComponents(CommandButton)
		.map((button) => (button.props("item") as { title: string }).title)
}

function selectedTitle(wrapper: VueWrapper): string | undefined {
	return wrapper
		.findAll("button")
		.find((button) => button.attributes("data-selected") !== undefined)
		?.text()
}

interface CommandListApi {
	onKeyDown: (event: KeyboardEvent) => boolean
	close: (afterClose: () => void) => void
}

// eslint's ts program cannot type a component exposed through a wrapper,
// so the exposed surface is named here
function api(wrapper: VueWrapper): CommandListApi {
	return wrapper.vm as unknown as CommandListApi
}

function pressKey(wrapper: VueWrapper, key: string): boolean {
	return api(wrapper).onKeyDown(new KeyboardEvent("keydown", { key }))
}

describe("<CommandList>", () => {
	it("lists the commands it was given", async ({ expect }) => {
		const wrapper = await mountList([
			item("heading-1", CommandGroup.Text),
			item("bulleted-list", CommandGroup.List),
		])

		expect(titles(wrapper)).toEqual([
			title("heading-1"),
			title("bulleted-list"),
		])
	})

	it("orders the commands by group", async ({ expect }) => {
		const wrapper = await mountList([
			item("mermaid-diagram", CommandGroup.PowerBlock),
			item("bulleted-list", CommandGroup.List),
			item("heading-1", CommandGroup.Text),
			item("code-block", CommandGroup.BasicBlock),
		])

		expect(titles(wrapper)).toEqual([
			title("heading-1"),
			title("bulleted-list"),
			title("code-block"),
			title("mermaid-diagram"),
		])
	})

	it("separates the groups from one another", async ({ expect }) => {
		const wrapper = await mountList([
			item("heading-1", CommandGroup.Text),
			item("heading-2", CommandGroup.Text),
			item("bulleted-list", CommandGroup.List),
		])

		expect(wrapper.findAll(".bg-border")).toHaveLength(1)
	})

	it("draws no separators inside a single group", async ({ expect }) => {
		const wrapper = await mountList([
			item("heading-1", CommandGroup.Text),
			item("heading-2", CommandGroup.Text),
		])

		expect(wrapper.findAll(".bg-border")).toHaveLength(0)
	})

	it("preselects the first command", async ({ expect }) => {
		const wrapper = await mountList([
			item("heading-1", CommandGroup.Text),
			item("heading-2", CommandGroup.Text),
		])

		expect(selectedTitle(wrapper)).toBe(title("heading-1"))
	})

	it("follows the pointer onto another command", async ({ expect }) => {
		const wrapper = await mountList([
			item("heading-1", CommandGroup.Text),
			item("heading-2", CommandGroup.Text),
		])

		await at(wrapper.findAll("button"), 1).trigger("mouseover")

		expect(selectedTitle(wrapper)).toBe(title("heading-2"))
	})

	it("moves the selection down with the arrow keys", async ({ expect }) => {
		const wrapper = await mountList([
			item("heading-1", CommandGroup.Text),
			item("heading-2", CommandGroup.Text),
		])

		expect(pressKey(wrapper, "ArrowDown")).toBe(true)
		await nextTick()

		expect(selectedTitle(wrapper)).toBe(title("heading-2"))
	})

	it("wraps around at the bottom of the list", async ({ expect }) => {
		const wrapper = await mountList([
			item("heading-1", CommandGroup.Text),
			item("heading-2", CommandGroup.Text),
		])
		pressKey(wrapper, "ArrowDown")

		pressKey(wrapper, "ArrowDown")
		await nextTick()

		expect(selectedTitle(wrapper)).toBe(title("heading-1"))
	})

	it("wraps around at the top of the list", async ({ expect }) => {
		const wrapper = await mountList([
			item("heading-1", CommandGroup.Text),
			item("heading-2", CommandGroup.Text),
		])

		expect(pressKey(wrapper, "ArrowUp")).toBe(true)
		await nextTick()

		expect(selectedTitle(wrapper)).toBe(title("heading-2"))
	})

	it("runs the selected command on enter", async ({ expect }) => {
		const command = vi.fn()
		const heading = item("heading-1", CommandGroup.Text)
		const wrapper = await mountList([heading], { command: command })

		expect(pressKey(wrapper, "Enter")).toBe(true)

		expect(command).toHaveBeenCalledTimes(1)
		expect(command).toHaveBeenCalledWith(heading)
	})

	it("runs the command the reader clicked", async ({ expect }) => {
		const command = vi.fn()
		const list = item("bulleted-list", CommandGroup.List)
		const wrapper = await mountList(
			[item("heading-1", CommandGroup.Text), list],
			{ command: command },
		)

		await at(wrapper.findAll("button"), 1).trigger("click")

		expect(command).toHaveBeenCalledTimes(1)
		expect(command).toHaveBeenCalledWith(list)
	})

	it("ignores keys it has no use for", async ({ expect }) => {
		const wrapper = await mountList([item("heading-1", CommandGroup.Text)])

		expect(pressKey(wrapper, "Escape")).toBe(false)
	})

	it("offers to keep the typed text when nothing matches", async ({
		expect,
	}) => {
		const wrapper = await mountList([], { query: "xyz" })

		expect(wrapper.text()).toContain("Add '/xyz' to the page")
	})

	it("closes the menu when the reader keeps the typed text", async ({
		expect,
	}) => {
		const initClose = vi.fn()
		const wrapper = await mountList([], { query: "xyz", initClose: initClose })

		await wrapper.get("button").trigger("click")

		expect(initClose).toHaveBeenCalledTimes(1)
	})

	it("closes the menu on enter once the query stops matching", async ({
		expect,
	}) => {
		const initClose = vi.fn()
		const command = vi.fn()
		const wrapper = await mountList([item("heading-1", CommandGroup.Text)], {
			query: "xyz",
			initClose: initClose,
			command: command,
		})
		await wrapper.setProps({ items: [] })

		expect(pressKey(wrapper, "Enter")).toBe(true)

		expect(initClose).toHaveBeenCalledTimes(1)
		expect(command).toHaveBeenCalledTimes(0)
	})

	it("selects the first command again when the list changes", async ({
		expect,
	}) => {
		const wrapper = await mountList([
			item("heading-1", CommandGroup.Text),
			item("heading-2", CommandGroup.Text),
		])
		pressKey(wrapper, "ArrowDown")

		await wrapper.setProps({
			items: [item("code-block", CommandGroup.BasicBlock)],
		})

		expect(selectedTitle(wrapper)).toBe(title("code-block"))
	})

	it("fades in once it is mounted", async ({ expect }) => {
		const wrapper = await mountList([item("heading-1", CommandGroup.Text)])

		expect(wrapper.get("div").attributes("data-state")).toBe("open")
	})

	it("fades out before handing control back", async ({ expect }) => {
		vi.useFakeTimers()
		const afterClose = vi.fn()
		const wrapper = await mountList([item("heading-1", CommandGroup.Text)])

		api(wrapper).close(afterClose)
		await nextTick()

		expect(wrapper.get("div").attributes("data-state")).toBe("closed")
		expect(afterClose).toHaveBeenCalledTimes(0)

		await vi.advanceTimersByTimeAsync(500)

		expect(afterClose).toHaveBeenCalledTimes(1)
	})
})
