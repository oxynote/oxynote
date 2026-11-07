import { mountSuspended } from "@nuxt/test-utils/runtime"
import { flushPromises, type VueWrapper } from "@vue/test-utils"
import { afterEach, describe, it, vi } from "vitest"
import RowDetails from "./RowDetails.vue"
import { findButtonByText, t } from "~/components/test-helpers"

// the clipboard is a browser-wide object and the copied label runs on the
// clock, so these tests cannot interleave
describe("<RowDetails>", { concurrent: false }, () => {
	afterEach(() => {
		vi.useRealTimers()
	})

	it("shows the raw line under its heading", async ({ expect }) => {
		const wrapper = await mountDetails(makeLog())

		expect(sectionHeadings(wrapper)).toEqual([
			t("editor.logs.view.line-heading"),
			t("editor.logs.view.indexed-heading"),
			t("editor.logs.view.unindexed-heading"),
		])
		expect(wrapper.get(".whitespace-pre-wrap > span").text()).toBe(
			'level=info msg="deploy finished"',
		)
	})

	it("lists the labels as indexed and the fields as unindexed, by key", async ({
		expect,
	}) => {
		const wrapper = await mountDetails(makeLog())

		expect(tableRows(wrapper)).toEqual([
			[
				["env", "prod"],
				["job", "api"],
			],
			[
				["level", "info"],
				["msg", "deploy finished"],
			],
		])
		expect(wrapper.text()).toContain(t("editor.logs.view.indexed-description"))
		expect(wrapper.text()).toContain(
			t("editor.logs.view.unindexed-description"),
		)
	})

	it("lists the parsed keys with the fields, a parsed key winning over a field of its name", async ({
		expect,
	}) => {
		const wrapper = await mountDetails(
			makeLog({
				fields: { trace_id: "abc", level: "debug" },
				parsed: { msg: "deploy finished", level: "info" },
			}),
		)

		expect(tableRows(wrapper)[1]).toEqual([
			["level", "info"],
			["msg", "deploy finished"],
			["trace_id", "abc"],
		])
	})

	it.for([
		{
			name: "leaves out the indexed table for a line without labels",
			input: { labels: {} },
			expected: ["line-heading", "unindexed-heading"],
		},
		{
			name: "leaves out the unindexed table for a line without fields",
			input: { fields: {} },
			expected: ["line-heading", "indexed-heading"],
		},
	])("$name", async ({ input, expected }, { expect }) => {
		const wrapper = await mountDetails(makeLog(input))

		expect(sectionHeadings(wrapper)).toEqual(
			expected.map((key) => t(`editor.logs.view.${key}`)),
		)
		expect(wrapper.findAll(".divide-y > div")).toHaveLength(2)
	})

	it.for([
		{ table: "indexed", key: "job", value: "api", indexed: true },
		{
			table: "unindexed",
			key: "msg",
			value: "deploy finished",
			indexed: false,
		},
	])(
		"passes on a filter from the $table table",
		async ({ key, value, indexed }, { expect }) => {
			const wrapper = await mountDetails(makeLog())

			await findButtonByText(
				wrapper,
				t("editor.logs.view.exclude-value", { key }),
			).trigger("click")

			expect(wrapper.emitted("filter")).toEqual([
				[{ key, value, indexed, exclude: true }],
			])
		},
	)

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
			props: { log: makeLog() },
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
		fields: { msg: "deploy finished", level: "info" },
		...overrides,
	}
}

function mountDetails(log: Log) {
	return mountSuspended(RowDetails, { props: { log } })
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
