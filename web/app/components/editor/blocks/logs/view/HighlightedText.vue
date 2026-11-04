<script lang="ts" setup>
import { highlightParts } from "../rows"

const props = defineProps<{
	text: string
	search: string
}>()

const parts = computed(() => highlightParts(props.text, props.search))
</script>
<template>
	<template v-for="(part, index) in parts" :key="index">
		<mark
			v-if="part.match"
			class="rounded-sm bg-comment-highlight/40 text-inherit"
		>
			{{ part.text }}
		</mark>
		<template v-else>{{ part.text }}</template>
	</template>
</template>
