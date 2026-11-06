<script lang="ts" setup>
const TOOLTIP_DELAY_MS = 1000

const props = defineProps<{
	kind: "labels" | "fields"
}>()
</script>
<template>
	<!-- the wait is long, so a reader who only rests the pointer on a row
	is not interrupted -->
	<ShadcnUiTooltip :delay-duration="TOOLTIP_DELAY_MS">
		<ShadcnUiTooltipTrigger as="span" class="opacity-40">
			<slot />
		</ShadcnUiTooltipTrigger>
		<ShadcnUiTooltipContent side="bottom">
			{{
				props.kind === "labels"
					? $t("editor.logs.view.labels-not-searched")
					: $t("editor.logs.view.fields-not-searched")
			}}
		</ShadcnUiTooltipContent>
	</ShadcnUiTooltip>
</template>
