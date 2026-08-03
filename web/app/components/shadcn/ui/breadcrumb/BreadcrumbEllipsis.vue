<script lang="ts" setup>
import type { HTMLAttributes } from "vue"
import { cn } from "@/lib/utils"

const props = defineProps<{
	class?: HTMLAttributes["class"]
}>()
</script>

<template>
	<span
		data-slot="breadcrumb-ellipsis"
		role="presentation"
		aria-hidden="true"
		:class="cn('flex h-9 w-4 items-center justify-center', props.class)"
	>
		<slot>
			<Icon name="lucide:ellipsis" class="mt-1.75" />
		</slot>
		<span class="sr-only">More</span>
	</span>
</template>
