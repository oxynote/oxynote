<script lang="ts" setup>
const props = defineProps<{
	// the hook's messages sit under its type in editor.hooks
	type: DocumentHookType
	// a triggered hook says what it did, one still waiting what it will do
	triggered: boolean
	nodeId: string | null // null means global
}>()

const keypath = computed(
	() =>
		`editor.hooks.${props.type}.${props.triggered ? "triggered" : "existing"}-item-${props.nodeId ? "block" : "full-document"}-explanation`,
)
</script>

<template>
	<div class="flex flex-col gap-1 px-0.75 pb-0.75 text-2sm">
		<i18n-t
			scope="global"
			:keypath="keypath"
			tag="div"
			class="w-full text-center text-xs break-words text-muted-foreground"
		>
			<template #value>
				<slot name="value" />
			</template>
		</i18n-t>
	</div>
</template>
