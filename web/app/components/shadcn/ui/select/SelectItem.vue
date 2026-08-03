<script setup lang="ts">
import type { SelectItemProps } from "reka-ui"
import type { HTMLAttributes } from "vue"
import { reactiveOmit } from "@vueuse/core"
import {
	SelectItem,
	SelectItemIndicator,
	SelectItemText,
	useForwardProps,
} from "reka-ui"
import { cn } from "@/lib/utils"

const props = defineProps<
	SelectItemProps & { class?: HTMLAttributes["class"] }
>()

const delegatedProps = reactiveOmit(props, "class")

const forwardedProps = useForwardProps(delegatedProps)
</script>

<template>
	<SelectItem
		data-slot="select-item"
		v-bind="forwardedProps"
		:class="
			cn(
				`relative flex w-full cursor-pointer items-center gap-2 rounded py-1.5 pr-8 pl-2 text-sm outline-hidden select-none focus:bg-accent/50 focus:text-accent-foreground active:bg-accent active:text-accent-foreground data-[disabled]:pointer-events-none data-[disabled]:opacity-50 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4 [&_svg:not([class*='text-'])]:text-muted-foreground *:[span]:last:flex *:[span]:last:items-center *:[span]:last:gap-2`,
				props.class,
			)
		"
	>
		<span class="absolute right-2 flex size-3.5 items-center justify-center">
			<SelectItemIndicator>
				<Icon name="lucide:check" class="mt-[0.25rem] size-4" />
			</SelectItemIndicator>
		</span>

		<SelectItemText class="truncate">
			<slot />
		</SelectItemText>
	</SelectItem>
</template>
