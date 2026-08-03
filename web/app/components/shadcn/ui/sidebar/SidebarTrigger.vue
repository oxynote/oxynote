<script setup lang="ts">
import type { HTMLAttributes } from "vue"
import { cn } from "@/lib/utils"
import { Button } from "@/components/shadcn/ui/button"
import { useSidebar } from "./utils"

const props = defineProps<{
	class?: HTMLAttributes["class"]
}>()

const { toggleSidebar } = useSidebar()
</script>

<template>
	<Button
		data-sidebar="trigger"
		data-slot="sidebar-trigger"
		variant="ghost"
		size="icon"
		:class="
			cn(
				'h-7 w-7 cursor-pointer hover:bg-sidebar-accent/50 active:bg-sidebar-accent',
				props.class,
			)
		"
		@click="toggleSidebar"
	>
		<Icon name="lucide:panel-left" />
		<span class="sr-only">{{ $t("sidebar.toggle-sidebar") }}</span>
	</Button>
</template>
