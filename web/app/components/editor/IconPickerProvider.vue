<script lang="ts" setup>
import {
	autoUpdate,
	computePosition,
	flip,
	offset,
	shift,
} from "@floating-ui/dom"
import { buttonVariants } from "~/components/shadcn/ui/button"
import { cn } from "~/lib/utils"

// With defineAsyncComponent, Vue doesn't load the actual module until it's
// time to render—and since <ClientOnly> prevents that render on the server,
// the import never happens during SSR. Otherwise, vue-virtual-scroller would
// try to access window and document during SSR, causing hydration errors,
// even with <ClientOnly>.
// This is like a Lazy- prefix for components.
const RecycleScroller = defineAsyncComponent(() =>
	import("vue-virtual-scroller").then((m) => m.RecycleScroller),
)

const ITEM_SIZE = 28
const GRID_ITEMS_PER_ROW = 12
const GRID_WIDTH = ITEM_SIZE * GRID_ITEMS_PER_ROW
const TOOLTIP_DELAY_MS = 1000

const { open, anchor, closeIconPicker, selectIcon } = useIconPicker()
const { cssSelectorPrefix } = useAppConfig().icon

const iconFilter = ref("")
// the one tooltip the grid shares, anchored to the hovered cell
const tooltip = ref<{ name: string; cell: HTMLElement } | null>(null)
let tooltipTimer: ReturnType<typeof setTimeout> | null = null

const icons = computed<
	{
		name: string
		id: string
	}[]
>(() => {
	return selectableIcons.filter((icon) => {
		return (
			!iconFilter.value || icon.name.includes(iconFilter.value.toLowerCase())
		)
	})
})

const floatingElem = useTemplateRef("icon-picker")
let cleanupAutoUpdate: (() => void) | null = null

// the icons' stylesheet is large and only the picker needs it, so it loads
// once the page's own load has finished rather than competing with it
onMounted(() => {
	const load = () => void import("virtual:icon-picker.css")

	if (document.readyState === "complete") {
		load()
	} else {
		window.addEventListener("load", load, { once: true })
	}
})

onBeforeUnmount(() => {
	document.removeEventListener("mousedown", handleClickOutside)
	document.removeEventListener("keydown", handleEscape)
	cleanupAutoUpdate?.()
})

watch(open, (newV) => {
	if (newV) {
		iconFilter.value = ""
		hideTooltip()

		void nextTick(() => {
			document.addEventListener("mousedown", handleClickOutside)
			document.addEventListener("keydown", handleEscape)

			void updatePosition()

			if (anchor.value && floatingElem.value) {
				cleanupAutoUpdate = autoUpdate(anchor.value, floatingElem.value, () => {
					void updatePosition()
				})
			}
		})
	} else {
		document.removeEventListener("mousedown", handleClickOutside)
		document.removeEventListener("keydown", handleEscape)

		cleanupAutoUpdate?.()
		cleanupAutoUpdate = null
	}
})

async function updatePosition() {
	if (!anchor.value || !floatingElem.value) {
		return
	}

	const { x, y } = await computePosition(anchor.value, floatingElem.value, {
		strategy: "fixed",
		placement: "bottom-start",
		middleware: [offset(4), flip(), shift({ padding: 8 })],
	})

	Object.assign(floatingElem.value.style, {
		top: "0px",
		left: "0px",
		transform: `translate(${x}px, ${y}px)`,
	})
}

function handleScroll() {
	hideTooltip()
}

function hideTooltip() {
	if (tooltipTimer) {
		clearTimeout(tooltipTimer)
		tooltipTimer = null
	}

	tooltip.value = null
}

// one delegated listener for the whole grid, so a cell is a plain button
function handleGridHover(e: Event) {
	const cell = (e.target as HTMLElement).closest<HTMLElement>("[data-name]")
	if (!cell) {
		return
	}

	const name = cell.dataset.name ?? ""
	if (tooltip.value?.name === name) {
		return
	}

	hideTooltip()

	tooltipTimer = setTimeout(() => {
		tooltip.value = { name, cell }
	}, TOOLTIP_DELAY_MS)
}

function handleClickOutside(e: MouseEvent) {
	if (!floatingElem.value) {
		return
	}

	if (anchor.value?.contains(e.target as Node)) {
		return
	}

	// eslint-disable-next-line @typescript-eslint/no-unnecessary-type-assertion, @typescript-eslint/no-unsafe-call -- eslint can't resolve floatingElem's type here, vue-tsc needs the assertion
	if (!floatingElem.value.contains(e.target as Node)) {
		closeIconPicker()
	}
}

function handleEscape(e: KeyboardEvent) {
	if (e.key === "Escape") {
		closeIconPicker()
	}
}
</script>

<template>
	<slot />
	<ClientOnly>
		<Teleport to="body">
			<div
				ref="icon-picker"
				:class="
					cn(
						'pointer-events-none fixed z-dropdown rounded-lg border bg-popover p-1 text-popover-foreground opacity-0 shadow-md transition-opacity duration-150',
						open && 'pointer-events-auto opacity-100',
					)
				"
				style="top: 0px; left: 0px"
			>
				<!--a grid row is a fixed 12 x 28px, so where scrollbars take up
				space the popover has to grow by the gutter the scroller reserves
				below, or the row overflows into a horizontal scrollbar-->
				<div
					class="flex h-60 max-h-[50dvh] flex-col"
					:style="{ minWidth: `${GRID_WIDTH}px` }"
				>
					<div class="px-1 pt-0.5 pb-1">
						<div class="relative block items-center overflow-hidden">
							<LazyShadcnUiInput
								v-model="iconFilter"
								type="text"
								:placeholder="$t('editor.icon-select-menu.input-placeholder')"
								class="h-7 pl-8 text-2sm"
								disable-focus-effect
								autocomplete="off"
							/>
							<span
								class="absolute inset-y-0 start-0 flex items-center justify-center px-2"
							>
								<Icon
									class="size-4 text-muted-foreground"
									name="lucide:search"
								/>
							</span>
						</div>
					</div>
					<div
						v-if="!icons.length"
						class="flex w-full flex-1 items-center justify-center text-2base"
					>
						<div class="flex items-center gap-1">
							<Icon class="size-[1.2em]" name="lucide:search-x" />
							<span>{{ $t("editor.icon-select-menu.no-results") }}</span>
						</div>
					</div>
					<template v-else>
						<!--the reserved gutter is what firefox measures the popover's
						width from; chrome counts the scrollbar without it-->
						<RecycleScroller
							v-slot="{ item }"
							class="[scrollbar-gutter:stable]"
							:items="icons"
							:item-size="ITEM_SIZE"
							:item-secondary-size="ITEM_SIZE"
							:grid-items="GRID_ITEMS_PER_ROW"
							:buffer="600"
							key-field="id"
							skip-hover
							@scroll="handleScroll"
							@mouseover="handleGridHover"
							@mouseleave="hideTooltip"
							@focusin="handleGridHover"
							@focusout="hideTooltip"
						>
							<!--the scroller patches each recycled cell on every scroll, so
							a cell has to be cheap to patch: plain elements, no components-->
							<button
								type="button"
								:class="cn(buttonVariants({ variant: 'ghost', size: 'icon' }))"
								:style="{
									width: `${ITEM_SIZE}px`,
									height: `${ITEM_SIZE}px`,
								}"
								:aria-label="item.name"
								:data-name="item.name"
								@click="selectIcon(item.id)"
							>
								<span
									class="iconify size-[20px]"
									:class="cssSelectorPrefix + item.id"
								/>
							</button>
						</RecycleScroller>
						<ShadcnUiTooltip :open="tooltip !== null" :delay-duration="0">
							<!--the trigger registers the anchor; the hovered cell, not the
							trigger itself, is what the content positions against-->
							<ShadcnUiTooltipTrigger
								as="span"
								class="hidden"
								:reference="tooltip?.cell"
							/>
							<ShadcnUiTooltipContent
								align="center"
								side="bottom"
								hide-when-detached
								position-strategy="absolute"
								sticky="always"
								class="px-1.5 py-1"
							>
								<span>{{ tooltip?.name }}</span>
							</ShadcnUiTooltipContent>
						</ShadcnUiTooltip>
					</template>
				</div>
			</div>
		</Teleport>
	</ClientOnly>
</template>
