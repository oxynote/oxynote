import { mockNuxtImport } from "@nuxt/test-utils/runtime"
import { enableAutoUnmount, mount } from "@vue/test-utils"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import { defineComponent, h, nextTick, ref, shallowRef } from "vue"
import type VChart from "vue-echarts"
import useChartSize from "./useChartSize"

const SKIPPED_CASES = [
	{
		name: "echarts has not started the chart",
		input: { started: false, width: 401 },
	},
	{ name: "the chart is hidden", input: { started: true, width: 0 } },
]

// happy-dom lays nothing out and intersects nothing, so the container's
// measured size and its nearness to the viewport are refs the tests set
const { useElementSizeMock, useElementVisibilityMock } = vi.hoisted(() => ({
	useElementSizeMock: vi.fn(),
	useElementVisibilityMock: vi.fn(),
}))

mockNuxtImport("useElementSize", () => useElementSizeMock)
mockNuxtImport("useElementVisibility", () => useElementVisibilityMock)

enableAutoUnmount(afterEach)

// the print media query is stubbed so a test can announce print layout,
// which happy-dom never enters on its own
describe("useChartSize", { concurrent: false }, () => {
	beforeEach(() => {
		useElementSizeMock.mockReset()
		useElementVisibilityMock.mockReset()
		vi.useFakeTimers()
	})

	afterEach(() => {
		vi.useRealTimers()
	})

	it("draws the chart at its container's size once it is measured", async ({
		expect,
	}) => {
		const host = await mountHost()

		await measure(host, 525, 318)

		expect.soft(host.size.width.value).toBe(525)
		expect.soft(host.size.height.value).toBe(318)
		expect.soft(host.instance.resize).toHaveBeenCalledTimes(1)
		expect.soft(host.canvas).toEqual({ width: 525, height: 318 })
		expect.soft(host.option).toHaveBeenCalledTimes(0)
		expect.soft(host.instance.setOption).toHaveBeenCalledTimes(0)
	})

	it("redraws a chart that keeps resizing at most every 100ms", async ({
		expect,
	}) => {
		const host = await mountHost()
		await measure(host, 525, 318)

		await measure(host, 520, 318)
		await measure(host, 510, 318)

		expect(host.size.width.value).toBe(525)
		expect(host.instance.resize).toHaveBeenCalledTimes(1)

		vi.advanceTimersByTime(100)
		await nextTick()

		expect.soft(host.size.width.value).toBe(510)
		expect.soft(host.instance.resize).toHaveBeenCalledTimes(2)
		expect.soft(host.canvas).toEqual({ width: 510, height: 318 })
		expect.soft(host.instance.setOption).toHaveBeenCalledTimes(0)
	})

	it("redraws a chart away from the viewport once its container settles", async ({
		expect,
	}) => {
		const host = await mountHost({ near: false })

		await measure(host, 525, 318)

		expect(host.size.width.value).toBe(525)
		expect(host.instance.resize).toHaveBeenCalledTimes(0)

		vi.advanceTimersByTime(300)

		expect.soft(host.instance.resize).toHaveBeenCalledTimes(1)
		expect.soft(host.canvas).toEqual({ width: 525, height: 318 })
		expect.soft(host.option).toHaveBeenCalledTimes(0)
		expect.soft(host.instance.setOption).toHaveBeenCalledTimes(0)
	})

	it("redraws a chart as soon as it comes close to the viewport", async ({
		expect,
	}) => {
		const host = await mountHost({ near: false })
		await measure(host, 525, 318)

		host.nearViewport.value = true
		await nextTick()

		expect(host.instance.resize).toHaveBeenCalledTimes(1)

		// the redraw it was waiting for finds the canvas already fitting
		vi.advanceTimersByTime(300)

		expect.soft(host.instance.resize).toHaveBeenCalledTimes(1)
		expect.soft(host.canvas).toEqual({ width: 525, height: 318 })
		expect.soft(host.option).toHaveBeenCalledTimes(0)
		expect.soft(host.instance.setOption).toHaveBeenCalledTimes(0)
	})

	it("does not redraw a chart that comes and goes from the viewport at the same size", async ({
		expect,
	}) => {
		const host = await mountHost()
		await measure(host, 525, 318)
		host.nearViewport.value = false
		await nextTick()

		host.nearViewport.value = true
		await nextTick()
		vi.advanceTimersByTime(300)

		expect.soft(host.instance.resize).toHaveBeenCalledTimes(1)
		expect.soft(host.option).toHaveBeenCalledTimes(0)
		expect.soft(host.instance.setOption).toHaveBeenCalledTimes(0)
	})

	it("does not redraw a chart whose container has no size", async ({
		expect,
	}) => {
		const host = await mountHost()
		await measure(host, 525, 318)
		vi.advanceTimersByTime(100)

		await measure(host, 0, 0)
		vi.advanceTimersByTime(100)
		await nextTick()

		expect.soft(host.size.width.value).toBe(0)
		expect.soft(host.instance.resize).toHaveBeenCalledTimes(1)
		expect.soft(host.canvas).toEqual({ width: 525, height: 318 })
		expect.soft(host.option).toHaveBeenCalledTimes(0)
		expect.soft(host.instance.setOption).toHaveBeenCalledTimes(0)
	})

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
		expect.soft(host.canvas).toEqual({ width: 401, height: 300 })
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
		layOut(host, 600, 300)
		announcePrint(host.printQuery, true)

		expect.soft(host.instance.resize).toHaveBeenCalledTimes(2)
		expect.soft(host.canvas).toEqual({ width: 600, height: 300 })
		expect.soft(host.option).toHaveBeenCalledTimes(2)
		expect.soft(host.instance.setOption).toHaveBeenCalledTimes(2)
	})

	// the size returns to what it was before the print without a flush in
	// between, which a watcher comparing values would take for no change
	it("redraws the chart at its screen size once printing ends", async ({
		expect,
	}) => {
		const host = await mountHost({ width: 834, height: 307 })
		await measure(host, 834, 307)
		layOut(host, 401, 300)
		announcePrint(host.printQuery, true)
		layOut(host, 834, 307)

		announcePrint(host.printQuery, false)
		await nextTick()

		expect.soft(host.size.width.value).toBe(834)
		expect.soft(host.size.height.value).toBe(307)
		expect.soft(host.instance.resize).toHaveBeenCalledTimes(3)
		expect.soft(host.canvas).toEqual({ width: 834, height: 307 })
		expect.soft(host.option).toHaveBeenCalledTimes(1)
		expect.soft(host.instance.setOption).toHaveBeenCalledTimes(1)
	})

	it("redraws a chart away from the viewport at its screen size once printing ends", async ({
		expect,
	}) => {
		const host = await mountHost({ width: 834, height: 307, near: false })
		await measure(host, 834, 307)
		vi.advanceTimersByTime(300)
		layOut(host, 401, 300)
		announcePrint(host.printQuery, true)
		layOut(host, 834, 307)

		announcePrint(host.printQuery, false)
		await nextTick()
		vi.advanceTimersByTime(300)

		expect.soft(host.instance.resize).toHaveBeenCalledTimes(3)
		expect.soft(host.canvas).toEqual({ width: 834, height: 307 })
	})

	it.for(SKIPPED_CASES)(
		"leaves the chart alone in print when $name",
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
// every test mounts a small host. The echarts stand-in keeps a canvas size
// that resize fits to its host element, which follows the container the way
// the chart's does. The measured size starts unset, and the option is built
// from the size, the way the charts build theirs.
async function mountHost({
	started = true,
	width = 401,
	height = 300,
	near = true,
} = {}) {
	const printQuery = new EventTarget()
	const matchMedia = vi.fn(() => printQuery)
	vi.stubGlobal("matchMedia", matchMedia)

	const containerWidth = shallowRef(0)
	const containerHeight = shallowRef(0)
	// a plain ref, so only the composable's own refs decide whether its
	// watcher fires for a size that ends up unchanged
	const nearViewport = ref(near)
	useElementSizeMock.mockImplementationOnce(() => ({
		width: containerWidth,
		height: containerHeight,
	}))
	useElementVisibilityMock.mockImplementationOnce(() => nearViewport)

	const chartHost = { clientWidth: 0, clientHeight: 0 }
	const canvas = { width: 0, height: 0 }
	const instance = {
		resize: vi.fn(() => {
			canvas.width = chartHost.clientWidth
			canvas.height = chartHost.clientHeight
		}),
		setOption: vi.fn(),
		getDom: () => chartHost,
		getWidth: () => canvas.width,
		getHeight: () => canvas.height,
	}
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

	const host = {
		printQuery,
		matchMedia,
		instance,
		canvas,
		option,
		size,
		container: container.value,
		chartHost,
		containerWidth,
		containerHeight,
		nearViewport,
	}
	layOut(host, width, height)

	return host
}

// the size the browser lays the container and the chart's host out at
function layOut(
	host: {
		container: HTMLElement
		chartHost: { clientWidth: number; clientHeight: number }
	},
	width: number,
	height: number,
) {
	Object.defineProperty(host.container, "clientWidth", {
		value: width,
		configurable: true,
	})
	Object.defineProperty(host.container, "clientHeight", {
		value: height,
		configurable: true,
	})
	host.chartHost.clientWidth = width
	host.chartHost.clientHeight = height
}

// the measured size changes the way the resize observer reports it, after
// the browser laid the container out at it, and the redraw follows once vue
// has flushed
async function measure(
	host: Awaited<ReturnType<typeof mountHost>>,
	width: number,
	height: number,
) {
	layOut(host, width, height)
	host.containerWidth.value = width
	host.containerHeight.value = height
	await nextTick()
}

function announcePrint(printQuery: EventTarget, matches: boolean) {
	printQuery.dispatchEvent(Object.assign(new Event("change"), { matches }))
}
