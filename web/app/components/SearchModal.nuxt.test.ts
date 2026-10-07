import { mockNuxtImport, mountSuspended } from "@nuxt/test-utils/runtime"
import { enableAutoUnmount, flushPromises } from "@vue/test-utils"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import { toast } from "vue-sonner"
import {
	clearQueryCache,
	disposeMockEndpoints,
	mockDeferredEndpoint,
	mockEndpoint,
	runInApp,
	seedQueryData,
} from "~/composables/api/test-helpers"
import { TooltipProvider } from "./shadcn/ui/tooltip"
import SearchModal from "./SearchModal.vue"
import {
	at,
	clearTeleportedOverlays,
	seedAuthOrganization,
	seedAuthSession,
	t,
	teleportedButton,
} from "./test-helpers"

vi.mock("vue-sonner", () => ({
	toast: { custom: vi.fn(), dismiss: vi.fn() },
}))

// the keyboard opens a result by navigating; the destination it picks is
// the whole behaviour, and the test app has no page to land on
const navigateToMock = vi.hoisted(() => vi.fn())
mockNuxtImport("navigateTo", () => navigateToMock)

// happy-dom lays nothing out, so the real scroll watcher would see the list
// as always at its end. The modal's callbacks are driven by hand instead.
const infiniteScroll = vi.hoisted(() => ({
	load: (): unknown => undefined,
	canLoadMore: (): boolean => false,
}))
mockNuxtImport(
	"useInfiniteScroll",
	() =>
		(
			_element: unknown,
			onLoadMore: () => unknown,
			options: { canLoadMore: () => boolean },
		) => {
			infiniteScroll.load = onLoadMore
			infiniteScroll.canLoadMore = options.canLoadMore
		},
)

const DEBOUNCE_MS = 300
const TOOLTIP_DELAY_MS = 300
const SEARCH_URL = "/api/documents/search"
const RECENT_URL = "/api/documents/recent"
const DOC_ID = "doc1".padEnd(20, "0")
const PARENT_ID = "prnt".padEnd(20, "0")
const MAIN_ID = "main".padEnd(20, "0")
const DRAFT_ID = "draft".padEnd(20, "0")
const USER_ID = "user".padEnd(20, "0")
const PARENT_RESULT = {
	id: PARENT_ID,
	title: "Handbook",
	branch: { id: "pmain".padEnd(20, "0"), name: "main", default: true },
}
const BRANCH_SEARCH_URL = `/api/documents/${DOC_ID}/branches/${MAIN_ID}/search`
const VIEWS_URL = `/api/documents/${DOC_ID}/branches/${MAIN_ID}/views`

// the rows' tooltips need the context the app installs at page level. The
// model passes through, so a test drives this as it would the modal.
const ModalUnderTooltips = defineComponent({
	props: { modelValue: { type: Boolean, required: true } },
	emits: ["update:modelValue"],
	setup(props, { emit }) {
		return () =>
			h(TooltipProvider, null, () =>
				h(SearchModal, {
					modelValue: props.modelValue,
					"onUpdate:modelValue": (value: boolean) => {
						emit("update:modelValue", value)
					},
				}),
			)
	},
})

// the dialog body is teleported into the shared <body>, the debounce runs
// on the global fake timers, and the document tree, the organization and
// the editor store are app-wide
describe("<SearchModal>", { concurrent: false }, () => {
	// the modal keeps its search query watching while mounted
	enableAutoUnmount(afterEach)

	beforeEach(() => {
		clearTeleportedOverlays()
		clearQueryCache()
		vi.mocked(toast.custom).mockReset()
		navigateToMock.mockReset()
		openDocument(null, null)
		vi.useFakeTimers()
		vi.setSystemTime(new Date("2026-03-14T12:00:00Z"))
		// the modal asks for the recent pages whenever it opens
		mockRecent([])
	})

	afterEach(disposeMockEndpoints)

	it("stays closed while the model is false", async ({ expect }) => {
		await mountSuspended(ModalUnderTooltips, { props: { modelValue: false } })

		expect(
			document.body.querySelector("[data-slot='dialog-content']"),
		).toBeNull()
	})

	it("invites the user to start typing while the query is empty", async ({
		expect,
	}) => {
		const calls = mockSearch(makePage([]))

		await mountModal()

		expect(dialogText()).toBe(t("sidebar.search.empty-state"))
		expect(calls).toHaveLength(0)
	})

	it("sends the typed query to the search endpoint", async ({ expect }) => {
		const calls = mockSearch(makePage([]))
		await mountModal()

		await search("runbook")

		expect(calls).toHaveLength(1)
		expect(calls[0]?.query).toEqual({ q: "runbook" })
	})

	it("asks for the open document's results first", async ({ expect }) => {
		openDocument(DOC_ID, MAIN_ID)
		const calls = mockSearch(makePage([]))
		await mountModal()

		await search("runbook")

		expect(calls[0]?.query).toEqual({ q: "runbook", currentDocId: DOC_ID })
	})

	it("trims the query before searching", async ({ expect }) => {
		const calls = mockSearch(makePage([]))
		await mountModal()

		await search("  runbook  ")

		expect(calls[0]?.query).toEqual({ q: "runbook" })
	})

	it("does not search while the query is only whitespace", async ({
		expect,
	}) => {
		const calls = mockSearch(makePage([]))
		await mountModal()

		await search("   ")

		expect(calls).toHaveLength(0)
	})

	it.for([
		{ name: "one character", input: "r" },
		{ name: "one character between spaces", input: "  r  " },
	])("does not search a query of $name", async ({ input }, { expect }) => {
		const calls = mockSearch(makePage([]))
		await mountModal()

		await search(input)

		expect(calls).toHaveLength(0)
		expect(dialogText()).toBe(t("sidebar.search.empty-state"))
	})

	it("takes no more characters than a search accepts", async ({ expect }) => {
		await mountModal()

		expect(searchInput().getAttribute("maxlength")).toBe(
			String(DOCUMENT_SEARCH_QUERY_MAX_LENGTH),
		)
	})

	describe("recent pages", { concurrent: false }, () => {
		it("lists the pages opened from search before, the latest first", async ({
			expect,
		}) => {
			disposeMockEndpoints()
			const calls = mockRecent([makeRecent(), makeRecent({ ...PARENT_RESULT })])

			await mountModal()
			await settle()

			expect(dialogText()).toContain(t("sidebar.search.recent"))
			expect(rowTexts()).toEqual([
				expect.stringContaining("Runbook"),
				expect.stringContaining("Handbook"),
			])
			expect(calls).toHaveLength(1)
			expect(calls[0]?.query).toEqual({ from: "search" })
		})

		it("explains a recent page's time in a tooltip", async ({ expect }) => {
			disposeMockEndpoints()
			mockRecent([makeRecent({ viewedAt: "2026-03-14T10:00:00Z" })])
			await mountModal()
			await settle()

			expect(await metaTooltip(0)).toBe(
				t("sidebar.search.viewed-tooltip", {
					date: fullDate("2026-03-14T10:00:00Z"),
				}),
			)
		})

		it("explains in a tooltip where a page from the tree comes from", async ({
			expect,
		}) => {
			seedPages([makeTreePage("a", "Alpha")])
			await mountModal()
			await settle()

			expect(await metaTooltip(0)).toBe(t("sidebar.search.suggested-tooltip"))
		})

		it("says when a page was opened, not who changed it", async ({
			expect,
		}) => {
			disposeMockEndpoints()
			seedAuthSession({ id: USER_ID })
			mockRecent([
				makeRecent({
					updatedBy: USER_ID,
					updatedAt: "2026-03-14T11:59:00Z",
					viewedAt: "2026-03-14T10:00:00Z",
				}),
			])

			await mountModal()
			await settle()

			expect(at(rows(), 0).textContent).toBe(
				`Runbook${t("general.relative-time.hours", { count: 2 })}`,
			)
		})

		it("links a recent page on a branch with that branch selected", async ({
			expect,
		}) => {
			disposeMockEndpoints()
			seedAuthOrganization({ name: "Acme Corp", slug: "acme-corp" })
			seedTree()
			mockRecent([
				makeRecent({
					branch: { id: DRAFT_ID, name: "draft", default: false },
				}),
			])

			await mountModal()
			await settle()

			expect(hrefs()).toEqual([
				`/Acme-Corp/Runbook-${DOC_ID}?branch=${DRAFT_ID}`,
			])
		})

		it("tags the page on screen", async ({ expect }) => {
			disposeMockEndpoints()
			openDocument(DOC_ID, MAIN_ID)
			mockRecent([makeRecent(), makeRecent({ ...PARENT_RESULT })])

			await mountModal()
			await settle()

			expect(
				rows().map((row) =>
					row.textContent.includes(t("sidebar.search.current-page")),
				),
			).toEqual([true, false])
		})

		it("fills a short list with pages from the tree, roots first", async ({
			expect,
		}) => {
			disposeMockEndpoints()
			seedPages([
				makeTreePage("a", "Alpha", [makeTreePage("a1", "Alpha child")]),
				makeTreePage(DOC_ID, "Runbook"),
				makeTreePage("b", "Beta"),
			])
			mockRecent([makeRecent()])

			await mountModal()
			await settle()

			expect(rowTexts()).toEqual([
				expect.stringContaining("Runbook"),
				`Alpha${t("sidebar.search.suggested")}`,
				expect.stringContaining("Beta"),
				expect.stringContaining("Alpha child"),
			])
		})

		it("fills an empty list up to five pages", async ({ expect }) => {
			seedPages(
				["a", "b", "c", "d", "e", "f"].map((id) =>
					makeTreePage(id, `Page ${id}`),
				),
			)

			await mountModal()
			await settle()

			expect(rowTexts()).toEqual([
				expect.stringContaining("Page a"),
				expect.stringContaining("Page b"),
				expect.stringContaining("Page c"),
				expect.stringContaining("Page d"),
				expect.stringContaining("Page e"),
			])
		})

		it("adds no page from the tree to five recent ones", async ({ expect }) => {
			disposeMockEndpoints()
			seedPages([makeTreePage("a", "Alpha")])
			mockRecent(
				["1", "2", "3", "4", "5"].map((id) =>
					makeRecent({
						id: id,
						title: `Recent ${id}`,
						branch: { id: `branch-${id}`, name: "main", default: true },
					}),
				),
			)

			await mountModal()
			await settle()

			expect(rowTexts()).toHaveLength(5)
			expect(dialogText()).not.toContain("Alpha")
		})

		it("opens a page from the tree on its default branch", async ({
			expect,
		}) => {
			seedAuthOrganization({ name: "Acme Corp", slug: "acme-corp" })
			seedPages([makeTreePage(DOC_ID, "Runbook")])
			const calls = mockViews()
			await mountModal()
			await settle()
			const pageHrefs = hrefs()

			at(rows(), 0).click()
			await settle()

			expect(pageHrefs).toEqual([`/Acme-Corp/Runbook-${DOC_ID}`])
			expect(calls).toHaveLength(1)
			expect(calls[0]?.query).toEqual({ from: "search" })
		})

		it("leaves out a page that has no branch yet", async ({ expect }) => {
			seedPages([
				{ ...makeTreePage("a", "Alpha"), defaultBranchId: undefined },
				makeTreePage("b", "Beta"),
			])

			await mountModal()
			await settle()

			expect(rowTexts()).toEqual([expect.stringContaining("Beta")])
		})

		it("waits for the recent pages before it adds any from the tree", async ({
			expect,
		}) => {
			disposeMockEndpoints()
			seedPages([makeTreePage("a", "Alpha")])
			const pending = mockDeferredEndpoint("GET", RECENT_URL)

			await mountModal()
			await pending.reached

			expect(rowTexts()).toEqual([])

			pending.resolve({ results: [] })
			await settle()
			expect(rowTexts()).toEqual([expect.stringContaining("Alpha")])
		})

		it("is loaded before the box is opened", async ({ expect }) => {
			disposeMockEndpoints()
			const calls = mockRecent([makeRecent()])
			const wrapper = await mountSuspended(ModalUnderTooltips, {
				props: { modelValue: false },
			})
			await settle()
			expect(calls).toHaveLength(1)

			await wrapper.setProps({ modelValue: true })
			await nextTick()

			expect(rowTexts()).toEqual([expect.stringContaining("Runbook")])
			expect(calls).toHaveLength(1)
		})

		it("is loaded again in the background once an open is recorded", async ({
			expect,
		}) => {
			disposeMockEndpoints()
			seedTree()
			const calls = mockRecent([makeRecent()])
			mockViews()
			const wrapper = await mountModal()
			await settle()

			at(rows(), 0).click()
			await settle()

			expect(wrapper.emitted("update:modelValue")?.at(-1)).toEqual([false])
			expect(calls).toHaveLength(2)
		})

		it("invites the user to start typing when nothing was opened yet", async ({
			expect,
		}) => {
			await mountModal()
			await settle()

			expect(dialogText()).toBe(t("sidebar.search.empty-state"))
		})

		it("invites the user to start typing when the pages fail to load", async ({
			expect,
		}) => {
			disposeMockEndpoints()
			mockEndpoint("GET", RECENT_URL, () => {
				throw createError({ statusCode: 400 })
			})

			await mountModal()
			await settle()

			expect(dialogText()).toBe(t("sidebar.search.empty-state"))
		})

		it("stays for a query too short to search", async ({ expect }) => {
			disposeMockEndpoints()
			mockRecent([makeRecent()])
			await mountModal()
			await settle()

			await search("r")

			expect(rowTexts()).toEqual([expect.stringContaining("Runbook")])
		})

		it("stays until the first search answers", async ({ expect }) => {
			disposeMockEndpoints()
			mockRecent([makeRecent()])
			const pending = mockDeferredEndpoint("GET", SEARCH_URL)
			await mountModal()
			await settle()

			await search("hand")
			await pending.reached

			expect(rowTexts()).toEqual([expect.stringContaining("Runbook")])

			pending.resolve(makePage([makeResult(PARENT_RESULT)]))
			await settle()
			expect(rowTexts()).toEqual([expect.stringContaining("Handbook")])
		})

		it("comes back when the query is emptied", async ({ expect }) => {
			disposeMockEndpoints()
			mockRecent([makeRecent()])
			mockSearch(makePage([makeResult(PARENT_RESULT)]))
			await mountModal()
			await search("hand")

			await search("")

			expect(rowTexts()).toEqual([expect.stringContaining("Runbook")])
		})

		it("asks for the pages again each time it opens", async ({ expect }) => {
			disposeMockEndpoints()
			const calls = mockRecent([makeRecent()])
			const wrapper = await mountModal()
			await settle()
			await wrapper.setProps({ modelValue: false })
			// the list counts as fresh for a minute
			await vi.advanceTimersByTimeAsync(61_000)

			await wrapper.setProps({ modelValue: true })
			await settle()

			expect(calls).toHaveLength(2)
		})

		it("explains the keys and names the shortcut that toggles the box", async ({
			expect,
		}) => {
			disposeMockEndpoints()
			mockRecent([makeRecent()])

			await mountModal()
			await settle()

			expect(dialogText()).toContain(t("sidebar.search.hints.navigate"))
			expect(dialogText()).toContain(t("sidebar.search.hints.open"))
			expect(dialogText()).not.toContain(t("sidebar.search.hints.new-tab"))
			expect(keyCaps().slice(-2)).toEqual(
				SHORTCUT_ACTIONS.searchForDocuments.keyboardKey.other.split("+"),
			)
		})

		it("starts with none selected and moves onto the first with the down arrow", async ({
			expect,
		}) => {
			disposeMockEndpoints()
			mockRecent([makeRecent(), makeRecent({ ...PARENT_RESULT })])
			await mountModal()
			await settle()
			expect(selectedRows()).toEqual([])

			await press("ArrowDown")

			expect(selectedRows()).toEqual([0])
		})

		it("selects none again when a search gives way to them", async ({
			expect,
		}) => {
			disposeMockEndpoints()
			mockRecent([makeRecent()])
			mockSearch(makePage([makeResult(PARENT_RESULT)]))
			await mountModal()
			await search("hand")
			await press("ArrowDown")
			expect(selectedRows()).toEqual([0])

			await search("")

			expect(selectedRows()).toEqual([])
		})

		it("opens nothing on enter while none is selected", async ({ expect }) => {
			disposeMockEndpoints()
			mockRecent([makeRecent()])
			const wrapper = await mountModal()
			await settle()

			await press("Enter")

			expect(navigateToMock).not.toHaveBeenCalled()
			expect(wrapper.emitted("update:modelValue")).toBeUndefined()
		})
	})

	describe("recording an open", { concurrent: false }, () => {
		it.for([
			{ name: "a page", input: 0 },
			{ name: "a hit", input: 1 },
		])(
			"records it as coming from search when $name is followed",
			async ({ input }, { expect }) => {
				seedTree()
				mockSearch(makePage([makeResult({ hits: [makeHit("h1", "Run it")] })]))
				const views = mockViews()
				await mountModal()
				await search("run")

				at(rows(), input).click()
				await settle()

				expect(views).toHaveLength(1)
				expect(views[0]?.query).toEqual({ from: "search" })
			},
		)

		it("records a recent page that is followed again", async ({ expect }) => {
			disposeMockEndpoints()
			seedTree()
			mockRecent([makeRecent()])
			const views = mockViews()
			const wrapper = await mountModal()
			await settle()

			at(rows(), 0).click()
			await settle()

			expect(views).toHaveLength(1)
			expect(views[0]?.query).toEqual({ from: "search" })
			expect(wrapper.emitted("update:modelValue")?.at(-1)).toEqual([false])
		})

		it("records the row opened with enter", async ({ expect }) => {
			seedTree()
			mockSearch(makePage([makeResult()]))
			const views = mockViews()
			await mountModal()
			await search("run")
			await press("ArrowDown")

			await press("Enter")
			await settle()

			expect(views).toHaveLength(1)
			expect(views[0]?.query).toEqual({ from: "search" })
		})

		it("records a row opened with the middle button, and stays open", async ({
			expect,
		}) => {
			seedTree()
			mockSearch(makePage([makeResult({ hits: [makeHit("h1", "Run it")] })]))
			const views = mockViews()
			const wrapper = await mountModal()
			await search("run")

			for (const row of rows()) {
				row.dispatchEvent(
					new MouseEvent("mouseup", { button: 1, bubbles: true }),
				)
			}
			await settle()

			expect(views).toHaveLength(2)
			expect(wrapper.emitted("update:modelValue")).toBeUndefined()
		})

		it("records nothing for the row that loads more matches", async ({
			expect,
		}) => {
			mockSearch(makePage([makeResultWithMoreHits()]))
			mockEndpoint("GET", BRANCH_SEARCH_URL, () => ({
				totalHits: 2,
				nextToken: null,
				hits: [makeHit("h2", "Second run")],
			}))
			const views = mockViews()
			await mountModal()
			await search("run")

			at(rows(), 2).click()
			await settle()

			expect(views).toHaveLength(0)
		})

		it("still closes when the open cannot be recorded", async ({ expect }) => {
			seedTree()
			mockSearch(makePage([makeResult()]))
			mockEndpoint("POST", VIEWS_URL, () => {
				throw createError({ statusCode: 400 })
			})
			const logged = vi.spyOn(console, "error").mockImplementation(() => {
				// the failure is expected here
			})
			const wrapper = await mountModal()
			await search("run")

			at(rows(), 0).click()
			await settle()

			expect(wrapper.emitted("update:modelValue")?.at(-1)).toEqual([false])
			expect(logged).toHaveBeenCalledTimes(1)
		})
	})

	describe("while a search loads", { concurrent: false }, () => {
		it("shows a spinner in place of the magnifier", async ({ expect }) => {
			mockSearch(makePage([makeResult()]))
			await mountModal()
			await search("run")
			expect(headerIcons()).toEqual(["lucide:search", "lucide:x"])
			disposeMockEndpoints()
			const pending = mockDeferredEndpoint("GET", SEARCH_URL)

			await search("runs")
			await pending.reached

			expect(headerIcons()).toEqual([
				"svg-spinners:blocks-shuffle-3",
				"lucide:x",
			])

			// settle the held request so nothing stays in flight
			pending.resolve(makePage([]))
			await settle()
			expect(headerIcons()).toEqual(["lucide:search", "lucide:x"])
		})

		it("shows the spinner as soon as the query is typed", async ({
			expect,
		}) => {
			mockSearch(makePage([makeResult()]))
			await mountModal()
			await search("run")

			await type("runs")

			expect(headerIcons()).toEqual([
				"svg-spinners:blocks-shuffle-3",
				"lucide:x",
			])
			expect(rowTexts()).toEqual([expect.stringContaining("Runbook")])
		})

		it("keeps the previous results until the next ones arrive", async ({
			expect,
		}) => {
			mockSearch(makePage([makeResult()]))
			await mountModal()
			await search("run")
			disposeMockEndpoints()
			const pending = mockDeferredEndpoint("GET", SEARCH_URL)

			await search("hand")
			await pending.reached

			expect(rowTexts()).toEqual([expect.stringContaining("Runbook")])

			pending.resolve(makePage([makeResult(PARENT_RESULT)]))
			await settle()
			expect(rowTexts()).toEqual([expect.stringContaining("Handbook")])
		})

		it("keeps naming the query that found nothing until the next one answers", async ({
			expect,
		}) => {
			mockSearch(makePage([]))
			await mountModal()
			await search("nothing")
			disposeMockEndpoints()
			const pending = mockDeferredEndpoint("GET", SEARCH_URL)

			await search("other")
			await pending.reached

			expect(dialogText()).toContain(
				t("sidebar.search.no-results", { query: "nothing" }),
			)

			pending.resolve(makePage([]))
			await settle()
			expect(dialogText()).toContain(
				t("sidebar.search.no-results", { query: "other" }),
			)
		})

		it("keeps the invitation to type until the first search answers", async ({
			expect,
		}) => {
			const pending = mockDeferredEndpoint("GET", SEARCH_URL)
			await mountModal()

			await search("run")
			await pending.reached

			expect(dialogText()).toBe(t("sidebar.search.empty-state"))
			expect(headerIcons()).toEqual([
				"svg-spinners:blocks-shuffle-3",
				"lucide:x",
			])

			// settle the held request so nothing stays in flight
			pending.resolve(makePage([]))
			await settle()
		})

		it("does not bring back the results from before the query was emptied", async ({
			expect,
		}) => {
			mockSearch(makePage([makeResult()]))
			await mountModal()
			await search("run")
			await search("")
			disposeMockEndpoints()
			const pending = mockDeferredEndpoint("GET", SEARCH_URL)

			await search("hand")
			await pending.reached

			expect(rows()).toHaveLength(0)

			// settle the held request so nothing stays in flight
			pending.resolve(makePage([]))
			await settle()
		})
	})

	it("reports that nothing matched the query", async ({ expect }) => {
		mockSearch(makePage([]))
		await mountModal()

		await search("nothing")

		expect(dialogText()).toContain(
			t("sidebar.search.no-results", { query: "nothing" }),
		)
		expect(dialogText()).toContain(t("sidebar.search.no-results-description"))
	})

	it("reports no results when the search fails", async ({ expect }) => {
		mockEndpoint("GET", SEARCH_URL, () => {
			throw createError({ statusCode: 400 })
		})
		await mountModal()

		await search("run")

		expect(dialogText()).toContain(
			t("sidebar.search.no-results", { query: "run" }),
		)
	})

	describe("results", { concurrent: false }, () => {
		it("lists each matching page with its hits under it", async ({
			expect,
		}) => {
			mockSearch(
				makePage([
					makeResult({
						hits: [makeHit("h1", "Rollback steps"), makeHit("h2", "Run it")],
					}),
					makeResult(PARENT_RESULT),
				]),
			)
			await mountModal()

			await search("ru")

			expect(rowTexts()).toEqual([
				expect.stringContaining("Runbook"),
				"Rollback steps",
				"Run it",
				expect.stringContaining("Handbook"),
			])
		})

		it("lists a branch the next page repeats only once", async ({ expect }) => {
			mockSearch(makePage([makeResult(), makeResult()]))
			await mountModal()

			await search("run")

			expect(rows()).toHaveLength(1)
		})

		it("writes the workspace and the parent pages under a page", async ({
			expect,
		}) => {
			seedAuthOrganization({ name: "Acme Corp", slug: "acme-corp" })
			seedTree()
			mockSearch(makePage([makeResult()]))
			await mountModal()

			await search("run")

			expect(at(rows(), 0).textContent).toContain(
				["Acme Corp", "Handbook"].join(
					t("sidebar.search.breadcrumb-separator"),
				),
			)
		})

		it("writes the workspace alone for a page the tree does not hold", async ({
			expect,
		}) => {
			seedAuthOrganization({ name: "Acme Corp", slug: "acme-corp" })
			mockSearch(makePage([makeResult()]))
			await mountModal()

			await search("run")

			expect(at(rows(), 0).textContent).toContain("Acme Corp")
			expect(at(rows(), 0).textContent).not.toContain("Handbook")
		})

		it.for([
			{
				name: "says in a tooltip who edited a result and when",
				input: USER_ID,
				expected: () =>
					t("sidebar.search.updated-by-tooltip", {
						user: t("sidebar.search.updated-by-you"),
						date: fullDate("2026-03-14T11:00:00Z"),
					}),
			},
			{
				name: "says in a tooltip when a result with no editor was edited",
				input: null,
				expected: () =>
					t("sidebar.search.updated-tooltip", {
						date: fullDate("2026-03-14T11:00:00Z"),
					}),
			},
		])("$name", async ({ input, expected }, { expect }) => {
			seedAuthSession({ id: USER_ID })
			mockSearch(makePage([makeResult({ updatedBy: input })]))
			await mountModal()
			await search("run")

			expect(await metaTooltip(0)).toBe(expected())
		})

		it.for([
			{
				name: "calls the signed-in user you",
				input: USER_ID,
				expected: () => t("sidebar.search.updated-by-you"),
			},
			{
				name: "names the member who changed the page last",
				input: "member-1",
				expected: () => "Lena",
			},
			{
				name: "calls a member who left the organization a deleted user",
				input: "gone",
				expected: () => t("general.deleted-user"),
			},
		])("$name", async ({ input, expected }, { expect }) => {
			seedAuthSession({ id: USER_ID })
			seedAuthOrganization({
				name: "Acme Corp",
				slug: "acme-corp",
				members: [{ userId: "member-1", user: { name: "Lena" } }],
			})
			mockSearch(
				makePage([
					makeResult({ updatedBy: input, updatedAt: "2026-03-14T10:00:00Z" }),
				]),
			)
			await mountModal()

			await search("run")

			expect(at(rows(), 0).textContent).toContain(
				t("sidebar.search.updated-meta", {
					user: expected(),
					time: t("general.relative-time.hours", { count: 2 }),
				}),
			)
		})

		it.for([
			{
				name: "names both branches of a page that has a draft",
				input: true,
				expected: () => [
					t("general.branch-labels.main-short"),
					t("general.branch-labels.draft-short"),
				],
			},
			{
				name: "leaves the only branch of a page unnamed",
				input: false,
				expected: () => [null, t("general.branch-labels.draft-short")],
			},
		])("$name", async ({ input, expected }, { expect }) => {
			seedTree(input)
			mockSearch(
				makePage([
					makeResult(),
					makeResult({
						branch: { id: DRAFT_ID, name: "draft", default: false },
					}),
				]),
			)
			await mountModal()

			await search("run")

			expect(
				rows().map((row) => {
					const labels = [
						t("general.branch-labels.main-short"),
						t("general.branch-labels.draft-short"),
					]

					return labels.find((label) => row.textContent.includes(label)) ?? null
				}),
			).toEqual(expected())
		})

		it("tags the branch on screen as the current page", async ({ expect }) => {
			openDocument(DOC_ID, DRAFT_ID)
			mockSearch(
				makePage([
					makeResult(),
					makeResult({
						branch: { id: DRAFT_ID, name: "draft", default: false },
					}),
				]),
			)
			await mountModal()

			await search("run")

			expect(
				rows().map((row) =>
					row.textContent.includes(t("sidebar.search.current-page")),
				),
			).toEqual([false, true])
		})
	})

	describe("links", { concurrent: false }, () => {
		it("links a page at its slug under the organization", async ({
			expect,
		}) => {
			seedAuthOrganization({ name: "Acme Corp", slug: "acme-corp" })
			seedTree()
			mockSearch(makePage([makeResult()]))
			await mountModal()

			await search("run")

			expect(hrefs()).toEqual([`/Acme-Corp/Runbook-${DOC_ID}`])
		})

		it("links a hit at its block", async ({ expect }) => {
			seedAuthOrganization({ name: "Acme Corp", slug: "acme-corp" })
			seedTree()
			mockSearch(
				makePage([makeResult({ hits: [makeHit("block 7", "Rollback")] })]),
			)
			await mountModal()

			await search("roll")

			expect(hrefs()).toEqual([
				`/Acme-Corp/Runbook-${DOC_ID}`,
				`/Acme-Corp/Runbook-${DOC_ID}#block%207`,
			])
		})

		it("links a result on another branch with that branch selected", async ({
			expect,
		}) => {
			seedAuthOrganization({ name: "Acme Corp", slug: "acme-corp" })
			seedTree()
			mockSearch(
				makePage([
					makeResult({
						branch: { id: DRAFT_ID, name: "draft", default: false },
						hits: [makeHit("block-7", "Rollback")],
					}),
				]),
			)
			await mountModal()

			await search("roll")

			expect(hrefs()).toEqual([
				`/Acme-Corp/Runbook-${DOC_ID}?branch=${DRAFT_ID}`,
				`/Acme-Corp/Runbook-${DOC_ID}?branch=${DRAFT_ID}#block-7`,
			])
		})

		it("falls back to the bare document id when the tree has no such document", async ({
			expect,
		}) => {
			seedAuthOrganization({ name: "Acme Corp", slug: "acme-corp" })
			mockSearch(makePage([makeResult()]))
			await mountModal()

			await search("run")

			expect(hrefs()).toEqual([`/Acme-Corp/${DOC_ID}`])
		})

		it("drops the organization segment when there is no organization", async ({
			expect,
		}) => {
			seedTree()
			mockSearch(makePage([makeResult()]))
			await mountModal()

			await search("run")

			expect(hrefs()).toEqual([`/Runbook-${DOC_ID}`])
		})
	})

	describe("footer", { concurrent: false }, () => {
		it("counts the matches and the pages they are on", async ({ expect }) => {
			mockSearch(
				makePage([makeResult()], {
					total: { hits: 20, documents: 6, capped: false },
				}),
			)
			await mountModal()

			await search("run")

			expect(dialogText()).toContain(
				t("sidebar.search.total", {
					matches: t("sidebar.search.total-matches", { count: 20 }),
					pages: t("sidebar.search.total-pages", { count: 6 }),
				}),
			)
		})

		it("counts a single match and page in the singular", async ({ expect }) => {
			mockSearch(
				makePage([makeResult()], {
					total: { hits: 1, documents: 1, capped: false },
				}),
			)
			await mountModal()

			await search("run")

			expect(dialogText()).toContain(
				t("sidebar.search.total", {
					matches: t("sidebar.search.total-matches", { count: 1 }),
					pages: t("sidebar.search.total-pages", { count: 1 }),
				}),
			)
		})

		it.for([
			{
				name: "marks a capped count as a lower bound",
				input: 950,
				expected: () => "950",
			},
			{
				name: "shortens a capped count of a thousand",
				input: 1000,
				expected: () => t("sidebar.search.total-thousands", { count: 1 }),
			},
			{
				name: "rounds a capped count down to whole thousands",
				input: 2999,
				expected: () => t("sidebar.search.total-thousands", { count: 2 }),
			},
		])("$name", async ({ input, expected }, { expect }) => {
			mockSearch(
				makePage([makeResult()], {
					total: { hits: input, documents: input, capped: true },
				}),
			)
			await mountModal()

			await search("run")

			expect(dialogText()).toContain(
				t("sidebar.search.total", {
					matches: t("sidebar.search.total-matches-capped", {
						count: expected(),
					}),
					pages: t("sidebar.search.total-pages-capped", {
						count: expected(),
					}),
				}),
			)
		})

		it("explains the keys that move through the results", async ({
			expect,
		}) => {
			mockSearch(makePage([makeResult()]))
			await mountModal()

			await search("run")

			expect(dialogText()).toContain(t("sidebar.search.hints.navigate"))
			expect(dialogText()).toContain(t("sidebar.search.hints.open"))
			expect(dialogText()).toContain(t("sidebar.search.hints.new-tab"))
		})

		it("is left out while there is nothing to count", async ({ expect }) => {
			mockSearch(makePage([]))
			await mountModal()

			await search("run")

			expect(dialogText()).not.toContain(t("sidebar.search.hints.navigate"))
		})
	})

	describe("more matches of a page", { concurrent: false }, () => {
		it("offers the matches the page did not show", async ({ expect }) => {
			mockSearch(makePage([makeResultWithMoreHits()]))
			await mountModal()

			await search("run")

			expect(rowTexts().at(-1)).toBe(
				t("sidebar.search.more-matches", { count: 9 }),
			)
		})

		it("offers nothing more once the page has shown every match", async ({
			expect,
		}) => {
			mockSearch(makePage([makeResult({ hits: [makeHit("h1", "Run it")] })]))
			await mountModal()

			await search("run")

			expect(rows()).toHaveLength(2)
		})

		it("lists the next matches under the ones already there", async ({
			expect,
		}) => {
			mockSearch(makePage([makeResultWithMoreHits()]))
			const moreCalls = mockEndpoint("GET", BRANCH_SEARCH_URL, () => ({
				totalHits: 10,
				nextToken: "hits-3",
				hits: [makeHit("h2", "Second run")],
			}))
			await mountModal()
			await search("run")

			at(rows(), 2).click()
			await settle()

			expect(rowTexts()).toEqual([
				expect.stringContaining("Runbook"),
				"First run",
				"Second run",
				t("sidebar.search.more-matches", { count: 8 }),
			])
			expect(moreCalls).toHaveLength(1)
			expect(moreCalls[0]?.query).toEqual({ q: "run", nextToken: "hits-2" })
		})

		it("continues from the token of the matches it loaded last", async ({
			expect,
		}) => {
			mockSearch(makePage([makeResultWithMoreHits()]))
			const moreCalls = mockEndpoint("GET", BRANCH_SEARCH_URL, (call) =>
				call.query.nextToken === "hits-2"
					? {
							totalHits: 3,
							nextToken: "hits-3",
							hits: [makeHit("h2", "Second run")],
						}
					: {
							totalHits: 3,
							nextToken: null,
							hits: [makeHit("h3", "Third run")],
						},
			)
			await mountModal()
			await search("run")
			at(rows(), 2).click()
			await settle()

			at(rows(), 3).click()
			await settle()

			expect(rowTexts()).toEqual([
				expect.stringContaining("Runbook"),
				"First run",
				"Second run",
				"Third run",
			])
			expect(moreCalls).toHaveLength(2)
			expect(moreCalls[1]?.query).toEqual({ q: "run", nextToken: "hits-3" })
		})

		it("asks once while the next matches are on their way", async ({
			expect,
		}) => {
			mockSearch(makePage([makeResultWithMoreHits()]))
			const more = mockDeferredEndpoint("GET", BRANCH_SEARCH_URL)
			await mountModal()
			await search("run")

			at(rows(), 2).click()
			at(rows(), 2).click()
			await more.reached

			// settle the held request so nothing stays in flight
			more.resolve({ totalHits: 2, nextToken: null, hits: [] })
			await settle()
			expect(more.calls).toHaveLength(1)
		})

		it("drops matches that arrive after the query has changed", async ({
			expect,
		}) => {
			mockSearch(makePage([makeResultWithMoreHits()]))
			const more = mockDeferredEndpoint("GET", BRANCH_SEARCH_URL)
			await mountModal()
			await search("run")
			at(rows(), 2).click()
			await more.reached

			await search("runs")
			more.resolve({
				totalHits: 10,
				nextToken: null,
				hits: [makeHit("h2", "Second run")],
			})
			await settle()

			expect(rowTexts()).not.toContain("Second run")
		})

		it("forgets the loaded matches when the query changes", async ({
			expect,
		}) => {
			mockSearch(makePage([makeResultWithMoreHits()]))
			mockEndpoint("GET", BRANCH_SEARCH_URL, () => ({
				totalHits: 10,
				nextToken: null,
				hits: [makeHit("h2", "Second run")],
			}))
			await mountModal()
			await search("run")
			at(rows(), 2).click()
			await settle()

			await search("runs")

			expect(rowTexts()).not.toContain("Second run")
		})

		it("warns when the next matches fail to load", async ({ expect }) => {
			mockSearch(makePage([makeResultWithMoreHits()]))
			mockEndpoint("GET", BRANCH_SEARCH_URL, () => {
				throw createError({ statusCode: 400 })
			})
			await mountModal()
			await search("run")

			at(rows(), 2).click()
			await settle()

			expect(rows()).toHaveLength(3)
			expect(toast.custom).toHaveBeenCalledTimes(1)
		})
	})

	describe("keyboard", { concurrent: false }, () => {
		it.for([
			{ name: "results", input: null },
			{ name: "results that start with the page on screen", input: DOC_ID },
		])(
			"starts with nothing selected in $name",
			async ({ input }, { expect }) => {
				openDocument(input, input ? MAIN_ID : null)
				mockSearch(makePage([makeResult({ hits: [makeHit("h1", "Run it")] })]))
				await mountModal()

				await search("run")

				expect(selectedRows()).toEqual([])
			},
		)

		it("selects nothing again when new results arrive", async ({ expect }) => {
			mockSearch(makePage([makeResult()]))
			await mountModal()
			await search("run")
			await press("ArrowDown")
			expect(selectedRows()).toEqual([0])

			await search("runs")

			expect(selectedRows()).toEqual([])
		})

		it.for([
			{ name: "the row of the input", input: 2 },
			{ name: "the footer", input: 4 },
		])(
			"selects nothing again on a press in $name",
			async ({ input }, { expect }) => {
				mockSearch(makePage([makeResult()]))
				await mountModal()
				await search("run")
				await press("ArrowDown")
				expect(selectedRows()).toEqual([0])

				const target = at(Array.from(dialog().children), input)
				// under the faked clock the footer only takes a press once
				// the clock has moved on from the moment it was drawn
				await vi.advanceTimersByTimeAsync(1)
				target.dispatchEvent(new PointerEvent("pointerdown", { bubbles: true }))
				await nextTick()

				expect(selectedRows()).toEqual([])
			},
		)

		it.for([
			{
				name: "moves onto the first row with the down arrow",
				input: ["ArrowDown"],
				expected: 0,
			},
			{
				name: "moves down a row with the down arrow",
				input: ["ArrowDown", "ArrowDown"],
				expected: 1,
			},
			{
				name: "moves back up with the up arrow",
				input: ["ArrowDown", "ArrowDown", "ArrowDown", "ArrowUp"],
				expected: 1,
			},
			{
				name: "stays on the first row at the top",
				input: ["ArrowUp"],
				expected: 0,
			},
			{
				name: "stays on the last row at the bottom",
				input: ["ArrowDown", "ArrowDown", "ArrowDown", "ArrowDown"],
				expected: 2,
			},
		])("$name", async ({ input, expected }, { expect }) => {
			mockSearch(makePage([makeResultWithMoreHits()]))
			await mountModal()
			await search("run")

			for (const key of input) {
				await press(key)
			}

			expect(selectedRows()).toEqual([expected])
		})

		it("ignores the arrows while there is nothing to move through", async ({
			expect,
		}) => {
			mockSearch(makePage([]))
			await mountModal()
			await search("run")

			await press("ArrowDown")

			expect(selectedRows()).toEqual([])
		})

		it("selects the row the pointer moves over", async ({ expect }) => {
			mockSearch(makePage([makeResultWithMoreHits()]))
			await mountModal()
			await search("run")

			for (const row of rows()) {
				row.dispatchEvent(new MouseEvent("mousemove", { bubbles: true }))
				await nextTick()
			}

			expect(selectedRows()).toEqual([2])
		})

		it("opens the selected row on enter", async ({ expect }) => {
			seedTree()
			mockSearch(makePage([makeResult({ hits: [makeHit("h1", "Run it")] })]))
			const wrapper = await mountModal()
			await search("run")
			await press("ArrowDown")
			await press("ArrowDown")

			await press("Enter")

			expect(navigateToMock).toHaveBeenCalledExactlyOnceWith(
				`/Runbook-${DOC_ID}#h1`,
				undefined,
			)
			expect(wrapper.emitted("update:modelValue")?.at(-1)).toEqual([false])
		})

		it.for([
			{ name: "the command key", input: { metaKey: true } },
			{ name: "the control key", input: { ctrlKey: true } },
		])(
			"opens the selected row in a new tab with $name held",
			async ({ input }, { expect }) => {
				seedTree()
				mockSearch(makePage([makeResult()]))
				await mountModal()
				await search("run")
				await press("ArrowDown")

				await press("Enter", input)

				expect(navigateToMock).toHaveBeenCalledExactlyOnceWith(
					`/Runbook-${DOC_ID}`,
					{ open: { target: "_blank" } },
				)
			},
		)

		it("loads the next matches on enter when their row is selected", async ({
			expect,
		}) => {
			mockSearch(makePage([makeResultWithMoreHits()]))
			const moreCalls = mockEndpoint("GET", BRANCH_SEARCH_URL, () => ({
				totalHits: 2,
				nextToken: null,
				hits: [makeHit("h2", "Second run")],
			}))
			const wrapper = await mountModal()
			await search("run")
			await press("ArrowDown")
			await press("ArrowDown")
			await press("ArrowDown")

			await press("Enter")
			await settle()

			expect(moreCalls).toHaveLength(1)
			expect(rowTexts()).toContain("Second run")
			expect(navigateToMock).not.toHaveBeenCalled()
			expect(wrapper.emitted("update:modelValue")).toBeUndefined()
		})

		it("does nothing on enter while there are no results", async ({
			expect,
		}) => {
			mockSearch(makePage([]))
			const wrapper = await mountModal()
			await search("run")

			await press("Enter")

			expect(navigateToMock).not.toHaveBeenCalled()
			expect(wrapper.emitted("update:modelValue")).toBeUndefined()
		})
	})

	describe("loading more", { concurrent: false }, () => {
		it.for([
			{ name: "offers more while the last page names a next one", input: "p2" },
			{ name: "offers no more on the last page", input: null },
		])("$name", async ({ input }, { expect }) => {
			mockSearch(makePage([makeResult()], { nextToken: input }))
			await mountModal()

			await search("run")

			expect(infiniteScroll.canLoadMore()).toBe(input !== null)
		})

		it("keeps the first page when the list asks for more as soon as it may", async ({
			expect,
		}) => {
			const calls = mockEndpoint("GET", SEARCH_URL, (call) =>
				call.query.nextToken
					? makePage([makeResult(PARENT_RESULT)])
					: makePage([makeResult()], { nextToken: "p2" }),
			)
			await mountModal()
			// the real watcher loads at once while the list is still short,
			// which it is until the first page has been drawn
			const stop = watch(
				() => infiniteScroll.canLoadMore(),
				(canLoadMore) => {
					if (canLoadMore) {
						void infiniteScroll.load()
					}
				},
				{ flush: "post" },
			)

			await search("run")
			await settle()
			stop()

			expect(calls.map((call) => call.query)).toEqual([
				{ q: "run" },
				{ q: "run", nextToken: "p2" },
			])
		})

		it("keeps the first page of a later search as well", async ({ expect }) => {
			const calls = mockEndpoint("GET", SEARCH_URL, (call) =>
				call.query.nextToken
					? makePage([makeResult(PARENT_RESULT)])
					: makePage([makeResult()], { nextToken: "p2" }),
			)
			await mountModal()
			const stop = watch(
				() => infiniteScroll.canLoadMore(),
				(canLoadMore) => {
					if (canLoadMore) {
						void infiniteScroll.load()
					}
				},
				{ flush: "post" },
			)
			await search("run")
			await settle()
			calls.length = 0

			await search("runs")
			await settle()
			stop()

			expect(calls.map((call) => call.query)).toEqual([
				{ q: "runs" },
				{ q: "runs", nextToken: "p2" },
			])
		})

		it("adds the next page when the list nears its end", async ({ expect }) => {
			const calls = mockEndpoint("GET", SEARCH_URL, (call) =>
				call.query.nextToken
					? makePage([makeResult(PARENT_RESULT)])
					: makePage([makeResult()], { nextToken: "p2" }),
			)
			await mountModal()
			await search("run")

			await loadNextPage()

			expect(rowTexts()).toEqual([
				expect.stringContaining("Runbook"),
				expect.stringContaining("Handbook"),
			])
			expect(calls).toHaveLength(2)
			expect(calls[1]?.query).toEqual({ q: "run", nextToken: "p2" })
			expect(infiniteScroll.canLoadMore()).toBe(false)
		})

		it("stops asking for more once a page fails to load", async ({
			expect,
		}) => {
			// the client retries a GET once on a 5xx, which a 400 keeps out
			// of the count
			const calls = mockEndpoint("GET", SEARCH_URL, (call) => {
				if (call.query.nextToken) {
					throw createError({ statusCode: 400 })
				}

				return makePage([makeResult()], { nextToken: "p2" })
			})
			await mountModal()
			await search("run")

			await loadNextPage()

			expect(infiniteScroll.canLoadMore()).toBe(false)
			expect(rows()).toHaveLength(1)
			expect(calls).toHaveLength(2)
		})

		it("shows a spinner while the next page loads", async ({ expect }) => {
			mockSearch(makePage([makeResult()], { nextToken: "p2" }))
			await mountModal()
			await search("run")
			disposeMockEndpoints()
			const next = mockDeferredEndpoint("GET", SEARCH_URL)

			const loading = infiniteScroll.load()
			await next.reached
			await nextTick()

			expect(dialogIcons()).toContain("svg-spinners:blocks-shuffle-3")

			// settle the held request so nothing stays in flight
			next.resolve(makePage([]))
			await settle()
			await loading
			expect(dialogIcons()).not.toContain("svg-spinners:blocks-shuffle-3")
		})
	})

	describe("closing", { concurrent: false }, () => {
		it.for([
			{ name: "a page", input: 0 },
			{ name: "a hit", input: 1 },
		])("closes when $name is followed", async ({ input }, { expect }) => {
			seedTree()
			mockSearch(makePage([makeResult({ hits: [makeHit("h1", "Run it")] })]))
			const wrapper = await mountModal()
			await search("run")

			at(rows(), input).click()
			await nextTick()

			expect(wrapper.emitted("update:modelValue")?.at(-1)).toEqual([false])
		})

		it("closes on a press outside the modal", async ({ expect }) => {
			const wrapper = await mountModal()
			// the dialog starts listening for outside presses a tick after
			// it opens
			await vi.advanceTimersByTimeAsync(0)

			document.body.dispatchEvent(
				new PointerEvent("pointerdown", { bubbles: true }),
			)
			await vi.advanceTimersByTimeAsync(0)
			await nextTick()

			expect(wrapper.emitted("update:modelValue")?.at(-1)).toEqual([false])
		})

		it("closes on escape", async ({ expect }) => {
			const wrapper = await mountModal()

			await press("Escape")

			expect(wrapper.emitted("update:modelValue")?.at(-1)).toEqual([false])
		})

		it("closes from the button a touch screen gets in place of the key", async ({
			expect,
		}) => {
			const wrapper = await mountModal()

			teleportedButton(t("general.modal-close-screen-reader-hint")).click()
			await nextTick()

			expect(wrapper.emitted("update:modelValue")?.at(-1)).toEqual([false])
		})

		it("forgets the previous query when it is reopened", async ({ expect }) => {
			mockSearch(makePage([makeResult()]))
			const wrapper = await mountModal()
			await search("run")

			await wrapper.setProps({ modelValue: false })
			await wrapper.setProps({ modelValue: true })
			await nextTick()

			expect(dialogText()).toBe(t("sidebar.search.empty-state"))
		})
	})
})

function makeHit(id: string, text: string, type = "paragraph") {
	return { id: id, type: type, text: text }
}

// a result on the main branch of the one document the tree holds
function makeResult(
	overrides: Partial<{
		id: string
		title: string
		branch: { id: string; name: string; default: boolean }
		updatedAt: string
		updatedBy: string | null
		hits: ReturnType<typeof makeHit>[]
		totalHits: number
		nextHitsToken: string | null
	}> = {},
) {
	const hits = overrides.hits ?? []

	return {
		document: {
			id: overrides.id ?? DOC_ID,
			title: overrides.title ?? "Runbook",
			titleHtml: null,
			icon: "mingcute:book-2-line",
			branch: overrides.branch ?? { id: MAIN_ID, name: "main", default: true },
			updatedAt: overrides.updatedAt ?? "2026-03-14T11:00:00Z",
			updatedBy: overrides.updatedBy ?? null,
		},
		hits: hits,
		totalHits: overrides.totalHits ?? hits.length,
		nextHitsToken: overrides.nextHitsToken ?? null,
	}
}

// a result showing one of its ten hits
function makeResultWithMoreHits() {
	return makeResult({
		hits: [makeHit("h1", "First run")],
		totalHits: 10,
		nextHitsToken: "hits-2",
	})
}

function makePage(
	results: ReturnType<typeof makeResult>[],
	overrides: Partial<{
		total: { hits: number; documents: number; capped: boolean }
		nextToken: string | null
	}> = {},
) {
	return {
		total: overrides.total ?? {
			hits: results.reduce((sum, result) => sum + result.totalHits, 0),
			documents: results.length,
			capped: false,
		},
		nextToken: overrides.nextToken ?? null,
		results: results,
	}
}

// a page the user opened from search an hour ago
function makeRecent(
	overrides: Parameters<typeof makeResult>[0] & { viewedAt?: string } = {},
) {
	return {
		...makeResult(overrides).document,
		viewedAt: overrides.viewedAt ?? "2026-03-14T11:00:00Z",
	}
}

function mockRecent(results: ReturnType<typeof makeRecent>[]) {
	return mockEndpoint("GET", RECENT_URL, () => ({ results: results }))
}

function mockViews() {
	return mockEndpoint("POST", VIEWS_URL, () => null)
}

function mockSearch(page: ReturnType<typeof makePage>) {
	return mockEndpoint("GET", SEARCH_URL, () => page)
}

// the document sits under a parent, so its row has a path to write
function seedTree(hasDraft = false) {
	seedQueryData(
		["documents", "tree"],
		[
			{
				id: PARENT_ID,
				documentName: "Handbook",
				icon: "mingcute:book-2-line",
				protected: false,
				children: [
					{
						id: DOC_ID,
						documentName: "Runbook",
						icon: "mingcute:book-2-line",
						// the main branch is protected once a page has a draft
						protected: hasDraft,
						children: null,
					},
				],
			},
		],
	)
}

// a page of the tree, on the main branch every recent page here uses
function makeTreePage(
	id: string,
	documentName: string,
	children: DocumentTreeElement[] = [],
): DocumentTreeElement {
	return {
		id: id.padEnd(20, "0"),
		documentName: documentName,
		icon: "mingcute:book-2-line",
		protected: false,
		defaultBranchId: id === DOC_ID ? MAIN_ID : `branch-${id}`,
		children: children,
	}
}

function seedPages(tree: DocumentTreeElement[]) {
	seedQueryData(["documents", "tree"], tree)
}

function openDocument(documentId: string | null, branchId: string | null) {
	const editorStore = runInApp(() => useEditorStore())

	editorStore.updateActiveDocumentId(documentId)
	editorStore.updateActiveBranchId(branchId)
}

function mountModal() {
	return mountSuspended(ModalUnderTooltips, { props: { modelValue: true } })
}

function dialog() {
	const content = document.body.querySelector<HTMLElement>(
		"[data-slot='dialog-content']",
	)
	if (!content) {
		throw new Error("the search modal is not open")
	}

	return content
}

// what the dialog shows below its header, without the screen reader texts
function dialogText() {
	return Array.from(dialog().children)
		.slice(3)
		.map((section) => section.textContent.trim())
		.join("")
}

// the icons of the row that holds the input
function headerIcons() {
	const header = at(Array.from(dialog().children), 2)

	return Array.from(header.querySelectorAll(".iconify")).flatMap((icon) =>
		Array.from(icon.classList)
			.filter((cls) => cls.startsWith("i-"))
			.map((cls) => cls.slice("i-".length)),
	)
}

// the text of every key cap the dialog draws
function keyCaps() {
	return Array.from(dialog().querySelectorAll("kbd"))
		.filter((cap) => !cap.querySelector("kbd"))
		.map((cap) => cap.textContent.trim())
}

function dialogIcons() {
	return Array.from(dialog().querySelectorAll(".iconify")).flatMap((icon) =>
		Array.from(icon.classList)
			.filter((cls) => cls.startsWith("i-"))
			.map((cls) => cls.slice("i-".length)),
	)
}

// every row the list offers: a page, a hit or the button that loads the
// next hits of a page
function rows() {
	const list = at(Array.from(dialog().children), 3)

	return Array.from(list.querySelectorAll<HTMLElement>("a, button"))
}

function rowTexts() {
	return rows().map((row) => row.textContent.trim())
}

function hrefs() {
	return rows().map((row) => row.getAttribute("href"))
}

function selectedRows() {
	return rows().flatMap((row, index) =>
		row.hasAttribute("data-selected") ? [index] : [],
	)
}

function searchInput() {
	const input = dialog().querySelector<HTMLInputElement>("input")
	if (!input) {
		throw new Error("the search modal has no input")
	}

	return input
}

// types without waiting out the debounce
async function type(query: string) {
	const input = searchInput()
	input.value = query
	input.dispatchEvent(new Event("input", { bubbles: true }))
	await nextTick()
}

// the tooltip a row's note on the right opens once the pointer has
// rested on it
async function metaTooltip(index: number) {
	const trigger = at(rows(), index).querySelector(
		"[data-slot='tooltip-trigger']",
	)

	trigger?.dispatchEvent(
		new PointerEvent("pointermove", { pointerType: "mouse" }),
	)
	await vi.advanceTimersByTimeAsync(TOOLTIP_DELAY_MS)
	await settle()

	const id = trigger?.getAttribute("aria-describedby")

	return id ? document.getElementById(id)?.textContent : undefined
}

function fullDate(date: string) {
	return runInApp(() => useNuxtApp().$i18n.d(new Date(date), "short-with-time"))
}

async function search(query: string) {
	const input = searchInput()
	input.value = query
	input.dispatchEvent(new Event("input", { bubbles: true }))
	await vi.advanceTimersByTimeAsync(DEBOUNCE_MS)
	await flushPromises()
	await nextTick()
}

// the clock is faked for the debounce, and a response only lands once
// the timers of its request have run
async function settle() {
	await vi.advanceTimersByTimeAsync(0)
	await flushPromises()
	await nextTick()
}

async function loadNextPage() {
	const loading = infiniteScroll.load()
	await settle()
	await loading
	await settle()
}

async function press(key: string, modifiers: KeyboardEventInit = {}) {
	searchInput().dispatchEvent(
		new KeyboardEvent("keydown", { key: key, bubbles: true, ...modifiers }),
	)
	await nextTick()
}
