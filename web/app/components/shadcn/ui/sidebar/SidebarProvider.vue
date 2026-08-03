<script setup lang="ts">
import type { HTMLAttributes, Ref } from "vue"
import { useMediaQuery, useVModel } from "@vueuse/core"
import { TooltipProvider } from "reka-ui"
import { computed, ref } from "vue"
import { cn } from "@/lib/utils"
import {
	provideSidebarContext,
	provideSidebarWidthContext,
	SIDEBAR_COOKIE_MAX_AGE,
	SIDEBAR_COOKIE_NAME,
	SIDEBAR_DEFAULT_WIDTH,
	SIDEBAR_MOBILE_BREAKPOINT,
} from "./utils"

const props = withDefaults(
	defineProps<{
		defaultOpen?: boolean
		open?: boolean
		class?: HTMLAttributes["class"]
	}>(),
	{
		defaultOpen: true,
		open: undefined,
	},
)

const emits = defineEmits<{
	"update:open": [open: boolean]
}>()

const isMobile = useMediaQuery(`(max-width: ${SIDEBAR_MOBILE_BREAKPOINT}px)`)
const openMobile = ref(false)

const persistedOpen = usePersistentState<boolean>({
	key: SIDEBAR_COOKIE_NAME,
	defaultValue: props.defaultOpen ?? false,
	cookie: {
		maxAge: SIDEBAR_COOKIE_MAX_AGE,
	},
})

const open = useVModel(props, "open", emits, {
	defaultValue: persistedOpen.value,
	passive: (props.open === undefined) as false,
}) as Ref<boolean>

function setOpen(value: boolean) {
	open.value = value // emits('update:open', value)
	persistedOpen.value = value
}

function setOpenMobile(value: boolean) {
	openMobile.value = value
}

// Helper to toggle the sidebar.
function toggleSidebar() {
	return isMobile.value
		? setOpenMobile(!openMobile.value)
		: setOpen(!open.value)
}

// We add a state so that we can do data-state="expanded" or "collapsed".
// This makes it easier to style the sidebar with Tailwind classes.
const state = computed(() => (open.value ? "expanded" : "collapsed"))

provideSidebarContext({
	state,
	open,
	setOpen,
	isMobile,
	openMobile,
	setOpenMobile,
	toggleSidebar,
})

const sidebarWidth = ref(SIDEBAR_DEFAULT_WIDTH)
provideSidebarWidthContext({
	width: sidebarWidth,
	setWidth: (value: number) => {
		sidebarWidth.value = value
	},
})
</script>

<template>
	<TooltipProvider :delay-duration="0">
		<div
			data-slot="sidebar-wrapper"
			:class="
				cn(
					'group/sidebar-wrapper flex min-h-svh w-full has-data-[variant=inset]:bg-sidebar',
					props.class,
				)
			"
			v-bind="$attrs"
		>
			<slot />
		</div>
	</TooltipProvider>
</template>
