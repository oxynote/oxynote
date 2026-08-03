<script setup lang="ts">
import type { DropdownMenuItemProps } from "reka-ui"
import type { HTMLAttributes } from "vue"
import { reactiveOmit } from "@vueuse/core"
import { DropdownMenuItem, useForwardProps } from "reka-ui"
import { cn } from "@/lib/utils"

const props = withDefaults(
	defineProps<
		DropdownMenuItemProps & {
			class?: HTMLAttributes["class"]
			inset?: boolean
			variant?: "default" | "destructive"
			active?: boolean
		}
	>(),
	{
		variant: "default",
	},
)

const delegatedProps = reactiveOmit(props, "inset", "variant", "class")

const forwardedProps = useForwardProps(delegatedProps)
</script>

<template>
	<DropdownMenuItem
		data-slot="dropdown-menu-item"
		:data-inset="inset ? '' : undefined"
		:data-variant="variant"
		v-bind="forwardedProps"
		:class="
			cn(
				`relative flex cursor-pointer items-center gap-1.5 rounded px-2 py-1.25 text-2sm outline-hidden select-none focus:bg-accent/50 focus:text-accent-foreground active:bg-accent active:text-accent-foreground data-[disabled]:pointer-events-none data-[disabled]:opacity-50 data-[inset]:pl-8 data-[variant=destructive]:text-destructive-foreground data-[variant=destructive]:focus:bg-destructive/10 data-[variant=destructive]:focus:text-destructive-foreground dark:data-[variant=destructive]:focus:bg-destructive/40 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4 [&_svg:not([class*='text-'])]:text-muted-foreground data-[variant=destructive]:*:[svg]:!text-destructive-foreground`,
				props.class,
			)
		"
	>
		<slot />
		<Icon v-if="props.active" name="lucide:check" />
	</DropdownMenuItem>
</template>
