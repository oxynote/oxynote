import type { EChartsCoreOption } from "echarts/core"
import type VChart from "vue-echarts"

// how often a chart follows a container that keeps changing size, such as
// while a side panel slides open
const RESIZE_INTERVAL_MS = 100
// how long a chart away from the viewport waits for its container to stop
// changing size before it is redrawn
const SETTLE_DELAY_MS = 300

// the size a chart is drawn at, and the redraw that follows it. Each redraw
// lays out and paints the whole chart again, so the size follows the
// container at most every RESIZE_INTERVAL_MS, and a chart away from the
// viewport is redrawn only once its container has settled or it comes close.
//
// Printing lays the page out at the paper's width, but resize observers do
// not run then. The print media listener does run in print layout, but the
// page is printed before any microtask, so vue cannot render the chart again
// in time. The listener hands echarts the new size and option itself.
export default function (
	container: Readonly<ShallowRef<HTMLElement | null>>,
	chart: Readonly<ShallowRef<InstanceType<typeof VChart> | null>>,
	option: () => EChartsCoreOption,
) {
	const { width: containerWidth, height: containerHeight } =
		useElementSize(container)
	const nearViewport = useElementVisibility(container, {
		rootMargin: "10%",
		initialValue: true,
	})
	const width = shallowRef(0)
	const height = shallowRef(0)
	const printQuery = shallowRef<MediaQueryList>()

	const followContainer = useThrottleFn(
		() => {
			width.value = containerWidth.value
			height.value = containerHeight.value
		},
		RESIZE_INTERVAL_MS,
		true,
	)
	const redrawOnceSettled = useDebounceFn(redraw, SETTLE_DELAY_MS)

	onMounted(() => {
		printQuery.value = window.matchMedia("print")
	})

	watch([containerWidth, containerHeight], followContainer)

	// post, as the gauge sizes its canvas host from the rendered size. Vue
	// fires it even when the size ends up where it was, such as after a
	// print, because width and height are shallow refs.
	watch(
		[width, height, nearViewport],
		([, , near]) => {
			if (near) {
				redraw()
				return
			}

			void redrawOnceSettled()
		},
		{ flush: "post" },
	)

	// the print preview lays the page out again when its settings change,
	// and reports each layout as a change to print without one back to
	// screen, so the listener cannot skip a repeated value
	useEventListener(printQuery, "change", (event: MediaQueryListEvent) => {
		const elem = container.value
		const instance = chart.value?.chart
		if (!elem?.clientWidth || !elem.clientHeight || !instance) {
			return
		}

		width.value = elem.clientWidth
		height.value = elem.clientHeight

		// back on screen the watcher above redraws once vue has rendered the
		// size
		if (!event.matches) {
			return
		}

		instance.resize()
		// unlike resize, setOption paints before it returns
		instance.setOption(option())
	})

	// redraws the chart when its canvas no longer fits its host. A hidden
	// chart has nothing to fit, and echarts would size it from its style
	// instead.
	function redraw() {
		const instance = chart.value?.chart
		const host = instance?.getDom()
		if (!instance || !host?.clientWidth || !host.clientHeight) {
			return
		}

		if (
			host.clientWidth === instance.getWidth() &&
			host.clientHeight === instance.getHeight()
		) {
			return
		}

		instance.resize()
	}

	return { width, height, redraw }
}
