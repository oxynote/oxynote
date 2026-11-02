import { mountSuspended } from "@nuxt/test-utils/runtime"
import { flushPromises, type VueWrapper } from "@vue/test-utils"
import { describe, it } from "vitest"
import Row from "./Row.vue"
import { LogTimestampFormat } from "../utils"
import { findButtonByText, t } from "~/components/test-helpers"

// built from local fields, so the standard format reads the same clock in
// any time zone
const LOCAL_TIME = new Date(2026, 0, 2, 3, 4, 5).toISOString()

describe("<Row>", () => {
	it.for([
		{
			name: "shows the date and time of the standard format apart",
			input: { time: LOCAL_TIME, format: LogTimestampFormat.Standard },
			expected: "Jan 2, 2026 03:04:05",
		},
		{
			name: "shows the date and time of a detailed format apart",
			input: {
				time: LOCAL_TIME.replace(".000Z", ".123456789Z"),
				format: LogTimestampFormat.DetailedNano,
			},
			expected: "2026-01-02 03:04:05.123456789",
		},
	])("$name", async ({ input, expected }, { expect }) => {
		const wrapper = await mountRow({
			line: makeLine({ time: input.time }),
			timestampFormat: input.format,
		})

		expect(wrapper.get("button .tabular-nums").text()).toBe(expected)
	})

	it("shows no time when the format hides it", async ({ expect }) => {
		const wrapper = await mountRow({ timestampFormat: LogTimestampFormat.None })

		expect(wrapper.find("button .tabular-nums").exists()).toBe(false)
	})

	it("shows the line's level", async ({ expect }) => {
		const wrapper = await mountRow({
			line: makeLine({ level: LogLevel.Error }),
		})

		expect(wrapper.get("button .rounded-full").text()).toBe(
			t("editor.logs.levels.error"),
		)
	})

	it.for<{
		name: string
		input: Pick<LogLine, "line" | "labels">
		expected: string
	}>([
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
		const wrapper = await mountRow({ line: makeLine(input) })

		expect(summary(wrapper).text()).toBe(expected)
	})

	it("marks the search in the message and the values", async ({ expect }) => {
		const wrapper = await mountRow({
			line: makeLine({
				line: '{"msg":"Ada signed in","user":"ada"}',
				labels: {},
			}),
			search: "ada",
		})

		expect(
			summary(wrapper)
				.findAll("mark")
				.map((m) => m.text()),
		).toEqual(["Ada", "ada"])
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

function makeLine(overrides: Partial<LogLine> = {}): LogLine {
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

function mountRow(
	props: {
		line?: LogLine
		timestampFormat?: LogTimestampFormat
		search?: string
		expanded?: boolean
	} = {},
) {
	return mountSuspended(Row, {
		props: {
			line: makeLine(),
			timestampFormat: LogTimestampFormat.Detailed,
			search: "",
			expanded: false,
			...props,
		},
	})
}

function summary(wrapper: VueWrapper) {
	return wrapper.get("button .font-mono")
}
