<script setup lang="ts">
import { cn } from "@/lib/utils"
import type { InputProps } from "."

const props = defineProps<InputProps>()
const model = defineModel<string | number>({ default: "" })
</script>

<template>
	<input
		v-model="model"
		data-slot="input"
		:placeholder="props.placeholder"
		:type="props.type"
		:readonly="props.transparentDisable"
		:class="
			cn(
				'flex bg-transparent text-base transition-[color,box-shadow] outline-none file:inline-flex file:h-7 file:border-0 file:bg-transparent file:text-sm file:font-medium file:text-foreground placeholder:text-muted-foreground disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50 md:text-sm',
				!props.ghost &&
					'w-full min-w-0 rounded-md border border-input px-3 py-1',
				!props.disableDestructiveEffect && 'aria-invalid:border-destructive',
				!props.disableFocusEffect &&
					'focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50',
				!props.disableFocusEffect &&
					!props.disableDestructiveEffect &&
					'aria-invalid:ring-destructive/20 dark:aria-invalid:ring-destructive/40',
				props.transparentDisable &&
					'cursor-default caret-transparent select-text', // don't use pointer-events-none to allow text selection
				props.class,
			)
		"
		@blur="
			(e) => {
				;(e.target as HTMLInputElement).scrollLeft = 0
			}
		"
	/>
</template>
