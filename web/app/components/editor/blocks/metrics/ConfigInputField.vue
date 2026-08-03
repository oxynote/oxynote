<script setup lang="ts">
import ConfigField from "./ConfigField.vue"

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
	<ConfigField>
		<template #label>
			<slot name="label" />
		</template>
		<template #default>
			<ShadcnUiInput
				v-model="value"
				:class="props.inputClass"
				type="url"
				:placeholder="props.placeholder"
				:disabled="props.disabled"
				disable-focus-effect
			/>
		</template>
	</ConfigField>
</template>
