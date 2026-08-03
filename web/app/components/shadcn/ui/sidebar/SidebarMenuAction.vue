<script setup lang="ts">
import type { PrimitiveProps } from "reka-ui"
import type { HTMLAttributes } from "vue"
import { Primitive } from "reka-ui"
import { cn } from "@/lib/utils"

const props = withDefaults(
	defineProps<
		PrimitiveProps & {
			side?: "left" | "right"
			showOnHover?: boolean
			disableDirectInteraction?: boolean
			class?: HTMLAttributes["class"]
		}
	>(),
	{
		side: "right",
		as: "button",
	},
)
</script>

<template>
	<Primitive
		data-slot="sidebar-menu-action"
		data-sidebar="menu-action"
		:data-sidebar-menu-action-side="props.side"
		:class="
			cn(
				'absolute top-1 flex aspect-square w-5 cursor-pointer items-center justify-center rounded-md p-0 ring-sidebar-ring outline-hidden transition-transform focus-visible:ring-2 [&>svg]:size-4 [&>svg]:shrink-0',
				'after:absolute after:-inset-2 md:after:hidden',
				'peer-data-[size=sm]/menu-button:top-1',
				'peer-data-[size=default]/menu-button:top-1.5',
				'peer-data-[size=lg]/menu-button:top-2.5',
				'group-data-[collapsible=icon]:hidden',
				!props.disableDirectInteraction &&
					'hover:bg-sidebar-accent/50 hover:text-sidebar-accent-foreground active:bg-sidebar-accent',
				props.disableDirectInteraction && 'pointer-events-none!',
				props.showOnHover && 'data-[state=open]:opacity-100 md:opacity-0',
				props.side === 'right' && 'right-1 text-sidebar-foreground/70',
				props.side === 'left' && 'left-1.25 text-sidebar-foreground/80',
				props.class,
			)
		"
		:as="as"
		:as-child="asChild"
	>
		<slot />
	</Primitive>
</template>
