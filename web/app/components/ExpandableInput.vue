<script lang="ts" setup>
import type { InputProps } from "./shadcn/ui/input"
import { cn } from "~/lib/utils"

// NOTE: currently, only "ghost" inputs are supported

const props = defineProps<
	Omit<InputProps, "modelValue" | "ghost"> & {
		inputClass?: string
	}
>()
const value = defineModel<string>()
</script>

<template>
	<div :class="cn('relative inline-grid min-w-0', props.class)">
		<span
			aria-hidden="true"
			:class="
				cn(
					props.inputClass,
					'pointer-events-none invisible block whitespace-pre [grid-area:1/1]',
				)
			"
		>
			{{ value || props.placeholder }}
		</span>
		<ShadcnUiInput
			v-model="value"
			v-bind="{
				...props,
				ghost: true,
				class: cn(props.inputClass, 'absolute inset-0 w-full [grid-area:1/1]'),
			}"
		/>
	</div>
</template>
