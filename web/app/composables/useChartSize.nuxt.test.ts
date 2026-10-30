import { enableAutoUnmount, mount } from "@vue/test-utils"
import { afterEach, describe, it, vi } from "vitest"
import { defineComponent, h, nextTick, shallowRef } from "vue"
import type VChart from "vue-echarts"
import useChartSize from "./useChartSize"

const SKIPPED_CASES = [
	{
		name: "echarts has not started the chart",
		input: { started: false, width: 401 },
	},
	{ name: "the chart is hidden", input: { started: true, width: 0 } },
]

enableAutoUnmount(afterEach)

// the print media query is stubbed so a test can announce print layout,
// which happy-dom never enters on its own
describe("useChartSize", { concurrent: false }, () => {
	// nothing is awaited after the event, as the browser prints before any
	// microtask runs
	it("refits the chart to its container as soon as print layout starts", async ({
		expect,
	}) => {
		const host = await mountHost()

		announcePrint(host.printQuery, true)

		expect.soft(host.matchMedia).toHaveBeenCalledTimes(1)
		expect.soft(host.matchMedia).toHaveBeenCalledWith("print")
		expect.soft(host.size.width.value).toBe(401)
		expect.soft(host.size.height.value).toBe(300)
		expect.soft(host.instance.resize).toHaveBeenCalledTimes(1)
		expect.soft(host.option).toHaveBeenCalledTimes(1)
		expect.soft(host.instance.setOption).toHaveBeenCalledTimes(1)
		expect
			.soft(host.instance.setOption)
			.toHaveBeenCalledWith({ width: 401, height: 300 })
		// setOption paints, so it has to come after the canvas is resized
		expect
			.soft(host.instance.resize.mock.invocationCallOrder[0])
			.toBeLessThan(host.instance.setOption.mock.invocationCallOrder[0] ?? 0)
	})

	// print preview reports every layout, such as a switch to landscape, as
	// another change to print
	it("refits the chart again for each print layout", async ({ expect }) => {
		const host = await mountHost()

		announcePrint(host.printQuery, true)
		announcePrint(host.printQuery, true)

		expect.soft(host.instance.resize).toHaveBeenCalledTimes(2)
		expect.soft(host.option).toHaveBeenCalledTimes(2)
		expect.soft(host.instance.setOption).toHaveBeenCalledTimes(2)
	})

	it("refits the chart once vue has rendered when printing ends", async ({
		expect,
	}) => {
		const host = await mountHost({ width: 834, height: 307 })

		announcePrint(host.printQuery, false)

		expect(host.size.width.value).toBe(834)
		expect(host.instance.resize).toHaveBeenCalledTimes(0)

		await nextTick()

		expect.soft(host.instance.resize).toHaveBeenCalledTimes(1)
		expect.soft(host.option).toHaveBeenCalledTimes(1)
		expect.soft(host.instance.setOption).toHaveBeenCalledTimes(1)
		expect
			.soft(host.instance.setOption)
			.toHaveBeenCalledWith({ width: 834, height: 307 })
	})

	it.for(SKIPPED_CASES)(
		"leaves the chart alone when $name",
		async ({ input }, { expect }) => {
			const host = await mountHost(input)

			announcePrint(host.printQuery, true)
			await nextTick()

			expect.soft(host.size.width.value).toBe(0)
			expect.soft(host.size.height.value).toBe(0)
			expect.soft(host.option).toHaveBeenCalledTimes(0)
			expect.soft(host.instance.resize).toHaveBeenCalledTimes(0)
			expect.soft(host.instance.setOption).toHaveBeenCalledTimes(0)
		},
	)
})

// the composable needs a component instance for its lifecycle hooks, so
// every test mounts a small host around a container with a fixed size. The
// option is built from the size, the way the charts build theirs.
async function mountHost({ started = true, width = 401, height = 300 } = {}) {
	const printQuery = new EventTarget()
	const matchMedia = vi.fn(() => printQuery)
	vi.stubGlobal("matchMedia", matchMedia)

	const instance = { resize: vi.fn(), setOption: vi.fn() }
	const container = shallowRef<HTMLElement | null>(null)
	const chart = shallowRef({
		chart: started ? instance : undefined,
	} as unknown as InstanceType<typeof VChart>)
	let size: ReturnType<typeof useChartSize> | undefined
	const option = vi.fn(() => ({
		width: size?.width.value,
		height: size?.height.value,
	}))

	const Host = defineComponent({
		setup() {
			size = useChartSize(container, chart, option)

			return () => h("div", { ref: container })
		},
	})

	mount(Host)
	await nextTick()

	if (!container.value || !size) {
		throw new Error("the host did not mount")
	}

	Object.defineProperty(container.value, "clientWidth", { value: width })
	Object.defineProperty(container.value, "clientHeight", { value: height })

	return { printQuery, matchMedia, instance, option, size }
}

function announcePrint(printQuery: EventTarget, matches: boolean) {
	printQuery.dispatchEvent(Object.assign(new Event("change"), { matches }))
}
