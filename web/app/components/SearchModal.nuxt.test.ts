import { mountSuspended } from "@nuxt/test-utils/runtime"
import { flushPromises } from "@vue/test-utils"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import {
	clearQueryCache,
	disposeMockEndpoints,
	mockEndpoint,
	seedQueryData,
} from "~/composables/api/test-helpers"
import SearchModal from "./SearchModal.vue"
import { clearTeleportedOverlays, seedAuthOrganization } from "./test-helpers"

const DEBOUNCE_MS = 300
const DOC_ID = "doc1".padEnd(20, "0")

function seedTree() {
	seedQueryData(
		["documents", "tree"],
		[
			{
				id: DOC_ID,
				documentName: "Runbook",
				icon: "lucide:file",
				protected: false,
				children: null,
			},
		],
	)
}

function mockSearch(results: unknown) {
	return mockEndpoint("GET", "/api/documents/search", () => results)
}

function mountModal() {
	return mountSuspended(SearchModal, { props: { modelValue: true } })
}

function dialogText() {
	return (
		document.body.querySelector("[data-slot='dialog-content']")?.textContent ??
		""
	)
}

function searchInput() {
	const input = document.body.querySelector<HTMLInputElement>("input")
	if (!input) {
		throw new Error("the search modal has no input")
	}

	return input
}

async function search(query: string) {
	const input = searchInput()
	input.value = query
	input.dispatchEvent(new Event("input", { bubbles: true }))
	await vi.advanceTimersByTimeAsync(DEBOUNCE_MS)
	await flushPromises()
	await nextTick()
}

// the dialog body is teleported into the shared <body>, the debounce runs
// on the global fake timers, and the document tree and organization live
// in the app-wide query cache
describe("<SearchModal>", { concurrent: false }, () => {
	beforeEach(() => {
		clearTeleportedOverlays()
		clearQueryCache()
		vi.useFakeTimers()
	})

	afterEach(disposeMockEndpoints)

	it("stays closed while the model is false", async ({ expect }) => {
		await mountSuspended(SearchModal, { props: { modelValue: false } })

		expect(
			document.body.querySelector("[data-slot='dialog-content']"),
		).toBeNull()
	})

	it("invites the user to start typing while the query is empty", async ({
		expect,
	}) => {
		await mountModal()

		expect(dialogText()).toContain("Start typing to search for pages")
	})

	it("sends the typed query to the search endpoint", async ({ expect }) => {
		const calls = mockSearch([])
		await mountModal()

		await search("runbook")

		expect(calls).toHaveLength(1)
		expect(calls[0]?.query).toEqual({ q: "runbook" })
	})

	it("trims the query before searching", async ({ expect }) => {
		const calls = mockSearch([])
		await mountModal()

		await search("  runbook  ")

		expect(calls[0]?.query).toEqual({ q: "runbook" })
	})

	it("does not search while the query is only whitespace", async ({
		expect,
	}) => {
		const calls = mockSearch([])
		await mountModal()

		await search("   ")

		expect(calls).toHaveLength(0)
	})

	it("reports that nothing matched the query", async ({ expect }) => {
		mockSearch([])
		await mountModal()

		await search("nothing")

		expect(dialogText()).toContain("No results found for")
		expect(dialogText()).toContain("nothing")
	})

	it("lists the matching documents", async ({ expect }) => {
		mockSearch([
			{ id: DOC_ID, documentId: DOC_ID, type: "document", text: "Runbook" },
		])
		await mountModal()

		await search("run")

		expect(dialogText()).toContain("Runbook")
	})

	it("links a document hit at its slug under the organization", async ({
		expect,
	}) => {
		seedAuthOrganization({ name: "Acme Corp", slug: "acme-corp" })
		seedTree()
		mockSearch([
			{ id: DOC_ID, documentId: DOC_ID, type: "document", text: "Runbook" },
		])
		await mountModal()

		await search("run")

		expect(document.body.querySelector("a")?.getAttribute("href")).toBe(
			`/Acme-Corp/Runbook-${DOC_ID}`,
		)
	})

	it("links a hit inside a document at the matching block", async ({
		expect,
	}) => {
		seedAuthOrganization({ name: "Acme Corp", slug: "acme-corp" })
		seedTree()
		mockSearch([
			{
				id: "block-7",
				documentId: DOC_ID,
				type: "heading",
				text: "Rollback steps",
			},
		])
		await mountModal()

		await search("roll")

		expect(document.body.querySelector("a")?.getAttribute("href")).toBe(
			`/Acme-Corp/Runbook-${DOC_ID}#block-7`,
		)
	})

	it("falls back to the bare document id when the tree has no such document", async ({
		expect,
	}) => {
		seedAuthOrganization({ name: "Acme Corp", slug: "acme-corp" })
		mockSearch([
			{ id: DOC_ID, documentId: DOC_ID, type: "document", text: "Runbook" },
		])
		await mountModal()

		await search("run")

		expect(document.body.querySelector("a")?.getAttribute("href")).toBe(
			`/Acme-Corp/${DOC_ID}`,
		)
	})

	it("drops the organization segment when there is no organization", async ({
		expect,
	}) => {
		seedTree()
		mockSearch([
			{ id: DOC_ID, documentId: DOC_ID, type: "document", text: "Runbook" },
		])
		await mountModal()

		await search("run")

		expect(document.body.querySelector("a")?.getAttribute("href")).toBe(
			`/Runbook-${DOC_ID}`,
		)
	})

	it.for([
		{ type: "document", expected: "i-lucide:file-text" },
		{ type: "heading", expected: "i-lucide:heading" },
		{ type: "paragraph", expected: "i-lucide:text" },
	])(
		"marks a $type hit with its own icon",
		async ({ type, expected }, { expect }) => {
			mockSearch([{ id: "x", documentId: DOC_ID, type: type, text: "Hit" }])
			await mountModal()

			await search("hit")

			expect(document.body.querySelector("a")?.innerHTML).toContain(expected)
		},
	)

	it("keeps the result list empty when the search fails", async ({
		expect,
	}) => {
		mockEndpoint("GET", "/api/documents/search", () => {
			throw createError({ statusCode: 500 })
		})
		await mountModal()

		await search("run")

		expect(dialogText()).toContain("No results found for")
	})

	it("closes when a result is followed", async ({ expect }) => {
		seedTree()
		mockSearch([
			{ id: DOC_ID, documentId: DOC_ID, type: "document", text: "Runbook" },
		])
		const wrapper = await mountModal()
		await search("run")

		document.body.querySelector<HTMLElement>("a")?.click()
		await nextTick()

		expect(wrapper.emitted("update:modelValue")?.at(-1)).toEqual([false])
	})

	it("forgets the previous query when it is reopened", async ({ expect }) => {
		mockSearch([])
		const wrapper = await mountModal()
		await search("nothing")

		await wrapper.setProps({ modelValue: false })
		await wrapper.setProps({ modelValue: true })
		await nextTick()

		expect(dialogText()).toContain("Start typing to search for pages")
	})
})
