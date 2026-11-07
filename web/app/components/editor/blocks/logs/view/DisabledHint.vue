<script lang="ts" setup>
const props = defineProps<{
	reason: string
}>()

// a screen without hover cannot show a tooltip, so a tap opens the reason
const cannotHover = useMediaQuery("(hover: none)")
</script>
<template>
	<ShadcnUiPopover v-if="cannotHover">
		<ShadcnUiPopoverTrigger as="span" class="flex" tabindex="0">
			<slot />
		</ShadcnUiPopoverTrigger>
		<ShadcnUiPopoverContent side="bottom" class="w-fit max-w-64 p-2 text-xs">
			{{ props.reason }}
		</ShadcnUiPopoverContent>
	</ShadcnUiPopover>
	<ShadcnUiTooltip v-else>
		<ShadcnUiTooltipTrigger as="span" class="flex" tabindex="0">
			<slot />
		</ShadcnUiTooltipTrigger>
		<ShadcnUiTooltipContent side="bottom">
			{{ props.reason }}
		</ShadcnUiTooltipContent>
	</ShadcnUiTooltip>
</template>
