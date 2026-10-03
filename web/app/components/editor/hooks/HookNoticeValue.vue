<script lang="ts" setup>
const props = defineProps<{
	// left out while the hook has nothing to name yet
	value?: string | null
	// what the sentence says in the value's place until then
	fallback?: string
}>()
</script>
<template>
	<span v-if="props.value" class="font-semibold text-foreground">
		{{ props.value }}
	</span>
	<template v-else>{{ props.fallback }}</template>
</template>
