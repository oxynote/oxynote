<script setup lang="ts">
import type { SelectTriggerProps } from "reka-ui"
import type { HTMLAttributes } from "vue"
import { reactiveOmit } from "@vueuse/core"
import { SelectIcon, SelectTrigger, useForwardProps } from "reka-ui"
import { cn } from "@/lib/utils"

const props = withDefaults(
	defineProps<
		SelectTriggerProps & {
			class?: HTMLAttributes["class"]
			size?: "sm" | "default" | "custom"
			ghost?: boolean
			disable?: boolean
			disableDefaultStyles?: boolean
		}
	>(),
	{ size: "default" },
)

const delegatedProps = reactiveOmit(props, "class", "size")
const forwardedProps = useForwardProps(delegatedProps)
</script>

<template>
	<SelectTrigger
		data-slot="select-trigger"
		:data-size="size"
		v-bind="forwardedProps"
		:class="
			cn(
				!props.disableDefaultStyles &&
					`flex w-fit cursor-pointer items-center justify-between gap-1 rounded-md bg-transparent text-sm whitespace-nowrap transition-[color,box-shadow] outline-none disabled:cursor-not-allowed disabled:opacity-50 aria-invalid:border-destructive aria-invalid:ring-destructive/20 data-[placeholder]:text-muted-foreground data-[size=default]:h-9 data-[size=sm]:h-8 *:data-[slot=select-value]:line-clamp-1 *:data-[slot=select-value]:flex *:data-[slot=select-value]:items-center *:data-[slot=select-value]:gap-2 dark:aria-invalid:ring-destructive/40 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4 [&_svg:not([class*='text-'])]:text-muted-foreground`,
				!props.ghost &&
					!props.disableDefaultStyles &&
					`border border-input px-1.5 py-1 shadow-xs`,
				props.ghost &&
					!props.disable &&
					!props.disableDefaultStyles &&
					`hover:opacity-70`,
				props.disable &&
					!props.disableDefaultStyles &&
					`pointer-events-none select-text`,
				props.class,
			)
		"
	>
		<slot />
		<SelectIcon
			v-if="!props.disableDefaultStyles"
			v-show="!props.disable"
			as-child
		>
			<Icon name="lucide:chevron-down" class="mt-[0.15rem] size-4" />
		</SelectIcon>
	</SelectTrigger>
</template>
