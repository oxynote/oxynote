<script setup lang="ts">
defineProps<{
	hook?: DocumentHook | null
}>()
</script>

<template>
	<template v-if="hook && !hook.state && hook.status === 'active'">
		<div
			class="flex items-center justify-center gap-1 px-0.75 pb-0.75 text-2sm text-muted-foreground"
		>
			<Icon name="mingcute:loading-line" class="shrink-0 animate-spin" />
			<span>{{ $t("editor.hooks.initializing") }}</span>
		</div>
		<ShadcnUiDropdownMenuSeparator />
	</template>
</template>
