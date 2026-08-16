<script lang="ts" setup>
import {
	autoUpdate,
	computePosition,
	flip,
	offset,
	shift,
} from "@floating-ui/dom"
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
const POPOVER_WIDTH = ITEM_SIZE * GRID_ITEMS_PER_ROW

const { open, anchor, closeIconPicker, selectIcon } = useIconPicker()

const iconFilter = ref("")
const tooltipOpen = ref<string | null>(null)

const icons = computed<
	{
		normalName: string
		id: string
	}[]
>(() => {
	return selectableIcons.filter((icon) => {
		return (
			!iconFilter.value ||
			icon.prettyName.includes(iconFilter.value.toLowerCase())
		)
	})
})

const floatingElem = useTemplateRef("icon-picker")
let cleanupAutoUpdate: (() => void) | null = null

onBeforeUnmount(() => {
	document.removeEventListener("mousedown", handleClickOutside)
	document.removeEventListener("keydown", handleEscape)
	cleanupAutoUpdate?.()
})

watch(open, (newV) => {
	if (newV) {
		iconFilter.value = ""
		tooltipOpen.value = null

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
	tooltipOpen.value = null
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
				<div
					class="flex h-60 max-h-[50dvh] flex-col"
					:style="{ width: `${POPOVER_WIDTH}px` }"
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
						<RecycleScroller
							v-slot="{ item }"
							:items="icons"
							:item-size="ITEM_SIZE"
							:item-secondary-size="ITEM_SIZE"
							:grid-items="GRID_ITEMS_PER_ROW"
							:buffer="400"
							key-field="id"
							@scroll="handleScroll"
						>
							<ShadcnUiTooltip
								:delay-duration="1000"
								:open="tooltipOpen === item.id"
								@update:open="(v) => (tooltipOpen = v ? item.id : null)"
							>
								<ShadcnUiTooltipTrigger as-child>
									<ShadcnUiButton
										:style="{
											width: `${ITEM_SIZE}px`,
											height: `${ITEM_SIZE}px`,
										}"
										size="icon"
										variant="ghost"
										@click="selectIcon(item.id)"
									>
										<Icon class="size-[20px]" :name="item.id" />
									</ShadcnUiButton>
								</ShadcnUiTooltipTrigger>
								<ShadcnUiTooltipContent
									align="center"
									side="bottom"
									hide-when-detached
									position-strategy="absolute"
									sticky="always"
									class="px-1.5 py-1"
								>
									<span>{{ item.normalName }}</span>
								</ShadcnUiTooltipContent>
							</ShadcnUiTooltip>
						</RecycleScroller>
					</template>
				</div>
			</div>
		</Teleport>
	</ClientOnly>
</template>
