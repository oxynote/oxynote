// shared helpers for the log block component suites. Test-only: the
// app/**/test-helpers.ts coverage exclude keeps this out of the
// denominator, and nothing here is imported by app code.
import { flushPromises, type VueWrapper } from "@vue/test-utils"
import { defineComponent, h, type ComponentPublicInstance } from "vue"

const STUB_SCROLLER_ROWS = 50
const STUB_ROW_HEIGHT = 32

// a log request runs through the test-time h3 app, which answers a few
// macrotasks later, and the view updates after that
export async function settleLogRequests() {
	for (let round = 0; round < 5; round++) {
		await flushPromises()
	}
}

// a plain line on the first of January 2026, which is its own message
export function makeLog(id: string, clock: string): Log {
	return {
		id,
		time: `2026-01-01T${clock}`,
		level: LogLevel.Info,
		line: id,
		labels: { env: "prod" },
		fields: {},
	}
}

export function logsResult(
	lines: Log[],
	nextCursor: string | null = null,
): LogsResult {
	return {
		status: lines.length
			? GenericQueryResultStatus.Ok
			: GenericQueryResultStatus.NoData,
		lines,
		nextCursor,
		previousCursor: null,
		partial: false,
	}
}

// happy-dom lays nothing out, so the real scroller would show no row. The
// stand-in renders the first rows, as many as a tall view would show, and
// a test says which rows the reader sees through showRowsUpTo. Every row
// is as tall as a closed one.
export function virtualScrollerStub() {
	return {
		DynamicScroller: defineComponent({
			name: "DynamicScroller",
			props: { items: { type: Array, required: true }, shift: Boolean },
			emits: ["update"],
			setup: (props, { slots, expose }) => {
				expose({ getItemOffset: (index: number) => index * STUB_ROW_HEIGHT })

				return () =>
					h("div", [
						slots.before?.(),
						...(props.items as Record<string, unknown>[])
							.slice(0, STUB_SCROLLER_ROWS)
							.map((item, index) =>
								slots.default?.({ item: item, index: index, active: true }),
							),
						slots.after?.(),
					])
			},
		}),
		DynamicScrollerItem: defineComponent({
			name: "DynamicScrollerItem",
			setup:
				(_props, { slots }) =>
				() =>
					h("div", slots.default?.()),
		}),
	}
}

// showRowsUpTo reports the rows up to the index as the ones the scroller
// shows, as the real one does once the reader scrolls. A first index
// scrolls the rows above it out of view.
export async function showRowsUpTo(
	wrapper: VueWrapper,
	index: number,
	first = 0,
) {
	const scroller = wrapper.findComponent({ name: "DynamicScroller" })
	const element = scroller.element as HTMLElement
	const vm = scroller.vm as ComponentPublicInstance

	element.scrollTop = first * STUB_ROW_HEIGHT
	vm.$emit("update", first, index, first, index)
	await settleLogRequests()
}
