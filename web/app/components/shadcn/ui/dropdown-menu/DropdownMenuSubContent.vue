<script setup lang="ts">
import type {
	DropdownMenuSubContentEmits,
	DropdownMenuSubContentProps,
} from "reka-ui"
import type { HTMLAttributes } from "vue"
import { reactiveOmit } from "@vueuse/core"
import { DropdownMenuSubContent, useForwardPropsEmits } from "reka-ui"
import { cn } from "@/lib/utils"

const props = withDefaults(
	defineProps<
		DropdownMenuSubContentProps & {
			class?: HTMLAttributes["class"]
			insideModal?: boolean
			insideSheet?: boolean
		}
	>(),
	{
		sideOffset: 8,
	},
)

const emits = defineEmits<DropdownMenuSubContentEmits>()

const delegatedProps = reactiveOmit(props, "class")

const forwarded = useForwardPropsEmits(delegatedProps, emits)
</script>

<template>
	<DropdownMenuSubContent
		data-slot="dropdown-menu-sub-content"
		v-bind="forwarded"
		:class="
			cn(
				'z-dropdown min-w-[8rem] origin-(--reka-dropdown-menu-content-transform-origin) overflow-hidden rounded-lg border bg-popover p-1 text-sm text-popover-foreground shadow-lg',
				props.insideModal && 'z-[calc(theme(zIndex.modal)+5)]!',
				props.insideSheet && 'z-[calc(theme(zIndex.sheet)+5)]!',
				props.class,
			)
		"
	>
		<slot />
	</DropdownMenuSubContent>
</template>
