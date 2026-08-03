<script setup lang="ts">
import type { DropdownMenuSubTriggerProps } from "reka-ui"
import type { HTMLAttributes } from "vue"
import { reactiveOmit } from "@vueuse/core"
import { DropdownMenuSubTrigger, useForwardProps } from "reka-ui"
import { cn } from "@/lib/utils"

const props = defineProps<
	DropdownMenuSubTriggerProps & {
		class?: HTMLAttributes["class"]
		inset?: boolean
	}
>()

const delegatedProps = reactiveOmit(props, "class", "inset")
const forwardedProps = useForwardProps(delegatedProps)
</script>

<template>
	<DropdownMenuSubTrigger
		data-slot="dropdown-menu-sub-trigger"
		v-bind="forwardedProps"
		:class="
			cn(
				'flex cursor-pointer items-center gap-1.5 rounded px-2 py-1.25 text-2sm outline-hidden select-none focus:bg-accent/50 focus:text-accent-foreground data-[inset]:pl-8 data-[state=open]:bg-accent/50 data-[state=open]:text-accent-foreground',
				props.class,
			)
		"
	>
		<slot />
		<Icon name="lucide:chevron-right" class="ml-auto shrink-0" />
	</DropdownMenuSubTrigger>
</template>
