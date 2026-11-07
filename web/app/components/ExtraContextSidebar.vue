<script lang="ts" setup>
import { cn } from "#layers/ui/app/lib/utils"
import {
	useSidebar,
	useSidebarWidth,
} from "#layers/ui/app/components/shadcn/ui/sidebar"

const props = defineProps<{
	title: string
	description: string
}>()
const open = defineModel<boolean>({ default: false })
defineSlots<{
	default(props: { close: () => void }): unknown
}>()

const { open: sidebarOpen, state: sidebarState } = useSidebar()
const { width: sidebarWidth } = useSidebarWidth()
const isMobile = useMediaQuery("(max-width: 1250px)") // match SidebarProvider (needs to fit maintainers/reviewers and other name editor things)
const mounted = useMounted()

// the panel stays beside the sidebar while closed, so it slides shut from
// where it was instead of jumping under the sidebar first
const leftPosition = computed(() => {
	if (sidebarState.value === "collapsed") {
		return 0
	}

	return sidebarWidth.value
})

function close() {
	open.value = false
}
</script>
<template>
	<ShadcnUiSheet
		v-if="isMobile"
		:open="open"
		@update:open="(value) => (open = value)"
	>
		<ShadcnUiSheetContent side="left" class="w-fit p-0">
			<ShadcnUiSheetHeader class="sr-only">
				<ShadcnUiSheetTitle>{{ props.title }}</ShadcnUiSheetTitle>
				<ShadcnUiSheetDescription>
					{{ props.description }}
				</ShadcnUiSheetDescription>
			</ShadcnUiSheetHeader>
			<div class="h-svh w-80">
				<slot :close="close" />
			</div>
		</ShadcnUiSheetContent>
	</ShadcnUiSheet>
	<!--
		the server renders this column on any screen, and a narrow one swaps in
		the sheet only once the page runs, so the column hides below the same
		breakpoint (1250px) until then. A rem breakpoint follows the browser's
		font size and the switch above does not, so it stops once mounted.
	-->
	<div
		v-else
		:class="
			cn(
				'grid transition-[grid-template-columns] duration-200 ease-out print:hidden',
				!mounted && 'max-[78.125rem]:hidden',
				open && sidebarOpen
					? 'grid-cols-[clamp(20rem,22dvw,26.25rem)]'
					: 'grid-cols-[0rem]',
			)
		"
	>
		<div
			:class="
				cn(
					'fixed inset-y-0 h-svh overflow-hidden border-r border-border transition-transform duration-200 ease-out',
					open && sidebarOpen ? 'translate-x-0' : '-translate-x-full',
				)
			"
			:style="{ left: `${leftPosition}px` }"
		>
			<div class="h-svh w-[clamp(20rem,22dvw,26.25rem)]">
				<slot :close="close" />
			</div>
		</div>
	</div>
</template>
