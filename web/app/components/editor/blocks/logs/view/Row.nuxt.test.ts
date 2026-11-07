import { mountSuspended } from "@nuxt/test-utils/runtime"
import { flushPromises, type VueWrapper } from "@vue/test-utils"
import { describe, it } from "vitest"
import Row from "./Row.vue"
import { LineFormat } from "../dialect"
import { lokiDialect } from "../loki"
import { LogTimestampFormat } from "../utils"
import {
	findButtonByText,
	mountUnderTooltipProvider,
	t,
} from "~/components/test-helpers"

// built from local fields, so every format reads the same clock in any
// time zone
const LOCAL_TIME = new Date(2026, 0, 2, 3, 4, 5).toISOString()

describe("<Row>", () => {
	it.for([
		{
			name: "shows the date and time of the readable format apart",
			input: { time: LOCAL_TIME, format: LogTimestampFormat.Readable },
			expected: "Jan 2, 2026 03:04:05 AM",
		},
		{
			name: "shows only the time of a readable time-only format",
			input: { time: LOCAL_TIME, format: LogTimestampFormat.ReadableTime },
			expected: "03:04:05 AM",
		},
		{
			name: "shows the date and time of a numeric format apart",
			input: {
				time: LOCAL_TIME.replace(".000Z", ".123456789Z"),
				format: LogTimestampFormat.NumericNano,
			},
			expected: "2026-01-02 03:04:05.123456789",
		},
	])("$name", async ({ input, expected }, { expect }) => {
		const wrapper = await mountRow({
			log: makeLog({ time: input.time }),
			timestampFormat: input.format,
		})

		// newer ICU versions put a narrow no-break space before AM and PM
		expect(
			wrapper.get("button .tabular-nums").text().replace(/\s/gu, " "),
		).toBe(expected)
	})

	it.for([
		{
			name: "shows no time when the format hides it",
			input: { time: LOCAL_TIME, format: LogTimestampFormat.None },
		},
		{
			name: "shows no time when the time is unknown",
			input: {
				time: "0001-01-01T00:00:00Z",
				format: LogTimestampFormat.Numeric,
			},
		},
	])("$name", async ({ input }, { expect }) => {
		const wrapper = await mountRow({
			log: makeLog({ time: input.time }),
			timestampFormat: input.format,
		})

		expect(wrapper.find("button .tabular-nums").exists()).toBe(false)
	})

	it("shows the line's level", async ({ expect }) => {
		const wrapper = await mountRow({
			log: makeLog({ level: LogLevel.Error }),
		})

		expect(wrapper.get("button .rounded-full").text()).toBe(
			t("editor.logs.levels.error"),
		)
	})

	it.for<{
		name: string
		input: Pick<Log, "line" | "labels"> &
			Partial<Pick<Log, "level" | "fields" | "parsed">>
		expected: string
	}>([
		{
			name: "splits a logfmt line and shows the extra parser keys with its keys",
			input: {
				level: LogLevel.Error,
				line: 'level=error msg="value out of range" value=+Inf',
				labels: { app: "chaos" },
				fields: { trace_id: "abc" },
				parsed: {
					level: "error",
					msg: "value out of range",
					value: "+Inf",
					unit: "ms",
				},
			},
			expected:
				"value out of range · value=+Inf unit=ms · trace_id=abc · app=chaos",
		},
		{
			name: "starts with the labels of a line of only white space",
			input: { line: "  ", labels: { app: "chaos" } },
			expected: "app=chaos",
		},
		{
			name: "sums up a line by its message, fields and labels",
			input: {
				line: '{"msg":"done","status":"200","user":"ada lovelace"}',
				labels: { job: "api" },
			},
			expected: 'done · status=200 user="ada lovelace" · job=api',
		},
		{
			name: "starts with the fields of a line without a message",
			input: { line: '{"status":"200"}', labels: { job: "api" } },
			expected: "status=200 · job=api",
		},
		{
			name: "shows only the message of a line without fields or labels",
			input: { line: "deploy finished", labels: {} },
			expected: "deploy finished",
		},
	])("$name", async ({ input, expected }, { expect }) => {
		const wrapper = await mountRow({ log: makeLog(input) })

		expect(summary(wrapper).text()).toBe(expected)
	})

	it.for<{
		name: string
		input: Pick<Log, "line" | "labels" | "fields"> &
			Partial<Pick<Log, "parsed">>
		expected: string[]
	}>([
		{
			name: "marks the search in the message and the fields of a JSON line",
			input: {
				line: '{"msg":"Ada signed in","user":"ada"}',
				labels: {},
				fields: {},
			},
			expected: ["Ada", "ada"],
		},
		{
			name: "leaves the labels unmarked, since the search reads only the line",
			input: {
				line: '{"msg":"Ada signed in"}',
				labels: { user: "ada" },
				fields: {},
			},
			expected: ["Ada"],
		},
		{
			name: "leaves the fields carried beside a JSON line unmarked",
			input: {
				line: '{"msg":"Ada signed in"}',
				labels: {},
				fields: { user: "ada" },
			},
			expected: ["Ada"],
		},
		{
			name: "marks the search in the keys a parser took from a plain line",
			input: {
				line: "Ada signed in",
				labels: {},
				fields: {},
				parsed: { user: "ada" },
			},
			expected: ["Ada", "ada"],
		},
		{
			name: "leaves the fields carried beside a plain line unmarked",
			input: {
				line: "Ada signed in",
				labels: {},
				fields: { user: "ada" },
			},
			expected: ["Ada"],
		},
	])("$name", async ({ input, expected }, { expect }) => {
		const wrapper = await mountUnderTooltipProvider(Row, {
			props: rowProps({ log: makeLog(input), search: "ada" }),
		})

		expect(
			summary(wrapper)
				.findAll("mark")
				.map((m) => m.text()),
		).toEqual(expected)
	})

	it.for([
		{
			name: "fades the fields carried beside the line and the labels while a search is on",
			input: "ada",
			expected: ["trace_id=abc", "env=prod"],
		},
		{ name: "fades nothing without a search", input: "", expected: [] },
	])("$name", async ({ input, expected }, { expect }) => {
		const wrapper = await mountUnderTooltipProvider(Row, {
			props: rowProps({
				log: makeLog({
					line: '{"msg":"Ada signed in"}',
					labels: { env: "prod" },
					fields: { trace_id: "abc" },
				}),
				search: input,
			}),
		})

		expect(
			summary(wrapper)
				.findAll(".opacity-40")
				.map((part) => part.text()),
		).toEqual(expected)
	})

	it("shows a collapsed row without its details", async ({ expect }) => {
		const wrapper = await mountRow({ expanded: false })

		expect(wrapper.get("button").attributes("aria-expanded")).toBe("false")
		expect(wrapper.text()).not.toContain(t("editor.logs.view.line-heading"))
	})

	it("shows an expanded row with its details", async ({ expect }) => {
		const wrapper = await mountRow({ expanded: true })

		expect(wrapper.get("button").attributes("aria-expanded")).toBe("true")
		expect(wrapper.get("h4").text()).toBe(t("editor.logs.view.line-heading"))
	})

	it("grows its details open when expanded after mounting", async ({
		expect,
	}) => {
		const wrapper = await mountRow({ expanded: false })

		await wrapper.setProps({ expanded: true })
		await flushPromises()

		const details = wrapper.get(
			`#${String(wrapper.get("button").attributes("aria-controls"))}`,
		)
		expect(details.attributes("data-state")).toBe("open")
		expect(details.classes()).toContain(
			"data-[state=open]:animate-collapsible-down",
		)
		expect(details.get("h4").text()).toBe(t("editor.logs.view.line-heading"))
	})

	it.for([
		{ input: false, expected: true },
		{ input: true, expected: false },
	])(
		"asks to set expanded to $expected when pressed while it is $input",
		async ({ input, expected }, { expect }) => {
			const wrapper = await mountRow({ expanded: input })

			await wrapper.get("button").trigger("click")

			expect(wrapper.emitted("update:expanded")).toEqual([[expected]])
		},
	)

	it("passes on a filter from its details", async ({ expect }) => {
		const wrapper = await mountRow({ expanded: true })

		await findButtonByText(
			wrapper,
			t("editor.logs.view.include-value", { key: "job" }),
		).trigger("click")

		expect(wrapper.emitted("filter")).toEqual([
			[{ key: "job", value: "api", indexed: true, exclude: false }],
		])
	})
})

function makeLog(overrides: Partial<Log> = {}): Log {
	return {
		id: "l1",
		time: "2026-01-02T03:04:05Z",
		level: LogLevel.Info,
		line: "deploy finished",
		labels: { job: "api" },
		fields: {},
		...overrides,
	}
}

function mountRow(props: Parameters<typeof rowProps>[0] = {}) {
	return mountSuspended(Row, { props: rowProps(props) })
}

function rowProps(
	props: {
		log?: Log
		timestampFormat?: LogTimestampFormat
		search?: string
		expanded?: boolean
	} = {},
) {
	return {
		log: makeLog(),
		timestampFormat: LogTimestampFormat.Numeric,
		search: "",
		dialect: lokiDialect,
		lineFilterFormats: Object.values(LineFormat),
		expanded: false,
		...props,
	}
}

function summary(wrapper: VueWrapper) {
	return wrapper.get("button .flex-1.overflow-x-auto")
}
