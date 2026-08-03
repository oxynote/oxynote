<script setup lang="ts">
import type { TooltipContentEmits, TooltipContentProps } from "reka-ui"
import type { HTMLAttributes } from "vue"
import { reactiveOmit } from "@vueuse/core"
import {
	TooltipArrow,
	TooltipContent,
	TooltipPortal,
	useForwardPropsEmits,
} from "reka-ui"
import { cn } from "@/lib/utils"

defineOptions({
	inheritAttrs: false,
})

const props = withDefaults(
	defineProps<
		TooltipContentProps & {
			class?: HTMLAttributes["class"]
			enableArrow?: boolean
		}
	>(),
	{
		sideOffset: 4,
	},
)

const emits = defineEmits<TooltipContentEmits>()

const delegatedProps = reactiveOmit(props, "class")
const forwarded = useForwardPropsEmits(delegatedProps, emits)
</script>

<template>
	<TooltipPortal>
		<TooltipContent
			data-slot="tooltip-content"
			v-bind="{ ...forwarded, ...$attrs }"
			:class="
				cn(
					'z-tooltip w-fit animate-in rounded-md border bg-popover px-3 py-1.5 text-2sm text-balance text-popover-foreground fade-in-0 data-[state=closed]:animate-out data-[state=closed]:fade-out-0',
					props.class,
				)
			"
		>
			<slot />

			<TooltipArrow
				v-if="props.enableArrow"
				class="z-tooltip size-2.5 translate-y-[calc(-50%_-_2px)] rotate-45 rounded-[2px] bg-primary fill-primary"
			/>
		</TooltipContent>
	</TooltipPortal>
</template>
