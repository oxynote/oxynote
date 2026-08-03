<script setup lang="ts">
import type { PrimitiveProps } from "reka-ui"
import type { HTMLAttributes } from "vue"
import { Primitive } from "reka-ui"
import { cn } from "@/lib/utils"

const props = defineProps<
	PrimitiveProps & {
		class?: HTMLAttributes["class"]
	}
>()
</script>

<template>
	<Primitive
		data-slot="sidebar-group-action"
		data-sidebar="group-action"
		:as="as"
		:as-child="asChild"
		:class="
			cn(
				'absolute top-3.5 right-3 flex aspect-square w-5 cursor-pointer items-center justify-center rounded-md p-0 text-sidebar-foreground/70 ring-sidebar-ring outline-hidden transition-transform hover:bg-sidebar-accent/50 focus-visible:ring-2 active:bg-sidebar-accent [&>svg]:size-4 [&>svg]:shrink-0',
				'after:absolute after:-inset-2 md:after:hidden',
				'group-data-[collapsible=icon]:hidden',
				props.class,
			)
		"
	>
		<slot />
	</Primitive>
</template>
