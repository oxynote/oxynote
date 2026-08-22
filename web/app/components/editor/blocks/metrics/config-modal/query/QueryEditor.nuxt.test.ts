import { mountSuspended } from "@nuxt/test-utils/runtime"
import type { VueWrapper } from "@vue/test-utils"
import { beforeEach, describe, it } from "vitest"
import CodeMirror from "vue-codemirror6"
import QueryEditor from "./QueryEditor.vue"
import { TimeRangePreset } from "../../utils"
import { makeXid } from "~/composables/api/test-helpers"
import { emitFrom, t } from "~/components/test-helpers"
import { DiffStatus } from "~/components/editor/diff/position-map"

const DATA_SOURCE_ID = makeXid("ds")

function mountEditor(props: Record<string, unknown> = {}) {
	return mountSuspended(QueryEditor, {
		props: {
			dataSourceId: DATA_SOURCE_ID,
			timeRange: TimeRangePreset.Last5Minutes,
			modelValue: "",
			...props,
		},
	})
}

function content(wrapper: VueWrapper) {
	return wrapper.get(".cm-content")
}

// codemirror reports focus changes through the wrapper component's own
// focus event rather than a dom event on the editable area
async function focusEditor(wrapper: VueWrapper) {
	emitFrom(wrapper, CodeMirror, "focus", true)
	await nextTick()
}

async function blurEditor(wrapper: VueWrapper) {
	emitFrom(wrapper, CodeMirror, "focus", false)
	await nextTick()
}

// the editable flag is a shared cookie state and the editor store is
// app-wide, so these tests cannot interleave
describe("<QueryEditor>", { concurrent: false }, () => {
	beforeEach(() => {
		useEditorMeta().setEditable(true)
		useEditorStore().setReviewableDiffActive(false)
	})

	it("shows the query it was given", async ({ expect }) => {
		const wrapper = await mountEditor({ modelValue: "up{job='api'}" })

		expect(content(wrapper).text()).toBe("up{job='api'}")
	})

	it("prompts for a query while the field is empty", async ({ expect }) => {
		const wrapper = await mountEditor()

		expect(content(wrapper).attributes("aria-placeholder")).toBe(
			t("editor.metrics.config.query-placeholder.default"),
		)
	})

	it("stays fully lit once a data source and time range are set", async ({
		expect,
	}) => {
		const wrapper = await mountEditor()

		expect(wrapper.classes()).not.toContain("opacity-50")
		expect(content(wrapper).attributes("contenteditable")).toBe("true")
	})

	it("dims itself while no data source is set", async ({ expect }) => {
		const wrapper = await mountEditor({ dataSourceId: null })

		expect(wrapper.classes()).toContain("opacity-50")
		expect(content(wrapper).attributes("contenteditable")).toBe("false")
	})

	it("dims itself while no time range is set", async ({ expect }) => {
		const wrapper = await mountEditor({ timeRange: null })

		expect(wrapper.classes()).toContain("opacity-50")
	})

	it("hides its caret in read mode", async ({ expect }) => {
		useEditorMeta().setEditable(false)

		const wrapper = await mountEditor()

		expect(wrapper.classes()).toContain("query-editor--readonly")
	})

	it("hides its caret while a reviewable diff is shown", async ({ expect }) => {
		useEditorStore().setReviewableDiffActive(true)

		const wrapper = await mountEditor()

		expect(wrapper.classes()).toContain("query-editor--readonly")
	})

	it.for([
		{ status: DiffStatus.Added, expected: "bg-diff-field-added" },
		{ status: DiffStatus.Removed, expected: "bg-diff-field-removed" },
	])(
		"tints a $status query field",
		async ({ status, expected }, { expect }) => {
			const wrapper = await mountEditor({ diffStatus: status })

			expect(wrapper.classes()).toContain(expected)
		},
	)

	it("reports that the reader started editing", async ({ expect }) => {
		const wrapper = await mountEditor({ modelValue: "up" })

		await focusEditor(wrapper)

		expect(wrapper.emitted("focus-change")).toEqual([[true]])
	})

	it("reports that the reader finished editing", async ({ expect }) => {
		const wrapper = await mountEditor({ modelValue: "up" })
		await focusEditor(wrapper)

		await blurEditor(wrapper)

		expect(wrapper.emitted("focus-change")?.slice(0, 2)).toEqual([
			[true],
			[false],
		])
	})

	it("reports nothing for a blur before the field was ever focused", async ({
		expect,
	}) => {
		const wrapper = await mountEditor({ modelValue: "up" })

		await blurEditor(wrapper)

		expect(wrapper.emitted("focus-change")).toBeUndefined()
	})

	it("trims the query once the reader leaves the field", async ({ expect }) => {
		const wrapper = await mountEditor({ modelValue: "  up  " })
		await focusEditor(wrapper)

		await blurEditor(wrapper)

		expect(content(wrapper).text()).toBe("up")
		expect(wrapper.emitted("update:modelValue")?.at(-1)).toEqual(["up"])
	})
})
