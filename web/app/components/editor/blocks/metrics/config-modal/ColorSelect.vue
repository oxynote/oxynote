<script lang="ts" setup>
import { chartStyles } from "~/assets/css"

const color = defineModel<string | undefined>({
	required: true,
})
const st = chartStyles()
</script>
<template>
	<div class="grid w-fit grid-cols-4 gap-0.25">
		<ShadcnUiButton
			v-for="c in st.thresholdColors.available"
			:key="c"
			size="icon"
			variant="outline-transparent"
			class="size-fit border-0 p-1"
			@click="color = c"
		>
			<div class="size-4 rounded-full" :style="{ backgroundColor: c }" />
		</ShadcnUiButton>
	</div>
</template>
