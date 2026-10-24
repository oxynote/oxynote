<script lang="ts" setup>
const color = defineModel<string | undefined>({
	required: true,
})
</script>
<template>
	<div class="grid w-fit grid-cols-4 gap-0.25">
		<ShadcnUiButton
			v-for="c in PALETTE_COLORS"
			:key="c"
			size="icon"
			variant="outline-transparent"
			class="size-fit border-0 p-1"
			@click="color = c"
		>
			<div
				class="size-4 rounded-full"
				:style="{ backgroundColor: paletteColor(c) }"
			/>
		</ShadcnUiButton>
	</div>
</template>
