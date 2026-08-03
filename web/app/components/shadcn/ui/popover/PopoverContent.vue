<script setup lang="ts">
import type { PopoverContentEmits, PopoverContentProps } from "reka-ui"
import type { HTMLAttributes } from "vue"
import { reactiveOmit } from "@vueuse/core"
import { PopoverContent, PopoverPortal, useForwardPropsEmits } from "reka-ui"
import { cn } from "@/lib/utils"

defineOptions({
	inheritAttrs: false,
})

const props = withDefaults(
	defineProps<
		PopoverContentProps & {
			class?: HTMLAttributes["class"]
			insideModal?: boolean
			insideSheet?: boolean
		}
	>(),
	{
		align: "center",
		sideOffset: 4,
	},
)
const emits = defineEmits<PopoverContentEmits>()

const delegatedProps = reactiveOmit(props, "class")

const forwarded = useForwardPropsEmits(delegatedProps, emits)
</script>

<template>
	<PopoverPortal>
		<PopoverContent
			data-slot="popover-content"
			v-bind="{ ...forwarded, ...$attrs }"
			:class="
				cn(
					'z-popover min-w-40 origin-(--reka-popover-content-transform-origin) rounded-lg border bg-popover p-1 text-popover-foreground shadow-md outline-hidden data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:animate-in data-[state=open]:fade-in-0',
					props.insideModal && 'z-[calc(theme(zIndex.modal)+5)]!',
					props.insideSheet && 'z-[calc(theme(zIndex.sheet)+5)]!',
					props.class,
				)
			"
		>
			<slot />
		</PopoverContent>
	</PopoverPortal>
</template>
