<script setup lang="ts">
import type {
	DropdownMenuContentEmits,
	DropdownMenuContentProps,
} from "reka-ui"
import type { HTMLAttributes } from "vue"
import { reactiveOmit } from "@vueuse/core"
import {
	DropdownMenuContent,
	DropdownMenuPortal,
	useForwardPropsEmits,
} from "reka-ui"
import { cn } from "@/lib/utils"

const props = withDefaults(
	defineProps<
		DropdownMenuContentProps & {
			class?: HTMLAttributes["class"]
			insideModal?: boolean
			insideSheet?: boolean
		}
	>(),
	{
		sideOffset: 4,
	},
)
const emits = defineEmits<DropdownMenuContentEmits>()

const delegatedProps = reactiveOmit(props, "class")

const forwarded = useForwardPropsEmits(delegatedProps, emits)
</script>

<template>
	<DropdownMenuPortal>
		<DropdownMenuContent
			data-slot="dropdown-menu-content"
			v-bind="forwarded"
			:class="
				cn(
					'z-dropdown max-h-(--reka-dropdown-menu-content-available-height) min-w-[8rem] overflow-x-hidden overflow-y-auto rounded-lg border bg-popover p-1 text-popover-foreground shadow-md data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:animate-in data-[state=open]:fade-in-0',
					props.insideModal && 'z-[calc(theme(zIndex.modal)+5)]!',
					props.insideSheet && 'z-[calc(theme(zIndex.sheet)+5)]!',
					props.class,
				)
			"
		>
			<slot />
		</DropdownMenuContent>
	</DropdownMenuPortal>
</template>
