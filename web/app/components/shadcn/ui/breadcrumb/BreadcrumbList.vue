<script lang="ts" setup>
import type { HTMLAttributes } from "vue"
import { cn } from "@/lib/utils"

const props = defineProps<{
	class?: HTMLAttributes["class"]
}>()
</script>

<template>
	<ol
		data-slot="breadcrumb-list"
		:class="
			cn(
				'flex shrink-0 items-center gap-1 text-sm text-muted-foreground sm:gap-1',
				props.class,
			)
		"
	>
		<slot />
	</ol>
</template>
