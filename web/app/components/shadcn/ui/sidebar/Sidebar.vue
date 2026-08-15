<script setup lang="ts">
import type { SidebarProps } from "."
import { cn } from "@/lib/utils"
import { Sheet, SheetContent } from "@/components/shadcn/ui/sheet"
import SheetDescription from "@/components/shadcn/ui/sheet/SheetDescription.vue"
import SheetHeader from "@/components/shadcn/ui/sheet/SheetHeader.vue"
import SheetTitle from "@/components/shadcn/ui/sheet/SheetTitle.vue"
import {
	SIDEBAR_DEFAULT_WIDTH,
	SIDEBAR_MAX_WIDTH,
	SIDEBAR_MIN_WIDTH,
	SIDEBAR_WIDTH_MOBILE,
	useSidebar,
	useSidebarWidth,
} from "./utils"

defineOptions({
	inheritAttrs: false,
})

const props = withDefaults(
	defineProps<SidebarProps & { disableResizeHandle?: boolean }>(),
	{
		variant: "sidebar",
	},
)

const { isMobile, state, openMobile, setOpenMobile } = useSidebar()
const { setWidth } = useSidebarWidth()
const resizeHandle = useTemplateRef("resize-handle")
const isDragging = ref(false)
const currentWidth = ref(SIDEBAR_DEFAULT_WIDTH)

// Sync local width to shared context
watch(
	currentWidth,
	(value) => {
		setWidth(value)
	},
	{ immediate: true },
)

whenever(resizeHandle, () => {
	resizeHandle.value?.addEventListener("mousedown", startDrag)
	window.addEventListener("mouseup", endDrag)
	window.addEventListener("mousemove", moveDrag)

	if (import.meta.hot) {
		import.meta.hot.dispose(() => {
			resizeHandle.value?.removeEventListener("mousedown", startDrag)
			window.removeEventListener("mouseup", endDrag)
			window.removeEventListener("mousemove", moveDrag)
		})
	}
})
watch(isDragging, (newVal) => {
	if (newVal) {
		document.documentElement.classList.add(
			cn("select-none!"),
			cn("[&_*]:select-none!"),
			cn("cursor-col-resize!"),
			cn("[&_*]:cursor-col-resize!"),
		)
		return
	}

	document.documentElement.classList.remove(
		cn("select-none!"),
		cn("[&_*]:select-none!"),
		cn("cursor-col-resize!"),
		cn("[&_*]:cursor-col-resize!"),
	)
})

onBeforeUnmount(() => {
	resizeHandle.value?.removeEventListener("mousedown", startDrag)
	window.removeEventListener("mouseup", endDrag)
	window.removeEventListener("mousemove", moveDrag)
})

function startDrag() {
	isDragging.value = true
}

function endDrag() {
	isDragging.value = false
}

function moveDrag(e: MouseEvent) {
	if (!isDragging.value) {
		return
	}

	currentWidth.value = Math.min(
		SIDEBAR_MAX_WIDTH,
		Math.max(SIDEBAR_MIN_WIDTH, e.clientX),
	)
}
</script>

<template>
	<Sheet
		v-if="isMobile"
		:open="openMobile"
		v-bind="$attrs"
		@update:open="setOpenMobile"
	>
		<SheetContent
			data-sidebar="sidebar"
			data-slot="sidebar"
			data-mobile="true"
			side="left"
			class="w-(--sidebar-width) bg-sidebar p-0 text-sidebar-foreground [&>button]:hidden"
			:style="{
				'--sidebar-width': `${SIDEBAR_WIDTH_MOBILE}px`,
			}"
		>
			<SheetHeader class="sr-only">
				<SheetTitle>Sidebar</SheetTitle>
				<SheetDescription>Displays the mobile sidebar.</SheetDescription>
			</SheetHeader>
			<div class="flex h-full w-full flex-col">
				<slot />
			</div>
		</SheetContent>
	</Sheet>

	<div
		v-else
		class="group peer hidden text-sidebar-foreground md:block"
		data-slot="sidebar"
		:data-state="state"
		:data-collapsible="state === 'collapsed' ? 'offcanvas' : ''"
		:data-variant="variant"
		:data-dragging="isDragging ? 'true' : 'false'"
		data-side="left"
		:style="{
			'--sidebar-width': `${currentWidth}px`,
		}"
	>
		<!-- This is what handles the sidebar gap on desktop  -->
		<div
			:class="
				cn(
					'relative w-[calc(var(--sidebar-width)+var(--spacing)*0.25)] bg-transparent group-data-[dragging=false]:transition-all group-data-[dragging=false]:duration-200',
					'group-data-[collapsible=offcanvas]:w-0',
					'group-data-[side=right]:rotate-180',
				)
			"
		/>
		<div
			ref="resize-handle"
			:class="
				cn(
					'group/handle fixed inset-y-0 left-(--sidebar-width) z-sidebar flex w-0.75 cursor-col-resize group-data-[dragging=false]:transition-all group-data-[dragging=false]:duration-200',
					'group-data-[collapsible=offcanvas]:left-0',
					props.disableResizeHandle && 'pointer-events-none',
				)
			"
		>
			<div
				:class="
					cn(
						'h-full w-0.25 bg-border transition-[width] group-hover/handle:w-0.75',
					)
				"
			/>
		</div>
		<div
			:class="
				cn(
					'fixed inset-y-0 z-sidebar hidden h-svh w-(--sidebar-width) transition-[left,right] group-data-[dragging=false]:transition-[left,right,width] group-data-[dragging=false]:duration-200 md:flex',
					'left-0 group-data-[collapsible=offcanvas]:left-[calc(var(--sidebar-width)*-1)]',
					(variant === 'floating' || variant === 'inset') && 'p-2',
					props.class,
				)
			"
			v-bind="$attrs"
		>
			<div
				data-sidebar="sidebar"
				class="flex h-full w-full flex-col bg-sidebar group-data-[variant=floating]:rounded-lg group-data-[variant=floating]:border group-data-[variant=floating]:border-sidebar-border group-data-[variant=floating]:shadow-sm"
			>
				<slot />
			</div>
		</div>
	</div>
</template>
