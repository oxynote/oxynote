import { format } from "date-fns"
import { mockNuxtImport, mountSuspended } from "@nuxt/test-utils/runtime"
import { enableAutoUnmount, type VueWrapper } from "@vue/test-utils"
import { setResponseStatus } from "h3"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import DataView from "./DataView.vue"
import {
	makeLog,
	logsResult,
	settleLogRequests,
	showRowsUpTo,
} from "../test-helpers"
import { defaultLogConfig, LogTimestampFormat, type LogConfig } from "../utils"
import {
	clearQueryCache,
	disposeMockEndpoints,
	makeXid,
	mockDeferredEndpoint,
	mockEndpoint,
	seedQueryData,
} from "~/composables/api/test-helpers"
import { VARIABLE_QUERY_KEYS } from "~/composables/api/useVariableAPI"
import {
	findButtonByText,
	mountUnderTooltipProvider,
	renderedIconNames,
	t,
} from "~/components/test-helpers"
import { RefreshInterval, TimeRangePreset } from "~/utils/time-range"
import { stubThemeColorContext } from "~/components/editor/test-helpers/theme"

const DATA_SOURCE_ID = makeXid("ds")
const LINES_URL = `/api/data-sources/${DATA_SOURCE_ID}/logs`
const VOLUME_URL = `/api/data-sources/${DATA_SOURCE_ID}/logs/volume`
const DOCUMENT_ID = makeXid("doc")
const BRANCH_ID = makeXid("branch")
const UID = "log-block-1"

// happy-dom lays nothing out, so whether a block is near the viewport is
// set by each test. It starts near.
const visibility = vi.hoisted(() => ({
	near: null as Ref<boolean> | null,
}))

mockNuxtImport("useElementVisibility", () => () => visibility.near ?? ref(true))
const NOW = new Date("2026-01-01T12:00:00.000Z")
const WINDOW = { from: "2026-01-01T11:55:00.000Z", to: NOW.toISOString() }

vi.mock("vue-virtual-scroller", async () =>
	(await import("../test-helpers")).virtualScrollerStub(),
)

// echarts draws the volume onto a canvas happy-dom does not implement;
// the option the strip builds is what these tests read
vi.mock("vue-echarts", async () => {
	const { defineComponent, h } = await import("vue")

	return {
		default: defineComponent({
			name: "VChart",
			props: { option: { type: Object, required: true } },
			setup: () => () => h("div"),
		}),
	}
})

// the view keeps a refresh interval running after its test
enableAutoUnmount(afterEach)

// the editor store, the query cache, the editable flag, the clock and the
// scroll stand-in are shared app-wide, so these tests cannot interleave
describe("<DataView>", { concurrent: false }, () => {
	beforeEach(() => {
		const editorStore = useEditorStore()

		stubThemeColorContext()
		clearQueryCache()
		seedQueryData(VARIABLE_QUERY_KEYS.tree, [])
		useEditorMeta().setEditable(true)
		// leaving the document drops the refresh times the last test stored
		editorStore.updateActiveDocumentId(null)
		editorStore.updateActiveDocumentId(DOCUMENT_ID)
		editorStore.updateActiveBranchId(BRANCH_ID)
		editorStore.setReviewableDiffActive(false)
		visibility.near = ref(true)
		// the view picks how to write its queries from the source's type
		mockEndpoint("GET", "/api/data-sources", () => [
			{ id: DATA_SOURCE_ID, name: "Loki", type: DataSourceType.Loki },
		])
		// the window is read from the clock. Only Date and the refresh
		// interval are faked, so the app's own setup still runs.
		vi.useFakeTimers({
			toFake: ["Date", "setInterval", "clearInterval"],
			now: NOW,
		})
	})

	afterEach(() => {
		vi.useRealTimers()
		disposeMockEndpoints()
	})

	describe("header", () => {
		it("shows the block's title", async ({ expect }) => {
			const wrapper = await mountView(logConfig({ dataSourceId: null }))

			expect(wrapper.get(".text-2base").text()).toBe("API logs")
		})

		it("shows the host's actions", async ({ expect }) => {
			const wrapper = await mountView(logConfig({ dataSourceId: null }), {
				slots: { actions: () => h("button", { class: "host-action" }) },
			})

			expect(wrapper.find(".host-action").exists()).toBe(true)
		})

		it("keeps clear of what the host lays over its right side, drawn closer", async ({
			expect,
		}) => {
			mockEndpoint("GET", LINES_URL, () =>
				logsResult([makeLog("a1", "11:57:00Z")]),
			)
			const wrapper = await mountView(logConfig(), {
				props: { headerRightPadding: 80, compact: true },
			})
			await settleLogRequests()

			const header = wrapper.get(".text-2base").element.parentElement
			expect(header?.style.paddingRight).toBe("calc(5rem + 0.125rem)")
			expect(header?.classList.contains("gap-1")).toBe(false)
			expect(
				wrapper.get("button[aria-pressed]").classes().includes("-mx-0.5"),
			).toBe(true)
		})

		it("keeps its own spacing with nothing laid over it", async ({
			expect,
		}) => {
			mockEndpoint("GET", LINES_URL, () =>
				logsResult([makeLog("a1", "11:57:00Z")]),
			)
			const wrapper = await mountView(logConfig())
			await settleLogRequests()

			const header = wrapper.get(".text-2base").element.parentElement
			expect(header?.style.paddingRight).toBe("")
			expect(header?.classList.contains("gap-1")).toBe(true)
			expect(
				wrapper.get("button[aria-pressed]").classes().includes("-mx-0.5"),
			).toBe(false)
		})

		it("puts the host's marker before the volume toggle and its actions after it, without the search when asked", async ({
			expect,
		}) => {
			mockEndpoint("GET", LINES_URL, () =>
				logsResult([makeLog("a1", "11:57:00Z")]),
			)
			const wrapper = await mountView(logConfig(), {
				props: { hideSearch: true },
				slots: {
					marker: () => h("span", { class: "host-marker" }),
					actions: () => h("button", { class: "host-action" }),
				},
			})
			await settleLogRequests()

			const header = wrapper.get(".text-2base").element.parentElement
			const order = [...(header?.children ?? [])].map((el) =>
				el.classList.contains("host-marker")
					? "marker"
					: el.hasAttribute("aria-pressed")
						? "volume"
						: el.classList.contains("host-action")
							? "actions"
							: null,
			)
			expect(order.filter(Boolean)).toEqual(["marker", "volume", "actions"])
			expect(
				wrapper
					.findAll("button")
					.some((b) => b.text().includes(t("editor.logs.view.search-label"))),
			).toBe(false)
		})

		it.for([
			{ name: "the height it is given", input: "h-12", expected: "h-12" },
			{ name: "its usual height otherwise", input: undefined, expected: "h-8" },
		])("draws its header at $name", async ({ input, expected }, { expect }) => {
			mockEndpoint("GET", LINES_URL, () =>
				logsResult([makeLog("a1", "11:57:00Z")]),
			)
			const wrapper = await mountView(logConfig(), {
				props: { headerClass: input },
			})
			await settleLogRequests()

			const header = wrapper.get(".text-2base").element.parentElement
			expect(header?.classList.contains(expected)).toBe(true)
			expect(header?.classList.contains("h-8")).toBe(expected === "h-8")
		})
	})

	describe("states", () => {
		it("asks for a data source before anything else", async ({ expect }) => {
			const calls = mockEndpoint("GET", LINES_URL, () => logsResult([]))

			const wrapper = await mountView(logConfig({ dataSourceId: null }))
			await settleLogRequests()

			expect(emptyState(wrapper)).toEqual({
				title: t("editor.logs.status.data-source-not-selected.title"),
				description: t(
					"editor.logs.status.data-source-not-selected.description",
				),
			})
			expect(
				findButtonByText(
					wrapper,
					t("editor.logs.status.data-source-not-selected.normal-action-button"),
				).exists(),
			).toBe(true)
			expect(calls).toHaveLength(0)
		})

		it.for([
			{ name: "that was deleted", input: [] },
			{
				name: "whose queries it cannot write",
				input: [
					{
						id: DATA_SOURCE_ID,
						name: "Prometheus",
						type: DataSourceType.Prometheus,
					},
				],
			},
		])(
			"asks for a data source again for one $name",
			async ({ input }, { expect }) => {
				const calls = mockEndpoint("GET", LINES_URL, () => logsResult([]))
				mockEndpoint("GET", "/api/data-sources", () => input)

				const wrapper = await mountView(logConfig())
				await settleLogRequests()

				expect(emptyState(wrapper).title).toBe(
					t("editor.logs.status.data-source-not-selected.title"),
				)
				expect(calls).toHaveLength(0)
			},
		)

		it.for([
			{ name: "without a query", queries: [] },
			{
				name: "while a query is blank",
				queries: [
					{ name: "Query 1", query: '{job="api"}' },
					{ name: "Query 2", query: " " },
				],
			},
		])("asks for a query $name", async ({ queries }, { expect }) => {
			const calls = mockEndpoint("GET", LINES_URL, () => logsResult([]))

			const wrapper = await mountView(logConfig({ queries }))
			await settleLogRequests()

			expect(emptyState(wrapper)).toEqual({
				title: t("editor.logs.status.no-query.title"),
				description: t("editor.logs.status.no-query.description"),
			})
			expect(
				findButtonByText(
					wrapper,
					t("editor.logs.status.no-query.normal-action-button"),
				).exists(),
			).toBe(true)
			expect(calls).toHaveLength(0)
		})

		it("offers only to view the block in read mode", async ({ expect }) => {
			useEditorMeta().setEditable(false)

			const wrapper = await mountView(logConfig({ dataSourceId: null }))

			expect(
				findButtonByText(
					wrapper,
					t(
						"editor.logs.status.data-source-not-selected.readonly-action-button",
					),
				).exists(),
			).toBe(true)
			expect(renderedIconNames(wrapper)).toContain("mingcute:eye-line")
		})

		it("offers only to view the block while a reviewable diff is shown", async ({
			expect,
		}) => {
			useEditorStore().setReviewableDiffActive(true)

			const wrapper = await mountView(logConfig({ queries: [] }))

			expect(
				findButtonByText(
					wrapper,
					t("editor.logs.status.no-query.readonly-action-button"),
				).exists(),
			).toBe(true)
		})

		it("asks the host to configure the block from the action button", async ({
			expect,
		}) => {
			const wrapper = await mountView(logConfig({ dataSourceId: null }))

			await findButtonByText(
				wrapper,
				t("editor.logs.status.data-source-not-selected.normal-action-button"),
			).trigger("click")

			expect(wrapper.emitted("configure")).toEqual([[]])
		})

		it("hides the action button when the host asks it to", async ({
			expect,
		}) => {
			const wrapper = await mountView(logConfig({ dataSourceId: null }), {
				props: { hideEmptyActionButton: true },
			})

			expect(emptyState(wrapper).title).toBe(
				t("editor.logs.status.data-source-not-selected.title"),
			)
			expect(wrapper.findAll("button")).toHaveLength(0)
		})

		it("shows a spinner while the first lines load", async ({
			expect,
			onTestFinished,
		}) => {
			const held = mockDeferredEndpoint("GET", LINES_URL)
			onTestFinished(() => {
				held.resolve(logsResult([]))
			})

			const wrapper = await mountView(logConfig())
			await held.reached

			expect(renderedIconNames(wrapper)).toContain(
				"svg-spinners:blocks-shuffle-3",
			)
			expect(wrapper.find("[data-slot='empty-title']").exists()).toBe(false)
			expect(rowMessages(wrapper)).toEqual([])
			expect(held.calls).toHaveLength(1)
		})

		it("shows the message the data source answered a failed query with", async ({
			expect,
		}) => {
			const calls = mockEndpoint("GET", LINES_URL, (_call, event) => {
				setResponseStatus(event, 400)

				return { code: "query.error", message: "parse error at line 1" }
			})

			const wrapper = await mountView(logConfig())
			await settleLogRequests()

			expect(emptyState(wrapper)).toEqual({
				title: t("editor.logs.status.query-error.title"),
				description: "parse error at line 1",
			})
			expect(calls).toHaveLength(1)
		})

		it("explains a failed query that came without a message", async ({
			expect,
		}) => {
			const calls = mockEndpoint("GET", LINES_URL, (_call, event) => {
				setResponseStatus(event, 400)

				return { code: "query.error" }
			})

			const wrapper = await mountView(logConfig())
			await settleLogRequests()

			expect(emptyState(wrapper)).toEqual({
				title: t("editor.logs.status.query-error.title"),
				description: t("editor.logs.status.query-error.description"),
			})
			expect(calls).toHaveLength(1)
		})

		it("reports that no line matched, without the search or the volume", async ({
			expect,
		}) => {
			const calls = mockEndpoint("GET", LINES_URL, () => logsResult([]))

			const wrapper = await mountView(logConfig())
			await settleLogRequests()

			expect(emptyState(wrapper)).toEqual({
				title: t("editor.logs.status.no-lines.title"),
				description: t("editor.logs.status.no-lines.description"),
			})
			expect(wrapper.findAll("button")).toHaveLength(0)
			expect(wrapper.find(".border-b").exists()).toBe(false)
			expect(calls).toHaveLength(1)
		})
	})

	describe("lines", () => {
		it("reads every query over the block's window", async ({ expect }) => {
			const calls = mockEndpoint("GET", LINES_URL, () => logsResult([]))

			await mountView(
				logConfig({
					queries: [
						{ name: "Query 1", query: '{job="api"}' },
						{ name: "Query 2", query: '{job="db"}' },
					],
				}),
			)
			await settleLogRequests()

			expect(calls).toHaveLength(2)
			expect(calls.map((call) => call.query)).toContainEqual({
				q: '{job="api"}',
				limit: "100",
				...WINDOW,
				order: "desc",
			})
			expect(calls.map((call) => call.query)).toContainEqual({
				q: '{job="db"}',
				limit: "100",
				...WINDOW,
				order: "desc",
			})
		})

		it("shows the lines of every query newest first", async ({ expect }) => {
			mockEndpoint("GET", LINES_URL, (call) =>
				call.query.q === '{job="api"}'
					? logsResult([makeLog("a2", "11:59:00Z"), makeLog("a1", "11:57:00Z")])
					: logsResult([makeLog("d1", "11:58:00Z")]),
			)

			const wrapper = await mountView(
				logConfig({
					queries: [
						{ name: "Query 1", query: '{job="api"}' },
						{ name: "Query 2", query: '{job="db"}' },
					],
				}),
			)
			await settleLogRequests()

			expect(rowMessages(wrapper)).toEqual(["a2", "d1", "a1"])
			expect(rows(wrapper).props("items")).toHaveLength(3)
		})

		it("scrolls back to the top and reads again when the order changes", async ({
			expect,
		}) => {
			const calls = mockEndpoint("GET", LINES_URL, () =>
				logsResult([makeLog("a1", "11:57:00Z")]),
			)
			const wrapper = await mountView(logConfig())
			await settleLogRequests()
			const list = rows(wrapper).element as HTMLElement
			list.scrollTop = 200

			await wrapper.setProps({ config: logConfig({ order: LogOrder.Asc }) })
			await settleLogRequests()

			expect(list.scrollTop).toBe(0)
			expect(calls.map((call) => call.query.order)).toEqual(["desc", "asc"])
		})

		it("reads the oldest lines first when the oldest come first", async ({
			expect,
		}) => {
			const calls = mockEndpoint("GET", LINES_URL, () =>
				logsResult([makeLog("a1", "11:57:00Z"), makeLog("a2", "11:59:00Z")]),
			)

			const wrapper = await mountView(logConfig({ order: LogOrder.Asc }))
			await settleLogRequests()

			expect(calls.map((call) => call.query.order)).toEqual(["asc"])
			expect(rowMessages(wrapper)).toEqual(["a1", "a2"])
			expect(rows(wrapper).classes()).not.toContain("flex-col-reverse")
		})

		it("shows times in the block's format", async ({ expect }) => {
			mockEndpoint("GET", LINES_URL, () =>
				logsResult([makeLog("a1", "11:57:00.123456789Z")]),
			)

			const wrapper = await mountView(
				logConfig({ timestampFormat: LogTimestampFormat.NumericNano }),
			)
			await settleLogRequests()

			expect(wrapper.get("button[aria-expanded] .tabular-nums").text()).toBe(
				`${format(new Date("2026-01-01T11:57:00Z"), "yyyy-MM-dd HH:mm:ss")}.123456789`,
			)
		})

		it("opens a row's details when pressed", async ({ expect }) => {
			mockEndpoint("GET", LINES_URL, () =>
				logsResult([makeLog("a2", "11:59:00Z"), makeLog("a1", "11:57:00Z")]),
			)
			const wrapper = await mountView(logConfig())
			await settleLogRequests()

			await rowButtons(wrapper)[1]?.trigger("click")

			expect(
				rowButtons(wrapper).map((b) => b.attributes("aria-expanded")),
			).toEqual(["false", "true"])
			expect(wrapper.get("h4").text()).toBe(t("editor.logs.view.line-heading"))
		})

		it("closes an open row when pressed again", async ({ expect }) => {
			mockEndpoint("GET", LINES_URL, () =>
				logsResult([makeLog("a1", "11:57:00Z")]),
			)
			const wrapper = await mountView(logConfig())
			await settleLogRequests()
			await rowButtons(wrapper)[0]?.trigger("click")

			await rowButtons(wrapper)[0]?.trigger("click")

			expect(rowButtons(wrapper)[0]?.attributes("aria-expanded")).toBe("false")
			expect(wrapper.find("h4").exists()).toBe(false)
		})
	})

	describe("filters", () => {
		it("narrows every query by a value picked from a row", async ({
			expect,
		}) => {
			const calls = mockEndpoint("GET", LINES_URL, () =>
				logsResult([makeLog("a1", "11:57:00Z")]),
			)
			const wrapper = await mountView(
				logConfig({
					queries: [
						{ name: "Query 1", query: '{job="api"}' },
						{ name: "Query 2", query: '{job="db"}' },
					],
				}),
			)
			await settleLogRequests()
			await rowButtons(wrapper)[0]?.trigger("click")

			await findButtonByText(
				wrapper,
				t("editor.logs.view.include-value", { key: "env" }),
			).trigger("click")
			await settleLogRequests()

			expect(calls).toHaveLength(4)
			expect(calls.slice(2).map((call) => call.query)).toContainEqual({
				q: '{job="api", env="prod"}',
				limit: "100",
				...WINDOW,
				order: "desc",
			})
			expect(calls.slice(2).map((call) => call.query)).toContainEqual({
				q: '{job="db", env="prod"}',
				limit: "100",
				...WINDOW,
				order: "desc",
			})
			expect(chipTexts(wrapper)).toEqual(["env", "=", "prod"])
		})

		it("replaces the filter on a key that already has one", async ({
			expect,
		}) => {
			const calls = mockEndpoint("GET", LINES_URL, () =>
				logsResult([makeLog("a1", "11:57:00Z")]),
			)
			const wrapper = await mountView(logConfig())
			await settleLogRequests()
			await rowButtons(wrapper)[0]?.trigger("click")
			await findButtonByText(
				wrapper,
				t("editor.logs.view.include-value", { key: "env" }),
			).trigger("click")
			await settleLogRequests()

			await findButtonByText(
				wrapper,
				t("editor.logs.view.exclude-value", { key: "env" }),
			).trigger("click")
			await settleLogRequests()

			expect(calls.map((call) => call.query.q)).toEqual([
				'{job="api"}',
				'{job="api", env="prod"}',
				'{job="api", env!="prod"}',
			])
			expect(chipTexts(wrapper)).toEqual(["env", "≠", "prod"])
		})

		it("keeps showing the lines while a narrowed query loads", async ({
			expect,
			onTestFinished,
		}) => {
			let releaseNarrowed: (value: unknown) => void = () => undefined
			const calls = mockEndpoint("GET", LINES_URL, (call) =>
				call.query.q === '{job="api"}'
					? logsResult([makeLog("a1", "11:57:00Z")])
					: new Promise((resolve) => {
							releaseNarrowed = resolve
						}),
			)
			onTestFinished(() => {
				releaseNarrowed(logsResult([]))
			})
			const wrapper = await mountView(logConfig())
			await settleLogRequests()
			await rowButtons(wrapper)[0]?.trigger("click")

			await findButtonByText(
				wrapper,
				t("editor.logs.view.include-value", { key: "env" }),
			).trigger("click")
			await settleLogRequests()

			expect(calls).toHaveLength(2)
			expect(rowMessages(wrapper)).toEqual(["a1"])
			expect(renderedIconNames(wrapper)).not.toContain(
				"svg-spinners:blocks-shuffle-3",
			)
		})
	})

	describe("search", () => {
		it("narrows the query 300ms after the reader stops typing", async ({
			expect,
		}) => {
			const calls = mockEndpoint("GET", LINES_URL, () =>
				logsResult([makeLog("a1", "11:57:00Z")]),
			)
			const wrapper = await mountUnderTooltipProvider(DataView, {
				props: { config: logConfig(), uid: UID },
			})
			await settleLogRequests()
			fakeTimeouts()
			await findButtonByText(
				wrapper,
				t("editor.logs.view.search-label"),
			).trigger("click")
			await wrapper.get("input").setValue("timeout")
			await vi.advanceTimersByTimeAsync(299)
			await settleLogRequests()
			expect(calls).toHaveLength(1)

			await vi.advanceTimersByTimeAsync(1)
			await settleLogRequests()

			// the window is read again when the search changes the query
			expect(calls).toHaveLength(2)
			expect(calls[1]?.query).toEqual({
				q: '{job="api"} |~ "(?i)timeout"',
				limit: "100",
				from: "2026-01-01T11:55:00.300Z",
				to: "2026-01-01T12:00:00.300Z",
				order: "desc",
			})
		})

		it("explains what the search looks at when nothing matched it", async ({
			expect,
		}) => {
			mockEndpoint("GET", LINES_URL, (call) =>
				String(call.query.q).includes("|~")
					? logsResult([])
					: logsResult([makeLog("a1", "11:57:00Z")]),
			)
			const wrapper = await mountUnderTooltipProvider(DataView, {
				props: { config: logConfig(), uid: UID },
			})
			await settleLogRequests()
			fakeTimeouts()
			await findButtonByText(
				wrapper,
				t("editor.logs.view.search-label"),
			).trigger("click")
			await wrapper.get("input").setValue("timeout")
			await vi.advanceTimersByTimeAsync(300)
			await settleLogRequests()

			// the search stays, so the reader can change what emptied the list
			expect(wrapper.find("input").exists()).toBe(true)
			expect(emptyState(wrapper)).toEqual({
				title: t("editor.logs.status.no-lines.title"),
				description: t("editor.logs.status.no-lines.search-description"),
			})
		})

		it("keeps the header's border while a cleared search loads the logs again", async ({
			expect,
		}) => {
			mockEndpoint("GET", LINES_URL, (call) =>
				String(call.query.q).includes("|~")
					? logsResult([])
					: logsResult([makeLog("a1", "11:57:00Z")]),
			)
			const wrapper = await mountUnderTooltipProvider(DataView, {
				props: { config: logConfig(), uid: UID },
			})
			await settleLogRequests()
			fakeTimeouts()
			await findButtonByText(
				wrapper,
				t("editor.logs.view.search-label"),
			).trigger("click")
			await wrapper.get("input").setValue("timeout")
			await vi.advanceTimersByTimeAsync(300)
			await settleLogRequests()

			await wrapper.get("input").setValue("")

			expect(wrapper.find(".border-b.pb-1").exists()).toBe(true)

			await vi.advanceTimersByTimeAsync(300)
			await settleLogRequests()

			expect(wrapper.find(".border-b.pb-1").exists()).toBe(true)
			expect(rowMessages(wrapper)).toEqual(["a1"])
		})
	})

	describe("queries", () => {
		it("reads an edited query 500ms after the last edit", async ({
			expect,
		}) => {
			const calls = mockEndpoint("GET", LINES_URL, () => logsResult([]))
			const wrapper = await mountView(logConfig())
			await settleLogRequests()
			fakeTimeouts()
			await wrapper.setProps({
				config: logConfig({
					queries: [{ name: "Query 1", query: '{job="db"}' }],
				}),
			})
			await vi.advanceTimersByTimeAsync(499)
			await settleLogRequests()
			expect(calls).toHaveLength(1)

			await vi.advanceTimersByTimeAsync(1)
			await settleLogRequests()

			expect(calls.map((call) => call.query.q)).toEqual([
				'{job="api"}',
				'{job="db"}',
			])
		})
	})

	describe("volume", () => {
		it("reads no volume while the strip is hidden", async ({ expect }) => {
			mockEndpoint("GET", LINES_URL, () =>
				logsResult([makeLog("a1", "11:57:00Z")]),
			)
			const volumeCalls = mockEndpoint("GET", VOLUME_URL, () => noVolume())

			const wrapper = await mountView(logConfig())
			await settleLogRequests()

			expect(volumeCalls).toHaveLength(0)
			expect(volumeToggle(wrapper).attributes("aria-pressed")).toBe("false")
			expect(renderedIconNames(wrapper)).toContain(
				"mingcute:layout-top-open-line",
			)
			expect(wrapper.find("[role='img']").exists()).toBe(false)
		})

		it("reads and draws the volume of every query once shown", async ({
			expect,
		}) => {
			mockEndpoint("GET", LINES_URL, () =>
				logsResult([makeLog("a1", "11:57:00Z")]),
			)
			// core sends every bucket of the window, empty ones as zero
			const start = Date.parse(WINDOW.from) / 1000
			const volumeCalls = mockEndpoint("GET", VOLUME_URL, () => ({
				status: GenericQueryResultStatus.Ok,
				data: [
					{
						labels: { level: "error" },
						metrics: [
							[start, 3],
							[start + 15, 0],
							[start + 30, 1],
						],
					},
				],
			}))
			const wrapper = await mountView(logConfig())
			await settleLogRequests()

			await volumeToggle(wrapper).trigger("click")
			await settleLogRequests()

			expect(volumeCalls).toHaveLength(1)
			expect(volumeCalls[0]?.query).toEqual({ q: '{job="api"}', ...WINDOW })
			expect(volumeToggle(wrapper).attributes("aria-pressed")).toBe("true")
			expect(volumeToggle(wrapper).text()).toBe(
				t("editor.logs.view.hide-volume"),
			)
			expect(renderedIconNames(wrapper)).toContain(
				"mingcute:layout-top-close-line",
			)
			expect(volumeSeries(wrapper)).toEqual([
				{ id: "error", data: [3, 0, 1] },
				{ id: "empty", data: [null, 0, null] },
			])
		})

		it("marks every interval of a window without lines as empty", async ({
			expect,
		}) => {
			mockEndpoint("GET", LINES_URL, () =>
				logsResult([makeLog("a1", "11:57:00Z")]),
			)
			// core answers no data, with every bucket of the window as zero
			const start = Date.parse(WINDOW.from) / 1000
			mockEndpoint("GET", VOLUME_URL, () => ({
				status: GenericQueryResultStatus.NoData,
				data: [
					{
						labels: {},
						metrics: [
							[start, 0],
							[start + 15, 0],
							[start + 30, 0],
						],
					},
				],
			}))
			const wrapper = await mountView(logConfig())
			await settleLogRequests()

			await volumeToggle(wrapper).trigger("click")
			await settleLogRequests()

			expect(volumeSeries(wrapper)).toEqual([{ id: "empty", data: [0, 0, 0] }])
		})
	})

	describe("paging", () => {
		it.for([
			{
				order: LogOrder.Desc,
				first: makeLog("a2", "11:59:00Z"),
				next: makeLog("a1", "11:57:00Z"),
			},
			{
				order: LogOrder.Asc,
				first: makeLog("a1", "11:57:00Z"),
				next: makeLog("a2", "11:59:00Z"),
			},
		])(
			"loads the next page at the bottom when the order is $order",
			async ({ order, first, next }, { expect }) => {
				const calls = mockEndpoint("GET", LINES_URL, (call) =>
					call.query.cursor ? logsResult([next]) : logsResult([first], "c1"),
				)
				const wrapper = await mountView(logConfig({ order }))
				await settleLogRequests()

				await showRowsUpTo(wrapper, 0)

				expect(calls).toHaveLength(2)
				expect(calls[1]?.query).toEqual({
					q: '{job="api"}',
					limit: "100",
					...WINDOW,
					order: order,
					cursor: "c1",
				})
				expect(rowMessages(wrapper)).toEqual([first.line, next.line])
			},
		)

		it("loads nothing while the shown rows are far from the last one", async ({
			expect,
		}) => {
			const calls = mockEndpoint("GET", LINES_URL, () =>
				logsResult(
					Array.from({ length: 30 }, (_, i) =>
						makeLog(`a${String(i)}`, `11:${String(59 - i)}:00Z`),
					),
					"c1",
				),
			)
			const wrapper = await mountView(logConfig())
			await settleLogRequests()

			await showRowsUpTo(wrapper, 9)

			expect(calls).toHaveLength(1)
		})

		it("loads nothing more once no query has older lines", async ({
			expect,
		}) => {
			const calls = mockEndpoint("GET", LINES_URL, () =>
				logsResult([makeLog("a2", "11:59:00Z")]),
			)
			const wrapper = await mountView(logConfig())
			await settleLogRequests()

			await showRowsUpTo(wrapper, 0)

			expect(calls).toHaveLength(1)
		})

		it("keeps loading pages while the shown rows reach the end", async ({
			expect,
		}) => {
			const calls = mockEndpoint("GET", LINES_URL, (call) => {
				if (call.query.cursor === "c2") {
					return logsResult([makeLog("a1", "11:57:00Z")])
				}

				return call.query.cursor
					? logsResult([makeLog("a2", "11:58:00Z")], "c2")
					: logsResult([makeLog("a3", "11:59:00Z")], "c1")
			})

			const wrapper = await mountView(logConfig())
			await settleLogRequests()

			await showRowsUpTo(wrapper, 0)

			expect(calls.map((call) => call.query.cursor)).toEqual([
				undefined,
				"c1",
				"c2",
			])
			expect(rowMessages(wrapper)).toEqual(["a3", "a2", "a1"])
		})

		it("stops loading pages after one failed", async ({ expect }) => {
			const calls = mockEndpoint("GET", LINES_URL, (call, event) => {
				if (call.query.cursor) {
					setResponseStatus(event, 400)

					return { code: "cursor.invalid", message: "bad cursor" }
				}

				return logsResult(
					[makeLog("a2", "11:59:00Z"), makeLog("a1", "11:58:00Z")],
					"c1",
				)
			})
			const wrapper = await mountView(logConfig())
			await settleLogRequests()
			await showRowsUpTo(wrapper, 0)

			await showRowsUpTo(wrapper, 1)

			expect(calls.map((call) => call.query.cursor)).toEqual([undefined, "c1"])
		})
	})

	describe("away from the viewport", () => {
		it("reads nothing while the block is more than a screen away, and its first lines once it comes near", async ({
			expect,
		}) => {
			const calls = mockEndpoint("GET", LINES_URL, () =>
				logsResult([makeLog("a1", "11:57:00Z")]),
			)
			setNear(false)
			const wrapper = await mountView(logConfig())
			await settleLogRequests()

			expect(calls).toHaveLength(0)

			setNear(true)
			await settleLogRequests()

			expect(calls).toHaveLength(1)
			expect(rowMessages(wrapper)).toEqual(["a1"])
		})

		it("reads a changed query only once the block comes near", async ({
			expect,
		}) => {
			const calls = mockEndpoint("GET", LINES_URL, () => logsResult([]))
			const wrapper = await mountView(logConfig())
			await settleLogRequests()
			setNear(false)
			fakeTimeouts()
			await wrapper.setProps({
				config: logConfig({
					queries: [{ name: "Query 1", query: '{job="db"}' }],
				}),
			})
			await vi.advanceTimersByTimeAsync(500)
			await settleLogRequests()

			expect(calls).toHaveLength(1)

			setNear(true)
			await settleLogRequests()

			expect(calls.map((call) => call.query.q)).toEqual([
				'{job="api"}',
				'{job="db"}',
			])
		})

		it("reads the volume of a changed query only once the block comes near", async ({
			expect,
		}) => {
			mockEndpoint("GET", LINES_URL, () =>
				logsResult([makeLog("a1", "11:57:00Z")]),
			)
			const volumeCalls = mockEndpoint("GET", VOLUME_URL, () => noVolume())
			const wrapper = await mountView(logConfig())
			await settleLogRequests()
			await volumeToggle(wrapper).trigger("click")
			await settleLogRequests()
			setNear(false)
			fakeTimeouts()
			await wrapper.setProps({
				config: logConfig({
					queries: [{ name: "Query 1", query: '{job="db"}' }],
				}),
			})
			await vi.advanceTimersByTimeAsync(500)
			await settleLogRequests()

			expect(volumeCalls).toHaveLength(1)

			setNear(true)
			await settleLogRequests()

			expect(volumeCalls.map((call) => call.query.q)).toEqual([
				'{job="api"}',
				'{job="db"}',
			])
		})

		it("reads a changed order once, without a refresh on top, when the block comes near after a refresh came due", async ({
			expect,
		}) => {
			const calls = mockEndpoint("GET", LINES_URL, (call) =>
				logsResult([
					makeLog(call.query.order === "asc" ? "asc1" : "desc1", "11:57:00Z"),
				]),
			)
			const wrapper = await mountView(
				logConfig({ refreshInterval: RefreshInterval.S5 }),
			)
			await settleLogRequests()
			setNear(false)
			await wrapper.setProps({
				config: logConfig({
					refreshInterval: RefreshInterval.S5,
					order: LogOrder.Asc,
				}),
			})
			await vi.advanceTimersByTimeAsync(5_000)
			await settleLogRequests()

			setNear(true)
			await settleLogRequests()

			expect(calls.map((call) => call.query.order)).toEqual(["desc", "asc"])
			expect(rowMessages(wrapper)).toEqual(["asc1"])
		})
	})

	describe("refresh", () => {
		it("reads the lines that arrived since once the refresh interval passes", async ({
			expect,
		}) => {
			const calls = mockEndpoint("GET", LINES_URL, () => logsResult([]))
			await mountView(logConfig({ refreshInterval: RefreshInterval.S5 }))
			await settleLogRequests()

			await vi.advanceTimersByTimeAsync(5_000)
			await settleLogRequests()

			expect(calls).toHaveLength(2)
			expect(calls[1]?.query).toEqual({
				q: '{job="api"}',
				limit: "100",
				from: "2026-01-01T11:55:05.000Z",
				to: "2026-01-01T12:00:05.000Z",
				order: "desc",
			})
			expect(useEditorStore().isLogBlockDueForRefresh(UID)).toBe(false)
		})

		it("adds the lines that arrived since, keeping the loaded ones, while scrolled", async ({
			expect,
		}) => {
			const calls = mockEndpoint("GET", LINES_URL, (call) => {
				if (isRefreshRead(call)) {
					return logsResult(
						[makeLog("a4", "12:00:03Z"), makeLog("a3", "11:59:00Z")],
						"r1",
					)
				}

				return call.query.cursor
					? logsResult([makeLog("a1", "11:57:00Z")])
					: logsResult(
							[makeLog("a3", "11:59:00Z"), makeLog("a2", "11:58:00Z")],
							"c1",
						)
			})
			const wrapper = await mountView(
				logConfig({ refreshInterval: RefreshInterval.S5 }),
			)
			await settleLogRequests()
			await showRowsUpTo(wrapper, 1)
			const list = rows(wrapper).element as HTMLElement
			list.scrollTop = 200

			await vi.advanceTimersByTimeAsync(5_000)
			await settleLogRequests()

			expect(calls).toHaveLength(3)
			expect(calls[2]?.query).toEqual({
				q: '{job="api"}',
				limit: "100",
				from: "2026-01-01T11:55:05.000Z",
				to: "2026-01-01T12:00:05.000Z",
				order: "desc",
			})
			expect(rowMessages(wrapper)).toEqual(["a4", "a3", "a2", "a1"])
			// the view keeps the reader's row itself, so the scroller must not
			expect(rows(wrapper).props("shift")).toBe(false)
		})

		it("waits with the new lines while the reader is away from the top, and reads them on return", async ({
			expect,
		}) => {
			const calls = mockEndpoint("GET", LINES_URL, (call) =>
				isRefreshRead(call)
					? logsResult([makeLog("new", "12:00:03Z"), ...logsBefore(NOW, 60)])
					: logsResult(logsBefore(NOW, 60)),
			)
			const wrapper = await mountView(
				logConfig({ refreshInterval: RefreshInterval.S5 }),
			)
			await settleLogRequests()
			await showRowsUpTo(wrapper, 50, 30)

			await vi.advanceTimersByTimeAsync(5_000)
			await settleLogRequests()

			expect(calls).toHaveLength(1)

			await showRowsUpTo(wrapper, 20, 0)

			expect(calls).toHaveLength(2)
			expect(calls[1]?.query.to).toBe("2026-01-01T12:00:05.000Z")
			expect(rowMessages(wrapper)[0]).toBe("new")
		})

		it.for([
			{ name: "shows them at the very top", input: 0, expected: 0 },
			{ name: "keeps the row read below the top", input: 200, expected: 232 },
		])(
			"adds the new lines and $name",
			async ({ input, expected }, { expect }) => {
				mockEndpoint("GET", LINES_URL, (call) =>
					isRefreshRead(call)
						? logsResult([
								makeLog("new", "12:00:03Z"),
								makeLog("a2", "11:58:00Z"),
							])
						: logsResult([makeLog("a2", "11:58:00Z")]),
				)
				const wrapper = await mountView(
					logConfig({ refreshInterval: RefreshInterval.S5 }),
				)
				await settleLogRequests()
				await showRowsUpTo(wrapper, 0)
				const list = rows(wrapper).element as HTMLElement
				list.scrollTop = input

				await vi.advanceTimersByTimeAsync(5_000)
				await settleLogRequests()

				expect(rowMessages(wrapper)).toEqual(["new", "a2"])
				expect(list.scrollTop).toBe(expected)
			},
		)

		it("reads the new lines once a page that was loading arrives", async ({
			expect,
		}) => {
			let release: () => void = () => undefined
			const held = new Promise<void>((resolve) => {
				release = resolve
			})
			const calls = mockEndpoint("GET", LINES_URL, async (call) => {
				if (isRefreshRead(call)) {
					return logsResult(
						[makeLog("new", "12:00:03Z"), makeLog("a2", "11:58:00Z")],
						"r1",
					)
				}

				if (call.query.cursor) {
					await held

					return logsResult([makeLog("a1", "11:57:00Z")])
				}

				return logsResult([makeLog("a2", "11:58:00Z")], "c1")
			})
			const wrapper = await mountView(
				logConfig({ refreshInterval: RefreshInterval.S5 }),
			)
			await settleLogRequests()
			await showRowsUpTo(wrapper, 0)

			await vi.advanceTimersByTimeAsync(5_000)
			await settleLogRequests()

			expect(calls.filter((c) => isRefreshRead(c))).toHaveLength(0)

			release()
			await settleLogRequests()

			expect(calls.filter((c) => isRefreshRead(c))).toHaveLength(1)
			expect(rowMessages(wrapper)).toEqual(["new", "a2", "a1"])
		})

		it("stops refreshing while the block is more than a screen away", async ({
			expect,
		}) => {
			const calls = mockEndpoint("GET", LINES_URL, () => logsResult([]))
			await mountView(logConfig({ refreshInterval: RefreshInterval.S5 }))
			await settleLogRequests()

			setNear(false)
			await vi.advanceTimersByTimeAsync(15_000)
			await settleLogRequests()

			expect(calls).toHaveLength(1)
		})

		it("refreshes as soon as the block comes near again once a refresh is due, then again on the timer's next tick", async ({
			expect,
		}) => {
			const calls = mockEndpoint("GET", LINES_URL, () => logsResult([]))
			await mountView(logConfig({ refreshInterval: RefreshInterval.S5 }))
			await settleLogRequests()
			setNear(false)
			await vi.advanceTimersByTimeAsync(12_000)

			setNear(true)
			await settleLogRequests()

			expect(calls).toHaveLength(2)
			expect(calls[1]?.query.to).toBe("2026-01-01T12:00:12.000Z")

			await vi.advanceTimersByTimeAsync(3_000)
			await settleLogRequests()

			expect(calls).toHaveLength(3)
			expect(calls[2]?.query.to).toBe("2026-01-01T12:00:15.000Z")
		})

		it("waits for the timer's next refresh when the block comes near before one is due", async ({
			expect,
		}) => {
			const calls = mockEndpoint("GET", LINES_URL, () => logsResult([]))
			await mountView(logConfig({ refreshInterval: RefreshInterval.S5 }))
			await settleLogRequests()
			setNear(false)
			await vi.advanceTimersByTimeAsync(2_000)

			setNear(true)
			await settleLogRequests()

			expect(calls).toHaveLength(1)

			await vi.advanceTimersByTimeAsync(3_000)
			await settleLogRequests()

			expect(calls).toHaveLength(2)
			expect(calls[1]?.query.to).toBe("2026-01-01T12:00:05.000Z")
		})

		it("keeps the reader's row in place when lines leave the top, oldest first", async ({
			expect,
		}) => {
			mockEndpoint("GET", LINES_URL, (call) =>
				isRefreshRead(call)
					? logsResult([])
					: logsResult(
							logsBefore(new Date("2026-01-01T11:55:07.600Z"), 30).reverse(),
						),
			)
			const wrapper = await mountView(
				logConfig({
					refreshInterval: RefreshInterval.S5,
					order: LogOrder.Asc,
				}),
			)
			await settleLogRequests()
			await showRowsUpTo(wrapper, 29, 10)
			const anchor = rowMessages(wrapper)[10]

			await vi.advanceTimersByTimeAsync(5_000)
			await settleLogRequests()

			// the lines before 11:55:05 left, so the anchor is 4 rows higher
			expect(rowMessages(wrapper).indexOf(anchor ?? "")).toBe(6)
			expect((rows(wrapper).element as HTMLElement).scrollTop).toBe(6 * 32)
		})

		it("drops lines far above the reader and loads them again on the way back up", async ({
			expect,
		}) => {
			const all = logsBefore(NOW, 5050)
			const first = all.slice(0, 100)
			const second = all.slice(100)
			const calls = mockEndpoint("GET", LINES_URL, (call) => {
				if (call.query.cursor === "back") {
					return {
						...logsResult([...first].reverse(), null),
						previousCursor: "c1",
					}
				}

				return call.query.cursor
					? { ...logsResult(second), previousCursor: "back" }
					: logsResult(first, "c1")
			})
			const wrapper = await mountView(logConfig())
			await settleLogRequests()

			await showRowsUpTo(wrapper, 99, 80)

			expect(rowMessages(wrapper)[0]).toBe(second[0]?.id)

			await showRowsUpTo(wrapper, 30, 0)

			expect(rowMessages(wrapper)[0]).toBe(first[0]?.id)
			expect(calls[2]?.query).toEqual({
				q: '{job="api"}',
				limit: "100",
				...WINDOW,
				cursor: "back",
				order: "asc",
			})
			expect(rows(wrapper).props("items")).toHaveLength(100)
		})

		it("reads the volume again while the strip is shown", async ({
			expect,
		}) => {
			mockEndpoint("GET", LINES_URL, () =>
				logsResult([makeLog("a1", "11:57:00Z")]),
			)
			const volumeCalls = mockEndpoint("GET", VOLUME_URL, () => noVolume())
			const wrapper = await mountView(
				logConfig({ refreshInterval: RefreshInterval.S5 }),
			)
			await settleLogRequests()
			await volumeToggle(wrapper).trigger("click")
			await settleLogRequests()

			await vi.advanceTimersByTimeAsync(5_000)
			await settleLogRequests()

			expect(volumeCalls).toHaveLength(2)
		})

		it("skips the refresh while refreshing is disabled", async ({ expect }) => {
			const calls = mockEndpoint("GET", LINES_URL, () => logsResult([]))
			await mountView(logConfig({ refreshInterval: RefreshInterval.S5 }), {
				props: { disableRefresh: true },
			})
			await settleLogRequests()

			await vi.advanceTimersByTimeAsync(5_000)
			await settleLogRequests()

			expect(calls).toHaveLength(1)
		})

		it("skips the refresh of a block that is not due yet", async ({
			expect,
		}) => {
			// a remounted block finds the time its last mount set
			useEditorStore().setLogBlockNextRefreshTimestamp(UID, 60_000)
			const calls = mockEndpoint("GET", LINES_URL, () => logsResult([]))
			await mountView(logConfig({ refreshInterval: RefreshInterval.S5 }))
			await settleLogRequests()

			await vi.advanceTimersByTimeAsync(5_000)
			await settleLogRequests()

			expect(calls).toHaveLength(1)
		})
	})
})

function setNear(near: boolean) {
	if (visibility.near) {
		visibility.near.value = near
	}
}

// a refresh reads the window the clock has moved on to
function isRefreshRead(call: { query: Record<string, unknown> }): boolean {
	return call.query.to !== WINDOW.to
}

function logConfig(overrides: Partial<LogConfig> = {}): LogConfig {
	return {
		...defaultLogConfig(),
		title: "API logs",
		dataSourceId: DATA_SOURCE_ID,
		queries: [{ name: "Query 1", query: '{job="api"}' }],
		timeRange: TimeRangePreset.Last5Minutes,
		refreshInterval: RefreshInterval.M5,
		...overrides,
	}
}

function mountView(
	config: LogConfig,
	options: {
		props?: Record<string, unknown>
		slots?: Record<string, () => unknown>
	} = {},
) {
	return mountSuspended(DataView, {
		props: { config, uid: UID, ...options.props },
		slots: options.slots,
	})
}

// the debounces run on timeouts, which have to stay real until the view
// is mounted
function fakeTimeouts() {
	vi.useFakeTimers({
		toFake: [
			"Date",
			"setInterval",
			"clearInterval",
			"setTimeout",
			"clearTimeout",
		],
		now: Date.now(),
	})
}

function noVolume() {
	return { status: GenericQueryResultStatus.NoData, data: [] }
}

function emptyState(wrapper: VueWrapper) {
	return {
		title: wrapper.get("[data-slot='empty-title']").text(),
		description: wrapper.get("[data-slot='empty-description']").text(),
	}
}

function rows(wrapper: VueWrapper) {
	return wrapper.findComponent({ name: "DynamicScroller" })
}

// count lines a tenth of a second apart, newest first, the newest a
// tenth of a second before end
function logsBefore(end: Date, count: number): Log[] {
	return Array.from({ length: count }, (_, index) => {
		const time = new Date(end.getTime() - (index + 1) * 100)

		return makeLog(`l${String(index)}`, time.toISOString().slice(11))
	})
}

function rowButtons(wrapper: VueWrapper) {
	return wrapper.findAll("button[aria-expanded]")
}

// each row's message, which is the line itself for these plain lines
function rowMessages(wrapper: VueWrapper): string[] {
	return rowButtons(wrapper).map(
		(b) => b.get(".flex-1.overflow-x-auto").text().split(" · ")[0] ?? "",
	)
}

function chipTexts(wrapper: VueWrapper): string[] {
	return wrapper
		.findAll(".inline-flex.bg-muted > span")
		.map((part) => part.text())
}

function volumeToggle(wrapper: VueWrapper) {
	return wrapper.get("button[aria-pressed]")
}

function volumeSeries(wrapper: VueWrapper) {
	const option = wrapper.findComponent({ name: "VChart" }).props("option") as {
		series: { id: string; data: (number | null)[] }[]
	}

	return option.series.map(({ id, data }) => ({ id, data }))
}
