<script lang="ts" setup>
import { cn } from "~/lib/utils"
import { useSidebar, useSidebarWidth } from "./shadcn/ui/sidebar"

const open = defineModel<boolean>({ default: false })
const emit = defineEmits<{
	(event: "notification-navigation"): void
}>()

const { open: sidebarOpen, state: sidebarState } = useSidebar()
const { width: sidebarWidth } = useSidebarWidth()
const isMobile = useMediaQuery("(max-width: 1250px)") // match SidebarProvider (needs to fit maintainers/reviewers and other name editor things)

const leftPosition = computed(() => {
	if (!open.value || sidebarState.value === "collapsed") {
		return 0
	}

	return sidebarWidth.value
})
</script>
<template>
	<ShadcnUiSheet
		v-if="isMobile"
		:open="open"
		@update:open="(value) => (open = value)"
	>
		<ShadcnUiSheetContent side="left" class="w-fit p-0">
			<ShadcnUiSheetHeader class="sr-only">
				<ShadcnUiSheetTitle>
					{{ $t("sidebar.notification-sheet.title") }}
				</ShadcnUiSheetTitle>
				<ShadcnUiSheetDescription>
					{{ $t("sidebar.notification-sheet.description") }}
				</ShadcnUiSheetDescription>
			</ShadcnUiSheetHeader>
			<NotificationBox
				v-model="open"
				class="w-80"
				mobile
				@close-notification-box="open = false"
				@notification-navigation="emit('notification-navigation')"
			/>
		</ShadcnUiSheetContent>
	</ShadcnUiSheet>
	<div
		v-else
		:class="
			cn(
				'grid transition-[grid-template-columns] duration-200 ease-out',
				open && sidebarOpen ? 'grid-cols-[20rem]' : 'grid-cols-[0rem]',
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
			<NotificationBox
				v-model="open"
				class="w-80"
				@notification-navigation="emit('notification-navigation')"
			/>
		</div>
	</div>
</template>
