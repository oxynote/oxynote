import type { EChartsCoreOption } from "echarts/core"
import type VChart from "vue-echarts"

// the size of a chart's container, kept right while the page prints.
// Printing lays the page out at the paper's width, but resize observers do
// not run then, so neither useElementSize nor the chart's autoresize sees
// it. The print media listener does run in print layout, but the page is
// printed before any microtask, so vue cannot render the chart again in
// time. The listener hands echarts the new size and option itself.
export default function (
	container: Readonly<ShallowRef<HTMLElement | null>>,
	chart: Readonly<ShallowRef<InstanceType<typeof VChart> | null>>,
	option: () => EChartsCoreOption,
) {
	const { width, height } = useElementSize(container)
	const printQuery = shallowRef<MediaQueryList>()

	onMounted(() => {
		printQuery.value = window.matchMedia("print")
	})

	// the print preview lays the page out again when its settings change,
	// and reports each layout as a change to print without one back to
	// screen, so the listener cannot skip a repeated value
	useEventListener(printQuery, "change", (event: MediaQueryListEvent) => {
		const elem = container.value
		const instance = chart.value?.chart
		// a hidden chart has nothing to fit, and echarts would size it from
		// its style instead
		if (!elem?.clientWidth || !elem.clientHeight || !instance) {
			return
		}

		width.value = elem.clientWidth
		height.value = elem.clientHeight

		const redraw = () => {
			instance.resize()
			// unlike resize, setOption paints before it returns
			instance.setOption(option())
		}

		if (event.matches) {
			redraw()
			return
		}

		// back on screen there is time to let vue render the new size first,
		// which the gauge sizes its canvas host from
		void nextTick(redraw)
	})

	return { width, height }
}
