import type { VueWrapper } from "@vue/test-utils"
import { describe, it } from "vitest"
import FieldTable from "./FieldTable.vue"
import { LineFormat } from "../dialect"
import type { FieldRow } from "../rows"
import {
	findButtonByText,
	mountUnderTooltipProvider,
	t,
} from "~/components/test-helpers"

const ENTRIES: FieldRow[] = [
	{
		key: "env",
		value: "prod",
		filter: { key: "env", value: "prod", indexed: true },
	},
	{
		key: "http.status",
		value: "500",
		filter: {
			key: "http_status",
			value: "500",
			indexed: false,
			format: LineFormat.JSON,
		},
	},
	{
		key: "user",
		value: '{"id":1}',
		filter: null,
		disabledReason: "nested",
	},
]

describe("<FieldTable>", () => {
	it("shows a row per entry with its key and value", async ({ expect }) => {
		const wrapper = await mountTable({ entries: ENTRIES })

		expect(rows(wrapper)).toEqual([
			["env", "prod"],
			["http.status", "500"],
			["user", '{"id":1}'],
		])
	})

	it("says none without entries", async ({ expect }) => {
		const wrapper = await mountTable({ entries: [] })

		expect(wrapper.get(".divide-y").text()).toBe(
			t("editor.logs.view.section-empty"),
		)
		expect(wrapper.findAll("button")).toHaveLength(0)
	})

	it("widens the key column once the view has room", async ({ expect }) => {
		const wrapper = await mountTable({ entries: ENTRIES })

		const keyCell = wrapper.get(".divide-y > div > div").classes()
		expect(keyCell).toContain("w-31")
		expect(keyCell).toContain("@xl:w-44")
	})

	it.for([
		{ label: "include-value", exclude: false },
		{ label: "exclude-value", exclude: true },
	])(
		"asks for its row's filter from the $label button",
		async ({ label, exclude }, { expect }) => {
			const wrapper = await mountTable({ entries: ENTRIES })

			await findButtonByText(
				wrapper,
				t(`editor.logs.view.${label}`, { key: "http.status" }),
			).trigger("click")

			expect(wrapper.emitted("filter")).toBeUndefined()
			expect(table(wrapper).emitted("filter")).toEqual([
				[
					{
						key: "http_status",
						value: "500",
						indexed: false,
						format: LineFormat.JSON,
						exclude,
					},
				],
			])
		},
	)

	it("turns off the buttons of a row without a filter and explains why", async ({
		expect,
	}) => {
		const wrapper = await mountTable({ entries: ENTRIES })
		const userRow = wrapper.findAll(".divide-y > div").at(2)
		const buttons = userRow?.findAll("button") ?? []

		expect(buttons).toHaveLength(2)
		expect(buttons.every((b) => b.attributes("disabled") !== undefined)).toBe(
			true,
		)
		expect(wrapper.findAll("[data-slot='tooltip-trigger']")).toHaveLength(1)
		expect(userRow?.find("[data-slot='tooltip-trigger']").exists()).toBe(true)

		await buttons[0]?.trigger("click")

		expect(table(wrapper).emitted("filter")).toBeUndefined()
	})
})

function mountTable(props: { entries: FieldRow[] }) {
	return mountUnderTooltipProvider(FieldTable, { props })
}

// the table inside the tooltip provider, which holds its emitted events
function table(wrapper: VueWrapper) {
	return wrapper.findComponent(FieldTable)
}

// each row's key and value, which are the cells that truncate
function rows(wrapper: VueWrapper): string[][] {
	return wrapper
		.findAll(".divide-y > div")
		.map((row) => row.findAll(".truncate").map((cell) => cell.text()))
}
