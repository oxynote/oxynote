<script setup lang="ts">
const props = withDefaults(
	defineProps<{
		disabled?: boolean
		placeholder: string
		inputClass?: string
	}>(),
	{
		inputClass: "text-2sm md:text-2sm px-2 h-[1.775rem]",
	},
)
const value = defineModel<string | undefined>()
</script>
<template>
	<div class="mt-0.5 flex flex-col">
		<div class="mb-1 text-left text-2sm font-medium text-foreground">
			<slot name="label" />
		</div>
		<ShadcnUiInput
			v-model="value"
			:class="props.inputClass"
			type="url"
			:placeholder="props.placeholder"
			:disabled="props.disabled"
			disable-focus-effect
		/>
	</div>
</template>
