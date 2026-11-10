import { afterEach, beforeEach, describe, it, onTestFinished, vi } from "vitest"
import { userEvent } from "vitest/browser"
import {
	isWideGrid,
	moveTabFocus,
	redirectTabEntry,
	tabDirection,
	trackTabbing,
} from "./screen-order"

// the grid shows its blocks as c, a, b, but the document holds a, b, c
const SCREEN_ORDER = ["c", "a", "b"]

const cleanups: (() => void)[] = []

describe("isWideGrid", () => {
	it.for([
		{ input: 1023, expected: false },
		{ input: 1024, expected: true },
	])(
		"tells a grid $input pixels wide shows its columns: $expected",
		({ input, expected }, { expect }) => {
			expect(isWideGrid(input)).toBe(expected)
		},
	)
})

// the listeners live on the one shared document, so this test runs alone
describe("trackTabbing", { concurrent: false }, () => {
	it("adds its listeners once however many grids call it", ({ expect }) => {
		trackTabbing()
		const addEventListener = vi.spyOn(document, "addEventListener")

		trackTabbing()

		expect(addEventListener).not.toHaveBeenCalled()
	})
})

// the keyboard is one per page, so these tests take turns
describe("tabDirection", { concurrent: false }, () => {
	// focus starts on a button. Then Tab moves between the buttons, and
	// every key reaches this page.
	beforeEach(() => {
		trackTabbing()
		mountGrid()
		focus("b1")
	})
	afterEach(cleanUp)

	it.for([
		{
			name: "forward while Tab is down",
			input: { keys: "{Tab>}", release: "{/Tab}" },
			expected: "forward",
		},
		{
			name: "backward while Shift+Tab is down",
			input: { keys: "{Shift>}{Tab>}", release: "{/Tab}{/Shift}" },
			expected: "backward",
		},
		{
			name: "nothing once another key goes down",
			input: { keys: "{Tab>}{Escape>}", release: "{/Escape}{/Tab}" },
			expected: null,
		},
	])("tells $name", async ({ input, expected }, { expect }) => {
		onTestFinished(() => userEvent.keyboard(input.release))

		await userEvent.keyboard(input.keys)

		expect(tabDirection()).toBe(expected)
	})

	it("tells nothing once the Tab key is up", async ({ expect }) => {
		await userEvent.keyboard("{Tab}")

		expect(tabDirection()).toBeNull()
	})
})

// focus is one per document, so these tests take turns
describe("moveTabFocus", { concurrent: false }, () => {
	afterEach(cleanUp)

	it.for([
		{ name: "moves Tab to the next block on screen", from: "c1", to: "a1" },
		{ name: "moves Tab within a block", from: "a1", to: "a2" },
		{ name: "moves Tab past the block it leaves", from: "a2", to: "b1" },
		{ name: "leaves the grid after its last block", from: "b1", to: "after" },
	])("$name", async ({ from, to }, { expect }) => {
		mountGrid()
		focus(from)

		await userEvent.keyboard("{Tab}")

		expect(document.activeElement?.id).toBe(to)
	})

	it.for([
		{ name: "moves Shift+Tab to the previous block", from: "b1", to: "a2" },
		{
			name: "leaves the grid before its first block",
			from: "c1",
			to: "before",
		},
	])("$name", async ({ from, to }, { expect }) => {
		mountGrid()
		focus(from)

		await userEvent.keyboard("{Shift>}{Tab}{/Shift}")

		expect(document.activeElement?.id).toBe(to)
	})

	it("keeps focus on the last block when nothing after the grid takes Tab", async ({
		expect,
	}) => {
		mountGrid({ nothingAfter: true })
		focus("b1")

		await userEvent.keyboard("{Tab}")

		expect(document.activeElement?.id).toBe("b1")
	})

	it("skips elements Tab cannot stop on", async ({ expect }) => {
		mountGrid({ disabled: "a1", hidden: "a2" })
		focus("c1")

		await userEvent.keyboard("{Tab}")

		expect(document.activeElement?.id).toBe("b1")
	})

	it("leaves a key other than Tab alone", ({ expect }) => {
		const { grid } = mountGrid()
		const event = new KeyboardEvent("keydown", {
			key: "Enter",
			cancelable: true,
		})

		focus("c1")
		grid.dispatchEvent(event)

		expect(event.defaultPrevented).toBe(false)
		expect(document.activeElement?.id).toBe("c1")
	})
})

describe("redirectTabEntry", { concurrent: false }, () => {
	afterEach(cleanUp)

	it("sends Tab coming into the grid to its first block on screen", async ({
		expect,
	}) => {
		mountGrid({ entering: "forward" })
		focus("before")

		await userEvent.keyboard("{Tab}")

		expect(document.activeElement?.id).toBe("c1")
	})

	it("sends Shift+Tab coming into the grid to its last block on screen", async ({
		expect,
	}) => {
		mountGrid({ entering: "backward" })
		focus("after")

		await userEvent.keyboard("{Shift>}{Tab}{/Shift}")

		expect(document.activeElement?.id).toBe("b1")
	})

	it("keeps focus that moves within the grid", ({ expect }) => {
		mountGrid({ entering: "forward" })
		focus("b1")

		focus("a2")

		expect(document.activeElement?.id).toBe("a2")
	})
})

// a page with a button before and after a grid of the blocks a (two
// buttons), b and c. The grid moves Tab in SCREEN_ORDER. With entering
// set, it also redirects Tab that comes in from outside.
function mountGrid(
	options: {
		disabled?: string
		hidden?: string
		entering?: "forward" | "backward"
		nothingAfter?: boolean
	} = {},
) {
	const page = document.createElement("div")

	page.innerHTML = `
		<button id="before">before</button>
		<div id="grid">
			<div id="a"><button id="a1">a1</button><button id="a2">a2</button></div>
			<div id="b"><button id="b1">b1</button></div>
			<div id="c"><button id="c1">c1</button></div>
		</div>
		<button id="after">after</button>
	`
	document.body.append(page)
	cleanups.push(() => {
		page.remove()
	})

	if (options.nothingAfter) {
		element("after").remove()
	}

	if (options.disabled) {
		element(options.disabled).setAttribute("disabled", "")
	}

	if (options.hidden) {
		element(options.hidden).style.display = "none"
	}

	const grid = element("grid")
	const blocks = () => SCREEN_ORDER.map(element)

	grid.addEventListener("keydown", (event) => {
		moveTabFocus(event, grid, blocks)
	})

	if (options.entering) {
		grid.addEventListener("focusin", (event) => {
			redirectTabEntry(event, grid, blocks(), options.entering === "backward")
		})
	}

	return { grid: grid }
}

function element(id: string): HTMLElement {
	const found = document.getElementById(id)

	if (!found) {
		throw new Error(`no element ${id}`)
	}

	return found
}

function focus(id: string) {
	element(id).focus()
}

function cleanUp() {
	for (const cleanup of cleanups.splice(0)) {
		cleanup()
	}
}
