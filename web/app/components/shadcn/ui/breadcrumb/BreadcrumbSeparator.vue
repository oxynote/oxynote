<script lang="ts" setup>
import type { HTMLAttributes } from "vue"
import { cn } from "@/lib/utils"

const props = defineProps<{
	class?: HTMLAttributes["class"]
}>()
</script>

<template>
	<li
		data-slot="breadcrumb-separator"
		role="presentation"
		aria-hidden="true"
		:class="cn('flex items-center', props.class)"
	>
		<slot>
			<Icon name="lucide:chevron-right" class="mt-0.25" />
		</slot>
	</li>
</template>
