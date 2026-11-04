import type { TooltipComponentOption } from "echarts/components"
import type VChart from "vue-echarts"
import { cn } from "#layers/ui/app/lib/utils"

type PinnableChart = Pick<
	InstanceType<typeof VChart>,
	"dispatchAction" | "containPixel"
>

// a pinned tooltip stays at its instant until a click elsewhere or Esc, so
// the mouse can move into it and scroll a list taller than the chart. The
// chart spreads tooltipOption into its tooltip and calls togglePin on
// zr:click.
export function useTooltipPin(
	chart: Readonly<ShallowRef<PinnableChart | null>>,
	container: Readonly<ShallowRef<HTMLElement | null>>,
) {
	const pinned = ref(false)

	const tooltipOption = computed<TooltipComponentOption>(() => ({
		confine: true,
		triggerOn: pinned.value ? "none" : "mousemove",
		enterable: pinned.value,
		alwaysShowContent: pinned.value,
		className: cn("z-tooltip max-w-full"),
	}))

	useEventListener(document, "keydown", (event: KeyboardEvent) => {
		if (event.key === "Escape") {
			void unpin()
		}
	})

	useEventListener(document, "pointerdown", (event: PointerEvent) => {
		if (
			!(event.target instanceof Node) ||
			!container.value?.contains(event.target)
		) {
			void unpin()
		}
	})

	async function togglePin(event: { offsetX: number; offsetY: number }) {
		if (pinned.value) {
			await unpin()

			return
		}

		// a click on the legend or the title has no instant to pin
		const instance = chart.value
		if (!instance?.containPixel("grid", [event.offsetX, event.offsetY])) {
			return
		}

		pinned.value = true
		// echarts lets the mouse into the tooltip only from its next show, so
		// it is shown again at the click once the chart has the new option
		await nextTick()
		instance.dispatchAction({
			type: "showTip",
			x: event.offsetX,
			y: event.offsetY,
		})
	}

	async function unpin() {
		if (!pinned.value) {
			return
		}

		pinned.value = false
		// alwaysShowContent ignores a hide until the chart has the new option
		await nextTick()
		chart.value?.dispatchAction({ type: "hideTip" })
	}

	return { pinned, tooltipOption, togglePin }
}
