<script lang="ts" setup>
import { chartStyles } from "~/assets/css"

const props = defineProps<{
	// the colours on offer, by default the theme's selectable css colours
	colors?: string[]
	// a colour that is not css itself, such as a palette name, is painted
	// with what this returns
	paint?: (color: string) => string
}>()
const color = defineModel<string | undefined>({
	required: true,
})

const colors = computed(
	() => props.colors ?? chartStyles().selectableColors.available,
)
</script>
<template>
	<div class="grid w-fit grid-cols-4 gap-0.25">
		<ShadcnUiButton
			v-for="c in colors"
			:key="c"
			size="icon"
			variant="outline-transparent"
			class="size-fit border-0 p-1"
			@click="color = c"
		>
			<div
				class="size-4 rounded-full"
				:style="{ backgroundColor: props.paint ? props.paint(c) : c }"
			/>
		</ShadcnUiButton>
	</div>
</template>
