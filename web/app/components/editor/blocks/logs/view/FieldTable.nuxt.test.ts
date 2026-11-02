import { mountSuspended } from "@nuxt/test-utils/runtime"
import type { VueWrapper } from "@vue/test-utils"
import { describe, it } from "vitest"
import FieldTable from "./FieldTable.vue"
import type { KeyValue } from "../lines"
import { findButtonByText, t } from "~/components/test-helpers"

const ENTRIES: KeyValue[] = [
	["env", "prod"],
	["job", "api"],
]

describe("<FieldTable>", () => {
	it("shows a row per entry with its key and value", async ({ expect }) => {
		const wrapper = await mountTable({ entries: ENTRIES })

		expect(rows(wrapper)).toEqual([
			["env", "prod"],
			["job", "api"],
		])
	})

	it("widens the key column once the view has room", async ({ expect }) => {
		const wrapper = await mountTable({ entries: ENTRIES })

		const keyCell = wrapper.get(".group > div").classes()
		expect(keyCell).toContain("w-31")
		expect(keyCell).toContain("@xl:w-44")
	})

	it.for([
		{ label: "include-value", indexed: true, exclude: false },
		{ label: "exclude-value", indexed: true, exclude: true },
		{ label: "include-value", indexed: false, exclude: false },
		{ label: "exclude-value", indexed: false, exclude: true },
	])(
		"asks to filter by a value from its $label button when indexed is $indexed",
		async ({ label, indexed, exclude }, { expect }) => {
			const wrapper = await mountTable({ entries: ENTRIES, indexed })

			await findButtonByText(
				wrapper,
				t(`editor.logs.view.${label}`, { key: "job" }),
			).trigger("click")

			expect(wrapper.emitted("filter")).toEqual([
				[{ key: "job", value: "api", indexed, exclude }],
			])
		},
	)
})

function mountTable(props: { entries: KeyValue[]; indexed?: boolean }) {
	return mountSuspended(FieldTable, {
		props: { indexed: false, ...props },
	})
}

// each row's key and value, which are the cells that truncate
function rows(wrapper: VueWrapper): string[][] {
	return wrapper
		.findAll(".group")
		.map((row) => row.findAll(".truncate").map((cell) => cell.text()))
}
