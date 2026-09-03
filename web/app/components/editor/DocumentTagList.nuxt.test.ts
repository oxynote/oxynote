import { afterEach, beforeEach, describe, it } from "vitest"
import { enableAutoUnmount } from "@vue/test-utils"
import {
	clearQueryCache,
	disposeMockEndpoints,
	makeXid,
	mockEndpoint,
} from "~/composables/api/test-helpers"
import DocumentTagList from "./DocumentTagList.vue"
import TagPill from "./TagPill.vue"
import ColorSelect from "./ColorSelect.vue"
import {
	stubThemeColorContext,
	stubSelectableColors,
} from "./test-helpers/theme"
import {
	clearTeleportedOverlays,
	mountUnderSidebarProvider,
	renderedIconNames,
	settleMutations,
	t,
} from "../test-helpers"

let palette: string[] = []

const DOC_ID = makeXid("doc")
const OTHER_DOC_ID = makeXid("odc")
const TAG_A = makeXid("taga")
const TAG_B = makeXid("tagb")

type TestDocument = ReturnType<typeof makeDoc>

function makeDoc(id: string) {
	return {
		id: id,
		documentName: "Runbook",
		icon: "lucide:file",
		protected: false,
		children: null,
	}
}

function makeTag(
	id: string,
	tagName: string,
	color: string,
	documents: TestDocument[] = [],
) {
	return {
		id: id,
		tagName: tagName,
		color: color,
		hidden: false,
		documents: documents,
	}
}

// the tags a document carries and the ones it does not both come from one
// tree, so each test states the whole thing
function stubTags(tags: unknown[]) {
	return mockEndpoint("GET", "/api/tags/tree", () => tags)
}

async function mountTags() {
	const wrapper = await mountUnderSidebarProvider(DocumentTagList)
	await settleMutations()

	return wrapper
}

async function openPicker(wrapper: Awaited<ReturnType<typeof mountTags>>) {
	await wrapper.get("[data-slot='dropdown-menu-trigger']").trigger("click")
	await settleMutations()
}

// the picker is portalled out of the wrapper, so everything inside it is
// reached through the document rather than through the mount
function pickerItems() {
	return Array.from(
		document.body.querySelectorAll<HTMLElement>(
			"[data-slot='dropdown-menu-content'] [data-slot='dropdown-menu-item']",
		),
	)
}

function pickerRows() {
	return pickerItems().map((el) => el.textContent.trim())
}

// the only button in the menu is the swatch opening the colour picker
function colorTrigger(): HTMLElement {
	const button = document.body.querySelector<HTMLElement>(
		"[data-slot='dropdown-menu-content'] button",
	)
	if (!button) {
		throw new Error("the picker has no colour trigger")
	}

	return button
}

function pickerInput(): HTMLInputElement {
	const input = document.body.querySelector<HTMLInputElement>(
		"[data-slot='dropdown-menu-content'] input",
	)
	if (!input) {
		throw new Error("the picker has no search input")
	}

	return input
}

async function search(text: string) {
	const input = pickerInput()
	input.value = text
	input.dispatchEvent(new Event("input", { bubbles: true }))
	await nextTick()
}

// the picker is teleported into the shared <body> and the editor store is
// an app-wide singleton every mount in the file shares
describe("<DocumentTagList>", { concurrent: false }, () => {
	// the picker keeps a portalled body alive past its test, which would
	// answer the next test's lookups
	enableAutoUnmount(afterEach)

	beforeEach(() => {
		clearQueryCache()
		clearTeleportedOverlays()
		// the picker reads its swatches off the root element and converts
		// them through a canvas, neither of which happy-dom provides
		stubThemeColorContext()
		palette = stubSelectableColors()
		useEditorStore().activeDocumentId = DOC_ID
	})

	afterEach(disposeMockEndpoints)

	it("labels the row", async ({ expect }) => {
		stubTags([])

		const wrapper = await mountTags()

		expect(wrapper.text()).toContain(t("editor.tags.label"))
	})

	it("shows a pill for every tag the document carries", async ({ expect }) => {
		stubTags([
			makeTag(TAG_A, "Production", "#1a9e4a", [makeDoc(DOC_ID)]),
			makeTag(TAG_B, "Staging", "#e8760c", [makeDoc(OTHER_DOC_ID)]),
		])

		const wrapper = await mountTags()

		expect(wrapper.findAllComponents(TagPill).map((p) => p.text())).toEqual([
			"Production",
		])
	})

	it("offers a plus button while the document carries none", async ({
		expect,
	}) => {
		stubTags([makeTag(TAG_A, "Production", "#1a9e4a")])

		const wrapper = await mountTags()

		expect(wrapper.findAllComponents(TagPill)).toHaveLength(0)
		expect(renderedIconNames(wrapper)).toContain("lucide:plus")
	})

	it("collapses past the fourth tag into a counter", async ({ expect }) => {
		stubTags(
			["a", "b", "c", "d", "e"].map((n) =>
				makeTag(makeXid(`t${n}`), `Tag ${n}`, "#1a9e4a", [makeDoc(DOC_ID)]),
			),
		)

		const wrapper = await mountTags()

		expect(wrapper.findAllComponents(TagPill).map((p) => p.text())).toEqual([
			"Tag a",
			"Tag b",
			"Tag c",
			"Tag d",
			t("editor.tags.overflow", { count: 1 }),
		])
	})

	it("ticks the tags the document already carries", async ({ expect }) => {
		stubTags([
			makeTag(TAG_A, "Production", "#1a9e4a", [makeDoc(DOC_ID)]),
			makeTag(TAG_B, "Staging", "#e8760c"),
		])
		const wrapper = await mountTags()

		await openPicker(wrapper)

		const ticks = pickerItems().map(
			(el) => !!el.querySelector(".i-lucide\\:check"),
		)

		expect(ticks).toEqual([true, false])
	})

	it("attaches a tag the document does not carry", async ({ expect }) => {
		stubTags([makeTag(TAG_A, "Production", "#1a9e4a")])
		const calls = mockEndpoint(
			"POST",
			`/api/documents/${DOC_ID}/tags`,
			() => ({}),
		)
		const wrapper = await mountTags()
		await openPicker(wrapper)

		pickerItems()[0]?.click()
		await settleMutations()

		expect(calls).toHaveLength(1)
		expect(calls[0]?.body).toEqual({ tagId: TAG_A })
	})

	it("keeps one trigger element across the first attachment", async ({
		expect,
	}) => {
		// the open popover anchors to the trigger element, so swapping the
		// plus button out for the pills strands the panel in the page corner
		const tree = [makeTag(TAG_A, "Production", "#1a9e4a")]
		stubTags(tree)
		mockEndpoint("POST", `/api/documents/${DOC_ID}/tags`, () => {
			tree[0] = makeTag(TAG_A, "Production", "#1a9e4a", [makeDoc(DOC_ID)])

			return {}
		})
		const wrapper = await mountTags()
		await openPicker(wrapper)
		const trigger = wrapper.get("[data-slot='dropdown-menu-trigger']").element

		pickerItems()[0]?.click()
		await settleMutations()

		expect(wrapper.findAllComponents(TagPill)).toHaveLength(1)
		expect(wrapper.get("[data-slot='dropdown-menu-trigger']").element).toBe(
			trigger,
		)
	})

	it("detaches a tag the document already carries", async ({ expect }) => {
		stubTags([makeTag(TAG_A, "Production", "#1a9e4a", [makeDoc(DOC_ID)])])
		const calls = mockEndpoint(
			"DELETE",
			`/api/documents/${DOC_ID}/tags/${TAG_A}`,
			() => ({}),
		)
		const wrapper = await mountTags()
		await openPicker(wrapper)

		pickerItems()[0]?.click()
		await settleMutations()

		expect(calls).toHaveLength(1)
	})

	it("narrows the list to what was typed", async ({ expect }) => {
		stubTags([
			makeTag(TAG_A, "Production", "#1a9e4a"),
			makeTag(TAG_B, "Staging", "#e8760c"),
		])
		const wrapper = await mountTags()
		await openPicker(wrapper)

		await search("stag")

		expect(pickerRows().filter((row) => row.includes("Staging"))).toHaveLength(
			1,
		)
		expect(pickerRows().filter((row) => row.includes("Production"))).toEqual([])
	})

	it("hints at creation while nothing is typed", async ({ expect }) => {
		stubTags([makeTag(TAG_A, "Production", "#1a9e4a")])
		const wrapper = await mountTags()

		await openPicker(wrapper)

		expect(
			document.body.querySelector("[data-slot='dropdown-menu-content']")
				?.textContent,
		).toContain(t("editor.tags.create-hint"))
	})

	it("creates the typed tag and attaches it in one go", async ({ expect }) => {
		// core answers the refetch that follows a creation with the new tag,
		// which is what the assignment then looks itself up in
		const tree = [makeTag(TAG_A, "Production", "#1a9e4a")]
		stubTags(tree)
		const created = mockEndpoint("POST", "/api/tags", () => {
			tree.push(makeTag(TAG_B, "Rollout", "#000000"))

			return { id: TAG_B }
		})
		const assigned = mockEndpoint(
			"POST",
			`/api/documents/${DOC_ID}/tags`,
			() => ({}),
		)
		const wrapper = await mountTags()
		await openPicker(wrapper)

		await search("Rollout")

		const createRow = pickerItems().find((el) =>
			el.textContent.includes(t("editor.tags.create")),
		)
		createRow?.click()
		await settleMutations()

		// the stubbed canvas reports one flat colour whatever swatch is set
		expect(created[0]?.body).toEqual({
			tagName: "Rollout",
			color: "#000000",
		})
		expect(assigned[0]?.body).toEqual({ tagId: TAG_B })
	})

	it("offers the shared colour picker on the create row", async ({
		expect,
	}) => {
		stubTags([makeTag(TAG_A, "Production", "#1a9e4a")])
		const wrapper = await mountTags()
		await openPicker(wrapper)

		await search("Rollout")

		const swatches = wrapper.findComponent(ColorSelect)
		expect(swatches.exists()).toBe(false)

		colorTrigger().click()
		await nextTick()

		// the swatch popover holds one button per selectable colour
		expect(
			document.body.querySelectorAll("[data-slot='popover-content'] button"),
		).toHaveLength(palette.length)
	})

	it("offers no create row for a name that already exists", async ({
		expect,
	}) => {
		stubTags([makeTag(TAG_A, "Production", "#1a9e4a")])
		const wrapper = await mountTags()
		await openPicker(wrapper)

		await search("production")

		expect(
			pickerRows().filter((row) => row.includes(t("editor.tags.create"))),
		).toEqual([])
	})
})
