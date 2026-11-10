import { mountSuspended } from "@nuxt/test-utils/runtime"
import { flushPromises, type VueWrapper } from "@vue/test-utils"
import { afterEach, describe, it, vi } from "vitest"
import RowDetails from "./RowDetails.vue"
import { LineFormat } from "../dialect"
import { lokiDialect } from "../loki"
import {
	findButtonByText,
	mountUnderTooltipProvider,
	openTooltipText,
	t,
} from "~/components/test-helpers"

// the clipboard is a browser-wide object and the copied label runs on the
// clock, so these tests cannot interleave
describe("<RowDetails>", { concurrent: false }, () => {
	afterEach(() => {
		vi.useRealTimers()
	})

	it("shows the raw line, then the origin, metadata and content fields", async ({
		expect,
	}) => {
		const wrapper = await mountDetails(makeLog())

		expect(sectionHeadings(wrapper)).toEqual([
			t("editor.logs.view.line-heading"),
			t("editor.logs.view.origin-heading"),
			t("editor.logs.view.metadata-heading"),
			t("editor.logs.view.content-heading"),
		])
		expect(wrapper.get(".whitespace-pre-wrap > span").text()).toBe(
			'level=info msg="deploy finished"',
		)
	})

	it("lists each kind of field in its own table, by key", async ({
		expect,
	}) => {
		const wrapper = await mountDetails(makeLog())

		expect(tableRows(wrapper)).toEqual([
			[
				["env", "prod"],
				["job", "api"],
			],
			[["trace_id", "abc"]],
			[
				["level", "info"],
				["msg", "deploy finished"],
			],
		])
	})

	it("describes each table in a note that narrow blocks hide", async ({
		expect,
	}) => {
		const wrapper = await mountDetails(makeLog())
		const notes = wrapper.findAll("h4 + span")

		expect(notes.map((note) => note.text())).toEqual([
			t("editor.logs.view.origin-description"),
			t("editor.logs.view.metadata-description"),
			t("editor.logs.view.content-description"),
		])
		expect(notes.every((note) => note.classes().includes("hidden"))).toBe(true)
	})

	it("says none in a table without entries", async ({ expect }) => {
		const wrapper = await mountDetails(
			makeLog({ line: "GET /cart", parsed: undefined, labels: {}, fields: {} }),
		)

		expect(wrapper.findAll(".divide-y").map((table) => table.text())).toEqual([
			t("editor.logs.view.section-empty"),
			t("editor.logs.view.section-empty"),
			t("editor.logs.view.section-empty"),
		])
	})

	it("says none for a line of only white space", async ({ expect }) => {
		const wrapper = await mountDetails(makeLog({ line: "  " }))

		expect(wrapper.get(".whitespace-pre-wrap > span").text()).toBe(
			t("editor.logs.view.section-empty"),
		)
	})

	it.for([
		{
			name: "passes on an origin field filter",
			input: { key: "job", log: {} },
			expected: { key: "job", value: "api", indexed: true },
		},
		{
			name: "passes on a metadata field filter",
			input: { key: "trace_id", log: {} },
			expected: { key: "trace_id", value: "abc", indexed: false },
		},
		{
			name: "passes on a filter for a key the query's parser added",
			input: {
				key: "region",
				log: { line: "GET /cart", parsed: { region: "eu" } },
			},
			expected: { key: "region", value: "eu", indexed: false },
		},
		{
			name: "passes on a filter for a line key with the parser it needs",
			input: { key: "msg", log: {} },
			expected: {
				key: "msg",
				value: "deploy finished",
				indexed: false,
				format: LineFormat.Logfmt,
			},
		},
		{
			name: "names a line key the way Loki does",
			input: {
				key: "job",
				log: { line: '{"job":"x"}', parsed: undefined },
			},
			expected: {
				key: "job_extracted",
				value: "x",
				indexed: false,
				format: LineFormat.JSON,
			},
		},
		// the filter is compared to the line as written, so a number has to
		// reach it as the line wrote it
		{
			name: "passes on a JSON number as the line wrote it",
			input: {
				key: "duration_ms",
				log: {
					line: '{"msg":"order placed","duration_ms":25.0}',
					parsed: undefined,
				},
			},
			expected: {
				key: "duration_ms",
				value: "25.0",
				indexed: false,
				format: LineFormat.JSON,
			},
		},
	])("$name", async ({ input, expected }, { expect }) => {
		const wrapper = await mountDetails(makeLog(input.log))

		// a line key can share its name with a label, so the button is the
		// last of its name, in the extracted table
		const buttons = wrapper
			.findAll("button")
			.filter(
				(b) =>
					b.text() === t("editor.logs.view.exclude-value", { key: input.key }),
			)
		await buttons.at(-1)?.trigger("click")

		expect(wrapper.emitted("filter")).toEqual([
			[{ ...expected, exclude: true }],
		])
	})

	it.for([
		{
			name: "turns off the filter of a line key the query cannot filter",
			input: { log: {}, formats: [LineFormat.JSON] },
			expected: "editor.logs.view.filter-query-blocked",
		},
		{
			name: "turns off the filter of a nested line value",
			input: {
				log: { line: '{"user":{"id":1}}', parsed: undefined },
				formats: [LineFormat.JSON],
			},
			expected: "editor.logs.view.filter-nested",
		},
		{
			name: "turns off the filter of a null line value",
			input: {
				log: { line: '{"status":null}', parsed: undefined },
				formats: [LineFormat.JSON],
			},
			expected: "editor.logs.view.filter-null",
		},
	])("$name", async ({ input, expected }, { expect }) => {
		const wrapper = await mountUnderTooltipProvider(RowDetails, {
			props: {
				log: makeLog(input.log),
				dialect: lokiDialect,
				lineFilterFormats: input.formats,
			},
		})
		const trigger = wrapper.get(".divide-y [data-slot='tooltip-trigger']")

		expect(
			trigger
				.findAll("button")
				.every((b) => b.attributes("disabled") !== undefined),
		).toBe(true)

		await trigger.trigger("focus")
		await flushPromises()

		expect(openTooltipText(wrapper)).toBe(t(expected))
	})

	it("copies the raw line and says so", async ({ expect }) => {
		const writeText = vi
			.spyOn(navigator.clipboard, "writeText")
			.mockResolvedValue(undefined)
		const wrapper = await mountDetails(makeLog())

		await findButtonByText(wrapper, t("editor.logs.view.copy-line")).trigger(
			"click",
		)
		await flushPromises()

		expect(writeText).toHaveBeenCalledTimes(1)
		expect(writeText).toHaveBeenCalledWith('level=info msg="deploy finished"')
		expect(copyButton(wrapper).text()).toBe(t("editor.logs.view.line-copied"))
	})

	it("goes back to the copy label 700ms after copying", async ({ expect }) => {
		vi.spyOn(navigator.clipboard, "writeText").mockResolvedValue(undefined)
		const wrapper = await mountDetails(makeLog())
		vi.useFakeTimers()
		await findButtonByText(wrapper, t("editor.logs.view.copy-line")).trigger(
			"click",
		)
		await flushPromises()
		await vi.advanceTimersByTimeAsync(699)
		expect(copyButton(wrapper).text()).toBe(t("editor.logs.view.line-copied"))

		await vi.advanceTimersByTimeAsync(1)

		expect(copyButton(wrapper).text()).toBe(t("editor.logs.view.copy-line"))
	})

	it("keeps the copy label when the clipboard refuses", async ({ expect }) => {
		const writeText = vi
			.spyOn(navigator.clipboard, "writeText")
			.mockRejectedValue(new Error("denied"))
		const errorHandler = vi.fn()
		const wrapper = await mountSuspended(RowDetails, {
			props: {
				log: makeLog(),
				dialect: lokiDialect,
				lineFilterFormats: Object.values(LineFormat),
			},
			global: { config: { errorHandler } },
		})

		await findButtonByText(wrapper, t("editor.logs.view.copy-line")).trigger(
			"click",
		)
		await flushPromises()

		expect(copyButton(wrapper).text()).toBe(t("editor.logs.view.copy-line"))
		expect(writeText).toHaveBeenCalledTimes(1)
		expect(errorHandler).not.toHaveBeenCalled()
	})
})

function makeLog(overrides: Partial<Log> = {}): Log {
	return {
		id: "l1",
		time: "2026-01-01T12:00:00Z",
		level: LogLevel.Info,
		line: 'level=info msg="deploy finished"',
		labels: { job: "api", env: "prod" },
		fields: { trace_id: "abc" },
		parsed: { msg: "deploy finished", level: "info" },
		...overrides,
	}
}

// every line format can be filtered, so no row needs a tooltip provider
function mountDetails(log: Log) {
	return mountSuspended(RowDetails, {
		props: {
			log,
			dialect: lokiDialect,
			lineFilterFormats: Object.values(LineFormat),
		},
	})
}

function sectionHeadings(wrapper: VueWrapper): string[] {
	return wrapper.findAll("h4").map((h) => h.text())
}

// each table's rows as key and value pairs
function tableRows(wrapper: VueWrapper): string[][][] {
	return wrapper
		.findAll(".divide-y")
		.map((table) =>
			table
				.findAll(":scope > div")
				.map((row) => row.findAll(".truncate").map((cell) => cell.text())),
		)
}

function copyButton(wrapper: VueWrapper) {
	return wrapper.get("section button")
}
