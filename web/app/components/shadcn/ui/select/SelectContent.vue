<script setup lang="ts">
import type { SelectContentEmits, SelectContentProps } from "reka-ui"
import type { HTMLAttributes } from "vue"
import { reactiveOmit } from "@vueuse/core"
import {
	SelectContent,
	SelectPortal,
	SelectViewport,
	useForwardPropsEmits,
} from "reka-ui"
import { cn } from "@/lib/utils"
import { SelectScrollDownButton, SelectScrollUpButton } from "."

defineOptions({
	inheritAttrs: false,
})

const props = withDefaults(
	defineProps<
		SelectContentProps & {
			class?: HTMLAttributes["class"]
			scrollButtons?: boolean
			inlinePortal?: boolean
			insideModal?: boolean
			insideSheet?: boolean
		}
	>(),
	{
		position: "popper",
	},
)
const emits = defineEmits<SelectContentEmits>()

const delegatedProps = reactiveOmit(props, "class")

const forwarded = useForwardPropsEmits(delegatedProps, emits)
</script>

<template>
	<SelectPortal :disabled="props.inlinePortal">
		<SelectContent
			data-slot="select-content"
			v-bind="{ ...forwarded, ...$attrs }"
			:class="
				cn(
					'relative z-dropdown max-h-(--reka-select-content-available-height) min-w-[8rem] overflow-x-hidden overflow-y-auto rounded-lg border bg-popover text-popover-foreground shadow-md data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:animate-in data-[state=open]:fade-in-0',
					position === 'popper' &&
						'data-[side=bottom]:translate-y-1 data-[side=left]:-translate-x-1 data-[side=right]:translate-x-1 data-[side=top]:-translate-y-1',
					props.insideModal && 'z-[calc(theme(zIndex.modal)+5)]!',
					props.insideSheet && 'z-[calc(theme(zIndex.sheet)+5)]!',
					props.class,
				)
			"
		>
			<SelectScrollUpButton v-if="props.scrollButtons" />
			<SelectViewport
				:class="
					cn(
						'p-1',
						position === 'popper' &&
							'h-[var(--reka-select-trigger-height)] w-full min-w-[var(--reka-select-trigger-width)] scroll-my-1',
					)
				"
			>
				<slot />
			</SelectViewport>
			<SelectScrollDownButton v-if="props.scrollButtons" />
		</SelectContent>
	</SelectPortal>
</template>
