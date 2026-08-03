<script setup lang="ts">
import type { HTMLAttributes } from "vue"
import { cn } from "@/lib/utils"
import { Input } from "@/components/shadcn/ui/input"

const props = defineProps<{
	class?: HTMLAttributes["class"]
	type?: string
	placeholder?: string
	transparentDisable?: boolean
	number?: boolean
}>()
</script>

<template>
	<Input
		v-if="!props.number"
		data-slot="input-group-control"
		:type="props.type"
		:placeholder="props.placeholder"
		:transparent-disable="props.transparentDisable"
		:class="
			cn(
				'h-full flex-1 rounded-none border-0 bg-transparent shadow-none focus-visible:ring-0 dark:bg-transparent',
				props.class,
			)
		"
	/>
	<NumberInput
		v-else
		data-slot="input-group-control"
		:type="props.type"
		:placeholder="props.placeholder"
		:transparent-disable="props.transparentDisable"
		:class="
			cn(
				'h-full flex-1 rounded-none border-0 bg-transparent shadow-none focus-visible:ring-0 dark:bg-transparent',
				props.class,
			)
		"
	/>
</template>
