import type { Editor } from "@tiptap/core"
import { Schema, type Node as PMNode } from "@tiptap/pm/model"
import { EditorState } from "@tiptap/pm/state"
import { EditorView } from "@tiptap/pm/view"
import { afterEach, describe, it, vi } from "vitest"
import {
	clearRepositionCooldown,
	disableGapZones,
	enableGapZones,
	findGapElementsByPos,
	getGapDropPosition,
	repositionGapZones,
} from "./gap-decorations"
import {
	METRIC_BLOCK_NAME,
	METRIC_GRID_NAME,
	SPLIT_DOCUMENTATION_LEFT_SIDE_NAME,
	SPLIT_DOCUMENTATION_NAME,
	SPLIT_DOCUMENTATION_RIGHT_SIDE_NAME,
	TITLED_CODE_BLOCK_NAME,
} from "../blocks/node-names"
import { gapPlugin } from "./test-helpers"
import { attrBlockBuilder, docBuilder } from "~/components/editor/test-helpers"

// fixed spacing and sizes so every measured gap centre, row wrap and
// sibling indent is the same on every run. The second item of a nested
// list is pushed further right on purpose: the horizontal measurement
// picks whichever sibling is more indented
const style = document.createElement("style")
style.textContent = `
	body { margin: 0; }
	.pm-host { position: relative; width: 600px; }
	.ProseMirror { position: relative; }
	.ProseMirror p, .ProseMirror h1, .ProseMirror pre { margin: 10px 0; }
	.ProseMirror ul { margin: 10px 0; padding-left: 20px; }
	.ProseMirror li { margin: 6px 0; }
	.ProseMirror ul ul li:nth-of-type(2) { margin-left: 30px; }
	.metric-grid { display: flex; flex-wrap: wrap; gap: 8px; }
	.metric-block { width: 250px; height: 60px; }
`
document.head.appendChild(style)

const schema = new Schema({
	nodes: {
		doc: { content: "block*" },
		text: { group: "inline" },
		paragraph: {
			group: "block",
			content: "inline*",
			attrs: { uid: { default: null } },
			toDOM: () => ["p", 0],
		},
		heading: {
			group: "block",
			content: "inline*",
			attrs: { uid: { default: null } },
			toDOM: () => ["h1", 0],
		},
		bulletList: {
			group: "block",
			content: "listItem+",
			attrs: { uid: { default: null } },
			toDOM: () => ["ul", 0],
		},
		orderedList: {
			group: "block",
			content: "listItem+",
			attrs: { uid: { default: null } },
			toDOM: () => ["ol", 0],
		},
		listItem: {
			content: "paragraph (bulletList | orderedList)?",
			attrs: { uid: { default: null } },
			toDOM: () => ["li", 0],
		},
		[TITLED_CODE_BLOCK_NAME]: {
			group: "block",
			content: "inline*",
			attrs: { uid: { default: null } },
			toDOM: () => ["pre", 0],
		},
		[METRIC_GRID_NAME]: {
			group: "block",
			content: `${METRIC_BLOCK_NAME}*`,
			attrs: { uid: { default: null } },
			toDOM: () => ["div", { class: "metric-grid" }, 0],
		},
		[METRIC_BLOCK_NAME]: {
			content: "inline*",
			attrs: { uid: { default: null }, height: { default: null } },
			toDOM: (node) => [
				"div",
				{
					class: "metric-block",
					style:
						typeof node.attrs.height === "number"
							? `height: ${node.attrs.height}px`
							: "",
				},
				0,
			],
		},
		[SPLIT_DOCUMENTATION_NAME]: {
			group: "block",
			content: `${SPLIT_DOCUMENTATION_LEFT_SIDE_NAME} ${SPLIT_DOCUMENTATION_RIGHT_SIDE_NAME}`,
			attrs: { uid: { default: null } },
			toDOM: () => ["div", { class: "split-doc" }, 0],
		},
		[SPLIT_DOCUMENTATION_LEFT_SIDE_NAME]: {
			content: "heading (paragraph | bulletList | orderedList)+",
			attrs: { uid: { default: null } },
			toDOM: () => ["div", { class: "left-side" }, 0],
		},
		[SPLIT_DOCUMENTATION_RIGHT_SIDE_NAME]: {
			content: `(${TITLED_CODE_BLOCK_NAME} | ${METRIC_BLOCK_NAME})+`,
			attrs: { uid: { default: null } },
			toDOM: () => ["div", { class: "right-side" }, 0],
		},
	},
})

// a second schema missing the wrapper types the gap logic falls back to,
// so the "wrapping is impossible" branches of canDropAtGap are reachable
const wrapperlessSchema = new Schema({
	nodes: {
		doc: { content: "block*" },
		text: { group: "inline" },
		paragraph: {
			group: "block",
			content: "inline*",
			attrs: { uid: { default: null } },
			toDOM: () => ["p", 0],
		},
		listItem: {
			content: "paragraph",
			attrs: { uid: { default: null } },
			toDOM: () => ["li", 0],
		},
		[METRIC_BLOCK_NAME]: {
			content: "inline*",
			attrs: { uid: { default: null } },
			toDOM: () => ["div", { class: "metric-block" }, 0],
		},
	},
})

// the gap config keys off type names alone, so a list holding a
// paragraph is enough to sit a "1rem" child next to a "1.5rem" one —
// a pair the real schema never produces
const mixedSchema = new Schema({
	nodes: {
		doc: { content: "block*" },
		text: { group: "inline" },
		paragraph: {
			group: "block",
			content: "inline*",
			attrs: { uid: { default: null } },
			toDOM: () => ["p", 0],
		},
		listItem: {
			group: "block",
			content: "inline*",
			attrs: { uid: { default: null } },
			toDOM: () => ["li", 0],
		},
		bulletList: {
			group: "block",
			content: "block+",
			attrs: { uid: { default: null } },
			toDOM: () => ["ul", 0],
		},
	},
})

const docOf = docBuilder(schema)
const block = attrBlockBuilder(schema)

const para = (uid: string | null, text = "ab") =>
	block("paragraph", { uid }, text)

const heading = (uid: string | null) => block("heading", { uid }, "ab")

const titledCodeBlock = (uid: string | null) =>
	block(TITLED_CODE_BLOCK_NAME, { uid }, "ab")

const bulletList = (uid: string | null, ...items: PMNode[]) =>
	block("bulletList", { uid }, ...items)

const orderedList = (uid: string | null, ...items: PMNode[]) =>
	block("orderedList", { uid }, ...items)

const listItem = (uid: string | null, ...content: PMNode[]) =>
	block("listItem", { uid }, ...content)

const metricGrid = (uid: string | null, ...blocks: PMNode[]) =>
	block(METRIC_GRID_NAME, { uid }, ...blocks)

const metricBlock = (uid: string | null, height: number | null = null) =>
	block(METRIC_BLOCK_NAME, { uid, height }, "ab")

const splitDoc = (uid: string | null, left: PMNode, right: PMNode) =>
	block(SPLIT_DOCUMENTATION_NAME, { uid }, left, right)

const leftSide = (uid: string | null, ...content: PMNode[]) =>
	block(SPLIT_DOCUMENTATION_LEFT_SIDE_NAME, { uid }, ...content)

const rightSide = (uid: string | null, ...content: PMNode[]) =>
	block(SPLIT_DOCUMENTATION_RIGHT_SIDE_NAME, { uid }, ...content)

const cleanups: (() => void)[] = []

// one root suite so the shared cleanup hook belongs to a suite, and so
// every case runs sequentially: they share the page, the gap zone
// selectors are document wide, and the module's reposition scheduling is
// global
describe("gap decorations", { concurrent: false }, () => {
	afterEach(async () => {
		for (const cleanup of cleanups.splice(0)) {
			cleanup()
		}

		// the module coalesces repositioning behind a shared frame flag and a
		// cooldown; both have to be drained or the next test's reposition
		// request is silently swallowed. Editor views schedule repositions of
		// their own through a resize observer, so the frame is drained first
		await nextFrame()
		clearRepositionCooldown()
	})

	interface Mounted {
		view: EditorView
		host: HTMLElement
		editor: Editor
	}

	// gap zones only measure correctly inside a laid-out, positioned host,
	// so every positioning test renders through a real editor view
	function mountEditor(doc: PMNode, hostWidth = 600): Mounted {
		const host = document.createElement("div")
		host.classList.add("pm-host")
		host.style.width = `${hostWidth}px`
		document.body.appendChild(host)

		const view = new EditorView(host, {
			state: EditorState.create({ doc, plugins: [gapPlugin()] }),
		})

		cleanups.push(() => {
			view.destroy()
			host.remove()
		})

		return { view, host, editor: { state: view.state } as unknown as Editor }
	}

	// the widgets position themselves in the frame after they are created
	function nextFrame(): Promise<void> {
		return new Promise((resolve) => {
			requestAnimationFrame(() => {
				resolve()
			})
		})
	}

	function gapEl(host: HTMLElement, key: string): HTMLElement {
		const el = host.querySelector<HTMLElement>(
			`.pm-gap-zone[data-gap-key="${key}"]:not(.pm-gap-zone-secondary)`,
		)

		if (!el) {
			throw new Error(`no gap zone rendered for key ${key}`)
		}

		return el
	}

	function blockEl(
		host: HTMLElement,
		selector: string,
		index = 0,
	): HTMLElement {
		const el = host.querySelectorAll<HTMLElement>(selector)[index]

		if (!el) {
			throw new Error(`no element matching ${selector}[${index}]`)
		}

		return el
	}

	function offsetParentRect(el: HTMLElement): DOMRect {
		const parent = el.offsetParent

		if (!parent) {
			throw new Error("gap zone has no offset parent")
		}

		return parent.getBoundingClientRect()
	}

	function centerY(el: HTMLElement): number {
		const rect = el.getBoundingClientRect()

		return rect.top + rect.height / 2
	}

	describe("gap widget rendering", () => {
		it("renders one absolutely positioned zone per collected gap", ({
			expect,
		}) => {
			const { host } = mountEditor(docOf(para("p1"), para("p2")))

			expect(
				[...host.querySelectorAll<HTMLElement>(".pm-gap-zone")].map((el) =>
					el.getAttribute("data-gap-key"),
				),
			).toEqual(["doc:before:p1", "doc:before:p2", "doc:after:p2"])

			const el = gapEl(host, "doc:before:p2")

			expect(el.classList.contains("drag-handle-ignore-self")).toBe(true)
			expect(el.parentElement?.classList.contains("pm-gap-wrapper")).toBe(true)
			expect(el.style.position).toBe("absolute")
			expect(el.style.height).toBe("1.5rem")
			expect(el.style.pointerEvents).toBe("none")
			expect(el.style.zIndex).toBe("50")
			expect(el.getAttribute("data-gap-indent-level")).toBe("0")
			expect(el.getAttribute("data-gap-left-inset")).toBe("0")
			expect(el.getAttribute("data-gap-right-inset")).toBe("0")
			expect(el.getAttribute("data-gap-orientation")).toBeNull()
			expect(el.getAttribute("data-debug-color")).toBeNull()
		})

		it("carries the per-position vertical offsets of the child config", ({
			expect,
		}) => {
			const { host } = mountEditor(docOf(heading("h1"), heading("h2")))

			expect(gapEl(host, "doc:before:h1").dataset.yOffset).toBe("3")
			expect(gapEl(host, "doc:before:h2").dataset.yOffset).toBe("3")
			expect(gapEl(host, "doc:after:h2").dataset.yOffset).toBe("0")
		})

		it("grows a middle gap to the taller of its two neighbours", ({
			expect,
		}) => {
			const { host } = mountEditor(docOf(titledCodeBlock("c1"), para("p1")))

			// the code block asks for 2rem, the paragraph for 1.5rem
			expect(gapEl(host, "doc:before:c1").style.height).toBe("2rem")
			expect(gapEl(host, "doc:before:p1").style.height).toBe("2rem")
			expect(gapEl(host, "doc:after:p1").style.height).toBe("1.5rem")
		})

		it("keeps the child height when the previous sibling asks for less", ({
			expect,
		}) => {
			const { host } = mountEditor(docOf(para("p1"), titledCodeBlock("c1")))

			expect(gapEl(host, "doc:before:c1").style.height).toBe("2rem")
		})

		it("compares neighbouring gap heights by length, not as strings", ({
			expect,
		}) => {
			const list = mixedSchema.nodes.bulletList.create({ uid: "l0" }, [
				mixedSchema.nodes.listItem.create(
					{ uid: "i1" },
					mixedSchema.text("ab"),
				),
				mixedSchema.nodes.paragraph.create(
					{ uid: "p1" },
					mixedSchema.text("ab"),
				),
			])
			const { host } = mountEditor(mixedSchema.nodes.doc.create(null, [list]))

			// the list item asks for 1rem and the paragraph for 1.5rem, a
			// pair a string comparison orders backwards
			expect(gapEl(host, "l0:before:i1").style.height).toBe("1rem")
			expect(gapEl(host, "l0:before:p1").style.height).toBe("1.5rem")
		})

		it("raises the indent level, inset and stacking order inside nested lists", ({
			expect,
		}) => {
			const { host } = mountEditor(
				docOf(
					bulletList(
						"l0",
						listItem(
							"li0",
							para("a"),
							bulletList("l1", listItem("li1", para("b"))),
						),
					),
				),
			)

			const outer = gapEl(host, "l0:before:li0")
			const inner = gapEl(host, "l1:before:li1")

			expect(outer.getAttribute("data-gap-indent-level")).toBe("0")
			expect(outer.getAttribute("data-gap-left-inset")).toBe("0")
			expect(outer.style.zIndex).toBe("50")
			expect(inner.getAttribute("data-gap-indent-level")).toBe("1")
			expect(inner.getAttribute("data-gap-left-inset")).toBe("-15")
			expect(inner.style.height).toBe("1rem")
			expect(inner.style.zIndex).toBe("51")
		})

		it("renders metric grid gaps as vertical zones", ({ expect }) => {
			const { host } = mountEditor(
				docOf(metricGrid("mg", metricBlock("m1"), metricBlock("m2"))),
			)

			const vertical = gapEl(host, "mg:before:m2")

			expect(vertical.getAttribute("data-gap-orientation")).toBe("vertical")
			expect(vertical.getAttribute("data-gap-x-offset")).toBe("0")
			expect(vertical.style.width).toBe("1rem")
			expect(
				gapEl(host, "doc:before:mg").getAttribute("data-gap-orientation"),
			).toBeNull()
		})
	})

	describe("horizontal gap positioning", () => {
		it("centres a middle gap between its two siblings", async ({ expect }) => {
			const { host } = mountEditor(docOf(para("p1"), para("p2")))
			await nextFrame()

			const prev = blockEl(host, "p", 0)
			const next = blockEl(host, "p", 1)
			const gap = gapEl(host, "doc:before:p2")
			const boundary =
				(prev.getBoundingClientRect().bottom +
					next.getBoundingClientRect().top) /
				2

			expect(centerY(gap)).toBeCloseTo(boundary, 1)
			expect(gap.style.left).toBe("0px")
			expect(gap.style.right).toBe("0px")
		})

		it("hangs the leading gap above the first sibling", async ({ expect }) => {
			const { host } = mountEditor(docOf(para("p1"), para("p2")))
			await nextFrame()

			const first = blockEl(host, "p", 0)
			const gap = gapEl(host, "doc:before:p1")

			expect(gap.getBoundingClientRect().bottom).toBeCloseTo(
				first.getBoundingClientRect().top,
				1,
			)
		})

		it("hangs the trailing gap below the last sibling", async ({ expect }) => {
			const { host } = mountEditor(docOf(para("p1"), para("p2")))
			await nextFrame()

			const last = blockEl(host, "p", 1)
			const gap = gapEl(host, "doc:after:p2")

			expect(gap.getBoundingClientRect().top).toBeCloseTo(
				last.getBoundingClientRect().bottom,
				1,
			)
		})

		it("applies the config offset on top of the measured centre", async ({
			expect,
		}) => {
			const { host } = mountEditor(docOf(heading("h1"), heading("h2")))
			await nextFrame()

			const prev = blockEl(host, "h1", 0)
			const next = blockEl(host, "h1", 1)
			const gap = gapEl(host, "doc:before:h2")
			const boundary =
				(prev.getBoundingClientRect().bottom +
					next.getBoundingClientRect().top) /
				2

			// heading gaps sit 3px below the geometric centre
			expect(centerY(gap)).toBeCloseTo(boundary + 3, 1)
		})

		it("bounds indented gaps to the more indented of the two siblings", async ({
			expect,
		}) => {
			const { host } = mountEditor(
				docOf(
					bulletList(
						"l0",
						listItem(
							"li0",
							para("a"),
							bulletList(
								"l1",
								listItem("li1", para("b")),
								listItem("li2", para("c")),
								listItem("li3", para("d")),
							),
						),
					),
				),
			)
			await nextFrame()

			const items = [...host.querySelectorAll<HTMLElement>("ul ul li")]
			const [item1, item2, item3] = items as [
				HTMLElement,
				HTMLElement,
				HTMLElement,
			]
			const parentRect = offsetParentRect(gapEl(host, "l1:before:li2"))

			// css pushes the second item 30px right, so it wins both middle
			// gaps: once as the next sibling and once as the previous one
			expect(item2.getBoundingClientRect().left).toBeGreaterThan(
				item1.getBoundingClientRect().left,
			)
			expect(gapEl(host, "l1:before:li2").style.left).toBe(
				`${item2.getBoundingClientRect().left - parentRect.left - 15}px`,
			)
			expect(gapEl(host, "l1:before:li3").style.left).toBe(
				`${item2.getBoundingClientRect().left - parentRect.left - 15}px`,
			)
			expect(gapEl(host, "l1:after:li3").style.left).toBe(
				`${item3.getBoundingClientRect().left - parentRect.left - 15}px`,
			)
		})

		it("falls back to zero insets when a zone carries no inset attributes", async ({
			expect,
		}) => {
			const container = document.createElement("div")
			container.style.position = "relative"
			container.style.width = "400px"
			container.innerHTML = `
				<div style="height: 20px"></div>
				<div class="pm-gap-wrapper" style="display: contents">
					<div class="pm-gap-zone" style="position: absolute; height: 10px"></div>
				</div>
				<div style="height: 20px"></div>`
			document.body.appendChild(container)
			cleanups.push(() => {
				container.remove()
			})

			repositionGapZones()
			await nextFrame()

			const zone = container.querySelector<HTMLElement>(".pm-gap-zone")

			expect(zone?.style.left).toBe("0px")
			expect(zone?.style.right).toBe("0px")
			expect(zone?.style.top).toBe("15px")
		})
	})

	describe("vertical gap positioning", () => {
		it("centres a same-row gap between two metric blocks", async ({
			expect,
		}) => {
			const { host } = mountEditor(
				docOf(metricGrid("mg", metricBlock("m1"), metricBlock("m2", 90))),
			)
			await nextFrame()

			const first = blockEl(host, ".metric-block", 0)
			const second = blockEl(host, ".metric-block", 1)
			const gap = gapEl(host, "mg:before:m2")
			const gapRect = gap.getBoundingClientRect()
			const boundary =
				(first.getBoundingClientRect().right +
					second.getBoundingClientRect().left) /
				2

			expect(gapRect.left + gapRect.width / 2).toBeCloseTo(boundary, 1)
			// the height of a between-children gap is the tallest grid child
			expect(gap.style.height).toBe(`${second.offsetHeight}px`)
			expect(gap.style.bottom).toBe("auto")
		})

		it("sizes the leading and trailing gaps after their single neighbour", async ({
			expect,
		}) => {
			const { host } = mountEditor(
				docOf(metricGrid("mg", metricBlock("m1"), metricBlock("m2", 90))),
			)
			await nextFrame()

			const first = blockEl(host, ".metric-block", 0)
			const second = blockEl(host, ".metric-block", 1)
			const leading = gapEl(host, "mg:before:m1")
			const trailing = gapEl(host, "mg:after:m2")

			expect(leading.style.height).toBe(`${first.offsetHeight}px`)
			expect(trailing.style.height).toBe(`${second.offsetHeight}px`)
			expect(leading.getBoundingClientRect().right).toBeCloseTo(
				first.getBoundingClientRect().left,
				1,
			)
			expect(trailing.getBoundingClientRect().left).toBeCloseTo(
				second.getBoundingClientRect().right,
				1,
			)
		})

		it("adds a secondary zone at the end of the previous row when blocks wrap", async ({
			expect,
		}) => {
			const { host } = mountEditor(
				docOf(
					metricGrid(
						"mg",
						metricBlock("m1"),
						metricBlock("m2"),
						metricBlock("m3"),
					),
				),
			)
			await nextFrame()

			const second = blockEl(host, ".metric-block", 1)
			const third = blockEl(host, ".metric-block", 2)

			expect(third.getBoundingClientRect().top).toBeGreaterThan(
				second.getBoundingClientRect().bottom - 10,
			)

			const primary = gapEl(host, "mg:before:m3")
			const secondary = primary.parentElement?.querySelector<HTMLElement>(
				".pm-gap-zone-secondary",
			)

			expect(secondary).not.toBeNull()
			expect(secondary?.style.display).toBe("block")
			expect(secondary?.getAttribute("data-gap-key")).toBe("mg:before:m3")
			expect(secondary?.getAttribute("data-gap-orientation")).toBe("vertical")
			expect(secondary?.style.height).toBe(`${second.offsetHeight}px`)
			// the primary zone moves to the start of the wrapped row
			expect(primary.getBoundingClientRect().right).toBeCloseTo(
				third.getBoundingClientRect().left,
				1,
			)
		})

		it("reuses and then hides the secondary zone when the rows stop wrapping", async ({
			expect,
		}) => {
			const { host } = mountEditor(
				docOf(
					metricGrid(
						"mg",
						metricBlock("m1"),
						metricBlock("m2"),
						metricBlock("m3"),
					),
				),
			)
			await nextFrame()

			const secondary = gapEl(
				host,
				"mg:before:m3",
			).parentElement?.querySelector<HTMLElement>(".pm-gap-zone-secondary")

			if (!secondary) {
				throw new Error("no secondary gap zone was created")
			}

			expect(secondary.style.display).toBe("block")

			repositionGapZones()
			await nextFrame()

			// the same element is updated in place rather than duplicated
			expect(
				gapEl(host, "mg:before:m3").parentElement?.querySelectorAll(
					".pm-gap-zone-secondary",
				).length,
			).toBe(1)

			clearRepositionCooldown()
			host.style.width = "1200px"
			repositionGapZones()
			await nextFrame()

			expect(secondary.style.display).toBe("none")
		})

		it("leaves zones alone when they cannot be measured", async ({
			expect,
		}) => {
			// a zone needs a laid-out offset parent and at least one sibling
			// to measure against; these have neither
			function fixture(
				key: string,
				hidden: boolean,
				vertical: boolean,
			): HTMLElement {
				const container = document.createElement("div")
				container.style.position = "relative"
				container.style.display = hidden ? "none" : "block"

				const wrapper = document.createElement("div")
				const zone = document.createElement("div")
				zone.classList.add("pm-gap-zone")
				zone.setAttribute("data-gap-key", key)
				zone.style.top = "1px"
				zone.style.left = "2px"

				if (vertical) {
					zone.setAttribute("data-gap-orientation", "vertical")
				}

				wrapper.appendChild(zone)
				container.appendChild(wrapper)
				document.body.appendChild(container)
				cleanups.push(() => {
					container.remove()
				})

				return zone
			}

			const zones = [
				fixture("hidden-horizontal", true, false),
				fixture("hidden-vertical", true, true),
				fixture("lonely-horizontal", false, false),
				fixture("lonely-vertical", false, true),
			]

			repositionGapZones()
			await nextFrame()

			for (const zone of zones) {
				expect(zone.style.top).toBe("1px")
				expect(zone.style.left).toBe("2px")
			}
		})

		it("copies the debug colour onto a recreated secondary zone", async ({
			expect,
		}) => {
			const { host } = mountEditor(
				docOf(
					metricGrid(
						"mg",
						metricBlock("m1"),
						metricBlock("m2"),
						metricBlock("m3"),
					),
				),
			)
			await nextFrame()

			const primary = gapEl(host, "mg:before:m3")
			primary.parentElement?.querySelector(".pm-gap-zone-secondary")?.remove()
			primary.setAttribute("data-debug-color", "rgb(1, 2, 3)")
			primary.style.backgroundColor = "rgb(1, 2, 3)"

			clearRepositionCooldown()
			repositionGapZones()
			await nextFrame()

			const secondary = primary.parentElement?.querySelector<HTMLElement>(
				".pm-gap-zone-secondary",
			)

			expect(secondary?.getAttribute("data-debug-color")).toBe("rgb(1, 2, 3)")
			expect(secondary?.style.backgroundColor).toBe("rgb(1, 2, 3)")
		})

		it("falls back to a zero offset and an empty key without attributes", async ({
			expect,
		}) => {
			const container = document.createElement("div")
			container.style.position = "relative"
			container.style.display = "flex"
			container.innerHTML = `
				<div style="width: 50px; height: 30px"></div>
				<div class="pm-gap-wrapper" style="display: contents">
					<div class="pm-gap-zone" data-gap-orientation="vertical"
						style="position: absolute; width: 10px"></div>
				</div>
				<div style="width: 50px; height: 30px"></div>`
			document.body.appendChild(container)
			cleanups.push(() => {
				container.remove()
			})

			repositionGapZones()
			await nextFrame()

			const zone = container.querySelector<HTMLElement>(".pm-gap-zone")

			expect(zone?.style.left).toBe("45px")
			expect(zone?.style.top).toBe("0px")
			expect(zone?.style.height).toBe("30px")
		})

		it("does not position zones whose editor was torn down first", async ({
			expect,
		}) => {
			const { view, host } = mountEditor(docOf(para("p1")))
			const gap = gapEl(host, "doc:before:p1")

			view.destroy()
			await nextFrame()

			expect(gap.style.top).toBe("")
		})
	})

	// the scheduling flags and the cooldown handle live in module scope, so
	// this suite drives them with fake timers and drains every frame it
	// schedules before handing back to real ones
	describe("reposition scheduling", () => {
		afterEach(() => {
			clearRepositionCooldown()
			// the frame spy below wraps the faked timer, so it has to be put
			// back before the fake timers are uninstalled
			vi.restoreAllMocks()
			vi.useRealTimers()
		})

		function useFrameTimers() {
			vi.useFakeTimers({
				toFake: [
					"setTimeout",
					"clearTimeout",
					"requestAnimationFrame",
					"cancelAnimationFrame",
				],
			})
		}

		it("coalesces repeated requests into one frame and one cooldown", ({
			expect,
		}) => {
			useFrameTimers()

			const raf = vi.spyOn(globalThis, "requestAnimationFrame")

			repositionGapZones()
			repositionGapZones()

			expect(raf).toHaveBeenCalledOnce()

			vi.advanceTimersToNextFrame()

			// inside the cooldown the request is only remembered
			repositionGapZones()

			expect(raf).toHaveBeenCalledOnce()

			vi.advanceTimersByTime(3000)

			expect(raf).toHaveBeenCalledTimes(2)

			vi.advanceTimersToNextFrame()
			vi.advanceTimersByTime(3000)

			// the trailing run had nothing pending, so it stops there
			expect(raf).toHaveBeenCalledTimes(2)
		})

		it("lets a new request through as soon as the cooldown is cleared", ({
			expect,
		}) => {
			useFrameTimers()

			const raf = vi.spyOn(globalThis, "requestAnimationFrame")

			repositionGapZones()
			vi.advanceTimersToNextFrame()
			clearRepositionCooldown()
			repositionGapZones()

			expect(raf).toHaveBeenCalledTimes(2)

			vi.advanceTimersToNextFrame()
			vi.advanceTimersByTime(3000)
		})

		it("does nothing when the document holds no gap zones", ({ expect }) => {
			useFrameTimers()

			expect(document.querySelectorAll(".pm-gap-zone")).toHaveLength(0)

			repositionGapZones()

			expect(() => {
				vi.advanceTimersToNextFrame()
			}).not.toThrow()

			vi.advanceTimersByTime(3000)
		})
	})

	describe("getGapDropPosition", () => {
		it("resolves the drop position of a gap zone element", ({ expect }) => {
			const { view, host } = mountEditor(docOf(para("p1"), para("p2")))

			expect(getGapDropPosition(gapEl(host, "doc:before:p2"), view.state)).toBe(
				4,
			)
		})

		it("resolves each zone of two siblings that share a uid separately", ({
			expect,
		}) => {
			const { view, host } = mountEditor(docOf(para("dup"), para("dup")))
			const zones = [
				...host.querySelectorAll<HTMLElement>(".pm-gap-zone"),
			].slice(0, 2)

			expect(zones.map((zone) => getGapDropPosition(zone, view.state))).toEqual(
				[0, 4],
			)
		})

		it("resolves the position from an ancestor gap zone", ({ expect }) => {
			const { view, host } = mountEditor(docOf(para("p1")))
			const inner = document.createElement("span")
			gapEl(host, "doc:after:p1").appendChild(inner)

			expect(getGapDropPosition(inner, view.state)).toBe(4)
		})

		it.for([
			{ name: "an unknown key on the element itself", nested: false },
			{ name: "an unknown key on an ancestor", nested: true },
		])("returns null for $name", ({ nested }, { expect }) => {
			const { view, host } = mountEditor(docOf(para("p1")))
			const el = gapEl(host, "doc:before:p1")
			el.setAttribute("data-gap-key", "stale")

			const target = nested
				? el.appendChild(document.createElement("span"))
				: el

			expect(getGapDropPosition(target, view.state)).toBeNull()
		})

		it("returns null for a missing element", ({ expect }) => {
			const { view } = mountEditor(docOf(para("p1")))

			expect(getGapDropPosition(null, view.state)).toBeNull()
		})

		it("returns null for an element outside any gap zone", ({ expect }) => {
			const { view, host } = mountEditor(docOf(para("p1")))

			expect(getGapDropPosition(blockEl(host, "p"), view.state)).toBeNull()
		})

		it("returns null when the plugin is not installed", ({ expect }) => {
			const { host } = mountEditor(docOf(para("p1")))
			const foreign = EditorState.create({ doc: docOf(para("p1")) })

			expect(
				getGapDropPosition(gapEl(host, "doc:before:p1"), foreign),
			).toBeNull()
		})
	})

	describe("findGapElementsByPos", () => {
		it("finds every element rendered for a gap position", ({ expect }) => {
			const { view, host } = mountEditor(docOf(para("p1"), para("p2")))

			expect(findGapElementsByPos(view.state, 4)).toEqual([
				gapEl(host, "doc:before:p2"),
			])
		})

		it("includes the secondary element sharing a wrapped gap key", async ({
			expect,
		}) => {
			const { view, host } = mountEditor(
				docOf(
					metricGrid(
						"mg",
						metricBlock("m1"),
						metricBlock("m2"),
						metricBlock("m3"),
					),
				),
			)
			await nextFrame()

			const primary = gapEl(host, "mg:before:m3")
			const found = findGapElementsByPos(view.state, 9)

			expect(found).toHaveLength(2)
			expect(found).toContain(primary)
		})

		it("returns nothing for a position without a gap", ({ expect }) => {
			const { view } = mountEditor(docOf(para("p1")))

			expect(findGapElementsByPos(view.state, 2)).toEqual([])
		})

		it("returns nothing when the plugin is not installed", ({ expect }) => {
			mountEditor(docOf(para("p1")))
			const foreign = EditorState.create({ doc: docOf(para("p1")) })

			expect(findGapElementsByPos(foreign, 0)).toEqual([])
		})
	})

	describe("enableGapZones", () => {
		function statesByKey(host: HTMLElement): Record<string, string> {
			const entries: [string, string][] = []

			host
				.querySelectorAll<HTMLElement>(".pm-gap-zone[data-gap-key]")
				.forEach((el) => {
					entries.push([
						el.getAttribute("data-gap-key") ?? "",
						el.style.pointerEvents,
					])
				})

			return Object.fromEntries(entries)
		}

		it("enables every gap a plain block may be dropped into", ({ expect }) => {
			const { host, editor } = mountEditor(docOf(para("p1"), para("p2")))

			enableGapZones(editor, editor.state.doc.child(0), 0)

			expect(statesByKey(host)).toEqual({
				"doc:before:p1": "auto",
				"doc:before:p2": "auto",
				"doc:after:p2": "auto",
			})
			expect(gapEl(host, "doc:before:p1").style.backgroundColor).toBe(
				"transparent",
			)
		})

		it("rejects gaps whose parent cannot hold the dragged block", ({
			expect,
		}) => {
			const { host, editor } = mountEditor(
				docOf(para("p1"), metricGrid("mg", metricBlock("m1"))),
			)

			enableGapZones(editor, editor.state.doc.child(0), 0)

			expect(statesByKey(host)).toEqual({
				"doc:before:p1": "auto",
				"doc:before:mg": "auto",
				"mg:before:m1": "none",
				"mg:after:m1": "none",
				"doc:after:mg": "auto",
			})
		})

		it("keeps list items inside lists of their original type", ({ expect }) => {
			const { host, editor } = mountEditor(
				docOf(
					bulletList(
						"bl",
						listItem("li1", para("a")),
						listItem("li2", para("b")),
					),
					orderedList("ol", listItem("li3", para("c"))),
				),
			)

			enableGapZones(editor, editor.state.doc.child(0).child(0), 1)

			const states = statesByKey(host)

			// the same list type accepts the item directly, the document level
			// accepts it wrapped in a new bullet list, the ordered list does not
			expect(states["bl:before:li1"]).toBe("auto")
			expect(states["bl:after:li2"]).toBe("auto")
			expect(states["doc:before:bl"]).toBe("auto")
			expect(states["ol:before:li3"]).toBe("none")
			expect(states["ol:after:li3"]).toBe("none")
		})

		it("wraps a list item dragged from outside any list in the default list", ({
			expect,
		}) => {
			const { host, editor } = mountEditor(docOf(para("p1")))
			const orphan = listItem("orphan", para("a"))

			enableGapZones(editor, orphan, 1)

			expect(statesByKey(host)).toEqual({
				"doc:before:p1": "auto",
				"doc:after:p1": "auto",
			})
		})

		it("rejects a heading at a gap the remaining siblings would invalidate", ({
			expect,
		}) => {
			const { host, editor } = mountEditor(
				docOf(
					splitDoc(
						"sd",
						leftSide("ls", heading("h1"), para("p1")),
						rightSide("rs", titledCodeBlock("tc1")),
					),
				),
			)

			enableGapZones(editor, heading("dragged"), 0)

			const states = statesByKey(host)

			// the left side must start with exactly one heading, so a second
			// one is valid neither before nor after the existing one
			expect(states["ls:before:h1"]).toBe("none")
			expect(states["ls:before:p1"]).toBe("none")
			expect(states["doc:before:sd"]).toBe("auto")
		})

		it("skips the gaps touching the dragged metric block and its grid", ({
			expect,
		}) => {
			const { host, editor } = mountEditor(
				docOf(
					para("p0"),
					metricGrid("mg", metricBlock("m1"), metricBlock("m2")),
					para("p1"),
				),
			)

			enableGapZones(editor, editor.state.doc.child(1).child(0), 5)

			expect(statesByKey(host)).toEqual({
				"doc:before:p0": "auto",
				// the gap right before the source grid
				"doc:before:mg": "none",
				// the gaps immediately before and after the dragged block
				"mg:before:m1": "none",
				"mg:before:m2": "none",
				"mg:after:m2": "auto",
				// the gap right after the source grid
				"doc:before:p1": "none",
				"doc:after:p1": "auto",
			})
		})

		it("rejects everything when neither the node nor its wrapper fits", ({
			expect,
		}) => {
			const host = document.createElement("div")
			host.classList.add("pm-host")
			document.body.appendChild(host)

			const view = new EditorView(host, {
				state: EditorState.create({
					doc: wrapperlessSchema.nodes.doc.create(null, [
						wrapperlessSchema.nodes.paragraph.create(
							{ uid: "p1" },
							wrapperlessSchema.text("ab"),
						),
					]),
					plugins: [gapPlugin()],
				}),
			})

			cleanups.push(() => {
				view.destroy()
				host.remove()
			})

			const editor = { state: view.state } as unknown as Editor

			enableGapZones(
				editor,
				wrapperlessSchema.nodes.listItem.create(
					{ uid: "li" },
					wrapperlessSchema.nodes.paragraph.create(
						null,
						wrapperlessSchema.text("a"),
					),
				),
				1,
			)

			expect(statesByKey(host)).toEqual({
				"doc:before:p1": "none",
				"doc:after:p1": "none",
			})

			enableGapZones(
				editor,
				wrapperlessSchema.nodes[METRIC_BLOCK_NAME].create(
					{ uid: "mb" },
					wrapperlessSchema.text("a"),
				),
				1,
			)

			expect(statesByKey(host)).toEqual({
				"doc:before:p1": "none",
				"doc:after:p1": "none",
			})
		})

		it("ignores zones whose key is no longer in the plugin state", ({
			expect,
		}) => {
			const { host, editor } = mountEditor(docOf(para("p1")))
			const stale = gapEl(host, "doc:before:p1")
			stale.setAttribute("data-gap-key", "stale")
			stale.style.pointerEvents = "all"

			enableGapZones(editor, editor.state.doc.child(0), 0)

			expect(stale.style.pointerEvents).toBe("all")
			expect(gapEl(host, "doc:after:p1").style.pointerEvents).toBe("auto")
		})

		it("does nothing when the plugin is not installed", ({ expect }) => {
			const { host } = mountEditor(docOf(para("p1")))
			const doc = docOf(para("p1"))
			const foreign = {
				state: EditorState.create({ doc }),
			} as unknown as Editor

			enableGapZones(foreign, doc.child(0), 0)

			expect(gapEl(host, "doc:before:p1").style.pointerEvents).toBe("none")
		})
	})

	describe("disableGapZones", () => {
		it("turns every gap zone back off", ({ expect }) => {
			const { host, editor } = mountEditor(docOf(para("p1"), para("p2")))

			enableGapZones(editor, editor.state.doc.child(0), 0)
			disableGapZones()

			host.querySelectorAll<HTMLElement>(".pm-gap-zone").forEach((el) => {
				expect(el.style.pointerEvents).toBe("none")
				expect(el.style.backgroundColor).toBe("transparent")
			})
		})
	})

	// the observer callback and the shared scheduler both live outside any
	// single editor, so this drives them through a stubbed constructor
	describe("plugin view", () => {
		it("schedules a reposition when the observed editor element resizes", async ({
			expect,
		}) => {
			// collected rather than assigned to a local: the constructor runs
			// out of the checker's sight, which would leave a plain binding
			// narrowed to its initial value
			const callbacks: (() => void)[] = []
			const observe = vi.fn()
			const disconnect = vi.fn()

			vi.stubGlobal(
				"ResizeObserver",
				class {
					constructor(callback: () => void) {
						callbacks.push(callback)
					}

					observe = observe
					disconnect = disconnect
				},
			)

			const frame = vi.spyOn(globalThis, "requestAnimationFrame")
			const dom = document.createElement("div")
			const view = gapPlugin().spec.view?.({ dom } as never)

			expect(observe).toHaveBeenCalledWith(dom)

			expect(callbacks).toHaveLength(1)

			callbacks[0]?.()

			expect(frame).toHaveBeenCalledOnce()

			view?.destroy?.()

			expect(disconnect).toHaveBeenCalledOnce()

			// drain the frame the notification scheduled so the module's
			// coalescing flag does not stay raised
			await nextFrame()
		})
	})
})
