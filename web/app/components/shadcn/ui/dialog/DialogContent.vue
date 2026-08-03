<script setup lang="ts">
import type { DialogContentEmits, DialogContentProps } from "reka-ui"
import type { HTMLAttributes } from "vue"
import { reactiveOmit } from "@vueuse/core"
import { DialogContent, DialogPortal, useForwardPropsEmits } from "reka-ui"
import { cn } from "@/lib/utils"
import DialogOverlay from "./DialogOverlay.vue"

const props = defineProps<
	DialogContentProps & {
		class?: HTMLAttributes["class"]
	}
>()
const emits = defineEmits<DialogContentEmits>()

const delegatedProps = reactiveOmit(props, "class")

const forwarded = useForwardPropsEmits(delegatedProps, emits)
</script>

<template>
	<DialogPortal>
		<DialogOverlay />
		<DialogContent
			data-slot="dialog-content"
			v-bind="forwarded"
			:class="
				cn(
					'fixed top-[50%] left-[50%] z-modal translate-x-[-50%] translate-y-[-50%] rounded-lg border bg-background p-6 shadow-lg',
					props.class,
				)
			"
		>
			<slot />
		</DialogContent>
	</DialogPortal>
</template>
